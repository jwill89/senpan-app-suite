<script setup lang="ts">
/**
 * Admin Announcements tab — the first item under "Senpan Tea House".
 *
 * List-first: the default screen shows existing announcements (with a search
 * box). Creating/editing an announcement, managing types, and creating/editing a
 * type each open as their own full-tab screen (a lightweight in-tab router via
 * `screen`), so the long form isn't shown all the time.
 *
 *   - Types: name + Discord webhook of a posting destination.
 *   - Announcements: title, type, optional start/end window, markdown details,
 *     optional uploaded-or-reused image, and an optional schedule (one-time,
 *     daily, weekly multi-weekday, or monthly Nth-weekday). Recurring schedules
 *     carry an IANA timezone so their wall-clock time survives DST. Each can be
 *     sent immediately, have its next occurrence skipped, edited, or deleted.
 *
 * Absolute times (start/end, one-time) are entered as local wall-clock and stored
 * as UTC (the store converts via lib/datetime.ts); displayed in the viewer's zone.
 */
import { computed, ref } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import ListRow from '@/components/common/ui/ListRow.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import ImagePicker from '@/components/common/ui/ImagePicker.vue'
import EmojiPickerModal from '@/components/common/EmojiPickerModal.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormRow from '@/components/common/ui/FormRow.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { useAnnouncementsStore } from '@/stores/announcements'
import { detailPartCount } from '@/lib/announcementDetails'
import { formatServerTimestamp } from '@/lib/datetime'
import { supportedTimezones } from '@/lib/constants'
import { nextUid } from '@/lib/uid'
import type { Announcement, AnnouncementType, AnnouncementRole } from '@/types/api'
import { DISCORD_TIME_FORMATS } from '@/types/api'

const store = useAnnouncementsStore()

/** Which full-tab screen is showing. */
type Screen = 'list' | 'form' | 'types' | 'type-form' | 'roles' | 'role-form'
const screen = ref<Screen>('list')

const timezones = supportedTimezones()
const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const WEEK_OF_MONTH = [
  { value: 1, label: '1st' },
  { value: 2, label: '2nd' },
  { value: 3, label: '3rd' },
  { value: 4, label: '4th' },
  { value: -1, label: 'Last' },
]

const hasTypes = computed(() => store.types.length > 0)

// Drag-and-drop reordering of the list is only allowed when the full list is
// shown (no search / category filter) — reordering a filtered subset would be
// ambiguous to persist. When filtered, rows still render (v-show) but can't be
// dragged, and a hint explains why.
const canReorder = computed(() => !store.search.trim() && !store.typeFilter)
/** Ids currently passing the search/category filter (for v-show in the list). */
const visibleIds = computed(() => new Set(store.filteredAnnouncements.map((a) => a.id)))
/** Persist the order after a drag (vue-draggable-plus has already mutated the array). */
function onReorder(): void {
  void store.reorder(store.announcements.map((a) => a.id))
}

// How many Discord embed fields the current details will occupy. >1 means the
// post will be split (at line breaks) to stay under Discord's 1024-char field cap.
const detailParts = computed(() => detailPartCount(store.form.details))

/** Options for the optional "tag a role" dropdown on the announcement form. */
const mentionOptions = computed(() => [
  { value: '', label: 'Do Not Tag' },
  { value: 'everyone', label: 'Tag @everyone' },
  ...store.roles.map((r) => ({ value: `role:${r.id}`, label: `Tag @${r.name}` })),
])

const isRecurring = computed(() =>
  ['daily', 'weekly', 'monthly'].includes(store.form.schedule_kind),
)

const typeName = (a: Announcement): string =>
  a.type_name || store.types.find((t) => t.id === a.type_id)?.name || '—'

/** Format a stored UTC instant in the announcement's own timezone. */
function inZone(iso: string, tz: string): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleString(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: tz || undefined,
    })
  } catch {
    return formatServerTimestamp(iso)
  }
}

function scheduleLabel(a: Announcement): string {
  switch (a.schedule_kind) {
    case 'once':
      return 'One-time'
    case 'daily':
      return 'Daily'
    case 'weekly':
      return 'Weekly'
    case 'monthly':
      return 'Monthly'
    default:
      return 'Not scheduled'
  }
}

function maskWebhook(url: string): string {
  if (!url) return 'No webhook set'
  return `…${url.slice(-6)}`
}

function toggleWeekday(day: number): void {
  const idx = store.form.weekdays.indexOf(day)
  if (idx === -1) store.form.weekdays.push(day)
  else store.form.weekdays.splice(idx, 1)
}

// ── Discord buttons (up to 5 link buttons under the embed) ───────────────────
const MAX_BUTTONS = 5
function addButton(): void {
  if (store.form.buttons.length >= MAX_BUTTONS) return
  store.form.buttons.push({ label: '', emoji: '', url: '', _uid: nextUid() })
}
function removeButton(i: number): void {
  store.form.buttons.splice(i, 1)
}

/** Which button row's emoji picker is open (null = none). */
const emojiPickerRow = ref<number | null>(null)
function onButtonEmoji(emoji: string): void {
  if (emojiPickerRow.value !== null) store.form.buttons[emojiPickerRow.value].emoji = emoji
  emojiPickerRow.value = null
}

// ── Navigation ───────────────────────────────────────────────────────────────
function openNew(): void {
  store.resetForm()
  screen.value = 'form'
}
function openEdit(a: Announcement): void {
  store.editAnnouncement(a)
  screen.value = 'form'
}
function backToList(): void {
  store.resetForm()
  screen.value = 'list'
}
async function submit(): Promise<void> {
  if (await store.save()) screen.value = 'list'
}

function openTypes(): void {
  store.resetTypeForm()
  screen.value = 'types'
}
function openNewType(): void {
  store.resetTypeForm()
  screen.value = 'type-form'
}
function openEditType(t: AnnouncementType): void {
  store.editType(t)
  screen.value = 'type-form'
}
async function submitType(): Promise<void> {
  if (await store.saveType()) screen.value = 'types'
}

function openRoles(): void {
  store.resetRoleForm()
  screen.value = 'roles'
}
function openNewRole(): void {
  store.resetRoleForm()
  screen.value = 'role-form'
}
function openEditRole(r: AnnouncementRole): void {
  store.editRole(r)
  screen.value = 'role-form'
}
async function submitRole(): Promise<void> {
  if (await store.saveRole()) screen.value = 'roles'
}
</script>

<template>
  <div class="tab-body">
    <!-- ── List ───────────────────────────────────────────────────────────── -->
    <ManagerView v-if="screen === 'list'" title="Announcements" :icon="['fad', 'megaphone']">
      <template #actions>
        <button class="btn-view btn-sm" @click="openTypes()">
          <font-awesome-icon :icon="['fad', 'folder-open']" /> Manage Types
        </button>
        <button class="btn-view btn-sm" @click="openRoles()">
          <font-awesome-icon :icon="['fad', 'at']" /> Manage Roles
        </button>
        <button class="btn-confirm btn-sm" @click="openNew()">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Announcement
        </button>
      </template>

      <template #toolbar>
        <SearchInput
          v-model="store.search"
          placeholder="Search announcements…"
          aria-label="Search announcements"
        />
        <select
          v-model.number="store.typeFilter"
          aria-label="Filter by category"
          class="manager-filter"
        >
          <option :value="0">All categories</option>
          <option v-for="t in store.sortedTypes" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select>
      </template>

      <EmptyState v-if="!hasTypes">
        Create an <strong>Announcement Type</strong> (with a Discord webhook) under
        <button class="link-btn" @click="openTypes()">Manage Types</button> first — every
        announcement posts through a type.
      </EmptyState>

      <LoadingSpinner
        v-if="store.loading && store.announcements.length === 0"
        block
        label="Loading announcements…"
      />
      <template v-else>
        <p v-if="store.announcements.length > 1" class="text-dim text-xs mb-12">
          <template v-if="canReorder">
            <font-awesome-icon :icon="['fad', 'bars']" /> Drag a row by its handle to reorder the
            list. The order is saved automatically.
          </template>
          <template v-else>
            Clear the search and category filter to drag-and-drop reorder the list.
          </template>
        </p>

        <VueDraggable
          v-if="store.announcements.length"
          v-show="store.filteredAnnouncements.length"
          v-model="store.announcements"
          class="list-rows"
          handle=".ann-drag"
          :animation="150"
          ghost-class="dragging"
          :disabled="!canReorder"
          @end="onReorder"
        >
          <ListRow v-for="a in store.announcements" v-show="visibleIds.has(a.id)" :key="a.id">
            <template #media>
              <span
                v-if="canReorder"
                class="ann-drag drag-handle"
                title="Drag to reorder"
                aria-label="Drag to reorder"
              >
                <font-awesome-icon :icon="['fad', 'bars']" />
              </span>
              <span
                class="ann-swatch"
                :style="{ background: a.color || '#ff3131' }"
                aria-hidden="true"
              ></span>
              <img
                v-if="a.image"
                :src="a.image"
                class="media-cover media-cover--wide"
                alt="Announcement image"
              />
              <div v-else class="media-cover media-cover--wide media-empty">
                <font-awesome-icon :icon="['fad', 'image']" />
              </div>
            </template>

            <h4 class="ann-title">{{ a.title }}</h4>
            <p class="text-sm text-dim ann-meta">
              <font-awesome-icon :icon="['fad', 'folder-open']" /> {{ typeName(a) }}
            </p>
            <p v-if="a.start_at" class="text-sm ann-meta">
              <font-awesome-icon :icon="['fad', 'calendar-days']" />
              {{ inZone(a.start_at, a.timezone) }}
              <span v-if="a.end_at">– {{ inZone(a.end_at, a.timezone) }}</span>
              <span v-if="a.timezone" class="text-dim">({{ a.timezone }})</span>
            </p>
            <p class="text-sm ann-meta">
              <span v-if="a.schedule_kind" class="badge badge--accent ann-badge">
                {{ scheduleLabel(a) }}
                <template v-if="a.next_post_at">
                  · next {{ inZone(a.next_post_at, a.timezone) }}
                </template>
              </span>
              <span v-else class="badge badge--muted ann-badge">Manual only</span>
              <span v-if="a.skip_next" class="badge badge--warning ann-badge">⏭ next skipped</span>
            </p>

            <template #actions>
              <button
                class="btn-action btn-sm"
                :disabled="store.sendingId === a.id"
                title="Post to Discord now"
                @click="store.sendNow(a)"
              >
                <LoadingSpinner v-if="store.sendingId === a.id" label="Sending…" />
                <template v-else
                  ><font-awesome-icon :icon="['fas', 'paper-plane']" /> Send now</template
                >
              </button>
              <button
                v-if="a.schedule_kind && a.next_post_at"
                class="btn-caution btn-sm"
                :disabled="store.skippingId === a.id || a.skip_next"
                title="Skip the next scheduled occurrence"
                @click="store.skipNext(a)"
              >
                <font-awesome-icon :icon="['fas', 'forward-step']" /> Skip next
              </button>
              <button
                class="btn-confirm btn-sm"
                aria-label="Edit"
                title="Edit"
                @click="openEdit(a)"
              >
                <font-awesome-icon :icon="['fas', 'pen-to-square']" />
              </button>
              <button
                class="btn-danger btn-sm"
                aria-label="Delete"
                title="Delete"
                @click="store.deleteAnnouncement(a)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </template>
          </ListRow>
        </VueDraggable>
        <EmptyState
          v-if="store.announcements.length && !store.filteredAnnouncements.length"
          text="No announcements match your filters."
        />
        <EmptyState
          v-else-if="!store.announcements.length"
          text="No announcements yet. Create one with “New Announcement”."
        />
      </template>
    </ManagerView>

    <!-- ── Announcement form ──────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'form'">
      <SubPageHeader
        :icon="['fad', 'megaphone']"
        :title="store.form.id ? 'Edit Announcement' : 'New Announcement'"
        @back="backToList()"
      />

      <div class="flex-row mb-10">
        <FormField label="Title" required style="flex: 2; min-width: 200px">
          <input
            v-model="store.form.title"
            placeholder="e.g. Saturday Tea Social"
            aria-label="Announcement title"
          />
        </FormField>
        <FormField label="Type" required style="flex: 1; min-width: 160px">
          <select v-model.number="store.form.type_id" aria-label="Announcement type">
            <option :value="0" disabled>Select a type…</option>
            <option v-for="t in store.sortedTypes" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
        </FormField>
      </div>

      <div class="flex-row items-start mb-10">
        <FormField
          label="Timezone"
          required
          style="flex: 1 1 auto; min-width: 200px"
          help="Anchors every time (event window + schedule); times stay put across DST."
        >
          <select v-model="store.form.timezone" aria-label="Timezone">
            <option v-for="tz in timezones" :key="tz" :value="tz">{{ tz }}</option>
          </select>
        </FormField>
        <FormField
          label="Embed color"
          style="flex: 0 0 auto"
          help="Accent stripe on the embed's left edge."
        >
          <div class="ann-color-row">
            <input
              v-model="store.form.color"
              type="color"
              class="ann-color-input"
              aria-label="Embed accent color"
            />
            <code class="ann-color-hex">{{ store.form.color }}</code>
            <button
              type="button"
              class="btn-neutral btn-sm"
              :disabled="store.form.color === '#ff3131'"
              @click="store.form.color = '#ff3131'"
            >
              Reset
            </button>
          </div>
        </FormField>
      </div>

      <FormRow>
        <FormField label="Start (optional)">
          <input
            v-model="store.form.start_local"
            type="datetime-local"
            aria-label="Start date and time"
          />
        </FormField>
        <FormField label="End (optional)">
          <input
            v-model="store.form.end_local"
            type="datetime-local"
            aria-label="End date and time"
          />
        </FormField>
      </FormRow>

      <FormField
        v-if="store.form.start_local"
        label="Dynamic dates"
        help="For recurring day-of events: each time this announcement posts, the Start/End shift to the day it's sent, keeping the same time of day (and a next-day end stays next-day). The dates above define that time-of-day template — e.g. set the first occurrence's 10pm–1am and every post shows that day's 10pm–1am."
      >
        <label class="checkbox-inline">
          <input v-model="store.form.dynamic_dates" type="checkbox" />
          Re-anchor the Start/End to the day each post is sent
        </label>
      </FormField>

      <FormRow v-if="store.form.start_local || store.form.end_local">
        <FormField
          v-if="store.form.start_local"
          label="Start format"
          help="How the start displays in Discord. Each viewer still sees their own time zone."
        >
          <select v-model="store.form.start_format" aria-label="Discord start time format">
            <option v-for="fmt in DISCORD_TIME_FORMATS" :key="fmt.value" :value="fmt.value">
              {{ fmt.label }}
            </option>
          </select>
        </FormField>
        <FormField
          v-if="store.form.end_local"
          label="End format"
          help="How the end displays in Discord. Each viewer still sees their own time zone."
        >
          <select v-model="store.form.end_format" aria-label="Discord end time format">
            <option v-for="fmt in DISCORD_TIME_FORMATS" :key="fmt.value" :value="fmt.value">
              {{ fmt.label }}
            </option>
          </select>
        </FormField>
      </FormRow>

      <FormField
        label="Location (optional)"
        help="Free text shown on the embed, e.g. a Discord voice channel."
      >
        <input
          v-model="store.form.location"
          placeholder="e.g. Discord — Voice Channel 1"
          aria-label="Location"
        />
      </FormField>

      <FormField label="Details" required>
        <MarkdownEditor
          v-model="store.form.details"
          min-height="120px"
          placeholder="The announcement body (supports markdown — bold, italics, lists, links…)"
        />
        <p v-if="detailParts > 1" class="ann-split-note text-xs">
          <font-awesome-icon :icon="['fad', 'triangle-exclamation']" />
          These details exceed Discord's 1024-character limit per embed field, so the post will be
          split into <strong>{{ detailParts }}</strong> stacked sections (at line breaks). Nothing
          is cut off, and they render in order below the event time.
        </p>
      </FormField>

      <!-- Embed images (optional): pick a thumbnail and/or a main image from the
           central Images page categories. Upload new images on System → Images. -->
      <FormField
        label="Embed images (optional)"
        help="Pick a thumbnail (small, top-right) and/or a main image (large, bottom). Upload new images on the System → Images page (categories “Announcement Thumbnail” and “Announcement Main”)."
      >
        <div class="image-pickers">
          <div class="image-picker-block">
            <span class="field-label">Thumbnail — small, top-right</span>
            <ImagePicker v-model="store.form.thumbnail" :images="store.thumbImages" />
          </div>
          <div class="image-picker-block">
            <span class="field-label">Main image — large, bottom</span>
            <ImagePicker v-model="store.form.image" :images="store.mainImages" />
          </div>
        </div>
      </FormField>

      <!-- Tag a role: optional mention posted in the message content above the embed -->
      <FormField
        label="Tag a role (optional)"
        help="Posted above the embed (mentions inside an embed don't notify). Manage the list under “Manage Roles”."
      >
        <select v-model="store.form.mention" aria-label="Role to tag">
          <option v-for="opt in mentionOptions" :key="opt.value" :value="opt.value">
            {{ opt.label }}
          </option>
        </select>
      </FormField>

      <!-- Discord buttons: optional link buttons rendered beneath the embed -->
      <hr class="ann-divider" />
      <h4 class="section-heading"><font-awesome-icon :icon="['fab', 'discord']" /> Buttons</h4>
      <p class="text-dim text-sm mb-8">
        Up to {{ MAX_BUTTONS }} link buttons shown under the embed. Each needs a label and URL; the
        emoji is optional — click the emoji box to pick one.
      </p>
      <div v-if="store.form.buttons.length" class="ann-buttons">
        <div v-for="(btn, i) in store.form.buttons" :key="btn._uid" class="ann-button-row">
          <button
            type="button"
            class="ann-button-emoji"
            :title="btn.emoji ? 'Change emoji' : 'Pick an emoji'"
            aria-label="Button emoji"
            @click="emojiPickerRow = i"
          >
            <span v-if="btn.emoji">{{ btn.emoji }}</span>
            <font-awesome-icon v-else :icon="['fad', 'face-smile']" />
          </button>
          <input
            v-model="btn.label"
            class="ann-button-label"
            placeholder="Button label"
            aria-label="Button label"
          />
          <input
            v-model="btn.url"
            class="ann-button-url"
            placeholder="https://…"
            aria-label="Button URL"
          />
          <button
            type="button"
            class="btn-danger btn-sm"
            aria-label="Remove button"
            title="Remove button"
            @click="removeButton(i)"
          >
            <font-awesome-icon :icon="['fas', 'trash']" />
          </button>
        </div>
      </div>
      <button
        type="button"
        class="btn-confirm btn-sm"
        :disabled="store.form.buttons.length >= MAX_BUTTONS"
        @click="addButton()"
      >
        <font-awesome-icon :icon="['fas', 'plus']" /> Add button
      </button>

      <EmojiPickerModal
        v-if="emojiPickerRow !== null"
        allow-clear
        @select="onButtonEmoji"
        @close="emojiPickerRow = null"
      />

      <!-- Scheduling: when (if ever) this announcement auto-posts to Discord -->
      <hr class="ann-divider" />
      <h4 class="section-heading"><font-awesome-icon :icon="['fad', 'clock']" /> Scheduling</h4>

      <!-- Schedule builder -->
      <FormField label="Schedule">
        <select v-model="store.form.schedule_kind" aria-label="Schedule kind">
          <option value="">Not scheduled (send manually)</option>
          <option value="once">One-time</option>
          <option value="daily">Daily</option>
          <option value="weekly">Weekly</option>
          <option value="monthly">Monthly</option>
        </select>
      </FormField>

      <FormField v-if="store.form.schedule_kind === 'once'" label="Post at" required>
        <input
          v-model="store.form.once_local"
          type="datetime-local"
          aria-label="One-time post date and time"
        />
      </FormField>

      <template v-else-if="isRecurring">
        <FormField v-if="store.form.schedule_kind === 'weekly'" label="On these days" required>
          <div class="ann-weekdays">
            <button
              v-for="(label, day) in WEEKDAYS"
              :key="day"
              type="button"
              class="toggle-btn"
              :class="{ active: store.form.weekdays.includes(day) }"
              @click="toggleWeekday(day)"
            >
              {{ label }}
            </button>
          </div>
        </FormField>

        <div v-if="store.form.schedule_kind === 'monthly'" class="flex-row mb-10">
          <FormField label="Week" required style="flex: 0 0 120px">
            <select v-model.number="store.form.week_of_month" aria-label="Week of month">
              <option v-for="w in WEEK_OF_MONTH" :key="w.value" :value="w.value">
                {{ w.label }}
              </option>
            </select>
          </FormField>
          <FormField label="Weekday" required style="flex: 0 0 140px">
            <select
              :value="store.form.weekdays[0] ?? ''"
              aria-label="Weekday"
              @change="store.form.weekdays = [Number(($event.target as HTMLSelectElement).value)]"
            >
              <option value="" disabled>Pick…</option>
              <option v-for="(label, day) in WEEKDAYS" :key="day" :value="day">{{ label }}</option>
            </select>
          </FormField>
        </div>

        <FormField label="Time" required help="In the timezone selected above.">
          <input
            v-model="store.form.time_local"
            type="time"
            style="max-width: 160px"
            aria-label="Recurring post time"
          />
        </FormField>
      </template>

      <FormActions align="start">
        <button class="btn-neutral" @click="backToList()">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="store.saving || !store.form.title.trim()"
          @click="submit()"
        >
          <LoadingSpinner v-if="store.saving" label="Saving…" />
          <template v-else>{{ store.form.id ? 'Save Changes' : 'Create Announcement' }}</template>
        </button>
      </FormActions>
    </AdminPanel>

    <!-- ── Types list ─────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'types'">
      <SubPageHeader
        title="Announcement Types"
        :icon="['fad', 'folder-open']"
        @back="screen = 'list'"
      />
      <div class="flex-toolbar flex-end mb-16">
        <button class="btn-confirm btn-sm" @click="openNewType()">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Type
        </button>
      </div>

      <div v-if="store.types.length" class="list-rows">
        <ListRow v-for="t in store.sortedTypes" :key="t.id">
          <h4 class="ann-title">{{ t.name }}</h4>
          <p class="text-sm text-dim">
            <font-awesome-icon :icon="['fab', 'discord']" /> {{ maskWebhook(t.webhook_url) }}
          </p>
          <template #actions>
            <button class="btn-confirm btn-sm" aria-label="Edit type" @click="openEditType(t)">
              <font-awesome-icon :icon="['fas', 'pen-to-square']" />
            </button>
            <button class="btn-danger btn-sm" aria-label="Delete type" @click="store.deleteType(t)">
              <font-awesome-icon :icon="['fas', 'trash']" />
            </button>
          </template>
        </ListRow>
      </div>
      <EmptyState v-else text="No types yet. Add one with “New Type”." />
    </AdminPanel>

    <!-- ── Type form ──────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'type-form'">
      <SubPageHeader
        :icon="['fad', 'folder-open']"
        :title="store.typeForm.id ? 'Edit Type' : 'New Type'"
        @back="screen = 'types'"
      />

      <FormField label="Name" required>
        <input
          v-model="store.typeForm.name"
          placeholder="e.g. Events Channel"
          aria-label="Type name"
        />
      </FormField>
      <FormField
        label="Discord webhook URL"
        help="Announcements of this type post to this channel webhook."
      >
        <input
          v-model="store.typeForm.webhook_url"
          placeholder="https://discord.com/api/webhooks/…"
          aria-label="Discord webhook URL"
        />
      </FormField>
      <FormActions align="start">
        <button class="btn-neutral" @click="screen = 'types'">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="store.savingType || !store.typeForm.name.trim()"
          @click="submitType()"
        >
          <LoadingSpinner v-if="store.savingType" label="Saving…" />
          <template v-else>{{ store.typeForm.id ? 'Save Changes' : 'Add Type' }}</template>
        </button>
      </FormActions>
    </AdminPanel>

    <!-- ── Roles list ─────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'roles'">
      <SubPageHeader title="Taggable Roles" :icon="['fad', 'at']" @back="screen = 'list'" />
      <p class="text-dim text-sm mb-16">
        Roles available to tag on an announcement. Each is a group name plus its Discord role ID
        (enable Developer Mode in Discord, right-click a role → “Copy Role ID”).
      </p>
      <div class="flex-toolbar flex-end mb-16">
        <button class="btn-confirm btn-sm" @click="openNewRole()">
          <font-awesome-icon :icon="['fas', 'plus']" /> New Role
        </button>
      </div>

      <div v-if="store.roles.length" class="list-rows">
        <ListRow v-for="r in store.roles" :key="r.id">
          <h4 class="ann-title">{{ r.name }}</h4>
          <p class="text-sm text-dim">
            <font-awesome-icon :icon="['fad', 'at']" /> {{ r.role_id }}
          </p>
          <template #actions>
            <button class="btn-confirm btn-sm" aria-label="Edit role" @click="openEditRole(r)">
              <font-awesome-icon :icon="['fas', 'pen-to-square']" />
            </button>
            <button class="btn-danger btn-sm" aria-label="Delete role" @click="store.deleteRole(r)">
              <font-awesome-icon :icon="['fas', 'trash']" />
            </button>
          </template>
        </ListRow>
      </div>
      <EmptyState v-else text="No roles yet. Add one with “New Role”." />
    </AdminPanel>

    <!-- ── Role form ──────────────────────────────────────────────────────── -->
    <AdminPanel v-else-if="screen === 'role-form'">
      <SubPageHeader
        :icon="['fad', 'at']"
        :title="store.roleForm.id ? 'Edit Role' : 'New Role'"
        @back="screen = 'roles'"
      />

      <FormField label="Group name" required>
        <input
          v-model="store.roleForm.name"
          placeholder="e.g. Event Crew"
          aria-label="Role group name"
        />
      </FormField>
      <FormField
        label="Discord role ID"
        required
        help="The numeric role ID. In Discord: Settings → Advanced → Developer Mode, then right-click the role → Copy Role ID."
      >
        <input
          v-model="store.roleForm.role_id"
          placeholder="e.g. 123456789012345678"
          inputmode="numeric"
          aria-label="Discord role ID"
        />
      </FormField>
      <FormActions align="start">
        <button class="btn-neutral" @click="screen = 'roles'">Cancel</button>
        <button
          class="btn-confirm"
          :disabled="
            store.savingRole || !store.roleForm.name.trim() || !store.roleForm.role_id.trim()
          "
          @click="submitRole()"
        >
          <LoadingSpinner v-if="store.savingRole" label="Saving…" />
          <template v-else>{{ store.roleForm.id ? 'Save Changes' : 'Add Role' }}</template>
        </button>
      </FormActions>
    </AdminPanel>
  </div>
</template>

<style scoped>
/* Inline checkbox + label (e.g. the dynamic-dates toggle). */
.checkbox-inline {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

/* The two embed-image pickers (thumbnail + main) stacked below the shared upload. */
.image-pickers {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 12px;
}
.image-picker-block .field-label {
  margin-bottom: 6px;
}

.link-btn {
  background: none;
  border: none;
  color: var(--accent);
  cursor: pointer;
  text-decoration: underline;
  padding: 0;
  font: inherit;
}
.ann-swatch {
  width: 6px;
  align-self: stretch;
  border-radius: 3px;
  flex: 0 0 auto;
}
/* Drag handle for reordering the list; grab cursor + muted until hovered. */
.ann-drag {
  display: inline-flex;
  align-items: center;
  align-self: stretch;
  padding: 0 2px;
  color: var(--text-muted);
  cursor: grab;
}
.ann-drag:hover {
  color: var(--highlight);
}
.ann-drag:active {
  cursor: grabbing;
}
/* vue-draggable-plus ghost while dragging a row. */
.dragging {
  opacity: 0.5;
}
.ann-title {
  margin: 0 0 4px;
}
/* Heads-up note under the Details editor when the post will be split into
   multiple embed fields (long details past Discord's 1024-char field cap). */
.ann-split-note {
  margin: 8px 0 0;
  color: var(--text-muted);
}
.ann-split-note strong {
  color: var(--highlight);
}
.ann-meta {
  margin: 0 0 4px;
}
.ann-divider {
  border: none;
  border-top: 1px solid var(--panel-raised-bg);
  margin: 20px 0 12px;
}
.ann-buttons {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 10px;
}
.ann-button-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.ann-button-emoji {
  flex: 0 0 56px;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 44px;
  font-size: 1.2rem;
  background: var(--input-bg);
  color: var(--text);
  border: 2px solid var(--control-border);
  border-radius: var(--radius);
  cursor: pointer;
}
.ann-button-emoji:hover {
  border-color: var(--accent);
}
.ann-button-label {
  flex: 1 1 160px;
  min-width: 120px;
}
.ann-button-url {
  flex: 2 1 220px;
  min-width: 160px;
}
.ann-color-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.ann-color-input {
  width: 48px;
  height: 36px;
  padding: 2px;
  border: 1px solid var(--panel-raised-bg);
  border-radius: 6px;
  background: var(--panel-bg);
  cursor: pointer;
}
.ann-color-hex {
  font-family: monospace;
  text-transform: uppercase;
  color: var(--text-muted);
}
/* Weekday buttons are `.toggle-btn`s; this is just their flex container. */
.ann-weekdays {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
/* Status pills use the global `.badge` object + shared `.badge--*` state
   modifiers; only the inter-pill spacing is component-specific. */
.ann-badge {
  margin-right: 6px;
}
</style>
