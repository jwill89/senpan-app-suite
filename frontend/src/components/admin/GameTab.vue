<script setup lang="ts">
/**
 * Admin Game tab — "New Game" setup (pattern picker + game details) when no
 * game is active, and the live "Current Game" controls (draw, countdown, last
 * drawn, patterns, winners, frequent winners, called numbers) once started.
 * Mirrors the original `adminTab==='bingo-game'` block exactly.
 */
import CalledNumbers from '@/components/common/CalledNumbers.vue'
import PatternMini from '@/components/common/PatternMini.vue'
import { useMarkdown } from '@/lib/markdown'
import { DRAW_DELAY_OPTIONS } from '@/lib/constants'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'

const game = useGameStore()
const cards = useCardsStore()
const patterns = usePatternsStore()
const { render: renderMarkdown } = useMarkdown()

/** Player name for a winning card id (shown under the winner chip). */
function playerNameFor(id: string): string | undefined {
  return cards.cards.find((c) => c.id === id)?.player_name
}

const delayLabel = (s: number): string => (s === 0 ? 'Instant' : `${s}s Delay`)
</script>

<template>
  <div class="tab-body">
    <div class="admin-panel">
      <h3 class="mb-12">{{ game.adminGameLabel }}</h3>

      <!-- No active game → start one -->
      <div v-if="!game.currentGame" class="game-setup">
        <p v-if="patterns.patterns.length === 0" class="text-dim mb-8">
          Create some win patterns first.
        </p>
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
            :disabled="game.selectedPatternIds.length === 0"
            @click="game.startGame()"
          >
            Start Game
          </button>
        </div>
      </div>

      <!-- Active game -->
      <div v-else class="game-active">
        <div class="draw-area">
          <div class="draw-controls">
            <button
              class="btn-primary btn-lg"
              :disabled="game.drawCountdown !== null"
              @click="game.drawNumber()"
            >
              <i class="fa-solid fa-circle-dot"></i> Draw Number
            </button>
            <select
              v-model.number="game.drawDelay"
              aria-label="Draw delay"
              class="btn-ghost"
              style="padding: 10px 14px; font-size: 0.95rem; font-weight: 600; cursor: pointer"
            >
              <option v-for="s in DRAW_DELAY_OPTIONS" :key="s" :value="s">{{ delayLabel(s) }}</option>
            </select>
            <button class="btn-danger" @click="game.endGame()">End Game</button>
          </div>

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
