<script setup lang="ts">
/**
 * Admin login. On success navigates to the post-login destination (the
 * `redirect` query param if present, else the admin dashboard). The admin's
 * initial data load happens in AdminView when the dashboard mounts.
 */
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const password = ref('')

async function submit(): Promise<void> {
  const pw = password.value
  password.value = ''
  const ok = await auth.login(pw)
  if (ok) {
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : null
    router.push(redirect || { name: 'admin-bingo-game' })
  }
}

function goHome(): void {
  router.push({ name: 'home' })
}
</script>

<template>
  <div class="admin-login">
    <div class="box">
      <h2><i class="fa-solid fa-lock"></i> Admin Login</h2>
      <p>Enter the admin password</p>
      <form autocomplete="off" @submit.prevent="submit">
        <input
          v-model="password"
          type="password"
          placeholder="Password"
          aria-label="Admin password"
          autocomplete="new-password"
        />
        <div class="btns">
          <button type="button" class="btn-ghost" @click="goHome">Back</button>
          <button type="submit" class="btn-secondary">Login</button>
        </div>
      </form>
      <p v-if="auth.authError" class="error-msg">{{ auth.authError }}</p>
    </div>
  </div>
</template>
