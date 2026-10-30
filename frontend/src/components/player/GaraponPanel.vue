<script setup lang="ts">
/**
 * A player's Garapon: their remaining draws, the drum wheel (while the garapon is
 * open), the grand-prize showcase, the other prizes, the event details, and their
 * own draw record. Spinning the drum performs an authoritative draw; the win
 * reveals as the ball lands. After the garapon closes - or once draws run out -
 * the wheel disappears and the panel is a read-only record.
 *
 * It renders the garapon store's loaded public state, so the drawing page
 * (views/GaraponView) and the festival dashboard show one and the same drum. The
 * dashboard may load a drum with no player - the visitor has no drawing link on
 * this device - and then the panel is the prize list alone.
 */
import { computed } from 'vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import BallSwatch from '@/components/common/ui/BallSwatch.vue'
import GaraponWheel from '@/components/player/GaraponWheel.vue'
import { useGaraponsStore } from '@/stores/garapons'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import type { GaraponDrawResponse } from '@/types/api'

const props = withDefaults(
  defineProps<{
    /** The drawing token: the capability the draw endpoint is called with. */
    token: string
    /** Show the garapon's own description; the dashboard leaves it out. */
    showDetails?: boolean
  }>(),
  { showDetails: true },
)

const emit = defineEmits<{
  /** A draw landed and was committed to the record. */
  drawn: [resp: GaraponDrawResponse]
}>()

const garapons = useGaraponsStore()

const isClosed = computed(() => garapons.publicGarapon?.status === 'closed')

/** Called by the wheel when the player taps it: clears the prior win + draws. */
function onDraw(): Promise<GaraponDrawResponse | null> {
  garapons.lastWin = null
  return garapons.draw(props.token)
}

/** Called by the wheel once the ball lands: commit the win to the record. */
function onSettled(resp: GaraponDrawResponse): void {
  garapons.commitDraw(resp)
  emit('drawn', resp)
}

function when(ts: string): string {
  return ts ? formatServerTimestamp(ts) : ''
}
</script>

<template>
  <div v-if="garapons.publicGarapon">
    <!-- Whose link this is -->
    <p v-if="garapons.publicPlayer" class="lead-line">
      <font-awesome-icon :icon="['fad', 'user']" /> Drawing for
      <strong>{{ garapons.publicPlayer.player_name }}</strong>
    </p>

    <!-- Status / remaining draws. With no player there is no allowance to state:
         the dashboard shows a drum before the visitor has a link for it. -->
    <p v-if="garapons.publicPlayer || isClosed" class="lead-line lead-line--tally">
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
    <div v-if="garapons.lastWin" class="callout">
      <BallSwatch :color="garapons.lastWin.ball_color" />
      <font-awesome-icon :icon="['fad', 'champagne-glasses']" /> Congratulations, you've won
      <strong>{{ garapons.lastWin.prize_name }}</strong
      >!
    </div>

    <!-- The drum (open + a link to draw with) -->
    <div v-if="!isClosed && garapons.publicPlayer" class="garapon-wheel-wrap">
      <GaraponWheel :disabled="!garapons.canDraw" :perform-draw="onDraw" @settled="onSettled" />
    </div>

    <!-- Grand prize showcase -->
    <div v-if="garapons.grandPrize" class="mb-16">
      <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'trophy']" /> Grand Prize</h3>
      <div class="garapon-grand">
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
      v-if="showDetails && garapons.publicGarapon.details"
      class="game-details mb-16"
      :source="garapons.publicGarapon.details"
    />

    <!-- Other prizes -->
    <div v-if="garapons.otherPrizes.length" class="mb-16">
      <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'gift']" /> Other Prizes</h3>
      <table class="data-table w-full">
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
      <table class="data-table w-full">
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
            <td class="ta-right text-sm text-muted">{{ when(d.drawn_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
