import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { DiscordStatus } from '@/types/api'

function status(over: Partial<DiscordStatus> = {}): DiscordStatus {
  return {
    key_available: true,
    token_set: true,
    token_readable: true,
    connected: true,
    online: true,
    connected_at: '',
    last_error: '',
    bot_id: '1',
    bot_name: 'Senpan Tea House',
    bot_avatar_url: '',
    application_id: '1563399760994635906',
    token_application_id: '1563399760994635906',
    application_mismatch: false,
    guild_id: '999011226568888410',
    guild_name: 'Senpan Tea House',
    guild_icon_url: '',
    in_guild: true,
    bot_is_admin: true,
    invite_url: '',
    status_text: '',
    public_base_url: 'https://apps.senpan.cafe',
    test_mode: false,
    test_channel_id: '',
    dry_run: false,
    redirect_channel_id: '',
    problem: '',
    ...over,
  }
}

const ep = vi.hoisted(() => ({
  status: vi.fn(),
  setToken: vi.fn(),
  clearToken: vi.fn(),
  saveConfig: vi.fn(),
  test: vi.fn(),
  channels: vi.fn(),
  roles: vi.fn(),
  importReport: vi.fn(),
  retryImport: vi.fn(),
  removeOldWebhooks: vi.fn(),
}))
vi.mock('@/lib/endpoints', () => ({ endpoints: { discord: ep } }))

import { useDiscordStore } from './discord'
import { useUiStore } from './ui'

const channelsOK = {
  channels: [{ id: '5', name: 'test-channel', type: 'text', category: '' }],
  available: true,
  problem: '',
}
const rolesOK = {
  roles: [{ id: '7', name: 'Staff', color: '#e53170', managed: false }],
  available: true,
  problem: '',
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockReset())
  ep.channels.mockResolvedValue(channelsOK)
  ep.roles.mockResolvedValue(rolesOK)
})

describe('status and directory', () => {
  it('loads the status, and exposes test mode', async () => {
    ep.status.mockResolvedValue({ status: status({ test_mode: true }) })
    const s = useDiscordStore()
    await s.loadStatus()
    expect(s.status?.bot_name).toBe('Senpan Tea House')
    expect(s.testMode).toBe(true)
  })

  it('loads channels and roles together', async () => {
    const s = useDiscordStore()
    await s.loadDirectory()
    expect(s.channels).toHaveLength(1)
    expect(s.roles[0]?.name).toBe('Staff')
    expect(s.directoryAvailable).toBe(true)
    expect(s.channelLabel('5')).toBe('#test-channel')
    expect(s.channelLabel('404')).toBe('')
  })

  it('reports why the lists are empty when the bot is not connected', async () => {
    ep.channels.mockResolvedValue({
      channels: [],
      available: false,
      problem: 'Paste the bot token',
    })
    ep.roles.mockResolvedValue({ roles: [], available: false, problem: 'Paste the bot token' })
    const s = useDiscordStore()
    await s.loadDirectory()
    expect(s.directoryAvailable).toBe(false)
    expect(s.directoryProblem).toBe('Paste the bot token')
  })
})

describe('token', () => {
  it('saves a token, then refreshes the lists', async () => {
    ep.setToken.mockResolvedValue({ status: status() })
    const s = useDiscordStore()
    expect(await s.saveToken('abc.def')).toBe(true)
    expect(ep.setToken).toHaveBeenCalledWith('abc.def')
    expect(s.status?.connected).toBe(true)
    await vi.waitFor(() => expect(ep.channels).toHaveBeenCalled())
  })

  it('reports a refused token', async () => {
    ep.setToken.mockRejectedValue(new Error('Discord refused this token.'))
    const s = useDiscordStore()
    const ui = useUiStore()
    const notify = vi.spyOn(ui, 'notify')
    expect(await s.saveToken('nope')).toBe(false)
    expect(notify).toHaveBeenCalledWith('Discord refused this token.', 'error')
  })

  it('removes the token only after confirming', async () => {
    const s = useDiscordStore()
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => false)
    await s.clearToken()
    expect(ep.clearToken).not.toHaveBeenCalled()

    ui.confirm = vi.fn(async () => true)
    ep.clearToken.mockResolvedValue({ status: status({ token_set: false, connected: false }) })
    s.channels = channelsOK.channels
    await s.clearToken()
    expect(ep.clearToken).toHaveBeenCalled()
    expect(s.status?.token_set).toBe(false)
    expect(s.channels).toEqual([])
  })
})

describe('webhook import', () => {
  const row = (status: string) => ({
    source: 'tea-rooms',
    label: 'Tea Rooms',
    where: 'Senpan Tea House -> Tea Rooms -> Discord',
    status,
    detail: '',
    channel_id: '',
    settled_at: '',
  })

  it('loads the report and retries', async () => {
    ep.importReport.mockResolvedValue({ rows: [row('error')] })
    ep.retryImport.mockResolvedValue({ rows: [row('imported')] })
    const s = useDiscordStore()
    await s.loadImport()
    expect(s.importRows[0]?.status).toBe('error')
    await s.retryImport()
    expect(s.importRows[0]?.status).toBe('imported')
  })

  it('removes old webhooks only after confirming, then clears the report', async () => {
    const s = useDiscordStore()
    const ui = useUiStore()
    s.importRows = [row('imported')]
    ui.confirm = vi.fn(async () => false)
    await s.removeOldWebhooks()
    expect(ep.removeOldWebhooks).not.toHaveBeenCalled()

    ui.confirm = vi.fn(async () => true)
    ep.removeOldWebhooks.mockResolvedValue({ deleted: 1, skipped: 1, failed: 0 })
    const notify = vi.spyOn(ui, 'notify')
    await s.removeOldWebhooks()
    expect(s.importRows).toEqual([])
    expect(notify).toHaveBeenCalledWith('Old webhooks: 1 deleted, 1 left alone', 'success')
  })
})

describe('configuration and test message', () => {
  it('trims the configuration before saving', async () => {
    ep.saveConfig.mockResolvedValue({ status: status() })
    const s = useDiscordStore()
    await s.saveConfig({
      guild_id: ' 999011226568888410 ',
      application_id: '1563399760994635906',
      status_text: '  Pouring tea ',
      public_base_url: ' https://apps.senpan.cafe ',
      test_mode: true,
      test_channel_id: ' 5 ',
    })
    expect(ep.saveConfig).toHaveBeenCalledWith({
      guild_id: '999011226568888410',
      application_id: '1563399760994635906',
      status_text: 'Pouring tea',
      public_base_url: 'https://apps.senpan.cafe',
      test_mode: true,
      test_channel_id: '5',
    })
  })

  it('says when a test message was suppressed by a dry run', async () => {
    const s = useDiscordStore()
    const notify = vi.spyOn(useUiStore(), 'notify')
    ep.test.mockResolvedValue({
      sent: false,
      suppressed: true,
      channel_id: '5',
      message_id: '',
      note: 'Dry run.',
    })
    await s.sendTest('5')
    expect(notify).toHaveBeenLastCalledWith('Dry run.', 'info')
    ep.test.mockResolvedValue({
      sent: true,
      suppressed: false,
      channel_id: '5',
      message_id: '9',
      note: '',
    })
    await s.sendTest('5')
    expect(notify).toHaveBeenLastCalledWith('Test message sent', 'success')
  })
})
