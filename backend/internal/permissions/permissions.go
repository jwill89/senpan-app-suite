// Package permissions names the page permissions an admin grants an account, one
// per admin page. An admin holds every one; the Users page is admin-only and not
// grantable. The SPA keeps the same keys (frontend/src/lib/permissions.ts).
package permissions

import "app-suite/internal/bookclubs"

// The grantable keys.
const (
	BingoGame             = "bingo-game"
	BingoCards            = "bingo-cards"
	BingoWinnersLog       = "bingo-winners-log"
	BingoPatterns         = "bingo-patterns"
	BingoPresets          = "bingo-presets"
	TeahouseAnnouncements = "teahouse-announcements"
	TeahouseAffiliates    = "teahouse-affiliates"
	TeahouseTeaRooms      = "teahouse-tea-rooms"
	TeahouseRaffles       = "teahouse-raffles"
	FestivalMap           = "festival-map"
	FestivalGarapon       = "festival-garapon"
	FestivalStampRally    = "festival-stamp-rally"
	FestivalEma           = "festival-ema"
	FestivalEmaRequests   = "festival-ema-requests"
	AtelierFonts          = "atelier-fonts"
	AtelierCarrd          = "atelier-carrd"
	SystemSettings        = "system-settings"
	SystemThemes          = "system-themes"
	SystemImages          = "system-images"
)

// ImageBrowsers are the pages whose editors pick pictures from the image library, so
// any of them may read it: the Images page, and every page with a picture field
// (a festival map's base plan and a tea room's art among them).
func ImageBrowsers() []string {
	return []string{
		SystemImages, TeahouseAnnouncements, TeahouseRaffles, TeahouseAffiliates,
		FestivalGarapon, FestivalStampRally, SystemThemes, FestivalMap, TeahouseTeaRooms, FestivalEma,
	}
}

// BookClub is the key for one book club's reading lists.
func BookClub(slug string) string { return "bookclub-" + slug }

// Valid is every grantable key.
func Valid() map[string]bool {
	keys := []string{
		BingoGame, BingoCards, BingoWinnersLog, BingoPatterns, BingoPresets,
		TeahouseAnnouncements, TeahouseAffiliates, TeahouseTeaRooms, TeahouseRaffles,
		FestivalMap, FestivalGarapon, FestivalStampRally, FestivalEma, FestivalEmaRequests,
		AtelierFonts, AtelierCarrd,
		SystemSettings, SystemThemes, SystemImages,
	}
	set := make(map[string]bool, len(keys)+len(bookclubs.Clubs))
	for _, k := range keys {
		set[k] = true
	}
	for _, slug := range bookclubs.Slugs() {
		set[BookClub(slug)] = true
	}
	return set
}
