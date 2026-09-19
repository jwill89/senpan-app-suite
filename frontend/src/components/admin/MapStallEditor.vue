<script setup lang="ts">
/**
 * Interactive Festival Map editor. Renders the base floor-plan image with each
 * stall drawn on top at its %-based {@link Placement} - a circle or a rectangle in
 * the stall's color, with its title inside - and lets the admin DRAG a stall to
 * move it, drag the bottom-right handle to RESIZE it, and drag the top handle to
 * ROTATE it. Positions are percentages of the map box, so they render identically
 * at any display size (read-only view = FestivalMapCanvas).
 *
 * The parent owns the stall array; this component is controlled - it emits
 * `select` and `update(uid, placement)` rather than mutating props. Continuous
 * drags emit on each pointermove; the parent applies the placement to its stall.
 */
import { ref } from 'vue'
import { assetUrl } from '@/lib/assets'
import { isCircle, occupantMapLabel, stallCaption, stallColor, stallStyle } from '@/lib/festivalmap'
import { useFestivalMapsStore } from '@/stores/festivalMaps'
import { usePlacementDrag, type DragMode } from '@/composables/usePlacementDrag'
import type { FestivalStallForm, FestivalStallOccupantForm, Placement } from '@/types/api'

defineProps<{
  mapImage: string
  stalls: FestivalStallForm[]
  /** `_uid` of the selected stall (null = nothing selected). */
  selectedUid: number | null
}>()

const emit = defineEmits<{
  select: [uid: number | null]
  update: [uid: number, placement: Placement]
}>()

const canvasRef = ref<HTMLElement | null>(null)

const store = useFestivalMapsStore()

/**
 * What a pitch is labelled with here: its AFFILIATE, the same as the public map.
 * Labelling by title showed "Untitled stall" on every pitch whose title was left
 * blank - which is most of them, since a title is optional and the map never uses
 * one - naming a pitch after the fact that it has no name.
 *
 * The form holds an affiliate ID rather than a name, so it is resolved against the
 * same list the affiliate picker offers. No match (or none chosen) is the venue's
 * own booth, which occupantMapLabel names.
 */
function editorLabel(occupant: FestivalStallOccupantForm): string {
  const affiliate = store.affiliates.find((a) => a.id === occupant.affiliate_id)
  return occupantMapLabel({ affiliate_name: affiliate?.name ?? '' })
}

const { beginDrag: startDrag } = usePlacementDrag<number>(canvasRef, (uid, placement) =>
  emit('update', uid, placement),
)

/**
 * Selects the stall, then starts dragging it (grabbing one always picks it).
 * A circle drags as a SQUARE - its height follows its width on screen, so the
 * geometry has to bound it by what is actually drawn, not by the stored height.
 */
function beginDrag(mode: DragMode, stall: FestivalStallForm, e: PointerEvent): void {
  emit('select', stall._uid ?? 0)
  startDrag(mode, stall._uid ?? 0, stall.placement, e, { square: isCircle(stall.shape) })
}

/**
 * Keyboard equivalent of grabbing a pitch. Dragging is inherently pointer-only, but
 * SELECTING one must not be: once selected, the form's numeric position, size and
 * rotation fields edit the same values a drag would, so a keyboard user can place a
 * stall precisely. Without this the whole editor was unreachable without a mouse -
 * the pitches were bare divs carrying only @pointerdown.
 */
function selectStall(stall: FestivalStallForm): void {
  emit('select', stall._uid ?? 0)
}

/** A stall's accessible name: who is in it, plus its place in the list. */
function stallLabel(stall: FestivalStallForm, index: number): string {
  const names = stall.occupants
    .map((o) => o.title.trim())
    .filter(Boolean)
    .join(', ')
  return `Pitch ${index + 1}${names ? `: ${names}` : ''} - select to edit its position and size`
}

/** Click on empty map area -> deselect. */
function onCanvasPointerDown(e: PointerEvent): void {
  const target = e.target as HTMLElement
  if (target === canvasRef.value || target.classList.contains('map-canvas-bg')) {
    emit('select', null)
  }
}
</script>

<template>
  <div>
    <div ref="canvasRef" class="map-canvas is-editable" @pointerdown="onCanvasPointerDown">
      <img
        v-if="mapImage"
        :src="assetUrl(mapImage)"
        class="map-canvas-bg"
        alt="Festival map"
        draggable="false"
      />
      <div v-else class="map-canvas-bg map-canvas-empty">
        <font-awesome-icon :icon="['fad', 'image']" /> Pick a base map image
      </div>

      <div
        v-for="(stall, stallIndex) in stalls"
        :key="stall._uid"
        class="map-stall"
        :class="{
          'is-selected': stall._uid === selectedUid,
          'map-stall--round': isCircle(stall.shape),
        }"
        :style="
          stallStyle(
            stall.placement,
            stall.shape,
            stallColor(stall),
            '',
            stall.text_color,
            stall.occupants.length,
          )
        "
        role="button"
        tabindex="0"
        :aria-pressed="stall._uid === selectedUid"
        :aria-label="stallLabel(stall, stallIndex)"
        @pointerdown="beginDrag('move', stall, $event)"
        @keydown.enter.prevent="selectStall(stall)"
        @keydown.space.prevent="selectStall(stall)"
      >
        <span class="map-stall-label">
          <span v-for="occupant in stall.occupants" :key="occupant._uid" class="map-stall-occupant">
            <span>{{ editorLabel(occupant) }}</span>
            <span v-if="stallCaption(occupant)" class="map-stall-caption">{{
              stallCaption(occupant)
            }}</span>
          </span>
        </span>

        <template v-if="stall._uid === selectedUid">
          <!-- Rotate handle (above, top-centre) -->
          <button
            class="placement-handle placement-rotate"
            type="button"
            aria-label="Rotate"
            title="Drag to rotate"
            @pointerdown.stop="beginDrag('rotate', stall, $event)"
          >
            <font-awesome-icon :icon="['fas', 'rotate']" />
          </button>
          <!-- Resize handle (bottom-right) -->
          <button
            class="placement-handle placement-resize"
            type="button"
            aria-label="Resize"
            title="Drag to resize"
            @pointerdown.stop="beginDrag('resize', stall, $event)"
          ></button>
        </template>
      </div>
    </div>
    <p class="placement-hint text-muted text-xs">
      Click a stall to select it, then drag it to move - drag the square handle at its bottom-right
      to resize - drag the round handle above it to rotate. A circle resizes by its diameter, since
      its height follows its width. Positions are saved as a share of the map, so they scale to any
      screen.
    </p>
  </div>
</template>
