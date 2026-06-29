<script setup lang="ts">
/**
 * Cloudflare Turnstile ("are you a robot?") widget. Lazily loads the Turnstile
 * script, renders the challenge into this element, and emits the resulting
 * one-time token via `verified`. The parent passes that token to the login
 * request, which the backend checks with Cloudflare's siteverify API.
 *
 * Tokens are single-use and expire, so the parent should call the exposed
 * `reset()` after a failed login (or on `expired`) to issue a fresh one.
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{ siteKey: string }>()
const emit = defineEmits<{ verified: [token: string]; expired: []; error: [] }>()

const host = ref<HTMLDivElement | null>(null)
let widgetId: string | undefined

const SCRIPT_SRC = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'

/** Load the Turnstile script once (idempotent across mounts). */
function loadScript(): Promise<void> {
  if (window.turnstile) return Promise.resolve()
  const existing = document.querySelector<HTMLScriptElement>(`script[src="${SCRIPT_SRC}"]`)
  if (existing) {
    return new Promise((resolve, reject) => {
      existing.addEventListener('load', () => resolve(), { once: true })
      existing.addEventListener('error', () => reject(new Error('turnstile load failed')), {
        once: true,
      })
    })
  }
  return new Promise((resolve, reject) => {
    const s = document.createElement('script')
    s.src = SCRIPT_SRC
    s.async = true
    s.defer = true
    s.addEventListener('load', () => resolve(), { once: true })
    s.addEventListener('error', () => reject(new Error('turnstile load failed')), { once: true })
    document.head.appendChild(s)
  })
}

onMounted(async () => {
  try {
    await loadScript()
    if (!window.turnstile || !host.value) {
      emit('error')
      return
    }
    widgetId = window.turnstile.render(host.value, {
      sitekey: props.siteKey,
      callback: (token: string) => emit('verified', token),
      'expired-callback': () => emit('expired'),
      'error-callback': () => emit('error'),
    })
  } catch {
    emit('error')
  }
})

onBeforeUnmount(() => {
  if (window.turnstile && widgetId !== undefined) window.turnstile.remove(widgetId)
})

/** Re-issue a fresh token (tokens are single-use; call after a failed attempt). */
function reset(): void {
  if (window.turnstile && widgetId !== undefined) window.turnstile.reset(widgetId)
}
defineExpose({ reset })
</script>

<template>
  <div ref="host" class="turnstile-widget"></div>
</template>

<style scoped>
.turnstile-widget {
  display: flex;
  justify-content: center;
  min-height: 65px; /* reserve the widget's height to avoid a layout jump */
}
</style>
