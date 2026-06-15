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

/** Human summary of a user's access for the table cell. */
function permissionSummary(u: User): string {
  if (u.is_admin) return 'Full access (admin)'
  if (!u.permissions.length) return 'None'
  return `${u.permissions.length} page${u.permissions.length === 1 ? '' : 's'}`
}

// ── Permission editor modal ──────────────────────────────────────────────────
const permEditUser = ref<User | null>(null)
const permDraft = ref<Set<string>>(new Set())
const savingPerms = ref(false)

function openPermissions(u: User): void {
  permEditUser.value = u
  permDraft.value = new Set(u.permissions)
}
function togglePerm(key: string): void {
  if (permDraft.value.has(key)) permDraft.value.delete(key)
  else permDraft.value.add(key)
  // Reassign to trigger reactivity on the Set.
  permDraft.value = new Set(permDraft.value)
}
async function savePermissions(): Promise<void> {
  if (!permEditUser.value) return
  savingPerms.value = true
  const ok = await users.setPermissions(permEditUser.value.id, [...permDraft.value])
  savingPerms.value = false
  if (ok) permEditUser.value = null
}

// ── Set-password modal ───────────────────────────────────────────────────────
const pwUser = ref<User | null>(null)
const newPassword = ref('')
const confirmPassword = ref('')
const pwError = ref('')
const savingPw = ref(false)

function openSetPassword(u: User): void {
  pwUser.value = u
  newPassword.value = ''
  confirmPassword.value = ''
  pwError.value = ''
}
async function savePassword(): Promise<void> {
  pwError.value = ''
  if (newPassword.value.length < 8) {
    pwError.value = 'Password must be at least 8 characters.'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    pwError.value = 'Passwords do not match.'
    return
  }
  if (!pwUser.value) return
  savingPw.value = true
  const ok = await users.setPassword(pwUser.value.id, newPassword.value)
  savingPw.value = false
  if (ok) pwUser.value = null
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
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <template v-if="isProtected(row)">
              <span class="text-dim text-xs">Protected account</span>
            </template>
            <template v-else>
              <button
                v-if="row.is_active"
                class="btn-neutral btn-sm"
                title="Deactivate this account"
                @click="users.setActive(row.id, false)"
              >
                Deactivate
              </button>
              <button
                v-else
                class="btn-confirm btn-sm"
                title="Activate this account"
                @click="users.setActive(row.id, true)"
              >
                <font-awesome-icon :icon="['fas', 'circle-check']" /> Activate
              </button>
              <button
                class="btn-neutral btn-sm"
                :title="row.is_admin ? 'Revoke admin' : 'Make admin'"
                @click="users.setAdmin(row.id, !row.is_admin)"
              >
                {{ row.is_admin ? 'Revoke Admin' : 'Make Admin' }}
              </button>
              <button
                class="btn-view btn-sm"
                :disabled="row.is_admin"
                :title="
                  row.is_admin
                    ? 'Admins already have full access'
                    : 'Edit page permissions'
                "
                @click="openPermissions(row)"
              >
                <font-awesome-icon :icon="['fas', 'sliders']" /> Permissions
              </button>
              <button
                class="btn-confirm btn-sm"
                title="Set a new password for this user"
                @click="openSetPassword(row)"
              >
                <font-awesome-icon :icon="['fas', 'lock']" /> Set Password
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

    <!-- Permission editor -->
    <ModalOverlay
      v-if="permEditUser"
      aria-label="Edit permissions"
      box-style="max-width: 560px"
      @close="permEditUser = null"
    >
      <h3 class="mt-0">
        <font-awesome-icon :icon="['fad', 'sliders']" /> Permissions —
        <span class="code-gold">{{ permEditUser.username }}</span>
      </h3>
      <p class="text-dim text-xs">Choose which pages this user can access.</p>

      <div class="perm-groups">
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

      <div class="modal-actions">
        <button class="btn-neutral" :disabled="savingPerms" @click="permEditUser = null">
          Cancel
        </button>
        <button class="btn-action" :disabled="savingPerms" @click="savePermissions">
          <LoadingSpinner v-if="savingPerms" label="Saving…" />
          <template v-else>Save Permissions</template>
        </button>
      </div>
    </ModalOverlay>

    <!-- Set password -->
    <ModalOverlay
      v-if="pwUser"
      aria-label="Set password"
      box-style="max-width: 420px"
      @close="pwUser = null"
    >
      <h3 class="mt-0">
        <font-awesome-icon :icon="['fad', 'lock']" /> Set Password —
        <span class="code-gold">{{ pwUser.username }}</span>
      </h3>
      <form autocomplete="off" @submit.prevent="savePassword">
        <FormField label="New password" html-for="set-pw">
          <input
            id="set-pw"
            v-model="newPassword"
            type="password"
            autocomplete="new-password"
            placeholder="At least 8 characters"
          />
        </FormField>
        <FormField label="Confirm password" html-for="set-pw-confirm">
          <input
            id="set-pw-confirm"
            v-model="confirmPassword"
            type="password"
            autocomplete="new-password"
          />
        </FormField>
        <p v-if="pwError" class="error-msg">{{ pwError }}</p>
        <div class="modal-actions">
          <button type="button" class="btn-neutral" :disabled="savingPw" @click="pwUser = null">
            Cancel
          </button>
          <button type="submit" class="btn-action" :disabled="savingPw">
            <LoadingSpinner v-if="savingPw" label="Saving…" />
            <template v-else>Set Password</template>
          </button>
        </div>
      </form>
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
