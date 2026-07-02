<script setup lang="ts">
/**
 * Home view — landing page with the app logo/title, the Join card, and an
 * optional Raffles card. Navigates via the router (`/play/:cardId`,
 * `/raffles`, `/admin/login`).
 */
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useMarkdown } from '@/lib/markdown'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { usePlayerStore } from '@/stores/player'
import { useRafflesStore } from '@/stores/raffles'

const router = useRouter()
const app = useAppStore()
const player = usePlayerStore()
const raffles = useRafflesStore()
const game = useGameStore()
const { render: renderMarkdown, ready: markdownReady } = useMarkdown()

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

function goAdminLogin(): void {
  void router.push({ name: 'admin-login' })
}

function onJoinInput(e: Event): void {
  player.joinId = (e.target as HTMLInputElement).value.toUpperCase()
}

// Focus the board-ID field on load so players can type their code immediately.
const joinInput = ref<HTMLInputElement | null>(null)
onMounted(() => joinInput.value?.focus())

// The logo (and the other brand images) are served at runtime from the web
// root's persistent `images/` folder — see vite.config.ts — not bundled. Bind
// the URL as a runtime string so the build never tries to resolve it as a
// module (a static `src="/images/logo.png"` makes Vite import it, which fails a
// clean build where public/images/ — gitignored — isn't present).
const logoUrl = '/images/logo.png'
</script>

<template>
  <div class="home">
    <div class="home-brand">
      <img :src="logoUrl" alt="Senpan Logo" class="home-logo" />
      <h1>{{ app.settings.app_title }}</h1>
    </div>
    <div class="home-cards">
      <!-- Join game -->
      <div class="home-card">
        <h2><font-awesome-icon :icon="['fad', 'game-board-simple']" /> Bingo</h2>
        <!-- Admin-editable markdown prompt; plain-text fallback until parser loads -->
        <p v-if="!markdownReady">{{ app.settings.bingo_join_prompt }}</p>
        <div
          v-else
          class="home-card-prompt"
          v-html="renderMarkdown(app.settings.bingo_join_prompt)"
        ></div>
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
        </div>
        <button
          class="btn-action"
          :disabled="player.joinId.length === 0 || player.joining"
          @click="join"
        >
          <LoadingSpinner v-if="player.joining" label="Joining…" />
          <template v-else>Join</template>
        </button>
        <p v-if="player.joinError" class="error-msg">{{ player.joinError }}</p>
      </div>
      <!-- Raffles (only if open raffles exist) -->
      <div v-if="raffles.homeRaffles.length" class="home-card">
        <h2><font-awesome-icon :icon="['fad', 'ticket']" /> Raffles</h2>
        <p>View currently open raffles and enter for a chance to win!</p>
        <button class="btn-view" @click="viewRaffles">View Raffles</button>
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
