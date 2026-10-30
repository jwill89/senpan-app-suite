<script setup lang="ts">
/**
 * The festival dashboard (/festival/:id): one page where a visitor follows and
 * takes part in everything filed under a festival - the plan, the stamp rally,
 * the garapon and the raffles - without being sent off to a page for each.
 *
 * A wide screen shows every section at once: the plan across the top, the rest in
 * columns under it. A phone shows one at a time, picked from the tiles, which
 * double as the at-a-glance summary on both - stamps collected, draws left,
 * raffles entered.
 *
 * The state and every action live in the festival-dashboard store. The stamp card
 * and the drum are the same panels their own pages render.
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useTemplateRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import FestivalMapWidget from '@/components/common/ui/FestivalMapWidget.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import WorldPicker from '@/components/common/ui/WorldPicker.vue'
import FestivalRaffleCard from '@/components/player/FestivalRaffleCard.vue'
import GaraponPanel from '@/components/player/GaraponPanel.vue'
import StampCardPanel from '@/components/player/StampCardPanel.vue'
import { useFestivalDashboardStore } from '@/stores/festivalDashboard'
import { useGaraponsStore } from '@/stores/garapons'
import { useStampRalliesStore } from '@/stores/stampRallies'
import { endpoints } from '@/lib/endpoints'
import { formatEventTime } from '@/lib/festivalmap'
import { participantLabel } from '@/lib/participant'

// The route param is the festival's shortcode OR its numeric id - the server
// resolves either, so it is passed through as typed.
const props = defineProps<{ id: string }>()

type Section = 'map' | 'stamps' | 'garapon' | 'raffles'

const router = useRouter()
const store = useFestivalDashboardStore()
const stampRallies = useStampRalliesStore()
const garapons = useGaraponsStore()

const turnstileSiteKey = ref('')
/** The section a phone shows; on a wide screen, the one last jumped to. */
const current = ref<Section>('map')

onMounted(async () => {
  void store.open(props.id)
  try {
    turnstileSiteKey.value = (await endpoints.system.config()).turnstile_site_key
  } catch {
    turnstileSiteKey.value = '' // config probe failed -> behave as if disabled
  }
})
// An in-app link from one festival to another reuses this component.
watch(
  () => props.id,
  (id) => {
    current.value = 'map'
    void store.open(id)
  },
)

/**
 * Coming back to the tab re-reads the visitor's standing, so a stall that opened
 * or a raffle that was drawn while they were away shows without a reload. Not
 * mid-spin: the drum commits its draw when the ball lands, and a refresh before
 * then would show the prize first.
 */
function onVisible(): void {
  if (document.visibilityState !== 'visible' || garapons.drawing) return
  if (Date.now() - store.loadedAt > 60_000) void store.refresh()
}
onMounted(() => document.addEventListener('visibilitychange', onVisible))
onBeforeUnmount(() => document.removeEventListener('visibilitychange', onVisible))

const data = computed(() => store.dashboard)
const rally = computed(() => data.value?.rally ?? null)
const garapon = computed(() => data.value?.garapon ?? null)
const raffles = computed(() =>
  // Running ones first: they are the ones a visitor can still do something about.
  [...(data.value?.raffles ?? [])].sort((a, b) => Number(b.running) - Number(a.running)),
)

// -- The tiles ------------------------------------------------------------------

const stampTally = computed(() => {
  const p = stampRallies.cardProgress
  if (!store.card) return rally.value?.signup_open ? 'Get a card' : 'No card yet'
  if (p.byType) {
    const need = p.food.required + p.game.required
    const have =
      Math.min(p.food.collected, p.food.required) + Math.min(p.game.collected, p.game.required)
    return store.card.completed ? 'Complete' : `${have} of ${need}`
  }
  return store.card.completed ? 'Complete' : `${p.collected} of ${p.total}`
})

const drawTally = computed(() => {
  const g = garapon.value
  if (!g) return ''
  if (g.garapon.status === 'closed') return 'Closed'
  if (g.player) return `${garapons.drawsRemaining} left`
  if (g.has_paired_link) return `${g.paired_draws_left} waiting`
  return 'With a card'
})

const raffleTally = computed(() => {
  const mine = store.enteredRaffles.length
  if (mine) return `In ${mine}`
  const open = store.runningRaffles.length
  return open ? `${open} open` : 'Drawn'
})

const tiles = computed(() => {
  const list: { key: Section; label: string; icon: string; value: string }[] = [
    {
      key: 'map',
      label: 'Map',
      icon: 'map-location-dot',
      value: `${store.map?.stalls.length ?? 0} stalls`,
    },
  ]
  if (rally.value)
    list.push({ key: 'stamps', label: 'Stamp Rally', icon: 'stamp', value: stampTally.value })
  if (garapon.value)
    list.push({ key: 'garapon', label: 'Garapon', icon: 'ferris-wheel', value: drawTally.value })
  if (raffles.value.length)
    list.push({ key: 'raffles', label: 'Raffles', icon: 'ticket', value: raffleTally.value })
  return list
})

/** Shows a section (a phone) or brings it into view (a wide screen). */
async function goTo(section: Section, anchor?: string): Promise<void> {
  current.value = section
  await nextTick()
  document
    .getElementById(anchor ?? `festival-${section}`)
    ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

// -- Who is visiting --------------------------------------------------------------

const editingVisitor = ref(false)
const visitorName = ref('')
const visitorWorld = ref('')

const visitorLabel = computed(() =>
  store.visitorName ? participantLabel(store.visitorName, store.visitorWorld) : '',
)

function editVisitor(): void {
  visitorName.value = store.visitorName
  visitorWorld.value = store.visitorWorld
  editingVisitor.value = true
}

async function saveVisitor(): Promise<void> {
  if (await store.setVisitor(visitorName.value, visitorWorld.value)) editingVisitor.value = false
}

// -- Stamp rally: sign up / find a card / stamp from the plan ----------------------

const signupName = ref('')
const signupWorld = ref('')
const signupTurnstile = ref('')
const signupWidget = useTemplateRef<InstanceType<typeof TurnstileWidget>>('signupWidget')

// The sign-up form starts from whoever the visitor said they are.
watch(
  () => [store.visitorName, store.visitorWorld] as const,
  ([name, world]) => {
    if (!signupName.value) signupName.value = name
    if (!signupWorld.value) signupWorld.value = world
  },
  { immediate: true },
)

async function signUp(): Promise<void> {
  const ok = await store.signUp(signupName.value, signupWorld.value, signupTurnstile.value)
  if (!ok) {
    // Single-use: a refused attempt (a taken name, most often) needs a fresh one.
    signupTurnstile.value = ''
    signupWidget.value?.reset()
  }
}

function findCard(): void {
  void store.findCard(signupName.value, signupWorld.value)
}

/** One code box for the stall panel: only one stall's panel is open at a time. */
const stallCode = ref('')
async function stampFromStall(): Promise<void> {
  if (await store.stamp(stallCode.value)) stallCode.value = ''
}

function forget(): void {
  editingVisitor.value = false
  void store.forgetDevice()
}

function goBack(): void {
  void router.push({ name: 'festival-maps' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button type="button" class="btn-neutral btn-sm" @click="goBack">
        <font-awesome-icon :icon="['fas', 'arrow-left']" /> Festivals
      </button>
      <h2>
        <font-awesome-icon :icon="['fad', 'map-location-dot']" />
        {{ store.map?.title ?? 'Festival' }}
      </h2>
      <span></span>
    </div>

    <div class="tab-body content-container festival-dash">
      <LoadingSpinner v-if="store.loading && !store.map" block label="Loading the festival..." />

      <EmptyState
        v-else-if="store.notFound || !store.map"
        :icon="['fad', 'map-location-dot']"
        text="That festival isn't available."
        hint="It may not be published yet, or the festival is over."
      />

      <template v-else>
        <!-- The festival, and who is visiting it -->
        <header class="festival-dash-header">
          <ul v-if="store.map.times.length" class="map-times">
            <li v-for="(t, i) in store.map.times" :key="i">
              <font-awesome-icon :icon="['fad', 'calendar-days']" /> {{ formatEventTime(t) }}
            </li>
          </ul>
          <span v-if="store.map.is_active" class="status-badge status-badge-open">
            happening now
          </span>
          <details v-if="store.map.description" class="festival-dash-about">
            <summary>About this festival</summary>
            <MarkdownText :source="store.map.description" />
          </details>

          <div class="festival-dash-visitor">
            <template v-if="visitorLabel && !editingVisitor">
              <span>
                <font-awesome-icon :icon="['fad', 'user']" /> Visiting as
                <strong>{{ visitorLabel }}</strong>
              </span>
              <span class="festival-dash-visitor-actions">
                <button
                  v-if="!store.identityFromCard"
                  type="button"
                  class="btn-neutral btn-sm"
                  @click="editVisitor"
                >
                  Change
                </button>
                <button type="button" class="btn-caution btn-sm" @click="forget">
                  Forget this device
                </button>
              </span>
            </template>
            <form v-else class="festival-dash-form" @submit.prevent="saveVisitor">
              <p class="text-sm m-0">
                Who's visiting? Your character name finds your raffle entries and fills in your
                sign-ups. It's kept on this device only.
              </p>
              <input
                v-model="visitorName"
                placeholder="Character name"
                aria-label="Character name"
                autocomplete="off"
              />
              <WorldPicker v-model="visitorWorld" />
              <div class="form-actions">
                <button type="submit" class="btn-confirm">Continue</button>
                <button
                  v-if="editingVisitor"
                  type="button"
                  class="btn-neutral"
                  @click="editingVisitor = false"
                >
                  Cancel
                </button>
              </div>
            </form>
          </div>
        </header>

        <!-- The tiles: a summary on every screen, the section picker on a phone. -->
        <nav class="festival-dash-tiles" aria-label="Festival sections">
          <button
            v-for="tile in tiles"
            :key="tile.key"
            type="button"
            class="festival-dash-tile"
            :class="{ 'is-current': current === tile.key }"
            :aria-current="current === tile.key ? 'true' : undefined"
            @click="goTo(tile.key)"
          >
            <!-- Wrapped, so the icon-button rule in base.css (inline-flex for a
                 button whose direct child is an icon) leaves the tile's grid be. -->
            <span class="festival-dash-tile-icon">
              <font-awesome-icon :icon="['fad', tile.icon]" />
            </span>
            <span class="festival-dash-tile-label">{{ tile.label }}</span>
            <span class="festival-dash-tile-value">{{ tile.value }}</span>
          </button>
        </nav>

        <div class="festival-dash-grid">
          <!-- The plan -->
          <section
            id="festival-map"
            class="festival-dash-section festival-dash-section--wide"
            :class="{ 'is-current': current === 'map' }"
            aria-label="Festival map"
          >
            <FestivalMapWidget
              :map="store.map"
              :collected-occupant-ids="store.collectedOccupantIds"
            >
              <template #rally-action="{ collected }">
                <form
                  v-if="store.card && !collected"
                  class="stamp-entry mt-8"
                  @submit.prevent="stampFromStall"
                >
                  <input
                    v-model="stallCode"
                    class="stamp-entry-input"
                    placeholder="Stamp password"
                    aria-label="Stamp password for this stall"
                    autocomplete="off"
                    :disabled="stampRallies.submitting"
                  />
                  <button
                    type="submit"
                    class="btn-confirm btn-sm"
                    :disabled="stampRallies.submitting || !stallCode.trim()"
                  >
                    <font-awesome-icon :icon="['fas', 'stamp']" /> Stamp
                  </button>
                </form>
                <button
                  v-else-if="!store.card && rally"
                  type="button"
                  class="link-btn"
                  @click="goTo('stamps')"
                >
                  {{ rally.signup_open ? 'Get a stamp card' : 'Find your stamp card' }}
                </button>
              </template>
              <template #raffle-action="{ occupant }">
                <button
                  v-if="occupant.raffle"
                  type="button"
                  class="link-btn"
                  @click="goTo('raffles', `festival-raffle-${occupant.raffle.id}`)"
                >
                  See the raffle
                </button>
              </template>
            </FestivalMapWidget>
          </section>

          <!-- The stamp rally -->
          <section
            v-if="rally"
            id="festival-stamps"
            class="festival-dash-section"
            :class="{ 'is-current': current === 'stamps' }"
            aria-labelledby="festival-stamps-title"
          >
            <h3 id="festival-stamps-title" class="festival-dash-section-title">
              <font-awesome-icon :icon="['fad', 'stamp']" /> {{ rally.title }}
            </h3>

            <StampCardPanel v-if="store.card" :token="store.cardToken" :show-details="false" />

            <template v-else>
              <MarkdownText
                v-if="rally.details"
                class="game-details mb-16"
                :source="rally.details"
              />
              <form v-if="rally.signup_open" class="festival-dash-form" @submit.prevent="signUp">
                <p class="m-0">
                  Get a stamp card: visit the marked stalls, enter each stall's password here, and
                  collect a prize when your card is done.
                </p>
                <input
                  v-model="signupName"
                  placeholder="Character name"
                  aria-label="Character name"
                  autocomplete="off"
                />
                <WorldPicker v-model="signupWorld" />
                <TurnstileWidget
                  v-if="turnstileSiteKey"
                  ref="signupWidget"
                  :site-key="turnstileSiteKey"
                  @verified="signupTurnstile = $event"
                  @expired="signupTurnstile = ''"
                  @error="signupTurnstile = ''"
                />
                <div class="form-actions">
                  <button
                    type="submit"
                    class="btn-confirm"
                    :disabled="
                      stampRallies.submitting ||
                      !signupName.trim() ||
                      !signupWorld.trim() ||
                      (!!turnstileSiteKey && !signupTurnstile)
                    "
                  >
                    <LoadingSpinner v-if="stampRallies.submitting" label="Signing up..." />
                    <template v-else>Get my stamp card</template>
                  </button>
                  <button
                    type="button"
                    class="btn-view"
                    :disabled="store.busy || !signupName.trim() || !signupWorld.trim()"
                    @click="findCard"
                  >
                    I already have one
                  </button>
                </div>
              </form>
              <div v-else class="festival-dash-form">
                <p class="m-0">
                  Staff hand out the cards for this rally. Already have one? Open the card link you
                  were given and choose <strong>Open the festival dashboard</strong>, or find it by
                  the name it was issued under.
                </p>
                <input
                  v-model="signupName"
                  placeholder="Character name"
                  aria-label="Character name"
                  autocomplete="off"
                />
                <WorldPicker v-model="signupWorld" />
                <div class="form-actions">
                  <button
                    type="button"
                    class="btn-view"
                    :disabled="store.busy || !signupName.trim() || !signupWorld.trim()"
                    @click="findCard"
                  >
                    Find my card
                  </button>
                </div>
              </div>
            </template>
          </section>

          <!-- The garapon -->
          <section
            v-if="garapon"
            id="festival-garapon"
            class="festival-dash-section"
            :class="{ 'is-current': current === 'garapon' }"
            aria-labelledby="festival-garapon-title"
          >
            <h3 id="festival-garapon-title" class="festival-dash-section-title">
              <font-awesome-icon :icon="['fad', 'ferris-wheel']" /> {{ garapon.garapon.title }}
            </h3>
            <!-- Without the drawing link there is nothing to spin: say why, and
                 how to get there. -->
            <p v-if="!garapon.player" class="form-alert form-alert-info" role="status">
              <font-awesome-icon :icon="['fas', 'circle-info']" class="form-alert-icon" />
              <span v-if="garapon.has_paired_link">
                <strong>{{ garapon.paired_draws_left }}</strong>
                {{ garapon.paired_draws_left === 1 ? 'draw is' : 'draws are' }} waiting on your
                drawing link, which isn't on this device. Open that link here and choose
                <strong>Open the festival dashboard</strong> to draw from this page.
              </span>
              <span v-else-if="store.card">
                Your stamp card didn't come with garapon draws. Ask a member of staff.
              </span>
              <span v-else-if="rally?.signup_open">
                Get a stamp card and you'll get
                {{ garapon.default_draws }}
                {{ garapon.default_draws === 1 ? 'draw' : 'draws' }} with it.
              </span>
              <span v-else>Draws come with a stamp card from staff.</span>
            </p>
            <GaraponPanel :token="store.garaponToken" :show-details="!garapon.player" />
          </section>

          <!-- The raffles -->
          <section
            v-if="raffles.length"
            id="festival-raffles"
            class="festival-dash-section"
            :class="{ 'is-current': current === 'raffles' }"
            aria-labelledby="festival-raffles-title"
          >
            <h3 id="festival-raffles-title" class="festival-dash-section-title">
              <font-awesome-icon :icon="['fad', 'ticket']" /> Raffles
            </h3>
            <div class="list-stack">
              <FestivalRaffleCard
                v-for="row in raffles"
                :key="row.raffle.id"
                :row="row"
                :turnstile-site-key="turnstileSiteKey"
              />
            </div>
          </section>
        </div>
      </template>
    </div>
  </div>
</template>
