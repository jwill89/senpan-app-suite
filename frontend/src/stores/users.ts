/**
 * Users store: manages accounts for the System → Users admin tab (admin only).
 * Lists accounts, activates/deactivates them, toggles admin, edits per-page
 * permissions, sets a user's password, and deletes accounts. All mutations
 * refresh the list so the table reflects server state.
 *
 * The seeded "admin" account is protected server-side (it can't be modified
 * here); the UI also hides those actions for it (see UsersTab).
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { User } from '@/types/api'
import { useUiStore } from './ui'

/** Username of the protected bootstrap admin account. */
export const PROTECTED_ADMIN = 'admin'

export const useUsersStore = defineStore('users', () => {
  const ui = useUiStore()

  const users = ref<User[]>([])
  /** True while the user list is loading (drives the table spinner). */
  const loading = ref(false)

  async function loadUsers(): Promise<void> {
    loading.value = true
    try {
      const data = await endpoints.users.list()
      users.value = data.users
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
  }

  async function setActive(id: number, active: boolean): Promise<void> {
    try {
      await endpoints.users.setActive(id, active)
      ui.notify(active ? 'Account activated' : 'Account deactivated', 'success')
      await loadUsers()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function setAdmin(id: number, admin: boolean): Promise<void> {
    try {
      await endpoints.users.setAdmin(id, admin)
      ui.notify(admin ? 'Granted admin' : 'Revoked admin', 'success')
      await loadUsers()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Saves a user's page-permission set; returns true on success. */
  async function setPermissions(id: number, permissions: string[]): Promise<boolean> {
    try {
      await endpoints.users.setPermissions(id, permissions)
      ui.notify('Permissions saved', 'success')
      await loadUsers()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  /** Sets a user's password (admin action); returns true on success. */
  async function setPassword(id: number, password: string): Promise<boolean> {
    try {
      await endpoints.users.setPassword(id, password)
      ui.notify('Password updated', 'success')
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  /**
   * Applies all edits from the "Edit User" modal in one go: only the fields that
   * actually changed are sent (active, admin, permissions), plus an optional new
   * password. Reloads + notifies once at the end (instead of per-field), and
   * reloads on failure too so the table reflects any partial change. Returns true
   * on full success. Permissions are skipped when the account is (or becomes) an
   * admin, since admins implicitly hold every permission.
   */
  async function updateUser(
    id: number,
    desired: { is_active: boolean; is_admin: boolean; permissions: string[] },
    password?: string,
  ): Promise<boolean> {
    const orig = users.value.find((u) => u.id === id)
    if (!orig) {
      ui.notify('User not found', 'error')
      return false
    }
    try {
      if (desired.is_active !== orig.is_active)
        await endpoints.users.setActive(id, desired.is_active)
      if (desired.is_admin !== orig.is_admin) await endpoints.users.setAdmin(id, desired.is_admin)
      if (!desired.is_admin) {
        const a = [...desired.permissions].sort().join('|')
        const b = [...orig.permissions].sort().join('|')
        if (a !== b) await endpoints.users.setPermissions(id, desired.permissions)
      }
      if (password) await endpoints.users.setPassword(id, password)
      ui.notify('User updated', 'success')
      await loadUsers()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      await loadUsers()
      return false
    }
  }

  async function deleteUser(id: number, username: string): Promise<void> {
    if (
      !(await ui.confirm(`Delete account "${username}"? This cannot be undone.`, {
        title: 'Delete account',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.users.delete(id)
      ui.notify('Account deleted', 'info')
      await loadUsers()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  return {
    users,
    loading,
    loadUsers,
    setActive,
    setAdmin,
    setPermissions,
    setPassword,
    updateUser,
    deleteUser,
  }
})
