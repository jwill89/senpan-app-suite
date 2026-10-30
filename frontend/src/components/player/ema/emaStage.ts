/**
 * The Konva side of the Ema designer: the canvas, its layers, selection handles,
 * the eraser and the PNG export. EmaDesigner.vue owns the state (the layer list
 * and its undo history) and hands every version of it to `setLayers`; this class
 * draws what it is given and reports back what the visitor did with the pointer,
 * so the component never touches a Konva node and this file never touches Vue.
 *
 * Coordinates: everything is laid out in TEMPLATE pixels - the plaque PNG's own
 * size - and the stage is scaled to fit the screen. A layer's position and size,
 * the eraser brush and the export all use those units, whatever the window does.
 *
 * Three Konva layers, each its own canvas:
 *   - `base`: the plaque template, never editable;
 *   - `design`: one Group per visitor layer, then the "keep inside" clip;
 *   - `ui`: the selection handles and the eraser ring, left out of the export.
 *
 * Imported piecemeal (Core plus the five shapes it uses) rather than from `konva`,
 * which bundles every shape and filter.
 */
import Konva from 'konva/lib/Core'
import type { Group } from 'konva/lib/Group'
import type { Layer } from 'konva/lib/Layer'
import type { KonvaEventObject } from 'konva/lib/Node'
import type { Stage } from 'konva/lib/Stage'
import { Circle } from 'konva/lib/shapes/Circle'
import { Image as KonvaImage } from 'konva/lib/shapes/Image'
import { Line } from 'konva/lib/shapes/Line'
import { Text as KonvaText } from 'konva/lib/shapes/Text'
import { Transformer } from 'konva/lib/shapes/Transformer'
import {
  type Affine,
  type EmaLayer,
  type EmaLayers,
  type EmaStroke,
  type EmaTextAlign,
  type Size,
  applyAffine,
  exportSize,
  fitScale,
  invertAffine,
  konvaFontStyle,
  localBrushWidth,
  reachableShift,
  roundCoord,
  textAnchor,
} from '@/lib/emaDesigner'

export type EmaTool = 'move' | 'erase'

/** A picture's pixels: the decoded upload, or the canvas it was shrunk onto. */
export type EmaAsset = HTMLImageElement | HTMLCanvasElement

/** What the visitor did on the canvas. */
export interface EmaStageEvents {
  /** A layer was tapped, or the empty plaque (null). */
  select: (id: string | null) => void
  /** A drag, resize or turn finished. */
  transform: (
    id: string,
    placement: { x: number; y: number; rotation: number; scaleX: number; scaleY: number },
  ) => void
  /** An eraser stroke finished on a layer. */
  stroke: (id: string, stroke: EmaStroke) => void
}

/** Colors for the handles and the eraser ring, read from the theme by the component. */
export interface EmaStageColors {
  handle: string
  handleFill: string
}

type LayerNodes = {
  group: Group
  /** The stroke list last drawn, compared by identity to skip rebuilding the lines. */
  strokes: readonly EmaStroke[] | null
  lines: Line[]
} & ({ kind: 'image'; content: KonvaImage } | { kind: 'text'; content: KonvaText })

interface ActiveStroke {
  id: string
  pointerId: number
  nodes: LayerNodes
  line: Line
  /** Stage pixels -> the layer's own coordinates, fixed for the stroke. */
  toLocal: Affine
  width: number
  points: number[]
}

/** A dragged-off layer is pulled back until this much of it is on the plaque. */
const REACHABLE_MARGIN = 24
/** Pointer moves shorter than this, in the layer's units, add no point. */
const MIN_STROKE_STEP = 0.75

function asAffine(m: number[]): Affine {
  return [m[0] ?? 1, m[1] ?? 0, m[2] ?? 0, m[3] ?? 1, m[4] ?? 0, m[5] ?? 0]
}

export class EmaStage {
  private readonly stage: Stage
  private readonly base: Layer
  private readonly design: Layer
  private readonly ui: Layer
  private readonly plaque: KonvaImage
  private readonly clip: KonvaImage
  private readonly transformer: Transformer
  private readonly ring: Circle
  private readonly nodes = new Map<string, LayerNodes>()
  private selectedId: string | null = null
  private tool: EmaTool = 'move'
  private brushSize = 24
  /** The template's natural size: the design's coordinate space. */
  private size: Size = { width: 1, height: 1 }
  /** Screen pixels per template pixel. */
  private scale = 1
  private active: ActiveStroke | null = null

  constructor(
    container: HTMLDivElement,
    private readonly assets: ReadonlyMap<string, EmaAsset>,
    private readonly events: EmaStageEvents,
    colors: EmaStageColors,
  ) {
    this.stage = new Konva.Stage({ container, width: 1, height: 1 })
    this.base = new Konva.Layer({ listening: false })
    this.design = new Konva.Layer()
    this.ui = new Konva.Layer()

    this.plaque = new KonvaImage({ image: undefined })
    this.base.add(this.plaque)

    // The "keep inside the plaque" pass. Drawn last on the design canvas with
    // destination-in, it keeps only the design pixels that sit over the template's
    // opaque ones. It is the design layer's own canvas, so the plaque underneath is
    // untouched, and the export composites the layers the same way.
    this.clip = new KonvaImage({
      image: undefined,
      listening: false,
      globalCompositeOperation: 'destination-in',
    })
    this.design.add(this.clip)

    const coarse = window.matchMedia('(pointer: coarse)').matches
    this.transformer = new Transformer({
      keepRatio: true,
      flipEnabled: false,
      enabledAnchors: ['top-left', 'top-right', 'bottom-left', 'bottom-right'],
      rotationSnaps: [0, 90, 180, 270],
      rotationSnapTolerance: 4,
      anchorSize: coarse ? 18 : 11,
      rotateAnchorOffset: coarse ? 36 : 26,
      anchorStroke: colors.handle,
      anchorFill: colors.handleFill,
      borderStroke: colors.handle,
      borderStrokeWidth: 1.5,
      // A finger needs more than the drawn square to land on: widen what counts
      // as a hit without drawing a bigger handle over the design.
      anchorStyleFunc: coarse ? (anchor) => anchor.hitStrokeWidth(26) : undefined,
      // Never let a resize collapse a layer to a sliver that can't be grabbed again.
      boundBoxFunc: (previous, next) =>
        Math.abs(next.width) < 12 || Math.abs(next.height) < 12 ? previous : next,
    })
    this.ring = new Circle({
      visible: false,
      listening: false,
      stroke: colors.handle,
      strokeWidth: 1.5,
      strokeScaleEnabled: false,
      shadowColor: colors.handleFill,
      shadowBlur: 2,
      shadowOpacity: 0.9,
    })
    this.ui.add(this.transformer, this.ring)
    this.stage.add(this.base, this.design, this.ui)

    this.stage.on('pointerdown', (e: KonvaEventObject<PointerEvent>) => {
      this.onPointerDown(e)
    })
    this.stage.on('pointermove', () => {
      this.moveRing()
    })
    container.addEventListener('pointerleave', this.hideRing)
  }

  // -- Setup -------------------------------------------------------------------

  /** Puts the plaque in place; its natural size becomes the design's coordinate space. */
  setTemplate(image: HTMLImageElement): void {
    this.size = { width: image.naturalWidth, height: image.naturalHeight }
    const box = { image, width: this.size.width, height: this.size.height }
    this.plaque.setAttrs(box)
    this.clip.setAttrs(box)
    this.base.batchDraw()
    this.design.batchDraw()
  }

  /** The template's natural size. */
  get designSize(): Size {
    return this.size
  }

  /** Scales the stage to the largest size that fits `box`. */
  fit(box: Size): void {
    const scale = fitScale(this.size, box)
    if (scale <= 0) return
    // A whole number of CSS pixels across, so the canvas has no half-pixel seam.
    const width = Math.max(1, Math.floor(this.size.width * scale))
    this.scale = width / this.size.width
    this.stage.size({ width, height: Math.round(this.size.height * this.scale) })
    this.stage.scale({ x: this.scale, y: this.scale })
    this.refreshHandles()
    this.stage.batchDraw()
  }

  // -- Layers ------------------------------------------------------------------

  /** Brings the canvas in line with `layers`, bottom first. */
  setLayers(layers: EmaLayers): void {
    const finish = Konva.beginUpdateBatch()
    try {
      const seen = new Set<string>()
      layers.forEach((layer, index) => {
        seen.add(layer.id)
        let nodes = this.nodes.get(layer.id)
        if (nodes && nodes.kind !== layer.kind) {
          nodes.group.destroy()
          nodes = undefined
        }
        nodes ??= this.createNodes(layer)
        this.applyLayer(nodes, layer)
        nodes.group.zIndex(index)
      })
      for (const [id, nodes] of this.nodes) {
        if (seen.has(id)) continue
        if (this.active?.id === id) this.cancelStroke()
        nodes.group.destroy()
        this.nodes.delete(id)
      }
      this.clip.moveToTop()
      this.refreshHandles()
    } finally {
      finish()
    }
    this.design.batchDraw()
  }

  select(id: string | null): void {
    this.selectedId = id
    this.refreshHandles()
  }

  setTool(tool: EmaTool): void {
    if (tool === this.tool) return
    this.finishStroke()
    this.tool = tool
    // While erasing, every press belongs to the brush: nothing on the design
    // canvas should catch it, and no layer should start a drag.
    this.design.listening(tool === 'move')
    for (const nodes of this.nodes.values()) nodes.group.draggable(tool === 'move')
    if (tool === 'move') this.hideRing()
    this.refreshHandles()
  }

  /** Eraser width, in template pixels. */
  setBrushSize(size: number): void {
    this.brushSize = size
    this.ring.radius(size / 2)
    this.ui.batchDraw()
  }

  setClip(on: boolean): void {
    this.clip.visible(on)
    this.design.batchDraw()
  }

  /**
   * Measures every text layer again. Konva measures text when its attributes
   * change, so a web font that finishes downloading afterwards would otherwise
   * keep the fallback face's box - and draw the real face into it.
   */
  refreshText(): void {
    for (const nodes of this.nodes.values()) {
      if (nodes.kind !== 'text') continue
      nodes.content._setTextData()
      this.anchorText(nodes.content)
    }
    this.refreshHandles()
    this.design.batchDraw()
  }

  // -- Export ------------------------------------------------------------------

  /**
   * The finished plaque as a PNG: the template and every visible layer, clipped
   * when "keep inside" is on, at twice the template's resolution (capped). The
   * selection handles and the eraser ring are on the `ui` layer, which is hidden
   * for the duration - the draw is synchronous, so it never shows on screen.
   */
  async toPng(): Promise<Blob> {
    this.finishStroke()
    const out = exportSize(this.size.width, this.size.height)
    const stageSize = { width: this.stage.width(), height: this.stage.height() }
    let drawn: HTMLCanvasElement
    this.ui.visible(false)
    try {
      drawn = this.stage.toCanvas({
        x: 0,
        y: 0,
        ...stageSize,
        pixelRatio: out.width / stageSize.width,
      })
    } finally {
      this.ui.visible(true)
    }
    // The stage's height is rounded to whole pixels, so its scaled copy can land a
    // pixel short of the exact export size. Settle it on a canvas of that size.
    let canvas = drawn
    if (drawn.width !== out.width || drawn.height !== out.height) {
      canvas = document.createElement('canvas')
      canvas.width = out.width
      canvas.height = out.height
      canvas.getContext('2d')?.drawImage(drawn, 0, 0, out.width, out.height)
      // Hand the scratch bitmap back now rather than at garbage collection: iOS
      // caps the total canvas memory a page may hold.
      drawn.width = drawn.height = 0
    }
    return new Promise((resolve, reject) => {
      canvas.toBlob((blob) => {
        if (blob) resolve(blob)
        else reject(new Error('The plaque could not be saved as a PNG.'))
      }, 'image/png')
    })
  }

  destroy(): void {
    this.cancelStroke()
    this.stage.container().removeEventListener('pointerleave', this.hideRing)
    this.stage.destroy()
    this.nodes.clear()
  }

  // -- Internals ---------------------------------------------------------------

  private createNodes(layer: EmaLayer): LayerNodes {
    // `isolated` gives the group its own surface on every draw: the eraser's
    // destination-out lines inside it rub out only this layer's pixels, never the
    // layers beneath on the shared design canvas. It draws live at whatever pixel
    // ratio the destination has, so the export gets full-resolution erasing too.
    const group = new Konva.Group({ id: layer.id, isolated: true })
    const nodes: LayerNodes =
      layer.kind === 'image'
        ? {
            kind: 'image',
            content: new KonvaImage({ image: undefined }),
            group,
            strokes: null,
            lines: [],
          }
        : {
            kind: 'text',
            // Stroke first, fill over it: an outline then frames the letters
            // instead of eating into them.
            content: new KonvaText({ fillAfterStrokeEnabled: true, lineHeight: 1.15 }),
            group,
            strokes: null,
            lines: [],
          }
    const content = nodes.content
    group.add(content)
    // The eraser strokes are children of the group too, and a stroke that runs
    // off the edge of a picture - which is how anyone rubs out an overhang - would
    // stretch the selection box and the isolation surface out to wherever the
    // pointer went. The layer's bounds are its content's. With `skipTransform` the
    // caller wants them in the group's own space, which for text is offset from
    // the content's own (see anchorText), so that case is measured from the group.
    group.getClientRect = (config = {}) =>
      config.skipTransform
        ? content.getClientRect({ ...config, skipTransform: false, relativeTo: group })
        : content.getClientRect(config)
    group.on('dragend', () => {
      this.keepReachable(group)
      this.emitPlacement(group)
    })
    group.on('transformend', () => {
      this.emitPlacement(group)
    })
    group.draggable(this.tool === 'move')
    this.design.add(group)
    this.nodes.set(layer.id, nodes)
    return nodes
  }

  private applyLayer(nodes: LayerNodes, layer: EmaLayer): void {
    nodes.group.setAttrs({
      x: layer.x,
      y: layer.y,
      rotation: layer.rotation,
      scaleX: layer.scaleX,
      scaleY: layer.scaleY,
      opacity: layer.opacity,
      visible: layer.visible,
    })
    if (nodes.kind === 'image' && layer.kind === 'image') {
      nodes.content.setAttrs({
        image: this.assets.get(layer.assetId),
        width: layer.width,
        height: layer.height,
      })
    } else if (nodes.kind === 'text' && layer.kind === 'text') {
      nodes.content.setAttrs({
        text: layer.text,
        fontFamily: layer.fontFamily,
        fontSize: layer.fontSize,
        fontStyle: konvaFontStyle(layer.bold, layer.italic),
        align: layer.align,
        fill: layer.fill,
        stroke: layer.stroke,
        strokeWidth: layer.strokeWidth,
        strokeEnabled: layer.strokeWidth > 0,
      })
      this.anchorText(nodes.content)
    }
    if (nodes.strokes !== layer.strokes) {
      for (const line of nodes.lines) line.destroy()
      nodes.lines = layer.strokes.map((s) => this.strokeLine(s.size, s.points))
      if (nodes.lines.length) nodes.group.add(...nodes.lines)
      nodes.strokes = layer.strokes
    }
  }

  private anchorText(text: KonvaText): void {
    text.offset(textAnchor(text.align() as EmaTextAlign, text.size()))
  }

  private strokeLine(width: number, points: number[]): Line {
    return new Line({
      points,
      stroke: '#000',
      strokeWidth: width,
      lineCap: 'round',
      lineJoin: 'round',
      globalCompositeOperation: 'destination-out',
      listening: false,
    })
  }

  private emitPlacement(group: Group): void {
    this.events.transform(group.id(), {
      x: group.x(),
      y: group.y(),
      rotation: group.rotation(),
      scaleX: group.scaleX(),
      scaleY: group.scaleY(),
    })
  }

  private keepReachable(group: Group): void {
    const shift = reachableShift(
      group.getClientRect({ relativeTo: this.stage }),
      this.size,
      REACHABLE_MARGIN,
    )
    if (shift.x || shift.y) group.position({ x: group.x() + shift.x, y: group.y() + shift.y })
  }

  private refreshHandles(): void {
    const nodes = this.selectedId ? this.nodes.get(this.selectedId) : undefined
    const target = nodes?.group.visible() ? [nodes.group] : []
    const editable = this.tool === 'move'
    this.transformer.setAttrs({ resizeEnabled: editable, rotateEnabled: editable })
    this.transformer.nodes(target)
    if (target.length) this.transformer.forceUpdate()
    this.ui.batchDraw()
  }

  // -- Pointer -----------------------------------------------------------------

  private onPointerDown(e: KonvaEventObject<PointerEvent>): void {
    if (this.tool === 'move') {
      if (e.target === this.stage) {
        this.events.select(null)
        return
      }
      const group = e.target.getParent()
      if (group?.getParent() === this.design) this.events.select(group.id())
      return
    }
    if (e.evt.button === 0) this.startStroke(e.evt.pointerId)
  }

  private startStroke(pointerId: number): void {
    const nodes = this.selectedId ? this.nodes.get(this.selectedId) : undefined
    const pos = this.stage.getPointerPosition()
    if (!nodes?.group.visible() || !pos) return
    const absolute = asAffine(nodes.group.getAbsoluteTransform().getMatrix())
    const toLocal = invertAffine(absolute)
    if (!toLocal) return
    const start = applyAffine(toLocal, pos)
    const width = localBrushWidth(this.brushSize, this.scale, absolute)
    // Two copies of the first point, so a single tap leaves a round dot.
    const points = [start.x, start.y, start.x, start.y]
    const line = this.strokeLine(width, points)
    nodes.group.add(line)
    this.active = { id: nodes.group.id(), pointerId, nodes, line, toLocal, width, points }
    // Followed on the window rather than the stage, so a stroke that runs off the
    // canvas and back keeps going, and one let go outside it still ends.
    window.addEventListener('pointermove', this.extendStroke)
    window.addEventListener('pointerup', this.endStroke)
    window.addEventListener('pointercancel', this.endStroke)
    this.design.batchDraw()
  }

  private readonly extendStroke = (e: PointerEvent): void => {
    const active = this.active
    if (e.pointerId !== active?.pointerId) return
    this.stage.setPointersPositions(e)
    const pos = this.stage.getPointerPosition()
    if (!pos) return
    const p = applyAffine(active.toLocal, pos)
    const lastX = active.points[active.points.length - 2] ?? p.x
    const lastY = active.points[active.points.length - 1] ?? p.y
    if (Math.hypot(p.x - lastX, p.y - lastY) < MIN_STROKE_STEP) return
    active.points.push(p.x, p.y)
    active.line.points(active.points)
    this.moveRing()
    this.design.batchDraw()
  }

  private readonly endStroke = (e: PointerEvent): void => {
    if (e.pointerId !== this.active?.pointerId) return
    this.finishStroke()
    // A lifted finger or pen is gone; only a mouse is still hovering where it was.
    if (e.pointerType !== 'mouse') this.hideRing()
  }

  /** Ends the stroke in progress and reports it. */
  private finishStroke(): void {
    const active = this.active
    if (!active) return
    this.cancelStroke()
    this.events.stroke(active.id, {
      size: roundCoord(active.width),
      points: active.points.map(roundCoord),
    })
  }

  /**
   * Drops the stroke in progress without reporting it. Its line goes too: the
   * reported stroke comes back through `setLayers` as part of the layer.
   */
  private cancelStroke(): void {
    const active = this.active
    if (!active) return
    this.active = null
    window.removeEventListener('pointermove', this.extendStroke)
    window.removeEventListener('pointerup', this.endStroke)
    window.removeEventListener('pointercancel', this.endStroke)
    active.line.destroy()
    this.design.batchDraw()
  }

  private moveRing(): void {
    const pos = this.stage.getPointerPosition()
    if (this.tool !== 'erase' || !pos) return
    this.ring.setAttrs({
      x: pos.x / this.scale,
      y: pos.y / this.scale,
      radius: this.brushSize / 2,
      visible: true,
    })
    this.ui.batchDraw()
  }

  private readonly hideRing = (): void => {
    if (this.active) return
    this.ring.visible(false)
    this.ui.batchDraw()
  }
}
