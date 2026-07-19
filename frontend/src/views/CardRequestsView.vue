<script setup lang="ts">
/**
 * Public Personal Card Requests page. A player builds a bingo card from scratch
 * (or "Generate Random"), picks a 6-character card ID and their character/world,
 * and submits it for staff approval. The form validates the card client-side
 * (mirroring the backend) and blocks submission until it's a valid card; the
 * server additionally rejects a taken ID or a duplicate board with a clear message.
 */
import { computed, onMounted, ref, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import BingoCardEditor from '@/components/common/BingoCardEditor.vue'
import { useCardRequestsStore } from '@/stores/cardRequests'
import { useAppStore } from '@/stores/app'
import { FF14_WORLDS } from '@/lib/constants'
import { endpoints } from '@/lib/endpoints'

const router = useRouter()
const cr = useCardRequestsStore()
const app = useAppStore()

// Cloudflare Turnstile bot check (empty site key = disabled).
const turnstileSiteKey = ref('')
const turnstile = useTemplateRef<InstanceType<typeof TurnstileWidget>>('turnstile')

onMounted(async () => {
  cr.reset()
  try {
    turnstileSiteKey.value = (await endpoints.system.config()).turnstile_site_key
  } catch {
    turnstileSiteKey.value = ''
  }
})

const cost = computed(() => {
  const n = Number(app.settings.custom_card_cost || '0')
  return Number.isFinite(n) ? n : 0
})

const validationMessage = computed(() => cr.validate())
// Only surface the inline hint once the player has started filling the form, so a
// pristine page doesn't open with a red error.
const touched = computed(
  () =>
    !!(cr.characterName || cr.world || cr.cardId) || cr.board.some((r) => r.some((n) => n !== 0)),
)

function onCardIdInput(e: Event): void {
  cr.cardId = (e.target as HTMLInputElement).value
    .toUpperCase()
    .replace(/[^A-Z0-9]/g, '')
    .slice(0, 6)
}

function onTurnstileVerified(token: string): void {
  cr.turnstileToken = token
}
function onTurnstileCleared(): void {
  cr.turnstileToken = ''
}

async function submit(): Promise<void> {
  await cr.submit()
  // On failure the (single-use) token was cleared — re-issue a fresh one.
  if (!cr.result) turnstile.value?.reset()
}

function back(): void {
  void router.push({ name: 'home' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-neutral btn-sm" @click="back">← Back</button>
      <h2>Personal Card Request</h2>
      <span></span>
    </div>
    <div class="tab-body card-request-body">
      <!-- Success state -->
      <div v-if="cr.result" class="request-result">
        <h3 class="text-success mb-8">
          <font-awesome-icon :icon="['fad', 'circle-check']" /> Request submitted!
        </h3>
        <p>
          Your custom card <strong class="code-gold">{{ cr.result.id }}</strong> is now
          <strong>pending staff approval</strong>. Once it's approved (and paid for), it becomes
          playable on regular bingo nights.
        </p>
        <button class="btn-view mt-12" @click="cr.reset()">Request another card</button>
      </div>

      <!-- Request form -->
      <template v-else>
        <div class="request-terms mb-16">
          <p>
            Design your own bingo card below. <strong>Senpan staff must approve</strong> each custom
            card, and it costs <strong>{{ cost.toLocaleString() }} gil</strong>.
          </p>
          <p class="text-dim text-sm">
            A custom card lasts until the end of the calendar year and can be used on any regular
            (non-event) bingo night.
          </p>
        </div>

        <div class="card-request-form">
          <div class="field">
            <label class="field-label">Character Name</label>
            <input
              v-model="cr.characterName"
              placeholder="Character Name"
              maxlength="60"
              aria-label="Character name"
            />
          </div>

          <div class="field">
            <label class="field-label">World</label>
            <select v-model="cr.world" aria-label="World">
              <option value="" disabled>Select your world…</option>
              <optgroup
                v-for="dc in FF14_WORLDS"
                :key="dc.name"
                :label="`${dc.name} (${dc.region})`"
              >
                <option v-for="w in dc.worlds" :key="w" :value="w">{{ w }}</option>
              </optgroup>
            </select>
          </div>

          <div class="field">
            <label class="field-label">Custom Card ID</label>
            <input
              :value="cr.cardId"
              placeholder="ABC123"
              maxlength="6"
              autocapitalize="characters"
              autocomplete="off"
              spellcheck="false"
              aria-label="Custom card ID"
              @input="onCardIdInput"
            />
            <p class="text-dim text-xs mt-4">6 letters or numbers — your card's join code.</p>
          </div>

          <div class="field">
            <label class="field-label">Your Card</label>
            <p class="text-dim text-sm mb-8">
              Enter a number in each cell (B 1–15, I 16–30, N 31–45, G 46–60, O 61–75), or use
              “Generate Random” and tweak from there. The centre is a free space.
            </p>
            <div class="card-editor-wrap">
              <BingoCardEditor v-model="cr.board" />
            </div>
          </div>

          <!-- Validation error, shown as a danger callout above the bot check. -->
          <div
            v-if="touched && validationMessage"
            class="form-alert form-alert-danger"
            role="alert"
          >
            <font-awesome-icon :icon="['fas', 'triangle-exclamation']" class="form-alert-icon" />
            <span>{{ validationMessage }}</span>
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
            class="btn-confirm card-request-submit"
            :disabled="
              cr.submitting || !!validationMessage || (!!turnstileSiteKey && !cr.turnstileToken)
            "
            @click="submit"
          >
            <LoadingSpinner v-if="cr.submitting" label="Submitting…" />
            <template v-else>Submit Request</template>
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.card-request-body {
  max-width: 560px;
  margin: 0 auto;
}

/* Short intro / terms note above the form. */
.request-terms {
  background: var(--panel-raised-bg);
  border-radius: var(--radius, 8px);
  padding: 12px 16px;
}

/* Panel background around the whole form. */
.card-request-form {
  background: var(--panel-bg);
  border: 1px solid var(--control-border);
  border-radius: var(--radius, 8px);
  padding: 20px;
}
.card-request-form input,
.card-request-form select {
  width: 100%;
}

/* Group + centre the bingo card editor in a raised sub-panel. */
.card-editor-wrap {
  background: var(--panel-raised-bg);
  border-radius: var(--radius, 8px);
  padding: 16px;
}

/* Validation error shown as a danger callout (icon + tinted box). */
.form-alert {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 14px;
  border-radius: var(--radius, 8px);
  font-size: 0.9rem;
  margin-bottom: 14px;
}
.form-alert-danger {
  background: color-mix(in srgb, var(--danger) 15%, transparent);
  border: 1px solid var(--danger);
  color: var(--text);
}
.form-alert-icon {
  color: var(--danger);
  margin-top: 2px;
  flex-shrink: 0;
}

/* Centre the bot-check widget. */
.turnstile-row {
  display: flex;
  justify-content: center;
  margin-bottom: 14px;
}

.card-request-submit {
  width: 100%;
}

/* Success confirmation panel. */
.request-result {
  background: var(--panel-bg);
  border: 2px solid var(--success);
  border-radius: var(--radius, 8px);
  padding: 20px;
  text-align: center;
}
</style>
