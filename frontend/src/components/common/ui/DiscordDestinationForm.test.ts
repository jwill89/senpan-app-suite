import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import type { DiscordDestination } from '@/types/api'

const ep = vi.hoisted(() => ({ channels: vi.fn(), roles: vi.fn() }))
vi.mock('@/lib/endpoints', () => ({ endpoints: { discord: ep } }))

import DiscordDestinationForm from './DiscordDestinationForm.vue'
import DiscordPostStatus from './DiscordPostStatus.vue'
import { useDiscordStore } from '@/stores/discord'
import { ITEM_REPOST_OPTIONS, LIST_REPOST_OPTIONS, postVerb } from '@/lib/discordPosts'

beforeEach(() => {
  setActivePinia(createPinia())
  ep.channels.mockResolvedValue({ channels: [], available: false, problem: '' })
  ep.roles.mockResolvedValue({ roles: [], available: false, problem: '' })
})

function dest(over: Partial<DiscordDestination> = {}): DiscordDestination {
  return {
    key: 'tea-rooms',
    channel_id: '3',
    mention: 'everyone',
    repost_mode: 'edit',
    updated_at: '',
    ...over,
  }
}

const live = {
  state: 'live',
  channel_id: '3',
  url: 'https://discord.com/channels/1/3/9',
  posted_at: '',
  edited_at: '',
}

describe('DiscordDestinationForm', () => {
  it('starts from the saved destination and saves the edited draft', async () => {
    const w = mount(DiscordDestinationForm, {
      props: { destination: dest(), options: ITEM_REPOST_OPTIONS },
    })
    const modes = w.findAll('.toggle-btn')
    expect(modes.map((b) => b.text())).toEqual(['Keep one post', 'Replace it', 'Add another'])
    expect(modes[0].classes()).toContain('is-active')

    await modes[1].trigger('click')
    await w.find('.btn-confirm').trigger('click')
    expect(w.emitted('save')?.[0]).toEqual([
      { channel_id: '3', mention: 'everyone', repost_mode: 'replace' },
    ])
  })

  it('falls back to the first offered mode when the saved one is not offered', () => {
    // A book club can't edit in place, so a stray "edit" reads as the first choice.
    const w = mount(DiscordDestinationForm, {
      props: { destination: dest({ repost_mode: 'edit' }), options: LIST_REPOST_OPTIONS },
    })
    expect(w.find('.toggle-btn.is-active').text()).toBe('Replace the previous list')
  })

  it('cancels without saving', async () => {
    const w = mount(DiscordDestinationForm, {
      props: { destination: null, options: ITEM_REPOST_OPTIONS },
    })
    await w.find('.btn-neutral').trigger('click')
    expect(w.emitted('cancel')).toHaveLength(1)
    expect(w.emitted('save')).toBeUndefined()
  })
})

describe('DiscordPostStatus', () => {
  it('names the channel and links to a live post', () => {
    const d = useDiscordStore()
    d.directoryAvailable = true
    d.channels = [{ id: '3', name: 'tea-rooms', type: 'text', category: '' }]
    const w = mount(DiscordPostStatus, { props: { post: live } })
    expect(w.text()).toContain('#tea-rooms')
    expect(w.find('a').attributes('href')).toBe(live.url)
  })

  it('says when the post was deleted in Discord, and shows nothing when there is none', () => {
    expect(
      mount(DiscordPostStatus, { props: { post: { ...live, state: 'gone', url: '' } } }).text(),
    ).toBe('Deleted in Discord')
    expect(mount(DiscordPostStatus, { props: { post: undefined } }).text()).toBe('')
  })
})

describe('postVerb', () => {
  it('updates only a live post in a destination that edits in place', () => {
    expect(postVerb(dest(), live)).toBe('Update')
    expect(postVerb(dest({ repost_mode: 'replace' }), live)).toBe('Post')
    expect(postVerb(dest(), { ...live, state: 'gone' })).toBe('Post')
    expect(postVerb(dest(), undefined)).toBe('Post')
  })
})
