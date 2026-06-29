<script setup lang="ts">
/**
 * Admin login. On success navigates to the post-login destination (the
 * `redirect` query param if present, else the admin dashboard). The admin's
 * initial data load happens in AdminView when the dashboard mounts.
 */
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import FormField from '@/components/common/ui/FormField.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import { endpoints } from '@/lib/endpoints'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const username = ref('')
const password = ref('')

// Cloudflare Turnstile bot check. The site key (empty = disabled) comes from the
// backend; when present, the widget renders and a token is required to log in.
const turnstileSiteKey = ref('')
const turnstileToken = ref('')
const turnstile = ref<InstanceType<typeof TurnstileWidget> | null>(null)

onMounted(async () => {
  try {
    turnstileSiteKey.value = (await endpoints.system.config()).turnstile_site_key
  } catch {
    turnstileSiteKey.value = '' // config probe failed → behave as if disabled
  }
})

async function submit(): Promise<void> {
  // When the bot check is enabled, require a completed challenge first.
  if (turnstileSiteKey.value && !turnstileToken.value) {
    auth.authError = 'Please complete the “I’m not a robot” check.'
    return
  }
  const name = username.value.trim()
  const pw = password.value
  password.value = ''
  const ok = await auth.login(name, pw, turnstileToken.value || undefined)
  if (ok) {
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : null
    // Land on /admin; the router guard forwards to the first page this account
    // may access (admins → the game tab, others → their first permitted page).
    router.push(redirect || { path: '/admin' })
    return
  }
  // Turnstile tokens are single-use — re-issue one for the next attempt.
  turnstileToken.value = ''
  turnstile.value?.reset()
}

function goHome(): void {
  router.push({ name: 'home' })
}
</script>

<template>
  <div class="admin-login">
    <div class="box">
      <h2><font-awesome-icon :icon="['fad', 'lock']" /> Sign In</h2>
      <p>Enter your username and password</p>
      <form class="login-form" autocomplete="off" @submit.prevent="submit">
        <FormField label="Username" html-for="login-username">
          <input
            id="login-username"
            v-model="username"
            type="text"
            autocomplete="username"
          />
        </FormField>
        <FormField label="Password" html-for="login-password">
          <input
            id="login-password"
            v-model="password"
            type="password"
            autocomplete="current-password"
          />
        </FormField>
        <!-- Cloudflare Turnstile bot check (only when a site key is configured). -->
        <TurnstileWidget
          v-if="turnstileSiteKey"
          ref="turnstile"
          :site-key="turnstileSiteKey"
          @verified="turnstileToken = $event"
          @expired="turnstileToken = ''"
          @error="turnstileToken = ''"
        />
        <div class="btns">
          <button type="button" class="btn-neutral" :disabled="auth.loggingIn" @click="goHome">
            Back
          </button>
          <button
            type="submit"
            class="btn-action"
            :disabled="auth.loggingIn || (!!turnstileSiteKey && !turnstileToken)"
          >
            <LoadingSpinner v-if="auth.loggingIn" label="Logging in…" />
            <template v-else>Login</template>
          </button>
        </div>
      </form>
      <p v-if="auth.authError" class="error-msg">{{ auth.authError }}</p>
    </div>
  </div>
</template>
