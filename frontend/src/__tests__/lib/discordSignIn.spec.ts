import { describe, it, expect } from 'vitest'
import { discordSignInError, discordStartUrl } from '@/lib/discordSignIn'

describe('discordStartUrl', () => {
  it('is the start, with the page to land on when there is one', () => {
    expect(discordStartUrl()).toBe('/api/v1/auth/discord/start')
    expect(discordStartUrl('')).toBe('/api/v1/auth/discord/start')
    expect(discordStartUrl('/admin/bingo/cards?x=1')).toBe(
      '/api/v1/auth/discord/start?next=%2Fadmin%2Fbingo%2Fcards%3Fx%3D1',
    )
  })
})

describe('discordSignInError', () => {
  it("says why for each of the server's reasons", () => {
    for (const code of [
      'discord_denied',
      'discord_failed',
      'discord_busy',
      'discord_unavailable',
      'discord_not_linked',
      'discord_inactive',
      'discord_link_expired',
      'discord_taken',
    ]) {
      expect(discordSignInError(code)).not.toBe('')
    }
    expect(discordSignInError('discord_not_linked')).toMatch(/No account is linked/)
  })

  it('says nothing for anything else', () => {
    expect(discordSignInError(undefined)).toBe('')
    expect(discordSignInError(['discord_failed'])).toBe('')
    expect(discordSignInError('toString')).toBe('')
    expect(discordSignInError('<script>')).toBe('')
  })
})
