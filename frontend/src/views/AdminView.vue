<script setup lang="ts">
/**
 * Admin dashboard shell: a topbar, the collapsible sidebar, and a content area
 * that renders the active admin tab via <router-view> (the tab is selected by
 * the matched `/admin/...` child route). The four admin modals (winner
 * verification, halftime prompt, end-game, card preview) live here so they
 * overlay any tab.
 *
 * Loads the initial admin data set once when the dashboard mounts (this used to
 * happen in the App shell after a successful login; now it runs whenever the
 * admin area is entered, including via a direct link / refresh). Logout
 * disconnects the WebSocket implicitly (App's route watcher) and returns home.
 */
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AdminSidebar from '@/components/admin/AdminSidebar.vue'
import WinnerVerifyModal from '@/components/admin/WinnerVerifyModal.vue'
import HalftimePromptModal from '@/components/admin/HalftimePromptModal.vue'
import EndGameModal from '@/components/admin/EndGameModal.vue'
import CardPreviewModal from '@/components/admin/CardPreviewModal.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import FormField from '@/components/common/ui/FormField.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'
import { useRafflesStore } from '@/stores/raffles'
import { useUiStore } from '@/stores/ui'
import { endpoints } from '@/lib/endpoints'
import { listPasskeys, registerPasskey, deletePasskey, passkeysSupported } from '@/lib/passkeys'
import type { AccountTokenInfoResponse, Passkey } from '@/types/api'

const router = useRouter()
const auth = useAuthStore()
const app = useAppStore()
const game = useGameStore()
const cards = useCardsStore()
const patterns = usePatternsStore()
const raffles = useRafflesStore()
const ui = useUiStore()

onMounted(async () => {
  // Load the core admin data set, but only what this account may access (admins
  // can read everything). Settings are public, so they always load. allSettled
  // keeps one 403 from aborting the rest.
  try {
    const loads: Promise<unknown>[] = [app.loadSettings()]
    if (auth.hasPermission('bingo-cards')) loads.push(cards.loadCards())
    if (auth.hasPermission('bingo-patterns')) loads.push(patterns.loadPatterns())
    if (auth.hasPermission('bingo-game')) loads.push(game.loadGameState())
    if (auth.hasPermission('teahouse-raffles')) loads.push(raffles.loadRaffles())
    await Promise.allSettled(loads)
    game.drawDelay = parseInt(app.settings.default_draw_delay) || 0
  } catch {
    /* show the dashboard even if a data load failed */
  }
})

// ── Mobile nav drawer ────────────────────────────────────────────────────────
// On narrow viewports the sidebar collapses into a hamburger-triggered drawer
// (see app.css). Selecting a nav item navigates, so close the drawer on every
// route change; tapping the backdrop closes it too.
const navOpen = ref(false)
watch(
  () => router.currentRoute.value.fullPath,
  () => {
    navOpen.value = false
  },
)

// The hamburger toggle (and drawer backdrop) only exist on narrow viewports where
// the sidebar collapses into a drawer. We drive this from matchMedia and `v-if`
// rather than CSS `display`, so a custom theme's generic button styling can't
// accidentally re-show the button on desktop (where the sidebar is always shown).
const NARROW_QUERY = '(max-width: 768px)'
const isNarrow = ref(false)
let navMql: MediaQueryList | null = null
function syncIsNarrow(): void {
  isNarrow.value = !!navMql?.matches
  if (!isNarrow.value) navOpen.value = false // back to desktop → ensure the drawer is closed
}
onMounted(() => {
  navMql = window.matchMedia(NARROW_QUERY)
  syncIsNarrow()
  navMql.addEventListener('change', syncIsNarrow)
})
onBeforeUnmount(() => navMql?.removeEventListener('change', syncIsNarrow))

/** Log out, then return home (App's route watcher disconnects the WebSocket). */
async function logout(): Promise<void> {
  await auth.logout()
  void router.push({ name: 'home' })
}

// ── Change-password modal (available to every logged-in account) ──────────────
const showChangePw = ref(false)
const currentPw = ref('')
const newPw = ref('')
const confirmPw = ref('')
const pwError = ref('')
const savingPw = ref(false)

function openChangePw(): void {
  currentPw.value = ''
  newPw.value = ''
  confirmPw.value = ''
  pwError.value = ''
  showChangePw.value = true
}

/** Sidebar "Change Password" → close the mobile drawer, then open the modal. */
function onChangePassword(): void {
  navOpen.value = false
  openChangePw()
}

async function submitChangePw(): Promise<void> {
  pwError.value = ''
  if (newPw.value.length < 8) {
    pwError.value = 'New password must be at least 8 characters.'
    return
  }
  if (newPw.value !== confirmPw.value) {
    pwError.value = 'New passwords do not match.'
    return
  }
  savingPw.value = true
  try {
    await endpoints.account.changePassword(currentPw.value, newPw.value)
    ui.notify('Password changed', 'success')
    showChangePw.value = false
  } catch (e) {
    pwError.value = (e as Error).message
  } finally {
    savingPw.value = false
  }
}

// ── Access-token modal (personal access token for external API clients) ───────
const showToken = ref(false)
const tokenInfo = ref<AccountTokenInfoResponse | null>(null)
const tokenLoading = ref(false)
const tokenBusy = ref(false)
const tokenError = ref('')
/** The freshly generated token plaintext — shown exactly once, then forgotten. */
const newToken = ref('')

/** Formats a SQLite UTC timestamp ("YYYY-MM-DD HH:MM:SS") for local display. */
function fmtTokenTime(ts: string): string {
  if (!ts) return 'Never'
  const d = new Date(ts.replace(' ', 'T') + 'Z')
  return Number.isNaN(d.getTime()) ? ts : d.toLocaleString()
}

/** Sidebar "Access Token" → close the mobile drawer, open the modal, load info. */
async function onAccessToken(): Promise<void> {
  navOpen.value = false
  newToken.value = ''
  tokenError.value = ''
  tokenInfo.value = null
  showToken.value = true
  tokenLoading.value = true
  try {
    tokenInfo.value = await endpoints.account.tokenInfo()
  } catch (e) {
    tokenError.value = (e as Error).message
  } finally {
    tokenLoading.value = false
  }
}

/** Generate (or replace) the token; reveals the plaintext once and refreshes info. */
async function generateToken(): Promise<void> {
  tokenBusy.value = true
  tokenError.value = ''
  try {
    const res = await endpoints.account.generateToken()
    newToken.value = res.token
    tokenInfo.value = await endpoints.account.tokenInfo()
  } catch (e) {
    tokenError.value = (e as Error).message
  } finally {
    tokenBusy.value = false
  }
}

/** Revoke the token (an external client loses access until a new one is generated). */
async function revokeToken(): Promise<void> {
  tokenBusy.value = true
  tokenError.value = ''
  try {
    await endpoints.account.revokeToken()
    newToken.value = ''
    tokenInfo.value = await endpoints.account.tokenInfo()
    ui.notify('Token revoked', 'success')
  } catch (e) {
    tokenError.value = (e as Error).message
  } finally {
    tokenBusy.value = false
  }
}

/** Copy the freshly generated token to the clipboard. */
async function copyToken(): Promise<void> {
  try {
    await navigator.clipboard.writeText(newToken.value)
    ui.notify('Token copied to clipboard', 'success')
  } catch {
    ui.notify('Copy failed — select the token and copy it manually', 'error')
  }
}

// ── Passkeys modal (WebAuthn credentials — add / list / remove) ───────────────
const showPasskeys = ref(false)
const passkeys = ref<Passkey[]>([])
const passkeysLoading = ref(false)
const passkeyBusy = ref(false)
const passkeyError = ref('')
const newPasskeyName = ref('')

/** Sidebar "Add Passkey" → close the drawer, open the modal, load the list. */
async function onManagePasskeys(): Promise<void> {
  navOpen.value = false
  passkeyError.value = ''
  newPasskeyName.value = ''
  showPasskeys.value = true
  passkeysLoading.value = true
  try {
    passkeys.value = (await listPasskeys()).passkeys
  } catch (e) {
    passkeyError.value = (e as Error).message
  } finally {
    passkeysLoading.value = false
  }
}

/** Create a new passkey (prompts the browser's authenticator). */
async function addPasskey(): Promise<void> {
  if (!passkeysSupported()) {
    passkeyError.value = 'This browser does not support passkeys.'
    return
  }
  passkeyBusy.value = true
  passkeyError.value = ''
  try {
    passkeys.value = (await registerPasskey(newPasskeyName.value.trim() || 'Passkey')).passkeys
    newPasskeyName.value = ''
    ui.notify('Passkey added', 'success')
  } catch (e) {
    passkeyError.value = (e as Error).message
  } finally {
    passkeyBusy.value = false
  }
}

/** Remove a passkey. */
async function removePasskey(id: number): Promise<void> {
  passkeyBusy.value = true
  passkeyError.value = ''
  try {
    passkeys.value = (await deletePasskey(id)).passkeys
    ui.notify('Passkey removed', 'success')
  } catch (e) {
    passkeyError.value = (e as Error).message
  } finally {
    passkeyBusy.value = false
  }
}

/** Formats a SQLite UTC timestamp for local display ("Never" when empty). */
function fmtPasskeyTime(ts: string): string {
  if (!ts) return 'Never'
  const d = new Date(ts.replace(' ', 'T') + 'Z')
  return Number.isNaN(d.getTime()) ? ts : d.toLocaleString()
}
</script>

<template>
  <div>
    <div class="topbar">
      <div class="flex gap-sm">
        <button
          v-if="isNarrow"
          class="btn-neutral btn-sm admin-nav-toggle"
          :aria-expanded="navOpen"
          aria-label="Toggle navigation menu"
          @click="navOpen = !navOpen"
        >
          <font-awesome-icon :icon="['fad', 'bars']" />
        </button>
        <h2><font-awesome-icon :icon="['fad', 'gear']" /> Admin Dashboard</h2>
      </div>
      <div class="topbar-actions">
        <span v-if="auth.user" class="topbar-user text-dim">
          <font-awesome-icon :icon="['fad', 'user']" /> {{ auth.user.username }}
        </span>
      </div>
    </div>

    <div class="admin-layout">
      <!-- Backdrop behind the mobile drawer; only exists in the narrow drawer mode. -->
      <div v-if="isNarrow && navOpen" class="admin-nav-backdrop" @click="navOpen = false"></div>

      <AdminSidebar
        :class="{ 'is-open': navOpen }"
        @access-token="onAccessToken"
        @change-password="onChangePassword"
        @manage-passkeys="onManagePasskeys"
        @logout="logout"
      />

      <div class="admin-content">
        <div class="admin-content-inner content-container">
          <router-view />
        </div>
      </div>
    </div>

    <!-- Modals (overlay any tab) -->
    <WinnerVerifyModal />
    <HalftimePromptModal />
    <EndGameModal />
    <CardPreviewModal />

    <ModalOverlay
      v-if="showChangePw"
      aria-label="Change password"
      box-style="max-width: 420px"
      @close="showChangePw = false"
    >
      <h3 class="mt-0"><font-awesome-icon :icon="['fad', 'lock']" /> Change Password</h3>
      <form autocomplete="off" @submit.prevent="submitChangePw">
        <FormField label="Current password" html-for="cur-pw">
          <input id="cur-pw" v-model="currentPw" type="password" autocomplete="current-password" />
        </FormField>
        <FormField label="New password" html-for="new-pw">
          <input
            id="new-pw"
            v-model="newPw"
            type="password"
            autocomplete="new-password"
            placeholder="At least 8 characters"
          />
        </FormField>
        <FormField label="Confirm new password" html-for="confirm-pw">
          <input id="confirm-pw" v-model="confirmPw" type="password" autocomplete="new-password" />
        </FormField>
        <p v-if="pwError" class="error-msg">{{ pwError }}</p>
        <div class="modal-actions">
          <button
            type="button"
            class="btn-neutral"
            :disabled="savingPw"
            @click="showChangePw = false"
          >
            Cancel
          </button>
          <button type="submit" class="btn-action" :disabled="savingPw">
            <LoadingSpinner v-if="savingPw" label="Saving…" />
            <template v-else>Change Password</template>
          </button>
        </div>
      </form>
    </ModalOverlay>

    <ModalOverlay
      v-if="showToken"
      aria-label="Personal access token"
      box-style="max-width: 540px"
      @close="showToken = false"
    >
      <h3 class="mt-0"><font-awesome-icon :icon="['fad', 'key']" /> Access Token</h3>
      <p class="text-dim pat-intro">
        A personal access token lets an external client — such as the FFXIV plugin — sign in to this
        server as you, with your exact permissions. Treat it like a password: anyone who has it can
        act as your account.
      </p>

      <p v-if="tokenLoading"><LoadingSpinner label="Loading…" /></p>

      <!-- Freshly generated token: revealed exactly once. -->
      <template v-else-if="newToken">
        <FormField
          label="Your new token — copy it now, it won't be shown again"
          html-for="pat-value"
        >
          <div class="pat-reveal">
            <input
              id="pat-value"
              class="pat-input"
              :value="newToken"
              readonly
              @focus="($event.target as HTMLInputElement).select()"
            />
            <button type="button" class="btn-neutral" @click="copyToken">
              <font-awesome-icon :icon="['fas', 'copy']" /> Copy
            </button>
          </div>
        </FormField>
        <p class="text-dim pat-note">
          Paste it into the plugin's settings. Generating or revoking a token later invalidates this
          one immediately.
        </p>
        <div class="modal-actions">
          <button type="button" class="btn-action" @click="showToken = false">Done</button>
        </div>
      </template>

      <!-- Existing-token metadata + controls. -->
      <template v-else>
        <template v-if="tokenInfo?.has_token">
          <dl class="pat-meta">
            <dt class="text-dim">Token</dt>
            <dd>
              <code>{{ tokenInfo.prefix }}…</code>
            </dd>
            <dt class="text-dim">Created</dt>
            <dd>{{ fmtTokenTime(tokenInfo.created_at) }}</dd>
            <dt class="text-dim">Last used</dt>
            <dd>{{ fmtTokenTime(tokenInfo.last_used_at) }}</dd>
          </dl>
          <p class="text-dim pat-note">
            For security the token itself can't be shown again. If you've lost it, regenerate to get
            a new one — the old token stops working immediately.
          </p>
        </template>
        <p v-else>You don't have an access token yet. Generate one to connect a plugin.</p>

        <p v-if="tokenError" class="error-msg">{{ tokenError }}</p>

        <div class="modal-actions">
          <button
            v-if="tokenInfo?.has_token"
            type="button"
            class="btn-danger"
            :disabled="tokenBusy"
            @click="revokeToken"
          >
            <font-awesome-icon :icon="['fas', 'trash']" /> Revoke
          </button>
          <button type="button" class="btn-action" :disabled="tokenBusy" @click="generateToken">
            <LoadingSpinner v-if="tokenBusy" label="Working…" />
            <template v-else>
              <font-awesome-icon
                :icon="tokenInfo?.has_token ? ['fas', 'rotate'] : ['fad', 'key']"
              />
              {{ tokenInfo?.has_token ? 'Regenerate' : 'Generate token' }}
            </template>
          </button>
        </div>
      </template>
    </ModalOverlay>

    <ModalOverlay
      v-if="showPasskeys"
      aria-label="Passkeys"
      box-style="max-width: 520px"
      @close="showPasskeys = false"
    >
      <h3 class="mt-0"><font-awesome-icon :icon="['fad', 'user-key']" /> Passkeys</h3>
      <p class="text-dim pat-intro">
        A passkey lets you sign in with your device's fingerprint, face, PIN, or a security key
        instead of your password. You can add more than one (e.g. one per device); your password
        still works too.
      </p>

      <p v-if="passkeysLoading"><LoadingSpinner label="Loading…" /></p>
      <template v-else>
        <ul v-if="passkeys.length" class="passkey-list">
          <li v-for="pk in passkeys" :key="pk.id" class="passkey-row">
            <span class="passkey-info">
              <font-awesome-icon :icon="['fad', 'user-key']" />
              <strong>{{ pk.name }}</strong>
              <span class="text-dim text-xs">
                Added {{ fmtPasskeyTime(pk.created_at) }} · Last used
                {{ fmtPasskeyTime(pk.last_used_at) }}
              </span>
            </span>
            <button
              type="button"
              class="btn-danger btn-sm"
              :disabled="passkeyBusy"
              :aria-label="`Remove passkey ${pk.name}`"
              @click="removePasskey(pk.id)"
            >
              <font-awesome-icon :icon="['fas', 'trash']" />
            </button>
          </li>
        </ul>
        <p v-else class="text-dim">No passkeys yet.</p>

        <form class="passkey-add" @submit.prevent="addPasskey">
          <FormField label="Name this passkey (optional)" html-for="pk-name">
            <input
              id="pk-name"
              v-model="newPasskeyName"
              placeholder="e.g. My Laptop"
              maxlength="60"
            />
          </FormField>
          <p v-if="passkeyError" class="error-msg">{{ passkeyError }}</p>
          <div class="modal-actions">
            <button
              type="button"
              class="btn-neutral"
              :disabled="passkeyBusy"
              @click="showPasskeys = false"
            >
              Close
            </button>
            <button type="submit" class="btn-action" :disabled="passkeyBusy">
              <LoadingSpinner v-if="passkeyBusy" label="Working…" />
              <template v-else>
                <font-awesome-icon :icon="['fad', 'user-key']" /> Add Passkey
              </template>
            </button>
          </div>
        </form>
      </template>
    </ModalOverlay>
  </div>
</template>

<style scoped>
.topbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.topbar-user {
  font-size: 0.85rem;
}
.modal-actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 16px;
}
.pat-intro {
  font-size: 0.9rem;
}
.pat-note {
  font-size: 0.85rem;
}
.pat-reveal {
  display: flex;
  gap: 8px;
}
.pat-input {
  flex: 1;
  font-family: monospace;
}
.pat-meta {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 4px 16px;
  margin: 0 0 12px;
}
.pat-meta dt {
  font-size: 0.85rem;
}
.pat-meta dd {
  margin: 0;
}
.passkey-list {
  list-style: none;
  margin: 0 0 16px;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.passkey-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-radius: var(--radius-sm, 6px);
  background: color-mix(in srgb, var(--color-text) 6%, transparent);
}
.passkey-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.passkey-add {
  margin-top: 8px;
}
</style>
