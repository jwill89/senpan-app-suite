package store_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// TestEmaArtistLimitHoldsUnderARace sends many requests for a window's last place
// at once: exactly one gets it, and every other is refused as full rather than
// slipping past the count or failing on the lock.
func TestEmaArtistLimitHoldsUnderARace(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	mapID, err := s.CreateFestivalMap(ctx, &model.FestivalMap{Title: "Obon", Status: model.MapStatusPublished})
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := s.CreateEmaBoard(ctx, &model.EmaBoard{
		FestivalMapID: mapID, ArtistName: "Tara", AllowArtist: true, Status: "open",
		Windows: []model.EmaWindow{{
			Label: "Day 1", StartAt: "2026-08-01T00:00:00Z", EndAt: "2026-08-02T00:00:00Z", ArtistLimit: 1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	board, err := s.GetEmaBoard(ctx, boardID)
	if err != nil {
		t.Fatal(err)
	}
	windowID := board.Windows[0].ID

	const racers = 8
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := range racers {
		wg.Go(func() {
			_, errs[i] = s.CreateEmaPlaque(ctx, &model.EmaPlaque{
				BoardID: boardID, WindowID: &windowID, WindowLabel: "Day 1", Kind: model.EmaKindArtist,
				CharacterName: fmt.Sprintf("Visitor %d", i), World: "Gilgamesh", Description: "A koi",
				Status: model.EmaStatusRequested,
			})
		})
	}
	wg.Wait()

	won := 0
	for i, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, store.ErrEmaArtistFull):
			t.Errorf("racer %d: %v; want success or ErrEmaArtistFull", i, err)
		}
	}
	if won != 1 {
		t.Errorf("%d requests got the last place; want exactly 1", won)
	}
	if board, _ = s.GetEmaBoard(ctx, boardID); board.Windows[0].ArtistCount != 1 {
		t.Errorf("window holds %d requests; want 1", board.Windows[0].ArtistCount)
	}
}
