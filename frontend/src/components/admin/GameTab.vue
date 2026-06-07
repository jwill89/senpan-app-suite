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
import { useMarkdown } from '@/lib/markdown'
import { DRAW_DELAY_OPTIONS } from '@/lib/constants'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'

const router = useRouter()
const game = useGameStore()
const cards = useCardsStore()
const patterns = usePatternsStore()
const { render: renderMarkdown } = useMarkdown()

// ── Elapsed-game clock (admin-only, Current Game view) ──────────────────────
// Ticks once a second while this tab is mounted; the start time comes from the
// game state's `created_at` (stored as UTC), so it stays accurate across
// refreshes and for multiple admins.
const now = ref(Date.now())
let clockTimer: ReturnType<typeof setInterval> | null = null

/** Parses a SQLite "YYYY-MM-DD HH:MM:SS" (UTC) or ISO timestamp to epoch ms. */
function parseUtc(ts: string): number {
  if (!ts) return NaN
  // Already has a timezone designator (Z or ±HH:MM) → parse as-is.
  if (/[zZ]|[+-]\d{2}:?\d{2}$/.test(ts)) return new Date(ts).getTime()
  // SQLite CURRENT_TIMESTAMP form: treat the space-separated value as UTC.
  return new Date(ts.replace(' ', 'T') + 'Z').getTime()
}

/** Live elapsed-time string (H:MM:SS / MM:SS) since the game started. */
const elapsedTime = computed(() => {
  const start = game.currentGame ? parseUtc(game.currentGame.created_at) : NaN
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

/** Jump to the New Pattern tab (from the "no patterns yet" hint). */
function goToNewPattern(): void {
  router.push({ name: 'admin-bingo-new-pattern' })
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
  game.drawNumber()
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
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
    <div class="admin-panel">
      <h3 class="mb-12">
        {{ game.adminGameLabel }}
        <span v-if="game.currentGame" class="live-badge" role="status" aria-label="Game in progress">
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
          <i class="fa-solid fa-clock" aria-hidden="true"></i> {{ elapsedTime }}
        </span>
      </h3>

      <!-- No active game → start one -->
      <div v-if="!game.currentGame" class="game-setup">
        <div v-if="patterns.patterns.length === 0" class="mb-12">
          <p class="text-dim mb-8">Create some win patterns first.</p>
          <button class="btn-secondary btn-sm" @click="goToNewPattern">
            <i class="fa-solid fa-plus"></i> Create a Pattern
          </button>
        </div>
        <div v-else>
          <p class="text-dim mb-12">Select one or more win patterns:</p>

          <!-- Pattern filter bar -->
          <div class="flex-toolbar mb-12">
            <input
              v-model="patterns.patternSearchQuery"
              placeholder="Search patterns…"
              aria-label="Search patterns"
              style="flex: 1; min-width: 140px; max-width: 260px"
            />
            <select
              v-model="patterns.patternCategoryFilter"
              aria-label="Filter by category"
              style="
                padding: 6px 10px;
                border-radius: 6px;
                background: var(--surface);
                color: var(--text);
                border: 1px solid var(--surface2);
              "
            >
              <option :value="null">All Categories</option>
              <option v-for="c in patterns.categories" :key="c.id" :value="c.id">
                {{ c.name }}
              </option>
            </select>
          </div>

          <div class="pattern-checks">
            <label
              v-for="p in patterns.gameFilteredPatterns"
              :key="p.id"
              :class="['pattern-check', game.selectedPatternIds.includes(p.id) ? 'selected' : '']"
            >
              <input type="checkbox" :value="p.id" v-model="game.selectedPatternIds" />
              <span class="dot"></span>
              <span>{{ p.name }}</span>
              <span style="font-size: 0.75rem; color: var(--text-dim); margin-left: 4px">
                ({{ p.category_name }})
              </span>
              <PatternMini
                :pattern-data="p.pattern_data"
                size="pattern-mini-sm"
                inline
                style="margin-left: 6px"
              />
            </label>
          </div>

          <!-- Game details editor -->
          <div class="game-details-editor">
            <label
              style="color: var(--text-dim); font-size: 0.9rem; display: block; margin-bottom: 6px"
            >
              Game Details <span style="font-size: 0.8rem; opacity: 0.6">(Markdown supported)</span>
            </label>
            <textarea
              v-model="game.gameDetails"
              aria-label="Game details"
              placeholder="Enter game details, rules, prizes, etc.&#10;Supports **bold**, *italic*, lists, and more…"
              rows="4"
              @blur="game.saveGameDetails()"
            ></textarea>
          </div>

          <button
            class="btn-primary btn-lg"
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
              class="btn-primary btn-lg"
              :disabled="game.drawCountdown !== null || game.drawing"
              @click="game.drawNumber()"
            >
              <LoadingSpinner v-if="game.drawing" label="Drawing…" />
              <template v-else><i class="fa-solid fa-circle-dot"></i> Draw Number</template>
            </button>
            <select
              v-model.number="game.drawDelay"
              aria-label="Draw delay"
              class="btn-ghost"
              style="padding: 10px 14px; font-size: 0.95rem; font-weight: 600; cursor: pointer"
            >
              <option v-for="s in DRAW_DELAY_OPTIONS" :key="s" :value="s">{{ delayLabel(s) }}</option>
            </select>
            <button class="btn-danger" :disabled="game.ending" @click="game.endGame()">
              <LoadingSpinner v-if="game.ending" label="Ending…" />
              <template v-else>End Game</template>
            </button>
          </div>

          <p class="text-dim text-xs mt-8">
            Tip: press <kbd>Space</kbd> to draw the next number.
          </p>

          <!-- Countdown / Sent indicator -->
          <div v-if="game.drawCountdown !== null" class="draw-countdown">
            <div class="countdown-ring">
              <span class="countdown-number">{{ game.drawCountdown }}</span>
            </div>
            <span class="countdown-label">Sending to players…</span>
          </div>
          <div v-else-if="game.drawSent" class="draw-sent">
            <span class="sent-icon"><i class="fa-solid fa-circle-check"></i></span>
            <span class="sent-label">Sent to players!</span>
          </div>

          <div v-if="game.lastDrawn" class="last-drawn">
            <span class="letter">{{ game.lastDrawn.letter }}</span>
            <span class="number">{{ game.lastDrawn.number }}</span>
          </div>
        </div>

        <!-- Two-column layout for game info -->
        <div class="game-active-columns">
          <!-- Left column: patterns + details + winners -->
          <div class="game-active-left">
            <div class="patterns-panel">
              <h3>Active Win Patterns</h3>
              <div class="pattern-cards">
                <div v-for="(p, i) in game.currentGame.patterns" :key="i" class="pattern-card">
                  <PatternMini :pattern-data="p.pattern_data" />
                  <span :title="p.name">{{ p.name }}</span>
                </div>
              </div>
            </div>

            <div
              v-if="game.gameDetails"
              class="game-details game-details--wide"
              v-html="renderMarkdown(game.gameDetails)"
            ></div>

            <div v-if="game.winners.length" class="winners-panel">
              <h3><i class="fa-solid fa-trophy"></i> Winning Cards</h3>
              <p class="text-dim text-xs mb-8">Click a card ID to verify</p>
              <div class="winner-chips">
                <span
                  v-for="w in game.winners"
                  :key="w"
                  class="winner-chip winner-chip-btn"
                  @click="game.viewWinner(w)"
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
                <i class="fa-solid fa-triangle-exclamation"></i> Frequent Winners (3+ in 12h)
              </h3>
              <div class="frequent-winner-chips">
                <span v-for="fw in game.frequentWinners" :key="fw.player_name" class="winner-chip">
                  {{ fw.player_name }} ({{ fw.win_count }})
                </span>
              </div>
            </div>
          </div>

          <!-- Right column: called numbers -->
          <div class="game-active-right">
            <CalledNumbers
              :count="game.currentGame.called_numbers.length"
              :is-called="game.isCalledAdmin"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
