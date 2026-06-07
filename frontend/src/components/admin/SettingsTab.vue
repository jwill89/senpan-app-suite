<script setup lang="ts">
/**
 * Admin App Settings tab — app title, default draw delay, frequent-winner
 * thresholds, Google Fonts API key, and the header/board font (with live
 * preview + autocomplete datalist). Mirrors the original
 * `adminTab==='system-settings'` block. The fallback font datalist uses the
 * shared FALLBACK_GOOGLE_FONTS constant (identical list to the original).
 */
import { computed } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { FALLBACK_GOOGLE_FONTS } from '@/lib/constants'
import { useAppStore } from '@/stores/app'

const app = useAppStore()

/** Datalist options: the live Google Fonts list if available, else the fallback. */
const fontOptions = computed(() =>
  app.googleFontsList.length ? app.googleFontsList : FALLBACK_GOOGLE_FONTS,
)
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12"><i class="fa-solid fa-gear"></i> App Settings</h3>
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
          <label class="field-label">Header / Board Font</label>
          <div class="flex gap-sm">
            <input
              v-model="app.settings.header_font"
              placeholder="Arapey"
              aria-label="Header font"
              class="field-input-full"
              list="google-fonts-list"
              @input="app.previewHeaderFont()"
            />
            <a
              href="https://fonts.google.com"
              target="_blank"
              rel="noopener"
              class="btn-ghost btn-sm"
              style="white-space: nowrap"
              >Browse Fonts ↗</a
            >
          </div>
          <datalist id="google-fonts-list">
            <option v-for="f in fontOptions" :key="f" :value="f"></option>
          </datalist>
          <small class="text-dim">
            Google Font family name for headings and the bingo board. Type a name and it previews
            live.
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
        <button class="btn-primary" :disabled="app.savingSettings" @click="app.saveSettings()">
          <LoadingSpinner v-if="app.savingSettings" label="Saving…" />
          <template v-else>Save Settings</template>
        </button>
      </div>
    </div>
  </div>
</template>
