/**
 * Users store: manages accounts for the System -> Users admin tab (admin only).
 * Lists accounts, changes one (active, admin, permissions, password) in a single
 * PATCH, and deletes accounts. Every change reloads the list so the table reflects
 * the server.
 *
 * The seeded "admin" account is protected server-side (it can't be deactivated,
 * demoted, deleted or have its password set here); the UI also hides those actions
 * for it (see UsersTab).
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, unwrap, type User } from '@/api/client'
import type { components } from '@/api/schema'
import { useUiStore } from './ui'

/** Username of the protected bootstrap admin account. */
export const PROTECTED_ADMIN = 'admin'

type UserChanges = components['schemas']['UserChanges']

/** What the Edit User modal sets an account to. */
export interface UserEdit {
  isActive: boolean
  isAdmin: boolean
  permissions: string[]
}

export const useUsersStore = defineStore('users', () => {
  const ui = useUiStore()

  const users = ref<User[]>([])
  /** True while the user list is loading (drives the table spinner). */
  const loading = ref(false)

  async function loadUsers(): Promise<void> {
    loading.value = true
    try {
      users.value = (await unwrap(api.GET('/users'))).users
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
  }

  /**
   * Applies the Edit User modal in one PATCH: only what changed (active, admin,
   * permissions), plus a new password when one is given. Permissions are left alone
   * when the account is (or becomes) an admin, who holds every one. Reloads and
   * notifies once; returns true on success.
   */
  async function updateUser(id: number, desired: UserEdit, password?: string): Promise<boolean> {
    const orig = users.value.find((u) => u.id === id)
    if (!orig) {
      ui.notify('User not found', 'error')
      return false
    }
    const changes: UserChanges = {}
    if (desired.isActive !== orig.isActive) {
      changes.isActive = desired.isActive
    }
    if (desired.isAdmin !== orig.isAdmin) {
      changes.isAdmin = desired.isAdmin
    }
    if (!desired.isAdmin) {
      const a = [...desired.permissions].sort().join('|')
      const b = [...orig.permissions].sort().join('|')
      if (a !== b) {
        changes.permissions = desired.permissions
      }
    }
    if (password) {
      changes.password = password
    }
    try {
      await unwrap(api.PATCH('/users/{id}', { params: { path: { id } }, body: changes }))
      ui.notify('User updated', 'success')
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      await loadUsers()
    }
  }

  async function deleteUser(id: number, username: string): Promise<void> {
    if (
      !(await ui.confirm(`Delete account "${username}"? This cannot be undone.`, {
        title: 'Delete account',
        confirmText: 'Delete',
      }))
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/users/{id}', { params: { path: { id } } }))
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
    updateUser,
    deleteUser,
  }
})
