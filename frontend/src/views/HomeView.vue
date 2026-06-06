<script setup lang="ts">
/**
 * Home view — landing page with the app logo/title, the Join card, and an
 * optional Raffles card. Navigates via the router (`/play/:cardId`,
 * `/raffles`, `/admin/login`).
 */
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { usePlayerStore } from '@/stores/player'
import { useRafflesStore } from '@/stores/raffles'

const router = useRouter()
const app = useAppStore()
const player = usePlayerStore()
const raffles = useRafflesStore()
const game = useGameStore()

async function join(): Promise<void> {
  const details = await player.joinGame()
  if (details !== null && player.playerCard) {
    game.gameDetails = details
    router.push({ name: 'player', params: { cardId: player.playerCard.id } })
  }
}

function viewRaffles(): void {
  raffles.raffles = raffles.homeRaffles
  router.push({ name: 'raffles' })
}

function goAdminLogin(): void {
  router.push({ name: 'admin-login' })
}

function onJoinInput(e: Event): void {
  player.joinId = (e.target as HTMLInputElement).value.toUpperCase()
}
</script>

<template>
  <div class="home">
    <div class="home-brand">
      <img src="/images/logo.png" alt="Senpan Logo" class="home-logo" />
      <h1>{{ app.settings.app_title }}</h1>
    </div>
    <div class="home-cards">
      <!-- Join game -->
      <div class="home-card">
        <h2><i class="fa-solid fa-circle-dot"></i> Bingo</h2>
        <p>Enter your unique bingo board ID to play</p>
        <div class="field">
          <input
            v-model="player.joinId"
            placeholder="ABC123"
            aria-label="Board ID"
            maxlength="6"
            @keyup.enter="join"
            @input="onJoinInput"
          />
        </div>
        <button class="btn-primary" :disabled="player.joinId.length === 0" @click="join">
          Join
        </button>
        <p v-if="player.joinError" class="error-msg">{{ player.joinError }}</p>
      </div>
      <!-- Raffles (only if open raffles exist) -->
      <div v-if="raffles.homeRaffles.length" class="home-card">
        <h2><i class="fa-solid fa-ticket"></i> Raffles</h2>
        <p>View currently open raffles and enter for a chance to win!</p>
        <button class="btn-primary" @click="viewRaffles">View Raffles</button>
      </div>
    </div>
    <!-- Admin portal (separate) -->
    <div class="home-admin">
      <button class="btn-ghost btn-sm" @click="goAdminLogin">
        <i class="fa-solid fa-lock"></i> Admin Portal
      </button>
    </div>
  </div>
</template>
