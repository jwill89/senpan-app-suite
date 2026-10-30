<script setup lang="ts">
/**
 * Ema designer: decorate a wooden wishing plaque right on the site. Pictures and
 * text sit on layers over the plaque template; they move, resize and turn with
 * handles (mouse or touch), an eraser rubs out the parts of a layer that aren't
 * wanted, and the result comes out as a PNG.
 *
 * Contract (the parent shows this full-screen in a modal):
 *   - `templateSrc`: same-origin URL of the transparent plaque PNG;
 *   - `fonts`: Google families from the server, or empty for the built-in list;
 *   - `busy`: the parent is sending, so the finishing buttons wait;
 *   - `done(blob)`: "Send for posting" - the finished PNG for the parent to upload;
 *   - `cancel()`: closed, after confirming when there is work to lose.
 *   `confirmDiscard()` is exposed for the parent's own close paths (Escape, the
 *   backdrop), so every way out asks the same question.
 *
 * State lives here and only here: `layers` (bottom first) plus an undo history of
 * serialized snapshots of it, eraser strokes included. The canvas (emaStage.ts) is
 * handed every new version and reports back what the pointer did; it keeps no
 * design state of its own. Picture pixels stay out of the
 * history - a layer names its upload by `assetId`, and uploads are kept for the
 * life of the designer so an undone delete can find them again.
 */
import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  useId,
  useTemplateRef,
  watch,
} from 'vue'
import { FALLBACK_GOOGLE_FONTS } from '@/lib/constants'
import { triggerDownload } from '@/lib/download'
import {
  type EmaHistory,
  type EmaImageLayer,
  type EmaLayerPatch,
  type EmaLayers,
  type EmaTextLayer,
  createHistory,
  deserializeLayers,
  downscaleSize,
  fontChoices,
  fontFaceSpec,
  historyPush,
  historyRedo,
  historyUndo,
  isWebSafeFont,
  layerLabel,
  moveLayer,
  placementScale,
  removeLayer,
  serializeLayers,
  updateLayer,
  waitForFontFace,
} from '@/lib/emaDesigner'
import { loadGoogleFont } from '@/lib/theme'
import { nextUid } from '@/lib/uid'
import { useUiStore } from '@/stores/ui'
import EmaLayersPanel from './EmaLayersPanel.vue'
import EmaPropertiesPanel from './EmaPropertiesPanel.vue'
import EmaSlider from './EmaSlider.vue'
import { type EmaAsset, EmaStage, type EmaStageEvents, type EmaTool } from './emaStage'

const props = defineProps<{ templateSrc: string; fonts: string[]; busy?: boolean }>()
const emit = defineEmits<{ done: [blob: Blob]; cancel: [] }>()

const ui = useUiStore()
const uid = useId()

/** Panels beside the canvas from here up; must match the breakpoint in ema.css. */
const WIDE_LAYOUT = '(width >= 900px)'
/** On a stacked layout the canvas may take this share of the box's height. */
const STACKED_HEIGHT_SHARE = 0.62
/** Upload types offered. SVG is left out: Safari taints a canvas that draws one. */
const IMAGE_TYPES = 'image/png,image/jpeg,image/webp,image/gif,image/avif'

const root = useTemplateRef<HTMLDivElement>('root')
const stageWrap = useTemplateRef<HTMLDivElement>('stageWrap')
const stageEl = useTemplateRef<HTMLDivElement>('stageEl')
const fileInput = useTemplateRef<HTMLInputElement>('fileInput')

const layers = shallowRef<EmaLayers>([])
const history = shallowRef<EmaHistory<string>>(createHistory(serializeLayers([])))
const selectedId = ref<string | null>(null)
const tool = ref<EmaTool>('move')
const brushSize = ref(28)
const keepInside = ref(true)
const templateState = ref<'loading' | 'ready' | 'error'>('loading')
const exporting = ref(false)
const reading = ref(0)
/** The design as last downloaded, so closing straight after asks nothing. */
const savedSnapshot = ref(serializeLayers([]))

const assets = new Map<string, EmaAsset>()
let stage: EmaStage | null = null
let observer: ResizeObserver | null = null
let fitFrame = 0
let fontTimer: number | undefined

const choices = computed(() => fontChoices(props.fonts, FALLBACK_GOOGLE_FONTS))
const selected = computed(() => layers.value.find((l) => l.id === selectedId.value) ?? null)
const snapshot = computed(() => serializeLayers(layers.value))
const canUndo = computed(() => history.value.past.length > 0)
const canRedo = computed(() => history.value.future.length > 0)
const hasUnsavedWork = computed(
  () => layers.value.length > 0 && snapshot.value !== savedSnapshot.value,
)
const ready = computed(() => templateState.value === 'ready')
const canFinish = computed(
  () => ready.value && layers.value.some((l) => l.visible) && !props.busy && !exporting.value,
)

const hint = computed(() => {
  if (!ready.value) return ''
  if (!layers.value.length) return 'Add a picture or some text to get started.'
  const layer = selected.value
  if (tool.value === 'erase') {
    if (!layer) return 'Pick a layer in the list, then rub over the parts you want gone.'
    if (!layer.visible) return 'That layer is hidden. Show it to erase from it.'
    return `Erasing from "${layerLabel(layer)}" only. Undo brings a stroke back.`
  }
  if (!layer) return 'Tap something on the plaque to move it, or pick it in the list.'
  return 'Drag to move it. Pull a corner to resize, or the top handle to turn it.'
})

// -- Edits and history ---------------------------------------------------------

/** Applies an edit without recording it: a slider mid-drag, a word mid-typing. */
function patch(id: string, p: EmaLayerPatch): void {
  layers.value = updateLayer(layers.value, id, p)
}

function patchSelected(p: EmaLayerPatch): void {
  if (selectedId.value) patch(selectedId.value, p)
}

/** Records the layers as they are now as one undo step. */
function commit(): void {
  history.value = historyPush(history.value, snapshot.value)
}

/** Replaces the layers and records the change. */
function edit(next: EmaLayers): void {
  layers.value = next
  commit()
}

function restore(next: EmaHistory<string>): void {
  if (next === history.value) return
  history.value = next
  layers.value = deserializeLayers(next.present)
  if (!layers.value.some((l) => l.id === selectedId.value)) selectedId.value = null
}

function undo(): void {
  restore(historyUndo(history.value))
}

function redo(): void {
  restore(historyRedo(history.value))
}

function select(id: string | null): void {
  selectedId.value = id
}

function toggleVisible(id: string): void {
  const layer = layers.value.find((l) => l.id === id)
  if (layer) edit(updateLayer(layers.value, id, { visible: !layer.visible }))
}

function move(id: string, direction: 'up' | 'down'): void {
  edit(moveLayer(layers.value, id, direction))
}

function remove(id: string): void {
  edit(removeLayer(layers.value, id))
  if (selectedId.value === id) selectedId.value = null
}

function restoreErased(): void {
  if (selected.value) edit(updateLayer(layers.value, selected.value.id, { strokes: [] }))
}

const stageEvents: EmaStageEvents = {
  select,
  transform: (id, placement) => {
    edit(updateLayer(layers.value, id, placement))
  },
  stroke: (id, stroke) => {
    const layer = layers.value.find((l) => l.id === id)
    if (layer) edit(updateLayer(layers.value, id, { strokes: [...layer.strokes, stroke] }))
  },
}

// -- Adding layers -------------------------------------------------------------

function designSize(): { width: number; height: number } {
  return stage?.designSize ?? { width: 1, height: 1 }
}

function addText(): void {
  const plaque = designSize()
  const layer: EmaTextLayer = {
    id: `ema-${nextUid()}`,
    kind: 'text',
    text: 'Your wish here',
    fontFamily: choices.value.google[0] ?? 'Georgia',
    fontSize: Math.round(plaque.height * 0.1),
    // Dark ink with a pale outline ready to switch on: reads on bare wood.
    fill: '#2b1a0e',
    stroke: '#fff4e0',
    strokeWidth: 0,
    bold: false,
    italic: false,
    align: 'center',
    visible: true,
    opacity: 1,
    // Centered, a little below the middle so it clears the rope bow.
    x: plaque.width / 2,
    y: plaque.height * 0.58,
    rotation: 0,
    scaleX: 1,
    scaleY: 1,
    strokes: [],
  }
  edit([...layers.value, layer])
  selectedId.value = layer.id
  tool.value = 'move'
}

function pickImages(): void {
  fileInput.value?.click()
}

/**
 * Reads an upload as a `data:` URL. Never `URL.createObjectURL`: the live site's
 * Content-Security-Policy allows images from 'self', data: and https: only, so a
 * blob: URL picture loads in dev and silently fails in production.
 */
function readAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      if (typeof reader.result === 'string') resolve(reader.result)
      else reject(new Error('The file could not be read.'))
    }
    reader.onerror = () => {
      reject(reader.error ?? new Error('The file could not be read.'))
    }
    reader.readAsDataURL(file)
  })
}

/**
 * Loads an image from `src`. Waits on `load` rather than `decode()`: Safari has
 * rejected `decode()` for large, perfectly valid pictures, and a page in the
 * background may hold a decode back until it is shown again.
 */
function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => {
      resolve(img)
    }
    img.onerror = () => {
      reject(new Error('The image could not be loaded.'))
    }
    img.src = src
  })
}

/**
 * Decodes an upload, shrinking anything past the size cap onto a canvas once, so
 * a phone photo doesn't hold 50 MB of pixels for something drawn a few hundred
 * pixels across.
 */
async function decodeUpload(
  file: File,
): Promise<{ asset: EmaAsset; width: number; height: number }> {
  const img = await loadImage(await readAsDataUrl(file))
  const size = downscaleSize(img.naturalWidth, img.naturalHeight)
  const ctx = size.scaled ? document.createElement('canvas').getContext('2d') : null
  if (!ctx) return { asset: img, width: img.naturalWidth, height: img.naturalHeight }
  ctx.canvas.width = size.width
  ctx.canvas.height = size.height
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(img, 0, 0, size.width, size.height)
  return { asset: ctx.canvas, width: size.width, height: size.height }
}

async function onFilesChosen(e: Event): Promise<void> {
  const input = e.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  // Cleared so choosing the same file again still fires `change`.
  input.value = ''
  if (!files.length) return
  const plaque = designSize()
  const added: EmaImageLayer[] = []
  reading.value++
  try {
    for (const file of files) {
      try {
        const { asset, width, height } = await decodeUpload(file)
        const assetId = `ema-asset-${nextUid()}`
        assets.set(assetId, asset)
        const scale = placementScale({ width, height }, plaque)
        // Several at once fan out a little instead of landing exactly on top of each other.
        const offset = added.length * 24
        added.push({
          id: `ema-${nextUid()}`,
          kind: 'image',
          name: file.name,
          assetId,
          width,
          height,
          x: (plaque.width - width * scale) / 2 + offset,
          y: (plaque.height - height * scale) / 2 + offset,
          scaleX: scale,
          scaleY: scale,
          rotation: 0,
          opacity: 1,
          visible: true,
          strokes: [],
        })
      } catch {
        ui.notify(`Couldn't open ${file.name}. Try a PNG, JPEG or WebP picture.`, 'error')
      }
    }
  } finally {
    reading.value--
  }
  const last = added.at(-1)
  if (!last) return
  edit([...layers.value, ...added])
  selectedId.value = last.id
  tool.value = 'move'
}

// -- Fonts ---------------------------------------------------------------------

/**
 * Fetches the Google families the text layers use and redraws once each face is
 * in. Konva measures and draws text with whatever face the browser has at that
 * moment, so without the wait a new family would show as the fallback face.
 */
function loadTextFonts(): void {
  for (const layer of layers.value) {
    if (layer.kind !== 'text' || isWebSafeFont(layer.fontFamily)) continue
    loadGoogleFont(layer.fontFamily)
    void waitForFontFace(
      document.fonts,
      fontFaceSpec(layer.fontFamily, layer.bold, layer.italic),
      layer.text,
    ).then(() => stage?.refreshText())
  }
}

// Every face a text layer needs, and the characters it needs them for (a wish in
// Japanese pulls in a different subset of the same family).
watch(
  () =>
    layers.value
      .map((l) => (l.kind === 'text' ? `${l.fontFamily}|${l.bold}|${l.italic}|${l.text}` : ''))
      .join('\n'),
  () => {
    window.clearTimeout(fontTimer)
    fontTimer = window.setTimeout(loadTextFonts, 200)
  },
)

// -- Canvas --------------------------------------------------------------------

watch(layers, (next) => stage?.setLayers(next))
watch(selectedId, (id) => stage?.select(id))
watch(tool, (next) => stage?.setTool(next))
watch(brushSize, (size) => stage?.setBrushSize(size))
watch(keepInside, (on) => stage?.setClip(on))
watch(
  () => props.templateSrc,
  (src) => void loadTemplate(src),
)

async function loadTemplate(src: string): Promise<void> {
  templateState.value = 'loading'
  let img: HTMLImageElement
  try {
    img = await loadImage(src)
  } catch {
    if (src === props.templateSrc) templateState.value = 'error'
    return
  }
  // A newer template may have been asked for while this one loaded.
  if (src !== props.templateSrc || !stage) return
  stage.setTemplate(img)
  stage.setLayers(layers.value)
  templateState.value = 'ready'
  fitStage()
}

/**
 * Sizes the canvas to its box. Beside the panels it fills the column; stacked
 * on a phone its height comes from its width, so it is capped by a share of the
 * designer's height instead of measured - measuring a box sized by the canvas
 * would only ever give back the canvas.
 */
function fitStage(): void {
  const wrap = stageWrap.value
  const box = root.value
  if (!stage || !wrap || !box) return
  const wide = window.matchMedia(WIDE_LAYOUT).matches
  stage.fit({
    width: wrap.clientWidth,
    height: wide ? wrap.clientHeight : box.clientHeight * STACKED_HEIGHT_SHARE,
  })
}

function scheduleFit(): void {
  cancelAnimationFrame(fitFrame)
  fitFrame = requestAnimationFrame(fitStage)
}

/** A theme token's value, for the handles Konva draws on the canvas. */
function token(styles: CSSStyleDeclaration, name: string): string {
  return styles.getPropertyValue(name).trim() || styles.color
}

// -- Keyboard ------------------------------------------------------------------

/** True for a target that owns its own keystrokes (a text field, the font search). */
function isTypingInto(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el?.tagName) return false
  return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName)
}

function onKeydown(e: KeyboardEvent): void {
  if (e.defaultPrevented || ui.confirmState.show || isTypingInto(e.target)) return
  // Kept mounted but hidden (a parent's v-show), the designer isn't what the keys are for.
  if (!root.value?.getClientRects().length) return
  const mod = e.ctrlKey || e.metaKey
  const key = e.key.toLowerCase()
  if (mod && key === 'z') {
    e.preventDefault()
    if (e.shiftKey) redo()
    else undo()
  } else if (mod && key === 'y') {
    e.preventDefault()
    redo()
  } else if (!mod && (e.key === 'Delete' || e.key === 'Backspace') && selectedId.value) {
    e.preventDefault()
    remove(selectedId.value)
  }
}

// -- Finishing -----------------------------------------------------------------

async function renderPng(): Promise<Blob | null> {
  if (!stage) return null
  exporting.value = true
  try {
    return await stage.toPng()
  } catch {
    ui.notify("Your plaque couldn't be turned into a picture. Please try again.", 'error')
    return null
  } finally {
    exporting.value = false
  }
}

async function download(): Promise<void> {
  const blob = await renderPng()
  if (!blob) return
  triggerDownload(blob, 'ema-plaque.png')
  savedSnapshot.value = snapshot.value
}

async function send(): Promise<void> {
  const blob = await renderPng()
  if (blob) emit('done', blob)
}

/**
 * Whether it's fine to close: nothing to lose, or the visitor said so. Sending
 * doesn't count as saved - if the parent's upload fails, the design is still
 * only here.
 */
async function confirmDiscard(): Promise<boolean> {
  if (!hasUnsavedWork.value) return true
  return ui.confirm("Your plaque hasn't been sent or downloaded yet. Close it anyway?", {
    title: 'Leave your plaque?',
    confirmText: 'Close and lose it',
    cancelText: 'Keep designing',
  })
}

async function onCancel(): Promise<void> {
  if (await confirmDiscard()) emit('cancel')
}

defineExpose({ confirmDiscard })

onMounted(() => {
  const el = stageEl.value
  if (!el) return
  const styles = getComputedStyle(el)
  stage = new EmaStage(el, assets, stageEvents, {
    handle: token(styles, '--highlight'),
    handleFill: token(styles, '--panel-bg'),
  })
  stage.setBrushSize(brushSize.value)
  stage.setClip(keepInside.value)
  observer = new ResizeObserver(scheduleFit)
  if (root.value) observer.observe(root.value)
  if (stageWrap.value) observer.observe(stageWrap.value)
  window.addEventListener('keydown', onKeydown)
  void loadTemplate(props.templateSrc)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  observer?.disconnect()
  cancelAnimationFrame(fitFrame)
  window.clearTimeout(fontTimer)
  stage?.destroy()
  stage = null
  assets.clear()
})
</script>

<template>
  <div ref="root" class="ema-designer">
    <section class="ema-workspace" :aria-labelledby="`${uid}-plaque`">
      <h3 :id="`${uid}-plaque`" class="sr-only">Your plaque</h3>
      <div class="ema-toolbar" role="toolbar" aria-label="Designer tools">
        <div class="toggle-group" role="group" aria-label="Tool">
          <button
            type="button"
            class="toggle-btn"
            :class="{ 'is-active': tool === 'move' }"
            :aria-pressed="tool === 'move'"
            @click="tool = 'move'"
          >
            <font-awesome-icon :icon="['fas', 'arrow-pointer']" /> Move
          </button>
          <button
            type="button"
            class="toggle-btn"
            :class="{ 'is-active': tool === 'erase' }"
            :aria-pressed="tool === 'erase'"
            @click="tool = 'erase'"
          >
            <font-awesome-icon :icon="['fas', 'eraser']" /> Eraser
          </button>
        </div>
        <div class="ema-toolbar-group">
          <button
            type="button"
            class="btn-neutral btn-sm ema-icon-btn"
            aria-label="Undo"
            title="Undo (Ctrl+Z)"
            :disabled="!canUndo"
            @click="undo"
          >
            <font-awesome-icon :icon="['fas', 'arrow-rotate-left']" />
          </button>
          <button
            type="button"
            class="btn-neutral btn-sm ema-icon-btn"
            aria-label="Redo"
            title="Redo (Shift+Ctrl+Z)"
            :disabled="!canRedo"
            @click="redo"
          >
            <font-awesome-icon :icon="['fas', 'arrow-rotate-right']" />
          </button>
        </div>
        <div class="ema-toolbar-group push-right">
          <button
            type="button"
            class="btn-confirm btn-sm"
            :disabled="!ready || reading > 0"
            @click="pickImages"
          >
            <font-awesome-icon :icon="['fas', 'image']" /> Picture
          </button>
          <button type="button" class="btn-confirm btn-sm" :disabled="!ready" @click="addText">
            <font-awesome-icon :icon="['fas', 'font']" /> Text
          </button>
        </div>
        <input
          ref="fileInput"
          type="file"
          :accept="IMAGE_TYPES"
          multiple
          hidden
          aria-label="Choose pictures"
          @change="onFilesChosen"
        />
      </div>

      <div ref="stageWrap" class="ema-stage-wrap">
        <div
          ref="stageEl"
          class="ema-stage"
          :class="{ 'is-erasing': tool === 'erase' }"
          role="img"
          aria-label="Your plaque design"
        />
        <p v-if="templateState === 'loading'" class="ema-stage-status">Loading the plaque...</p>
        <p v-else-if="templateState === 'error'" class="ema-stage-status">
          The plaque couldn't be loaded. Close this and try again.
        </p>
      </div>
      <p class="ema-hint">{{ hint }}</p>
    </section>

    <div class="ema-panels">
      <section
        v-if="tool === 'erase'"
        class="subpanel ema-panel"
        :aria-labelledby="`${uid}-eraser`"
      >
        <h3 :id="`${uid}-eraser`" class="ema-panel-title">
          <font-awesome-icon :icon="['fad', 'eraser']" /> Eraser
        </h3>
        <EmaSlider v-model="brushSize" label="Size" :min="4" :max="120" unit=" px" />
        <button
          v-if="selected?.strokes.length"
          type="button"
          class="btn-caution btn-sm"
          @click="restoreErased"
        >
          Bring back everything erased from this layer
        </button>
      </section>

      <section
        v-if="tool === 'move' && selected"
        class="subpanel ema-panel"
        :aria-labelledby="`${uid}-props`"
      >
        <h3 :id="`${uid}-props`" class="ema-panel-title">
          <font-awesome-icon :icon="['fad', 'sliders']" />
          {{ selected.kind === 'text' ? 'Text' : 'Picture' }}
        </h3>
        <EmaPropertiesPanel
          :layer="selected"
          :choices="choices"
          @patch="patchSelected"
          @commit="commit"
        />
      </section>

      <section class="subpanel ema-panel" :aria-labelledby="`${uid}-layers`">
        <h3 :id="`${uid}-layers`" class="ema-panel-title">
          <font-awesome-icon :icon="['fad', 'layer-group']" /> Layers
        </h3>
        <p v-if="!layers.length" class="ema-panel-note">Nothing here yet.</p>
        <EmaLayersPanel
          v-else
          :layers="layers"
          :selected-id="selectedId"
          @select="select"
          @toggle-visible="toggleVisible"
          @move="move"
          @remove="remove"
        />
      </section>

      <section class="subpanel ema-panel">
        <label class="checkbox-inline ema-check">
          <input v-model="keepInside" type="checkbox" />
          Keep the design inside the plaque
        </label>
        <p class="ema-panel-note">
          Anything hanging over the edge is trimmed off, so there's no need to erase it.
        </p>
      </section>
    </div>

    <div class="ema-actions">
      <button type="button" class="btn-neutral" @click="onCancel">Cancel</button>
      <button type="button" class="btn-view push-right" :disabled="!canFinish" @click="download">
        <font-awesome-icon :icon="['fas', 'download']" /> Download
      </button>
      <button type="button" class="btn-action ema-send" :disabled="!canFinish" @click="send">
        <font-awesome-icon :icon="['fas', 'paper-plane']" />
        {{ busy ? 'Sending...' : 'Send for posting' }}
      </button>
    </div>
  </div>
</template>
