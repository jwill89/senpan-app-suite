<script setup lang="ts">
/**
 * Home view - landing page with the app logo/title, the Join card, and an
 * optional Raffles card. Navigates via the router (`/play/:cardId`,
 * `/raffles`, `/admin/login`).
 */
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import { useMarkdown } from '@/lib/markdown'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { usePlayerStore } from '@/stores/player'
import { useRafflesStore } from '@/stores/raffles'
import { useStampRalliesStore } from '@/stores/stampRallies'

const router = useRouter()
const app = useAppStore()
const player = usePlayerStore()
const raffles = useRafflesStore()
const stampRallies = useStampRalliesStore()
const game = useGameStore()
const { ready: markdownReady } = useMarkdown()

async function join(): Promise<void> {
  const details = await player.joinGame()
  if (details !== null && player.playerCard) {
    game.gameDetails = details
    void router.push({ name: 'player', params: { cardId: player.playerCard.id } })
  }
}

function viewRaffles(): void {
  raffles.raffles = raffles.homeRaffles
  void router.push({ name: 'raffles' })
}

function viewStampRallies(): void {
  void router.push({ name: 'stamp-rallies' })
}

function goCardRequests(): void {
  void router.push({ name: 'card-requests' })
}

function goAdminLogin(): void {
  void router.push({ name: 'admin-login' })
}

function onJoinInput(e: Event): void {
  player.joinId = (e.target as HTMLInputElement).value.toUpperCase()
}

// Focus the board-ID field on load so players can type their code immediately.
const joinInput = ref<HTMLInputElement | null>(null)
onMounted(() => {
  joinInput.value?.focus()
  // Decides whether the Stamp Rallies card is offered at all - the endpoint
  // returns only rallies open to public sign-up, so an empty list means there is
  // nothing to send anyone to.
  void stampRallies.loadSignupRallies()
})

// The logo (and the other brand images) are served at runtime from the web
// root's persistent `images/` folder - see vite.config.ts - not bundled. Bind
// the URL as a runtime string so the build never tries to resolve it as a
// module (a static `src="/images/logo.png"` makes Vite import it, which fails a
// clean build where public/images/ - gitignored - isn't present).
const logoUrl = '/images/logo.png'
</script>

<template>
  <div class="home">
    <!-- Masthead. The logo artwork includes the wordmark, so it stands in for the
         page title and `app_title` becomes its accessible name - the setting still
         drives the browser tab title and the player topbar. -->
    <header class="home-mast">
      <img :src="logoUrl" :alt="app.settings.app_title" class="home-logo" />
    </header>
    <!-- One column of distinct rows. The primary task leads and carries the
         weight (larger title, the only input, roughly double the height); the two
         destinations follow as compact full-width rows. Side-by-side columns
         squeezed the destination text into three wrapped lines beside a one-line
         button, and left a gap whenever the conditional Raffles row was absent. -->
    <div class="home-stack">
      <!-- Join game - the primary task. Its board-ID field is focused on mount. -->
      <div class="home-card home-card--primary">
        <h2><font-awesome-icon :icon="['fad', 'game-board-simple']" /> Join Bingo</h2>
        <!-- Admin-editable markdown prompt; plain-text fallback until parser loads -->
        <p v-if="!markdownReady">{{ app.settings.bingo_join_prompt }}</p>
        <MarkdownText v-else :source="app.settings.bingo_join_prompt" />
        <div class="field">
          <input
            ref="joinInput"
            v-model="player.joinId"
            placeholder="ABC123"
            aria-label="Board ID"
            maxlength="6"
            autocapitalize="characters"
            autocomplete="off"
            spellcheck="false"
            @keyup.enter="join"
            @input="onJoinInput"
          />
          <button
            class="btn-action"
            :disabled="player.joinId.length === 0 || player.joining"
            @click="join"
          >
            <LoadingSpinner v-if="player.joining" label="Joining..." />
            <template v-else>Join</template>
          </button>
        </div>
        <p v-if="player.joinError" class="error-msg">{{ player.joinError }}</p>
      </div>
      <!-- Raffles (only if open raffles exist) -->
      <div v-if="raffles.homeRaffles.length" class="home-card home-card--dest">
        <div class="home-dest-body">
          <h2><font-awesome-icon :icon="['fad', 'ticket']" /> Raffles</h2>
          <p>View currently open raffles and enter for a chance to win!</p>
        </div>
        <button class="btn-view" @click="viewRaffles">View Raffles</button>
      </div>
      <!-- Stamp Rallies (only when one is open to public sign-up) -->
      <div v-if="stampRallies.signupRallies.length" class="home-card home-card--dest">
        <div class="home-dest-body">
          <h2><font-awesome-icon :icon="['fad', 'stamp']" /> Stamp Rallies</h2>
          <p>Sign up for a stamp card and collect stamps from every stall!</p>
        </div>
        <button class="btn-view" @click="viewStampRallies">View Stamp Rallies</button>
      </div>
      <!-- Personal Card Requests -->
      <div class="home-card home-card--dest">
        <div class="home-dest-body">
          <h2><font-awesome-icon :icon="['fad', 'id-card']" /> Custom Card</h2>
          <p>Design your own bingo card and request it from Senpan staff.</p>
        </div>
        <button class="btn-view" @click="goCardRequests">Request a Card</button>
      </div>
    </div>
    <!-- Admin portal (separate) -->
    <div class="home-admin">
      <button class="btn-neutral btn-sm" @click="goAdminLogin">
        <font-awesome-icon :icon="['fas', 'lock']" /> Admin Portal
      </button>
    </div>
  </div>
</template>
