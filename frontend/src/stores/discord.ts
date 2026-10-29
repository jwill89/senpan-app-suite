/**
 * Discord bot store.
 *
 * Two halves with different audiences:
 * - The bot's own page (System -> Discord, admin-only): status, the token (sent once, never
 *   read back), the server/application/presence/public-URL/test-mode configuration, and a
 *   test message.
 * - The server directory every posting system's pickers read: postable channels and
 *   pingable roles. Any account that can post from a system may load these. When the bot
 *   isn't connected the lists come back empty with a `problem`, and the pickers fall back
 *   to typing an id.
 * - Saving a system's destination (channel, ping, repost mode), shared by the tea rooms,
 *   affiliates and book club pages; each page keeps its own copy of what it saved.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { withLoading } from '@/lib/withLoading'
import type {
  DiscordChannelOption,
  DiscordDestination,
  DiscordImportRow,
  DiscordRoleOption,
  DiscordStatus,
  RepostMode,
} from '@/types/api'
import { useUiStore } from './ui'

/** The editable part of the bot's configuration. */
export interface DiscordConfigForm {
  guild_id: string
  application_id: string
  status_text: string
  public_base_url: string
  test_mode: boolean
  test_channel_id: string
}

export const useDiscordStore = defineStore('discord', () => {
  const ui = useUiStore()

  const status = ref<DiscordStatus | null>(null)
  const loading = ref(false)
  const savingToken = ref(false)
  const savingConfig = ref(false)
  const testing = ref(false)

  /** The one-time webhook import report (empty once an admin has cleared it). */
  const importRows = ref<DiscordImportRow[]>([])
  const importBusy = ref(false)

  const channels = ref<DiscordChannelOption[]>([])
  const roles = ref<DiscordRoleOption[]>([])
  /** False until the lists have loaded from a connected bot that is in the server. */
  const directoryAvailable = ref(false)
  /** Why the lists are empty, in plain words ("" when available). */
  const directoryProblem = ref('')
  const loadingDirectory = ref(false)

  /** Test mode is on: every new post goes to the test channel. */
  const testMode = computed(() => status.value?.test_mode ?? false)

  /** Load the bot's status (admin-only endpoint). */
  async function loadStatus(): Promise<void> {
    await withLoading(loading, async () => {
      status.value = (await endpoints.discord.status()).status
    })
  }

  /** Load the channel and role lists for the pickers. */
  async function loadDirectory(): Promise<void> {
    await withLoading(loadingDirectory, async () => {
      const [ch, rl] = await Promise.all([endpoints.discord.channels(), endpoints.discord.roles()])
      channels.value = ch.channels
      roles.value = rl.roles
      directoryAvailable.value = ch.available && rl.available
      directoryProblem.value = ch.problem || rl.problem
    })
  }

  /** Check, store and connect a bot token. Returns true on success. */
  async function saveToken(token: string): Promise<boolean> {
    savingToken.value = true
    try {
      status.value = (await endpoints.discord.setToken(token)).status
      ui.notify('Bot token saved - the bot is connecting', 'success')
      void loadDirectory()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingToken.value = false
    }
  }

  /** Forget the token (after confirming) and take the bot offline. */
  async function clearToken(): Promise<void> {
    const ok = await ui.confirm(
      'Remove the bot token? The bot goes offline and nothing will post to Discord until a token is saved again.',
      { title: 'Remove bot token', confirmText: 'Remove' },
    )
    if (!ok) return
    savingToken.value = true
    try {
      status.value = (await endpoints.discord.clearToken()).status
      channels.value = []
      roles.value = []
      directoryAvailable.value = false
      ui.notify('Bot token removed', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      savingToken.value = false
    }
  }

  /** Save the configuration. Returns true on success. */
  async function saveConfig(form: DiscordConfigForm): Promise<boolean> {
    savingConfig.value = true
    try {
      status.value = (
        await endpoints.discord.saveConfig({
          ...form,
          guild_id: form.guild_id.trim(),
          application_id: form.application_id.trim(),
          status_text: form.status_text.trim(),
          public_base_url: form.public_base_url.trim(),
          test_channel_id: form.test_channel_id.trim(),
        })
      ).status
      ui.notify('Discord settings saved', 'success')
      void loadDirectory()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingConfig.value = false
    }
  }

  /** Post a test message to a channel. */
  async function sendTest(channelId: string): Promise<void> {
    testing.value = true
    try {
      const res = await endpoints.discord.test(channelId)
      if (res.suppressed) ui.notify(res.note || 'Dry run: nothing was sent', 'info')
      else ui.notify(res.note ? `Test message sent. ${res.note}` : 'Test message sent', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      testing.value = false
    }
  }

  /** Load the webhook import report (admin-only). */
  async function loadImport(): Promise<void> {
    await withLoading(importBusy, async () => {
      importRows.value = (await endpoints.discord.importReport()).rows
    })
  }

  /** Look up webhooks that couldn't be reached before. */
  async function retryImport(): Promise<void> {
    await withLoading(importBusy, async () => {
      importRows.value = (await endpoints.discord.retryImport()).rows
      ui.notify('Import tried again', 'success')
    })
  }

  /** Delete the old webhooks from Discord (after confirming) and clear the report. */
  async function removeOldWebhooks(): Promise<void> {
    const ok = await ui.confirm(
      'Delete the old webhooks from Discord? Only the ones in this server are deleted; messages they already posted stay. This cannot be undone, and the import report is cleared.',
      { title: 'Remove old webhooks', confirmText: 'Delete webhooks' },
    )
    if (!ok) return
    await withLoading(importBusy, async () => {
      const res = await endpoints.discord.removeOldWebhooks()
      importRows.value = []
      const parts = [`${res.deleted} deleted`]
      if (res.skipped) parts.push(`${res.skipped} left alone`)
      if (res.failed) parts.push(`${res.failed} refused by Discord`)
      ui.notify(`Old webhooks: ${parts.join(', ')}`, res.failed ? 'error' : 'success')
    })
  }

  /** The editable part of a destination. */
  type DestinationForm = { channel_id: string; mention: string; repost_mode: RepostMode }

  /** Save where a system posts. Returns the saved destination, or null on failure. */
  async function saveDestination(
    key: string,
    dest: DestinationForm,
  ): Promise<DiscordDestination | null> {
    try {
      const res = await endpoints.discord.saveDestination(key, {
        ...dest,
        channel_id: dest.channel_id.trim(),
      })
      ui.notify('Discord settings saved', 'success')
      return res.destination
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return null
    }
  }

  /** A channel's display label ("#name"), or "" when the id isn't in the list. */
  function channelLabel(id: string): string {
    const c = channels.value.find((ch) => ch.id === id)
    return c ? `#${c.name}` : ''
  }

  return {
    status,
    loading,
    savingToken,
    savingConfig,
    testing,
    channels,
    roles,
    directoryAvailable,
    directoryProblem,
    loadingDirectory,
    testMode,
    loadStatus,
    loadDirectory,
    saveToken,
    clearToken,
    saveConfig,
    sendTest,
    importRows,
    importBusy,
    loadImport,
    retryImport,
    removeOldWebhooks,
    saveDestination,
    channelLabel,
  }
})
