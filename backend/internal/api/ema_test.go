package api

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/festival"
	"app-suite/internal/store/storetest"
)

// TestEmaArtistLimitHoldsUnderARace sends many requests for a day's last place at
// once: exactly one gets it, and every other is refused as full rather than slipping
// past the count or failing on the lock.
func TestEmaArtistLimitHoldsUnderARace(t *testing.T) {
	st := storetest.Open(t)
	ctx := t.Context()
	q := db.New(st.DB())
	h := handlers{Deps{Store: st, Logger: slog.Default()}}

	mapID, err := q.CreateFestivalMap(ctx, db.CreateFestivalMapParams{Title: "Obon", Times: "[]"})
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := q.CreateEmaBoard(ctx, db.CreateEmaBoardParams{FestivalMapID: mapID, ArtistName: "Tara", AllowArtist: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.InsertEmaWindow(ctx, db.InsertEmaWindowParams{
		BoardID: boardID, Label: "Day 1", StartAt: "2026-08-01T00:00:00Z", EndAt: "2026-08-02T00:00:00Z", ArtistLimit: 1,
	}); err != nil {
		t.Fatal(err)
	}
	windows, err := q.ListEmaWindows(ctx, boardID)
	if err != nil {
		t.Fatal(err)
	}
	windowID := windows[0].ID

	const racers = 8
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := range racers {
		wg.Go(func() {
			_, errs[i] = h.insertPlaque(ctx, db.InsertEmaPlaqueParams{
				BoardID: boardID, WindowID: sql.NullInt64{Int64: windowID, Valid: true}, WindowLabel: "Day 1",
				Kind: festival.EmaKindArtist, CharacterName: fmt.Sprintf("Visitor %d", i), World: "Gilgamesh",
				Description: "A koi", Status: festival.EmaRequested,
			}, "Tara")
		})
	}
	wg.Wait()

	won := 0
	for i, err := range errs {
		var problem huma.StatusError
		switch {
		case err == nil:
			won++
		case !errors.As(err, &problem) || problem.GetStatus() != http.StatusConflict:
			t.Errorf("racer %d: %v; want success or the day being full (409)", i, err)
		}
	}
	if won != 1 {
		t.Errorf("%d requests got the last place; want exactly 1", won)
	}
	windows, err = q.ListEmaWindows(ctx, boardID)
	if err != nil {
		t.Fatal(err)
	}
	if windows[0].ArtistCount != 1 {
		t.Errorf("the day holds %d requests; want 1", windows[0].ArtistCount)
	}
}

// TestEmaImagePathStaysInThePlaqueFolder checks that a stored plaque image resolves
// only to a file directly inside the plaque folder.
func TestEmaImagePathStaysInThePlaqueFolder(t *testing.T) {
	h := handlers{Deps{DataDir: t.TempDir()}}
	if h.emaImagePath(emaImageRelDir+"/1-abc.png") == "" {
		t.Error("a plaque's own image should resolve")
	}
	for _, rel := range []string{
		"", "images/ema_plaques/", "images/ema_plaques/.hidden.png", "images/ema_plaques/../raffles/a.png",
		"images/raffles/a.png", `images/ema_plaques/..\secret.png`, `images/ema_plaques/a\b.png`,
		"images/ema_plaques/sub/a.png", "/etc/passwd",
	} {
		if p := h.emaImagePath(rel); p != "" {
			t.Errorf("emaImagePath(%q) = %q; want it refused", rel, p)
		}
	}
}
