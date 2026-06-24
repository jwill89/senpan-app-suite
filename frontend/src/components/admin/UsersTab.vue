<script setup lang="ts">
/**
 * Admin Users tab (System section) — manages accounts and their per-page
 * permissions. Lists accounts in the shared DataTable with status/role badges,
 * and per-row actions to activate/deactivate, toggle admin, edit permissions,
 * set a password, and delete.
 *
 * The seeded "admin" account is protected: its modify actions are hidden here
 * (it changes its own password via the topbar "Change Password" dialog), and the
 * backend rejects any such change too.
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
import { ADMIN_PERMISSIONS } from '@/lib/constants'
import { formatServerTimestamp } from '@/lib/datetime'
import type { User } from '@/types/api'

const users = useUsersStore()
const ui = useUiStore()

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

const displayedUsers = computed(() => {
  const term = search.value.trim().toLowerCase()
  const rows = term
    ? users.users.filter((u) => u.username.toLowerCase().includes(term))
    : users.users.slice()
  // Admins first, then by username.
  return rows.sort((a, b) => {
    if (a.is_admin !== b.is_admin) return a.is_admin ? -1 : 1
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
function formatLastLogin(ts: string): string {
  return ts ? formatServerTimestamp(ts) : 'Never'
}

/** Human summary of a user's access for the table cell. */
function permissionSummary(u: User): string {
  if (u.is_admin) return 'Full access (admin)'
  if (!u.permissions.length) return 'None'
  return `${u.permissions.length} page${u.permissions.length === 1 ? '' : 's'}`
}

// ── Edit-user modal ──────────────────────────────────────────────────────────
// One modal consolidates every per-account action (activate/deactivate, grant/
// revoke admin, page permissions, set password) so the table row needs only two
// buttons (Edit + Delete) instead of five. Changes are collected as a draft and
// applied together on Save (users.updateUser sends only what changed).
const editUserId = ref<number | null>(null)
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
  editActive.value = u.is_active
  editAdmin.value = u.is_admin
  permDraft.value = new Set(u.permissions)
  editPassword.value = ''
  editConfirm.value = ''
  editError.value = ''
}
function closeEdit(): void {
  editUserId.value = null
}
function togglePerm(key: string): void {
  if (permDraft.value.has(key)) permDraft.value.delete(key)
  else permDraft.value.add(key)
  // Reassign to trigger reactivity on the Set.
  permDraft.value = new Set(permDraft.value)
}
async function saveUser(): Promise<void> {
  editError.value = ''
  if (editUserId.value === null) return
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
    { is_active: editActive.value, is_admin: editAdmin.value, permissions: [...permDraft.value] },
    editPassword.value || undefined,
  )
  savingUser.value = false
  if (ok) editUserId.value = null
}

onMounted(() => users.loadUsers())
</script>

<template>
  <div class="tab-body">
    <AdminPanel>
      <div class="flex-toolbar mb-12">
        <h3 class="m-0"><font-awesome-icon :icon="['fad', 'users-gear']" /> Users</h3>
        <button
          class="btn-view btn-sm push-right"
          title="Copy the hidden registration link to share with a new user"
          @click="copyRegistrationLink"
        >
          <font-awesome-icon :icon="['fas', 'copy']" /> Copy Registration Link
        </button>
      </div>

      <p class="text-dim text-xs mb-12">
        Accounts sign in with a username and password. New sign-ups (via the shared
        registration link) start <strong>inactive</strong> — activate one here and grant the
        pages it should access. Admins have full access to everything.
      </p>

      <SearchInput
        v-if="users.users.length"
        v-model="search"
        class="mb-12"
        placeholder="Search users by name…"
        aria-label="Search users by name"
      />

      <LoadingSpinner
        v-if="users.loading && users.users.length === 0"
        block
        label="Loading users…"
      />

      <DataTable
        v-else-if="users.users.length"
        :columns="userColumns"
        :rows="displayedUsers"
        row-key="id"
      >
        <template #cell-username="{ row }">
          <span class="code-gold">{{ row.username }}</span>
        </template>
        <template #cell-status="{ row }">
          <span :class="row.is_active ? 'badge-active' : 'badge-inactive'">
            {{ row.is_active ? 'Active' : 'Inactive' }}
          </span>
        </template>
        <template #cell-role="{ row }">
          <span :class="row.is_admin ? 'badge-admin' : 'text-dim'">
            {{ row.is_admin ? 'Admin' : 'User' }}
          </span>
        </template>
        <template #cell-permissions="{ row }">
          <span class="text-dim">{{ permissionSummary(row) }}</span>
        </template>
        <template #cell-lastLogin="{ row }">
          <span class="text-dim">{{ formatLastLogin(row.last_login_at) }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <template v-if="isProtected(row)">
              <span class="text-dim text-xs">Protected account</span>
            </template>
            <template v-else>
              <button class="btn-view btn-sm" title="Edit this account" @click="openEdit(row)">
                <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
              </button>
              <button
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
          <p class="text-dim ta-center" style="padding: 20px">No users match “{{ search }}”.</p>
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
      <h3 class="mt-0">
        <font-awesome-icon :icon="['fad', 'users-gear']" /> Edit User —
        <span class="code-gold">{{ editUser.username }}</span>
      </h3>

      <!-- Account -->
      <div class="edit-section">
        <label class="perm-option">
          <input type="checkbox" v-model="editActive" /> Account active
        </label>
        <label class="perm-option">
          <input type="checkbox" v-model="editAdmin" /> Administrator (full access to every page)
        </label>
      </div>

      <!-- Page permissions -->
      <div class="edit-section">
        <h4 class="perm-group-title">Page permissions</h4>
        <p v-if="editAdmin" class="text-dim text-xs m-0">
          Admins have full access to every page — per-page permissions don't apply.
        </p>
        <div v-else class="perm-groups">
          <div v-for="group in permissionGroups" :key="group.section" class="perm-group">
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
        <button class="btn-neutral" :disabled="savingUser" @click="closeEdit">Cancel</button>
        <button class="btn-action" :disabled="savingUser" @click="saveUser">
          <LoadingSpinner v-if="savingUser" label="Saving…" />
          <template v-else>Save Changes</template>
        </button>
      </div>
    </ModalOverlay>
  </div>
</template>

<style scoped>
.badge-active {
  color: var(--success, #2cb67d);
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
