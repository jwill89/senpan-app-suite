<script setup lang="ts">
/**
 * A participant's Stamp Rally card: the card with stamps rendered at their
 * positions - the real stamp art once collected, the "not stamped" placeholder
 * until then - a password field to collect a stamp (the server checks the stall
 * is open), progress against whatever the rally counts as complete (the whole
 * card, or so many food stamps and so many game stamps), and, once complete, the
 * revealed prizes with redemption instructions.
 *
 * It renders the stamp-rally store's loaded public card, so the card page
 * (views/StampCardView) and the festival dashboard show one and the same thing.
 */
import { computed, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import StampCardCanvas, { type CanvasItem } from '@/components/common/ui/StampCardCanvas.vue'
import { useStampRalliesStore } from '@/stores/stampRallies'
import { assetUrl } from '@/lib/assets'
import { stallName, stampTypeShort } from '@/lib/stampcard'
import { formatServerTimestamp } from '@/lib/datetime'
import type { PublicStamp, PublicStampCard } from '@/types/api'

const props = withDefaults(
  defineProps<{
    /** The card's token: the capability the stamp endpoint is called with. */
    token: string
    /** Show the rally's own description. The dashboard leaves it out: the
     *  festival page around it already says what the festival is. */
    showDetails?: boolean
  }>(),
  { showDetails: true },
)

const store = useStampRalliesStore()

const password = ref('')

/** The loaded card. The panel is only rendered once there is one. */
const card = computed<PublicStampCard | null>(() => store.publicCard)

/** Canvas items: collected stamps show their art (others the placeholder); prizes
 * reveal only once the card is complete. */
const items = computed<CanvasItem[]>(() => {
  const c = card.value
  if (!c) return []
  const stamps = c.stamps.map((s) => ({
    key: `s${s.id}`,
    // Collected -> the stamp art; otherwise the optional not-stamped overlay ('' ->
    // nothing, so a card with its own slot placeholders shows through).
    image: s.collected ? s.image : c.rally.not_stamped_image,
    placement: s.placement,
  }))
  const prizes = c.prizes.map((p) => ({
    key: `p${p.id}`,
    // Revealed only once the card is complete; otherwise the not-stamped overlay.
    image: c.prizes_revealed ? p.image : c.rally.not_stamped_image,
    placement: p.placement,
  }))
  return [...stamps, ...prizes]
})

const progress = computed(() => store.cardProgress)

/**
 * The progress tallies to show, each as a collected count plus the rest of its
 * phrase. A rally that counts types shows one part per type it actually asks for
 * (a type it requires none of isn't progress); anything else - including a
 * "counts" rally that requires nothing - shows the whole card as one tally.
 */
const progressParts = computed<{ have: number; label: string }[]>(() => {
  const p = progress.value
  // The count shown is capped at what the tally asks for: collecting 2 of 5 food
  // stalls on a card that needs 1 is progress of 1 of 1, not "2 of 1".
  const part = (have: number, needed: number, kind = '') => ({
    have: Math.min(have, needed),
    label: `of ${needed} ${kind}stamp${needed === 1 ? '' : 's'}`,
  })
  const parts: { have: number; label: string }[] = []
  if (p.byType) {
    if (p.food.required > 0) parts.push(part(p.food.collected, p.food.required, 'food '))
    if (p.game.required > 0) parts.push(part(p.game.collected, p.game.required, 'game '))
  }
  return parts.length ? parts : [part(p.collected, p.total)]
})

/**
 * True when a per-type requirement can no longer be met, because the stalls that
 * would have satisfied it have closed for good. Nothing the participant does can
 * finish the card from here, so the panel says so instead of leaving them to keep
 * trying passwords.
 */
const unreachable = computed(
  () =>
    progress.value.byType && (progress.value.food.unreachable || progress.value.game.unreachable),
)

/**
 * A "counts" card can be complete with stalls left over, so keep the password
 * field for any that are still open - a finished participant can carry on
 * collecting the optional stamps. An "all" card has nothing left by definition.
 */
const canCollectMore = computed(
  () =>
    progress.value.byType && (card.value?.stamps.some((s) => !s.collected && s.available) ?? false),
)

function stampStatus(s: PublicStamp): { label: string; modifier: string } {
  if (s.collected) return { label: 'Collected', modifier: 'stamp-stall-status--collected' }
  if (s.available) return { label: 'Open now', modifier: 'stamp-stall-status--open' }
  return { label: 'Closed', modifier: 'stamp-stall-status--closed' }
}

function windowText(s: PublicStamp): string {
  if (!s.active_from && !s.active_to) return ''
  const from = s.active_from ? formatServerTimestamp(s.active_from) : '...'
  const to = s.active_to ? formatServerTimestamp(s.active_to) : '...'
  return `${from} - ${to}`
}

async function submit(): Promise<void> {
  if (await store.submitPassword(props.token, password.value)) password.value = ''
}
</script>

<template>
  <div v-if="card">
    <p class="lead-line">
      <font-awesome-icon :icon="['fad', 'id-card']" /> Card for
      <strong>{{ card.participant_name }}</strong>
    </p>

    <!-- Completion banner -->
    <div v-if="card.completed" class="callout">
      <font-awesome-icon :icon="['fad', 'champagne-glasses']" /> Your card is complete! Your prizes
      are revealed below.<template v-if="canCollectMore">
        You can still collect the stamps you haven't picked up yet.</template
      >
    </div>
    <!-- Progress: one tally per required type when the rally counts types,
         otherwise the whole card. -->
    <p v-else class="lead-line lead-line--tally">
      <template v-for="(part, i) in progressParts" :key="part.label"
        ><template v-if="i > 0"> - </template><strong>{{ part.have }}</strong>
        {{ part.label }}</template
      >
      collected
    </p>

    <!-- A requirement that closed stalls put out of reach -->
    <p v-if="!card.completed && unreachable" class="callout callout--warning">
      <font-awesome-icon :icon="['fad', 'triangle-exclamation']" /> The stalls this card still needs
      have closed, so it can't be finished now. Show a staff member what you collected.
    </p>

    <!-- The card -->
    <div class="stamp-canvas-wrap">
      <StampCardCanvas :card-image="card.rally.card_image" :items="items" />
    </div>

    <!-- Password entry (until complete, or while optional stalls remain) -->
    <form v-if="!card.completed || canCollectMore" class="stamp-entry" @submit.prevent="submit">
      <input
        v-model="password"
        class="stamp-entry-input"
        placeholder="Enter a stamp password"
        aria-label="Stamp password"
        autocomplete="off"
        :disabled="store.submitting"
      />
      <button class="btn-confirm" type="submit" :disabled="store.submitting || !password.trim()">
        <LoadingSpinner v-if="store.submitting" label="Stamping..." />
        <template v-else><font-awesome-icon :icon="['fad', 'stamp']" /> Stamp</template>
      </button>
    </form>

    <!-- Details (markdown) -->
    <MarkdownText
      v-if="showDetails && card.rally.details"
      class="game-details mb-16"
      :source="card.rally.details"
    />

    <!-- Stalls + availability -->
    <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'stamp']" /> Stalls</h3>
    <ul class="list-stack mb-16">
      <li v-for="s in card.stamps" :key="s.id" class="stamp-stall">
        <span class="stamp-stall-name">{{ stallName(s.affiliate_name) }}</span>
        <span class="badge badge--muted">{{ stampTypeShort(s.stamp_type) }}</span>
        <span v-if="windowText(s)" class="stamp-stall-window text-muted text-xs">{{
          windowText(s)
        }}</span>
        <span :class="['status-badge', 'stamp-stall-status', stampStatus(s).modifier]">
          <font-awesome-icon v-if="s.collected" :icon="['fas', 'circle-check']" />
          {{ stampStatus(s).label }}
        </span>
      </li>
    </ul>

    <!-- Prizes + redeem instructions (once complete) -->
    <template v-if="card.completed">
      <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'gift']" /> Your Prizes</h3>
      <div v-if="card.prizes.length" class="stamp-prizes mb-16">
        <div v-for="p in card.prizes" :key="p.id" class="stamp-prize">
          <img v-if="p.image" :src="assetUrl(p.image)" alt="" />
          <span>{{ p.name }}</span>
        </div>
      </div>
      <MarkdownText
        v-if="card.rally.redeem_instructions"
        class="game-details"
        :source="card.rally.redeem_instructions"
      />
      <figure v-if="card.rally.redeem_image" class="captioned-figure">
        <figcaption>Where to redeem</figcaption>
        <img :src="assetUrl(card.rally.redeem_image)" alt="Where to redeem your card" />
      </figure>
    </template>
  </div>
</template>
