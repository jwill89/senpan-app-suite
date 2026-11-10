import { describe, it, expect } from 'vitest'
import { APP_VERSION, serverDiffers } from '@/lib/version'

describe('version', () => {
  it('exposes the app version from package.json', () => {
    expect(APP_VERSION).toMatch(/^\d+\.\d+\.\d+$/)
  })

  it('sees no difference while the server runs this build', () => {
    expect(serverDiffers(APP_VERSION)).toBe(false)
  })

  it('tells a tab the server runs another version', () => {
    expect(serverDiffers('999.0.0')).toBe(true)
  })

  it('raises no false alarm while the server version is unknown', () => {
    expect(serverDiffers(null)).toBe(false)
  })
})
