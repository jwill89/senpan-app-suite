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
import BingoBoard from '@/components/common/BingoBoard.vue'
import CalledNumbers from '@/components/common/CalledNumbers.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import StampShapePicker from '@/components/player/StampShapePicker.vue'
import StampColorPicker from '@/components/player/StampColorPicker.vue'
import StampOpacitySlider from '@/components/player/StampOpacitySlider.vue'
import SecondaryStampControl from '@/components/player/SecondaryStampControl.vue'
import SoundControls from '@/components/player/SoundControls.vue'
import WinPatternsPanel from '@/components/player/WinPatternsPanel.vue'
import { useMarkdown } from '@/lib/markdown'
import { exportCardImage } from '@/lib/exportCard'
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

/**
 * Whether the "Stamp Customization" panel (shape/color/opacity pickers) is
 * expanded. Players usually set these once per night and then find them
 * distracting, so the panel is collapsible and its open/closed choice is
 * remembered across visits. Defaults to open for first-time discoverability.
 */
const showStampCustomization = ref(localStorage.getItem('bingo_stamp_custom_open') !== '0')
watch(showStampCustomization, (open) => {
  localStorage.setItem('bingo_stamp_custom_open', open ? '1' : '0')
})

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
      playerName: player.playerCard?.player_name ?? '',
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
 * Leave the board and return home (App.vue disconnects the WS on the route
 * change). We navigate *first* and only clear the player's card/stamps once
 * we've actually left — so a navigation that can't complete (e.g. fetching the
 * lazily-loaded Home chunk fails on a direct link, or its hash is stale after a
 * redeploy) can't strand the player on an emptied board. If the in-app
 * navigation fails or is aborted, fall back to a full page load to home.
 */
async function leave(): Promise<void> {
  let failed = false
  try {
    // router.push resolves to a NavigationFailure (truthy) when aborted/
    // redirected, and rejects when a chunk fails to load — treat both as failed.
    failed = Boolean(await router.push({ name: 'home' }))
  } catch {
    failed = true
  }
  if (failed) {
    window.location.assign(import.meta.env.BASE_URL || '/')
    return // full reload resets the store, so no resetPlayer() needed here
  }
  player.resetPlayer()
}

/** Opens the Senpan Discord invite in a new tab. */
function openDiscord(): void {
  window.open('https://discord.gg/QHg69gWBVy', '_blank', 'noopener')
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-neutral btn-sm" @click="leave">← Leave</button>
      <div class="topbar-id">
        <h2>Board <span class="code-gold">{{ player.playerCard?.id }}</span></h2>
        <span v-if="player.playerCard?.player_name" class="topbar-player">
          {{ player.playerCard.player_name }}
        </span>
      </div>
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
      <!-- Column 1: bingo board + stamp controls -->
      <div class="player-col player-col-board">
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
          :secondary-stamp-style="
            player.secondaryStampEnabled ? player.secondaryStampMarkStyle : undefined
          "
          :is-winning-pattern-cell="player.isWinningPatternCell"
          @cell-click="(ri, ci) => player.toggleStamp(ri, ci)"
        />

        <!-- Stamp Settings & Game Controls (collapsible — set once, then tuck away) -->
        <div class="stamp-customization">
          <button
            class="stamp-custom-toggle"
            :aria-expanded="showStampCustomization"
            @click="showStampCustomization = !showStampCustomization"
          >
            <font-awesome-icon :icon="['fas', 'sliders']" />
            <span>Stamp Settings &amp; Game Controls</span>
            <font-awesome-icon
              class="stamp-custom-chevron"
              :icon="['fas', showStampCustomization ? 'chevron-up' : 'chevron-down']"
            />
          </button>
          <div v-show="showStampCustomization" class="stamp-custom-body">
            <!-- Stamp shape + color pickers share the first row -->
            <div class="stamp-shape-color-row">
              <StampShapePicker />
              <StampColorPicker />
            </div>

            <!-- Optional secondary stamp (auto-marks non-pattern cells) -->
            <SecondaryStampControl />

            <!-- Opacity slider gets its own full-width row -->
            <StampOpacitySlider />

            <!-- Sound mode (off/basic/game) + volume slider -->
            <SoundControls />

            <!-- Clear / Save action bar -->
            <div class="player-actions">
              <button
                class="btn-caution btn-sm"
                title="Clear all stamps on the board"
                @click="player.clearAllStamps()"
              >
                <font-awesome-icon :icon="['fas', 'eraser']" />
                <span class="player-actions__label">Clear Board</span>
              </button>

              <button
                class="btn-view btn-sm"
                :disabled="exporting"
                :title="exporting ? 'Saving card image…' : 'Save card as image'"
                @click="exportCard"
              >
                <font-awesome-icon :icon="['fas', 'download']" />
                <span class="player-actions__label">{{ exporting ? 'Saving…' : 'Save Board' }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Column 2: last called number + called numbers list, in one box -->
      <div class="player-col player-col-called">
        <div class="called-combined">
          <!-- Last number the caller drew (announcement only — no board tracking) -->
          <template v-if="player.playerGame && player.lastDrawn">
            <div class="last-called">
              <span class="last-called-label">Last Called</span>
              <div class="last-called-row">
                <span
                  class="last-called-flourish last-called-flourish--left"
                  aria-hidden="true"
                ></span>
                <div :key="player.lastDrawn.call_order" class="last-drawn last-drawn--pop">
                  <span class="letter">{{ player.lastDrawn.letter }}</span>
                  <span class="number">{{ player.lastDrawn.number }}</span>
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
            :count="player.playerGame ? player.playerGame.called_numbers.length : 0"
            :is-called="player.isCalledPlayer"
          />
        </div>
      </div>

      <!-- Column 3: game details, winning patterns, misc messages -->
      <div class="player-col player-col-info">
        <!-- Game details (Markdown) — above the win patterns, full column width -->
        <div
          v-if="player.playerGame && game.gameDetails"
          class="game-details"
          v-html="renderMarkdown(game.gameDetails)"
        ></div>

        <WinPatternsPanel v-if="player.playerGame" :patterns="player.playerGame.patterns" />

        <template v-if="!player.playerGame">
          <div v-if="player.gameEnded" class="game-over-msg">
            <div class="go-icon"><font-awesome-icon :icon="['fad', 'trophy']" /></div>
            <p class="go-title">We have a Winner — Thanks for Playing!</p>
            <p class="go-sub">Numbers called this game: {{ player.endedCalledCount }}</p>
            <br/>
            <p class="go-sub">
              Feel free to save your board and dump them into the bingo-boards channel in the
              Senpan Discord server if you want to show off your cursed boards! Afterwards, you can
              clear your board.
            </p>
            <br/>
            <p class="go-sub">
              If you'd like more refreshments, please let our staff know before the round starts!
            </p>
            <br/>
            <p class="go-sub">The next game will begin soon — hang tight!</p>
            <button class="btn-action go-discord-btn" @click="openDiscord">
              <font-awesome-icon :icon="['fab', 'discord']" /> Join the Discord Server
            </button>
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
      <h3 class="mb-16"><font-awesome-icon :icon="['fad', 'champagne-glasses']" /> Half-Time Minigame!</h3>
      <p class="text-dim mb-20">
        It's time for a half-time minigame! Please check your in-game chat for details and
        instructions!
      </p>
      <button class="btn-neutral" @click="player.showMinigameModal = false">Got it!</button>
    </ModalOverlay>
  </div>
</template>
