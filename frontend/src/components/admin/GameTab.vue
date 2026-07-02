<script setup lang="ts">
/**
 * Admin Game tab — "New Game" setup (pattern picker + game details) when no
 * game is active, and the live "Current Game" controls (draw, countdown, last
 * drawn, patterns, winners, frequent winners, called numbers) once started.
 * Mirrors the original `adminTab==='bingo-game'` block exactly.
 */
import { onBeforeUnmount, onMounted, computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import CalledNumbers from '@/components/common/CalledNumbers.vue'
import PatternMini from '@/components/common/PatternMini.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownEditor from '@/components/common/MarkdownEditor.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import PatternPicker from '@/components/common/ui/PatternPicker.vue'
import { DRAW_DELAY_OPTIONS } from '@/lib/constants'
import { parseServerTimestamp } from '@/lib/datetime'
import { primeAudio, playWinnerChime } from '@/lib/sound'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'
import { usePresetsStore } from '@/stores/presets'

const router = useRouter()
const game = useGameStore()
const cards = useCardsStore()
const patterns = usePatternsStore()
const presets = usePresetsStore()

// Currently-selected preset in the "Start from a preset" picker (v-model).
const selectedPresetId = ref<number | null>(null)

/**
 * Applies a saved preset to the new-game form: pre-selects its win patterns
 * (skipping any that were since deleted) and fills in its game details. The
 * admin can still tweak the selection before starting.
 */
function applyPreset(): void {
  const preset = presets.presets.find((p) => p.id === selectedPresetId.value)
  if (!preset) return
  const validIds = new Set(patterns.patterns.map((p) => p.id))
  game.selectedPatternIds = preset.pattern_ids.filter((id) => validIds.has(id))
  game.gameDetails = preset.game_details || ''
  void game.saveGameDetails()
}

// ── Elapsed-game clock (admin-only, Current Game view) ──────────────────────
// Ticks once a second while this tab is mounted; the start time comes from the
// game state's `created_at` (stored as UTC), so it stays accurate across
// refreshes and for multiple admins.
const now = ref(Date.now())
let clockTimer: ReturnType<typeof setInterval> | null = null

/** Live elapsed-time string (H:MM:SS / MM:SS) since the game started. */
const elapsedTime = computed(() => {
  const start = game.currentGame ? parseServerTimestamp(game.currentGame.created_at) : NaN
  if (!Number.isFinite(start)) return ''
  const secs = Math.max(0, Math.floor((now.value - start) / 1000))
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  const s = secs % 60
  const mm = String(m).padStart(2, '0')
  const ss = String(s).padStart(2, '0')
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`
})

/** Player name for a winning card id (shown under the winner chip). */
function playerNameFor(id: string): string | undefined {
  return cards.cards.find((c) => c.id === id)?.player_name
}

const delayLabel = (s: number): string => (s === 0 ? 'Instant' : `${s}s Delay`)

/** Toggles the winner-sound alert; enabling primes audio and plays a sample. */
function toggleWinnerSound(): void {
  const next = !game.winnerSoundEnabled
  game.setWinnerSoundEnabled(next)
  if (next) {
    primeAudio()
    playWinnerChime()
  }
}

/** Jump to the Patterns tab (from the "no patterns yet" hint). */
function goToPatterns(): void {
  void router.push({ name: 'admin-bingo-patterns' })
}

// Keyboard shortcut: Space (or Enter) draws the next number during an active
// game — speeds up calling a fast game. Ignored while typing in a form field,
// while a draw is already in flight, or during the inter-draw countdown.
function onKeydown(e: KeyboardEvent): void {
  if (e.key !== ' ' && e.key !== 'Enter') return
  if (!game.currentGame || game.drawing || game.drawCountdown !== null) return
  // Don't hijack a focused form control or button — let it handle the key
  // itself (e.g. Enter on "End Game" should end, not draw).
  const el = document.activeElement as HTMLElement | null
  const tag = el?.tagName
  if (
    tag === 'INPUT' ||
    tag === 'TEXTAREA' ||
    tag === 'SELECT' ||
    tag === 'BUTTON' ||
    el?.isContentEditable
  ) {
    return
  }
  e.preventDefault()
  void game.drawNumber()
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  if (presets.presets.length === 0) void presets.loadPresets()
  clockTimer = setInterval(() => {
    now.value = Date.now()
  }, 1000)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  if (clockTimer) clearInterval(clockTimer)
})
</script>

<template>
  <div class="tab-body">
    <AdminPanel>
      <h3 class="mb-12">
        {{ game.adminGameLabel }}
        <span
          v-if="game.currentGame"
          class="live-badge"
          role="status"
          aria-label="Game in progress"
        >
          <span class="live-dot" aria-hidden="true"></span>Live
        </span>
        <span
          v-if="game.currentGame && elapsedTime"
          class="game-clock"
          role="timer"
          aria-live="off"
          :aria-label="`Game time elapsed: ${elapsedTime}`"
          title="Time elapsed since the game started"
        >
          <font-awesome-icon :icon="['fad', 'clock']" /> {{ elapsedTime }}
        </span>
      </h3>

      <!-- No active game → start one -->
      <div v-if="!game.currentGame" class="game-setup">
        <div v-if="patterns.patterns.length === 0" class="mb-12">
          <p class="text-dim mb-8">Create some win patterns first.</p>
          <button class="btn-confirm btn-sm" @click="goToPatterns">
            <font-awesome-icon :icon="['fas', 'plus']" /> Create a Pattern
          </button>
        </div>
        <div v-else>
          <!-- Start from a saved preset (auto-fills patterns + details) -->
          <div v-if="presets.presets.length" class="flex-toolbar mb-12">
            <label class="text-dim text-sm">Start from a preset:</label>
            <select
              v-model.number="selectedPresetId"
              aria-label="Game preset"
              class="manager-filter"
            >
              <option :value="null">— None —</option>
              <option v-for="p in presets.presets" :key="p.id" :value="p.id">{{ p.name }}</option>
            </select>
            <button
              class="btn-action btn-sm"
              :disabled="selectedPresetId === null"
              title="Apply this preset's patterns and game details"
              @click="applyPreset"
            >
              <font-awesome-icon :icon="['fas', 'circle-check']" /> Apply Preset
            </button>
          </div>

          <p class="text-dim mb-12">Select one or more win patterns:</p>

          <PatternPicker v-model="game.selectedPatternIds" />

          <!-- Game details editor -->
          <div class="game-details-editor">
            <label class="field-label">
              Game Details
              <span class="text-dim fw-normal text-xs"> (Markdown supported) </span>
            </label>
            <MarkdownEditor
              v-model="game.gameDetails"
              min-height="120px"
              placeholder="Enter game details, rules, prizes, etc. Supports bold, italics, lists, and more…"
              @blur="game.saveGameDetails()"
            />
          </div>

          <button
            class="btn-action btn-lg"
            :disabled="game.selectedPatternIds.length === 0 || game.starting"
            @click="game.startGame()"
          >
            <LoadingSpinner v-if="game.starting" label="Starting…" />
            <template v-else>
              Start Game
              <span v-if="game.selectedPatternIds.length" style="opacity: 0.85">
                ({{ game.selectedPatternIds.length }} selected)
              </span>
            </template>
          </button>
        </div>
      </div>

      <!-- Active game -->
      <div v-else class="game-active">
        <div class="draw-area">
          <div class="draw-controls">
            <button
              class="btn-action btn-lg"
              :disabled="game.drawCountdown !== null || game.drawing"
              @click="game.drawNumber()"
            >
              <LoadingSpinner v-if="game.drawing" label="Drawing…" />
              <template v-else
                ><font-awesome-icon :icon="['fas', 'circle-dot']" /> Draw Number</template
              >
            </button>
            <select
              v-model.number="game.drawDelay"
              aria-label="Draw delay"
              class="btn-neutral"
              style="padding: 10px 14px; font-size: 0.95rem; font-weight: 600; cursor: pointer"
              @change="game.persistDrawDelay()"
            >
              <option v-for="s in DRAW_DELAY_OPTIONS" :key="s" :value="s">
                {{ delayLabel(s) }}
              </option>
            </select>
            <button class="btn-caution" :disabled="game.ending" @click="game.endGame()">
              <LoadingSpinner v-if="game.ending" label="Ending…" />
              <template v-else>End Game</template>
            </button>

            <button
              class="btn-neutral btn-sm winner-sound-toggle"
              :aria-pressed="game.winnerSoundEnabled"
              :title="
                game.winnerSoundEnabled
                  ? 'Winner sound on — click to mute'
                  : 'Winner sound off — click to enable'
              "
              @click="toggleWinnerSound"
            >
              <font-awesome-icon
                :icon="['fas', game.winnerSoundEnabled ? 'volume-high' : 'volume-xmark']"
              />
              <span>Winner Sound</span>
            </button>
          </div>

          <p class="text-dim text-xs mt-8">Tip: press <kbd>Space</kbd> to draw the next number.</p>

          <!-- Countdown / Sent indicator -->
          <div v-if="game.drawCountdown !== null" class="draw-countdown">
            <div class="countdown-ring">
              <span class="countdown-number">{{ game.drawCountdown }}</span>
            </div>
            <span class="countdown-label">Sending to players…</span>
          </div>
          <div v-else-if="game.drawSent" class="draw-sent">
            <span class="sent-icon"><font-awesome-icon :icon="['fad', 'circle-check']" /></span>
            <span class="sent-label">Sent to players!</span>
          </div>
        </div>

        <!-- Two-column layout, mirroring the player board: called numbers (with
             the last-drawn announcement) on the left, winners/details/patterns
             on the right. -->
        <div class="game-active-columns">
          <!-- Left column: last drawn + called numbers, in one framed box -->
          <div class="game-active-left">
            <div class="called-combined">
              <template v-if="game.lastDrawn">
                <div class="last-called">
                  <span class="last-called-label">Last Called</span>
                  <div class="last-called-row">
                    <span
                      class="last-called-flourish last-called-flourish--left"
                      aria-hidden="true"
                    ></span>
                    <div :key="game.lastDrawn.call_order" class="last-drawn last-drawn--pop">
                      <span class="letter">{{ game.lastDrawn.letter }}</span>
                      <span class="number">{{ game.lastDrawn.number }}</span>
                    </div>
                    <span
                      class="last-called-flourish last-called-flourish--right"
                      aria-hidden="true"
                    ></span>
                  </div>
                </div>
                <hr class="called-divider" />
              </template>

              <CalledNumbers
                :count="game.currentGame.called_numbers.length"
                :is-called="game.isCalledAdmin"
              />
            </div>
          </div>

          <!-- Right column: winners, then game details, then active patterns -->
          <div class="game-active-right">
            <div v-if="game.winners.length" class="winners-panel">
              <h3><font-awesome-icon :icon="['fad', 'trophy']" /> Winning Cards</h3>
              <p class="text-dim text-xs mb-8">Click a card ID to verify</p>
              <div class="winner-chips">
                <span
                  v-for="w in game.winners"
                  :key="w"
                  class="winner-chip winner-chip-btn"
                  role="button"
                  tabindex="0"
                  @click="game.viewWinner(w)"
                  @keydown.enter="game.viewWinner(w)"
                  @keydown.space.prevent="game.viewWinner(w)"
                >
                  {{ w }}
                  <small
                    v-if="playerNameFor(w)"
                    style="display: block; font-size: 0.7rem; opacity: 0.8"
                  >
                    {{ playerNameFor(w) }}
                  </small>
                </span>
              </div>
            </div>

            <div v-if="game.frequentWinners.length" class="frequent-winners-panel">
              <h3>
                <font-awesome-icon :icon="['fad', 'triangle-exclamation']" /> Frequent Winners (3+
                in 12h)
              </h3>
              <div class="frequent-winner-chips">
                <span v-for="fw in game.frequentWinners" :key="fw.player_name" class="winner-chip">
                  {{ fw.player_name }} ({{ fw.win_count }})
                </span>
              </div>
            </div>

            <MarkdownText v-if="game.gameDetails" class="game-details" :source="game.gameDetails" />

            <div class="patterns-panel">
              <h3>Active Win Patterns</h3>
              <div class="pattern-cards">
                <div v-for="(p, i) in game.currentGame.patterns" :key="i" class="pattern-card">
                  <PatternMini :pattern-data="p.pattern_data" />
                  <span :title="p.name">{{ p.name }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </AdminPanel>
  </div>
</template>
