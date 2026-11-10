<script setup lang="ts">
/**
 * Admin Settings tab - app title, default draw delay, frequent-winner
 * thresholds, Google Fonts API key, and the header/board font (with live
 * preview). Mirrors the original `adminTab==='system-settings'` block.
 *
 * The header/board font is chosen from a single combo box grouped into two
 * <optgroup>s: fonts uploaded via System -> Font Upload (listed first) and
 * Google Fonts (the live API list when an API key is set, else the shared
 * FALLBACK_GOOGLE_FONTS list). Uploaded fonts' @font-face rules are registered
 * (applyUploadedFonts) so they preview live and can be selected. A previously
 * saved value that is in neither group is preserved as a "(custom)" option so
 * editing settings never silently drops it.
 */
import { computed, onMounted, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import { FALLBACK_GOOGLE_FONTS } from '@/lib/constants'
import { applyUploadedFonts } from '@/lib/theme'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useFontsStore, toUploadedFont } from '@/stores/fonts'

const app = useAppStore()
const auth = useAuthStore()
const fonts = useFontsStore()

/** The allowed tea-room API sites as the textarea shows them: one per line. */
const teaRoomOrigins = computed({
  get: () => app.settings.teaRoomOrigins.join('\n'),
  set: (text: string) => {
    app.settings.teaRoomOrigins = text
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => line !== '')
  },
})

/** Uploaded fonts' effective CSS family names, de-duplicated. */
const uploadedFontFamilies = computed(() => {
  const seen = new Set<string>()
  const families: string[] = []
  for (const f of fonts.fonts) {
    if (f.family && !seen.has(f.family)) {
      seen.add(f.family)
      families.push(f.family)
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
  const f = app.settings.headerFont.trim()
  if (!f) {
    return ''
  }
  if (uploadedFontFamilies.value.includes(f)) {
    return ''
  }
  if (fontOptions.value.includes(f)) {
    return ''
  }
  return f
})

// Load the uploaded-font list and (re)register its @font-face rules so they
// preview live here and reflect any uploads/deletes done on the Font Upload tab.
// Re-applying the header font afterwards lets a saved uploaded font preview as
// soon as its @font-face is registered (it can't render before that).
onMounted(() => {
  void fonts.loadFonts()
  // The integrations are an admin's alone (the server refuses anyone else).
  if (auth.isAdmin) {
    void app.loadIntegrations()
  }
})
watch(
  () => fonts.fonts,
  () => {
    applyUploadedFonts(fonts.fonts.map(toUploadedFont))
    app.previewHeaderFont()
  },
  { deep: true },
)
</script>

<template>
  <div class="tab-body">
    <AdminPanel title="Settings" :icon="['fad', 'gear']">
      <!-- Grouped into logical sections laid out in a responsive two-column flex
           layout: cards fill the row, capped at two per row, collapsing to one
           column on mobile so the page isn't one long scroll. -->
      <div class="settings-grid">
        <!-- General ----------------------------------------------------- -->
        <section class="settings-section">
          <h4 class="section-heading"><font-awesome-icon :icon="['fad', 'sliders']" /> General</h4>
          <FormField label="App Title" help="Displayed in the browser tab and home page header.">
            <input v-model="app.settings.appTitle" placeholder="My App" aria-label="App title" />
          </FormField>
          <FormField
            help="Shown on the home page above the board-ID field where players join a game."
          >
            <template #label>
              Bingo Join Prompt
              <span class="text-muted fw-normal">(Markdown supported)</span>
            </template>
            <MarkdownEditor
              v-model="app.settings.bingoJoinPrompt"
              min-height="100px"
              placeholder="Enter your unique bingo board ID to play"
            />
          </FormField>
          <FormField
            label="Hide Bingo"
            help="Takes Join Bingo and Custom Card off the public home page, for a stretch when no game is running. Existing board and request links keep working."
          >
            <button
              type="button"
              class="switch"
              role="switch"
              :class="{ 'is-on': app.hideBingo }"
              :aria-checked="app.hideBingo"
              aria-label="Hide Bingo on the public home page"
              @click="app.settings.hideBingo = !app.settings.hideBingo"
            >
              <span class="switch-knob"></span>
            </button>
          </FormField>
          <FormField
            label="Allowed sites for the public tea-room API"
            help="Sites whose pages may read the public tea-room data from a browser, one origin per line, such as https://senpan.cafe. https://*.example.com covers every subdomain. Leave empty to allow none."
          >
            <textarea
              v-model="teaRoomOrigins"
              rows="3"
              placeholder="https://senpan.cafe&#10;https://*.senpan.cafe"
              aria-label="Allowed sites for the public tea-room API"
            ></textarea>
          </FormField>
          <FormField
            label="Hide Custom Cards"
            help="Takes only the Custom Card request off the public home page, for when requests are closed but games are still running. Hide Bingo already hides it. Existing request links keep working."
          >
            <button
              type="button"
              class="switch"
              role="switch"
              :class="{ 'is-on': app.hideCustomCards }"
              :aria-checked="app.hideCustomCards"
              aria-label="Hide Custom Cards on the public home page"
              @click="app.settings.hideCustomCards = !app.settings.hideCustomCards"
            >
              <span class="switch-knob"></span>
            </button>
          </FormField>
        </section>

        <!-- Gameplay ----------------------------------------------------- -->
        <section class="settings-section">
          <h4 class="section-heading"><font-awesome-icon :icon="['fad', 'dice']" /> Gameplay</h4>
          <!-- The draw delay is set live on the Game page (in-game "Draw Delay"
               selector), which persists + broadcasts it, so it isn't configured
               here anymore. -->
          <FormField
            label="Frequent Winner Threshold"
            help="How many wins before a player is flagged as a frequent winner."
          >
            <input
              v-model.number="app.settings.frequentWinnerThreshold"
              type="number"
              min="1"
              max="100"
              aria-label="Frequent winner threshold"
            />
          </FormField>
          <FormField
            label="Frequent Winner Lookback (hours)"
            help="Time window to check for frequent winners (1-168 hours)."
          >
            <input
              v-model.number="app.settings.frequentWinnerHours"
              type="number"
              min="1"
              max="168"
              aria-label="Frequent winner hours"
            />
          </FormField>
          <FormField
            label='"It&apos;s Yoever" Cooldown (seconds)'
            help="How long each player must wait between triggering the It's Yoever reaction (0-3600; 0 disables the limit)."
          >
            <input
              v-model.number="app.settings.yoeverCooldownSeconds"
              type="number"
              min="0"
              max="3600"
              aria-label="It's Yoever cooldown seconds"
            />
          </FormField>
          <FormField
            label="Custom Card Cost (gil)"
            help="Gil price shown on the public Personal Card Requests page for a custom bingo card."
          >
            <input
              v-model.number="app.settings.customCardCost"
              type="number"
              min="0"
              max="1000000000"
              aria-label="Custom card cost in gil"
            />
          </FormField>
        </section>

        <!-- Fonts & Branding --------------------------------------------- -->
        <section class="settings-section">
          <h4 class="section-heading">
            <font-awesome-icon :icon="['fad', 'font']" /> Fonts &amp; Branding
          </h4>
          <!-- The key is one of the integrations, which only an admin reads or
               saves (GET/PUT /settings/integrations). -->
          <FormField v-if="auth.isAdmin" label="Google Fonts API Key">
            <input
              v-model="app.integrations.googleFontsApiKey"
              placeholder="Enter API key for font autocomplete"
              aria-label="Google Fonts API key"
              type="password"
              autocomplete="off"
            />
            <template #help>
              Optional. Enables full font autocomplete from Google Fonts.
              <a
                href="https://developers.google.com/fonts/docs/developer_api#APIKey"
                target="_blank"
                rel="noopener"
                >Get a free key <font-awesome-icon :icon="['fas', 'arrow-up-right-from-square']"
              /></a>
            </template>
          </FormField>
          <FormField html-for="header-font-select">
            <template #label>Header / Board Font</template>
            <div class="flex gap-sm">
              <select
                id="header-font-select"
                v-model="app.settings.headerFont"
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
                class="btn-view btn-sm nowrap"
                >Browse Fonts <font-awesome-icon :icon="['fas', 'arrow-up-right-from-square']"
              /></a>
            </div>
            <template #help>
              Choose a Google font or one uploaded under System -> Font Upload (uploaded fonts are
              listed first). The preview below updates live.
            </template>
          </FormField>
          <div
            class="font-preview"
            :style="{ fontFamily: '\'' + (app.settings.headerFont || 'Arapey') + '\', serif' }"
          >
            <span class="font-preview-bingo">B I N G O</span><br />
            <span class="font-preview-numbers">1 &nbsp; 23 &nbsp; 45 &nbsp; 67</span><br />
            <span class="font-preview-title">
              {{ app.settings.appTitle || 'App Title' }}
            </span>
          </div>
        </section>

        <!-- Book Clubs ----------------------------------------------------- -->
        <section class="settings-section">
          <h4 class="section-heading">
            <font-awesome-icon :icon="['fad', 'book-open-cover']" /> Book Clubs
          </h4>
          <FormField
            label="AniList API URL"
            help='GraphQL endpoint for the "Pull from AniList" lookup when adding reading list items. No API key needed.'
          >
            <input
              v-model="app.settings.aniListApiUrl"
              placeholder="https://graphql.anilist.co"
              aria-label="AniList API URL"
            />
          </FormField>
        </section>
      </div>

      <FormActions align="start">
        <button
          type="button"
          class="btn-confirm"
          :disabled="app.savingSettings"
          @click="app.saveSettings(auth.isAdmin)"
        >
          <LoadingSpinner v-if="app.savingSettings" label="Saving..." />
          <template v-else>Save Settings</template>
        </button>
      </FormActions>
    </AdminPanel>
  </div>
</template>

<style scoped>
/* Two-column flex layout: each logical group is a raised card. The ~50% basis
   (half the 20px gap subtracted) caps the row at two cards - a third can never
   fit - and `min-width: min(100%, 340px)` drops it to a single column once a
   card can't hold ~340px (mobile), without overflowing very narrow screens. */
.settings-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
  align-items: flex-start;
  margin-bottom: 20px;
}

.settings-section {
  flex: 1 1 calc(50% - 10px);
  min-width: min(100%, 340px);
  background: var(--panel-raised-bg);
  border-radius: var(--radius);
  padding: 16px 18px;
}

/* The shared .section-heading object sets colour + bottom margin; just clear the
   default heading top margin so it sits flush with the card top. */
.settings-section > .section-heading {
  margin-top: 0;
}

/* Last field/preview in a card shouldn't add trailing space below the card. */
.settings-section > :last-child {
  margin-bottom: 0;
}

.settings-section .font-preview {
  margin-top: 4px;
}

/* The three sample lines inside the live font preview (chosen header font). */
.font-preview-bingo {
  font-size: 2rem;
  font-weight: 800;
  letter-spacing: 2px;
}

.font-preview-numbers {
  font-size: 1.3rem;
  font-weight: 700;
}

.font-preview-title {
  font-size: 3rem;
  font-weight: 700;
  text-transform: uppercase;
}
</style>
