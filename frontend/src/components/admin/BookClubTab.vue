<script setup lang="ts">
/**
 * Admin Book Club tab — manage reading lists and their items, then publish a
 * list to Discord. Master/detail in one tab (like the Open Raffles tab): the
 * list of reading lists, and the selected list's items + add/edit form.
 *
 * Generic across all book clubs: the active club (set by the router via
 * `bookclub.openClub`) supplies the club name, icon, and the curator-comments
 * field label (e.g. "Yao's Comments" for Yaoi, "Drani's Comments" for Yuri).
 *
 * Items can be filled manually or pulled from AniList (search → pick a result →
 * the form is populated, cover image URL included). "Publish" posts every item
 * in the list as its own Discord embed via the club's configured webhook.
 */
import { ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import { useBookclubStore } from '@/stores/bookclub'
import { assetUrl } from '@/lib/assets'
import { MEETING_LENGTH_OPTIONS, supportedTimezones } from '@/lib/constants'
import type { ReadingList } from '@/types/api'

const bookclub = useBookclubStore()

// Inline rename state for a reading list (themed, avoids native prompt()).
const editingListId = ref<number | null>(null)
const editingTitle = ref('')

function startRename(list: ReadingList): void {
  editingListId.value = list.id
  editingTitle.value = list.title
}
async function commitRename(list: ReadingList): Promise<void> {
  await bookclub.renameList(list, editingTitle.value)
  editingListId.value = null
}
function cancelRename(): void {
  editingListId.value = null
}

// ── Event posts ──────────────────────────────────────────────────────────────
const timezones = supportedTimezones()
const lengthOptions = MEETING_LENGTH_OPTIONS

/** Format an absolute instant (unix seconds) in a given IANA timezone. */
function formatInZone(unix: number, tz: string): string {
  if (!unix) return '—'
  try {
    return new Date(unix * 1000).toLocaleString(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: tz,
    })
  } catch {
    return new Date(unix * 1000).toLocaleString()
  }
}
</script>

<template>
  <div class="tab-body">
    <!-- Sub-view toggle: Reading Lists / Event Posts -->
    <div class="bc-viewbar mb-16">
      <button
        class="bc-tab"
        :class="{ active: bookclub.view === 'lists' }"
        @click="bookclub.setView('lists')"
      >
        <i class="fa-solid fa-book"></i> Reading Lists
      </button>
      <button
        class="bc-tab"
        :class="{ active: bookclub.view === 'events' }"
        @click="bookclub.setView('events')"
      >
        <i class="fa-solid fa-calendar-days"></i> Event Posts
      </button>
    </div>

    <!-- Reading lists view -->
    <template v-if="bookclub.view === 'lists'">
    <!-- Reading list detail (items + add/edit form) -->
    <div v-if="bookclub.selectedList" class="admin-panel">
      <div class="flex-between mb-16" style="flex-wrap: wrap; gap: 8px">
        <h3><i class="fa-solid" :class="bookclub.clubIcon"></i> {{ bookclub.selectedList.title }}</h3>
        <div class="flex-toolbar">
          <button class="btn-ghost btn-sm" @click="bookclub.closeList()">← Back</button>
          <button
            class="btn-primary btn-sm"
            :disabled="bookclub.publishing || !bookclub.selectedList.items?.length"
            @click="bookclub.publishList(bookclub.selectedList)"
          >
            <LoadingSpinner v-if="bookclub.publishing" label="Publishing…" />
            <template v-else><i class="fa-solid fa-paper-plane"></i> Publish to Discord</template>
          </button>
        </div>
      </div>

      <LoadingSpinner v-if="bookclub.detailLoading" block label="Loading items…" />

      <!-- Items -->
      <template v-else>
        <div v-if="bookclub.selectedList.items?.length" class="bc-items">
          <div v-for="item in bookclub.selectedList.items" :key="item.id" class="bc-item-card">
            <img
              v-if="item.cover_image"
              :src="assetUrl(item.cover_image)"
              class="bc-item-cover"
              alt="Cover"
            />
            <div v-else class="bc-item-cover bc-item-cover-empty"><i class="fa-solid fa-image"></i></div>
            <div class="bc-item-body">
              <h4 class="bc-item-title">{{ item.title }}</h4>
              <p class="text-dim text-sm bc-item-meta">
                <span v-if="item.format">{{ item.format }}</span>
                <span v-if="item.chapters">· {{ item.chapters }} ch</span>
              </p>
              <p v-if="item.genres" class="text-dim text-sm">{{ item.genres }}</p>
              <p v-if="item.tropes" class="text-dim text-sm">Tropes: {{ item.tropes }}</p>
              <p v-if="item.sources.length" class="text-sm bc-item-sources">
                <a
                  v-for="(src, i) in item.sources"
                  :key="i"
                  :href="src.url"
                  target="_blank"
                  rel="noopener"
                  class="bc-source-link"
                >
                  <i class="fa-solid fa-link"></i> {{ src.title || 'Source' }}
                </a>
              </p>
            </div>
            <div class="bc-item-actions">
              <button class="btn-secondary btn-sm" @click="bookclub.editItem(item)">
                <i class="fa-solid fa-pen-to-square"></i>
              </button>
              <button class="btn-danger btn-sm" @click="bookclub.deleteItem(item)">
                <i class="fa-solid fa-trash"></i>
              </button>
            </div>
          </div>
        </div>
        <p v-else class="msg-block">No items yet — add one below.</p>

        <!-- Add / edit item form -->
        <div class="bc-form mt-16">
          <h3 class="raffle-section-heading">
            <i class="fa-solid fa-plus"></i>
            {{ bookclub.itemForm.id ? 'Edit Item' : 'Add Item' }}
          </h3>

          <!-- AniList lookup -->
          <div class="bc-lookup mb-12">
            <label class="field-label">Pull from AniList</label>
            <div class="flex-toolbar">
              <input
                v-model="bookclub.lookupQuery"
                class="field-input-full"
                placeholder="Search title…"
                aria-label="AniList search"
                @keyup.enter="bookclub.runLookup()"
              />
              <button
                class="btn-secondary btn-sm"
                :disabled="bookclub.looking || !bookclub.lookupQuery.trim()"
                @click="bookclub.runLookup()"
              >
                <LoadingSpinner v-if="bookclub.looking" label="Searching…" />
                <template v-else><i class="fa-solid fa-magnifying-glass"></i> Search</template>
              </button>
            </div>
            <div v-if="bookclub.lookupResults.length" class="bc-results mt-8">
              <button
                v-for="(res, i) in bookclub.lookupResults"
                :key="i"
                class="bc-result"
                @click="bookclub.applyLookupResult(res)"
              >
                <img v-if="res.cover_image" :src="res.cover_image" class="bc-result-cover" alt="" />
                <span class="bc-result-info">
                  <strong>{{ res.title }}</strong>
                  <small class="text-dim">{{ res.format }} · {{ res.chapters }} ch</small>
                </span>
              </button>
            </div>
          </div>

          <div class="flex-row mb-10" style="align-items: flex-start">
            <div class="field" style="flex: 0 0 120px">
              <label class="field-label">Cover</label>
              <img
                v-if="bookclub.itemForm.cover_image"
                :src="assetUrl(bookclub.itemForm.cover_image)"
                class="bc-cover-preview"
                alt="Cover preview"
              />
              <input
                type="file"
                accept="image/*"
                aria-label="Cover image"
                :disabled="bookclub.coverUploading"
                @change="bookclub.uploadCover($event)"
              />
              <span v-if="bookclub.coverUploading" class="text-dim text-sm">Uploading…</span>
              <button
                v-if="bookclub.itemForm.cover_image"
                class="btn-ghost btn-sm mt-8"
                @click="bookclub.itemForm.cover_image = ''"
              >
                Remove
              </button>
            </div>
            <div class="field" style="flex: 1; min-width: 200px">
              <label class="field-label">Title *</label>
              <input
                v-model="bookclub.itemForm.title"
                class="field-input-full"
                placeholder="Title"
                aria-label="Item title"
              />
              <label class="field-label mt-8">Cover Image URL</label>
              <input
                v-model="bookclub.itemForm.cover_image"
                class="field-input-full"
                placeholder="https://…"
                aria-label="Cover image URL"
              />
            </div>
          </div>

          <div class="field mb-10">
            <label class="field-label">Summary</label>
            <MarkdownEditor
              v-model="bookclub.itemForm.summary"
              placeholder="Summary (supports markdown — bold, italics, lists, links…)"
            />
          </div>

          <div class="flex-row mb-10">
            <div class="field" style="flex: 1; min-width: 140px">
              <label class="field-label">Format</label>
              <input
                v-model="bookclub.itemForm.format"
                class="field-input-full"
                placeholder="Manga, Manhwa, Danmei…"
                aria-label="Format"
              />
            </div>
            <div class="field" style="flex: 0 0 120px; min-width: 100px">
              <label class="field-label">Chapters</label>
              <input
                v-model="bookclub.itemForm.chapters"
                class="field-input-full"
                placeholder="e.g. 156"
                aria-label="Chapters"
              />
            </div>
          </div>

          <div class="field mb-10">
            <label class="field-label">Genres</label>
            <input
              v-model="bookclub.itemForm.genres"
              class="field-input-full"
              placeholder="Comma-separated, e.g. Romance, Fantasy"
              aria-label="Genres"
            />
          </div>

          <div class="field mb-10">
            <label class="field-label">Tropes</label>
            <input
              v-model="bookclub.itemForm.tropes"
              class="field-input-full"
              placeholder="Comma-separated, e.g. Enemies to Lovers, Slow Burn"
              aria-label="Tropes"
            />
          </div>

          <div class="field mb-10">
            <label class="field-label">{{ bookclub.commentsLabel }}</label>
            <MarkdownEditor
              v-model="bookclub.itemForm.comments"
              min-height="120px"
              :placeholder="bookclub.commentsLabel + ' (supports markdown)'"
            />
          </div>

          <!-- Sources repeater -->
          <div class="field mb-12">
            <label class="field-label">Sources</label>
            <div v-for="(src, i) in bookclub.itemForm.sources" :key="i" class="flex-toolbar mb-8">
              <input
                v-model="src.title"
                class="field-input-full"
                style="flex: 1"
                placeholder="Source title"
                aria-label="Source title"
              />
              <input
                v-model="src.url"
                class="field-input-full"
                style="flex: 2"
                placeholder="https://…"
                aria-label="Source URL"
              />
              <button class="btn-danger btn-sm" @click="bookclub.removeSourceRow(i)">&times;</button>
            </div>
            <button class="btn-ghost btn-sm" @click="bookclub.addSourceRow()">
              <i class="fa-solid fa-plus"></i> Add Source
            </button>
          </div>

          <div class="btns flex-toolbar">
            <button v-if="bookclub.itemForm.id" class="btn-ghost" @click="bookclub.resetItemForm()">
              Cancel Edit
            </button>
            <button
              class="btn-primary"
              :disabled="bookclub.savingItem || !bookclub.itemForm.title.trim()"
              @click="bookclub.saveItem()"
            >
              <LoadingSpinner v-if="bookclub.savingItem" label="Saving…" />
              <template v-else>{{ bookclub.itemForm.id ? 'Save Changes' : 'Add Item' }}</template>
            </button>
          </div>
        </div>
      </template>
    </div>

    <!-- Reading lists overview -->
    <div v-else class="admin-panel">
      <h3 class="mb-16"><i class="fa-solid fa-book"></i> {{ bookclub.clubName }} — Reading Lists</h3>

      <!-- Create list -->
      <div class="flex-toolbar mb-16">
        <input
          v-model="bookclub.newListTitle"
          class="field-input-full"
          placeholder="New reading list title…"
          aria-label="New reading list title"
          @keyup.enter="bookclub.createList()"
        />
        <button
          class="btn-primary btn-sm"
          :disabled="bookclub.creatingList || !bookclub.newListTitle.trim()"
          @click="bookclub.createList()"
        >
          <LoadingSpinner v-if="bookclub.creatingList" label="Creating…" />
          <template v-else><i class="fa-solid fa-plus"></i> Create List</template>
        </button>
      </div>

      <LoadingSpinner
        v-if="bookclub.listsLoading && bookclub.lists.length === 0"
        block
        label="Loading reading lists…"
      />
      <template v-else>
        <div v-if="bookclub.lists.length" class="bc-list">
          <div v-for="list in bookclub.lists" :key="list.id" class="bc-list-card">
            <template v-if="editingListId === list.id">
              <input
                v-model="editingTitle"
                class="field-input-full"
                aria-label="Rename reading list"
                @keyup.enter="commitRename(list)"
                @keyup.esc="cancelRename()"
              />
              <div class="flex-toolbar">
                <button class="btn-primary btn-sm" @click="commitRename(list)">Save</button>
                <button class="btn-ghost btn-sm" @click="cancelRename()">Cancel</button>
              </div>
            </template>
            <template v-else>
              <button class="bc-list-title" @click="bookclub.selectList(list)">
                {{ list.title }}
              </button>
              <div class="bc-list-actions flex-toolbar">
                <button class="btn-secondary btn-sm" @click="bookclub.selectList(list)">Open</button>
                <button class="btn-ghost btn-sm" @click="startRename(list)">
                  <i class="fa-solid fa-pen-to-square"></i>
                </button>
                <button
                  class="btn-primary btn-sm"
                  :disabled="bookclub.publishing"
                  @click="bookclub.publishList(list)"
                >
                  <i class="fa-solid fa-paper-plane"></i>
                </button>
                <button class="btn-danger btn-sm" @click="bookclub.deleteList(list)">
                  <i class="fa-solid fa-trash"></i>
                </button>
              </div>
            </template>
          </div>
        </div>
        <p v-else class="no-game-msg">No reading lists yet. Create one above.</p>
      </template>
    </div>
    </template>

    <!-- Event posts view -->
    <template v-else>
      <div class="admin-panel">
        <h3 class="mb-16">
          <i class="fa-solid fa-calendar-days"></i> {{ bookclub.clubName }} — Event Posts
        </h3>

        <!-- Add / edit event form -->
        <div class="bc-form mb-16">
          <h3 class="raffle-section-heading">
            <i class="fa-solid fa-plus"></i>
            {{ bookclub.eventForm.id ? 'Edit Event' : 'Schedule Event' }}
          </h3>

          <div class="field mb-10">
            <label class="field-label">Title *</label>
            <input
              v-model="bookclub.eventForm.title"
              class="field-input-full"
              placeholder="e.g. July 2026 Meeting"
              aria-label="Event title"
            />
          </div>

          <div class="flex-row mb-10">
            <div class="field" style="flex: 1; min-width: 180px">
              <label class="field-label">Start date &amp; time *</label>
              <input
                v-model="bookclub.eventForm.start_local"
                type="datetime-local"
                class="field-input-full"
                aria-label="Start date and time"
              />
            </div>
            <div class="field" style="flex: 1; min-width: 180px">
              <label class="field-label">Timezone *</label>
              <select
                v-model="bookclub.eventForm.timezone"
                class="field-input-full"
                aria-label="Timezone"
              >
                <option v-for="tz in timezones" :key="tz" :value="tz">{{ tz }}</option>
              </select>
            </div>
          </div>

          <div class="flex-row mb-10">
            <div class="field" style="flex: 0 0 160px; min-width: 140px">
              <label class="field-label">Meeting length</label>
              <select
                v-model.number="bookclub.eventForm.length_hours"
                class="field-input-full"
                aria-label="Meeting length"
              >
                <option v-for="h in lengthOptions" :key="h" :value="h">
                  {{ h }} hour{{ h > 1 ? 's' : '' }}
                </option>
              </select>
            </div>
            <div class="field" style="flex: 1; min-width: 180px">
              <label class="field-label">Location</label>
              <input
                v-model="bookclub.eventForm.location"
                class="field-input-full"
                placeholder="e.g. Discord — Voice Channel 1"
                aria-label="Location"
              />
            </div>
          </div>

          <div class="field mb-10">
            <label class="field-label">When to post *</label>
            <input
              v-model="bookclub.eventForm.post_at_local"
              type="datetime-local"
              class="field-input-full"
              aria-label="When to post"
            />
            <small class="text-dim">
              The embed posts automatically at this time (interpreted in the timezone above).
            </small>
          </div>

          <!-- Image: upload or reuse an existing one -->
          <div class="field mb-12">
            <label class="field-label">Image</label>
            <div class="flex-row" style="align-items: flex-start">
              <div style="flex: 0 0 150px">
                <img
                  v-if="bookclub.eventForm.image"
                  :src="bookclub.eventForm.image"
                  class="bc-event-img-preview"
                  alt="Event image preview"
                />
                <div v-else class="bc-event-img-preview bc-item-cover-empty">
                  <i class="fa-solid fa-image"></i>
                </div>
                <input
                  type="file"
                  accept="image/*"
                  aria-label="Upload event image"
                  :disabled="bookclub.eventImageUploading"
                  @change="bookclub.uploadEventImage($event)"
                />
                <span v-if="bookclub.eventImageUploading" class="text-dim text-sm">Uploading…</span>
                <button
                  v-if="bookclub.eventForm.image"
                  class="btn-ghost btn-sm mt-8"
                  @click="bookclub.eventForm.image = ''"
                >
                  Remove
                </button>
              </div>
              <div style="flex: 1; min-width: 160px">
                <label class="field-label">Or reuse an uploaded image</label>
                <div v-if="bookclub.eventImages.length" class="bc-img-picker">
                  <button
                    v-for="img in bookclub.eventImages"
                    :key="img"
                    type="button"
                    class="bc-img-thumb"
                    :class="{ active: bookclub.eventForm.image === img }"
                    :aria-label="'Use this image'"
                    @click="bookclub.pickEventImage(img)"
                  >
                    <img :src="img" alt="" />
                  </button>
                </div>
                <p v-else class="text-dim text-sm">No images uploaded yet.</p>
              </div>
            </div>
          </div>

          <div class="btns flex-toolbar">
            <button
              v-if="bookclub.eventForm.id"
              class="btn-ghost"
              @click="bookclub.resetEventForm()"
            >
              Cancel Edit
            </button>
            <button
              class="btn-primary"
              :disabled="bookclub.savingEvent || !bookclub.eventForm.title.trim()"
              @click="bookclub.saveEvent()"
            >
              <LoadingSpinner v-if="bookclub.savingEvent" label="Saving…" />
              <template v-else>{{ bookclub.eventForm.id ? 'Save Changes' : 'Schedule Event' }}</template>
            </button>
          </div>
        </div>

        <!-- Scheduled events -->
        <LoadingSpinner
          v-if="bookclub.eventsLoading && bookclub.events.length === 0"
          block
          label="Loading events…"
        />
        <template v-else>
          <div v-if="bookclub.events.length" class="bc-events">
            <div v-for="ev in bookclub.events" :key="ev.id" class="bc-event-card">
              <img v-if="ev.image" :src="ev.image" class="bc-event-cover" alt="Event image" />
              <div v-else class="bc-event-cover bc-item-cover-empty">
                <i class="fa-solid fa-image"></i>
              </div>
              <div class="bc-event-body">
                <h4 class="bc-item-title">{{ ev.title }}</h4>
                <p class="text-sm bc-item-meta">
                  <i class="fa-solid fa-calendar-days"></i>
                  {{ formatInZone(ev.start_at_unix, ev.timezone) }}
                  <span class="text-dim">({{ ev.timezone }})</span>
                </p>
                <p class="text-dim text-sm">
                  <i class="fa-solid fa-clock"></i> {{ ev.length_hours }} hour{{ ev.length_hours > 1 ? 's' : '' }}
                  <span v-if="ev.location">
                    · <i class="fa-solid fa-location-dot"></i> {{ ev.location }}
                  </span>
                </p>
                <p class="text-sm">
                  <span v-if="ev.posted" class="bc-badge bc-badge-posted">Posted</span>
                  <span v-else class="bc-badge bc-badge-scheduled">
                    Posts {{ formatInZone(ev.post_at_unix, ev.timezone) }}
                  </span>
                </p>
              </div>
              <div class="bc-item-actions">
                <button class="btn-secondary btn-sm" aria-label="Edit event" @click="bookclub.editEvent(ev)">
                  <i class="fa-solid fa-pen-to-square"></i>
                </button>
                <button
                  class="btn-primary btn-sm"
                  :disabled="bookclub.postingEventId === ev.id"
                  aria-label="Post event now"
                  @click="bookclub.postEventNow(ev)"
                >
                  <LoadingSpinner v-if="bookclub.postingEventId === ev.id" label="Posting…" />
                  <template v-else><i class="fa-solid fa-paper-plane"></i></template>
                </button>
                <button class="btn-danger btn-sm" aria-label="Delete event" @click="bookclub.deleteEvent(ev)">
                  <i class="fa-solid fa-trash"></i>
                </button>
              </div>
            </div>
          </div>
          <p v-else class="no-game-msg">No events scheduled yet. Add one above.</p>
        </template>
      </div>
    </template>
  </div>
</template>

<style scoped>
.bc-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.bc-list-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  background: var(--surface2);
  border-radius: var(--radius);
  padding: 10px 14px;
  flex-wrap: wrap;
}
.bc-list-title {
  background: none;
  border: none;
  color: inherit;
  font-size: 1.05rem;
  font-weight: 700;
  cursor: pointer;
  text-align: left;
  flex: 1;
  min-width: 140px;
}
.bc-list-title:hover {
  text-decoration: underline;
}
.bc-items {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.bc-item-card {
  display: flex;
  gap: 12px;
  background: var(--surface2);
  border-radius: var(--radius);
  padding: 12px;
}
.bc-item-cover {
  width: 64px;
  height: 90px;
  object-fit: cover;
  border-radius: 6px;
  flex: 0 0 auto;
}
.bc-item-cover-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--surface);
  color: var(--text-dim, #999);
}
.bc-item-body {
  flex: 1;
  min-width: 0;
}
.bc-item-title {
  margin: 0 0 4px;
}
.bc-item-meta {
  margin: 0 0 4px;
}
.bc-item-sources {
  margin: 6px 0 0;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.bc-source-link {
  white-space: nowrap;
}
.bc-item-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.bc-form {
  background: var(--surface2);
  border-radius: var(--radius);
  padding: 14px 16px;
}
.bc-results {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 280px;
  overflow-y: auto;
}
.bc-result {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--surface2);
  border-radius: 6px;
  padding: 6px 8px;
  cursor: pointer;
  text-align: left;
}
.bc-result:hover {
  border-color: var(--primary);
}
.bc-result-cover {
  width: 36px;
  height: 50px;
  object-fit: cover;
  border-radius: 4px;
}
.bc-result-info {
  display: flex;
  flex-direction: column;
}
.bc-cover-preview {
  width: 100px;
  height: 140px;
  object-fit: cover;
  border-radius: 6px;
  display: block;
  margin-bottom: 8px;
}

/* ── Event posts ─────────────────────────────────────────────────────────── */
.bc-viewbar {
  display: flex;
  gap: 8px;
}
.bc-tab {
  background: var(--surface2);
  color: var(--text);
  border: 1px solid var(--surface2);
  border-radius: var(--radius);
  padding: 8px 16px;
  font-weight: 600;
  cursor: pointer;
}
.bc-tab:hover {
  border-color: var(--primary);
}
.bc-tab.active {
  background: var(--primary);
  color: var(--text-on-primary);
  border-color: var(--primary);
}
.bc-events {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.bc-event-card {
  display: flex;
  gap: 12px;
  background: var(--surface2);
  border-radius: var(--radius);
  padding: 12px;
}
.bc-event-cover {
  width: 120px;
  height: 68px;
  object-fit: cover;
  border-radius: 6px;
  flex: 0 0 auto;
}
.bc-event-body {
  flex: 1;
  min-width: 0;
}
.bc-event-body p {
  margin: 0 0 4px;
}
.bc-event-img-preview {
  width: 150px;
  height: 85px;
  object-fit: cover;
  border-radius: 6px;
  display: block;
  margin-bottom: 8px;
}
.bc-img-picker {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  max-height: 180px;
  overflow-y: auto;
}
.bc-img-thumb {
  width: 72px;
  height: 48px;
  padding: 0;
  border: 2px solid var(--surface2);
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  background: var(--surface);
}
.bc-img-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.bc-img-thumb:hover {
  border-color: var(--primary);
}
.bc-img-thumb.active {
  border-color: var(--primary);
}
.bc-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 0.78rem;
  font-weight: 600;
}
.bc-badge-posted {
  background: rgba(44, 182, 125, 0.25);
  color: var(--text);
}
.bc-badge-scheduled {
  background: var(--surface);
  color: var(--text-dim);
  border: 1px solid var(--surface2);
}
</style>
