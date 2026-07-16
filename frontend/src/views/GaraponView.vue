<script setup lang="ts">
/**
 * Public Garapon player view (reached via a per-player drawing link, /garapon/:token).
 *
 * Shows the player their name (to confirm it's their link), the drum wheel (when
 * the garapon is open and they have draws left), the grand-prize showcase, the
 * other prizes, the event details, and their own draw record. Spinning the drum
 * performs an authoritative draw; the win reveals as the ball lands. After the
 * garapon closes — or once draws run out — the wheel disappears and the page is a
 * read-only record.
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import BallSwatch from '@/components/common/ui/BallSwatch.vue'
import GaraponWheel from '@/components/player/GaraponWheel.vue'
import { useGaraponsStore } from '@/stores/garapons'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import type { GaraponDrawResponse } from '@/types/api'

const props = defineProps<{ token: string }>()

const garapons = useGaraponsStore()

const notFound = ref(false)

const isClosed = computed(() => garapons.publicGarapon?.status === 'closed')

async function load(token: string): Promise<void> {
  notFound.value = false
  const ok = await garapons.loadByToken(token)
  if (!ok) notFound.value = true
}

onMounted(() => load(props.token))
watch(
  () => props.token,
  (t) => load(t),
)

/** Called by the wheel when the player taps it: clears the prior win + draws. */
function onDraw(): Promise<GaraponDrawResponse | null> {
  garapons.lastWin = null
  return garapons.draw(props.token)
}

/** Called by the wheel once the ball lands: commit the win to the record. */
function onSettled(resp: GaraponDrawResponse): void {
  garapons.commitDraw(resp)
}

function when(ts: string): string {
  return ts ? formatServerTimestamp(ts) : ''
}
</script>

<template>
  <div v-if="garapons.publicGarapon">
    <div class="topbar">
      <span></span>
      <h2>{{ garapons.publicGarapon.title }}</h2>
      <span></span>
    </div>

    <div class="tab-body garapon-body">
      <!-- Whose link this is -->
      <p v-if="garapons.publicPlayer" class="garapon-player-line">
        <font-awesome-icon :icon="['fad', 'user']" /> Drawing for
        <strong>{{ garapons.publicPlayer.player_name }}</strong>
      </p>

      <!-- Linked Stamp Rally card: shown only when this drawing link was issued
           alongside a paired rally card (same token). -->
      <router-link
        v-if="garapons.publicStampCardToken"
        :to="{ name: 'stamp-card', params: { token: garapons.publicStampCardToken } }"
        class="garapon-rally-link"
      >
        <font-awesome-icon :icon="['fad', 'stamp']" />
        <span>View your Stamp Rally card</span>
      </router-link>

      <!-- Status / remaining draws -->
      <p class="garapon-draws-line">
        <template v-if="isClosed">
          <font-awesome-icon :icon="['fad', 'lock']" /> This garapon has closed.
        </template>
        <template v-else-if="garapons.drawsRemaining > 0">
          <strong>{{ garapons.drawsRemaining }}</strong> of
          {{ garapons.publicPlayer?.max_draws }} draw{{
            garapons.publicPlayer?.max_draws === 1 ? '' : 's'
          }}
          remaining
        </template>
        <template v-else>
          <font-awesome-icon :icon="['fad', 'circle-check']" /> You've used all your draws.
        </template>
      </p>

      <!-- Congratulations banner -->
      <div v-if="garapons.lastWin" class="garapon-win-banner">
        <BallSwatch :color="garapons.lastWin.ball_color" />
        🎉 Congratulations, you've won <strong>{{ garapons.lastWin.prize_name }}</strong
        >!
      </div>

      <!-- The drum (open + has draws) -->
      <div v-if="!isClosed" class="garapon-wheel-wrap">
        <GaraponWheel :disabled="!garapons.canDraw" :perform-draw="onDraw" @settled="onSettled" />
      </div>

      <!-- Grand prize showcase -->
      <div v-if="garapons.grandPrize" class="garapon-grand">
        <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'trophy']" /> Grand Prize</h3>
        <div class="garapon-grand-card">
          <img
            v-if="garapons.publicGarapon.grand_prize_image"
            :src="assetUrl(garapons.publicGarapon.grand_prize_image)"
            class="garapon-grand-img"
            alt="Grand prize"
          />
          <div class="garapon-grand-meta">
            <BallSwatch :color="garapons.grandPrize.ball_color" size="lg" />
            <span class="garapon-grand-name">{{ garapons.grandPrize.name }}</span>
          </div>
        </div>
      </div>

      <!-- Details (markdown) -->
      <MarkdownText
        v-if="garapons.publicGarapon.details"
        class="game-details mb-16"
        :source="garapons.publicGarapon.details"
      />

      <!-- Other prizes -->
      <div v-if="garapons.otherPrizes.length" class="mb-16">
        <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'gift']" /> Other Prizes</h3>
        <table class="data-table garapon-prize-table">
          <thead>
            <tr>
              <th class="ta-center">Ball</th>
              <th>Prize</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in garapons.otherPrizes" :key="p.id">
              <td class="ta-center">
                <BallSwatch :color="p.ball_color" />
              </td>
              <td>{{ p.name }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Player's record -->
      <div v-if="garapons.publicDraws.length" class="mb-16">
        <h3 class="section-heading">
          <font-awesome-icon :icon="['fad', 'clipboard-list']" /> Your Prizes
        </h3>
        <table class="data-table garapon-prize-table">
          <thead>
            <tr>
              <th class="ta-center">Ball</th>
              <th>Prize</th>
              <th class="ta-right">When</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="d in garapons.publicDraws" :key="d.id">
              <td class="ta-center">
                <BallSwatch :color="d.ball_color" />
              </td>
              <td>{{ d.prize_name }}</td>
              <td class="ta-right text-sm text-dim">{{ when(d.drawn_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <!-- Not found -->
  <div v-else-if="notFound" class="tab-body">
    <p class="garapon-notfound text-dim">
      <font-awesome-icon :icon="['fad', 'ferris-wheel']" /> This drawing link is invalid or has been
      removed.
    </p>
  </div>

  <!-- Loading -->
  <div v-else-if="garapons.publicLoading" class="tab-body">
    <LoadingSpinner block label="Loading garapon…" />
  </div>
</template>

<style scoped>
.garapon-body {
  max-width: 640px;
  margin: 0 auto;
}
.garapon-player-line,
.garapon-draws-line {
  text-align: center;
  margin-bottom: 6px;
}
.garapon-draws-line {
  color: var(--highlight);
  font-weight: 600;
  margin-bottom: 16px;
}
.garapon-rally-link {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  max-width: 360px;
  margin: 0 auto 16px;
  padding: 10px 16px;
  background: color-mix(in srgb, var(--accent-2) 16%, transparent);
  border: 1px solid var(--accent-2);
  border-radius: var(--radius);
  color: var(--text);
  font-weight: 600;
  text-decoration: none;
  transition: background 0.15s ease;
}
.garapon-rally-link:hover {
  background: color-mix(in srgb, var(--accent-2) 28%, transparent);
}
.garapon-wheel-wrap {
  margin: 8px 0 20px;
}
.garapon-win-banner {
  text-align: center;
  background: color-mix(in srgb, var(--highlight) 16%, transparent);
  border: 1px solid var(--highlight);
  border-radius: var(--radius);
  padding: 12px 16px;
  margin-bottom: 16px;
  font-size: 1.05rem;
}
.garapon-grand {
  margin-bottom: 16px;
}
.garapon-grand-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  background: var(--panel-raised-bg);
  border: 1px solid var(--control-border);
  border-radius: 12px;
  padding: 16px;
}
.garapon-grand-img {
  max-width: 100%;
  max-height: 320px;
  border-radius: 8px;
  box-shadow: 0 4px 20px var(--shadow);
}
.garapon-grand-meta {
  display: flex;
  align-items: center;
  gap: 10px;
}
.garapon-grand-name {
  font-size: 1.2rem;
  font-weight: 700;
  color: var(--highlight);
}
.garapon-prize-table {
  width: 100%;
}
.garapon-notfound {
  text-align: center;
  padding: 40px 20px;
  font-size: 1.1rem;
}
</style>
