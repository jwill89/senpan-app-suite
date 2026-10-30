<script setup lang="ts">
/**
 * The selected layer's settings in the Ema designer: opacity for any layer, and
 * for text its words, font, size, colors, outline, bold/italic and alignment.
 *
 * Emits `patch` for every change so the canvas follows along live, and `commit`
 * when a change is complete - a slider let go, a color picked, the words left -
 * which is when the designer records an undo step. A one-tap change (a font, bold,
 * alignment) emits both at once.
 */
import { useId } from 'vue'
import FormField from '@/components/common/ui/FormField.vue'
import type { EmaFontChoices, EmaLayer, EmaLayerPatch, EmaTextAlign } from '@/lib/emaDesigner'
import EmaFontPicker from './EmaFontPicker.vue'
import EmaSlider from './EmaSlider.vue'

defineProps<{ layer: EmaLayer; choices: EmaFontChoices }>()
const emit = defineEmits<{ patch: [patch: EmaLayerPatch]; commit: [] }>()

const id = useId()

const ALIGNS: { value: EmaTextAlign; label: string; icon: string }[] = [
  { value: 'left', label: 'Align left', icon: 'align-left' },
  { value: 'center', label: 'Center', icon: 'align-center' },
  { value: 'right', label: 'Align right', icon: 'align-right' },
]

/** A change that is complete in one go. */
function set(patch: EmaLayerPatch): void {
  emit('patch', patch)
  emit('commit')
}

function inputValue(e: Event): string {
  return (e.target as HTMLInputElement | HTMLTextAreaElement).value
}
</script>

<template>
  <div class="ema-props">
    <template v-if="layer.kind === 'text'">
      <FormField label="Words" :html-for="`${id}-words`">
        <textarea
          :id="`${id}-words`"
          class="ema-words"
          rows="3"
          :value="layer.text"
          @input="emit('patch', { text: inputValue($event) })"
          @change="emit('commit')"
        />
      </FormField>

      <div class="field">
        <span class="field-label">
          Font: <span class="fw-normal">{{ layer.fontFamily }}</span>
        </span>
        <EmaFontPicker
          :model-value="layer.fontFamily"
          :choices="choices"
          label="Font"
          @update:model-value="(fontFamily) => set({ fontFamily })"
        />
      </div>

      <EmaSlider
        :model-value="layer.fontSize"
        label="Size"
        :min="10"
        :max="200"
        unit=" px"
        @update:model-value="(fontSize) => emit('patch', { fontSize })"
        @change="emit('commit')"
      />

      <div class="ema-style-row">
        <div class="toggle-group" role="group" aria-label="Style">
          <button
            type="button"
            class="toggle-btn ema-icon-btn"
            :class="{ 'is-active': layer.bold }"
            :aria-pressed="layer.bold"
            aria-label="Bold"
            title="Bold"
            @click="set({ bold: !layer.bold })"
          >
            <font-awesome-icon :icon="['fas', 'bold']" />
          </button>
          <button
            type="button"
            class="toggle-btn ema-icon-btn"
            :class="{ 'is-active': layer.italic }"
            :aria-pressed="layer.italic"
            aria-label="Italic"
            title="Italic"
            @click="set({ italic: !layer.italic })"
          >
            <font-awesome-icon :icon="['fas', 'italic']" />
          </button>
        </div>
        <div class="toggle-group" role="group" aria-label="Alignment">
          <button
            v-for="a in ALIGNS"
            :key="a.value"
            type="button"
            class="toggle-btn ema-icon-btn"
            :class="{ 'is-active': layer.align === a.value }"
            :aria-pressed="layer.align === a.value"
            :aria-label="a.label"
            :title="a.label"
            @click="set({ align: a.value })"
          >
            <font-awesome-icon :icon="['fas', a.icon]" />
          </button>
        </div>
      </div>

      <div class="form-row">
        <FormField label="Color" :html-for="`${id}-fill`">
          <span class="color-field">
            <input
              :id="`${id}-fill`"
              type="color"
              class="color-field-input"
              :value="layer.fill"
              @input="emit('patch', { fill: inputValue($event) })"
              @change="emit('commit')"
            />
            <code class="color-field-hex">{{ layer.fill }}</code>
          </span>
        </FormField>
        <FormField label="Outline" :html-for="`${id}-stroke`">
          <span class="color-field">
            <input
              :id="`${id}-stroke`"
              type="color"
              class="color-field-input"
              :value="layer.stroke"
              @input="emit('patch', { stroke: inputValue($event) })"
              @change="emit('commit')"
            />
            <code class="color-field-hex">{{ layer.stroke }}</code>
          </span>
        </FormField>
      </div>

      <EmaSlider
        :model-value="layer.strokeWidth"
        label="Outline width"
        :min="0"
        :max="16"
        unit=" px"
        @update:model-value="(strokeWidth) => emit('patch', { strokeWidth })"
        @change="emit('commit')"
      />
    </template>

    <EmaSlider
      :model-value="layer.opacity"
      label="Opacity"
      :min="0.05"
      :max="1"
      :step="0.05"
      percent
      @update:model-value="(opacity) => emit('patch', { opacity })"
      @change="emit('commit')"
    />
  </div>
</template>
