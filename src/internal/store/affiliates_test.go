package store_test

import (
	"testing"

	"app-suite/internal/model"
)

// TestAffiliatesCRUD exercises create → get → list → update → delete and verifies
// the owners/hours JSON columns round-trip (including order preservation).
func TestAffiliatesCRUD(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateAffiliate(&model.Affiliate{
		Name:     "The Tipsy Moogle",
		Owners:   []string{"Tataru", "Hildibrand"},
		Location: "Ul'dah, Steps of Nald",
		Timezone: "America/New_York",
		Hours: []model.AffiliateHour{
			{Label: "Mon–Fri", Start: "18:00", End: "23:00"},
			{Label: "Weekends", Start: "12:00"}, // open-ended
		},
		Details:    "**Cozy** tavern.",
		Logo:       "images/affiliate_logos/moogle.png",
		Screenshot: "images/affiliate_images/moogle.png",
	})
	if err != nil {
		t.Fatalf("CreateAffiliate: %v", err)
	}

	got, err := s.GetAffiliate(id)
	if err != nil || got == nil {
		t.Fatalf("GetAffiliate: got=%v err=%v", got, err)
	}
	if got.Name != "The Tipsy Moogle" {
		t.Errorf("name = %q; want The Tipsy Moogle", got.Name)
	}
	if len(got.Owners) != 2 || got.Owners[0] != "Tataru" || got.Owners[1] != "Hildibrand" {
		t.Errorf("owners = %v; want [Tataru Hildibrand] in order", got.Owners)
	}
	if len(got.Hours) != 2 || got.Hours[0].Label != "Mon–Fri" || got.Hours[1].End != "" {
		t.Errorf("hours = %+v; want 2 rows, first labelled Mon–Fri, second open-ended", got.Hours)
	}
	if got.Timezone != "America/New_York" {
		t.Errorf("timezone = %q; want America/New_York", got.Timezone)
	}

	// Update: rename + replace owners/hours.
	got.Name = "The Drowned Moogle"
	got.Owners = []string{"Tataru"}
	got.Hours = []model.AffiliateHour{{Start: "09:00", End: "17:00"}}
	if err := s.UpdateAffiliate(got); err != nil {
		t.Fatalf("UpdateAffiliate: %v", err)
	}
	reloaded, err := s.GetAffiliate(id)
	if err != nil || reloaded == nil {
		t.Fatalf("GetAffiliate after update: got=%v err=%v", reloaded, err)
	}
	if reloaded.Name != "The Drowned Moogle" || len(reloaded.Owners) != 1 || len(reloaded.Hours) != 1 {
		t.Errorf("after update: %+v; want renamed, 1 owner, 1 hours row", reloaded)
	}

	// List is alphabetical by name.
	if _, err := s.CreateAffiliate(&model.Affiliate{Name: "Aether Lounge"}); err != nil {
		t.Fatalf("CreateAffiliate (second): %v", err)
	}
	list, err := s.ListAffiliates()
	if err != nil {
		t.Fatalf("ListAffiliates: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d; want 2", len(list))
	}
	if list[0].Name != "Aether Lounge" || list[1].Name != "The Drowned Moogle" {
		t.Errorf("list order = [%q, %q]; want alphabetical", list[0].Name, list[1].Name)
	}

	// Delete.
	deleted, err := s.DeleteAffiliate(id)
	if err != nil || !deleted {
		t.Fatalf("DeleteAffiliate: deleted=%v err=%v; want true,nil", deleted, err)
	}
	if gone, _ := s.GetAffiliate(id); gone != nil {
		t.Errorf("GetAffiliate after delete = %+v; want nil", gone)
	}
}

// TestAffiliateEmptyCollectionsRoundTrip verifies nil owners/hours persist and
// read back as empty (non-nil) slices, never JSON null.
func TestAffiliateEmptyCollectionsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateAffiliate(&model.Affiliate{Name: "Bare Minimum"})
	if err != nil {
		t.Fatalf("CreateAffiliate: %v", err)
	}
	got, err := s.GetAffiliate(id)
	if err != nil || got == nil {
		t.Fatalf("GetAffiliate: got=%v err=%v", got, err)
	}
	if got.Owners == nil || len(got.Owners) != 0 {
		t.Errorf("owners = %v; want empty non-nil slice", got.Owners)
	}
	if got.Hours == nil || len(got.Hours) != 0 {
		t.Errorf("hours = %v; want empty non-nil slice", got.Hours)
	}
}
