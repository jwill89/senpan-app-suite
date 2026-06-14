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
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import { useAnnouncementsStore } from '@/stores/announcements'
import { formatServerTimestamp } from '@/lib/datetime'
import { supportedTimezones } from '@/lib/constants'
import type { Announcement, AnnouncementType } from '@/types/api'

const store = useAnnouncementsStore()

/** Which full-tab screen is showing. */
type Screen = 'list' | 'form' | 'types' | 'type-form'
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
</script>

<template>
  <div class="tab-body">
    <!-- ── List ───────────────────────────────────────────────────────────── -->
    <div v-if="screen === 'list'" class="admin-panel">
      <div class="flex-between mb-16" style="flex-wrap: wrap; gap: 8px">
        <h3><i class="fa-duotone fa-megaphone"></i> Announcements</h3>
        <div class="flex-toolbar">
          <button class="btn-ghost btn-sm" @click="openTypes()">
            <i class="fa-duotone fa-folder-open"></i> Manage Types
          </button>
          <button class="btn-primary btn-sm" @click="openNew()">
            <i class="fa-solid fa-plus"></i> New Announcement
          </button>
        </div>
      </div>

      <p v-if="!hasTypes" class="msg-block">
        Create an <strong>Announcement Type</strong> (with a Discord webhook) under
        <button class="link-btn" @click="openTypes()">Manage Types</button> first — every
        announcement posts through a type.
      </p>

      <div class="flex-toolbar mb-12">
        <span class="ann-search">
          <i class="fa-duotone fa-magnifying-glass"></i>
          <input
            v-model="store.search"
            class="field-input-full"
            placeholder="Search announcements…"
            aria-label="Search announcements"
          />
        </span>
      </div>

      <LoadingSpinner
        v-if="store.loading && store.announcements.length === 0"
        block
        label="Loading announcements…"
      />
      <template v-else>
        <div v-if="store.filteredAnnouncements.length" class="ann-list">
          <div v-for="a in store.filteredAnnouncements" :key="a.id" class="ann-card">
            <img v-if="a.image" :src="a.image" class="ann-cover" alt="Announcement image" />
            <div v-else class="ann-cover ann-img-empty"><i class="fa-duotone fa-image"></i></div>

            <div class="ann-body">
              <h4 class="ann-title">{{ a.title }}</h4>
              <p class="text-sm text-dim ann-meta">
                <i class="fa-duotone fa-folder-open"></i> {{ typeName(a) }}
              </p>
              <p v-if="a.start_at" class="text-sm ann-meta">
                <i class="fa-duotone fa-calendar-days"></i>
                {{ inZone(a.start_at, a.timezone) }}
                <span v-if="a.end_at">– {{ inZone(a.end_at, a.timezone) }}</span>
                <span v-if="a.timezone" class="text-dim">({{ a.timezone }})</span>
              </p>
              <p class="text-sm">
                <span v-if="a.schedule_kind" class="ann-badge ann-badge-sched">
                  {{ scheduleLabel(a) }}
                  <template v-if="a.next_post_at">
                    · next {{ inZone(a.next_post_at, a.timezone) }}
                  </template>
                </span>
                <span v-else class="ann-badge ann-badge-manual">Manual only</span>
                <span v-if="a.skip_next" class="ann-badge ann-badge-skip">⏭ next skipped</span>
              </p>
            </div>

            <div class="ann-actions">
              <button
                class="btn-primary btn-sm"
                :disabled="store.sendingId === a.id"
                aria-label="Send now"
                title="Send now"
                @click="store.sendNow(a)"
              >
                <LoadingSpinner v-if="store.sendingId === a.id" label="Sending…" />
                <template v-else><i class="fa-solid fa-paper-plane"></i></template>
              </button>
              <button
                v-if="a.schedule_kind && a.next_post_at"
                class="btn-secondary btn-sm"
                :disabled="store.skippingId === a.id || a.skip_next"
                aria-label="Skip next occurrence"
                title="Skip next occurrence"
                @click="store.skipNext(a)"
              >
                <i class="fa-solid fa-forward-step"></i>
              </button>
              <button class="btn-secondary btn-sm" aria-label="Edit" @click="openEdit(a)">
                <i class="fa-solid fa-pen-to-square"></i>
              </button>
              <button class="btn-danger btn-sm" aria-label="Delete" @click="store.deleteAnnouncement(a)">
                <i class="fa-solid fa-trash"></i>
              </button>
            </div>
          </div>
        </div>
        <p v-else-if="store.search" class="no-game-msg">No announcements match your search.</p>
        <p v-else class="no-game-msg">No announcements yet. Create one with “New Announcement”.</p>
      </template>
    </div>

    <!-- ── Announcement form ──────────────────────────────────────────────── -->
    <div v-else-if="screen === 'form'" class="admin-panel">
      <div class="flex-between mb-16" style="flex-wrap: wrap; gap: 8px">
        <h3>
          <i class="fa-duotone fa-megaphone"></i>
          {{ store.form.id ? 'Edit Announcement' : 'New Announcement' }}
        </h3>
        <button class="btn-ghost btn-sm" @click="backToList()">← Back</button>
      </div>

      <div class="flex-row mb-10">
        <div class="field" style="flex: 2; min-width: 200px">
          <label class="field-label">Title *</label>
          <input
            v-model="store.form.title"
            class="field-input-full"
            placeholder="e.g. Saturday Tea Social"
            aria-label="Announcement title"
          />
        </div>
        <div class="field" style="flex: 1; min-width: 160px">
          <label class="field-label">Type *</label>
          <select v-model.number="store.form.type_id" class="field-input-full" aria-label="Announcement type">
            <option :value="0" disabled>Select a type…</option>
            <option v-for="t in store.types" :key="t.id" :value="t.id">{{ t.name }}</option>
          </select>
        </div>
      </div>

      <div class="field mb-10">
        <label class="field-label">Timezone *</label>
        <select v-model="store.form.timezone" class="field-input-full" aria-label="Timezone">
          <option v-for="tz in timezones" :key="tz" :value="tz">{{ tz }}</option>
        </select>
        <small class="text-dim">
          Anchors every time below (event window + schedule); times stay put across DST.
        </small>
      </div>

      <div class="flex-row mb-10">
        <div class="field" style="flex: 1; min-width: 180px">
          <label class="field-label">Start (optional)</label>
          <input
            v-model="store.form.start_local"
            type="datetime-local"
            class="field-input-full"
            aria-label="Start date and time"
          />
        </div>
        <div class="field" style="flex: 1; min-width: 180px">
          <label class="field-label">End (optional)</label>
          <input
            v-model="store.form.end_local"
            type="datetime-local"
            class="field-input-full"
            aria-label="End date and time"
          />
        </div>
      </div>

      <div class="field mb-12">
        <label class="field-label">Details *</label>
        <MarkdownEditor
          v-model="store.form.details"
          min-height="120px"
          placeholder="The announcement body (supports markdown — bold, italics, lists, links…)"
        />
      </div>

      <!-- Image: upload or reuse an existing one -->
      <div class="field mb-12">
        <label class="field-label">Image (optional)</label>
        <div class="flex-row" style="align-items: flex-start">
          <div style="flex: 0 0 150px">
            <img
              v-if="store.form.image"
              :src="store.form.image"
              class="ann-img-preview"
              alt="Announcement image preview"
            />
            <div v-else class="ann-img-preview ann-img-empty"><i class="fa-duotone fa-image"></i></div>
            <input
              type="file"
              accept="image/*"
              aria-label="Upload announcement image"
              :disabled="store.uploading"
              @change="store.uploadImage($event)"
            />
            <span v-if="store.uploading" class="text-dim text-sm">Uploading…</span>
            <button v-if="store.form.image" class="btn-ghost btn-sm mt-8" @click="store.form.image = ''">
              Remove
            </button>
          </div>
          <div style="flex: 1; min-width: 160px">
            <label class="field-label">Or reuse an uploaded image</label>
            <div v-if="store.images.length" class="ann-img-picker">
              <button
                v-for="img in store.images"
                :key="img"
                type="button"
                class="ann-img-thumb"
                :class="{ active: store.form.image === img }"
                aria-label="Use this image"
                @click="store.pickImage(img)"
              >
                <img :src="img" alt="" />
              </button>
            </div>
            <p v-else class="text-dim text-sm">No images uploaded yet.</p>
          </div>
        </div>
      </div>

      <!-- Schedule builder -->
      <div class="field mb-10">
        <label class="field-label">Schedule</label>
        <select v-model="store.form.schedule_kind" class="field-input-full" aria-label="Schedule kind">
          <option value="">Not scheduled (send manually)</option>
          <option value="once">One-time</option>
          <option value="daily">Daily</option>
          <option value="weekly">Weekly</option>
          <option value="monthly">Monthly</option>
        </select>
      </div>

      <div v-if="store.form.schedule_kind === 'once'" class="field mb-12">
        <label class="field-label">Post at *</label>
        <input
          v-model="store.form.once_local"
          type="datetime-local"
          class="field-input-full"
          aria-label="One-time post date and time"
        />
      </div>

      <template v-else-if="isRecurring">
        <div v-if="store.form.schedule_kind === 'weekly'" class="field mb-10">
          <label class="field-label">On these days *</label>
          <div class="ann-weekdays">
            <button
              v-for="(label, day) in WEEKDAYS"
              :key="day"
              type="button"
              class="ann-weekday"
              :class="{ active: store.form.weekdays.includes(day) }"
              @click="toggleWeekday(day)"
            >
              {{ label }}
            </button>
          </div>
        </div>

        <div v-if="store.form.schedule_kind === 'monthly'" class="flex-row mb-10">
          <div class="field" style="flex: 0 0 120px">
            <label class="field-label">Week *</label>
            <select v-model.number="store.form.week_of_month" class="field-input-full" aria-label="Week of month">
              <option v-for="w in WEEK_OF_MONTH" :key="w.value" :value="w.value">{{ w.label }}</option>
            </select>
          </div>
          <div class="field" style="flex: 0 0 140px">
            <label class="field-label">Weekday *</label>
            <select
              :value="store.form.weekdays[0] ?? ''"
              class="field-input-full"
              aria-label="Weekday"
              @change="store.form.weekdays = [Number(($event.target as HTMLSelectElement).value)]"
            >
              <option value="" disabled>Pick…</option>
              <option v-for="(label, day) in WEEKDAYS" :key="day" :value="day">{{ label }}</option>
            </select>
          </div>
        </div>

        <div class="field mb-12">
          <label class="field-label">Time *</label>
          <input
            v-model="store.form.time_local"
            type="time"
            class="field-input-full"
            style="max-width: 160px"
            aria-label="Recurring post time"
          />
          <small class="text-dim">In the timezone selected above.</small>
        </div>
      </template>

      <div class="btns flex-toolbar">
        <button class="btn-ghost" @click="backToList()">Cancel</button>
        <button class="btn-primary" :disabled="store.saving || !store.form.title.trim()" @click="submit()">
          <LoadingSpinner v-if="store.saving" label="Saving…" />
          <template v-else>{{ store.form.id ? 'Save Changes' : 'Create Announcement' }}</template>
        </button>
      </div>
    </div>

    <!-- ── Types list ─────────────────────────────────────────────────────── -->
    <div v-else-if="screen === 'types'" class="admin-panel">
      <div class="flex-between mb-16" style="flex-wrap: wrap; gap: 8px">
        <h3><i class="fa-duotone fa-folder-open"></i> Announcement Types</h3>
        <div class="flex-toolbar">
          <button class="btn-ghost btn-sm" @click="screen = 'list'">← Back</button>
          <button class="btn-primary btn-sm" @click="openNewType()">
            <i class="fa-solid fa-plus"></i> New Type
          </button>
        </div>
      </div>

      <div v-if="store.types.length" class="ann-list">
        <div v-for="t in store.types" :key="t.id" class="ann-type-card">
          <div class="ann-body">
            <h4 class="ann-title">{{ t.name }}</h4>
            <p class="text-sm text-dim">
              <i class="fa-brands fa-discord"></i> {{ maskWebhook(t.webhook_url) }}
            </p>
          </div>
          <div class="ann-actions">
            <button class="btn-secondary btn-sm" aria-label="Edit type" @click="openEditType(t)">
              <i class="fa-solid fa-pen-to-square"></i>
            </button>
            <button class="btn-danger btn-sm" aria-label="Delete type" @click="store.deleteType(t)">
              <i class="fa-solid fa-trash"></i>
            </button>
          </div>
        </div>
      </div>
      <p v-else class="no-game-msg">No types yet. Add one with “New Type”.</p>
    </div>

    <!-- ── Type form ──────────────────────────────────────────────────────── -->
    <div v-else class="admin-panel">
      <div class="flex-between mb-16" style="flex-wrap: wrap; gap: 8px">
        <h3>
          <i class="fa-duotone fa-folder-open"></i>
          {{ store.typeForm.id ? 'Edit Type' : 'New Type' }}
        </h3>
        <button class="btn-ghost btn-sm" @click="screen = 'types'">← Back</button>
      </div>

      <div class="field mb-10">
        <label class="field-label">Name *</label>
        <input
          v-model="store.typeForm.name"
          class="field-input-full"
          placeholder="e.g. Events Channel"
          aria-label="Type name"
        />
      </div>
      <div class="field mb-12">
        <label class="field-label">Discord webhook URL</label>
        <input
          v-model="store.typeForm.webhook_url"
          class="field-input-full"
          placeholder="https://discord.com/api/webhooks/…"
          aria-label="Discord webhook URL"
        />
        <small class="text-dim">Announcements of this type post to this channel webhook.</small>
      </div>
      <div class="btns flex-toolbar">
        <button class="btn-ghost" @click="screen = 'types'">Cancel</button>
        <button
          class="btn-primary"
          :disabled="store.savingType || !store.typeForm.name.trim()"
          @click="submitType()"
        >
          <LoadingSpinner v-if="store.savingType" label="Saving…" />
          <template v-else>{{ store.typeForm.id ? 'Save Changes' : 'Add Type' }}</template>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.link-btn {
  background: none;
  border: none;
  color: var(--primary);
  cursor: pointer;
  text-decoration: underline;
  padding: 0;
  font: inherit;
}
.ann-search {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
}
.ann-search i {
  color: var(--text-dim);
}
.ann-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.ann-card,
.ann-type-card {
  display: flex;
  gap: 12px;
  background: var(--surface2);
  border-radius: var(--radius);
  padding: 12px;
  align-items: flex-start;
}
.ann-cover {
  width: 120px;
  height: 68px;
  object-fit: cover;
  border-radius: 6px;
  flex: 0 0 auto;
}
.ann-img-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface);
  color: var(--text-dim, #999);
}
.ann-body {
  flex: 1;
  min-width: 0;
}
.ann-body p {
  margin: 0 0 4px;
}
.ann-title {
  margin: 0 0 4px;
}
.ann-meta {
  margin: 0 0 4px;
}
.ann-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.ann-img-preview {
  width: 150px;
  height: 85px;
  object-fit: cover;
  border-radius: 6px;
  display: block;
  margin-bottom: 8px;
}
.ann-img-picker {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  max-height: 180px;
  overflow-y: auto;
}
.ann-img-thumb {
  width: 72px;
  height: 48px;
  padding: 0;
  border: 2px solid var(--surface2);
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  background: var(--surface);
}
.ann-img-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.ann-img-thumb:hover,
.ann-img-thumb.active {
  border-color: var(--primary);
}
.ann-weekdays {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.ann-weekday {
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--surface2);
  border-radius: var(--radius);
  padding: 6px 12px;
  font-weight: 600;
  cursor: pointer;
}
.ann-weekday:hover {
  border-color: var(--primary);
}
.ann-weekday.active {
  background: var(--primary);
  color: var(--text-on-primary);
  border-color: var(--primary);
}
.ann-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 0.78rem;
  font-weight: 600;
  margin-right: 6px;
}
.ann-badge-sched {
  background: rgba(229, 49, 112, 0.18);
  color: var(--text);
}
.ann-badge-manual {
  background: var(--surface);
  color: var(--text-dim);
  border: 1px solid var(--surface2);
}
.ann-badge-skip {
  background: rgba(255, 193, 7, 0.22);
  color: var(--text);
}
</style>
