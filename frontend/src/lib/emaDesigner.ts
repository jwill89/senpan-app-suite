/**
 * Ema designer - the logic behind `components/player/ema/EmaDesigner.vue` that
 * needs no canvas: the layer model and its edits, the undo history, export and
 * upload sizing, the point conversions the eraser relies on, and the font list.
 *
 * Kept apart from the component because jsdom has no canvas, so Konva cannot run
 * under the test runner. Everything here is plain data in, plain data out.
 */

// -- Sizes ---------------------------------------------------------------------

/** The PNG is drawn at twice the template's own resolution... */
export const EMA_EXPORT_SCALE = 2
/** ...but never wider than this, which Discord shows at full size. */
export const EMA_EXPORT_MAX_WIDTH = 2048
/**
 * Longest side an uploaded picture is kept at. A phone photo is 4000+ px a side,
 * about 64 MB once decoded, and a plaque only ever shows it a few hundred pixels
 * across.
 */
export const EMA_UPLOAD_MAX_SIDE = 2048
/** Undo steps kept. Each one is a full snapshot, eraser strokes included. */
export const EMA_HISTORY_LIMIT = 60

/** A width and height in pixels. */
export interface Size {
  width: number
  height: number
}

/** The exported PNG's pixel size for a template of the given natural size. */
export function exportSize(templateWidth: number, templateHeight: number): Size {
  const width = Math.min(Math.round(templateWidth * EMA_EXPORT_SCALE), EMA_EXPORT_MAX_WIDTH)
  return { width, height: Math.round((width * templateHeight) / templateWidth) }
}

/**
 * The size an upload is redrawn at so neither side passes `maxSide`, keeping its
 * proportions. `scaled` is false when the picture already fits and can be used
 * as it is.
 */
export function downscaleSize(
  width: number,
  height: number,
  maxSide = EMA_UPLOAD_MAX_SIDE,
): Size & { scaled: boolean } {
  const longest = Math.max(width, height)
  if (longest <= maxSide) return { width, height, scaled: false }
  const ratio = maxSide / longest
  return {
    width: Math.max(1, Math.round(width * ratio)),
    height: Math.max(1, Math.round(height * ratio)),
    scaled: true,
  }
}

/** The largest scale at which `content` fits inside `box`; 0 when either is empty. */
export function fitScale(content: Size, box: Size): number {
  if (content.width <= 0 || content.height <= 0 || box.width <= 0 || box.height <= 0) return 0
  return Math.min(box.width / content.width, box.height / content.height)
}

/**
 * The scale a newly added picture is placed at: small enough to take up at most
 * `share` of the plaque either way, and never blown up past its own pixels.
 */
export function placementScale(picture: Size, plaque: Size, share = 0.6): number {
  const fit = fitScale(picture, { width: plaque.width * share, height: plaque.height * share })
  return fit > 0 ? Math.min(1, fit) : 1
}

// -- Points and transforms -------------------------------------------------------

/** A point in some coordinate space. */
export interface Point {
  x: number
  y: number
}

/** A 2D affine matrix in canvas order `[a, b, c, d, e, f]` (Konva's `Transform.m`). */
export type Affine = readonly [number, number, number, number, number, number]

/** `m` applied to `p`. */
export function applyAffine(m: Affine, p: Point): Point {
  const [a, b, c, d, e, f] = m
  return { x: a * p.x + c * p.y + e, y: b * p.x + d * p.y + f }
}

/** The inverse of `m`, or null when it collapses space (a zero scale) and has none. */
export function invertAffine(m: Affine): Affine | null {
  const [a, b, c, d, e, f] = m
  const det = a * d - b * c
  if (!det || !Number.isFinite(det)) return null
  return [d / det, -b / det, -c / det, a / det, (c * f - d * e) / det, (b * e - a * f) / det]
}

/**
 * A point on the stage carried into a layer's own coordinates, given the layer's
 * absolute transform. The eraser stores its strokes this way so they ride along
 * when the layer is later moved, scaled or turned.
 */
export function toLocalPoint(absolute: Affine, stagePoint: Point): Point | null {
  const inverse = invertAffine(absolute)
  return inverse ? applyAffine(inverse, stagePoint) : null
}

/** How much `m` scales lengths, taken as the root of its area scale. */
export function affineScale(m: Affine): number {
  return Math.sqrt(Math.abs(m[0] * m[3] - m[1] * m[2]))
}

/**
 * An eraser brush `size` wide on the plaque, as a stroke width in a layer's own
 * coordinates. The brush is sized in template pixels, so it has to undo both the
 * stage's fit-to-screen scale and the layer's own scale.
 */
export function localBrushWidth(size: number, stageScale: number, absolute: Affine): number {
  const scale = affineScale(absolute)
  return scale > 0 ? (size * stageScale) / scale : size
}

/** A rectangle: its top-left corner and size. */
export type Rect = Point & Size

/**
 * How far to move a layer dropped at `rect` so at least `margin` of it is still
 * on the plaque. A layer dragged clean off the canvas could never be grabbed
 * again; (0, 0) when it is still within reach.
 */
export function reachableShift(rect: Rect, area: Size, margin: number): Point {
  const axis = (start: number, length: number, limit: number): number => {
    if (start + length < margin) return margin - (start + length)
    if (start > limit - margin) return limit - margin - start
    return 0
  }
  return {
    x: axis(rect.x, rect.width, area.width),
    y: axis(rect.y, rect.height, area.height),
  }
}

/**
 * Where a text layer's own origin sits within its box: on the side it is aligned
 * to, halfway down. Editing the words or switching to a font that has just
 * downloaded then grows the text away from that point, so it stays where it was
 * put instead of creeping right.
 */
export function textAnchor(align: EmaTextAlign, size: Size): Point {
  const x = align === 'left' ? 0 : align === 'right' ? size.width : size.width / 2
  return { x, y: size.height / 2 }
}

/** Rounds a coordinate to a tenth, which keeps history snapshots small. */
export function roundCoord(n: number): number {
  return Math.round(n * 10) / 10
}

// -- Layers --------------------------------------------------------------------

/** One eraser stroke, in its layer's own coordinates. */
export interface EmaStroke {
  /** Stroke width, in the layer's coordinates. */
  size: number
  /** Flat `[x0, y0, x1, y1, ...]`, as Konva's Line takes them. */
  points: number[]
}

export type EmaTextAlign = 'left' | 'center' | 'right'

interface EmaLayerBase {
  id: string
  visible: boolean
  opacity: number
  x: number
  y: number
  rotation: number
  scaleX: number
  scaleY: number
  strokes: EmaStroke[]
}

/** An uploaded picture. The pixels live outside the history, keyed by `assetId`. */
export interface EmaImageLayer extends EmaLayerBase {
  kind: 'image'
  /** The file name it was uploaded as, shown in the layers list. */
  name: string
  assetId: string
  width: number
  height: number
}

export interface EmaTextLayer extends EmaLayerBase {
  kind: 'text'
  text: string
  fontFamily: string
  fontSize: number
  fill: string
  stroke: string
  strokeWidth: number
  bold: boolean
  italic: boolean
  align: EmaTextAlign
}

export type EmaLayer = EmaImageLayer | EmaTextLayer

/** Every layer, BOTTOM first: index 0 sits directly on the plaque. */
export type EmaLayers = readonly EmaLayer[]

/** The fields of a layer that can be patched, whatever its kind. */
export type EmaLayerPatch = Partial<Omit<EmaImageLayer, 'id' | 'kind'>> &
  Partial<Omit<EmaTextLayer, 'id' | 'kind'>>

/** `layers` with one layer's fields replaced. Unknown ids leave it unchanged. */
export function updateLayer(layers: EmaLayers, id: string, patch: EmaLayerPatch): EmaLayers {
  const index = layers.findIndex((l) => l.id === id)
  if (index < 0) return layers
  const next = layers.slice()
  next[index] = { ...layers[index], ...patch }
  return next
}

/** `layers` without one layer. */
export function removeLayer(layers: EmaLayers, id: string): EmaLayers {
  return layers.some((l) => l.id === id) ? layers.filter((l) => l.id !== id) : layers
}

/**
 * `layers` with one layer swapped one step up (toward the viewer) or down
 * (toward the plaque). A layer already at that end stays put.
 */
export function moveLayer(layers: EmaLayers, id: string, direction: 'up' | 'down'): EmaLayers {
  const from = layers.findIndex((l) => l.id === id)
  const to = direction === 'up' ? from + 1 : from - 1
  if (from < 0 || to < 0 || to >= layers.length) return layers
  const next = layers.slice()
  next[to] = layers[from]
  next[from] = layers[to]
  return next
}

/** The layers in the order a layers panel lists them: the TOP one first. */
export function topFirst(layers: EmaLayers): EmaLayer[] {
  return layers.slice().reverse()
}

/** Whether a layer is the topmost (`'up'`) or bottommost (`'down'`) one. */
export function isAtEnd(layers: EmaLayers, id: string, direction: 'up' | 'down'): boolean {
  const index = layers.findIndex((l) => l.id === id)
  return direction === 'up' ? index === layers.length - 1 : index === 0
}

/** The name a layer goes by in the layers panel. */
export function layerLabel(layer: EmaLayer, maxLength = 28): string {
  const raw =
    layer.kind === 'text'
      ? (layer.text.split('\n').find((line) => line.trim()) ?? '').trim()
      : layer.name.trim()
  const fallback = layer.kind === 'text' ? 'Text' : 'Picture'
  if (!raw) return fallback
  return raw.length > maxLength ? raw.slice(0, maxLength - 1).trimEnd() + '…' : raw
}

/** A layer list as a history snapshot. */
export function serializeLayers(layers: EmaLayers): string {
  return JSON.stringify(layers)
}

/** A history snapshot back to a layer list. Snapshots are only ever our own. */
export function deserializeLayers(snapshot: string): EmaLayers {
  return JSON.parse(snapshot) as EmaLayers
}

// -- History -------------------------------------------------------------------

/** An undo history: the committed states behind, the present, and any redo states. */
export interface EmaHistory<T> {
  readonly past: readonly T[]
  readonly present: T
  readonly future: readonly T[]
}

export function createHistory<T>(initial: T): EmaHistory<T> {
  return { past: [], present: initial, future: [] }
}

/**
 * Records a new present. A state equal to the present is not a step - a slider
 * let go where it started should not cost an undo. Anything redoable is dropped,
 * and the oldest steps fall off past `limit`.
 */
export function historyPush<T>(
  h: EmaHistory<T>,
  state: T,
  limit = EMA_HISTORY_LIMIT,
): EmaHistory<T> {
  if (state === h.present) return h
  const past = [...h.past, h.present]
  return {
    past: past.length > limit ? past.slice(past.length - limit) : past,
    present: state,
    future: [],
  }
}

/** One step back, or the same history when there is nothing to undo. */
export function historyUndo<T>(h: EmaHistory<T>): EmaHistory<T> {
  const previous = h.past[h.past.length - 1]
  if (previous === undefined) return h
  return { past: h.past.slice(0, -1), present: previous, future: [h.present, ...h.future] }
}

/** One step forward again, or the same history when there is nothing to redo. */
export function historyRedo<T>(h: EmaHistory<T>): EmaHistory<T> {
  const next = h.future[0]
  if (next === undefined) return h
  return { past: [...h.past, h.present], present: next, future: h.future.slice(1) }
}

// -- Fonts ---------------------------------------------------------------------

/** System families every visitor already has, so they never wait on a download. */
export const WEB_SAFE_FONTS: readonly string[] = [
  'Arial',
  'Georgia',
  'Times New Roman',
  'Courier New',
  'Verdana',
  'Trebuchet MS',
  'Comic Sans MS',
  'Impact',
]

export function isWebSafeFont(family: string): boolean {
  return WEB_SAFE_FONTS.includes(family)
}

/** The families offered, in two groups. */
export interface EmaFontChoices {
  basic: readonly string[]
  google: string[]
}

/**
 * The font list: the web-safe families, then the Google families the parent
 * loaded from the server, or `fallback` when it sent none. Blank, repeated and
 * web-safe names are dropped from the Google group so nothing is listed twice.
 */
export function fontChoices(fonts: readonly string[], fallback: readonly string[]): EmaFontChoices {
  const source = fonts.some((f) => f.trim()) ? fonts : fallback
  const seen = new Set<string>()
  const google: string[] = []
  for (const raw of source) {
    const family = raw.trim()
    if (!family || isWebSafeFont(family) || seen.has(family)) continue
    seen.add(family)
    google.push(family)
  }
  return { basic: WEB_SAFE_FONTS, google }
}

/** The families whose name contains `query`, ignoring case and surrounding space. */
export function filterFonts(families: readonly string[], query: string): string[] {
  const q = query.trim().toLowerCase()
  return q ? families.filter((f) => f.toLowerCase().includes(q)) : families.slice()
}

/** Konva's `fontStyle` for a bold/italic pair. */
export function konvaFontStyle(bold: boolean, italic: boolean): string {
  if (bold && italic) return 'italic bold'
  if (bold) return 'bold'
  if (italic) return 'italic'
  return 'normal'
}

/** A CSS `font` shorthand naming one face of a family, for `document.fonts.load`. */
export function fontFaceSpec(family: string, bold: boolean, italic: boolean): string {
  return `${italic ? 'italic' : 'normal'} ${bold ? 700 : 400} 40px "${family.replace(/"/g, '')}"`
}

/**
 * Waits until the face `spec` names has loaded, for the characters in `text`.
 *
 * `loadGoogleFont()` only adds the stylesheet `<link>`; until that stylesheet
 * arrives there is no `@font-face` to load, and `fonts.load()` resolves at once
 * with nothing. So this asks again until a face turns up or `timeoutMs` passes.
 * Resolves true when the face loaded, false when it gave up - the caller redraws
 * either way, falling back to whatever face the browser has.
 */
export async function waitForFontFace(
  fonts: Pick<FontFaceSet, 'load'>,
  spec: string,
  text: string,
  timeoutMs = 6000,
  retryMs = 150,
): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  for (;;) {
    try {
      if ((await fonts.load(spec, text || ' ')).length > 0) return true
    } catch {
      // An unparseable spec or a failed download: keep asking until the deadline.
    }
    if (Date.now() + retryMs > deadline) return false
    await new Promise((resolve) => setTimeout(resolve, retryMs))
  }
}
