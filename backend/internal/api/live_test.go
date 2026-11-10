package api

import (
	"reflect"
	"slices"
	"testing"

	"app-suite/internal/live"
	"app-suite/internal/permissions"
)

// Each resource is heard by the pages that show it; the accounts and the
// integrations by the admins alone; a book club's by that club.
func TestResourceAudience(t *testing.T) {
	for resource, want := range map[string]live.Audience{
		"raffles":       {Staff: true, Perms: []string{permissions.TeahouseRaffles}},
		"festival-maps": {Staff: true, Perms: []string{permissions.FestivalMap, permissions.FestivalEma}},
		"images":        {Staff: true, Perms: permissions.ImageBrowsers()},
		"winners-log":   {Staff: true, Perms: []string{permissions.BingoWinnersLog}},
		"users":         {Staff: true, Admins: true},
		"integrations":  {Staff: true, Admins: true},
		"bookclub-yaoi": {Staff: true, Perms: []string{permissions.BookClub("yaoi")}},
	} {
		got, ok := resourceAudience(resource)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v, %v; want %+v", resource, got, ok, want)
		}
	}
	for _, unknown := range []string{"bookclub-nope", "bookclub", "cards", ""} {
		if _, ok := resourceAudience(unknown); ok {
			t.Errorf("%q is a resource", unknown)
		}
	}
	for resource := range resources {
		if to, _ := resourceAudience(resource); to.Players {
			t.Errorf("%s reaches the boards", resource)
		}
	}
}

// Each stream's events have names of their own, and the document says each event's
// type is its name alone, so a generated client tells them apart.
func TestEventsAreTypedInTheDocument(t *testing.T) {
	doc := Document()
	for path, events := range map[string][]live.Message{"/events": staffEvents(), "/board/{id}/events": boardEvents()} {
		if len(eventMap(events)) != len(events) {
			t.Errorf("%s: two events share a name", path)
		}
		op := doc.Paths[path].Get
		if op == nil || op.Responses["200"].Content["text/event-stream"] == nil {
			t.Fatalf("%s: no event stream in the document", path)
		}
		var names []string
		for _, e := range events {
			names = append(names, e.Name())
			s := doc.Components.Schemas.SchemaFromRef(doc.Components.Schemas.Schema(reflect.TypeOf(e).Elem(), true, "").Ref)
			if s == nil || !slices.Equal(s.Properties["type"].Enum, []any{e.Name()}) {
				t.Errorf("%s: %T's type isn't %q alone", path, e, e.Name())
				continue
			}
			for _, field := range []string{"id", "type", "gameId", "at", "payload"} {
				if !slices.Contains(s.Required, field) {
					t.Errorf("%T doesn't require %s", e, field)
				}
			}
		}
		if !slices.Contains(names, "ping") || !slices.Contains(names, "resync") {
			t.Errorf("%s lacks the ping or resync: %v", path, names)
		}
	}
}

// Only the staff hear the cards, patterns, resources, tea rooms and the log; a board
// hears its card deleted.
func TestStreamsCarryTheirOwnEvents(t *testing.T) {
	staff, board := eventMap(staffEvents()), eventMap(boardEvents())
	for _, name := range []string{"cards_changed", "patterns_changed", "resource_changed", "tea_room_unlocked", "log",
		"halftime_prompt", "halftime_resolved", "auto_draw", "draw_delay"} {
		if _, on := board[name]; on {
			t.Errorf("a board hears %s", name)
		}
		if _, on := staff[name]; !on {
			t.Errorf("the staff don't hear %s", name)
		}
	}
	for _, name := range []string{"card_deleted", "minigame"} {
		if _, on := staff[name]; on {
			t.Errorf("the staff hear %s", name)
		}
	}
}
