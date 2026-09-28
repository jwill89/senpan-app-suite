package model_test

import (
	"testing"

	"app-suite/internal/model"
)

func TestParticipantLabel(t *testing.T) {
	for _, tc := range []struct {
		name, world, want string
	}{
		{"Aria Ashwood", "Gilgamesh", "Aria Ashwood @ Gilgamesh"},
		{"  Aria Ashwood  ", "  Gilgamesh  ", "Aria Ashwood @ Gilgamesh"},
		// A record written before worlds had their own column, or one staff typed
		// by hand, must not render a dangling separator.
		{"Aria Ashwood", "", "Aria Ashwood"},
		{"Aria Ashwood", "   ", "Aria Ashwood"},
	} {
		if got := model.ParticipantLabel(tc.name, tc.world); got != tc.want {
			t.Errorf("ParticipantLabel(%q, %q) = %q; want %q", tc.name, tc.world, got, tc.want)
		}
	}
}

func TestSplitParticipantLabel(t *testing.T) {
	for _, tc := range []struct {
		in, wantName, wantWorld string
	}{
		{"Aria Ashwood @ Gilgamesh", "Aria Ashwood", "Gilgamesh"},
		{"  Aria Ashwood @ Gilgamesh  ", "Aria Ashwood", "Gilgamesh"},
		// No separator: report what was typed rather than guessing a world.
		{"Aria Ashwood", "Aria Ashwood", ""},
		// Split on the LAST separator - a world never contains one, so an earlier
		// occurrence is part of the name.
		{"Odd @ Name @ Gilgamesh", "Odd @ Name", "Gilgamesh"},
		// Half a label identifies nobody, so it stays whole rather than becoming a
		// nameless record.
		{"@ Gilgamesh", "@ Gilgamesh", ""},
		{" @ Gilgamesh", "@ Gilgamesh", ""},
		{"Aria Ashwood @ ", "Aria Ashwood @", ""},
		{"", "", ""},
	} {
		name, world := model.SplitParticipantLabel(tc.in)
		if name != tc.wantName || world != tc.wantWorld {
			t.Errorf("SplitParticipantLabel(%q) = (%q, %q); want (%q, %q)",
				tc.in, name, world, tc.wantName, tc.wantWorld)
		}
	}
}

// Whatever ParticipantLabel joins, SplitParticipantLabel must take apart again -
// otherwise a name written by one system stops matching in another, which is the
// entire reason these two exist.
func TestParticipantLabelRoundTrips(t *testing.T) {
	for _, tc := range [][2]string{
		{"Aria Ashwood", "Gilgamesh"},
		{"Y'shtola Rhul", "Balmung"},
		{"Odd @ Name", "Gilgamesh"},
		{"Aria Ashwood", ""},
	} {
		name, world := model.SplitParticipantLabel(model.ParticipantLabel(tc[0], tc[1]))
		if name != tc[0] || world != tc[1] {
			t.Errorf("round trip of (%q, %q) = (%q, %q)", tc[0], tc[1], name, world)
		}
	}
}

func TestSameParticipant(t *testing.T) {
	if !model.SameParticipant("aria ashwood", "GILGAMESH", "Aria Ashwood", "Gilgamesh") {
		t.Error("case differences must not split one person in two")
	}
	if !model.SameParticipant(" Aria Ashwood ", "Gilgamesh", "Aria Ashwood", " Gilgamesh ") {
		t.Error("surrounding space must not split one person in two")
	}
	// The world is what keeps two people who share a character name apart, so a
	// blank one cannot act as a wildcard.
	if model.SameParticipant("Aria Ashwood", "", "Aria Ashwood", "Gilgamesh") {
		t.Error("a blank world matched a real one; that merges two different people")
	}
	if model.SameParticipant("Aria Ashwood", "Balmung", "Aria Ashwood", "Gilgamesh") {
		t.Error("same name on different worlds must stay two people")
	}
}
