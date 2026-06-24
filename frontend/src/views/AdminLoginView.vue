<script setup lang="ts">
/**
 * Admin login. On success navigates to the post-login destination (the
 * `redirect` query param if present, else the admin dashboard). The admin's
 * initial data load happens in AdminView when the dashboard mounts.
 */
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import FormField from '@/components/common/ui/FormField.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const username = ref('')
const password = ref('')

async function submit(): Promise<void> {
  const name = username.value.trim()
  const pw = password.value
  password.value = ''
  const ok = await auth.login(name, pw)
  if (ok) {
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : null
    // Land on /admin; the router guard forwards to the first page this account
    // may access (admins → the game tab, others → their first permitted page).
    router.push(redirect || { path: '/admin' })
  }
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
        <div class="btns">
          <button type="button" class="btn-neutral" :disabled="auth.loggingIn" @click="goHome">
            Back
          </button>
          <button type="submit" class="btn-action" :disabled="auth.loggingIn">
            <LoadingSpinner v-if="auth.loggingIn" label="Logging in…" />
            <template v-else>Login</template>
          </button>
        </div>
      </form>
      <p v-if="auth.authError" class="error-msg">{{ auth.authError }}</p>
    </div>
  </div>
</template>
