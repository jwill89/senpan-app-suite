import { describe, it, expect } from 'vitest'
import { FRONTEND_VERSION, majorOf, versionsCompatible } from './version'

describe('version', () => {
  it('exposes a parseable frontend version (from package.json)', () => {
    expect(FRONTEND_VERSION).toMatch(/^\d+\.\d+\.\d+/)
    expect(majorOf(FRONTEND_VERSION)).not.toBeNull()
  })

  it('parses the major component, tolerating a v-prefix', () => {
    expect(majorOf('1.2.3')).toBe(1)
    expect(majorOf('v2.0.0')).toBe(2)
    expect(majorOf('not-a-version')).toBeNull()
  })

  it('treats equal major versions as compatible', () => {
    expect(versionsCompatible('1.0.0', '1.9.4')).toBe(true)
  })

  it('flags a major-version mismatch as incompatible', () => {
    expect(versionsCompatible('2.0.0', '1.5.0')).toBe(false)
  })

  it('does not raise a false alarm for unparseable versions', () => {
    expect(versionsCompatible('1.0.0', 'unknown')).toBe(true)
  })
})
