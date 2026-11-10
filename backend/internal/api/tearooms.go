package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/discord"
	"app-suite/internal/permissions"
	"app-suite/internal/tearooms"
)

// Tea rooms: the Tea House's bookable rooms. Staff keep a drag-ordered list and post
// each as a Discord embed to one shared webhook; the list is also read by sites
// outside the app (through the public operations), which render live availability.

var teaRoomTags = []string{"Tea rooms"}

var errNoTeaRoom = errors.New("no such tea room")

// teaRoomWebhookKey is the settings row holding the webhook tea rooms post to. It is
// outside the public settings, so it is only ever read through the page's operations.
const teaRoomWebhookKey = "tearoom_webhook_url"

const noSuchTeaRoom = "There is no such tea room."

// TeaRoom is a bookable room.
type TeaRoom struct {
	ID              int64   `json:"id" doc:"The room's id."`
	Name            string  `json:"name" doc:"The room's name, the title of its embed."`
	Subtitle        string  `json:"subtitle" doc:"A short line under the name, such as a phrase in another script."`
	RoomOwner       string  `json:"roomOwner" doc:"The character who owns the room; informational."`
	RoomNumber      string  `json:"roomNumber" doc:"The room's public number: unique, and the key the public API finds it by."`
	CostPerHalfHour int64   `json:"costPerHalfHour" doc:"The price in gil per half hour. A discounted room charges half."`
	Hashtags        string  `json:"hashtags" doc:"The room's hashtags, space-separated and each starting with #."`
	Description     string  `json:"description" doc:"The room's description (Markdown), the body of its embed."`
	Seasonal        bool    `json:"seasonal" doc:"Whether the room is seasonal rather than permanent."`
	Open            bool    `json:"open" doc:"Whether the room is open."`
	Lockable        bool    `json:"lockable" doc:"Whether the room can be locked, booked out for an extra fee."`
	Discounted      bool    `json:"discounted" doc:"Whether the room is on its half-price offer."`
	Locked          bool    `json:"locked" doc:"Whether the room is locked right now."`
	LockedUntil     *string `json:"lockedUntil" nullable:"true" doc:"When the lock lifts on its own, RFC 3339 in UTC; null when it stands until staff unlock the room. Only set while the room is locked."`
	Image           string  `json:"image" doc:"The room's image, from the image library; shown full width in its embed."`
	Color           string  `json:"color" doc:"The accent color of its embed, #rrggbb; empty for the default."`
	SortOrder       int64   `json:"sortOrder" doc:"Where it sits in the staff's order."`
	CreatedAt       *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// TeaRoomInput is a room as a save sends it: all of it, since a save replaces it.
type TeaRoomInput struct {
	Name            string  `json:"name" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The room's name."`
	Subtitle        string  `json:"subtitle" maxLength:"200" doc:"A short line under the name; empty for none."`
	RoomOwner       string  `json:"roomOwner" maxLength:"200" doc:"The character who owns the room; empty for none."`
	RoomNumber      string  `json:"roomNumber" minLength:"1" maxLength:"100" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The room's public number. No other room may have it."`
	CostPerHalfHour int64   `json:"costPerHalfHour" minimum:"0" maximum:"1000000000000" doc:"The price in gil per half hour."`
	Hashtags        string  `json:"hashtags" maxLength:"2000" doc:"Hashtags, separated by spaces or commas; they are stored as a list of #tags, repeats dropped, at most 30."`
	Description     string  `json:"description" maxLength:"20000" doc:"The room's description (Markdown); empty for none."`
	Seasonal        bool    `json:"seasonal" doc:"Whether the room is seasonal rather than permanent."`
	Open            bool    `json:"open" doc:"Whether the room is open."`
	Lockable        bool    `json:"lockable" doc:"Whether the room can be locked."`
	Discounted      bool    `json:"discounted" doc:"Whether the room is on its half-price offer."`
	Locked          bool    `json:"locked" doc:"Whether the room is locked."`
	LockedUntil     *string `json:"lockedUntil" nullable:"true" format:"date-time" doc:"When the lock lifts on its own; null for a lock that stands until staff unlock the room. Ignored unless the room is locked."`
	Image           string  `json:"image" maxLength:"500" doc:"The room's image from the image library; empty for none."`
	Color           string  `json:"color" maxLength:"20" doc:"The accent color of its embed, #rrggbb; empty for the default."`
}

// TeaRoomChanges is the quick changes staff make from the list. Only what is sent
// changes. Locking and the unlock time are separate: locked alone locks or unlocks, a
// lockedUntil alone re-times a lock already in place, and both do both. A lock with
// no unlock time stands until staff unlock the room.
type TeaRoomChanges struct {
	Open        *bool   `json:"open,omitempty" required:"false" doc:"Open or close the room."`
	Discounted  *bool   `json:"discounted,omitempty" required:"false" doc:"Put the room on its half-price offer, or take it off."`
	Locked      *bool   `json:"locked,omitempty" required:"false" doc:"Lock or unlock the room. Locking sets the lock afresh: it lasts until lockedUntil when that is sent beside it, otherwise until staff unlock it. Unlocking clears the unlock time."`
	LockedUntil *string `json:"lockedUntil,omitempty" required:"false" format:"date-time" doc:"When the lock lifts on its own. Sent alone, it moves the unlock time of a locked room. Ignored unless the room ends up locked."`
}

// TeaRoomList is every room.
type TeaRoomList struct {
	TeaRooms []TeaRoom `json:"teaRooms" nullable:"false" doc:"Every room, in the staff's order."`
}

// TeaRoomOrder is every room in its new order.
type TeaRoomOrder struct {
	OrderedIDs []int64 `json:"orderedIds" minItems:"1" maxItems:"1000" uniqueItems:"true" nullable:"false" doc:"The rooms' ids, top first. A room not named keeps its place."`
}

// PublicTeaRoom is a room as sites outside the app read it.
type PublicTeaRoom struct {
	ID              int64   `json:"id" doc:"The room's id."`
	Name            string  `json:"name" doc:"The room's name."`
	Subtitle        string  `json:"subtitle" doc:"A short line under the name."`
	RoomOwner       string  `json:"roomOwner" doc:"The character who owns the room."`
	RoomNumber      string  `json:"roomNumber" doc:"The room's public number."`
	CostPerHalfHour int64   `json:"costPerHalfHour" doc:"The price in gil per half hour. A discounted room charges half."`
	Hashtags        string  `json:"hashtags" doc:"The room's hashtags, space-separated and each starting with #."`
	Description     string  `json:"description" doc:"The room's description (Markdown)."`
	Seasonal        bool    `json:"seasonal" doc:"Whether the room is seasonal rather than permanent."`
	Open            bool    `json:"open" doc:"Whether the room is open."`
	Lockable        bool    `json:"lockable" doc:"Whether the room can be locked, booked out for an extra fee."`
	Discounted      bool    `json:"discounted" doc:"Whether the room is on its half-price offer."`
	Locked          bool    `json:"locked" doc:"Whether the room is locked right now."`
	LockedUntil     *string `json:"lockedUntil" nullable:"true" doc:"When the lock lifts, RFC 3339 in UTC; null when it has no set end."`
	Image           string  `json:"image" doc:"The room's image."`
	Color           string  `json:"color" doc:"The accent color of its embed, #rrggbb; empty for the default."`
	SortOrder       int64   `json:"sortOrder" doc:"Where it sits in the staff's order."`
	CreatedAt       *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// PublicTeaRoomList is every room, for sites outside the app.
type PublicTeaRoomList struct {
	TeaRooms []PublicTeaRoom `json:"teaRooms" nullable:"false" doc:"Every room, in the staff's order."`
}

type teaRoomIDInput struct {
	ID int64 `path:"id" doc:"The room's id."`
}

type teaRoomCreateInput struct {
	Body TeaRoomInput
}

type teaRoomSaveInput struct {
	ID   int64 `path:"id" doc:"The room's id."`
	Body TeaRoomInput
}

type teaRoomChangeInput struct {
	ID   int64 `path:"id" doc:"The room's id."`
	Body TeaRoomChanges
}

type teaRoomOrderInput struct {
	Body TeaRoomOrder
}

type teaRoomWebhookInput struct {
	Body DiscordWebhook
}

type teaRoomOutput struct{ Body TeaRoom }

type teaRoomCreatedOutput struct {
	Status int
	Body   TeaRoom
}

type teaRoomListOutput struct{ Body TeaRoomList }

type teaRoomWebhookOutput struct{ Body DiscordWebhook }

type publicTeaRoomNumberInput struct {
	Number string `path:"number" doc:"The room's public number."`
}

type publicTeaRoomOutput struct{ Body PublicTeaRoom }

type publicTeaRoomListOutput struct{ Body PublicTeaRoomList }

func (h handlers) registerTeaRooms(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.TeahouseTeaRooms)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	reads := huma.Middlewares{public, h.teaRoomCORS}

	huma.Register(api, huma.Operation{
		OperationID: "list-tea-rooms", Method: http.MethodGet, Path: "/tea-rooms",
		Summary: "Every tea room", Tags: teaRoomTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listTeaRooms)
	huma.Register(api, huma.Operation{
		OperationID: "create-tea-room", Method: http.MethodPost, Path: "/tea-rooms",
		Summary: "Make a tea room", Description: "A room number another room has is refused (409).",
		Tags: teaRoomTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.createTeaRoom)
	huma.Register(api, huma.Operation{
		OperationID: "update-tea-room", Method: http.MethodPut, Path: "/tea-rooms/{id}",
		Summary: "Replace a tea room", Description: "Where it sits in the order is kept. A room number another room has is refused (409).",
		Tags: teaRoomTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.updateTeaRoom)
	huma.Register(api, huma.Operation{
		OperationID: "change-tea-room", Method: http.MethodPatch, Path: "/tea-rooms/{id}",
		Summary: "Open, discount, lock or unlock a tea room", Description: "Only what is sent changes.",
		Tags: teaRoomTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.changeTeaRoom)
	huma.Register(api, huma.Operation{
		OperationID: "delete-tea-room", Method: http.MethodDelete, Path: "/tea-rooms/{id}",
		Summary: "Delete a tea room", Description: "Its image stays in the image library.",
		Tags: teaRoomTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound),
	}, h.deleteTeaRoom)
	huma.Register(api, huma.Operation{
		OperationID: "reorder-tea-rooms", Method: http.MethodPost, Path: "/tea-rooms/reorder",
		Summary: "Put the tea rooms in order", Description: "Answers every room, in their new order.",
		Tags: teaRoomTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.reorderTeaRooms)
	huma.Register(api, huma.Operation{
		OperationID: "get-tea-room-webhook", Method: http.MethodGet, Path: "/tea-rooms/webhook",
		Summary: "The Discord webhook tea rooms post to", Tags: teaRoomTags, Security: secured,
		Middlewares: page, Errors: guarded,
	}, h.getTeaRoomWebhook)
	huma.Register(api, huma.Operation{
		OperationID: "set-tea-room-webhook", Method: http.MethodPut, Path: "/tea-rooms/webhook",
		Summary: "Set the Discord webhook tea rooms post to", Description: "An empty webhook clears it.",
		Tags: teaRoomTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.setTeaRoomWebhook)
	huma.Register(api, huma.Operation{
		OperationID: "post-tea-room", Method: http.MethodPost, Path: "/tea-rooms/{id}/post",
		Summary: "Post a tea room to Discord now", Description: "Sends its embed to the webhook tea rooms post to.",
		Tags: teaRoomTags, Security: secured, Middlewares: page,
		Errors: append(append(guarded, http.StatusNotFound), discordErrors...),
	}, h.postTeaRoom)

	huma.Register(api, huma.Operation{
		OperationID: "list-public-tea-rooms", Method: http.MethodGet, Path: "/tea-rooms/public",
		Summary: "Every tea room, for sites outside the app",
		Description: "Open to anyone. A page on a site in the allowed list (System -> Settings) may read it " +
			"from a browser; the answer is revalidated with its ETag.",
		Tags: teaRoomTags, Middlewares: reads,
	}, h.listPublicTeaRooms)
	huma.Register(api, huma.Operation{
		OperationID: "get-public-tea-room", Method: http.MethodGet, Path: "/tea-rooms/public/{number}",
		Summary:     "One tea room by its room number, for sites outside the app",
		Description: "Open to anyone, with the same cross-origin and revalidation rules as the list.",
		Tags:        teaRoomTags, Middlewares: reads, Errors: []int{http.StatusNotFound},
	}, h.getPublicTeaRoom)
}

// teaRoomCORS lets a page on an allowed site read the public tea-room operations from
// a browser. It runs before the handler, so an unknown room's 404 carries the same
// headers. The origin is echoed, never "*", and no credentials are allowed.
func (h handlers) teaRoomCORS(ctx huma.Context, next func(huma.Context)) {
	// The answer differs by Origin whether or not this request sent one, so a cache
	// never hands one site's copy to another.
	ctx.AppendHeader("Vary", "Origin")
	if origin := ctx.Header("Origin"); origin != "" && TeaRoomOriginAllowed(ctx.Context(), h.Store, origin) {
		ctx.SetHeader("Access-Control-Allow-Origin", origin)
	}
	next(ctx)
}

func toTeaRoom(t *db.TeaRoom) TeaRoom {
	return TeaRoom{
		ID: t.ID, Name: t.Name, Subtitle: t.Subtitle, RoomOwner: t.RoomOwner, RoomNumber: t.RoomNumber,
		CostPerHalfHour: t.CostPerHalfHour, Hashtags: t.Hashtags, Description: t.Description,
		Seasonal: t.Seasonal == 1, Open: t.Open == 1, Lockable: t.Lockable == 1, Discounted: t.Discounted == 1,
		Locked: t.Locked == 1, LockedUntil: t.LockedUntil, Image: t.Image, Color: t.Color,
		SortOrder: t.SortOrder, CreatedAt: t.CreatedAt,
	}
}

// toPublicTeaRoom converts rather than copies field by field: a field added to
// either type stops this compiling, so a staff-only field never reaches the public
// without someone deciding it should.
func toPublicTeaRoom(t *db.TeaRoom) PublicTeaRoom {
	return PublicTeaRoom(toTeaRoom(t))
}

// lockProblem is the 422 for an unlock time that isn't a date and time.
func lockProblem(value string) error {
	return huma.Error422UnprocessableEntity("The room can't be saved as it is.", &huma.ErrorDetail{
		Location: "body.lockedUntil", Value: value,
		Message: "The unlock time must be a date and time (RFC 3339, e.g. 2026-06-13T20:00:00Z).",
	})
}

// lockColumn is a lock's expiry as it is stored: nil for none.
func lockColumn(locked bool, until *string) (*string, bool) {
	if until == nil {
		return nil, true
	}
	stored, ok := tearooms.NormalizeLockUntil(locked, *until)
	if !ok {
		return nil, false
	}
	if stored == "" {
		return nil, true
	}
	return &stored, true
}

// numberTaken is the 409 for a room number another room has.
func numberTaken() error {
	return huma.Error409Conflict("That room number is already in use by another room.")
}

// isUniqueViolation reports whether err is the database refusing a repeated value
// under a UNIQUE index.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// numberFree reports whether number is free to use: no room has it, or it is
// exceptID's own.
func numberFree(ctx context.Context, q *db.Queries, number string, exceptID int64) (bool, error) {
	other, err := q.GetTeaRoomByNumber(ctx, number)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return other.ID == exceptID, nil
}

func (h handlers) listTeaRooms(ctx context.Context, _ *struct{}) (*teaRoomListOutput, error) {
	list, err := h.teaRoomList(ctx)
	if err != nil {
		return nil, h.fail("list tea rooms", err)
	}
	return &teaRoomListOutput{Body: list}, nil
}

func (h handlers) teaRoomList(ctx context.Context) (TeaRoomList, error) {
	rows, err := h.read().ListTeaRooms(ctx)
	if err != nil {
		return TeaRoomList{}, err
	}
	out := TeaRoomList{TeaRooms: make([]TeaRoom, 0, len(rows))}
	for _, r := range rows {
		out.TeaRooms = append(out.TeaRooms, toTeaRoom(r))
	}
	return out, nil
}

// cleaned is the input as it is stored: trimmed, hashtags normalized and the lock
// expiry in its stored form. A bad unlock time is the 422.
func (in TeaRoomInput) cleaned() (TeaRoomInput, *string, error) {
	until, ok := lockColumn(in.Locked, in.LockedUntil)
	if !ok {
		return in, nil, lockProblem(*in.LockedUntil)
	}
	in.Name = strings.TrimSpace(in.Name)
	in.RoomNumber = strings.TrimSpace(in.RoomNumber)
	in.Subtitle = strings.TrimSpace(in.Subtitle)
	in.RoomOwner = strings.TrimSpace(in.RoomOwner)
	in.Description = strings.TrimSpace(in.Description)
	in.Image = strings.TrimSpace(in.Image)
	in.Color = strings.TrimSpace(in.Color)
	in.Hashtags = tearooms.Hashtags(in.Hashtags)
	return in, until, nil
}

func (h handlers) createTeaRoom(ctx context.Context, in *teaRoomCreateInput) (*teaRoomCreatedOutput, error) {
	body, until, err := in.Body.cleaned()
	if err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	var made *db.TeaRoom
	err = h.transact(ctx, func(q *db.Queries) error {
		free, err := numberFree(ctx, q, body.RoomNumber, 0)
		if err != nil {
			return err
		}
		if !free {
			return errNumberTaken
		}
		made, err = q.CreateTeaRoom(ctx, db.CreateTeaRoomParams{
			Name: body.Name, Subtitle: body.Subtitle, RoomOwner: body.RoomOwner, RoomNumber: body.RoomNumber,
			CostPerHalfHour: body.CostPerHalfHour, Hashtags: body.Hashtags, Description: body.Description,
			Seasonal: boolInt(body.Seasonal), Open: boolInt(body.Open), Lockable: boolInt(body.Lockable),
			Discounted: boolInt(body.Discounted), Locked: boolInt(body.Locked), LockedUntil: until,
			Image: body.Image, Color: body.Color,
		})
		return err
	})
	if errors.Is(err, errNumberTaken) || isUniqueViolation(err) {
		return nil, numberTaken()
	}
	if err != nil {
		return nil, h.fail("create a tea room", err)
	}
	return &teaRoomCreatedOutput{Status: http.StatusCreated, Body: toTeaRoom(made)}, nil
}

var errNumberTaken = errors.New("room number taken")

func (h handlers) updateTeaRoom(ctx context.Context, in *teaRoomSaveInput) (*teaRoomOutput, error) {
	body, until, err := in.Body.cleaned()
	if err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	var saved *db.TeaRoom
	err = h.transact(ctx, func(q *db.Queries) error {
		if _, err := q.GetTeaRoom(ctx, in.ID); err != nil {
			return err
		}
		free, err := numberFree(ctx, q, body.RoomNumber, in.ID)
		if err != nil {
			return err
		}
		if !free {
			return errNumberTaken
		}
		saved, err = q.UpdateTeaRoom(ctx, db.UpdateTeaRoomParams{
			Name: body.Name, Subtitle: body.Subtitle, RoomOwner: body.RoomOwner, RoomNumber: body.RoomNumber,
			CostPerHalfHour: body.CostPerHalfHour, Hashtags: body.Hashtags, Description: body.Description,
			Seasonal: boolInt(body.Seasonal), Open: boolInt(body.Open), Lockable: boolInt(body.Lockable),
			Discounted: boolInt(body.Discounted), Locked: boolInt(body.Locked), LockedUntil: until,
			Image: body.Image, Color: body.Color, ID: in.ID,
		})
		return err
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, huma.Error404NotFound(noSuchTeaRoom)
	case errors.Is(err, errNumberTaken) || isUniqueViolation(err):
		return nil, numberTaken()
	case err != nil:
		return nil, h.fail("update a tea room", err)
	}
	return &teaRoomOutput{Body: toTeaRoom(saved)}, nil
}

func (h handlers) changeTeaRoom(ctx context.Context, in *teaRoomChangeInput) (*teaRoomOutput, error) {
	ctx = context.WithoutCancel(ctx)
	var saved *db.TeaRoom
	var problem error
	err := h.transact(ctx, func(q *db.Queries) error {
		existing, err := q.GetTeaRoom(ctx, in.ID)
		if err != nil {
			return err
		}
		change := in.Body
		// The lock is worked out before anything is written, so an unreadable unlock
		// time leaves the flags sent beside it unapplied. It starts from what the room
		// is, so an unlock time can be sent alone to move it; sending locked: true sets
		// the lock afresh, with no unlock time unless one is sent beside it. An unlocked
		// room never carries an expiry.
		locked, until := existing.Locked == 1, existing.LockedUntil
		if change.Locked != nil {
			locked = *change.Locked
			until = nil
		}
		if change.LockedUntil != nil {
			stored, ok := lockColumn(locked, change.LockedUntil)
			if !ok {
				problem = lockProblem(*change.LockedUntil)
				return nil
			}
			until = stored
		}
		if !locked {
			until = nil
		}
		if change.Open != nil {
			if err := q.SetTeaRoomOpen(ctx, db.SetTeaRoomOpenParams{Open: boolInt(*change.Open), ID: in.ID}); err != nil {
				return err
			}
		}
		if change.Discounted != nil {
			if err := q.SetTeaRoomDiscounted(ctx, db.SetTeaRoomDiscountedParams{Discounted: boolInt(*change.Discounted), ID: in.ID}); err != nil {
				return err
			}
		}
		if change.Locked != nil || change.LockedUntil != nil {
			if err := q.SetTeaRoomLock(ctx, db.SetTeaRoomLockParams{Locked: boolInt(locked), LockedUntil: until, ID: in.ID}); err != nil {
				return err
			}
		}
		saved, err = q.GetTeaRoom(ctx, in.ID)
		return err
	})
	switch {
	case problem != nil:
		return nil, problem
	case errors.Is(err, sql.ErrNoRows):
		return nil, huma.Error404NotFound(noSuchTeaRoom)
	case err != nil:
		return nil, h.fail("change a tea room", err)
	}
	return &teaRoomOutput{Body: toTeaRoom(saved)}, nil
}

func (h handlers) deleteTeaRoom(ctx context.Context, in *teaRoomIDInput) (*struct{}, error) {
	n, err := h.write().DeleteTeaRoom(context.WithoutCancel(ctx), in.ID)
	if err != nil {
		return nil, h.fail("delete a tea room", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound(noSuchTeaRoom)
	}
	return nil, nil
}

func (h handlers) reorderTeaRooms(ctx context.Context, in *teaRoomOrderInput) (*teaRoomListOutput, error) {
	order := in.Body.OrderedIDs
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error {
		for i, id := range order {
			n, err := q.SetTeaRoomOrder(ctx, db.SetTeaRoomOrderParams{SortOrder: int64(i), ID: id})
			if err != nil {
				return err
			}
			if n == 0 {
				return missingError{err: errNoTeaRoom, index: i}
			}
		}
		return nil
	})
	var miss missingError
	switch {
	case errors.As(err, &miss):
		return nil, unknown("body.orderedIds["+strconv.Itoa(miss.index)+"]", order[miss.index], "tea room")
	case err != nil:
		return nil, h.fail("reorder tea rooms", err)
	}
	list, err := h.teaRoomList(ctx)
	if err != nil {
		return nil, h.fail("list tea rooms", err)
	}
	return &teaRoomListOutput{Body: list}, nil
}

func (h handlers) getTeaRoomWebhook(ctx context.Context, _ *struct{}) (*teaRoomWebhookOutput, error) {
	hook, err := h.read().GetSetting(ctx, teaRoomWebhookKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, h.fail("read the tea room webhook", err)
	}
	return &teaRoomWebhookOutput{Body: DiscordWebhook{WebhookURL: hook}}, nil
}

func (h handlers) setTeaRoomWebhook(ctx context.Context, in *teaRoomWebhookInput) (*teaRoomWebhookOutput, error) {
	body := DiscordWebhook{WebhookURL: strings.TrimSpace(in.Body.WebhookURL)}
	if err := body.check(); err != nil {
		return nil, err
	}
	err := h.write().SetSetting(context.WithoutCancel(ctx), db.SetSettingParams{Key: teaRoomWebhookKey, Value: body.WebhookURL})
	if err != nil {
		return nil, h.fail("save the tea room webhook", err)
	}
	return &teaRoomWebhookOutput{Body: body}, nil
}

func (h handlers) postTeaRoom(ctx context.Context, in *teaRoomIDInput) (*teaRoomOutput, error) {
	room, err := h.read().GetTeaRoom(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchTeaRoom)
	}
	if err != nil {
		return nil, h.fail("read a tea room", err)
	}
	hook, err := h.read().GetSetting(ctx, teaRoomWebhookKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, h.fail("read the tea room webhook", err)
	}
	if strings.TrimSpace(hook) == "" {
		return nil, noWebhook("tea rooms")
	}
	embed := tearooms.Embed(tearooms.Room{
		Name: room.Name, RoomNumber: room.RoomNumber, CostPerHalfHour: room.CostPerHalfHour, Hashtags: room.Hashtags,
		Description: room.Description, Open: room.Open == 1, Discounted: room.Discounted == 1,
		Image: room.Image, Color: room.Color,
	}, requestOf(ctx).Site)
	target := discord.Target{Kind: "tea_room", Name: room.Name}
	if err := h.Discord.PostEmbed(ctx, target, hook, embed); err != nil {
		return nil, h.discordProblem(err, "post tea room")
	}
	return &teaRoomOutput{Body: toTeaRoom(room)}, nil
}

func (h handlers) listPublicTeaRooms(ctx context.Context, _ *struct{}) (*publicTeaRoomListOutput, error) {
	rows, err := h.read().ListTeaRooms(ctx)
	if err != nil {
		return nil, h.fail("list tea rooms (public)", err)
	}
	out := PublicTeaRoomList{TeaRooms: make([]PublicTeaRoom, 0, len(rows))}
	for _, r := range rows {
		out.TeaRooms = append(out.TeaRooms, toPublicTeaRoom(r))
	}
	return &publicTeaRoomListOutput{Body: out}, nil
}

func (h handlers) getPublicTeaRoom(ctx context.Context, in *publicTeaRoomNumberInput) (*publicTeaRoomOutput, error) {
	number := strings.TrimSpace(in.Number)
	if number == "" {
		return nil, huma.Error404NotFound(noSuchTeaRoom)
	}
	room, err := h.read().GetTeaRoomByNumber(ctx, number)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(noSuchTeaRoom)
	}
	if err != nil {
		return nil, h.fail("read a tea room (public)", err)
	}
	return &publicTeaRoomOutput{Body: toPublicTeaRoom(room)}, nil
}
