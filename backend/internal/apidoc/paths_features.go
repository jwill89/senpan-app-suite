package apidoc

import "github.com/getkin/kin-openapi/openapi3"

// buildFeaturePaths registers the raffle/garapon/festival/book-club/announcement/
// system endpoints (continuation of buildPaths, split to keep files readable).
func buildFeaturePaths(b *pb) {
	// ── Raffles (resource-oriented: methods for CRUD, POST /{id}/{verb} commands)
	raffleFields := func() openapi3.Schemas {
		return props(
			"title", pstr("Title (required)."), "description", pstr(""), "rules", pstr(""),
			"max_entries", pint("Per-player cap."), "signup_instructions", pstr(""),
			"cost_per_entry", pnum(""), "available_from", pstr("UTC RFC-3339."),
			"available_to", pstr("UTC RFC-3339."), "prize_image", pstr(""))
	}
	b.add("GET", "/api/raffles", "Raffles", "List raffles", "public",
		"Role-filtered: admins see all; the public sees only open raffles within their availability window.", opt{
			resps: []respEntry{ok("RafflesResponse")}})
	b.add("POST", "/api/raffles", "Raffles", "Create a raffle", "permission:teahouse-raffles", "", opt{
		body:  actionBody("Raffle fields.", nil, raffleFields()),
		resps: []respEntry{created("RaffleResponse"), r("400", "Title required")}})
	b.add("GET", "/api/raffles/{id}", "Raffles", "Raffle detail", "public",
		"Admins get `entries`; the public gets `winner_entry` on a closed, verified raffle.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Raffle id.")},
			resps: []respEntry{ok("RaffleDetailResponse"), r("400", "Invalid id"), r("404", "Not found")}})
	b.add("PUT", "/api/raffles/{id}", "Raffles", "Replace a raffle", "permission:teahouse-raffles",
		"Full replace of the editable fields (status/winner are preserved).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Raffle id.")},
			body:  actionBody("Full raffle fields.", nil, raffleFields()),
			resps: []respEntry{ok("RaffleResponse"), r("400", "Title required")}})
	b.add("DELETE", "/api/raffles/{id}", "Raffles", "Delete a raffle", "permission:teahouse-raffles", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Raffle id.")},
		resps: []respEntry{noContent()}})
	b.add("POST", "/api/raffles/{id}/enter", "Raffles", "Enter a raffle", "public", "", opt{
		path: []*openapi3.Parameter{pparam("id", "Raffle id.")},
		body: actionBody("Public sign-up.", nil, props(
			"character_name", pstr("Required."), "world", pstr("Required."), "num_entries", pint("Tickets (≥1)."))),
		resps: []respEntry{{"200", jsonResp("Merged into an existing entry", "RaffleEnterResponse")}, {"201", jsonResp("New entry", "RaffleEnterResponse")}, r("400", "Closed / cap exceeded / outside window"), r("404", "Not found")}})
	b.add("POST", "/api/raffles/{id}/entries", "Raffles", "Add an entry (admin)", "permission:teahouse-raffles",
		"Admin add; skips the availability window but enforces the per-player cap. 201 when a new entry is created, 200 when merged into an existing one.", opt{
			path: []*openapi3.Parameter{pparam("id", "Raffle id.")},
			body: actionBody("Entry to add.", nil, props(
				"character_name", pstr("Required."), "world", pstr("Required."),
				"num_entries", pint("Tickets (≥1)."), "paid", pbool("Mark paid immediately."))),
			resps: []respEntry{created("RaffleEntryResponse"), {"200", jsonResp("Merged into an existing entry", "RaffleEntryResponse")}, r("400", "Invalid / cap exceeded")}})
	b.add("PATCH", "/api/raffles/{id}/entries/{entryId}", "Raffles", "Update an entry's paid flag", "permission:teahouse-raffles", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Raffle id."), pparam("entryId", "Entry id.")},
		body:  actionBody("Entry patch.", nil, props("paid", pbool("Paid flag."))),
		resps: []respEntry{ok("RaffleEntryResponse"), r("404", "Entry not found")}})
	b.add("DELETE", "/api/raffles/{id}/entries/{entryId}", "Raffles", "Delete an entry", "permission:teahouse-raffles", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Raffle id."), pparam("entryId", "Entry id.")},
		resps: []respEntry{noContent()}})
	b.add("POST", "/api/raffles/{id}/pick-winner", "Raffles", "Pick a winner", "permission:teahouse-raffles",
		"Selects a random paid entry as the pending winner.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Raffle id.")},
			resps: []respEntry{ok("RaffleWinnerResponse"), r("400", "No paid entries")}})
	b.add("POST", "/api/raffles/{id}/pick-another", "Raffles", "Re-pick a winner", "permission:teahouse-raffles",
		"Clears the pending winner and picks again.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Raffle id.")},
			resps: []respEntry{ok("RaffleWinnerResponse"), r("400", "No paid entries")}})
	b.add("POST", "/api/raffles/{id}/verify-winner", "Raffles", "Finalize the winner", "permission:teahouse-raffles",
		"Confirms the pending winner and closes the raffle.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Raffle id.")},
			resps: []respEntry{ok("StatusResponse"), r("400", "No winner selected")}})

	// ── Garapon (resource-oriented: methods for CRUD, POST /{id}/{verb} status) ─
	garaponFields := func() openapi3.Schemas {
		return props(
			"title", pstr("Title (required)."), "details", pstr("Markdown."), "grand_prize_image", pstr(""),
			"stamp_rally_id", pint("Optional linked open rally."),
			"prizes", parr("Prize tiers (≥1, exactly one grand).", ref("GaraponPrize")))
	}
	b.add("GET", "/api/garapons", "Garapon", "List garapons", "permission:festival-garapon", "", opt{resps: []respEntry{ok("GaraponsResponse")}})
	b.add("POST", "/api/garapons", "Garapon", "Create a garapon", "permission:festival-garapon", "", opt{
		body:  actionBody("Garapon fields.", nil, garaponFields()),
		resps: []respEntry{created("GaraponResponse"), r("400", "Validation failed")}})
	b.add("GET", "/api/garapons/{id}", "Garapon", "Garapon detail", "permission:festival-garapon", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Garapon id.")},
		resps: []respEntry{ok("GaraponDetailResponse"), r("404", "Not found")}})
	b.add("PUT", "/api/garapons/{id}", "Garapon", "Replace a garapon", "permission:festival-garapon",
		"Full replace of the editable fields (status is preserved — use close/reopen).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Garapon id.")},
			body:  actionBody("Full garapon fields.", nil, garaponFields()),
			resps: []respEntry{ok("OKResponse"), r("400", "Validation failed")}})
	b.add("DELETE", "/api/garapons/{id}", "Garapon", "Delete a garapon", "permission:festival-garapon", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Garapon id.")},
		resps: []respEntry{noContent()}})
	b.add("POST", "/api/garapons/{id}/close", "Garapon", "Close a garapon", "permission:festival-garapon",
		"Closes the garapon (no further draws).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Garapon id.")},
			resps: []respEntry{ok("StatusResponse")}})
	b.add("POST", "/api/garapons/{id}/reopen", "Garapon", "Reopen a garapon", "permission:festival-garapon",
		"Reopens a closed garapon.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Garapon id.")},
			resps: []respEntry{ok("StatusResponse")}})
	b.add("POST", "/api/garapons/{id}/players", "Garapon", "Create a drawing link", "permission:festival-garapon",
		"Issues a per-player drawing link (returns its token).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Garapon id.")},
			body:  actionBody("New drawing link.", nil, props("player_name", pstr("Required."), "max_draws", pint("≥1."))),
			resps: []respEntry{created("GaraponPlayerResponse"), r("404", "Not found")}})
	b.add("DELETE", "/api/garapons/{id}/players/{playerId}", "Garapon", "Delete a drawing link", "permission:festival-garapon",
		"A link that has already drawn can only be deleted once the garapon is closed (the draw stays in the log).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Garapon id."), pparam("playerId", "Drawing-link id.")},
			resps: []respEntry{noContent(), r("404", "Not found"), r("409", "Already drawn")}})
	b.add("GET", "/api/garapon/{token}", "Garapon", "Public player view", "public",
		"Tokenized: no odds (prize rates zeroed).", opt{
			path:  []*openapi3.Parameter{pparam("token", "The player's private link token.")},
			resps: []respEntry{ok("GaraponPublicResponse"), r("404", "Not found")}})
	b.add("POST", "/api/garapon/{token}/draw", "Garapon", "Perform a draw", "public", "", opt{
		path:  []*openapi3.Parameter{pparam("token", "The player's private link token.")},
		resps: []respEntry{ok("GaraponDrawResponse"), r("400", "Closed / no prizes"), r("409", "No draws remaining"), r("404", "Not found")}})

	// ── Affiliates (resource-oriented: methods for CRUD) ──────────────────────
	affiliateFields := func() openapi3.Schemas {
		return props(
			"name", pstr("Name (required)."), "owners", parr("", pstr("")), "location", pstr(""),
			"timezone", pstr("IANA zone."), "hours", parr("", ref("AffiliateHour")), "details", pstr("Markdown."),
			"logo", pstr(""), "screenshot", pstr(""))
	}
	b.add("GET", "/api/affiliates", "Affiliates", "List affiliates", "permission:teahouse-affiliates", "", opt{resps: []respEntry{ok("AffiliatesResponse")}})
	b.add("POST", "/api/affiliates", "Affiliates", "Create an affiliate", "permission:teahouse-affiliates", "", opt{
		body:  actionBody("Affiliate fields.", nil, affiliateFields()),
		resps: []respEntry{created("AffiliateResponse"), r("400", "Name required")}})
	b.add("PUT", "/api/affiliates/{id}", "Affiliates", "Replace an affiliate", "permission:teahouse-affiliates", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Affiliate id.")},
		body:  actionBody("Full affiliate fields.", nil, affiliateFields()),
		resps: []respEntry{ok("OKResponse"), r("400", "Name required")}})
	b.add("DELETE", "/api/affiliates/{id}", "Affiliates", "Delete an affiliate", "permission:teahouse-affiliates", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Affiliate id.")},
		resps: []respEntry{noContent()}})

	// ── Stamp Rally (resource-oriented: methods for CRUD, POST /{id}/{verb}) ────
	rallyFields := func() openapi3.Schemas {
		return props(
			"title", pstr("Title (required)."), "card_image", pstr(""), "not_stamped_image", pstr(""),
			"available_from", pstr("UTC RFC-3339."), "available_to", pstr("UTC RFC-3339."),
			"details", pstr("Markdown."), "redeem_instructions", pstr("Markdown."),
			"stamps", parr("", ref("StampRallyStamp")), "prizes", parr("", ref("StampRallyPrize")))
	}
	b.add("GET", "/api/stamp-rallies", "Stamp Rally", "List rallies", "permission:festival-stamp-rally", "", opt{resps: []respEntry{ok("StampRalliesResponse")}})
	b.add("POST", "/api/stamp-rallies", "Stamp Rally", "Create a rally", "permission:festival-stamp-rally", "", opt{
		body:  actionBody("Rally fields.", nil, rallyFields()),
		resps: []respEntry{created("StampRallyResponse"), r("400", "Title required")}})
	b.add("GET", "/api/stamp-rallies/{id}", "Stamp Rally", "Rally detail", "permission:festival-stamp-rally", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Rally id.")},
		resps: []respEntry{ok("StampRallyDetailResponse"), r("404", "Not found")}})
	b.add("PUT", "/api/stamp-rallies/{id}", "Stamp Rally", "Replace a rally", "permission:festival-stamp-rally",
		"Full replace of the editable fields (status is preserved — use close/reopen).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Rally id.")},
			body:  actionBody("Full rally fields.", nil, rallyFields()),
			resps: []respEntry{ok("OKResponse"), r("400", "Title required")}})
	b.add("DELETE", "/api/stamp-rallies/{id}", "Stamp Rally", "Delete a rally", "permission:festival-stamp-rally", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Rally id.")},
		resps: []respEntry{noContent()}})
	b.add("GET", "/api/stamp-rallies/{id}/logs", "Stamp Rally", "Event stamp log", "permission:festival-stamp-rally", "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Rally id.")},
		resps: []respEntry{ok("StampRallyLogsResponse")}})
	b.add("POST", "/api/stamp-rallies/{id}/close", "Stamp Rally", "Close a rally", "permission:festival-stamp-rally",
		"Closes the rally (read-only; moves to the closed table).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Rally id.")},
			resps: []respEntry{ok("StatusResponse")}})
	b.add("POST", "/api/stamp-rallies/{id}/reopen", "Stamp Rally", "Reopen a rally", "permission:festival-stamp-rally",
		"Reopens a closed rally.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Rally id.")},
			resps: []respEntry{ok("StatusResponse")}})
	b.add("PATCH", "/api/stamp-rallies/{id}/stamps/{stampId}", "Stamp Rally", "Pause/resume a stall", "permission:festival-stamp-rally",
		"Toggles a single stamp's paused flag without a full event re-save.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Rally id."), pparam("stampId", "Stamp id.")},
			body:  actionBody("Pause toggle.", nil, props("paused", pbool("Paused flag."))),
			resps: []respEntry{ok("PausedResponse"), r("404", "Stamp not found")}})
	b.add("POST", "/api/stamp-rallies/{id}/cards", "Stamp Rally", "Create a participant card", "permission:festival-stamp-rally",
		"Issues a tokenized participant card link (returns its token).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Rally id.")},
			body:  actionBody("New card.", nil, props("participant_name", pstr("Required."))),
			resps: []respEntry{created("StampRallyCardResponse"), r("400", "Name required"), r("404", "Not found")}})
	b.add("DELETE", "/api/stamp-rallies/{id}/cards/{cardId}", "Stamp Rally", "Delete a participant card", "permission:festival-stamp-rally",
		"A card with collected stamps can only be deleted once the rally is closed (the stamp log is kept).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Rally id."), pparam("cardId", "Card id.")},
			resps: []respEntry{noContent(), r("404", "Not found"), r("409", "Card has stamps")}})
	b.add("GET", "/api/stamp-card/{token}", "Stamp Rally", "Public card view", "public", "", opt{
		path: []*openapi3.Parameter{pparam("token", "The participant's card token.")},
		resps: []respEntry{ok("PublicStampCard"), r("404", "Not found")}})
	b.add("POST", "/api/stamp-card/{token}/stamp", "Stamp Rally", "Collect a stamp", "public", "", opt{
		path: []*openapi3.Parameter{pparam("token", "The participant's card token.")},
		body: actionBody("Stamp collection.", nil, props("password", pstr("The stall's password (required)."))),
		resps: []respEntry{ok("StampSubmitResponse"), r("400", "Wrong/empty password / stall closed"), r("409", "Already collected")}})

	// ── Book Club ─────────────────────────────────────────────────────────────
	b.add("POST", "/api/bookclub/upload", "Book Club", "Upload a cover image", "any-bookclub",
		"multipart field `image` (max 5 MB; jpg/png/webp/gif). Keeps the uploaded filename.", opt{
			body:  multipartBody("Cover image.", props("image", pbinary("Image file."))),
			resps: []respEntry{ok("BookclubUploadResponse"), r("400", "Invalid image")}})
	b.add("GET", "/api/bookclub/lookup", "Book Club", "AniList lookup", "any-bookclub", "", opt{
		query: []*openapi3.Parameter{qparam("q", "Search query.", false), qparam("id", "AniList media id (wins over q).", false)},
		resps: []respEntry{ok("BookclubLookupResponse"), r("400", "q or id required"), r("502", "AniList failed")}})
	clubParam := func() *openapi3.Parameter { return pparam("club", "Book club slug, e.g. \"yaoi\" or \"yuri\".") }
	b.add("GET", "/api/book-clubs/{club}/reading-lists", "Book Club", "List reading lists", "permission:bookclub-<club>", "", opt{
		path:  []*openapi3.Parameter{clubParam()},
		resps: []respEntry{ok("ReadingListsResponse")}})
	b.add("POST", "/api/book-clubs/{club}/reading-lists", "Book Club", "Create a reading list", "permission:bookclub-<club>",
		"The owning club comes from the path; the caller must hold that club's page permission.", opt{
			path: []*openapi3.Parameter{clubParam()},
			body: actionBody("New reading list.", nil, props("title", pstr("List title (required)."))),
			resps: []respEntry{created("ReadingListDetailResponse"), r("400", "Title required")}})
	b.add("GET", "/api/book-clubs/{club}/reading-lists/{id}", "Book Club", "Reading list detail", "permission:bookclub-<club>", "", opt{
		path: []*openapi3.Parameter{clubParam(), pparam("id", "List id.")},
		resps: []respEntry{ok("ReadingListDetailResponse"), r("404", "Not found")}})
	b.add("PUT", "/api/book-clubs/{club}/reading-lists/{id}", "Book Club", "Rename a reading list", "permission:bookclub-<club>", "", opt{
		path: []*openapi3.Parameter{clubParam(), pparam("id", "List id.")},
		body: actionBody("New title.", nil, props("title", pstr("List title (required)."))),
		resps: []respEntry{ok("OKResponse"), r("400", "Title required"), r("404", "Not found")}})
	b.add("DELETE", "/api/book-clubs/{club}/reading-lists/{id}", "Book Club", "Delete a reading list", "permission:bookclub-<club>",
		"Cascade-deletes its items and cleans up any orphaned cover images.", opt{
			path:  []*openapi3.Parameter{clubParam(), pparam("id", "List id.")},
			resps: []respEntry{noContent(), r("404", "Not found")}})
	b.add("POST", "/api/book-clubs/{club}/reading-lists/{id}/items", "Book Club", "Create an item", "permission:bookclub-<club>", "", opt{
		path: []*openapi3.Parameter{clubParam(), pparam("id", "List id.")},
		body: actionBody("New item.", nil, props("item", ref("ReadingListItem"))),
		resps: []respEntry{created("ReadingListItemResponse"), r("400", "Title required"), r("404", "Not found")}})
	b.add("PUT", "/api/book-clubs/{club}/reading-lists/{id}/items/{itemId}", "Book Club", "Replace an item", "permission:bookclub-<club>", "", opt{
		path: []*openapi3.Parameter{clubParam(), pparam("id", "List id."), pparam("itemId", "Item id.")},
		body: actionBody("Full item fields.", nil, props("item", ref("ReadingListItem"))),
		resps: []respEntry{ok("ReadingListItemResponse"), r("400", "Title required"), r("404", "Not found")}})
	b.add("DELETE", "/api/book-clubs/{club}/reading-lists/{id}/items/{itemId}", "Book Club", "Delete an item", "permission:bookclub-<club>",
		"Cleans up the item's cover image when no other item references it.", opt{
			path:  []*openapi3.Parameter{clubParam(), pparam("id", "List id."), pparam("itemId", "Item id.")},
			resps: []respEntry{noContent(), r("404", "Not found")}})
	b.add("POST", "/api/book-clubs/{club}/reading-lists/{id}/publish", "Book Club", "Publish to Discord", "permission:bookclub-<club>", "", opt{
		path: []*openapi3.Parameter{clubParam(), pparam("id", "List id.")},
		resps: []respEntry{ok("PublishResponse"), r("400", "No items / no webhook"), r("502", "Discord failed")}})

	// ── Announcements ─────────────────────────────────────────────────────────
	ann := "permission:teahouse-announcements"
	typeFields := func() openapi3.Schemas {
		return props("name", pstr("Type name (required)."), "webhook_url", pstr("Discord webhook URL."))
	}
	b.add("GET", "/api/announcement-types", "Announcements", "List types", ann, "", opt{resps: []respEntry{ok("AnnouncementTypesResponse")}})
	b.add("POST", "/api/announcement-types", "Announcements", "Create a type", ann, "", opt{
		body:  actionBody("Type fields.", nil, typeFields()),
		resps: []respEntry{created("AnnouncementTypeResponse"), r("400", "Name / webhook invalid")}})
	b.add("PUT", "/api/announcement-types/{id}", "Announcements", "Replace a type", ann, "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Type id.")},
		body:  actionBody("Full type fields.", nil, typeFields()),
		resps: []respEntry{ok("AnnouncementTypeResponse"), r("400", "Name / webhook invalid")}})
	b.add("DELETE", "/api/announcement-types/{id}", "Announcements", "Delete a type", ann,
		"Refused while the type is still used by any announcement.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Type id.")},
			resps: []respEntry{noContent(), r("400", "Type still in use")}})
	roleFields := func() openapi3.Schemas {
		return props("name", pstr("Role name (required)."), "role_id", pstr("Discord snowflake (required)."))
	}
	b.add("GET", "/api/announcement-roles", "Announcements", "List roles", ann, "", opt{resps: []respEntry{ok("AnnouncementRolesResponse")}})
	b.add("POST", "/api/announcement-roles", "Announcements", "Create a role", ann, "", opt{
		body:  actionBody("Role fields.", nil, roleFields()),
		resps: []respEntry{created("AnnouncementRoleResponse"), r("400", "Name / role id invalid")}})
	b.add("PUT", "/api/announcement-roles/{id}", "Announcements", "Replace a role", ann, "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Role id.")},
		body:  actionBody("Full role fields.", nil, roleFields()),
		resps: []respEntry{ok("AnnouncementRoleResponse"), r("400", "Name / role id invalid")}})
	b.add("DELETE", "/api/announcement-roles/{id}", "Announcements", "Delete a role", ann,
		"Refused while the role is still tagged by any announcement.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Role id.")},
			resps: []respEntry{noContent(), r("400", "Role still in use")}})
	b.add("GET", "/api/announcements", "Announcements", "List announcements", ann, "", opt{resps: []respEntry{ok("AnnouncementsResponse")}})
	b.add("POST", "/api/announcements", "Announcements", "Create an announcement", ann, "", opt{
		body:  actionBody("New announcement.", nil, props("announcement", ref("Announcement"))),
		resps: []respEntry{created("AnnouncementResponse"), r("400", "Validation failed")}})
	b.add("POST", "/api/announcements/reorder", "Announcements", "Reorder announcements", ann,
		"Persists a new drag-and-drop order (top-first ids).", opt{
			body:  actionBody("Bulk reorder.", nil, props("ordered_ids", parr("Announcement ids in the new order.", pint("")))),
			resps: []respEntry{ok("OKResponse")}})
	b.add("PUT", "/api/announcements/{id}", "Announcements", "Replace an announcement", ann, "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Announcement id.")},
		body:  actionBody("Full announcement fields.", nil, props("announcement", ref("Announcement"))),
		resps: []respEntry{ok("AnnouncementResponse"), r("400", "Validation failed"), r("404", "Not found")}})
	b.add("DELETE", "/api/announcements/{id}", "Announcements", "Delete an announcement", ann, "", opt{
		path:  []*openapi3.Parameter{pparam("id", "Announcement id.")},
		resps: []respEntry{noContent()}})
	b.add("POST", "/api/announcements/{id}/send", "Announcements", "Send now", ann,
		"Posts the announcement's embed to Discord immediately.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Announcement id.")},
			resps: []respEntry{ok("AnnouncementResponse"), r("400", "No Discord webhook configured"), r("404", "Not found"), r("502", "Discord failed")}})
	b.add("POST", "/api/announcements/{id}/skip", "Announcements", "Skip next occurrence", ann,
		"Skips the next scheduled occurrence of a scheduled announcement.", opt{
			path:  []*openapi3.Parameter{pparam("id", "Announcement id.")},
			resps: []respEntry{ok("AnnouncementResponse"), r("400", "Not scheduled"), r("404", "Not found")}})

	// ── Winners Log ───────────────────────────────────────────────────────────
	wl := "permission:bingo-winners-log"
	b.add("GET", "/api/winners-log", "Winners Log", "List winners", wl, "", opt{
		query: []*openapi3.Parameter{
			qparam("page", "Page (default 1).", false), qparam("per_page", "1–200 (default 25).", false),
			qparam("sort", "logged_at|card_id|player_name|game_details.", false), qparam("dir", "asc|desc.", false)},
		resps: []respEntry{ok("WinnersLogResponse")}})
	b.add("DELETE", "/api/winners-log/all", "Winners Log", "Clear the winners log", wl,
		"Bulk delete: removes every entry and reports how many rows were removed.", opt{
			resps: []respEntry{ok("DeletedCountResponse")}})
	b.add("DELETE", "/api/winners-log/{id}", "Winners Log", "Delete a winner entry", wl,
		"Deleting a non-existent entry is a no-op success (idempotent).", opt{
			path:  []*openapi3.Parameter{pparam("id", "Entry id.")},
			resps: []respEntry{noContent()}})
	b.add("GET", "/api/winners-log/frequent", "Winners Log", "Frequent winners", wl, "", opt{resps: []respEntry{ok("FrequentWinnersResponse")}})

	// ── Settings ──────────────────────────────────────────────────────────────
	b.add("GET", "/api/settings", "Settings", "Get settings", "public",
		"Secret settings (per-club webhook URLs) are blanked for non-admins.", opt{resps: []respEntry{ok("SettingsResponse")}})
	b.add("POST", "/api/settings", "Settings", "Update settings", "permission:system-settings", "", opt{
		body:  actionBody("Setting values.", nil, props("settings", desc(openapi3.NewObjectSchema(), "Map of setting key → value."))),
		resps: []respEntry{ok("OKResponse"), r("400", "Unknown / invalid setting")}})

	buildFilePaths(b)
}

// buildFilePaths registers the fonts / carrd / images endpoints.
func buildFilePaths(b *pb) {
	// Fonts. A logical font is a GROUP of uploaded files sharing a base name
	// (its format variants) plus an auto-converted WOFF2 copy; files are keyed
	// by filename, fonts by base name under /families.
	b.add("GET", "/api/fonts", "Files", "List fonts", "permission:atelier-fonts",
		"Fonts grouped by base name, each with its variants (uploaded files + the auto-converted WOFF2 copy), metadata (CSS family, served type, per-font origin allowlist), and rotating serving tokens.", opt{
			resps: []respEntry{ok("FontsResponse")}})
	b.add("POST", "/api/fonts/upload", "Files", "Upload font files", "permission:atelier-fonts",
		"multipart `files` (repeated, 64 MB total). Same-named files are skipped. Each owning font's WOFF2 conversion is reconciled after the save (a font with no uploaded WOFF2 gets one converted; uploading a real WOFF2 removes a redundant converted copy); conversion failures are reported in `warnings`.", opt{
			body:  multipartBody("Font files.", props("files", pfiles("Font files."))),
			resps: []respEntry{ok("FontUploadResponse"), r("400", "No files")}})
	b.add("DELETE", "/api/fonts/{name}", "Files", "Delete a font file", "permission:atelier-fonts",
		"Deletes ONE variant file (URL-encode the filename). The owning font's conversion and metadata are reconciled (deleting the last file drops the font).", opt{
			path:  []*openapi3.Parameter{pparam("name", "Font file name.")},
			resps: []respEntry{noContent(), r("400", "Invalid name"), r("404", "Not found")}})
	b.add("PATCH", "/api/fonts/{name}", "Files", "Rename a font file", "permission:atelier-fonts",
		"Renames one variant file; fails if the target name already exists. Group bookkeeping (conversion, metadata) follows the rename.", opt{
			path:  []*openapi3.Parameter{pparam("name", "Current font file name.")},
			body:  actionBody("Rename target.", nil, props("new_name", pstr("New file name."))),
			resps: []respEntry{ok("NamedOKResponse"), r("400", "Invalid name"), r("404", "Not found"), r("409", "Target exists")}})
	b.add("PATCH", "/api/fonts/families/{base}", "Files", "Update a font's metadata", "permission:atelier-fonts",
		"Partial update of one font (group): `family` sets the CSS font-family name (\"\" resets to the base name; must not collide with another font's), `serve` picks the served variant type (`TTF`/`OTF`/`WOFF`/`WOFF2`/`EOT`; \"\" = auto, WOFF2 preferred), `origins` replaces THIS font's external-site allowlist (bare origins, e.g. `https://mysite.carrd.co`; the app's own origin is always allowed).", opt{
			path: []*openapi3.Parameter{pparam("base", "Font base name (filename minus extension).")},
			body: actionBody("Font metadata to update.", nil, props(
				"family", pstr("Custom CSS family name (optional; \"\" resets to default)."),
				"serve", pstr("Served variant type (optional; \"\" = auto)."),
				"origins", parr("Allowed site origins (optional).", pstr("")))),
			resps: []respEntry{ok("OKResponse"), r("400", "Invalid field / nothing to update"), r("404", "Font not found"), r("409", "Family name taken")}})
	b.add("DELETE", "/api/fonts/families/{base}", "Files", "Delete a font", "permission:atelier-fonts",
		"Deletes a whole font: every uploaded variant file, the converted WOFF2 copy, and its metadata.", opt{
			path:  []*openapi3.Parameter{pparam("base", "Font base name (filename minus extension).")},
			resps: []respEntry{noContent(), r("404", "Font not found")}})
	b.add("GET", "/api/fonts/pub/kit.css", "Files", "Public font kit stylesheet", "public",
		"Generated `@font-face` stylesheet for external sites (embedded via the fonts vhost as `https://fonts.senpan.cafe/kit.css`). Sources are relative tokenized URLs that rotate on a schedule. Content is filtered per requesting site: a foreign Referer only sees fonts whose allowlist includes its origin; the font files themselves are the real gate.", opt{
			resps: []respEntry{rawResp("The @font-face stylesheet.", "text/css", false)}})
	b.add("GET", "/api/fonts/pub/f/{token}", "Files", "Serve a font file by token", "public",
		"Streams the font behind an opaque rotating token (valid 7–14 days). Same-origin requests are always allowed; cross-origin requests need an `Origin` on THAT FONT's allowlist, echoed in `Access-Control-Allow-Origin` (browsers require CORS for cross-origin fonts, so this is enforced by the browser too). Requests with no usable Origin (e.g. pasting the URL) are refused.", opt{
			path:  []*openapi3.Parameter{pparam("token", "Opaque font token from the kit stylesheet / settings payload.")},
			resps: []respEntry{rawResp("The font bytes.", "application/octet-stream", true), r("403", "Origin not approved"), r("404", "Unknown or expired token")}})
	// Carrd
	b.add("GET", "/api/carrd/projects", "Files", "List Carrd projects", "permission:atelier-carrd", "", opt{resps: []respEntry{ok("CarrdProjectsResponse")}})
	b.add("POST", "/api/carrd/projects", "Files", "Create a Carrd project", "permission:atelier-carrd",
		"Creates a project folder; `folder` is optional (derived from `title` when omitted).", opt{
			body:  actionBody("New project.", nil, props("title", pstr("Project title (required)."), "folder", pstr("URL folder (optional)."))),
			resps: []respEntry{created("CarrdProjectCreateResponse"), r("400", "Invalid"), r("409", "Duplicate")}})
	b.add("PATCH", "/api/carrd/projects/{folder}", "Files", "Rename a Carrd project", "permission:atelier-carrd",
		"Renames the project title and/or its URL folder (`new_folder` \"\"/omitted keeps the current folder).", opt{
			path:  []*openapi3.Parameter{pparam("folder", "Existing project folder.")},
			body:  actionBody("Rename fields.", nil, props("title", pstr("New title (required)."), "new_folder", pstr("New URL folder (optional)."))),
			resps: []respEntry{ok("CarrdProjectCreateResponse"), r("400", "Invalid"), r("404", "Not found"), r("409", "Duplicate")}})
	b.add("DELETE", "/api/carrd/projects/{folder}", "Files", "Delete a Carrd project", "permission:atelier-carrd",
		"Deletes a project folder and all of its contents.", opt{
			path:  []*openapi3.Parameter{pparam("folder", "Project folder.")},
			resps: []respEntry{noContent(), r("400", "Invalid folder")}})
	b.add("GET", "/api/carrd/images", "Files", "List Carrd images", "permission:atelier-carrd", "", opt{
		query: []*openapi3.Parameter{qparam("folder", "Project folder (required).", true), qparam("path", "Relative subpath (\"\" = root).", false)},
		resps: []respEntry{ok("CarrdImagesResponse"), r("400", "Invalid"), r("404", "Not found")}})
	b.add("DELETE", "/api/carrd/images", "Files", "Delete a Carrd image", "permission:atelier-carrd",
		"Deletes an image at a path within a project (identity via query params, since the path may contain slashes).", opt{
			query: []*openapi3.Parameter{qparam("folder", "Project folder (required).", true), qparam("path", "Relative subpath (\"\" = root).", false), qparam("name", "Image file name (required).", true)},
			resps: []respEntry{noContent(), r("400", "Invalid"), r("404", "Not found")}})
	b.add("POST", "/api/carrd/images/dirs", "Files", "Create a Carrd sub-directory", "permission:atelier-carrd",
		"Creates a sub-directory under a project path (`path` \"\" = project root).", opt{
			body:  actionBody("New sub-directory.", nil, props("folder", pstr("Project folder (required)."), "path", pstr("Parent subpath (\"\" = root)."), "name", pstr("New sub-directory name (required)."))),
			resps: []respEntry{created("NamedOKResponse"), r("400", "Invalid"), r("404", "Parent not found"), r("409", "Exists")}})
	b.add("DELETE", "/api/carrd/images/dirs", "Files", "Delete a Carrd sub-directory", "permission:atelier-carrd",
		"Deletes a sub-directory and its contents (identity via query params). The project root cannot be deleted here.", opt{
			query: []*openapi3.Parameter{qparam("folder", "Project folder (required).", true), qparam("path", "Sub-directory subpath (required, non-empty).", true)},
			resps: []respEntry{noContent(), r("400", "Invalid / project root")}})
	b.add("POST", "/api/carrd/upload", "Files", "Upload to a Carrd project", "permission:atelier-carrd",
		"multipart `folder`, `path`, `files` (64 MB total; images + mp3/mp4). Overwrites same-named.", opt{
			body:  multipartBody("Carrd files.", props("folder", pstr(""), "path", pstr(""), "files", pfiles("Files."))),
			resps: []respEntry{ok("CarrdUploadResponse")}})
	// Images
	b.add("GET", "/api/image-categories", "Files", "List image categories", "permission:system-images", "", opt{resps: []respEntry{ok("ImageCategoriesResponse")}})
	b.add("POST", "/api/image-categories", "Files", "Create an image category", "permission:system-images",
		"Creates a custom category; `dir` is optional (derived from `name` when omitted). Permanent dirs are reserved.", opt{
			body:  actionBody("New category.", nil, props("name", pstr("Category name (required)."), "dir", pstr("Directory name (optional)."))),
			resps: []respEntry{created("ImageCategoryActionResponse"), r("400", "Invalid"), r("409", "Duplicate / reserved")}})
	b.add("PATCH", "/api/image-categories/{dir}", "Files", "Rename an image category", "permission:system-images",
		"Renames a custom category's name and/or directory (`new_dir` \"\"/omitted derives from `name`). Permanent categories are protected.", opt{
			path:  []*openapi3.Parameter{pparam("dir", "Existing category directory.")},
			body:  actionBody("Rename fields.", nil, props("name", pstr("New name (required)."), "new_dir", pstr("New directory (optional)."))),
			resps: []respEntry{ok("ImageCategoryActionResponse"), r("400", "Invalid"), r("403", "Permanent category"), r("404", "Not found"), r("409", "Duplicate / reserved")}})
	b.add("DELETE", "/api/image-categories/{dir}", "Files", "Delete an image category", "permission:system-images",
		"Deletes a custom category folder and all its files. Permanent categories are protected.", opt{
			path:  []*openapi3.Parameter{pparam("dir", "Category directory.")},
			resps: []respEntry{noContent(), r("400", "Invalid"), r("403", "Permanent category"), r("404", "Not found")}})
	b.add("GET", "/api/images", "Files", "List images in a category", "auth",
		"Access: admin, `system-images`, or the editor permission that owns the category (e.g. `raffles`→teahouse-raffles).", opt{
			query: []*openapi3.Parameter{qparam("dir", "Category directory (required).", true)},
			resps: []respEntry{ok("ImagesResponse"), r("400", "Unknown category"), r("403", "No access")}})
	b.add("POST", "/api/images/upload", "Files", "Upload images", "permission:system-images",
		"multipart `dir`, `files` (64 MB total; raster + svg). Overwrites same-named.", opt{
			body:  multipartBody("Images.", props("dir", pstr(""), "files", pfiles("Image files."))),
			resps: []respEntry{ok("ImagesUploadResponse")}})
	b.add("DELETE", "/api/images", "Files", "Delete an image", "permission:system-images",
		"Deletes an image within a category (identity via query params).", opt{
			query: []*openapi3.Parameter{qparam("dir", "Category directory (required).", true), qparam("name", "Image file name (required).", true)},
			resps: []respEntry{noContent(), r("400", "Unknown category / invalid name"), r("404", "Not found")}})
}
