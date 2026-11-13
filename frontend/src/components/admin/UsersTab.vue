<script setup lang="ts">
/**
 * Admin Users tab (System section) - manages accounts and their per-page
 * permissions. Lists accounts in the shared DataTable with status/role badges and
 * the Discord account each signs in with, and per-row actions to rename,
 * activate/deactivate, toggle admin, edit permissions, set a password, link or
 * unlink Discord, and delete.
 *
 * Discord links: "Discord Sign-up Link" makes a link for someone to make an
 * account with their Discord account; an account's Discord dialog makes a link
 * that links whoever opens it to that account. Each is shown once, when made.
 *
 * The seeded "admin" account is protected: its modify actions are hidden here
 * (it changes its own password via the topbar "Change Password" dialog, and only
 * it links its own Discord account), and the backend rejects any such change too.
 */
import { computed, onMounted, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import FormField from '@/components/common/ui/FormField.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import DataTable, { type DataColumn } from '@/components/common/ui/DataTable.vue'
import { useUsersStore, PROTECTED_ADMIN } from '@/stores/users'
import { useUiStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'
import { ADMIN_PERMISSIONS } from '@/lib/constants'
import { formatServerTimestamp } from '@/lib/datetime'
import type { DiscordAccount, DiscordInvite, User } from '@/api/client'

const users = useUsersStore()
const ui = useUiStore()
const auth = useAuthStore()

/** Free-text filter on username. */
const search = ref('')

/** Absolute URL of the hidden registration page (shared with new users). */
const registrationUrl = `${window.location.origin}/admin/register`

/** Copies the registration link to the clipboard (falls back to a toast). */
async function copyRegistrationLink(): Promise<void> {
  try {
    await navigator.clipboard.writeText(registrationUrl)
    ui.notify('Registration link copied to clipboard', 'success')
  } catch {
    ui.notify(registrationUrl, 'info')
  }
}

const userColumns: DataColumn[] = [
  { key: 'username', label: 'Username', sortable: false },
  { key: 'discord', label: 'Discord', sortable: false },
  { key: 'status', label: 'Status' },
  { key: 'role', label: 'Role' },
  { key: 'permissions', label: 'Permissions' },
  { key: 'lastLogin', label: 'Last Login' },
  { key: 'actions', label: '', align: 'right' },
]

/** Whether a user is the protected bootstrap admin (no destructive actions). */
function isProtected(u: User): boolean {
  return u.username === PROTECTED_ADMIN
}

/** Whether an account's Discord dialog is offered: the protected admin's to itself only. */
function canManageDiscord(u: User): boolean {
  if (isProtected(u) && auth.user?.username !== PROTECTED_ADMIN) {
    return false
  }
  return users.discordSignIn || !!u.discord
}

/** A Discord account as the table shows it: its display name, then its username. */
function discordName(d: DiscordAccount): string {
  return d.displayName ? `${d.displayName} (@${d.username})` : `@${d.username}`
}

const displayedUsers = computed(() => {
  const term = search.value.trim().toLowerCase()
  const rows = term
    ? users.users.filter((u) => u.username.toLowerCase().includes(term))
    : users.users.slice()
  // Admins first, then by username.
  return rows.sort((a, b) => {
    if (a.isAdmin !== b.isAdmin) {
      return a.isAdmin ? -1 : 1
    }
    return a.username.localeCompare(b.username)
  })
})

/** Permission keys grouped by section, for the editor's checkbox layout. */
const permissionGroups = computed(() => {
  const groups: { section: string; perms: { key: string; label: string }[] }[] = []
  for (const p of ADMIN_PERMISSIONS) {
    let group = groups.find((g) => g.section === p.section)
    if (!group) {
      group = { section: p.section, perms: [] }
      groups.push(group)
    }
    group.perms.push({ key: p.key, label: p.label })
  }
  return groups
})

/** Formats a user's last-login timestamp for the table, or "Never". */
function formatLastLogin(ts: string | null): string {
  return ts ? formatServerTimestamp(ts) : 'Never'
}

/** Human summary of a user's access for the table cell. */
function permissionSummary(u: User): string {
  if (u.isAdmin) {
    return 'Full access (admin)'
  }
  if (!u.permissions.length) {
    return 'None'
  }
  return `${u.permissions.length} page${u.permissions.length === 1 ? '' : 's'}`
}

// -- Edit-user modal ----------------------------------------------------------
// One modal consolidates every per-account action (activate/deactivate, grant/
// revoke admin, page permissions, set password) so the table row needs only two
// buttons (Edit + Delete) instead of five. Changes are collected as a draft and
// applied together on Save (users.updateUser sends only what changed).
const editUserId = ref<number | null>(null)
const editUsername = ref('')
const editActive = ref(false)
const editAdmin = ref(false)
const permDraft = ref<Set<string>>(new Set())
const editPassword = ref('')
const editConfirm = ref('')
const editError = ref('')
const savingUser = ref(false)

/** The account currently being edited (live from the store, for the title). */
const editUser = computed(() => users.users.find((u) => u.id === editUserId.value) ?? null)

function openEdit(u: User): void {
  editUserId.value = u.id
  editUsername.value = u.username
  editActive.value = u.isActive
  editAdmin.value = u.isAdmin
  permDraft.value = new Set(u.permissions)
  editPassword.value = ''
  editConfirm.value = ''
  editError.value = ''
}
function closeEdit(): void {
  editUserId.value = null
}
function togglePerm(key: string): void {
  if (permDraft.value.has(key)) {
    permDraft.value.delete(key)
  } else {
    permDraft.value.add(key)
  }
  // Reassign to trigger reactivity on the Set.
  permDraft.value = new Set(permDraft.value)
}
async function saveUser(): Promise<void> {
  editError.value = ''
  if (editUserId.value === null) {
    return
  }
  if (!editUsername.value.trim()) {
    editError.value = 'A username needs more than spaces.'
    return
  }
  // Password is optional; validate only when one is being set.
  if (editPassword.value || editConfirm.value) {
    if (editPassword.value.length < 8) {
      editError.value = 'Password must be at least 8 characters.'
      return
    }
    if (editPassword.value !== editConfirm.value) {
      editError.value = 'Passwords do not match.'
      return
    }
  }
  savingUser.value = true
  const ok = await users.updateUser(
    editUserId.value,
    {
      username: editUsername.value,
      isActive: editActive.value,
      isAdmin: editAdmin.value,
      permissions: [...permDraft.value],
    },
    editPassword.value || undefined,
  )
  savingUser.value = false
  if (ok) {
    editUserId.value = null
  }
}

// -- Discord dialog -----------------------------------------------------------
// With an account: the Discord account linked to it, a link that links one, and
// unlinking. Without: a link for someone to sign up with Discord. A link is shown
// once, when it is made; unlinking asks again inside the dialog.
const discordOpen = ref(false)
const discordUserId = ref<number | null>(null)
const madeLink = ref<DiscordInvite | null>(null)
const makingLink = ref(false)
const confirmUnlink = ref(false)
const unlinking = ref(false)

/** The account the dialog is for (live from the store), null for a sign-up link. */
const discordUser = computed(() =>
  discordUserId.value === null
    ? null
    : (users.users.find((u) => u.id === discordUserId.value) ?? null),
)

function openDiscord(u: User | null): void {
  discordUserId.value = u?.id ?? null
  madeLink.value = null
  confirmUnlink.value = false
  discordOpen.value = true
}

function closeDiscord(): void {
  discordOpen.value = false
}

async function makeLink(): Promise<void> {
  makingLink.value = true
  madeLink.value = await users.createDiscordInvite(discordUserId.value ?? undefined)
  makingLink.value = false
}

async function copyLink(): Promise<void> {
  if (!madeLink.value) {
    return
  }
  try {
    await navigator.clipboard.writeText(madeLink.value.url)
    ui.notify('Link copied to clipboard', 'success')
  } catch {
    ui.notify('Copy the link from the box', 'info')
  }
}

async function unlink(): Promise<void> {
  if (discordUserId.value === null) {
    return
  }
  unlinking.value = true
  await users.unlinkDiscord(discordUserId.value)
  unlinking.value = false
  confirmUnlink.value = false
}

onMounted(() => {
  void users.loadUsers()
  void users.loadDiscordSignIn()
})
</script>

<template>
  <div class="tab-body">
    <AdminPanel>
      <div class="flex-toolbar mb-12">
        <h3 class="m-0"><font-awesome-icon :icon="['fad', 'users-gear']" /> Users</h3>
        <button
          type="button"
          class="btn-view btn-sm push-right"
          title="Copy the hidden registration link to share with a new user"
          @click="copyRegistrationLink"
        >
          <font-awesome-icon :icon="['fas', 'copy']" /> Copy Registration Link
        </button>
        <button
          v-if="users.discordSignIn"
          type="button"
          class="btn-action btn-sm"
          title="Make a one-time link for someone to sign up with their Discord account"
          @click="openDiscord(null)"
        >
          <font-awesome-icon :icon="['fab', 'discord']" /> Discord Sign-up Link
        </button>
      </div>

      <p class="text-muted text-xs mb-12">
        Accounts sign in with a username and password, a passkey, or a linked Discord account. New
        sign-ups via the shared registration link start <strong>inactive</strong> - activate one
        here and grant the pages it should access. A Discord sign-up link makes an active account
        with no pages until you grant them. Admins have full access to everything.
      </p>

      <SearchInput
        v-if="users.users.length"
        v-model="search"
        class="mb-12"
        placeholder="Search users by name..."
        aria-label="Search users by name"
      />

      <LoadingSpinner
        v-if="users.loading && users.users.length === 0"
        block
        label="Loading users..."
      />

      <DataTable
        v-else-if="users.users.length"
        :columns="userColumns"
        :rows="displayedUsers"
        row-key="id"
      >
        <template #cell-username="{ row }">
          <span class="code-highlight">{{ row.username }}</span>
        </template>
        <template #cell-discord="{ row }">
          <span v-if="row.discord">
            <font-awesome-icon :icon="['fab', 'discord']" /> {{ discordName(row.discord) }}
          </span>
          <span v-else class="text-muted">Not linked</span>
        </template>
        <template #cell-status="{ row }">
          <span :class="row.isActive ? 'badge-active' : 'badge-inactive'">
            {{ row.isActive ? 'Active' : 'Inactive' }}
          </span>
        </template>
        <template #cell-role="{ row }">
          <span :class="row.isAdmin ? 'badge-admin' : 'text-muted'">
            {{ row.isAdmin ? 'Admin' : 'User' }}
          </span>
        </template>
        <template #cell-permissions="{ row }">
          <span class="text-muted">{{ permissionSummary(row) }}</span>
        </template>
        <template #cell-lastLogin="{ row }">
          <span class="text-muted">{{ formatLastLogin(row.lastLoginAt) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <button
              v-if="canManageDiscord(row)"
              type="button"
              class="btn-view btn-sm"
              title="Link or unlink this account's Discord account"
              @click="openDiscord(row)"
            >
              <font-awesome-icon :icon="['fab', 'discord']" /> Discord
            </button>
            <template v-if="isProtected(row)">
              <span class="text-muted text-xs">Protected account</span>
            </template>
            <template v-else>
              <button
                type="button"
                class="btn-view btn-sm"
                title="Edit this account"
                @click="openEdit(row)"
              >
                <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
              </button>
              <button
                type="button"
                class="btn-danger btn-sm"
                title="Delete this account"
                @click="users.deleteUser(row.id, row.username)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" /> Delete
              </button>
            </template>
          </div>
        </template>
        <template #empty>
          <p class="text-muted ta-center p-20">No users match “{{ search }}”.</p>
        </template>
      </DataTable>

      <EmptyState v-else-if="!users.loading" text="No accounts yet." />
    </AdminPanel>

    <!-- Edit user (account + permissions + password in one) -->
    <ModalOverlay
      v-if="editUser"
      aria-label="Edit user"
      box-style="max-width: 560px"
      @close="closeEdit"
    >
      <h3>
        <font-awesome-icon :icon="['fad', 'users-gear']" /> Edit User -
        <span class="code-highlight">{{ editUser.username }}</span>
      </h3>

      <!-- Account -->
      <div class="edit-section">
        <FormField label="Username" html-for="edit-username">
          <input
            id="edit-username"
            v-model="editUsername"
            type="text"
            maxlength="32"
            autocomplete="off"
          />
        </FormField>
        <label class="perm-option">
          <input v-model="editActive" type="checkbox" /> Account active
        </label>
        <label class="perm-option">
          <input v-model="editAdmin" type="checkbox" /> Administrator (full access to every page)
        </label>
      </div>

      <!-- Page permissions -->
      <div class="edit-section">
        <h4 class="perm-group-title">Page permissions</h4>
        <p v-if="editAdmin" class="text-muted text-xs m-0">
          Admins have full access to every page - per-page permissions don't apply.
        </p>
        <div v-else class="perm-groups">
          <div v-for="group in permissionGroups" :key="group.section">
            <h4 class="perm-group-title">{{ group.section }}</h4>
            <label v-for="perm in group.perms" :key="perm.key" class="perm-option">
              <input
                type="checkbox"
                :checked="permDraft.has(perm.key)"
                @change="togglePerm(perm.key)"
              />
              {{ perm.label }}
            </label>
          </div>
        </div>
      </div>

      <!-- Password (optional) -->
      <div class="edit-section">
        <h4 class="perm-group-title">Set a new password (optional)</h4>
        <FormField label="New password" html-for="edit-pw">
          <input
            id="edit-pw"
            v-model="editPassword"
            type="password"
            autocomplete="new-password"
            placeholder="Leave blank to keep current"
          />
        </FormField>
        <FormField label="Confirm password" html-for="edit-pw-confirm">
          <input
            id="edit-pw-confirm"
            v-model="editConfirm"
            type="password"
            autocomplete="new-password"
          />
        </FormField>
      </div>

      <p v-if="editError" class="error-msg">{{ editError }}</p>
      <div class="modal-actions">
        <button type="button" class="btn-neutral" :disabled="savingUser" @click="closeEdit">
          Cancel
        </button>
        <button type="button" class="btn-action" :disabled="savingUser" @click="saveUser">
          <LoadingSpinner v-if="savingUser" label="Saving..." />
          <template v-else>Save Changes</template>
        </button>
      </div>
    </ModalOverlay>

    <!-- Discord: an account's link and unlink, or a sign-up link -->
    <ModalOverlay
      v-if="discordOpen"
      aria-label="Discord sign-in"
      box-style="max-width: 560px"
      @close="closeDiscord"
    >
      <h3>
        <font-awesome-icon :icon="['fab', 'discord']" />
        <template v-if="discordUser">
          Discord - <span class="code-highlight">{{ discordUser.username }}</span>
        </template>
        <template v-else>Sign Up with Discord</template>
      </h3>

      <!-- A link just made: shown this once. -->
      <template v-if="madeLink">
        <FormField label="The link - copy it now, it won't be shown again" html-for="discord-link">
          <div class="copy-field">
            <input
              id="discord-link"
              class="copy-field-input"
              :value="madeLink.url"
              readonly
              @focus="($event.target as HTMLInputElement).select()"
            />
            <button type="button" class="btn-neutral" @click="copyLink">
              <font-awesome-icon :icon="['fas', 'copy']" /> Copy
            </button>
          </div>
        </FormField>
        <p class="text-muted text-xs mb-12">
          It works once, until {{ formatServerTimestamp(madeLink.expiresAt) }}. Whoever opens it
          {{ discordUser ? 'signs in to this account' : 'gets an account' }}, so send it only to the
          person it is for.
        </p>
        <div class="modal-actions">
          <button type="button" class="btn-action" @click="closeDiscord">Done</button>
        </div>
      </template>

      <!-- An account: its Discord account, a link to link one, unlinking. -->
      <template v-else-if="discordUser">
        <p v-if="discordUser.discord" class="mb-12">
          Linked to <strong>{{ discordName(discordUser.discord) }}</strong> since
          {{ formatServerTimestamp(discordUser.discord.linkedAt) }}.
        </p>
        <p v-else class="text-muted mb-12">No Discord account is linked.</p>
        <p v-if="confirmUnlink && discordUser.discord" class="mb-12">
          Unlink <strong>{{ discordName(discordUser.discord) }}</strong>? This account will no
          longer sign in with Discord.
        </p>
        <p v-else-if="users.discordSignIn" class="text-muted text-xs mb-12">
          A link lets the person who uses this account link their Discord account to it and sign in
          with Discord. Whoever opens it is signed in to
          <strong>{{ discordUser.username }}</strong>, so send it only to them. It works once, for a
          day, and replaces a link made before.
        </p>
        <div class="modal-actions">
          <template v-if="confirmUnlink && discordUser.discord">
            <button
              type="button"
              class="btn-neutral"
              :disabled="unlinking"
              @click="confirmUnlink = false"
            >
              Keep It
            </button>
            <button type="button" class="btn-danger" :disabled="unlinking" @click="unlink">
              <LoadingSpinner v-if="unlinking" label="Unlinking..." />
              <template v-else>Unlink</template>
            </button>
          </template>
          <template v-else>
            <button type="button" class="btn-neutral" @click="closeDiscord">Close</button>
            <button
              v-if="discordUser.discord"
              type="button"
              class="btn-danger"
              @click="confirmUnlink = true"
            >
              Unlink
            </button>
            <button
              v-if="users.discordSignIn"
              type="button"
              class="btn-action"
              :disabled="makingLink"
              @click="makeLink"
            >
              <LoadingSpinner v-if="makingLink" label="Making..." />
              <template v-else>Make Link</template>
            </button>
          </template>
        </div>
      </template>

      <!-- A sign-up link. -->
      <template v-else>
        <p class="text-muted mb-12">
          A link for someone to make an account with their Discord account and sign in with it. The
          account is active, named after their Discord username (rename it under Edit), with no
          pages until you grant them. The link works once, for a week, so send it only to the person
          it is for.
        </p>
        <div class="modal-actions">
          <button type="button" class="btn-neutral" @click="closeDiscord">Cancel</button>
          <button type="button" class="btn-action" :disabled="makingLink" @click="makeLink">
            <LoadingSpinner v-if="makingLink" label="Making..." />
            <template v-else>Make Link</template>
          </button>
        </div>
      </template>
    </ModalOverlay>
  </div>
</template>

<style scoped>
.badge-active {
  font-weight: 600;
}

.badge-inactive {
  color: var(--text-muted);
}

.badge-admin {
  color: var(--highlight);
  font-weight: 600;
}

/* Each section of the Edit-user modal, divided by a subtle rule. */
.edit-section {
  padding: 14px 0;
  border-top: 1px solid color-mix(in srgb, var(--text-muted) 22%, transparent);
}

.edit-section:first-of-type {
  border-top: none;
  padding-top: 8px;
}

.perm-groups {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
  margin: 12px 0;
}

.perm-group-title {
  margin: 0 0 6px;
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-muted);
}

.perm-option {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
  cursor: pointer;
}

.perm-option input {
  width: auto;
  margin: 0;
}

.modal-actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
