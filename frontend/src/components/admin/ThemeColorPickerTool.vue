<script setup lang="ts">
/**
 * Theme color-picker helper tool (Themes tab).
 *
 * Purely an aid for the admin: pick a colour visually, type/paste a hex to
 * preview it, and copy the HEX or RGBA value to paste into the CSS editor. It
 * does not modify the theme directly. The Chrome picker (`@ckpack/vue-color`)
 * is lazy-loaded so it only lands when the Themes tab is opened.
 */
import { computed, ref } from 'vue'
import ColorPicker from '@/components/common/ui/ColorPicker.vue'
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()

const expanded = ref(false)
const hex = ref('#d6bdae')
const rgba = ref('rgba(214, 189, 174, 1)')

/** Seed value bound into the picker so it reflects the current/typed colour. */
const pickerSeed = computed(() => hex.value)

/** Normalizes any CSS colour string to `rgba(r, g, b, a)` via the browser. */
function toRgba(input: string): string {
  const el = document.createElement('div')
  el.style.color = input
  if (!el.style.color) return '' // invalid colour
  document.body.appendChild(el)
  const computed = getComputedStyle(el).color
  el.remove()
  return computed
}

/** Builds a #rrggbb(aa) hex from 0–255 channels + 0–1 alpha. */
function toHex(r: number, g: number, b: number, a: number): string {
  const part = (n: number) => Math.round(n).toString(16).padStart(2, '0')
  const base = '#' + part(r) + part(g) + part(b)
  return a < 1 ? base + part(a * 255) : base
}

/** Sets both readouts from any valid CSS colour string (hex, name, rgb…). */
function setFromAny(input: string): void {
  const norm = toRgba(input.trim())
  const nums = norm.match(/[\d.]+/g)
  if (!nums || nums.length < 3) return
  const [r, g, b, a = 1] = nums.map(Number)
  rgba.value = `rgba(${r}, ${g}, ${b}, ${a})`
  hex.value = toHex(r, g, b, a)
}

/** Picker change → take the normalized rgba + hex from ColorPicker. */
function onPickerChange(c: { rgba: string; hex: string }): void {
  rgba.value = c.rgba
  hex.value = c.hex
}

/** Typed/pasted hex (or any colour) → preview + readouts. */
function onHexInput(e: Event): void {
  setFromAny((e.target as HTMLInputElement).value)
}

async function copy(value: string, label: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value)
    ui.notify(`Copied ${label}: ${value}`, 'success')
  } catch {
    ui.notify('Could not copy to clipboard', 'error')
  }
}
</script>

<template>
  <div class="theme-color-tool">
    <div class="theme-color-tool__bar">
      <span class="theme-color-tool__swatch-wrap">
        <span class="theme-color-tool__swatch" :style="{ background: rgba }"></span>
      </span>
      <input
        :value="hex"
        class="theme-color-tool__hex"
        aria-label="Hex color"
        spellcheck="false"
        placeholder="#rrggbb"
        @input="onHexInput"
      />
      <code class="theme-color-tool__rgba">{{ rgba }}</code>
      <button class="btn-ghost btn-sm" title="Copy hex value" @click="copy(hex, 'HEX')">
        <i class="fa-solid fa-copy" aria-hidden="true"></i> HEX
      </button>
      <button class="btn-ghost btn-sm" title="Copy rgba value" @click="copy(rgba, 'RGBA')">
        <i class="fa-solid fa-copy" aria-hidden="true"></i> RGBA
      </button>
      <button
        class="btn-ghost btn-sm theme-color-tool__toggle"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >
        <i class="fa-duotone fa-palette" aria-hidden="true"></i>
        Color Picker
        <i
          class="fa-solid"
          :class="expanded ? 'fa-chevron-up' : 'fa-chevron-down'"
          aria-hidden="true"
        ></i>
      </button>
    </div>

    <div v-show="expanded" class="theme-color-tool__picker">
      <ColorPicker :value="pickerSeed" @change="onPickerChange" />
    </div>
  </div>
</template>

<style scoped>
.theme-color-tool {
  background: var(--panel-raised-bg);
  border: 1px solid var(--panel-raised-bg);
  border-radius: var(--radius);
  padding: 10px 12px;
  margin-bottom: 16px;
}

.theme-color-tool__bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

/* Checkerboard so the swatch's alpha is visible. */
.theme-color-tool__swatch-wrap {
  width: 34px;
  height: 28px;
  border-radius: 6px;
  overflow: hidden;
  flex: 0 0 auto;
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.3);
  background-image:
    linear-gradient(45deg, rgba(255, 255, 255, 0.25) 25%, transparent 25%),
    linear-gradient(-45deg, rgba(255, 255, 255, 0.25) 25%, transparent 25%),
    linear-gradient(45deg, transparent 75%, rgba(255, 255, 255, 0.25) 75%),
    linear-gradient(-45deg, transparent 75%, rgba(255, 255, 255, 0.25) 75%);
  background-size: 10px 10px;
  background-position:
    0 0,
    0 5px,
    5px -5px,
    -5px 0;
}

.theme-color-tool__swatch {
  display: block;
  width: 100%;
  height: 100%;
}

.theme-color-tool__hex {
  width: 110px;
  font-family: 'Consolas', 'Monaco', monospace;
  padding: 6px 8px;
  font-size: 0.85rem;
}

.theme-color-tool__rgba {
  color: var(--text-muted);
  font-size: 0.8rem;
  min-width: 150px;
}

.theme-color-tool__toggle {
  margin-left: auto;
}

.theme-color-tool__picker {
  display: flex;
  justify-content: center;
  margin-top: 12px;
}

/* The Chrome picker skin lives globally on the shared `.color-picker` object
   (app.css), so the Themes tool and the stamp-colour modal match. */
</style>

