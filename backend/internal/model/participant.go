package model

import "strings"

// The one way this app writes a participant's identity.
//
// Four systems record people - custom cards, raffle entries, stamp rally cards and
// garapon drawing links - and each stores the character name and the home world as
// two separate fields. This file is where those two become one string and back, so
// a person written by one system is recognisable to the others.
//
// The separator is " @ ", the format FFXIV players already use and the one the
// sign-up forms have always suggested. A world never contains it, which is what
// makes the split unambiguous.

// participantSep separates a character name from its home world.
const participantSep = " @ "

// ParticipantLabel renders a name and world the single way every system displays
// them: "Firstname Lastname @ World". A blank world yields just the name, so a
// pre-picker record (or one staff entered by hand) reads as what it is rather than
// trailing an empty separator.
func ParticipantLabel(name, world string) string {
	name, world = strings.TrimSpace(name), strings.TrimSpace(world)
	if world == "" {
		return name
	}
	return name + participantSep + world
}

// SplitParticipantLabel is ParticipantLabel's inverse, for input that arrives as
// one string - a lookup box, or a record written before the world had its own
// column.
//
// It splits on the LAST separator: a world never contains one, so anything earlier
// belongs to the name. With no separator the whole string is the name and the world
// is empty - the caller is told what was actually typed rather than being handed a
// guess. Both halves come back trimmed, and a blank half yields the original
// string as the name, since "@ Gilgamesh" identifies nobody.
func SplitParticipantLabel(s string) (name, world string) {
	s = strings.TrimSpace(s)
	at := strings.LastIndex(s, participantSep)
	if at < 0 {
		return s, ""
	}
	name = strings.TrimSpace(s[:at])
	world = strings.TrimSpace(s[at+len(participantSep):])
	if name == "" || world == "" {
		return s, ""
	}
	return name, world
}

// SameParticipant reports whether two name/world pairs identify one person.
//
// Case-insensitive, because that is how every one of these is already matched in
// SQL (COLLATE NOCASE, or LOWER() on both sides) - a lookup that disagreed with the
// storage would find nothing. A pair with no world matches only another with no
// world: treating a blank world as a wildcard would merge two people who share a
// character name across worlds, which is precisely what the world is here to
// prevent.
func SameParticipant(nameA, worldA, nameB, worldB string) bool {
	return strings.EqualFold(strings.TrimSpace(nameA), strings.TrimSpace(nameB)) &&
		strings.EqualFold(strings.TrimSpace(worldA), strings.TrimSpace(worldB))
}
