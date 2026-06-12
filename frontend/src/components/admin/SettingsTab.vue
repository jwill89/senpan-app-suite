<script setup lang="ts">
/**
 * Admin App Settings tab — app title, default draw delay, frequent-winner
 * thresholds, Google Fonts API key, and the header/board font (with live
 * preview). Mirrors the original `adminTab==='system-settings'` block.
 *
 * The header/board font is chosen from a single combo box grouped into two
 * <optgroup>s: fonts uploaded via System → Font Upload (listed first) and
 * Google Fonts (the live API list when an API key is set, else the shared
 * FALLBACK_GOOGLE_FONTS list). Uploaded fonts' @font-face rules are registered
 * (applyUploadedFonts) so they preview live and can be selected. A previously
 * saved value that is in neither group is preserved as a "(custom)" option so
 * editing settings never silently drops it.
 */
import { computed, onMounted, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import {
  BOOK_CLUBS,
  clubWebhookKey,
  clubEventsWebhookKey,
  FALLBACK_GOOGLE_FONTS,
} from '@/lib/constants'
import { applyUploadedFonts, fontFamilyFromFile } from '@/lib/theme'
import { useAppStore } from '@/stores/app'
import { useFontsStore } from '@/stores/fonts'

const app = useAppStore()
const fonts = useFontsStore()

/** Uploaded font family names (filename without extension), de-duplicated. */
const uploadedFontFamilies = computed(() => {
  const seen = new Set<string>()
  const families: string[] = []
  for (const f of fonts.fonts) {
    const family = fontFamilyFromFile(f.name)
    if (family && !seen.has(family)) {
      seen.add(family)
      families.push(family)
    }
  }
  return families
})

/** Google-font options: live Google Fonts (if available) else the fallback list. */
const fontOptions = computed(() =>
  app.googleFontsList.length ? app.googleFontsList : FALLBACK_GOOGLE_FONTS,
)

/**
 * The currently-saved header font when it appears in neither group (e.g. a font
 * typed in an earlier version, or a Google font not in the fallback list with
 * no API key configured). Surfaced as a "(custom)" option so the combo box
 * never blanks out / silently drops the saved value. Empty string when the
 * value is already covered by a group.
 */
const customHeaderFont = computed(() => {
  const f = (app.settings.header_font || '').trim()
  if (!f) return ''
  if (uploadedFontFamilies.value.includes(f)) return ''
  if (fontOptions.value.includes(f)) return ''
  return f
})

// Load the uploaded-font list and (re)register its @font-face rules so they
// preview live here and reflect any uploads/deletes done on the Font Upload tab.
// Re-applying the header font afterwards lets a saved uploaded font preview as
// soon as its @font-face is registered (it can't render before that).
onMounted(() => fonts.loadFonts())
watch(
  () => fonts.fonts,
  () => {
    applyUploadedFonts(fonts.fonts.map((f) => f.name))
    app.previewHeaderFont()
  },
  { deep: true },
)
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12"><i class="fa-duotone fa-gear"></i> App Settings</h3>
      <div class="settings-form">
        <div class="field mb-10">
          <label class="field-label">App Title</label>
          <input
            v-model="app.settings.app_title"
            placeholder="My App"
            aria-label="App title"
            class="field-input-full"
          />
          <small class="text-dim">Displayed in the browser tab and home page header.</small>
        </div>
        <div class="field mb-10">
          <label class="field-label">Default Draw Delay (seconds)</label>
          <select
            v-model="app.settings.default_draw_delay"
            aria-label="Default draw delay"
            class="field-input-full"
          >
            <option value="0">0 — Instant</option>
            <option value="3">3 seconds</option>
            <option value="5">5 seconds</option>
            <option value="10">10 seconds</option>
            <option value="15">15 seconds</option>
            <option value="20">20 seconds</option>
            <option value="30">30 seconds</option>
            <option value="45">45 seconds</option>
            <option value="60">60 seconds</option>
          </select>
          <small class="text-dim">
            Pre-selected delay when starting a new game. Can still be changed per-draw.
          </small>
        </div>
        <div class="field mb-10">
          <label class="field-label">Frequent Winner Threshold</label>
          <input
            v-model="app.settings.frequent_winner_threshold"
            type="number"
            min="1"
            max="100"
            aria-label="Frequent winner threshold"
            class="field-input-full"
          />
          <small class="text-dim">
            How many wins before a player is flagged as a frequent winner.
          </small>
        </div>
        <div class="field mb-10">
          <label class="field-label">Frequent Winner Lookback (hours)</label>
          <input
            v-model="app.settings.frequent_winner_hours"
            type="number"
            min="1"
            max="168"
            aria-label="Frequent winner hours"
            class="field-input-full"
          />
          <small class="text-dim">Time window to check for frequent winners (1–168 hours).</small>
        </div>
        <div class="field mb-10">
          <label class="field-label">Google Fonts API Key</label>
          <input
            v-model="app.settings.google_fonts_api_key"
            placeholder="Enter API key for font autocomplete"
            aria-label="Google Fonts API key"
            class="field-input-full"
            type="password"
            autocomplete="off"
          />
          <small class="text-dim">
            Optional. Enables full font autocomplete from Google Fonts.
            <a
              href="https://developers.google.com/fonts/docs/developer_api#APIKey"
              target="_blank"
              rel="noopener"
              >Get a free key ↗</a
            >
          </small>
        </div>
        <div class="field mb-10">
          <label class="field-label" for="header-font-select">Header / Board Font</label>
          <div class="flex gap-sm">
            <select
              id="header-font-select"
              v-model="app.settings.header_font"
              aria-label="Header font"
              class="field-input-full"
              @change="app.previewHeaderFont()"
            >
              <option v-if="customHeaderFont" :value="customHeaderFont">
                {{ customHeaderFont }} (custom)
              </option>
              <optgroup v-if="uploadedFontFamilies.length" label="Uploaded Fonts">
                <option
                  v-for="f in uploadedFontFamilies"
                  :key="'up-' + f"
                  :value="f"
                  :style="{ fontFamily: `'${f}', serif` }"
                >
                  {{ f }}
                </option>
              </optgroup>
              <optgroup label="Google Fonts">
                <option v-for="f in fontOptions" :key="'g-' + f" :value="f">{{ f }}</option>
              </optgroup>
            </select>
            <a
              href="https://fonts.google.com"
              target="_blank"
              rel="noopener"
              class="btn-ghost btn-sm"
              style="white-space: nowrap"
              >Browse Fonts ↗</a
            >
          </div>
          <small class="text-dim">
            Choose a Google font or one uploaded under System → Font Upload (uploaded fonts are
            listed first). The preview below updates live.
          </small>

          <div
            class="font-preview mt-8"
            :style="{ fontFamily: '\'' + (app.settings.header_font || 'Arapey') + '\', serif' }"
          >
            <span style="font-size: 2rem; font-weight: 800; letter-spacing: 2px">B I N G O</span
            ><br />
            <span style="font-size: 1.3rem; font-weight: 700">1 &nbsp; 23 &nbsp; 45 &nbsp; 67</span
            ><br />
            <span style="font-size: 3rem; font-weight: 700; text-transform: uppercase">
              {{ app.settings.app_title || 'App Title' }}
            </span>
          </div>
        </div>
        <template v-for="club in BOOK_CLUBS" :key="club.slug">
          <div class="field mb-10">
            <label class="field-label">{{ club.name }} — Reading List Webhook URL</label>
            <input
              v-model="app.settings[clubWebhookKey(club.slug)]"
              placeholder="https://discord.com/api/webhooks/…"
              :aria-label="club.name + ' reading list Discord webhook URL'"
              class="field-input-full"
              type="password"
              autocomplete="off"
            />
            <small class="text-dim">
              Publishes this club's reading lists — each item posted as its own embed to this
              channel. Kept private; never sent to non-admin visitors.
            </small>
          </div>
          <div class="field mb-10">
            <label class="field-label">{{ club.name }} — Events Channel Webhook URL</label>
            <input
              v-model="app.settings[clubEventsWebhookKey(club.slug)]"
              placeholder="https://discord.com/api/webhooks/…"
              :aria-label="club.name + ' events Discord webhook URL'"
              class="field-input-full"
              type="password"
              autocomplete="off"
            />
            <small class="text-dim">
              Where this club's scheduled event posts are sent. Kept private; never sent to
              non-admin visitors.
            </small>
          </div>
        </template>
        <div class="field mb-10">
          <label class="field-label">AniList API URL (Book Clubs)</label>
          <input
            v-model="app.settings.anilist_api_url"
            placeholder="https://graphql.anilist.co"
            aria-label="AniList API URL"
            class="field-input-full"
          />
          <small class="text-dim">
            GraphQL endpoint for the "Pull from AniList" lookup when adding reading list items.
            No API key needed.
          </small>
        </div>
        <button class="btn-primary" :disabled="app.savingSettings" @click="app.saveSettings()">
          <LoadingSpinner v-if="app.savingSettings" label="Saving…" />
          <template v-else>Save Settings</template>
        </button>
      </div>
    </div>
  </div>
</template>

