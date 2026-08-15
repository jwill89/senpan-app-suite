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
 * as a link the participant must keep - there is no account to log back into, so the
 * page is emphatic about saving it and points at the lookup page as the way back.
 */
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import { useStampRalliesStore } from '@/stores/stampRallies'
import { assetUrl } from '@/lib/assets'
import { endpoints } from '@/lib/endpoints'

const props = defineProps<{ id: string }>()

const router = useRouter()
const store = useStampRalliesStore()

const rallyId = computed(() => Number(props.id))
const rally = computed(() => store.signupRallies.find((r) => r.id === rallyId.value) ?? null)

const name = ref('')

// Cloudflare Turnstile bot check (empty site key = disabled).
const turnstileSiteKey = ref('')
const turnstileToken = ref('')
const turnstile = useTemplateRef<InstanceType<typeof TurnstileWidget>>('turnstile')

/** The links to hand back, built from the tokens the sign-up returned. */
const cardLink = computed(() =>
  store.signupResult ? store.stampCardUrl(store.signupResult.card_token) : '',
)
const garaponLink = computed(() =>
  store.signupResult?.garapon_token ? store.garaponUrl(store.signupResult.garapon_token) : '',
)

async function load(): Promise<void> {
  store.resetSignup()
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
  const ok = await store.signUp(rallyId.value, name.value, turnstileToken.value)
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
        <!-- Success: the links, and a push to keep them -->
        <div v-if="store.signupResult" class="stamp-signup-result">
          <h3 class="mb-8">
            <font-awesome-icon :icon="['fad', 'circle-check']" /> You're signed up!
          </h3>
          <p class="mb-16">
            Signed up as
            <strong class="code-highlight">{{ store.signupResult.participant_name }}</strong> for
            <strong>{{ store.signupResult.rally_title }}</strong
            >.
          </p>

          <div class="stamp-signup-link">
            <span class="field-label">Your stamp card</span>
            <a :href="cardLink" class="stamp-signup-link-url">{{ cardLink }}</a>
            <button class="btn-view btn-sm" @click="store.copyLink(cardLink)">
              <font-awesome-icon :icon="['fas', 'copy']" /> Copy
            </button>
          </div>

          <div v-if="garaponLink" class="stamp-signup-link">
            <span class="field-label">Your {{ store.signupResult.garapon_title }} draw</span>
            <a :href="garaponLink" class="stamp-signup-link-url">{{ garaponLink }}</a>
            <button class="btn-view btn-sm" @click="store.copyLink(garaponLink)">
              <font-awesome-icon :icon="['fas', 'copy']" /> Copy
            </button>
          </div>

          <div class="form-alert form-alert-warning mt-16" role="alert">
            <font-awesome-icon :icon="['fas', 'triangle-exclamation']" class="form-alert-icon" />
            <span>
              <strong>Save these links.</strong> They are the only way back to your card - there is
              no account to log into. If you lose them, you can
              <button class="link-btn" @click="goLookup">look them up by name</button>.
            </span>
          </div>
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
                placeholder="Firstname Lastname @ World"
                maxlength="60"
                autocomplete="off"
                :disabled="store.submitting"
              />
              <p class="text-muted text-sm mt-4">
                Use your <strong>full in-game character name</strong>, and add
                <strong>@ World</strong> if you'd like. Staff match sign-ups to characters when
                handing out prizes, so a nickname can leave you unrecognized. Note: names can only
                be used once per rally.
              </p>
            </div>

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
                store.submitting || !name.trim() || (!!turnstileSiteKey && !turnstileToken)
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
