<script setup lang="ts">
/**
 * Shown to a logged-in account that has no pages granted yet (active but with an
 * empty permission set). The router guard redirects here when there is no
 * permitted admin page to land on.
 */
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

async function signOut(): Promise<void> {
  await auth.logout()
  router.push({ name: 'admin-login' })
}
</script>

<template>
  <div class="admin-login">
    <div class="box">
      <h2><i class="fa-duotone fa-lock"></i> No Access Yet</h2>
      <p>
        Your account is active, but an administrator hasn’t granted you access to any pages yet.
        Please check back later.
      </p>
      <div class="btns">
        <button type="button" class="btn-neutral" @click="signOut">Sign Out</button>
      </div>
    </div>
  </div>
</template>
