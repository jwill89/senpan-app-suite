<script setup lang="ts">
/**
 * The Ema designer's layer list, top layer first as image editors list them. A
 * row selects its layer; its buttons hide or show it, move it a step up or down
 * the stack, or delete it. Every change is undoable, so none of them asks first.
 */
import { computed } from 'vue'
import { type EmaLayers, isAtEnd, layerLabel, topFirst } from '@/lib/emaDesigner'

const props = defineProps<{ layers: EmaLayers; selectedId: string | null }>()
const emit = defineEmits<{
  select: [id: string]
  toggleVisible: [id: string]
  move: [id: string, direction: 'up' | 'down']
  remove: [id: string]
}>()

const rows = computed(() =>
  topFirst(props.layers).map((layer) => ({
    layer,
    label: layerLabel(layer),
    isTop: isAtEnd(props.layers, layer.id, 'up'),
    isBottom: isAtEnd(props.layers, layer.id, 'down'),
  })),
)
</script>

<template>
  <ul class="list-stack">
    <li
      v-for="row in rows"
      :key="row.layer.id"
      class="ema-layer"
      :class="{ 'is-selected': row.layer.id === selectedId, 'is-hidden': !row.layer.visible }"
    >
      <button
        type="button"
        class="ema-layer-pick"
        :aria-pressed="row.layer.id === selectedId"
        @click="emit('select', row.layer.id)"
      >
        <font-awesome-icon :icon="['fas', row.layer.kind === 'image' ? 'image' : 'font']" />
        <span class="ema-layer-name">{{ row.label }}</span>
      </button>
      <button
        type="button"
        class="btn-neutral btn-sm ema-icon-btn"
        :aria-label="`${row.layer.visible ? 'Hide' : 'Show'} ${row.label}`"
        :title="row.layer.visible ? 'Hide' : 'Show'"
        @click="emit('toggleVisible', row.layer.id)"
      >
        <font-awesome-icon :icon="['fas', row.layer.visible ? 'eye' : 'eye-slash']" />
      </button>
      <button
        type="button"
        class="btn-neutral btn-sm ema-icon-btn"
        :aria-label="`Move ${row.label} up`"
        title="Move up"
        :disabled="row.isTop"
        @click="emit('move', row.layer.id, 'up')"
      >
        <font-awesome-icon :icon="['fas', 'chevron-up']" />
      </button>
      <button
        type="button"
        class="btn-neutral btn-sm ema-icon-btn"
        :aria-label="`Move ${row.label} down`"
        title="Move down"
        :disabled="row.isBottom"
        @click="emit('move', row.layer.id, 'down')"
      >
        <font-awesome-icon :icon="['fas', 'chevron-down']" />
      </button>
      <button
        type="button"
        class="btn-danger btn-sm ema-icon-btn"
        :aria-label="`Delete ${row.label}`"
        title="Delete"
        @click="emit('remove', row.layer.id)"
      >
        <font-awesome-icon :icon="['fas', 'trash']" />
      </button>
    </li>
  </ul>
</template>
