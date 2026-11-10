package festival

import (
	"strings"
	"testing"
	"time"
)

func win(label, start, end string) EmaWindow {
	return EmaWindow{Label: label, StartAt: start, EndAt: end}
}

func board(windows ...EmaWindow) EmaBoard {
	return EmaBoard{FestivalMapID: 1, TemplateImage: "images/t.png", AllowSite: true, Windows: windows}
}

func TestCheckBoard(t *testing.T) {
	day1 := win("Day 1", "2026-08-01T00:00:00Z", "2026-08-02T00:00:00Z")
	day2 := win("Day 2", "2026-08-02T00:00:00Z", "2026-08-03T00:00:00Z")
	with := func(b EmaBoard, f func(*EmaBoard)) EmaBoard {
		f(&b)
		return b
	}
	for name, c := range map[string]struct {
		board EmaBoard
		field string
	}{
		"no festival":                {with(board(day1), func(b *EmaBoard) { b.FestivalMapID = 0 }), "festivalMapId"},
		"no way to write":            {with(board(day1), func(b *EmaBoard) { b.AllowSite = false }), "allowSelf"},
		"design needs the template":  {with(board(day1), func(b *EmaBoard) { b.TemplateImage = " " }), "templateImage"},
		"link must be a web address": {with(board(day1), func(b *EmaBoard) { b.DiscordLink = "ftp://x" }), "discordLink"},
		"no days":                    {board(), "windows"},
		"unreadable start":           {board(win("Day 1", "soon", "2026-08-02T00:00:00Z")), "windows[0].startAt"},
		"ends first":                 {board(win("Day 1", "2026-08-02T00:00:00Z", "2026-08-01T00:00:00Z")), "windows[0].endAt"},
		"not a webhook":              {board(EmaWindow{Label: "Day 1", StartAt: day1.StartAt, EndAt: day1.EndAt, WebhookURL: "https://example.com/x"}), "windows[0].webhookUrl"},
		"limit below zero":           {board(EmaWindow{Label: "Day 1", StartAt: day1.StartAt, EndAt: day1.EndAt, ArtistLimit: -1}), "windows[0].artistLimit"},
		"limit past the cap":         {board(EmaWindow{Label: "Day 1", StartAt: day1.StartAt, EndAt: day1.EndAt, ArtistLimit: MaxArtistLimit + 1}), "windows[0].artistLimit"},
		"days overlap":               {board(win("Day 1", "2026-08-01T00:00:00Z", "2026-08-02T06:00:00Z"), day2), "windows"},
	} {
		bad := CheckBoard(&c.board)
		if bad == nil || bad.Field != c.field {
			t.Errorf("%s: got %v; want an error at %s", name, bad, c.field)
		}
	}

	// Touching days don't overlap; the board is put as it is stored.
	ok := board(win("", " 2026-08-01T02:00:00+02:00 ", "2026-08-02T00:00:00Z"), day2)
	ok.ArtistName = "  "
	ok.Windows[0].WebhookURL = " https://discord.com/api/webhooks/1/x "
	if bad := CheckBoard(&ok); bad != nil {
		t.Fatalf("CheckBoard = %v; want a good board", bad)
	}
	if ok.ArtistName != DefaultArtistName {
		t.Errorf("artist = %q; want the default", ok.ArtistName)
	}
	if ok.Windows[0].Label != "Day 1" || ok.Windows[0].StartAt != "2026-08-01T00:00:00Z" {
		t.Errorf("first day = %+v; want a label and the start in UTC", ok.Windows[0])
	}
	if !strings.HasPrefix(ok.Windows[0].WebhookURL, "https://") || strings.HasSuffix(ok.Windows[0].WebhookURL, " ") {
		t.Errorf("webhook = %q; want it trimmed", ok.Windows[0].WebhookURL)
	}
}

func TestCurrentDay(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	days := []Day{
		{ID: 1, Start: start, End: start.Add(24 * time.Hour)},
		{ID: 2, Start: start.Add(24 * time.Hour), End: start.Add(48 * time.Hour)},
	}
	for name, c := range map[string]struct {
		now  time.Time
		want int
	}{
		"before":                  {start.Add(-time.Second), -1},
		"the start is inside":     {start, 0},
		"the end belongs to next": {start.Add(24 * time.Hour), 1},
		"inside the second":       {start.Add(30 * time.Hour), 1},
		"after":                   {start.Add(48 * time.Hour), -1},
	} {
		if got := CurrentDay(days, c.now); got != c.want {
			t.Errorf("%s: day %d; want %d", name, got, c.want)
		}
	}
}
