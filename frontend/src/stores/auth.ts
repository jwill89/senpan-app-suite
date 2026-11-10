/**
 * Auth store: signing in and out, signing up, and who is signed in.
 *
 * `isAdmin` and `user` are the source of truth for the router's admin-route
 * guard and per-page permission gating. They are refreshed via `checkAuth()`
 * (GET /api/v1/me) on first navigation to an admin route and set directly on
 * sign-in and sign-out. The actual data-loading after sign-in is orchestrated by
 * the Admin route setup to keep this store focused.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, unwrap, type Account } from '@/api/client'
import { signInWithPasskey } from '@/lib/passkeys'

/** What the sign-up page says once an account is asked for. */
export const SIGN_UP_DONE =
  'Account created. An administrator must activate it before you can log in.'

export const useAuthStore = defineStore('auth', () => {
  const authError = ref('')
  /** The signed-in account (null when nobody is). */
  const user = ref<Account | null>(null)
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
    if (!user.value) {
      return false
    }
    if (user.value.isAdmin) {
      return true
    }
    return user.value.permissions.includes(key)
  }

  /** Applies the signed-in account to the store. */
  function setUser(u: Account | null): void {
    user.value = u
    isAdmin.value = !!u?.isAdmin
  }

  /**
   * Clears the cached session in place - drops the user and admin flag while
   * keeping `authChecked` true. Called by the global 401 handler when the server
   * reports the admin session is missing/expired, so both the router guard and
   * per-page permission checks (which read `user`) see a logged-out state.
   */
  function clearSession(): void {
    user.value = null
    isAdmin.value = false
    authChecked.value = true
  }

  /**
   * Asks the server who is signed in and caches it. Returns whether anyone is.
   * Used by the router guard for /admin routes.
   */
  async function checkAuth(): Promise<boolean> {
    try {
      setUser((await unwrap(api.GET('/me'))).account)
    } catch {
      setUser(null)
    } finally {
      authChecked.value = true
    }
    return !!user.value
  }

  /** Runs a sign-in: true on success; sets authError and returns false on failure. */
  async function signingIn(attempt: () => Promise<Account | null>): Promise<boolean> {
    authError.value = ''
    loggingIn.value = true
    try {
      setUser(await attempt())
      authChecked.value = true
      return !!user.value
    } catch (e) {
      authError.value = (e as Error).message
      return false
    } finally {
      loggingIn.value = false
    }
  }

  /** Signs in with a username and password (and the Turnstile result when on). */
  function login(username: string, password: string, turnstileToken?: string): Promise<boolean> {
    return signingIn(async () => {
      const me = await unwrap(
        api.POST('/auth/login', { body: { username, password, turnstileToken } }),
      )
      return me.account
    })
  }

  /** Signs in with a passkey (a discoverable WebAuthn credential). */
  function loginWithPasskey(): Promise<boolean> {
    return signingIn(signInWithPasskey)
  }

  /**
   * Asks for an account (the hidden sign-up page). Returns the message to show on
   * success; sets authError and returns null on failure. The server answers the same
   * whether or not the name was free.
   */
  async function register(
    username: string,
    password: string,
    turnstileToken?: string,
  ): Promise<string | null> {
    authError.value = ''
    loggingIn.value = true
    try {
      await unwrap(api.POST('/auth/register', { body: { username, password, turnstileToken } }))
      return SIGN_UP_DONE
    } catch (e) {
      authError.value = (e as Error).message
      return null
    } finally {
      loggingIn.value = false
    }
  }

  /** Signs out (best-effort; ignores errors). */
  async function logout(): Promise<void> {
    try {
      await api.POST('/auth/logout')
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
    clearSession,
    checkAuth,
    login,
    loginWithPasskey,
    register,
    logout,
  }
})
