import { describe, it, expect } from 'vitest'
import { participantLabel, splitParticipantLabel } from './participant'

/**
 * These cases deliberately mirror backend/internal/model/participant_test.go. The
 * two implementations have to agree: the server splits any composed string it is
 * sent, so a client that split differently would file the same person under a
 * different record.
 */
describe('participantLabel', () => {
  it('joins a name and world the one way every system displays them', () => {
    expect(participantLabel('Aria Ashwood', 'Gilgamesh')).toBe('Aria Ashwood @ Gilgamesh')
    expect(participantLabel('  Aria Ashwood  ', '  Gilgamesh  ')).toBe('Aria Ashwood @ Gilgamesh')
  })

  it('omits the separator when there is no world', () => {
    // A record written before the world picker, or one staff typed by hand, must
    // not render a dangling " @ ".
    expect(participantLabel('Aria Ashwood', '')).toBe('Aria Ashwood')
    expect(participantLabel('Aria Ashwood', '   ')).toBe('Aria Ashwood')
    expect(participantLabel('Aria Ashwood')).toBe('Aria Ashwood')
  })
})

describe('splitParticipantLabel', () => {
  it('splits a composed label', () => {
    expect(splitParticipantLabel('Aria Ashwood @ Gilgamesh')).toEqual({
      name: 'Aria Ashwood',
      world: 'Gilgamesh',
    })
    expect(splitParticipantLabel('  Aria Ashwood @ Gilgamesh  ')).toEqual({
      name: 'Aria Ashwood',
      world: 'Gilgamesh',
    })
  })

  it('reports what was written when there is no separator', () => {
    expect(splitParticipantLabel('Aria Ashwood')).toEqual({ name: 'Aria Ashwood', world: '' })
    expect(splitParticipantLabel('')).toEqual({ name: '', world: '' })
  })

  it('splits on the LAST separator, since a world never contains one', () => {
    expect(splitParticipantLabel('Odd @ Name @ Gilgamesh')).toEqual({
      name: 'Odd @ Name',
      world: 'Gilgamesh',
    })
  })

  it('keeps a half-empty label whole rather than making a nameless record', () => {
    expect(splitParticipantLabel('@ Gilgamesh')).toEqual({ name: '@ Gilgamesh', world: '' })
    expect(splitParticipantLabel('Aria Ashwood @ ')).toEqual({ name: 'Aria Ashwood @', world: '' })
  })

  it('round-trips whatever participantLabel joins', () => {
    for (const [name, world] of [
      ['Aria Ashwood', 'Gilgamesh'],
      ["Y'shtola Rhul", 'Balmung'],
      ['Odd @ Name', 'Gilgamesh'],
      ['Aria Ashwood', ''],
    ]) {
      expect(splitParticipantLabel(participantLabel(name, world))).toEqual({ name, world })
    }
  })
})
