package model

// ReadingList is a named, ordered collection of book-club reading items
// (e.g. a "Yaoi Book Club" reading list). ClubSlug groups lists under a
// particular book club so additional clubs can be added later without a
// schema change. Items is populated only on detail fetches.
type ReadingList struct {
	ID        int64             `json:"id"`
	ClubSlug  string            `json:"club_slug"` // book club identifier, e.g. "yaoi"
	Title     string            `json:"title"`
	CreatedAt string            `json:"created_at"`
	Items     []ReadingListItem `json:"items,omitempty"`
}

// ReadingListItem is a single entry in a reading list (a manga/manhwa/danmei
// title). CoverImage is stored as a full URL — either an uploaded image served
// from this site or a cover URL pulled from AniList.
type ReadingListItem struct {
	ID         int64               `json:"id"`
	ListID     int64               `json:"list_id"`
	CoverImage string              `json:"cover_image"` // full URL
	Title      string              `json:"title"`
	Summary    string              `json:"summary"`  // markdown (rendered by Discord on publish)
	Format     string              `json:"format"`   // Manga, Manhwa, Danmei, etc.
	Genres     string              `json:"genres"`   // comma-separated list
	Tropes     string              `json:"tropes"`   // comma-separated list
	Chapters   string              `json:"chapters"` // free text (e.g. "156", "156 (ongoing)")
	Comments   string              `json:"comments"` // Yao's Comments (markdown)
	Sources    []ReadingListSource `json:"sources"`  // external links (stored as JSON)
	SortOrder  int                 `json:"sort_order"`
}

// ReadingListSource is a named external link attached to a reading list item.
type ReadingListSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// ReadingListsResponse is the body of GET /api/book-clubs/{club}/reading-lists —
// all reading lists for a club, without their items. JSON: {"reading_lists": [...]}.
type ReadingListsResponse struct {
	ReadingLists []ReadingList `json:"reading_lists"`
}

// ReadingListDetailResponse is the body of GET /api/book-clubs/{club}/reading-lists/{id}
// and of POST /api/book-clubs/{club}/reading-lists (create) — a single reading
// list (with its items). The handler passes the ReadingList straight through, so
// the pointer is preserved (a nil would serialize to null, matching the old map
// literal). JSON: {"reading_list": ReadingList}.
type ReadingListDetailResponse struct {
	ReadingList ReadingList `json:"reading_list"`
}

// ReadingListItemResponse is the body of POST /api/book-clubs/{club}/reading-lists/{id}/items
// (create) and PUT …/items/{itemId} (replace) — the created/updated item. JSON: {"item": ...}.
type ReadingListItemResponse struct {
	Item ReadingListItem `json:"item"`
}

// BookclubLookupResponse is the body of GET /api/bookclub/lookup — AniList
// suggestions shaped like reading-list items. JSON: {"results": [...]}.
type BookclubLookupResponse struct {
	Results []ReadingListItem `json:"results"`
}

// BookclubUploadResponse is the shared single-image upload response (book-club
// cover uploads via saveSingleImageUpload) — the full URL of the stored image.
// JSON: {"url": <fullURL>}.
type BookclubUploadResponse struct {
	URL string `json:"url"`
}

// PublishResponse is the body of POST /api/book-clubs/{club}/reading-lists/{id}/publish
// — the number of items posted to Discord. JSON: {"published": <int>}.
type PublishResponse struct {
	Published int `json:"published"`
}
