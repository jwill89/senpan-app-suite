<script setup lang="ts">
/**
 * Admin login. On success navigates to the post-login destination (the
 * `redirect` query param if present, else the admin dashboard). The admin's
 * initial data load happens in AdminView when the dashboard mounts.
 */
import { onMounted, ref, useTemplateRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import FormField from '@/components/common/ui/FormField.vue'
import TurnstileWidget from '@/components/common/TurnstileWidget.vue'
import { endpoints } from '@/lib/endpoints'
import { passkeysSupported } from '@/lib/passkeys'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const username = ref('')
const password = ref('')

/**
 * A `?redirect=` value is only safe to navigate to when it's a same-origin path:
 * a single leading slash, not a protocol-relative `//host` and not a `scheme:`
 * URL (which could bounce the just-authenticated admin to an attacker's site).
 * Anything else falls back to the admin dashboard.
 */
function safeRedirect(value: unknown): string {
  return typeof value === 'string' && /^\/(?!\/)/.test(value) ? value : '/admin'
}

// Cloudflare Turnstile bot check. The site key (empty = disabled) comes from the
// backend; when present, the widget renders and a token is required to log in.
const turnstileSiteKey = ref('')
const turnstileToken = ref('')
const turnstile = useTemplateRef<InstanceType<typeof TurnstileWidget>>('turnstile')

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
    // Only honor a same-origin redirect path; otherwise land on /admin, where the
    // router guard forwards to the first page this account may access (admins →
    // the game tab, others → their first permitted page).
    void router.push(safeRedirect(route.query.redirect))
    return
  }
  // Turnstile tokens are single-use — re-issue one for the next attempt.
  turnstileToken.value = ''
  turnstile.value?.reset()
}

/** Sign in with a passkey (usernameless). Navigates on success like a password login. */
async function passkeyLogin(): Promise<void> {
  const ok = await auth.loginWithPasskey()
  if (ok) void router.push(safeRedirect(route.query.redirect))
}

function goHome(): void {
  void router.push({ name: 'home' })
}
</script>

<template>
  <div class="admin-login">
    <div class="box">
      <h2><font-awesome-icon :icon="['fad', 'lock']" /> Sign In</h2>
      <p>Enter your username and password</p>
      <form class="login-form" autocomplete="off" @submit.prevent="submit">
        <FormField label="Username" html-for="login-username">
          <input id="login-username" v-model="username" type="text" autocomplete="username" />
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
      <template v-if="passkeysSupported()">
        <div class="login-or"><span>or</span></div>
        <button
          type="button"
          class="btn-neutral login-passkey"
          :disabled="auth.loggingIn"
          @click="passkeyLogin"
        >
          <font-awesome-icon :icon="['fad', 'user-key']" /> Sign in with a passkey
        </button>
      </template>
      <p v-if="auth.authError" class="error-msg">{{ auth.authError }}</p>
    </div>
  </div>
</template>

<style scoped>
.login-or {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 16px 0;
  color: var(--color-text-dim, #888);
  font-size: 0.85rem;
}
.login-or::before,
.login-or::after {
  content: '';
  flex: 1;
  height: 1px;
  background: color-mix(in srgb, var(--color-text) 15%, transparent);
}
.login-passkey {
  width: 100%;
  justify-content: center;
}
</style>
