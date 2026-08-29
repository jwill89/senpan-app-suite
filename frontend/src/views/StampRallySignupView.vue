<script setup lang="ts">
/**
 * Public Stamp Rally sign-up: a participant issues themselves a card for one rally.
 *
 * Receives the rally id via the `id` route param and picks it out of the public
 * sign-up list (the same list the previous page renders), so the URL is directly
 * linkable - staff post it into Discord and people land straight on the form. A
 * rally that isn't in that list is closed, over, or was never opened to sign-ups;
 * either way this bounces back to the list rather than showing a form that cannot
 * succeed.
 *
 * On success the card token (and the Garapon token, when the rally has one) is shown
 * as a link the participant must keep - there is no account to log back into.
 * Those links open in a NEW TAB: following one must not navigate this page away.
 *
 * The browser also REMEMBERS them (lib/signups), so coming back here later shows
 * the links again instead of a form that would only 409 on the duplicate name.
 * That matters most for the drawing link: the lookup page can return a card link,
 * but it will never return a drawing token - a draw cannot be undone and a
 * character name is public - so this device is the only place it survives.
 */
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import { useStampRalliesStore } from '@/stores/stampRallies'
import { assetUrl } from '@/lib/assets'
import { endpoints } from '@/lib/endpoints'
import { savedRallySignup, forgetSignup, type SavedRallySignup } from '@/lib/signups'
import WorldPicker from '@/components/common/ui/WorldPicker.vue'
import { participantLabel } from '@/lib/participant'

const props = defineProps<{ id: string }>()

const router = useRouter()
const store = useStampRalliesStore()

const rallyId = computed(() => Number(props.id))
const rally = computed(() => store.signupRallies.find((r) => r.id === rallyId.value) ?? null)

const name = ref('')
const world = ref('')

// Cloudflare Turnstile bot check (empty site key = disabled).
const turnstileSiteKey = ref('')
const turnstileToken = ref('')
const turnstile = useTemplateRef<InstanceType<typeof TurnstileWidget>>('turnstile')

/** What this browser saved for this rally, if it signed up here before. */
const saved = ref<SavedRallySignup | undefined>()

/**
 * The tokens to hand back: the ones just issued, or failing that the ones this
 * browser kept from an earlier visit. Normalized to one shape so the result block
 * renders identically whether the sign-up happened a second ago or last week.
 */
const issued = computed(() => {
  const fresh = store.signupResult
  if (fresh)
    return {
      name: participantLabel(fresh.participant_name, fresh.world),
      rallyTitle: fresh.rally_title,
      cardToken: fresh.card_token,
      garaponToken: fresh.garapon_token ?? '',
      garaponTitle: fresh.garapon_title ?? '',
    }
  if (saved.value)
    return {
      name: participantLabel(saved.value.name, saved.value.world),
      rallyTitle: saved.value.rallyTitle,
      cardToken: saved.value.cardToken,
      garaponToken: saved.value.garaponToken ?? '',
      garaponTitle: saved.value.garaponTitle ?? '',
    }
  return null
})

/** True only for a sign-up that just happened, which is worth congratulating. */
const justSignedUp = computed(() => !!store.signupResult)

const cardLink = computed(() => (issued.value ? store.stampCardUrl(issued.value.cardToken) : ''))
const garaponLink = computed(() =>
  issued.value?.garaponToken ? store.garaponUrl(issued.value.garaponToken) : '',
)

/**
 * Takes the saved links off THIS device. Offered because the drawing link is
 * spendable: someone who signed up on a friend's phone needs a way to not leave it
 * there. It drops the local copy only - the sign-up itself stands.
 */
function forgetOnThisDevice(): void {
  forgetSignup('rally', rallyId.value)
  saved.value = undefined
  store.resetSignup()
}

async function load(): Promise<void> {
  store.resetSignup()
  // Read before the list loads: it is local, and it decides whether this page
  // shows a form at all.
  saved.value = savedRallySignup(rallyId.value)
  if (!store.signupRallies.length) await store.loadSignupRallies()
  if (!rally.value) void router.replace({ name: 'stamp-rallies' })
}

onMounted(async () => {
  void load()
  try {
    turnstileSiteKey.value = (await endpoints.system.config()).turnstile_site_key
  } catch {
    turnstileSiteKey.value = '' // config probe failed -> behave as if disabled
  }
})
watch(rallyId, () => load())

function onTurnstileVerified(token: string): void {
  turnstileToken.value = token
}
function onTurnstileCleared(): void {
  turnstileToken.value = ''
}

async function submit(): Promise<void> {
  const ok = await store.signUp(rallyId.value, name.value, world.value, turnstileToken.value)
  // The token is single-use, so a rejected attempt (a taken name, most often)
  // needs a fresh one before the participant can try a different name.
  if (!ok) {
    turnstileToken.value = ''
    turnstile.value?.reset()
  }
}

function back(): void {
  void router.push({ name: 'stamp-rallies' })
}

function goLookup(): void {
  void router.push({ name: 'stamp-lookup' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-neutral btn-sm" @click="back">
        <font-awesome-icon :icon="['fas', 'arrow-left']" /> Back
      </button>
      <h2>{{ rally?.title ?? 'Stamp Rally' }}</h2>
      <span></span>
    </div>

    <div class="tab-body stamp-signup-body">
      <LoadingSpinner v-if="store.signupLoading && !rally" block label="Loading rally..." />

      <template v-else-if="rally">
        <!-- The links: just issued, or restored from this device (see `issued`) -->
        <div v-if="issued" class="stamp-signup-result">
          <h3 class="mb-8">
            <font-awesome-icon :icon="['fad', 'circle-check']" />
            {{ justSignedUp ? "You're signed up!" : "You're already signed up" }}
          </h3>
          <p class="mb-16">
            Signed up as <strong class="code-highlight">{{ issued.name }}</strong> for
            <strong>{{ issued.rallyTitle }}</strong
            >.
          </p>

          <div class="stamp-signup-link">
            <span class="field-label">Your stamp card</span>
            <a :href="cardLink" target="_blank" rel="noopener" class="stamp-signup-link-url">
              {{ cardLink }}
            </a>
            <button class="btn-view btn-sm" @click="store.copyLink(cardLink)">
              <font-awesome-icon :icon="['fas', 'copy']" /> Copy
            </button>
          </div>

          <div v-if="garaponLink" class="stamp-signup-link">
            <span class="field-label">Your {{ issued.garaponTitle }} draw</span>
            <a :href="garaponLink" target="_blank" rel="noopener" class="stamp-signup-link-url">
              {{ garaponLink }}
            </a>
            <button class="btn-view btn-sm" @click="store.copyLink(garaponLink)">
              <font-awesome-icon :icon="['fas', 'copy']" /> Copy
            </button>
          </div>

          <!--
            The two links are not equally recoverable, so the warning must not
            promise the same for both: the lookup can return a card link by name,
            but never a drawing link - a draw cannot be undone and a character name
            is public. Saying "look them up" for both is what would strand someone.
          -->
          <div class="form-alert form-alert-warning mt-16" role="alert">
            <font-awesome-icon :icon="['fas', 'triangle-exclamation']" class="form-alert-icon" />
            <span>
              <strong>Save these links.</strong> This browser remembers them, so they will be here
              when you come back on this device - but clearing your browser data loses them. Your
              stamp card can also be
              <button class="link-btn" @click="goLookup">looked up by name</button>;
              <template v-if="garaponLink"
                >your draw link cannot, so keep that one somewhere safe or ask a staff member.
              </template>
            </span>
          </div>

          <p v-if="!justSignedUp" class="stamp-signup-lookup-line">
            Not your device?
            <button class="link-btn" @click="forgetOnThisDevice">Forget these links here</button>
          </p>
        </div>

        <!-- Sign-up form -->
        <template v-else>
          <img
            v-if="rally.card_image"
            :src="assetUrl(rally.card_image)"
            class="stamp-signup-card-image"
            alt="Stamp card"
          />

          <MarkdownText v-if="rally.details" class="game-details mb-16" :source="rally.details" />

          <div v-if="rally.garapon_title" class="stamp-signup-note mb-16">
            <font-awesome-icon :icon="['fad', 'circle-dot']" />
            Signing up also gets you a <strong>{{ rally.garapon_title }}</strong> garapon draw.
          </div>

          <form class="stamp-signup-form" @submit.prevent="submit">
            <div class="field">
              <label class="field-label" for="stamp-signup-name">Character Name</label>
              <input
                id="stamp-signup-name"
                v-model="name"
                placeholder="Firstname Lastname"
                maxlength="60"
                autocomplete="off"
                :disabled="store.submitting"
              />
              <p class="text-muted text-sm mt-4">
                Use your <strong>full in-game character name</strong>. Staff match sign-ups to
                characters when handing out prizes, so a nickname can leave you unrecognized. The
                same name and world can only sign up once per rally.
              </p>
            </div>

            <!--
              The world is picked, not typed. It is stored in its own field here,
              in the raffle, and on a custom card request, so the same person is
              recognisable across all three - which a typed world cannot promise.
            -->
            <WorldPicker v-model="world" :disabled="store.submitting" />

            <!-- Cloudflare Turnstile bot check (only when a site key is configured). -->
            <div v-if="turnstileSiteKey" class="turnstile-row">
              <TurnstileWidget
                ref="turnstile"
                :site-key="turnstileSiteKey"
                @verified="onTurnstileVerified"
                @expired="onTurnstileCleared"
                @error="onTurnstileCleared"
              />
            </div>

            <button
              class="btn-confirm stamp-signup-submit"
              type="submit"
              :disabled="
                store.submitting ||
                !name.trim() ||
                !world ||
                (!!turnstileSiteKey && !turnstileToken)
              "
            >
              <LoadingSpinner v-if="store.submitting" label="Signing up..." />
              <template v-else> <font-awesome-icon :icon="['fad', 'stamp']" /> Sign Up </template>
            </button>
          </form>

          <p class="stamp-signup-lookup-line">
            Already signed up?
            <button class="link-btn" @click="goLookup">Find my links</button>
          </p>
        </template>
      </template>
    </div>
  </div>
</template>
