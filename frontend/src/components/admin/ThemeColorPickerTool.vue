<script setup lang="ts">
/**
 * Theme color-picker helper tool (Themes tab).
 *
 * Purely an aid for the admin: pick a color with the native color input,
 * type/paste any CSS color (hex, rgb, name) to preview it, and copy the HEX or
 * RGBA value to paste into the CSS editor. It does not modify the theme directly.
 *
 * The native `<input type="color">` is opaque (#rrggbb); for colors that need
 * alpha, type the value into the hex field — the RGBA readout reflects it.
 */
import { computed, ref } from 'vue'
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()

const hex = ref('#d6bdae')
const rgba = ref('rgba(214, 189, 174, 1)')

/** Opaque `#rrggbb` for the native colour input (which can't represent alpha). */
const hexOpaque = computed(() => hex.value.slice(0, 7))

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

/** Native colour input or typed/pasted hex → preview + readouts. */
function onColorInput(e: Event): void {
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
      <input
        :value="hexOpaque"
        type="color"
        class="theme-color-tool__color"
        aria-label="Pick a color"
        @input="onColorInput"
      />
      <input
        :value="hex"
        class="theme-color-tool__hex"
        aria-label="Hex color"
        spellcheck="false"
        placeholder="#rrggbb"
        @input="onColorInput"
      />
      <code class="theme-color-tool__rgba">{{ rgba }}</code>
    </div>
    <!-- Copy buttons on their own row so they stay together (and the tool fits a
         narrow modal without wrapping awkwardly). -->
    <div class="theme-color-tool__buttons">
      <button class="btn-view btn-sm" title="Copy hex value" @click="copy(hex, 'HEX')">
        <font-awesome-icon :icon="['fas', 'copy']" /> HEX
      </button>
      <button class="btn-view btn-sm" title="Copy rgba value" @click="copy(rgba, 'RGBA')">
        <font-awesome-icon :icon="['fas', 'copy']" /> RGBA
      </button>
    </div>
  </div>
</template>

<style scoped>
.theme-color-tool {
  background: var(--panel-raised-bg);
  border: 1px solid var(--control-border);
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

.theme-color-tool__buttons {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

/* Native color input doubling as the swatch (matches the embed-color input). */
.theme-color-tool__color {
  width: 48px;
  height: 36px;
  padding: 2px;
  border: 1px solid var(--control-border);
  border-radius: 6px;
  background: var(--panel-bg);
  cursor: pointer;
  flex: 0 0 auto;
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
</style>
