<script setup lang="ts">
/**
 * Admin Book Club tab - manage reading lists and their items, then publish a
 * list to Discord. Master/detail in one tab (like the Open Raffles tab): the
 * list of reading lists, and the selected list's items + add/edit form.
 *
 * Generic across all book clubs: the active club (set by the router via
 * `bookclub.openClub`) supplies the club name, icon, and the curator-comments
 * field label (e.g. "Yao's Comments" for Yaoi, "Drani's Comments" for Yuri).
 *
 * Items can be filled manually or pulled from AniList (search -> pick a result ->
 * the form is populated, cover image URL included). "Publish" posts every item
 * in the list as its own Discord embed via the club's configured webhook.
 */
import { ref, useTemplateRef, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import ListRow from '@/components/common/ui/ListRow.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import { useBookclubStore } from '@/stores/bookclub'
import { assetUrl } from '@/lib/assets'
import type { ReadingListSummary } from '@/api/client'

const bookclub = useBookclubStore()

// Inline rename state for a reading list (themed, avoids native prompt()).
const editingListId = ref<number | null>(null)
const editingTitle = ref('')

// The list overview, or the club's Discord webhook form.
const screen = ref<'list' | 'webhook'>('list')
const webhookDraft = ref('')

// Another club's tab starts on its overview.
watch(
  () => bookclub.activeClubSlug,
  () => {
    screen.value = 'list'
  },
)

function openWebhook(): void {
  webhookDraft.value = bookclub.webhookUrl
  screen.value = 'webhook'
}
async function submitWebhook(): Promise<void> {
  if (await bookclub.saveWebhook(webhookDraft.value)) {
    screen.value = 'list'
  }
}

/** Hidden <input type="file"> behind the cover Upload button. A bare file input
 *  renders the browser's own grey "Choose File" control, which cannot be themed
 *  (only ::file-selector-button can, and not consistently) - so every upload in
 *  the app hides the input and triggers it from a real button instead. */
const coverInput = useTemplateRef<HTMLInputElement>('coverInput')

function pickCover(): void {
  coverInput.value?.click()
}

function startRename(list: ReadingListSummary): void {
  editingListId.value = list.id
  editingTitle.value = list.title
}
async function commitRename(list: ReadingListSummary): Promise<void> {
  await bookclub.renameList(list, editingTitle.value)
  editingListId.value = null
}
function cancelRename(): void {
  editingListId.value = null
}
</script>

<template>
  <div class="tab-body">
    <!-- The club's Discord webhook -->
    <AdminPanel v-if="screen === 'webhook'">
      <SubPageHeader
        :icon="['fab', 'discord']"
        :title="`${bookclub.clubName} Discord Webhook`"
        @back="screen = 'list'"
      />
      <p class="text-muted text-sm mb-16">
        Publishing a reading list posts each item as its own embed to this channel webhook. In
        Discord: Channel Settings -> Integrations -> Webhooks -> New Webhook -> Copy Webhook URL.
      </p>
      <FormField
        label="Discord webhook URL"
        help="Leave blank to disable publishing. Only Discord webhook URLs are accepted."
      >
        <input
          v-model="webhookDraft"
          placeholder="https://discord.com/api/webhooks/..."
          aria-label="Discord webhook URL"
        />
      </FormField>
      <FormActions align="start">
        <button
          type="button"
          class="btn-neutral"
          :disabled="bookclub.savingWebhook"
          @click="screen = 'list'"
        >
          Cancel
        </button>
        <button
          type="button"
          class="btn-confirm"
          :disabled="bookclub.savingWebhook"
          @click="submitWebhook"
        >
          <LoadingSpinner v-if="bookclub.savingWebhook" label="Saving..." />
          <template v-else>Save Webhook</template>
        </button>
      </FormActions>
    </AdminPanel>

    <!-- Reading list detail (items + add/edit form) -->
    <AdminPanel v-else-if="bookclub.selectedList">
      <SubPageHeader
        :icon="['fad', bookclub.clubIcon]"
        :title="bookclub.selectedList.title"
        @back="bookclub.closeList()"
      />
      <div class="flex-toolbar flex-end mb-16">
        <button
          type="button"
          class="btn-action btn-sm"
          :disabled="bookclub.publishing || !bookclub.selectedList.items?.length"
          @click="bookclub.publishList(bookclub.selectedList)"
        >
          <LoadingSpinner v-if="bookclub.publishing" label="Publishing..." />
          <template v-else
            ><font-awesome-icon :icon="['fas', 'paper-plane']" /> Publish to Discord</template
          >
        </button>
      </div>

      <LoadingSpinner v-if="bookclub.detailLoading" block label="Loading items..." />

      <!-- Items -->
      <template v-else>
        <div v-if="bookclub.selectedList.items?.length" class="list-rows">
          <ListRow v-for="item in bookclub.selectedList.items" :key="item.id">
            <template #media>
              <img
                v-if="item.coverImage"
                :src="assetUrl(item.coverImage)"
                class="media-cover media-cover--book"
                alt="Cover"
              />
              <div v-else class="media-cover media-cover--book media-empty">
                <font-awesome-icon :icon="['fad', 'image']" />
              </div>
            </template>
            <h4 class="list-row-title">{{ item.title }}</h4>
            <p class="text-muted text-sm list-row-meta">
              <span v-if="item.format">{{ item.format }}</span>
              <span v-if="item.chapters">- {{ item.chapters }} ch</span>
            </p>
            <p v-if="item.genres" class="text-muted text-sm">{{ item.genres }}</p>
            <p v-if="item.tropes" class="text-muted text-sm">Tropes: {{ item.tropes }}</p>
            <p v-if="item.sources.length" class="text-sm book-club-item-sources">
              <a
                v-for="(src, i) in item.sources"
                :key="i"
                :href="src.url"
                target="_blank"
                rel="noopener"
                class="book-club-source-link"
              >
                <font-awesome-icon :icon="['fad', 'link']" /> {{ src.title || 'Source' }}
              </a>
            </p>
            <template #actions>
              <!-- Icon-only buttons in a repeating list: FontAwesome renders the
                   glyph aria-hidden, so without a label these announce as just
                   "button". Naming them per ITEM also distinguishes one row's
                   Edit from the next when tabbing or listing controls. -->
              <button
                type="button"
                class="btn-confirm btn-sm"
                :aria-label="`Edit ${item.title}`"
                :title="`Edit ${item.title}`"
                @click="bookclub.editItem(item)"
              >
                <font-awesome-icon :icon="['fas', 'pen-to-square']" />
              </button>
              <button
                type="button"
                class="btn-danger btn-sm"
                :aria-label="`Delete ${item.title}`"
                :title="`Delete ${item.title}`"
                @click="bookclub.deleteItem(item)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </template>
          </ListRow>
        </div>
        <EmptyState v-else text="No items yet - add one below." />

        <!-- Add / edit item form -->
        <div class="subpanel mt-16">
          <h3 class="section-heading">
            <font-awesome-icon :icon="['fad', 'plus']" />
            {{ bookclub.itemForm.id ? 'Edit Item' : 'Add Item' }}
          </h3>

          <!-- AniList lookup -->
          <FormField label="Pull from AniList">
            <div class="flex-toolbar">
              <input
                v-model="bookclub.lookupQuery"
                class="field-input-full"
                placeholder="Search title..."
                aria-label="AniList search"
                @keyup.enter="bookclub.runLookup()"
              />
              <button
                type="button"
                class="btn-action btn-sm"
                :disabled="bookclub.looking || !bookclub.lookupQuery.trim()"
                @click="bookclub.runLookup()"
              >
                <LoadingSpinner v-if="bookclub.looking" label="Searching..." />
                <template v-else
                  ><font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Search</template
                >
              </button>
            </div>
            <div v-if="bookclub.lookupResults.length" class="book-club-results mt-8">
              <button
                v-for="(res, i) in bookclub.lookupResults"
                :key="i"
                type="button"
                class="book-club-result"
                @click="bookclub.applyLookupResult(res)"
              >
                <img
                  v-if="res.coverImage"
                  :src="res.coverImage"
                  class="media-cover media-cover--book-sm"
                  alt=""
                />
                <span class="book-club-result-info">
                  <strong>{{ res.title }}</strong>
                  <small class="text-muted">{{ res.format }} - {{ res.chapters }} ch</small>
                </span>
              </button>
            </div>
          </FormField>

          <div class="flex-row items-start mb-10">
            <FormField label="Cover" class="flex-fixed-120">
              <img
                v-if="bookclub.itemForm.coverImage"
                :src="assetUrl(bookclub.itemForm.coverImage)"
                class="media-cover media-cover--book-lg"
                alt="Cover preview"
              />
              <button
                type="button"
                class="btn-action btn-sm mt-8"
                :disabled="bookclub.coverUploading"
                @click="pickCover"
              >
                <LoadingSpinner v-if="bookclub.coverUploading" label="Uploading..." />
                <template v-else><font-awesome-icon :icon="['fas', 'upload']" /> Upload</template>
              </button>
              <input
                ref="coverInput"
                type="file"
                accept="image/*"
                aria-label="Cover image"
                hidden
                @change="bookclub.uploadCover($event)"
              />
              <button
                v-if="bookclub.itemForm.coverImage"
                type="button"
                class="btn-neutral btn-sm mt-8"
                @click="bookclub.itemForm.coverImage = ''"
              >
                Remove
              </button>
            </FormField>
            <div class="flex-1 min-w-200">
              <FormField label="Title" required>
                <input
                  v-model="bookclub.itemForm.title"
                  placeholder="Title"
                  aria-label="Item title"
                />
              </FormField>
              <FormField label="Cover Image URL">
                <input
                  v-model="bookclub.itemForm.coverImage"
                  placeholder="https://..."
                  aria-label="Cover image URL"
                />
              </FormField>
            </div>
          </div>

          <FormField label="Summary">
            <MarkdownEditor
              v-model="bookclub.itemForm.summary"
              placeholder="Summary (supports markdown - bold, italics, lists, links...)"
            />
          </FormField>

          <div class="flex-row mb-10">
            <FormField label="Format" class="flex-1 min-w-140">
              <input
                v-model="bookclub.itemForm.format"
                placeholder="Manga, Manhwa, Danmei..."
                aria-label="Format"
              />
            </FormField>
            <FormField label="Chapters" class="flex-fixed-120 min-w-100">
              <input
                v-model="bookclub.itemForm.chapters"
                placeholder="e.g. 156"
                aria-label="Chapters"
              />
            </FormField>
          </div>

          <FormField label="Genres">
            <input
              v-model="bookclub.itemForm.genres"
              placeholder="Comma-separated, e.g. Romance, Fantasy"
              aria-label="Genres"
            />
          </FormField>

          <FormField label="Tropes">
            <input
              v-model="bookclub.itemForm.tropes"
              placeholder="Comma-separated, e.g. Enemies to Lovers, Slow Burn"
              aria-label="Tropes"
            />
          </FormField>

          <FormField :label="bookclub.commentsLabel">
            <MarkdownEditor
              v-model="bookclub.itemForm.comments"
              min-height="120px"
              :placeholder="bookclub.commentsLabel + ' (supports markdown)'"
            />
          </FormField>

          <!-- Sources repeater -->
          <FormField label="Sources">
            <div
              v-for="(src, i) in bookclub.itemForm.sources"
              :key="src._uid"
              class="flex-toolbar mb-8"
            >
              <input
                v-model="src.title"
                class="field-input-full flex-1"
                placeholder="Source title"
                aria-label="Source title"
              />
              <input
                v-model="src.url"
                class="field-input-full flex-2"
                placeholder="https://..."
                aria-label="Source URL"
              />
              <button type="button" class="btn-danger btn-sm" @click="bookclub.removeSourceRow(i)">
                &times;
              </button>
            </div>
            <button type="button" class="btn-confirm btn-sm" @click="bookclub.addSourceRow()">
              <font-awesome-icon :icon="['fas', 'plus']" /> Add Source
            </button>
          </FormField>

          <FormActions align="start">
            <button
              v-if="bookclub.itemForm.id"
              type="button"
              class="btn-neutral"
              @click="bookclub.resetItemForm()"
            >
              Cancel Edit
            </button>
            <button
              type="button"
              class="btn-confirm"
              :disabled="bookclub.savingItem || !bookclub.itemForm.title.trim()"
              @click="bookclub.saveItem()"
            >
              <LoadingSpinner v-if="bookclub.savingItem" label="Saving..." />
              <template v-else>{{ bookclub.itemForm.id ? 'Save Changes' : 'Add Item' }}</template>
            </button>
          </FormActions>
        </div>
      </template>
    </AdminPanel>

    <!-- Reading lists overview -->
    <ManagerView v-else :title="`${bookclub.clubName} - Reading Lists`" :icon="['fad', 'book']">
      <template #actions>
        <button type="button" class="btn-view btn-sm" @click="openWebhook">
          <font-awesome-icon :icon="['fab', 'discord']" /> Webhook
        </button>
      </template>

      <template #toolbar>
        <input
          v-model="bookclub.newListTitle"
          placeholder="New reading list title..."
          aria-label="New reading list title"
          class="flex-1 min-w-160 max-w-360"
          @keyup.enter="bookclub.createList()"
        />
        <button
          type="button"
          class="btn-confirm btn-sm"
          :disabled="bookclub.creatingList || !bookclub.newListTitle.trim()"
          @click="bookclub.createList()"
        >
          <LoadingSpinner v-if="bookclub.creatingList" label="Creating..." />
          <template v-else><font-awesome-icon :icon="['fas', 'plus']" /> Create List</template>
        </button>
      </template>

      <div v-if="!bookclub.webhookUrl && bookclub.lists.length" class="text-muted text-xs mb-12">
        <font-awesome-icon :icon="['fad', 'triangle-exclamation']" /> No Discord webhook set yet -
        add one with the
        <button type="button" class="link-btn" @click="openWebhook">Webhook</button> button to
        publish reading lists.
      </div>

      <LoadingSpinner
        v-if="bookclub.listsLoading && bookclub.lists.length === 0"
        block
        label="Loading reading lists..."
      />
      <template v-else>
        <div v-if="bookclub.lists.length" class="list-rows">
          <ListRow v-for="list in bookclub.lists" :key="list.id">
            <input
              v-if="editingListId === list.id"
              v-model="editingTitle"
              aria-label="Rename reading list"
              class="w-full max-w-360"
              @keyup.enter="commitRename(list)"
              @keyup.esc="cancelRename()"
            />
            <button
              v-else
              type="button"
              class="book-club-list-title"
              @click="bookclub.selectList(list)"
            >
              {{ list.title }}
            </button>
            <template #actions>
              <template v-if="editingListId === list.id">
                <button type="button" class="btn-confirm btn-sm" @click="commitRename(list)">
                  Save
                </button>
                <button type="button" class="btn-neutral btn-sm" @click="cancelRename()">
                  Cancel
                </button>
              </template>
              <template v-else>
                <button type="button" class="btn-view btn-sm" @click="bookclub.selectList(list)">
                  Open
                </button>
                <button
                  type="button"
                  class="btn-action btn-sm"
                  aria-label="Publish"
                  :disabled="bookclub.publishing"
                  @click="bookclub.publishList(list)"
                >
                  <font-awesome-icon :icon="['fas', 'paper-plane']" />
                </button>
                <button
                  type="button"
                  class="btn-confirm btn-sm"
                  aria-label="Rename"
                  @click="startRename(list)"
                >
                  <font-awesome-icon :icon="['fas', 'pen-to-square']" />
                </button>
                <button
                  type="button"
                  class="btn-danger btn-sm"
                  aria-label="Delete"
                  @click="bookclub.deleteList(list)"
                >
                  <font-awesome-icon :icon="['fas', 'trash']" />
                </button>
              </template>
            </template>
          </ListRow>
        </div>
        <EmptyState v-else text="No reading lists yet. Create one above." />
      </template>
    </ManagerView>
  </div>
</template>

<style scoped>
.book-club-list-title {
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

.book-club-list-title:hover {
  text-decoration: underline;
}

.book-club-item-sources {
  margin: 6px 0 0;
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.book-club-source-link {
  white-space: nowrap;
}

.book-club-results {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 280px;
  overflow-y: auto;
}

.book-club-result {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--panel-bg);
  color: var(--text);
  border: 1px solid var(--panel-raised-bg);
  border-radius: 0;
  padding: 6px 8px;
  cursor: pointer;
  text-align: left;
}

.book-club-result:hover {
  border-color: var(--accent);
}

.book-club-result-info {
  display: flex;
  flex-direction: column;
}
</style>
