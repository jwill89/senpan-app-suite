package store_test

import (
	"errors"
	"testing"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

func mustCreateGarapon(t *testing.T, s *store.Store, title string, prizes []model.GaraponPrize) int64 {
	t.Helper()
	id, err := s.CreateGarapon(&model.Garapon{Title: title, Prizes: prizes})
	if err != nil {
		t.Fatalf("CreateGarapon(%s): %v", title, err)
	}
	return id
}

// TestGaraponDrawCapAndWeighting verifies two invariants of the authoritative
// draw: a prize with a non-positive appearance rate is never selected, and a
// player can never draw more than their allowance.
func TestGaraponDrawCapAndWeighting(t *testing.T) {
	s := newTestStore(t)
	// "Nothing" has rate 0 (must never win); "Jackpot" carries all the weight.
	gid := mustCreateGarapon(t, s, "Fest", []model.GaraponPrize{
		{Name: "Nothing", BallColor: "#111111", Rate: 0},
		{Name: "Jackpot", BallColor: "#ffcc00", Rate: 100, IsGrand: true},
	})
	player, err := s.CreateGaraponPlayer(gid, "Aria", 25)
	if err != nil {
		t.Fatalf("CreateGaraponPlayer: %v", err)
	}

	for i := 0; i < 25; i++ {
		d, err := s.RecordGaraponDraw(player.ID)
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		if d.PrizeName != "Jackpot" {
			t.Fatalf("draw %d landed on %q; a 0-rate prize must never win", i, d.PrizeName)
		}
	}
	// The 26th draw exceeds the cap.
	if _, err := s.RecordGaraponDraw(player.ID); !errors.Is(err, store.ErrGaraponNoDraws) {
		t.Fatalf("over-cap draw err = %v; want ErrGaraponNoDraws", err)
	}
	draws, err := s.ListPlayerDraws(player.ID)
	if err != nil {
		t.Fatalf("ListPlayerDraws: %v", err)
	}
	if len(draws) != 25 {
		t.Errorf("player draws = %d; want 25", len(draws))
	}
}

// TestGaraponDeletePlayerGuard verifies a drawing link is deletable only while the
// player has not drawn; once they have, the link is preserved.
func TestGaraponDeletePlayerGuard(t *testing.T) {
	s := newTestStore(t)
	gid := mustCreateGarapon(t, s, "Fest", []model.GaraponPrize{{Name: "Prize", BallColor: "#ffcc00", Rate: 1, IsGrand: true}})

	p1, err := s.CreateGaraponPlayer(gid, "Borin", 3)
	if err != nil {
		t.Fatalf("CreateGaraponPlayer: %v", err)
	}
	deleted, err := s.DeleteGaraponPlayer(p1.ID, false)
	if err != nil || !deleted {
		t.Fatalf("delete unused player: deleted=%v err=%v; want true,nil", deleted, err)
	}

	p2, err := s.CreateGaraponPlayer(gid, "Cyra", 3)
	if err != nil {
		t.Fatalf("CreateGaraponPlayer: %v", err)
	}
	if _, err := s.RecordGaraponDraw(p2.ID); err != nil {
		t.Fatalf("RecordGaraponDraw: %v", err)
	}
	deleted, err = s.DeleteGaraponPlayer(p2.ID, false)
	if err != nil {
		t.Fatalf("delete drawn player: %v", err)
	}
	if deleted {
		t.Error("a player who has drawn must NOT be deletable while the garapon is open")
	}
	if got, _ := s.GetGaraponPlayerByID(p2.ID); got == nil {
		t.Error("drawn player row should still exist after a blocked delete")
	}
}

// TestGaraponClosedDeletePlayerKeepsLog verifies that once a garapon is closed a
// drawing link can be force-deleted even after it has drawn, and the draw stays in
// the log (detached via ON DELETE SET NULL rather than cascade-deleted).
func TestGaraponClosedDeletePlayerKeepsLog(t *testing.T) {
	s := newTestStore(t)
	gid := mustCreateGarapon(t, s, "Fest", []model.GaraponPrize{{Name: "Prize", BallColor: "#ffcc00", Rate: 1, IsGrand: true}})
	p, err := s.CreateGaraponPlayer(gid, "Aria", 3)
	if err != nil {
		t.Fatalf("CreateGaraponPlayer: %v", err)
	}
	if _, err := s.RecordGaraponDraw(p.ID); err != nil {
		t.Fatalf("RecordGaraponDraw: %v", err)
	}
	if err := s.SetGaraponStatus(gid, "closed"); err != nil {
		t.Fatalf("SetGaraponStatus: %v", err)
	}

	deleted, err := s.DeleteGaraponPlayer(p.ID, true) // force (closed garapon)
	if err != nil {
		t.Fatalf("DeleteGaraponPlayer(force): %v", err)
	}
	if !deleted {
		t.Fatal("a closed garapon's drawn link should be force-deletable")
	}
	if pp, _ := s.GetGaraponPlayerByID(p.ID); pp != nil {
		t.Error("drawing link should be gone after force delete")
	}

	// The draw must remain in the garapon's log, with its snapshot intact.
	draws, err := s.ListGaraponDraws(gid)
	if err != nil {
		t.Fatalf("ListGaraponDraws: %v", err)
	}
	if len(draws) != 1 {
		t.Fatalf("draw log = %d; want 1 (kept after the link was deleted)", len(draws))
	}
	if draws[0].PlayerName != "Aria" {
		t.Errorf("kept draw player_name = %q; want %q", draws[0].PlayerName, "Aria")
	}
}

// TestGaraponClosedBlocksDraw verifies a closed garapon refuses draws.
func TestGaraponClosedBlocksDraw(t *testing.T) {
	s := newTestStore(t)
	gid := mustCreateGarapon(t, s, "Fest", []model.GaraponPrize{{Name: "Prize", BallColor: "#ffcc00", Rate: 1, IsGrand: true}})
	p, err := s.CreateGaraponPlayer(gid, "Dee", 3)
	if err != nil {
		t.Fatalf("CreateGaraponPlayer: %v", err)
	}
	if err := s.SetGaraponStatus(gid, "closed"); err != nil {
		t.Fatalf("SetGaraponStatus: %v", err)
	}
	if _, err := s.RecordGaraponDraw(p.ID); !errors.Is(err, store.ErrGaraponClosed) {
		t.Fatalf("draw on closed garapon err = %v; want ErrGaraponClosed", err)
	}
}

// TestDeleteGaraponCascades verifies that deleting a garapon removes ALL of its
// associated rows — prizes, drawing links (players), and draw-log entries — via
// the ON DELETE CASCADE foreign keys (which only fire with PRAGMA foreign_keys =
// ON). This guards the admin "delete garapon" action against leaving orphaned
// links/logs behind.
func TestDeleteGaraponCascades(t *testing.T) {
	s := newTestStore(t)
	gid := mustCreateGarapon(t, s, "Fest", []model.GaraponPrize{
		{Name: "A", BallColor: "#111111", Rate: 50, IsGrand: true},
		{Name: "B", BallColor: "#222222", Rate: 50},
	})
	p, err := s.CreateGaraponPlayer(gid, "Aria", 3)
	if err != nil {
		t.Fatalf("CreateGaraponPlayer: %v", err)
	}
	if _, err := s.RecordGaraponDraw(p.ID); err != nil {
		t.Fatalf("RecordGaraponDraw: %v", err)
	}

	// Precondition: the link + draw exist before the delete.
	if players, _ := s.ListGaraponPlayers(gid); len(players) != 1 {
		t.Fatalf("precondition: want 1 drawing link, got %d", len(players))
	}
	if draws, _ := s.ListGaraponDraws(gid); len(draws) != 1 {
		t.Fatalf("precondition: want 1 draw, got %d", len(draws))
	}

	deleted, err := s.DeleteGarapon(gid)
	if err != nil {
		t.Fatalf("DeleteGarapon: %v", err)
	}
	if !deleted {
		t.Fatal("DeleteGarapon reported no row deleted")
	}

	// The garapon and every associated row must be gone.
	if g, _ := s.GetGarapon(gid); g != nil {
		t.Error("garapon still present after delete")
	}
	if players, _ := s.ListGaraponPlayers(gid); len(players) != 0 {
		t.Errorf("drawing links not cascade-deleted: %d remain", len(players))
	}
	if draws, _ := s.ListGaraponDraws(gid); len(draws) != 0 {
		t.Errorf("draw-log entries not cascade-deleted: %d remain", len(draws))
	}
	if pp, _ := s.GetGaraponPlayerByID(p.ID); pp != nil {
		t.Error("player row not cascade-deleted")
	}
}

// TestGaraponListAggregatesAndUpdate verifies the admin list aggregates
// (player/draw counts) and that replacing a garapon's prizes on update preserves
// the existing draw log (draws snapshot the prize, so history survives edits).
func TestGaraponListAggregatesAndUpdate(t *testing.T) {
	s := newTestStore(t)
	gid := mustCreateGarapon(t, s, "Fest", []model.GaraponPrize{
		{Name: "A", BallColor: "#111111", Rate: 50, IsGrand: true},
		{Name: "B", BallColor: "#222222", Rate: 50},
	})
	pa, err := s.CreateGaraponPlayer(gid, "Aria", 2)
	if err != nil {
		t.Fatalf("CreateGaraponPlayer: %v", err)
	}
	if _, err := s.CreateGaraponPlayer(gid, "Borin", 2); err != nil {
		t.Fatalf("CreateGaraponPlayer: %v", err)
	}
	if _, err := s.RecordGaraponDraw(pa.ID); err != nil {
		t.Fatalf("RecordGaraponDraw: %v", err)
	}

	list, err := s.ListGarapons()
	if err != nil {
		t.Fatalf("ListGarapons: %v", err)
	}
	var g *model.Garapon
	for i := range list {
		if list[i].ID == gid {
			g = &list[i]
		}
	}
	if g == nil {
		t.Fatal("created garapon missing from list")
	}
	if g.PlayerCount != 2 {
		t.Errorf("player_count = %d; want 2", g.PlayerCount)
	}
	if g.DrawCount != 1 {
		t.Errorf("draw_count = %d; want 1", g.DrawCount)
	}

	if err := s.UpdateGarapon(&model.Garapon{ID: gid, Title: "Fest", Prizes: []model.GaraponPrize{
		{Name: "X", BallColor: "#aabbcc", Rate: 1, IsGrand: true},
	}}); err != nil {
		t.Fatalf("UpdateGarapon: %v", err)
	}
	got, err := s.GetGarapon(gid)
	if err != nil {
		t.Fatalf("GetGarapon: %v", err)
	}
	if len(got.Prizes) != 1 || got.Prizes[0].Name != "X" {
		t.Errorf("after update prizes = %+v; want single X", got.Prizes)
	}
	if draws, _ := s.ListGaraponDraws(gid); len(draws) != 1 {
		t.Errorf("draw log lost after prize replace: got %d; want 1", len(draws))
	}
}
