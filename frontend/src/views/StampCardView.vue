<script setup lang="ts">
/**
 * Public Stamp Rally card (reached via a per-participant card link, /stamp-card/:token).
 *
 * Shows the participant their card with stamps rendered at their positions — the real
 * stamp art once collected, the "not stamped" placeholder until then — a password
 * field to collect a stamp (the server checks the stall is open), progress, and, once
 * every still-collectable stamp is accounted for, the revealed prizes with redemption
 * instructions.
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import StampCardCanvas, { type CanvasItem } from '@/components/common/ui/StampCardCanvas.vue'
import { useStampRalliesStore } from '@/stores/stampRallies'
import { assetUrl } from '@/lib/assets'
import { stallName } from '@/lib/stampcard'
import { formatServerTimestamp } from '@/lib/datetime'
import type { PublicStamp } from '@/types/api'

const props = defineProps<{ token: string }>()

const store = useStampRalliesStore()

const notFound = ref(false)
const password = ref('')

async function load(token: string): Promise<void> {
  notFound.value = false
  const ok = await store.loadByToken(token)
  if (!ok) notFound.value = true
}

onMounted(() => load(props.token))
watch(
  () => props.token,
  (t) => load(t),
)

/** Canvas items: collected stamps show their art (others the placeholder); prizes
 * reveal only once the card is complete. */
const items = computed<CanvasItem[]>(() => {
  const c = store.publicCard
  if (!c) return []
  const stamps = c.stamps.map((s) => ({
    key: `s${s.id}`,
    // Collected → the stamp art; otherwise the optional not-stamped overlay ('' →
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

const collectedCount = computed(
  () => store.publicCard?.stamps.filter((s) => s.collected).length ?? 0,
)
const totalStamps = computed(() => store.publicCard?.stamps.length ?? 0)

function stampStatus(s: PublicStamp): { label: string; cls: string } {
  if (s.collected) return { label: 'Collected', cls: 'ok' }
  if (s.available) return { label: 'Open now', cls: 'open' }
  return { label: 'Closed', cls: 'closed' }
}

function windowText(s: PublicStamp): string {
  if (!s.active_from && !s.active_to) return ''
  const from = s.active_from ? formatServerTimestamp(s.active_from) : '…'
  const to = s.active_to ? formatServerTimestamp(s.active_to) : '…'
  return `${from} – ${to}`
}

async function submit(): Promise<void> {
  if (await store.submitPassword(props.token, password.value)) password.value = ''
}
</script>

<template>
  <div v-if="store.publicCard">
    <div class="topbar">
      <span></span>
      <h2>{{ store.publicCard.rally.title }}</h2>
      <span></span>
    </div>

    <div class="tab-body stamp-body">
      <p class="stamp-participant">
        <font-awesome-icon :icon="['fad', 'id-card']" /> Card for
        <strong>{{ store.publicCard.participant_name }}</strong>
      </p>

      <!-- Completion banner -->
      <div v-if="store.publicCard.completed" class="stamp-complete-banner">
        🎉 Your card is complete! Your prizes are revealed below.
      </div>
      <p v-else class="stamp-progress">
        <strong>{{ collectedCount }}</strong> of {{ totalStamps }} stamp{{
          totalStamps === 1 ? '' : 's'
        }}
        collected
      </p>

      <!-- The card -->
      <div class="stamp-canvas-wrap">
        <StampCardCanvas :card-image="store.publicCard.rally.card_image" :items="items" />
      </div>

      <!-- Password entry (until complete) -->
      <form v-if="!store.publicCard.completed" class="stamp-entry" @submit.prevent="submit">
        <input
          v-model="password"
          class="stamp-password"
          placeholder="Enter a stamp password"
          aria-label="Stamp password"
          :disabled="store.submitting"
        />
        <button class="btn-confirm" type="submit" :disabled="store.submitting || !password.trim()">
          <LoadingSpinner v-if="store.submitting" label="Stamping…" />
          <template v-else><font-awesome-icon :icon="['fad', 'stamp']" /> Stamp</template>
        </button>
      </form>

      <!-- Details (markdown) -->
      <MarkdownText
        v-if="store.publicCard.rally.details"
        class="game-details mb-16"
        :source="store.publicCard.rally.details"
      />

      <!-- Stalls + availability -->
      <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'stamp']" /> Stalls</h3>
      <ul class="stamp-stall-list mb-16">
        <li v-for="s in store.publicCard.stamps" :key="s.id" class="stamp-stall">
          <span class="stamp-stall-name">{{ stallName(s.affiliate_name) }}</span>
          <span v-if="windowText(s)" class="stamp-stall-window text-dim text-xs">{{
            windowText(s)
          }}</span>
          <span :class="['status-badge', `stall-${stampStatus(s).cls}`]">{{
            stampStatus(s).label
          }}</span>
        </li>
      </ul>

      <!-- Prizes + redeem instructions (once complete) -->
      <template v-if="store.publicCard.completed">
        <h3 class="section-heading"><font-awesome-icon :icon="['fad', 'gift']" /> Your Prizes</h3>
        <div v-if="store.publicCard.prizes.length" class="stamp-prizes mb-16">
          <div v-for="p in store.publicCard.prizes" :key="p.id" class="stamp-prize">
            <img v-if="p.image" :src="assetUrl(p.image)" alt="" />
            <span>{{ p.name }}</span>
          </div>
        </div>
        <MarkdownText
          v-if="store.publicCard.rally.redeem_instructions"
          class="game-details redeem"
          :source="store.publicCard.rally.redeem_instructions"
        />
      </template>
    </div>
  </div>

  <!-- Not found -->
  <div v-else-if="notFound" class="tab-body">
    <p class="stamp-notfound text-dim">
      <font-awesome-icon :icon="['fad', 'stamp']" /> This stamp card link is invalid or has been
      removed.
    </p>
  </div>

  <!-- Loading -->
  <div v-else-if="store.publicLoading" class="tab-body">
    <LoadingSpinner block label="Loading stamp card…" />
  </div>
</template>

<style scoped>
.stamp-body {
  max-width: 640px;
  margin: 0 auto;
}
.stamp-participant {
  text-align: center;
  margin-bottom: 6px;
}
.stamp-progress {
  text-align: center;
  color: var(--highlight);
  font-weight: 600;
  margin-bottom: 16px;
}
.stamp-complete-banner {
  text-align: center;
  background: color-mix(in srgb, var(--highlight) 16%, transparent);
  border: 1px solid var(--highlight);
  border-radius: var(--radius);
  padding: 12px 16px;
  margin-bottom: 16px;
  font-size: 1.05rem;
}
.stamp-canvas-wrap {
  margin: 8px auto 16px;
}
.stamp-entry {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}
.stamp-password {
  flex: 1;
}
.stamp-stall-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.stamp-stall {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: var(--panel-raised-bg);
  border-radius: var(--radius);
}
.stamp-stall-name {
  font-weight: 600;
}
.stamp-stall-window {
  margin-left: auto;
}
.stamp-stall .status-badge {
  margin-left: 8px;
}
.stall-ok {
  background: var(--success, #2cb67d);
  color: #fff;
}
.stall-open {
  background: var(--highlight);
  color: #fff;
}
.stall-closed {
  background: var(--control-border);
  color: var(--text-dim);
}
.stamp-prizes {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.stamp-prize {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  width: 120px;
  text-align: center;
}
.stamp-prize img {
  width: 100%;
  height: 110px;
  object-fit: contain;
  border-radius: 8px;
  background: var(--panel-raised-bg);
}
.stamp-notfound {
  text-align: center;
  padding: 40px 20px;
  font-size: 1.1rem;
}
.stamp-stall-window + .status-badge {
  margin-left: 0;
}
</style>
