package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/anilist"
	"app-suite/internal/bookclubs"
	"app-suite/internal/db"
	"app-suite/internal/discord"
	"app-suite/internal/media"
	"app-suite/internal/permissions"
	"app-suite/internal/settings"
	"app-suite/internal/uploads"
)

// Book clubs: each club keeps named reading lists of titles, published to the club's
// own Discord channel one embed per title. A title can be looked up on AniList to
// start its entry. A club's pages are guarded by its own permission, and a list or
// item is only ever reached through the club that owns it.

var bookClubTags = []string{"Book clubs"}

const (
	noSuchClub        = "There is no such book club."
	noSuchReadingList = "There is no such reading list."
	noSuchItem        = "There is no such reading-list item."
)

// maxCoverBytes bounds a cover upload.
const maxCoverBytes = 5 << 20

// publishPause is the wait between the posts of a published list, to stay under
// Discord's rate limit.
const publishPause = 350 * time.Millisecond

// ReadingListSource is a named link to where a title can be read.
type ReadingListSource struct {
	Title string `json:"title" maxLength:"200" doc:"What the link is called; empty to show the address."`
	URL   string `json:"url" maxLength:"2000" doc:"Where it goes. A source with no address is dropped."`
}

// ReadingListSummary is a reading list without its items.
type ReadingListSummary struct {
	ID        int64   `json:"id" doc:"The list's id."`
	Club      string  `json:"club" doc:"The slug of the book club it belongs to."`
	Title     string  `json:"title" doc:"The list's name."`
	CreatedAt *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// ReadingList is a reading list with its items in order.
type ReadingList struct {
	ID        int64             `json:"id" doc:"The list's id."`
	Club      string            `json:"club" doc:"The slug of the book club it belongs to."`
	Title     string            `json:"title" doc:"The list's name."`
	CreatedAt *string           `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
	Items     []ReadingListItem `json:"items" nullable:"false" doc:"Its titles, in the order they are published."`
}

// ReadingListTitle names a reading list.
type ReadingListTitle struct {
	Title string `json:"title" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The list's name."`
}

// ReadingListSummaryList is a club's reading lists.
type ReadingListSummaryList struct {
	ReadingLists []ReadingListSummary `json:"readingLists" nullable:"false" doc:"The club's lists, newest first, without their items."`
}

// ReadingListItem is one title on a reading list.
type ReadingListItem struct {
	ID         int64               `json:"id" doc:"The item's id."`
	ListID     int64               `json:"listId" doc:"The list it is on."`
	CoverImage string              `json:"coverImage" doc:"The cover: a full address, either an upload on this site or one from AniList."`
	Title      string              `json:"title" doc:"The title."`
	Summary    string              `json:"summary" doc:"What it is about, in Markdown, which Discord renders when it is published."`
	Format     string              `json:"format" doc:"Manga, Manhwa, Manhua, Danmei, Novel and so on."`
	Genres     string              `json:"genres" doc:"The genres, comma-separated."`
	Tropes     string              `json:"tropes" doc:"The tropes, comma-separated."`
	Chapters   string              `json:"chapters" doc:"How long it is, as free text such as 156 or 156 (ongoing)."`
	Comments   string              `json:"comments" doc:"The curator's comments, in Markdown."`
	Sources    []ReadingListSource `json:"sources" nullable:"false" doc:"Where to read it."`
	SortOrder  int64               `json:"sortOrder" doc:"Its place in the list, lowest first."`
}

// ReadingListItemInput is an item as a save sends it: all of it, since a save
// replaces it. Its place in the list is kept.
type ReadingListItemInput struct {
	CoverImage string              `json:"coverImage" maxLength:"2000" doc:"The cover: a full address; empty for none."`
	Title      string              `json:"title" minLength:"1" maxLength:"300" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The title."`
	Summary    string              `json:"summary" maxLength:"20000" doc:"What it is about, in Markdown."`
	Format     string              `json:"format" maxLength:"100" doc:"Manga, Manhwa, Manhua, Danmei, Novel and so on."`
	Genres     string              `json:"genres" maxLength:"1000" doc:"The genres, comma-separated."`
	Tropes     string              `json:"tropes" maxLength:"1000" doc:"The tropes, comma-separated."`
	Chapters   string              `json:"chapters" maxLength:"100" doc:"How long it is, as free text."`
	Comments   string              `json:"comments" maxLength:"20000" doc:"The curator's comments, in Markdown."`
	Sources    []ReadingListSource `json:"sources" maxItems:"50" nullable:"false" doc:"Where to read it. Rows with no address are dropped."`
}

// BookClubCoverForm is a cover picture.
type BookClubCoverForm struct {
	Image huma.FormFile `form:"image" contentType:"image/*" required:"false" doc:"A JPEG, PNG, GIF or WebP of at most 5 MB, judged from its bytes. One whose name is already used replaces it. A JPEG or PNG is saved again without its metadata."`
}

// BookClubCover is an uploaded cover.
type BookClubCover struct {
	URL string `json:"url" doc:"Its absolute address, to store as an item's cover."`
}

// PublishResult says how many titles were posted.
type PublishResult struct {
	Published int64 `json:"published" doc:"How many items were posted, one embed each."`
}

// TitleLookup is what AniList found.
type TitleLookup struct {
	Results []ReadingListItemInput `json:"results" nullable:"false" doc:"The matches as items to start from, best first; none when AniList doesn't know the title."`
}

type clubInput struct {
	Club string `path:"club" doc:"The book club's slug."`
}

type readingListsCreateInput struct {
	Club string `path:"club" doc:"The book club's slug."`
	Body ReadingListTitle
}

type readingListInput struct {
	Club string `path:"club" doc:"The book club's slug."`
	ID   int64  `path:"id" doc:"The reading list's id."`
}

type readingListSaveInput struct {
	Club string `path:"club" doc:"The book club's slug."`
	ID   int64  `path:"id" doc:"The reading list's id."`
	Body ReadingListTitle
}

type readingListItemCreateInput struct {
	Club string `path:"club" doc:"The book club's slug."`
	ID   int64  `path:"id" doc:"The reading list's id."`
	Body ReadingListItemInput
}

type readingListItemInputPath struct {
	Club   string `path:"club" doc:"The book club's slug."`
	ID     int64  `path:"id" doc:"The reading list's id."`
	ItemID int64  `path:"itemId" doc:"The item's id."`
}

type readingListItemSaveInput struct {
	Club   string `path:"club" doc:"The book club's slug."`
	ID     int64  `path:"id" doc:"The reading list's id."`
	ItemID int64  `path:"itemId" doc:"The item's id."`
	Body   ReadingListItemInput
}

type bookClubCoverInput struct {
	Club    string `path:"club" doc:"The book club's slug."`
	RawBody huma.MultipartFormFiles[BookClubCoverForm]
}

type bookClubCoverOutput struct {
	Status int
	Body   BookClubCover
}

type bookClubWebhookInput struct {
	Club string `path:"club" doc:"The book club's slug."`
	Body DiscordWebhook
}

type titleLookupInput struct {
	Club string `path:"club" doc:"The book club's slug."`
	Q    string `query:"q" required:"false" maxLength:"200" doc:"A title to search for. Give this or id."`
	ID   int64  `query:"id" required:"false" minimum:"1" doc:"An AniList id to fetch. Give this or q."`
}

type readingListSummaryListOutput struct{ Body ReadingListSummaryList }

type readingListOutput struct{ Body ReadingList }

type readingListCreatedOutput struct {
	Status int
	Body   ReadingList
}

type readingListItemOutput struct{ Body ReadingListItem }

type readingListItemCreatedOutput struct {
	Status int
	Body   ReadingListItem
}

type publishResultOutput struct{ Body PublishResult }

type bookClubWebhookOutput struct{ Body DiscordWebhook }

type titleLookupOutput struct{ Body TitleLookup }

func (h handlers) registerBookClubs(api huma.API) {
	club := huma.Middlewares{h.clubPage(api)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound}
	invalid := slices.Concat(guarded, []int{http.StatusUnprocessableEntity})
	const base = "/book-clubs/{club}"

	huma.Register(api, huma.Operation{
		OperationID: "list-reading-lists", Method: http.MethodGet, Path: base + "/reading-lists",
		Summary: "A book club's reading lists", Tags: bookClubTags, Security: secured, Middlewares: club, Errors: guarded,
	}, h.listReadingLists)
	huma.Register(api, huma.Operation{
		OperationID: "create-reading-list", Method: http.MethodPost, Path: base + "/reading-lists",
		Summary: "Make a reading list", Tags: bookClubTags, DefaultStatus: http.StatusCreated,
		Security: secured, Middlewares: club, Errors: invalid,
	}, h.createReadingList)
	huma.Register(api, huma.Operation{
		OperationID: "get-reading-list", Method: http.MethodGet, Path: base + "/reading-lists/{id}",
		Summary: "A reading list with its items", Description: "A list of another club is a 404.",
		Tags: bookClubTags, Security: secured, Middlewares: club, Errors: guarded,
	}, h.getReadingList)
	huma.Register(api, huma.Operation{
		OperationID: "update-reading-list", Method: http.MethodPut, Path: base + "/reading-lists/{id}",
		Summary: "Rename a reading list", Tags: bookClubTags, Security: secured, Middlewares: club, Errors: invalid,
	}, h.updateReadingList)
	huma.Register(api, huma.Operation{
		OperationID: "delete-reading-list", Method: http.MethodDelete, Path: base + "/reading-lists/{id}",
		Summary:     "Delete a reading list",
		Description: "Its items go with it, and the uploaded covers no other item uses are deleted.",
		Tags:        bookClubTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: club, Errors: guarded,
	}, h.deleteReadingList)
	huma.Register(api, huma.Operation{
		OperationID: "create-reading-list-item", Method: http.MethodPost, Path: base + "/reading-lists/{id}/items",
		Summary: "Add an item to a reading list", Description: "It goes to the end of the list.",
		Tags: bookClubTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: club, Errors: invalid,
	}, h.createReadingListItem)
	huma.Register(api, huma.Operation{
		OperationID: "update-reading-list-item", Method: http.MethodPut, Path: base + "/reading-lists/{id}/items/{itemId}",
		Summary: "Replace an item", Description: "An item of another list is a 404.",
		Tags: bookClubTags, Security: secured, Middlewares: club, Errors: invalid,
	}, h.updateReadingListItem)
	huma.Register(api, huma.Operation{
		OperationID: "delete-reading-list-item", Method: http.MethodDelete, Path: base + "/reading-lists/{id}/items/{itemId}",
		Summary: "Remove an item", Description: "Its uploaded cover is deleted if no other item uses it.",
		Tags: bookClubTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: club, Errors: guarded,
	}, h.deleteReadingListItem)
	huma.Register(api, huma.Operation{
		OperationID: "publish-reading-list", Method: http.MethodPost, Path: base + "/reading-lists/{id}/publish",
		Summary: "Post a reading list to Discord",
		Description: "Posts every item as its own embed to the club's webhook, in order. A list with no items is a 409. " +
			"A failure part-way says how many were posted before it.",
		Tags: bookClubTags, Security: secured, Middlewares: club,
		Errors: append(append(guarded, http.StatusRequestTimeout, http.StatusConflict), discordErrors...),
	}, h.publishReadingList)
	huma.Register(api, huma.Operation{
		OperationID: "upload-book-club-cover", Method: http.MethodPost, Path: base + "/covers",
		Summary: "Upload a cover picture", Description: "Stored under its uploaded name. Nothing on a reading list changes until an item is saved with the address.",
		Tags: bookClubTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: club, MaxBodyBytes: maxCoverBytes + multipartOverhead,
		Errors: slices.Concat(guarded, []int{http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity}),
	}, h.uploadBookClubCover)
	huma.Register(api, huma.Operation{
		OperationID: "get-book-club-webhook", Method: http.MethodGet, Path: base + "/webhook",
		Summary: "The Discord webhook a club's lists are published to", Tags: bookClubTags,
		Security: secured, Middlewares: club, Errors: guarded,
	}, h.getBookClubWebhook)
	huma.Register(api, huma.Operation{
		OperationID: "set-book-club-webhook", Method: http.MethodPut, Path: base + "/webhook",
		Summary: "Set the Discord webhook a club's lists are published to", Description: "An empty webhook clears it.",
		Tags: bookClubTags, Security: secured, Middlewares: club, Errors: invalid,
	}, h.setBookClubWebhook)
	huma.Register(api, huma.Operation{
		OperationID: "look-up-title", Method: http.MethodGet, Path: base + "/lookup",
		Summary: "Look a title up on AniList",
		Description: "Searches by q, or fetches one title by its AniList id, and answers items to start from. " +
			"Exactly one of q and id is given. AniList being unreachable is a 503, rate limiting a 429, and " +
			"a refusal (the API disabled, an address it has blocked) a 500, each saying why.",
		Tags: bookClubTags, Security: secured, Middlewares: club,
		Errors: append(invalid, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable),
	}, h.lookUpTitle)
}

// clubPage lets through an account holding the page of the club in the path: 401 when
// nobody is signed in, 404 for a club that isn't registered, 403 without its page.
func (h handlers) clubPage(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		club := ctx.Param("club")
		u := h.who(ctx)
		switch {
		case u == nil:
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Sign in to continue.")
		case !bookclubs.Exists(club):
			_ = huma.WriteErr(api, ctx, http.StatusNotFound, noSuchClub)
		case !Permitted(u, permissions.BookClub(club)):
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "Your account doesn't have access to this.")
		default:
			next(ctx)
		}
	}
}

func toReadingListSummary(l *db.ReadingList) ReadingListSummary {
	return ReadingListSummary{ID: l.ID, Club: l.ClubSlug, Title: l.Title, CreatedAt: l.CreatedAt}
}

func toReadingListItem(it *db.ReadingListItem) ReadingListItem {
	return ReadingListItem{
		ID: it.ID, ListID: it.ListID, CoverImage: it.CoverImage, Title: it.Title, Summary: it.Summary,
		Format: it.Format, Genres: it.Genres, Tropes: it.Tropes, Chapters: it.Chapters, Comments: it.Comments,
		Sources: decodeList[ReadingListSource](it.Sources), SortOrder: it.SortOrder,
	}
}

func (it ReadingListItem) item() bookclubs.Item {
	sources := make([]bookclubs.Source, 0, len(it.Sources))
	for _, s := range it.Sources {
		sources = append(sources, bookclubs.Source{Title: s.Title, URL: s.URL})
	}
	return bookclubs.Item{
		CoverImage: it.CoverImage, Title: it.Title, Summary: it.Summary, Format: it.Format, Genres: it.Genres,
		Tropes: it.Tropes, Chapters: it.Chapters, Comments: it.Comments, Sources: sources,
	}
}

// sanitized is the item as it is stored: the title trimmed and the rows with no
// address dropped from its sources.
func (in ReadingListItemInput) sanitized() ReadingListItemInput {
	sources := make([]bookclubs.Source, 0, len(in.Sources))
	for _, s := range in.Sources {
		sources = append(sources, bookclubs.Source{Title: s.Title, URL: s.URL})
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Sources = make([]ReadingListSource, 0, len(sources))
	for _, s := range bookclubs.SanitizeSources(sources) {
		in.Sources = append(in.Sources, ReadingListSource{Title: s.Title, URL: s.URL})
	}
	return in
}

// readingList reads a list of the club with its items, ErrNoRows when it isn't there.
func readingList(ctx context.Context, q *db.Queries, club string, id int64) (ReadingList, error) {
	l, err := q.GetReadingList(ctx, db.GetReadingListParams{ID: id, ClubSlug: club})
	if err != nil {
		return ReadingList{}, err
	}
	return withItems(ctx, q, l)
}

func withItems(ctx context.Context, q *db.Queries, l *db.ReadingList) (ReadingList, error) {
	rows, err := q.ListReadingListItems(ctx, l.ID)
	if err != nil {
		return ReadingList{}, err
	}
	out := ReadingList{ID: l.ID, Club: l.ClubSlug, Title: l.Title, CreatedAt: l.CreatedAt, Items: make([]ReadingListItem, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, toReadingListItem(r))
	}
	return out, nil
}

func (h handlers) listReadingLists(ctx context.Context, in *clubInput) (*readingListSummaryListOutput, error) {
	rows, err := h.read().ListReadingLists(ctx, in.Club)
	if err != nil {
		return nil, h.fail("list reading lists", err)
	}
	out := ReadingListSummaryList{ReadingLists: make([]ReadingListSummary, 0, len(rows))}
	for _, r := range rows {
		out.ReadingLists = append(out.ReadingLists, toReadingListSummary(r))
	}
	return &readingListSummaryListOutput{Body: out}, nil
}

func (h handlers) createReadingList(ctx context.Context, in *readingListsCreateInput) (*readingListCreatedOutput, error) {
	made, err := h.write().CreateReadingList(ctx, db.CreateReadingListParams{ClubSlug: in.Club, Title: strings.TrimSpace(in.Body.Title)})
	if err != nil {
		return nil, h.fail("create a reading list", err)
	}
	return &readingListCreatedOutput{Status: http.StatusCreated, Body: ReadingList{
		ID: made.ID, Club: made.ClubSlug, Title: made.Title, CreatedAt: made.CreatedAt, Items: []ReadingListItem{},
	}}, nil
}

func (h handlers) getReadingList(ctx context.Context, in *readingListInput) (*readingListOutput, error) {
	list, err := readingList(ctx, h.read(), in.Club, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchReadingList)
	}
	if err != nil {
		return nil, h.fail("read a reading list", err)
	}
	return &readingListOutput{Body: list}, nil
}

func (h handlers) updateReadingList(ctx context.Context, in *readingListSaveInput) (*readingListOutput, error) {
	ctx = context.WithoutCancel(ctx)
	var list ReadingList
	err := h.transact(ctx, func(q *db.Queries) error {
		l, err := q.UpdateReadingListTitle(ctx, db.UpdateReadingListTitleParams{
			Title: strings.TrimSpace(in.Body.Title), ID: in.ID, ClubSlug: in.Club,
		})
		if err != nil {
			return err
		}
		list, err = withItems(ctx, q, l)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchReadingList)
	}
	if err != nil {
		return nil, h.fail("rename a reading list", err)
	}
	return &readingListOutput{Body: list}, nil
}

func (h handlers) deleteReadingList(ctx context.Context, in *readingListInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	// The covers are read before the rows go (the items cascade), and the files are
	// cleaned up after, once the reference count reflects what is left.
	var covers []string
	err := h.transact(ctx, func(q *db.Queries) error {
		if _, err := q.GetReadingList(ctx, db.GetReadingListParams{ID: in.ID, ClubSlug: in.Club}); err != nil {
			return err
		}
		items, err := q.ListReadingListItems(ctx, in.ID)
		if err != nil {
			return err
		}
		for _, it := range items {
			covers = append(covers, it.CoverImage)
		}
		_, err = q.DeleteReadingList(ctx, db.DeleteReadingListParams{ID: in.ID, ClubSlug: in.Club})
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchReadingList)
	}
	if err != nil {
		return nil, h.fail("delete a reading list", err)
	}
	h.removeUnusedCovers(ctx, covers...)
	return nil, nil
}

// removeUnusedCovers deletes each uploaded cover file that no reading-list item uses.
// Covers keep their uploaded name, so two items can share one file; this is called
// after the rows that held them are gone, so the count reflects only the items that
// remain. A cover from AniList is not an upload and is left alone.
func (h handlers) removeUnusedCovers(ctx context.Context, urls ...string) {
	if h.Images == nil {
		return
	}
	q := h.read()
	for _, cover := range urls {
		if cover == "" {
			continue
		}
		if n, err := q.CountReadingListItemsByCover(ctx, cover); err != nil || n > 0 {
			continue
		}
		h.Images.RemoveCover(cover)
	}
}

// ownList is nil when the list is in the club, else the 404 to answer.
func (h handlers) ownList(ctx context.Context, club string, id int64) error {
	_, err := h.read().GetReadingList(ctx, db.GetReadingListParams{ID: id, ClubSlug: club})
	if errors.Is(err, sql.ErrNoRows) {
		return huma.Error404NotFound(noSuchReadingList)
	}
	if err != nil {
		return h.fail("read a reading list", err)
	}
	return nil
}

func (h handlers) createReadingListItem(ctx context.Context, in *readingListItemCreateInput) (*readingListItemCreatedOutput, error) {
	if err := h.ownList(ctx, in.Club, in.ID); err != nil {
		return nil, err
	}
	body := in.Body.sanitized()
	sources, err := json.Marshal(body.Sources)
	if err != nil {
		return nil, h.fail("encode item sources", err)
	}
	made, err := h.write().CreateReadingListItem(context.WithoutCancel(ctx), db.CreateReadingListItemParams{
		ListID: in.ID, CoverImage: body.CoverImage, Title: body.Title, Summary: body.Summary, Format: body.Format,
		Genres: body.Genres, Tropes: body.Tropes, Chapters: body.Chapters, Comments: body.Comments, Sources: string(sources),
	})
	if err != nil {
		return nil, h.fail("create a reading-list item", err)
	}
	return &readingListItemCreatedOutput{Status: http.StatusCreated, Body: toReadingListItem(made)}, nil
}

func (h handlers) updateReadingListItem(ctx context.Context, in *readingListItemSaveInput) (*readingListItemOutput, error) {
	if err := h.ownList(ctx, in.Club, in.ID); err != nil {
		return nil, err
	}
	body := in.Body.sanitized()
	sources, err := json.Marshal(body.Sources)
	if err != nil {
		return nil, h.fail("encode item sources", err)
	}
	// Scoped to the list, so an item id of another list (and so another club) finds nothing.
	saved, err := h.write().UpdateReadingListItem(context.WithoutCancel(ctx), db.UpdateReadingListItemParams{
		CoverImage: body.CoverImage, Title: body.Title, Summary: body.Summary, Format: body.Format, Genres: body.Genres,
		Tropes: body.Tropes, Chapters: body.Chapters, Comments: body.Comments, Sources: string(sources),
		ID: in.ItemID, ListID: in.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchItem)
	}
	if err != nil {
		return nil, h.fail("update a reading-list item", err)
	}
	return &readingListItemOutput{Body: toReadingListItem(saved)}, nil
}

func (h handlers) deleteReadingListItem(ctx context.Context, in *readingListItemInputPath) (*struct{}, error) {
	if err := h.ownList(ctx, in.Club, in.ID); err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	var cover string
	var deleted int64
	err := h.transact(ctx, func(q *db.Queries) error {
		it, err := q.GetReadingListItem(ctx, db.GetReadingListItemParams{ID: in.ItemID, ListID: in.ID})
		if err != nil {
			return err
		}
		cover = it.CoverImage
		deleted, err = q.DeleteReadingListItem(ctx, db.DeleteReadingListItemParams{ID: in.ItemID, ListID: in.ID})
		return err
	})
	if errors.Is(err, sql.ErrNoRows) || (err == nil && deleted == 0) {
		return nil, huma.Error404NotFound(noSuchItem)
	}
	if err != nil {
		return nil, h.fail("delete a reading-list item", err)
	}
	h.removeUnusedCovers(ctx, cover)
	return nil, nil
}

func (h handlers) bookClubWebhook(ctx context.Context, club string) (string, error) {
	url, err := h.read().GetSetting(ctx, bookclubs.WebhookSettingKey(club))
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return strings.TrimSpace(url), err
}

func (h handlers) getBookClubWebhook(ctx context.Context, in *clubInput) (*bookClubWebhookOutput, error) {
	url, err := h.bookClubWebhook(ctx, in.Club)
	if err != nil {
		return nil, h.fail("read a book club webhook", err)
	}
	return &bookClubWebhookOutput{Body: DiscordWebhook{WebhookURL: url}}, nil
}

func (h handlers) setBookClubWebhook(ctx context.Context, in *bookClubWebhookInput) (*bookClubWebhookOutput, error) {
	body := DiscordWebhook{WebhookURL: strings.TrimSpace(in.Body.WebhookURL)}
	if err := body.check(); err != nil {
		return nil, err
	}
	err := h.write().SetSetting(context.WithoutCancel(ctx), db.SetSettingParams{Key: bookclubs.WebhookSettingKey(in.Club), Value: body.WebhookURL})
	if err != nil {
		return nil, h.fail("save a book club webhook", err)
	}
	return &bookClubWebhookOutput{Body: body}, nil
}

func (h handlers) publishReadingList(ctx context.Context, in *readingListInput) (*publishResultOutput, error) {
	list, err := readingList(ctx, h.read(), in.Club, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchReadingList)
	}
	if err != nil {
		return nil, h.fail("read a reading list", err)
	}
	if len(list.Items) == 0 {
		return nil, huma.Error409Conflict("This reading list has no items to publish.")
	}
	webhook, err := h.bookClubWebhook(ctx, in.Club)
	if err != nil {
		return nil, h.fail("read a book club webhook", err)
	}
	if webhook == "" {
		return nil, noWebhook("this book club")
	}

	// A slow, sequential job: the request's context stops it early if the client goes,
	// rather than posting the rest of the list to Discord.
	total := len(list.Items)
	published := 0
	for i, it := range list.Items {
		if err := ctx.Err(); err != nil {
			return nil, huma.Error408RequestTimeout("Publishing was cancelled after " + strconv.Itoa(published) + " of " + strconv.Itoa(total) + " items.")
		}
		target := discord.Target{Kind: "bookclub_item", Name: it.Title}
		if err := h.Discord.PostEmbed(ctx, target, webhook, bookclubs.ItemEmbed(it.item(), in.Club)); err != nil {
			return nil, h.discordProblemAfter(err, "publish reading list "+strconv.FormatInt(in.ID, 10)+" item "+strconv.Quote(it.Title),
				"Posted "+strconv.Itoa(published)+" of "+strconv.Itoa(total)+", then: ")
		}
		published++
		if i < total-1 {
			select {
			case <-ctx.Done():
				return nil, huma.Error408RequestTimeout("Publishing was cancelled after " + strconv.Itoa(published) + " of " + strconv.Itoa(total) + " items.")
			case <-time.After(publishPause):
			}
		}
	}
	slog.Info("published reading list", "list_id", list.ID, "list", list.Title, "club", in.Club, "items", published, "by", by(ctx))
	return &publishResultOutput{Body: PublishResult{Published: int64(published)}}, nil
}

func (h handlers) lookUpTitle(ctx context.Context, in *titleLookupInput) (*titleLookupOutput, error) {
	q := strings.TrimSpace(in.Q)
	if (q == "") == (in.ID == 0) {
		return nil, huma.Error422UnprocessableEntity("Give a title to search for (q) or an AniList id (id), not both and not neither.",
			&huma.ErrorDetail{Location: "query.q", Value: in.Q, Message: "Give q or id, one of them."})
	}
	configured, err := h.read().GetSetting(ctx, settings.AniListAPIURL)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, h.fail("read the AniList endpoint", err)
	}
	endpoint := anilist.Endpoint(configured)

	var found []anilist.Suggestion
	if in.ID != 0 {
		found, err = h.AniList.ByID(ctx, endpoint, int(in.ID))
	} else {
		found, err = h.AniList.Search(ctx, endpoint, q)
	}
	if err != nil {
		return nil, h.aniListProblem(err)
	}
	out := TitleLookup{Results: make([]ReadingListItemInput, 0, len(found))}
	for _, s := range found {
		sources := make([]ReadingListSource, 0, len(s.Sources))
		for _, src := range s.Sources {
			sources = append(sources, ReadingListSource{Title: src.Title, URL: src.URL})
		}
		out.Results = append(out.Results, ReadingListItemInput{
			CoverImage: s.CoverImage, Title: s.Title, Summary: s.Summary, Format: s.Format, Genres: s.Genres,
			Chapters: s.Chapters, Sources: sources,
		})
	}
	return &titleLookupOutput{Body: out}, nil
}

// aniListProblem logs a failed lookup once and answers it, saying why in words staff
// can act on.
func (h handlers) aniListProblem(err error) error {
	status, wait := anilist.HTTPStatus(err)
	var ae *anilist.Error
	errors.As(err, &ae)

	level := h.Logger.Error
	if ae != nil && ae.Kind != anilist.Refused {
		level = h.Logger.Warn
	}
	level("anilist lookup failed", "status", status, "error", err)

	switch {
	case ae == nil:
		return huma.Error500InternalServerError("The AniList lookup failed.")
	case ae.Kind == anilist.Unreachable:
		return huma.Error503ServiceUnavailable("AniList can't be reached right now: " + ae.Message + ". Try again shortly.")
	case ae.Kind == anilist.RateLimited:
		seconds := max(int((wait+time.Second-1)/time.Second), 1)
		return huma.ErrorWithHeaders(
			huma.Error429TooManyRequests("AniList is limiting lookups right now. Try again in "+strconv.Itoa(seconds)+" seconds."),
			http.Header{"Retry-After": {strconv.Itoa(seconds)}})
	default:
		return huma.Error500InternalServerError("AniList refused the lookup: " + ae.Message + ".")
	}
}

func (h handlers) uploadBookClubCover(ctx context.Context, in *bookClubCoverInput) (*bookClubCoverOutput, error) {
	f := in.RawBody.Data().Image
	if !f.IsSet {
		return nil, fieldProblem("body.image", nil, "Choose a picture to upload.")
	}
	defer func() { _ = f.Close() }() // read in full below; nothing to report on close

	name, ok := uploads.SafeName(f.Filename, uploads.IsRasterExt)
	if !ok {
		return nil, fieldProblem("body.image", f.Filename, "Only jpg, png, webp, and gif images are allowed")
	}
	data, err := readUpload(f)
	if err != nil {
		return nil, h.fail("read a cover", err)
	}
	stored, reason := h.checkPicture(data)
	if reason != "" {
		return nil, fieldProblem("body.image", f.Filename, "That file does not appear to be a valid image")
	}
	if err := h.Images.SaveCover(name, stored); err != nil {
		return nil, h.fail("save a cover", err)
	}
	return &bookClubCoverOutput{Status: http.StatusCreated, Body: BookClubCover{
		URL: imageURL(requestOf(ctx).Site, media.CoverDir, name),
	}}, nil
}
