<script setup lang="ts">
/**
 * Admin Festival Map create/edit form. Festival fields (title, markdown
 * description, the datetime ranges it runs across, the base floor-plan image),
 * then the visual editor where stalls are dragged/resized/rotated onto the map,
 * with a per-stall panel below for the selected stall's settings - who runs it,
 * what it offers, how it is drawn, and its own opening times.
 *
 * Hosted as a Back sub-page of the Festival Map manager: emits `saved` / `cancel`.
 */
import { computed, onMounted, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormRow from '@/components/common/ui/FormRow.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import MapStallEditor from './MapStallEditor.vue'
import { blankEventTime, useFestivalMapsStore } from '@/stores/festivalMaps'
import { STALL_TYPES, stallCaption, stallColor, stallTypeMeta } from '@/lib/festivalmap'
import type {
  EventTimeForm,
  FestivalStallOccupantForm,
  Placement,
  StallShape,
  StallType,
} from '@/types/api'

const emit = defineEmits<{ saved: []; cancel: [] }>()
const store = useFestivalMapsStore()

onMounted(() => store.loadFormSources())

/** `_uid` of the stall being edited (null = none selected). */
const selectedUid = ref<number | null>(null)

const stalls = computed(() => store.mapForm?.stalls ?? [])

/** The selected stall plus its index, for the editing panel below the map. */
const selected = computed(() => {
  const index = stalls.value.findIndex((s) => s._uid === selectedUid.value)
  if (index < 0) return null
  return { index, stall: stalls.value[index] }
})

/** Apply an editor placement change back onto the matching stall. */
function applyUpdate(uid: number, placement: Placement): void {
  const stall = stalls.value.find((s) => s._uid === uid)
  if (stall) Object.assign(stall.placement, placement)
}

function addStall(type: StallType): void {
  selectedUid.value = store.addStall(type)?._uid ?? null
}

function removeSelected(): void {
  const sel = selected.value
  if (!sel) return
  store.removeStall(sel.index)
  selectedUid.value = null
}

/** Set an occupant's operator from the select ('' -> the venue itself). */
function setAffiliate(occupant: FestivalStallOccupantForm, value: string): void {
  occupant.affiliate_id = value ? Number(value) : null
}

/**
 * Switching what an occupant offers re-shapes the PITCH, since the shape is what
 * tells a visitor at a glance what kind of stall it is. Only the leading occupant
 * does this: a pitch has one shape, and letting the Day 2 tenant redraw it would
 * make the plan change under the Day 1 one.
 */
function setStallType(occupant: FestivalStallOccupantForm, value: string): void {
  const sel = selected.value
  if (!sel) return
  const type = (STALL_TYPES.find((t) => t.value === value) ?? STALL_TYPES[0]).value
  occupant.stall_type = type
  if (occupant === sel.stall.occupants[0]) sel.stall.shape = stallTypeMeta(type).shape
}

/** Adds another occupant - the business that takes the pitch on a different day. */
function addOccupant(): void {
  if (selected.value) store.addOccupant(selected.value.index)
}

/** Removes one occupant; the last is kept (a pitch with nobody in it is dropped). */
function removeOccupant(occupantIndex: number): void {
  if (selected.value) store.removeOccupant(selected.value.index, occupantIndex)
}

/** The color swatch always needs a literal, so show what the pitch renders as. */
const selectedColor = computed(() =>
  selected.value ? stallColor(selected.value.stall) : '#ffffff',
)

function setColor(value: string): void {
  if (selected.value) selected.value.stall.color = value
}

/** Drop back to the stall type's own color. */
function clearColor(): void {
  if (selected.value) selected.value.stall.color = ''
}

function setShape(value: string): void {
  if (selected.value) selected.value.stall.shape = value === 'circle' ? 'circle' : 'rect'
}

/**
 * Settles the rotation field into a whole number of degrees in 0-359. The handle
 * on the map is the quick way to turn a stall; this is the precise one, for the
 * rows of stalls that should sit at exactly the same angle. An emptied
 * `<input type="number">` binds as '', which would otherwise be sent as a string.
 */
function normalizeRotation(): void {
  const stall = selected.value?.stall
  if (!stall) return
  const degrees = stall.placement.rotation
  stall.placement.rotation = Number.isFinite(degrees)
    ? ((Math.round(degrees) % 360) + 360) % 360
    : 0
}

// -- Datetime repeaters (the festival's own, and each stall's) ----------------
function addTime(times: EventTimeForm[]): void {
  times.push(blankEventTime())
}
function removeTime(times: EventTimeForm[], index: number): void {
  times.splice(index, 1)
}

const SHAPES: { value: StallShape; label: string }[] = [
  { value: 'circle', label: 'Circle' },
  { value: 'rect', label: 'Rectangle' },
]

async function save(): Promise<void> {
  if (await store.saveMap()) emit('saved')
}
function cancel(): void {
  store.cancelMapForm()
  emit('cancel')
}
</script>

<template>
  <AdminPanel>
    <SubPageHeader
      :icon="['fad', 'map-location-dot']"
      :title="`${store.mapForm && store.mapForm.id ? 'Edit' : 'New'} Festival Map`"
      @back="cancel"
    />
    <template v-if="store.mapForm">
      <FormField label="Title" required>
        <input
          v-model="store.mapForm.title"
          placeholder="e.g. Obon Matsuri 2026"
          aria-label="Festival map title"
        />
      </FormField>

      <FormField
        label="Shortcode"
        help="Optional. Gives the map a readable link - /festival-maps/obon-2026 - instead of one built from its id. Letters, numbers and dashes; spaces are folded into dashes when saved. The map stays reachable by its id either way, so adding one later never breaks a link already posted."
      >
        <input
          v-model="store.mapForm.slug"
          placeholder="e.g. obon-2026"
          aria-label="Festival map shortcode"
          autocapitalize="none"
          autocomplete="off"
          spellcheck="false"
        />
      </FormField>

      <FormField label="Description">
        <MarkdownEditor
          v-model="store.mapForm.description"
          min-height="100px"
          placeholder="Describe the festival (supports markdown)"
        />
      </FormField>

      <FormField
        label="Festival Date(s)"
        help="When the festival runs. Add a row per day or session - each has an optional label, a start, and an optional end. Leave the list empty for a festival with no set dates."
      >
        <div class="repeater">
          <div class="repeater-row event-time-row text-muted text-xs">
            <span>Label</span>
            <span>Starts</span>
            <span>Ends</span>
            <span></span>
          </div>
          <div
            v-for="(t, i) in store.mapForm.times"
            :key="t._uid"
            class="repeater-row event-time-row"
          >
            <input v-model="t.label" placeholder="e.g. Day 1" aria-label="Date label" />
            <input v-model="t.start" type="datetime-local" aria-label="Starts" />
            <input v-model="t.end" type="datetime-local" aria-label="Ends (optional)" />
            <button
              class="btn-danger btn-sm"
              aria-label="Remove date row"
              title="Remove date row"
              @click="removeTime(store.mapForm.times, i)"
            >
              &times;
            </button>
          </div>
          <button class="btn-neutral btn-sm mt-8" @click="addTime(store.mapForm.times)">
            <font-awesome-icon :icon="['fas', 'plus']" /> Add Date
          </button>
        </div>
      </FormField>

      <FormField
        label="Base Map Image"
        help="The bare floor plan - walls, rest areas, the stage, seating. The stalls are drawn on top of it by the app, so their labels don't need to be part of the artwork. Required before the map can be published."
      >
        <ImagePicker v-model="store.mapForm.map_image" />
      </FormField>

      <!-- Stall editor -->
      <h3 class="section-heading mt-16">
        <font-awesome-icon :icon="['fad', 'shop']" /> Stalls
        <span class="text-muted text-sm">({{ stalls.length }})</span>
      </h3>
      <div class="flex-toolbar flex-end mb-10">
        <button
          v-for="t in STALL_TYPES"
          :key="t.value"
          class="btn-neutral btn-sm"
          @click="addStall(t.value)"
        >
          <font-awesome-icon :icon="['fas', 'plus']" /> Add {{ t.label }}
        </button>
      </div>

      <MapStallEditor
        :map-image="store.mapForm.map_image"
        :stalls="store.mapForm.stalls"
        :selected-uid="selectedUid"
        @select="selectedUid = $event"
        @update="applyUpdate"
      />

      <!-- Selected pitch panel: how it is drawn, then who stands in it -->
      <div v-if="selected" class="subpanel mt-16">
        <div class="flex-toolbar flex-between mb-10">
          <h4 class="section-heading m-0">
            <font-awesome-icon
              :icon="['fad', stallTypeMeta(selected.stall.occupants[0]?.stall_type ?? '').icon]"
            />
            {{ selected.stall.occupants[0]?.title || `Stall ${selected.index + 1}` }}
          </h4>
          <button class="btn-danger btn-sm" @click="removeSelected">
            <font-awesome-icon :icon="['fas', 'trash']" /> Remove Stall
          </button>
        </div>

        <FormRow>
          <FormField label="Shape">
            <select
              :value="selected.stall.shape"
              aria-label="Stall shape"
              @change="setShape(($event.target as HTMLSelectElement).value)"
            >
              <option v-for="sh in SHAPES" :key="sh.value" :value="sh.value">{{ sh.label }}</option>
            </select>
          </FormField>
          <FormField label="Rotation" help="Degrees. Or drag the round handle above the stall.">
            <input
              v-model.number="selected.stall.placement.rotation"
              type="number"
              min="0"
              max="359"
              aria-label="Stall rotation in degrees"
              @blur="normalizeRotation"
            />
          </FormField>
          <FormField label="Color" help="Reset to follow the stall type's own color.">
            <div class="color-field">
              <input
                :value="selectedColor"
                type="color"
                class="color-field-input"
                aria-label="Stall color"
                @input="setColor(($event.target as HTMLInputElement).value)"
              />
              <code class="color-field-hex">{{ selectedColor }}</code>
              <button
                type="button"
                class="btn-neutral btn-sm"
                :disabled="!selected.stall.color"
                @click="clearColor"
              >
                Reset
              </button>
            </div>
          </FormField>
        </FormRow>

        <!-- Occupants: who stands in this pitch, and when -->
        <div class="flex-toolbar flex-between mt-16 mb-10">
          <h4 class="section-heading m-0">
            <font-awesome-icon :icon="['fad', 'handshake']" /> Who's here
            <span class="text-muted text-sm">({{ selected.stall.occupants.length }})</span>
          </h4>
          <button class="btn-neutral btn-sm" @click="addOccupant">
            <font-awesome-icon :icon="['fas', 'plus']" /> Add another day
          </button>
        </div>
        <p class="text-muted text-xs mb-10">
          One entry per business standing here. Add a second when the pitch changes hands between
          days - give each its own dates and the map shows whoever is on.
        </p>

        <div
          v-for="(occupant, oi) in selected.stall.occupants"
          :key="occupant._uid"
          class="stall-occupant"
        >
          <div class="flex-toolbar flex-between mb-10">
            <strong class="text-sm">
              {{ occupant.title || `Occupant ${oi + 1}` }}
              <span v-if="stallCaption(occupant)" class="text-muted">
                - {{ stallCaption(occupant) }}
              </span>
            </strong>
            <button
              class="btn-danger btn-sm"
              :disabled="selected.stall.occupants.length <= 1"
              :title="
                selected.stall.occupants.length <= 1
                  ? 'A stall needs someone in it - remove the stall instead'
                  : 'Remove this occupant'
              "
              aria-label="Remove occupant"
              @click="removeOccupant(oi)"
            >
              &times;
            </button>
          </div>

          <FormRow>
            <FormField label="Affiliate" help="Who runs it. Defaults to the owning venue.">
              <select
                :value="occupant.affiliate_id ?? ''"
                aria-label="Stall affiliate"
                @change="setAffiliate(occupant, ($event.target as HTMLSelectElement).value)"
              >
                <option value="">Senpan Tea House (default)</option>
                <option v-for="a in store.affiliates" :key="a.id" :value="a.id">
                  {{ a.name }}
                </option>
              </select>
            </FormField>
            <FormField label="Title" help="Shown on the map itself, inside the stall's shape.">
              <input
                v-model="occupant.title"
                placeholder="e.g. Flora Teahouse (Day 1)"
                aria-label="Stall title"
              />
            </FormField>
          </FormRow>

          <FormRow>
            <FormField
              label="Offers"
              help="Drawn under the title on the map. The first occupant's choice also sets the stall's shape and color."
            >
              <select
                :value="occupant.stall_type"
                aria-label="Stall type"
                @change="setStallType(occupant, ($event.target as HTMLSelectElement).value)"
              >
                <option v-for="t in STALL_TYPES" :key="t.value" :value="t.value">
                  {{ t.label }}
                </option>
              </select>
            </FormField>
            <FormField
              v-if="occupant.stall_type === 'other'"
              label="Offers (wording)"
              help="What this stall actually offers, drawn under its title. Leave it empty and the stall shows its title alone - “Other” would tell a visitor nothing."
            >
              <input
                v-model="occupant.type_label"
                placeholder="e.g. Omikuji, Art Raffle"
                aria-label="Stall offering wording"
              />
            </FormField>
          </FormRow>

          <FormField label="Description" help="What this stall offers (supports markdown).">
            <MarkdownEditor
              v-model="occupant.description"
              min-height="90px"
              placeholder="Describe what's on offer here"
            />
          </FormField>

          <FormField
            label="Days / Hours"
            help="When this occupant is here. Give each one its own dates once a pitch has more than one - it is how the map decides who to show for a given day. Leave empty and they run the whole festival."
          >
            <div class="repeater">
              <div
                v-if="occupant.times.length"
                class="repeater-row event-time-row text-muted text-xs"
              >
                <span>Label</span>
                <span>Opens</span>
                <span>Closes</span>
                <span></span>
              </div>
              <div
                v-for="(t, i) in occupant.times"
                :key="t._uid"
                class="repeater-row event-time-row"
              >
                <input v-model="t.label" placeholder="e.g. Day 2" aria-label="Stall hours label" />
                <input v-model="t.start" type="datetime-local" aria-label="Opens" />
                <input v-model="t.end" type="datetime-local" aria-label="Closes (optional)" />
                <button
                  class="btn-danger btn-sm"
                  aria-label="Remove stall hours row"
                  title="Remove stall hours row"
                  @click="removeTime(occupant.times, i)"
                >
                  &times;
                </button>
              </div>
              <button class="btn-neutral btn-sm mt-8" @click="addTime(occupant.times)">
                <font-awesome-icon :icon="['fas', 'plus']" /> Add Hours
              </button>
            </div>
          </FormField>
        </div>
      </div>
      <p v-else class="text-muted text-sm mt-10">
        Add a stall, then click it on the map to position it and fill in who's there.
      </p>

      <FormActions align="start">
        <button class="btn-neutral" :disabled="store.savingMap" @click="cancel">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="!store.mapForm.title.trim() || store.savingMap"
          @click="save"
        >
          <LoadingSpinner v-if="store.savingMap" label="Saving..." />
          <template v-else>Save Festival Map</template>
        </button>
      </FormActions>
    </template>
  </AdminPanel>
</template>
