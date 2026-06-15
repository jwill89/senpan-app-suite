/**
 * Auth store: account login/logout/registration + authentication state.
 *
 * `isAdmin` and `user` are the source of truth for the router's admin-route
 * guard and per-page permission gating. They are refreshed via `checkAuth()`
 * (GET /api/auth) on first navigation to an admin route and set directly on
 * login/logout. The actual data-loading after login is orchestrated by the
 * Admin route setup to keep this store focused.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { User } from '@/types/api'

export const useAuthStore = defineStore('auth', () => {
  const authError = ref('')
  /** The logged-in account (null when not authenticated). */
  const user = ref<User | null>(null)
  /** Whether the current session is an authenticated admin. */
  const isAdmin = ref(false)
  /** True once checkAuth() has resolved at least once (guards initial load). */
  const authChecked = ref(false)
  /** True while a login request is in flight (drives the submit button). */
  const loggingIn = ref(false)

  /** The current user's granted page-permission keys (empty when logged out). */
  const permissions = computed(() => user.value?.permissions ?? [])

  /**
   * Whether the current user may access a page. Admins implicitly have access to
   * everything; other active users need the specific permission key granted.
   */
  function hasPermission(key: string): boolean {
    if (!user.value) return false
    if (user.value.is_admin) return true
    return user.value.permissions.includes(key)
  }

  /** Applies the user payload from an auth response to the store. */
  function setUser(u: User | null): void {
    user.value = u
    isAdmin.value = !!u?.is_admin
  }

  /**
   * Queries the server for the current auth status (and user) and caches it.
   * Returns the resulting boolean. Used by the router guard for /admin routes.
   */
  async function checkAuth(): Promise<boolean> {
    try {
      const data = await endpoints.auth.check()
      setUser(data.authenticated ? data.user : null)
    } catch {
      setUser(null)
    } finally {
      authChecked.value = true
    }
    return isAdmin.value || !!user.value
  }

  /**
   * Attempts login with the given username + password.
   * Returns true on success; sets authError and returns false on failure.
   */
  async function login(username: string, password: string): Promise<boolean> {
    authError.value = ''
    loggingIn.value = true
    try {
      const data = await endpoints.auth.login(username, password)
      setUser(data.user)
      authChecked.value = true
      return true
    } catch (e) {
      authError.value = (e as Error).message
      return false
    } finally {
      loggingIn.value = false
    }
  }

  /**
   * Registers a new account (hidden registration page). Returns the server's
   * confirmation message on success; sets authError and returns null on failure.
   */
  async function register(username: string, password: string): Promise<string | null> {
    authError.value = ''
    loggingIn.value = true
    try {
      const data = await endpoints.auth.register(username, password)
      return data.message
    } catch (e) {
      authError.value = (e as Error).message
      return null
    } finally {
      loggingIn.value = false
    }
  }

  /** Logs out (best-effort; ignores errors). */
  async function logout(): Promise<void> {
    try {
      await endpoints.auth.logout()
    } catch {
      /* ignore */
    } finally {
      setUser(null)
    }
  }

  return {
    authError,
    user,
    isAdmin,
    authChecked,
    loggingIn,
    permissions,
    hasPermission,
    checkAuth,
    login,
    register,
    logout,
  }
})
