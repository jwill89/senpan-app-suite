<script setup lang="ts">
/**
 * Player view — the bingo board, stamp controls, win patterns, called numbers,
 * game details, and the halftime minigame alert.
 *
 * Receives the board id via the `cardId` route param. On mount (and when the
 * param changes) it loads the board if it isn't already loaded — this makes the
 * URL directly linkable and survive a refresh. If the board can't be loaded
 * (e.g. bad/expired id) it redirects home. The WebSocket connect/disconnect is
 * driven by App.vue off the active route + loaded card.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { icon } from '@fortawesome/fontawesome-svg-core'
import { faVolumeHigh, faVolumeXmark } from '@fortawesome/free-solid-svg-icons'
import BingoBoard from '@/components/common/BingoBoard.vue'
import CalledNumbers from '@/components/common/CalledNumbers.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import StampShapePicker from '@/components/player/StampShapePicker.vue'
import StampColorPicker from '@/components/player/StampColorPicker.vue'
import StampOpacitySlider from '@/components/player/StampOpacitySlider.vue'
import WinPatternsPanel from '@/components/player/WinPatternsPanel.vue'
import { useMarkdown } from '@/lib/markdown'
import { exportCardImage } from '@/lib/exportCard'
import { primeAudio, playDrawChime } from '@/lib/sound'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { usePlayerStore } from '@/stores/player'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ cardId: string }>()

const router = useRouter()
const player = usePlayerStore()
const game = useGameStore()
const app = useAppStore()
const ui = useUiStore()
const { render: renderMarkdown } = useMarkdown()

/** Ref to the BingoBoard component so we can capture its `.board-wrap` root. */
const boardRef = ref<{ $el?: HTMLElement } | null>(null)
/** True while the card image is being generated (drives the export button). */
const exporting = ref(false)

/** Saves the current board (with stamps + active theme) as a framed PNG image. */
async function exportCard(): Promise<void> {
  const el = boardRef.value?.$el
  if (!el || exporting.value) return
  exporting.value = true
  try {
    await exportCardImage({
      element: el,
      fileName: `bingo-card-${player.playerCard?.id ?? 'card'}`,
      title: app.settings.app_title || 'Bingo',
      cardId: player.playerCard?.id ?? '',
      link: window.location.host,
      gameDetails: game.gameDetails,
    })
    ui.notify('Card image saved!', 'success')
  } catch {
    ui.notify('Could not export card image.', 'error')
  } finally {
    exporting.value = false
  }
}

/** Loads the board for the current cardId param if not already loaded. */
async function ensureLoaded(id: string): Promise<void> {
  if (player.playerCard && player.playerCard.id === id) return
  const details = await player.loadBoardById(id)
  if (details === null && !player.playerCard) {
    // Failed to load (bad id) → bounce home with the error toast.
    router.replace({ name: 'home' })
    return
  }
  if (details !== null) game.gameDetails = details
}

onMounted(() => ensureLoaded(props.cardId))
watch(
  () => props.cardId,
  (id) => ensureLoaded(id),
)

// ── Live connection badge ────────────────────────────────────────────────────
// On the player view the socket is always meant to be connected, so the brief
// pre-connect `closed` state (before App.vue opens the connection) is shown as
// "Connecting…" rather than an alarming red "Offline" — the player should never
// think something is broken just because the link hasn't opened yet. A genuine
// give-up after max reconnect attempts still raises its own "please refresh"
// error toast (see WsClient), so no information is lost.
const connLabel = computed(() => {
  switch (ui.wsStatus) {
    case 'open':
      return 'Live'
    case 'reconnecting':
      return 'Reconnecting…'
    default: // 'connecting' or the transient pre-connect 'closed'
      return 'Connecting…'
  }
})
const connClass = computed(() =>
  ui.wsStatus === 'closed' ? 'is-connecting' : `is-${ui.wsStatus}`,
)

/**
 * Toggles the opt-in draw chime. Enabling counts as the user gesture browsers
 * require to start audio, so we prime the context and play a sample so the
 * player can confirm it works.
 */
function toggleSound(): void {
  const next = !player.soundEnabled
  player.setSoundEnabled(next)
  if (next) {
    primeAudio()
    playDrawChime()
  }
}

/**
 * Reactive inline SVG for the sound toggle. FontAwesome's `dom.watch()` replaces
 * `<i class="fa-…">` placeholders with SVG once, so a reactive `:class` swap on
 * an already-replaced node never re-renders. Building the SVG via the `icon()`
 * API and binding it with `v-html` keeps the icon in sync with `soundEnabled`.
 */
const soundIconHtml = computed(
  () => icon(player.soundEnabled ? faVolumeHigh : faVolumeXmark).html.join(''),
)

/** Leave the board: reset state and return home (App disconnects the WS). */
function leave(): void {
  player.resetPlayer()
  router.push({ name: 'home' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-ghost btn-sm" @click="leave">← Leave</button>
      <h2>Board <span class="code-gold">{{ player.playerCard?.id }}</span></h2>
      <span
        class="conn-badge"
        :class="connClass"
        role="status"
        :aria-label="`Connection status: ${connLabel}`"
        :title="`Connection: ${connLabel}`"
      >
        <span class="conn-dot" aria-hidden="true"></span>{{ connLabel }}
      </span>
    </div>
    <div class="player-body">
      <div class="player-left">
        <!-- The bingo board -->
        <BingoBoard
          v-if="player.playerCard"
          ref="boardRef"
          :board="player.playerCard.board_data"
          mode="player"
          :is-stamped="player.isStamped"
          :cell-class="player.boardCellClass"
          :stamp-mark-style="player.stampMarkStyle"
          :stamp-emoji="player.currentStampEmoji"
          :stamp-shape="player.stampShape"
          :custom-stamp-image="player.customStampImage"
          @cell-click="(ri, ci) => player.toggleStamp(ri, ci)"
        />

        <StampShapePicker />
        <StampColorPicker />
        <StampOpacitySlider />

        <div class="player-actions">
          <button
            class="btn-ghost btn-sm"
            title="Clear all stamps on the board"
            @click="player.clearAllStamps()"
          >
            <i class="fa-solid fa-eraser" aria-hidden="true"></i>
            <span class="player-actions__label">Clear</span>
          </button>

          <button
            class="btn-ghost btn-sm"
            :disabled="exporting"
            :title="exporting ? 'Saving card image…' : 'Save card as image'"
            @click="exportCard"
          >
            <i class="fa-solid fa-download" aria-hidden="true"></i>
            <span class="player-actions__label">{{ exporting ? 'Saving…' : 'Save' }}</span>
          </button>

          <button
            class="btn-ghost btn-sm"
            :aria-pressed="player.soundEnabled"
            :title="player.soundEnabled ? 'Draw sound on — click to mute' : 'Draw sound off — click to enable'"
            @click="toggleSound"
          >
            <span class="fa-icon" v-html="soundIconHtml" aria-hidden="true"></span>
            <span class="player-actions__label">{{ player.soundEnabled ? 'Sound On' : 'Sound Off' }}</span>
          </button>
        </div>

        <!-- Game details (Markdown) -->
        <div
          v-if="player.playerGame && game.gameDetails"
          class="game-details"
          v-html="renderMarkdown(game.gameDetails)"
        ></div>
      </div>

      <div class="player-right">
        <!-- Last number the caller drew (announcement only — no board tracking) -->
        <div v-if="player.playerGame && player.lastDrawn" class="last-called">
          <span class="last-called-label">Last Called</span>
          <div :key="player.lastDrawn.call_order" class="last-drawn last-drawn--pop">
            <span class="letter">{{ player.lastDrawn.letter }}</span>
            <span class="number">{{ player.lastDrawn.number }}</span>
          </div>
        </div>

        <WinPatternsPanel v-if="player.playerGame" :patterns="player.playerGame.patterns" />

        <CalledNumbers
          :count="player.playerGame ? player.playerGame.called_numbers.length : 0"
          :is-called="player.isCalledPlayer"
        />

        <template v-if="!player.playerGame">
          <div v-if="player.gameEnded" class="game-over-msg">
            <div class="go-icon"><i class="fa-solid fa-flag-checkered"></i></div>
            <p class="go-title">That's a wrap — thanks for playing!</p>
            <p class="go-sub">Numbers called this game: {{ player.endedCalledCount }}</p>
            <p class="go-sub">Hang tight for the next game to begin.</p>
          </div>
          <div v-else class="no-game-msg">No game is currently active. Waiting…</div>
        </template>
      </div>
    </div>

    <!-- Halftime minigame alert (player) -->
    <ModalOverlay
      v-if="player.showMinigameModal"
      centered
      @close="player.showMinigameModal = false"
    >
      <h3 class="mb-16"><i class="fa-solid fa-champagne-glasses"></i> Half-Time Minigame!</h3>
      <p class="text-dim mb-20">
        It's time for a half-time minigame! Please check your in-game chat for details and
        instructions!
      </p>
      <button class="btn-primary" @click="player.showMinigameModal = false">Got it!</button>
    </ModalOverlay>
  </div>
</template>
