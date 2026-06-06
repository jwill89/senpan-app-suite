/**
 * Auth store: admin login/logout + authentication state.
 *
 * `isAdmin` is the source of truth for the router's admin-route guard. It is
 * refreshed via `checkAuth()` (GET /api/auth) on first navigation to an admin
 * route and set directly on login/logout. The actual data-loading after login
 * (cards, patterns, game, raffles, settings) is orchestrated by the Admin route
 * setup to keep this store focused.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '@/lib/api'

export const useAuthStore = defineStore('auth', () => {
  const authError = ref('')
  /** Whether the current session is an authenticated admin. */
  const isAdmin = ref(false)
  /** True once checkAuth() has resolved at least once (guards initial load). */
  const authChecked = ref(false)

  /**
   * Queries the server for the current admin auth status and caches it.
   * Returns the resulting boolean. Used by the router guard for /admin routes.
   */
  async function checkAuth(): Promise<boolean> {
    try {
      const data = await api<{ authenticated: boolean }>('auth')
      isAdmin.value = data.authenticated
    } catch {
      isAdmin.value = false
    } finally {
      authChecked.value = true
    }
    return isAdmin.value
  }

  /**
   * Attempts admin login with the given password.
   * Returns true on success; sets authError and returns false on failure.
   */
  async function login(password: string): Promise<boolean> {
    authError.value = ''
    try {
      await api('auth', { method: 'POST', body: { action: 'login', password } })
      isAdmin.value = true
      authChecked.value = true
      return true
    } catch (e) {
      authError.value = (e as Error).message
      return false
    }
  }

  /** Logs out (best-effort; ignores errors). */
  async function logout(): Promise<void> {
    try {
      await api('auth', { method: 'POST', body: { action: 'logout' } })
    } catch {
      /* ignore */
    } finally {
      isAdmin.value = false
    }
  }

  return { authError, isAdmin, authChecked, checkAuth, login, logout }
})
