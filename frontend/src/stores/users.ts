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
      users.value = data.users || []
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
    deleteUser,
  }
})
