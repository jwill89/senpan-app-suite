<script setup lang="ts">
/**
 * The festival dashboard's Ema Plaques section: the three ways to write a plaque,
 * the day that's taking them, and the visitor's own plaques.
 *
 * 1. Do it yourself - download the template, decorate it anywhere, post it in
 *    Discord. Nothing is recorded, so it isn't counted against the day.
 * 2. Design it here - the on-site designer (loaded only when opened), then a
 *    confirm step with the bot check. Staff approve it, and the app posts it.
 * 3. Ask the artist - a description, saved to the artist's queue.
 *
 * Options 2 and 3 share the one-plaque-per-day rule, which the server enforces;
 * the section mirrors it so nobody designs a plaque they can't send.
 */
import { computed, defineAsyncComponent, ref, useTemplateRef } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import { useFestivalDashboardStore } from '@/stores/festivalDashboard'
import { assetUrl } from '@/lib/assets'
import { formatEventRange } from '@/lib/festivalmap'
import type { FestivalDashboardEma } from '@/types/api'

const props = defineProps<{
  ema: FestivalDashboardEma
  /** Cloudflare Turnstile site key ('' = the check is off). */
  turnstileSiteKey: string
}>()

const store = useFestivalDashboardStore()

// The designer pulls in its canvas library, so it loads only when someone opens it.
const EmaDesigner = defineAsyncComponent(() => import('@/components/player/ema/EmaDesigner.vue'))

/** Status wording for the visitor's own plaques. */
const STATUS: Record<string, { text: string; badge: string }> = {
  pending: { text: 'Waiting for staff', badge: 'badge--warning' },
  posting: { text: 'Being posted', badge: 'badge--warning' },
  posted: { text: 'Posted to Discord', badge: 'badge--success' },
  rejected: { text: 'Not approved', badge: 'badge--muted' },
  requested: { text: 'With the artist', badge: 'badge--accent' },
  complete: { text: 'The artist finished it', badge: 'badge--success' },
}

const currentWindow = computed(
  () => props.ema.windows.find((w) => w.id === props.ema.current_window_id) ?? null,
)
/** The next window to open, when none is open now. */
const nextWindow = computed(() =>
  currentWindow.value
    ? null
    : (props.ema.windows.find((w) => Date.parse(w.start_at) > Date.now()) ?? null),
)
/** Whether the visitor may send a design or a request right now. */
const canSend = computed(
  () => !!currentWindow.value && !!store.visitorName && !props.ema.has_plaque_this_window,
)

// -- Design on the site -----------------------------------------------------------

const designerOpen = ref(false)
/**
 * The finished design, waiting for the bot check and "Send". The confirm step is
 * shown over the designer inside the same modal, with the designer hidden but still
 * mounted, so "Back to Editing" returns to the plaque as it was.
 */
const design = ref<Blob | null>(null)
const designPreview = ref('')
const designToken = ref('')
const designWidget = useTemplateRef<InstanceType<typeof TurnstileWidget>>('designWidget')

function openDesigner(): void {
  void store.loadGoogleFonts()
  designerOpen.value = true
}

/** The designer finished: show what will be sent before sending it. */
function onDesigned(png: Blob): void {
  design.value = png
  const reader = new FileReader()
  // A data: URL, not a blob: one - the site's Content-Security-Policy allows the
  // former for images and not the latter.
  reader.onload = () => {
    if (typeof reader.result === 'string') designPreview.value = reader.result
  }
  reader.readAsDataURL(png)
}

/** Back from the confirm step to the designer, the plaque as it was. */
function backToEditing(): void {
  design.value = null
  designPreview.value = ''
  designToken.value = ''
}

function closeDesigner(): void {
  backToEditing()
  designerOpen.value = false
}

/** The designer's own "anything to lose?" check, exposed for closing from outside it. */
const designer = useTemplateRef<{ confirmDiscard: () => Promise<boolean> }>('designer')

/**
 * Escape (or the backdrop) closes the modal from outside the designer, so it asks
 * the designer first: the plaque lives only there, and it knows whether anything
 * would be lost (nothing added, or already downloaded, closes without a question).
 */
async function onModalClose(): Promise<void> {
  if (!designer.value || (await designer.value.confirmDiscard())) closeDesigner()
}

async function sendDesign(): Promise<void> {
  if (!design.value) return
  if (await store.submitEmaPlaque(design.value, designToken.value)) {
    closeDesigner()
    return
  }
  designToken.value = ''
  designWidget.value?.reset()
}

// -- Ask the artist ---------------------------------------------------------------

const requestText = ref('')
const requestToken = ref('')
const requestWidget = useTemplateRef<InstanceType<typeof TurnstileWidget>>('requestWidget')

async function sendRequest(): Promise<void> {
  if (await store.requestEmaPlaque(requestText.value, requestToken.value)) {
    requestText.value = ''
    requestToken.value = ''
    return
  }
  requestToken.value = ''
  requestWidget.value?.reset()
}
</script>

<template>
  <div class="festival-ema">
    <!-- Which day is taking plaques -->
    <p v-if="currentWindow" class="festival-ema-day">
      <font-awesome-icon :icon="['fad', 'calendar-days']" />
      Plaques for <strong>{{ currentWindow.label }}</strong> are being taken now -
      {{ formatEventRange(currentWindow.start_at, currentWindow.end_at) }}
    </p>
    <p v-else-if="nextWindow" class="festival-ema-day">
      <font-awesome-icon :icon="['fad', 'calendar-days']" />
      The next plaque day is <strong>{{ nextWindow.label }}</strong
      >,
      {{ formatEventRange(nextWindow.start_at, nextWindow.end_at) }}
    </p>
    <p v-else class="festival-ema-day text-muted">The plaque days for this festival are over.</p>

    <!-- The visitor's own plaques -->
    <ul v-if="ema.plaques.length" class="list-stack mb-16">
      <li v-for="p in ema.plaques" :key="p.id" class="festival-ema-mine">
        <img v-if="p.image" :src="assetUrl(p.image)" class="festival-ema-thumb" alt="" />
        <span>
          <strong>{{ p.window_label }}</strong> -
          {{ p.kind === 'site' ? 'designed here' : `asked of ${ema.artist_name}` }}
        </span>
        <span :class="['badge', STATUS[p.status]?.badge ?? 'badge--muted']">
          {{ STATUS[p.status]?.text ?? p.status }}
        </span>
      </li>
    </ul>

    <p v-if="ema.has_plaque_this_window" class="callout">
      <font-awesome-icon :icon="['fad', 'torii-gate']" /> You've hung your plaque for
      {{ currentWindow?.label ?? 'today' }}. You can write another on the next plaque day.
    </p>
    <p v-else-if="currentWindow && !store.visitorName" class="form-alert form-alert-info">
      <font-awesome-icon :icon="['fas', 'circle-info']" class="form-alert-icon" />
      <span>Say who you're visiting as at the top of the page to send a plaque.</span>
    </p>

    <div class="festival-ema-options">
      <!-- 1. Do it yourself -->
      <section
        v-if="ema.allow_self && ema.template_image"
        class="card card--padded festival-ema-option"
      >
        <h4 class="festival-ema-option-title">
          <font-awesome-icon :icon="['fas', 'download']" /> Design It Yourself
        </h4>
        <img
          :src="assetUrl(ema.template_image)"
          class="festival-ema-template"
          alt="The blank plaque"
        />
        <MarkdownText v-if="ema.instructions" :source="ema.instructions" />
        <p v-else class="text-sm m-0">
          Save the blank plaque, decorate it in any image editor, and post it in the festival's
          Discord.
        </p>
        <div class="form-actions">
          <a class="btn-action btn-sm" :href="assetUrl(ema.template_image)" download>
            <font-awesome-icon :icon="['fas', 'download']" /> Save the Template
          </a>
          <a
            v-if="ema.discord_link"
            class="btn-neutral btn-sm"
            :href="ema.discord_link"
            target="_blank"
            rel="noopener noreferrer"
          >
            <font-awesome-icon :icon="['fab', 'discord']" /> Post It in Discord
          </a>
        </div>
      </section>

      <!-- 2. Design it here -->
      <section
        v-if="ema.allow_site && ema.template_image"
        class="card card--padded festival-ema-option"
      >
        <h4 class="festival-ema-option-title">
          <font-awesome-icon :icon="['fad', 'palette']" /> Design It Here
        </h4>
        <p class="text-sm m-0">
          Add pictures and words to the plaque right here - no image editor needed. Staff look it
          over, then post it to Discord with your name.
        </p>
        <div class="form-actions">
          <button
            type="button"
            class="btn-action btn-sm"
            :disabled="!canSend"
            @click="openDesigner"
          >
            <font-awesome-icon :icon="['fad', 'palette']" /> Open the Designer
          </button>
        </div>
      </section>

      <!-- 3. Ask the artist -->
      <section v-if="ema.allow_artist" class="card card--padded festival-ema-option">
        <h4 class="festival-ema-option-title">
          <font-awesome-icon :icon="['fad', 'torii-gate']" /> Ask {{ ema.artist_name }}
        </h4>
        <p class="text-sm m-0">
          Describe the plaque you'd like and {{ ema.artist_name }} will make it and post it for you.
        </p>
        <form class="festival-dash-form" @submit.prevent="sendRequest">
          <textarea
            v-model="requestText"
            rows="4"
            maxlength="2000"
            :placeholder="`What should ${ema.artist_name} draw and write?`"
            :aria-label="`Your request for ${ema.artist_name}`"
            :disabled="!canSend"
          ></textarea>
          <TurnstileWidget
            v-if="turnstileSiteKey && canSend"
            ref="requestWidget"
            :site-key="turnstileSiteKey"
            @verified="requestToken = $event"
            @expired="requestToken = ''"
            @error="requestToken = ''"
          />
          <div class="form-actions">
            <button
              type="submit"
              class="btn-confirm btn-sm"
              :disabled="
                !canSend ||
                store.busy ||
                !requestText.trim() ||
                (!!turnstileSiteKey && !requestToken)
              "
            >
              Send to {{ ema.artist_name }}
            </button>
          </div>
        </form>
      </section>
    </div>

    <!-- The designer, full screen. The confirm step replaces it on screen without
         unmounting it, so going back to edit keeps the plaque as it was. -->
    <ModalOverlay
      v-if="designerOpen"
      class="festival-ema-designer-modal"
      aria-label="Plaque designer"
      @close="onModalClose"
    >
      <EmaDesigner
        v-show="!design"
        ref="designer"
        :template-src="assetUrl(ema.template_image)"
        :fonts="store.googleFonts"
        :busy="store.busy"
        @done="onDesigned"
        @cancel="closeDesigner"
      />
      <div v-if="design" class="festival-ema-confirm">
        <h3 class="festival-ema-option-title">Send this plaque?</h3>
        <img
          v-if="designPreview"
          :src="designPreview"
          class="festival-ema-template"
          alt="Your plaque"
        />
        <p class="text-sm m-0">
          It goes to staff first. Once they approve it, it's posted to the festival's Discord as
          <strong>{{ store.visitorName }} @ {{ store.visitorWorld }}</strong> for
          {{ currentWindow?.label ?? 'today' }}.
        </p>
        <TurnstileWidget
          v-if="turnstileSiteKey"
          ref="designWidget"
          :site-key="turnstileSiteKey"
          @verified="designToken = $event"
          @expired="designToken = ''"
          @error="designToken = ''"
        />
        <div class="form-actions">
          <button
            type="button"
            class="btn-confirm"
            :disabled="!canSend || store.busy || (!!turnstileSiteKey && !designToken)"
            @click="sendDesign"
          >
            <LoadingSpinner v-if="store.busy" label="Sending..." />
            <template v-else>Send for Posting</template>
          </button>
          <button type="button" class="btn-neutral" @click="backToEditing">Back to Editing</button>
        </div>
      </div>
    </ModalOverlay>
  </div>
</template>
