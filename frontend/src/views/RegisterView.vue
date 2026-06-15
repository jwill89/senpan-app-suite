<script setup lang="ts">
/**
 * Account registration. This page is intentionally NOT linked anywhere in the
 * UI — an admin shares the /admin/register URL directly. New accounts are
 * created inactive and cannot log in until an admin activates them, so an
 * unsolicited signup is harmless.
 */
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

const username = ref('')
const password = ref('')
const confirm = ref('')
/** Server confirmation message shown after a successful registration. */
const done = ref('')
/** Client-side validation message (mismatched/short password). */
const localError = ref('')

async function submit(): Promise<void> {
  localError.value = ''
  const name = username.value.trim()
  if (!name) {
    localError.value = 'Username is required.'
    return
  }
  if (password.value.length < 8) {
    localError.value = 'Password must be at least 8 characters.'
    return
  }
  if (password.value !== confirm.value) {
    localError.value = 'Passwords do not match.'
    return
  }
  const message = await auth.register(name, password.value)
  password.value = ''
  confirm.value = ''
  if (message) done.value = message
}

function goLogin(): void {
  router.push({ name: 'admin-login' })
}
</script>

<template>
  <div class="admin-login">
    <div class="box">
      <h2><font-awesome-icon :icon="['fad', 'user-plus']" /> Create Account</h2>

      <template v-if="done">
        <p class="success-msg">{{ done }}</p>
        <div class="btns">
          <button type="button" class="btn-action" @click="goLogin">Go to Sign In</button>
        </div>
      </template>

      <template v-else>
        <p>Choose a username and password</p>
        <form autocomplete="off" @submit.prevent="submit">
          <input
            v-model="username"
            type="text"
            placeholder="Username"
            aria-label="Username"
            autocomplete="username"
          />
          <input
            v-model="password"
            type="password"
            placeholder="Password (min 8 characters)"
            aria-label="Password"
            autocomplete="new-password"
          />
          <input
            v-model="confirm"
            type="password"
            placeholder="Confirm password"
            aria-label="Confirm password"
            autocomplete="new-password"
          />
          <div class="btns">
            <button type="button" class="btn-neutral" :disabled="auth.loggingIn" @click="goLogin">
              Back
            </button>
            <button type="submit" class="btn-action" :disabled="auth.loggingIn">
              <LoadingSpinner v-if="auth.loggingIn" label="Creating…" />
              <template v-else>Create Account</template>
            </button>
          </div>
        </form>
        <p v-if="localError" class="error-msg">{{ localError }}</p>
        <p v-else-if="auth.authError" class="error-msg">{{ auth.authError }}</p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.success-msg {
  color: var(--highlight);
  font-size: 0.9rem;
  margin-bottom: 20px;
}
</style>
