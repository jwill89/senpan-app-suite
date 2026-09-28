/**
 * The one way this app writes a participant's identity, mirroring Go's
 * model.ParticipantLabel / SplitParticipantLabel.
 *
 * Four systems record people - custom cards, raffle entries, stamp rally cards and
 * garapon drawing links - and each stores the character name and the home world as
 * two separate fields. These turn that pair into one display string and back.
 *
 * It has to agree with the Go side exactly: the server splits any composed string
 * it receives, so a client that joined or split differently would produce records
 * the server files under a different person. Both sides split on the LAST " @ " and
 * refuse to produce a half-empty pair - see backend/internal/model/participant.go.
 */

/** Separates a character name from its home world. */
const SEPARATOR = ' @ '

/**
 * Renders a name and world the single way every system displays them:
 * "Firstname Lastname @ World". A blank world yields just the name, so a record
 * predating the world picker reads as what it is rather than trailing a separator.
 */
export function participantLabel(name: string, world = ''): string {
  const trimmedName = name.trim()
  const trimmedWorld = world.trim()
  return trimmedWorld ? `${trimmedName}${SEPARATOR}${trimmedWorld}` : trimmedName
}

/**
 * participantLabel's inverse, for a string that arrives already joined - a record
 * this browser saved before worlds had their own field, or a name someone pasted.
 *
 * Splits on the LAST separator, since a world never contains one. With no
 * separator the whole string is the name and the world is empty: the caller is told
 * what was actually written rather than being handed a guess. A half-empty result
 * ("@ Gilgamesh") identifies nobody, so the original string is kept as the name.
 */
export function splitParticipantLabel(value: string): { name: string; world: string } {
  const trimmed = value.trim()
  const at = trimmed.lastIndexOf(SEPARATOR)
  if (at < 0) return { name: trimmed, world: '' }

  const name = trimmed.slice(0, at).trim()
  const world = trimmed.slice(at + SEPARATOR.length).trim()
  if (!name || !world) return { name: trimmed, world: '' }
  return { name, world }
}
