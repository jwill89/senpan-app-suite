package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/festival"
	"app-suite/internal/permissions"
	"app-suite/internal/raffles"
)

// Festival maps: a floor plan the public can explore. A pitch is where a booth sits
// and how it is drawn; the occupants are who stands in it, several when a booth
// changes hands between days. Staff author a map and publish it; only a published
// map is public.

var festivalMapTags = []string{"Festival Maps"}

// EventTime is a range of time a festival or one of its occupants runs.
type EventTime struct {
	Label string  `json:"label" doc:"A descriptor such as Day 1; empty for none."`
	Start string  `json:"start" format:"date-time" doc:"When the range starts, RFC 3339 in UTC."`
	End   *string `json:"end" nullable:"true" doc:"When the range ends, RFC 3339 in UTC; null for no stated end."`
}

// EventTimeInput is a range of time as a save sends it.
type EventTimeInput struct {
	Label string  `json:"label,omitempty" maxLength:"100" doc:"A descriptor such as Day 1; empty or left out for none."`
	Start string  `json:"start" doc:"When the range starts, RFC 3339. A range with no start says nothing and is dropped."`
	End   *string `json:"end,omitempty" nullable:"true" doc:"When the range ends, RFC 3339; null or left out for no stated end."`
}

// FestivalStallOccupant is who stands in a pitch.
type FestivalStallOccupant struct {
	ID            int64       `json:"id" doc:"The occupant's id."`
	AffiliateID   *int64      `json:"affiliateId" nullable:"true" doc:"The affiliate it belongs to; null for the venue itself."`
	AffiliateName string      `json:"affiliateName" doc:"That affiliate's name; empty for none."`
	Title         string      `json:"title" doc:"The stall's title; empty to be named by its affiliate."`
	Description   string      `json:"description" doc:"What it offers (Markdown)."`
	EventCarrd    string      `json:"eventCarrd" doc:"A link to this occupant's page for the event; empty for none."`
	StallType     string      `json:"stallType" enum:"game,food,both,other" doc:"What it offers."`
	TypeLabel     string      `json:"typeLabel" doc:"The caption shown when stallType is other; empty for none."`
	Times         []EventTime `json:"times" nullable:"false" doc:"When it is here; empty for the whole festival."`
	SortOrder     int64       `json:"sortOrder" doc:"Its place within the pitch."`
}

// FestivalStall is one pitch on the plan.
type FestivalStall struct {
	ID             int64                   `json:"id" doc:"The pitch's id."`
	Shape          string                  `json:"shape" enum:"circle,rect" doc:"How it is drawn."`
	Color          string                  `json:"color" doc:"Its fill, #rrggbb; empty for the default of what its first occupant offers."`
	TextColor      string                  `json:"textColor" doc:"Its label's color, #rrggbb; empty for the default."`
	SelectionColor string                  `json:"selectionColor" doc:"The halo around it when tapped, #rrggbb; empty for the default."`
	Placement      Placement               `json:"placement" doc:"Where it sits on the plan, in percent of the plan's box."`
	SortOrder      int64                   `json:"sortOrder" doc:"Its place in the plan's draw order."`
	Occupants      []FestivalStallOccupant `json:"occupants" nullable:"false" doc:"Who stands in it."`
}

// FestivalMap is a festival map as the staff see it.
type FestivalMap struct {
	ID          int64           `json:"id" doc:"The map's id."`
	Title       string          `json:"title" doc:"The festival's title."`
	Slug        string          `json:"slug" doc:"The shortcode it is linked by; empty for none."`
	PublicPath  string          `json:"publicPath" doc:"The segment its public address uses: the shortcode, else the id."`
	Description string          `json:"description" doc:"What it is about (Markdown)."`
	Times       []EventTime     `json:"times" nullable:"false" doc:"When the festival runs."`
	MapImage    string          `json:"mapImage" doc:"The bare floor plan: a path from the image library."`
	Status      string          `json:"status" enum:"in_progress,published,closed" doc:"Only a published map is public."`
	CreatedAt   *string         `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
	Stalls      []FestivalStall `json:"stalls" nullable:"false" doc:"Its pitches in draw order. Empty in a list."`
	StallCount  int64           `json:"stallCount" doc:"How many pitches it has."`
}

// FestivalMapList is every festival map.
type FestivalMapList struct {
	Maps []FestivalMap `json:"maps" nullable:"false" doc:"Newest first, without their pitches."`
}

// FestivalStallOccupantInput is an occupant as a save sends it.
type FestivalStallOccupantInput struct {
	ID          int64            `json:"id,omitempty" minimum:"0" doc:"The occupant's id to keep it (and the stamps and raffles that name it); 0 or left out adds one. One left out of the pitch's list is deleted."`
	AffiliateID *int64           `json:"affiliateId,omitempty" nullable:"true" minimum:"1" doc:"The affiliate it belongs to; null or left out for the venue itself."`
	Title       string           `json:"title" maxLength:"200" doc:"The stall's title; empty to be named by its affiliate. A new occupant with neither a title nor an affiliate is dropped."`
	Description string           `json:"description" maxLength:"20000" doc:"What it offers (Markdown)."`
	EventCarrd  string           `json:"eventCarrd" maxLength:"500" doc:"A link to this occupant's page for the event; empty for none."`
	StallType   string           `json:"stallType" enum:"game,food,both,other" doc:"What it offers."`
	TypeLabel   string           `json:"typeLabel" maxLength:"100" doc:"The caption shown when stallType is other; empty for none."`
	Times       []EventTimeInput `json:"times" maxItems:"100" nullable:"false" doc:"When it is here; empty for the whole festival."`
}

// FestivalStallInput is a pitch as a save sends it.
type FestivalStallInput struct {
	ID             int64                        `json:"id,omitempty" minimum:"0" doc:"The pitch's id to keep it; 0 or left out adds one. A pitch left out of the save is deleted, and the stamps and raffles that named its occupants lose them. An id that isn't on this map is a 409."`
	Shape          string                       `json:"shape" enum:"circle,rect" doc:"How it is drawn."`
	Color          string                       `json:"color" maxLength:"100" doc:"Its fill, #rrggbb; anything else is dropped."`
	TextColor      string                       `json:"textColor" maxLength:"100" doc:"Its label's color, #rrggbb; anything else is dropped."`
	SelectionColor string                       `json:"selectionColor" maxLength:"100" doc:"The halo around it when tapped, #rrggbb; anything else is dropped."`
	Placement      Placement                    `json:"placement" doc:"Where it sits on the plan, in percent of the plan's box; clamped."`
	Occupants      []FestivalStallOccupantInput `json:"occupants" maxItems:"50" nullable:"false" doc:"Who stands in it. A pitch left with none is dropped."`
}

// FestivalMapInput is a map as a save sends it: all of its editable fields, since a
// save replaces them. Its status changes through a patch.
type FestivalMapInput struct {
	Title       string               `json:"title" minLength:"1" maxLength:"200" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The festival's title."`
	Slug        string               `json:"slug" maxLength:"100" doc:"The shortcode to link it by, folded to lowercase words joined by dashes (2-64 characters, not only digits); empty for none."`
	Description string               `json:"description" maxLength:"20000" doc:"What it is about (Markdown); empty for none."`
	Times       []EventTimeInput     `json:"times" maxItems:"100" nullable:"false" doc:"When the festival runs."`
	MapImage    string               `json:"mapImage" maxLength:"500" doc:"The bare floor plan: a path from the image library; empty for none."`
	Stalls      []FestivalStallInput `json:"stalls" maxItems:"500" nullable:"false" doc:"The pitches in draw order. Empty clears them."`
}

// FestivalMapChanges are the changes a patch sends.
type FestivalMapChanges struct {
	Status string `json:"status" enum:"in_progress,published,closed" doc:"Where the map stands. Publishing needs a base image."`
}

// PublicStallAffiliate is an affiliate as a visitor sees it on the plan.
type PublicStallAffiliate struct {
	Name        string   `json:"name" doc:"The affiliate's name."`
	Subtitle    string   `json:"subtitle" doc:"Its tagline."`
	Owners      []string `json:"owners" nullable:"false" doc:"Who runs it."`
	Logo        string   `json:"logo" doc:"Its logo."`
	DiscordLink string   `json:"discordLink" doc:"Its Discord invite; empty for none."`
	CarrdLink   string   `json:"carrdLink" doc:"Its page; empty for none."`
}

// PublicStallRaffle is a raffle running at a stall.
type PublicStallRaffle struct {
	ID         int64  `json:"id" doc:"The raffle's id."`
	Title      string `json:"title" doc:"The raffle's title."`
	PrizeImage string `json:"prizeImage" doc:"The prize's picture."`
}

// PublicStallOccupant is an occupant as a visitor sees it.
type PublicStallOccupant struct {
	ID           int64                 `json:"id" doc:"The occupant's id."`
	Title        string                `json:"title" doc:"The stall's title."`
	Description  string                `json:"description" doc:"What it offers (Markdown)."`
	EventCarrd   string                `json:"eventCarrd" doc:"A link to its page for the event; empty for none."`
	StallType    string                `json:"stallType" enum:"game,food,both,other" doc:"What it offers."`
	TypeLabel    string                `json:"typeLabel" doc:"The caption shown when stallType is other."`
	Times        []EventTime           `json:"times" nullable:"false" doc:"When it is here; empty for the whole festival."`
	IsOpen       bool                  `json:"isOpen" doc:"Whether it is open now: inside its own hours and the festival's."`
	Affiliate    *PublicStallAffiliate `json:"affiliate" doc:"The affiliate it belongs to; null for the venue itself."`
	InStampRally bool                  `json:"inStampRally" doc:"Whether a stamp of the festival's running stamp rally is collected here."`
	RallyID      *int64                `json:"rallyId" nullable:"true" doc:"That rally; null when none."`
	RallyTitle   string                `json:"rallyTitle" doc:"That rally's title; empty when none."`
	StampImage   string                `json:"stampImage" doc:"The stamp's picture; empty when none."`
	Raffle       *PublicStallRaffle    `json:"raffle" doc:"A raffle running at this stall now; null when none."`
}

// PublicFestivalStall is a pitch as a visitor sees it.
type PublicFestivalStall struct {
	ID             int64                 `json:"id" doc:"The pitch's id."`
	Shape          string                `json:"shape" enum:"circle,rect" doc:"How it is drawn."`
	Color          string                `json:"color" doc:"Its fill; empty for the default."`
	TextColor      string                `json:"textColor" doc:"Its label's color; empty for the default."`
	SelectionColor string                `json:"selectionColor" doc:"The halo around it when tapped; empty for the default."`
	Placement      Placement             `json:"placement" doc:"Where it sits on the plan, in percent of the plan's box."`
	Occupants      []PublicStallOccupant `json:"occupants" nullable:"false" doc:"Who stands in it."`
}

// PublicFestivalMap is a published map as a visitor sees it.
type PublicFestivalMap struct {
	ID               int64                 `json:"id" doc:"The map's id."`
	Title            string                `json:"title" doc:"The festival's title."`
	Slug             string                `json:"slug" doc:"The shortcode it is linked by; empty for none."`
	PublicPath       string                `json:"publicPath" doc:"The segment its address uses: the shortcode, else the id."`
	Description      string                `json:"description" doc:"What it is about (Markdown)."`
	Times            []EventTime           `json:"times" nullable:"false" doc:"When the festival runs."`
	MapImage         string                `json:"mapImage" doc:"The bare floor plan."`
	IsActive         bool                  `json:"isActive" doc:"Whether the festival is running now."`
	Stalls           []PublicFestivalStall `json:"stalls" nullable:"false" doc:"Its pitches in draw order."`
	StampRallyID     *int64                `json:"stampRallyId" nullable:"true" doc:"The festival's stamp rally if it is running now; null when none."`
	StampRallyTitle  string                `json:"stampRallyTitle" doc:"That rally's title; empty when none."`
	StampRallySignup bool                  `json:"stampRallySignup" doc:"Whether that rally takes sign-ups from the public."`
}

// PublicFestivalMapSummary is a published map in the public list.
type PublicFestivalMapSummary struct {
	ID          int64       `json:"id" doc:"The map's id."`
	Title       string      `json:"title" doc:"The festival's title."`
	Slug        string      `json:"slug" doc:"The shortcode it is linked by; empty for none."`
	PublicPath  string      `json:"publicPath" doc:"The segment its address uses: the shortcode, else the id."`
	Description string      `json:"description" doc:"What it is about (Markdown)."`
	Times       []EventTime `json:"times" nullable:"false" doc:"When the festival runs."`
	MapImage    string      `json:"mapImage" doc:"The bare floor plan."`
	StallCount  int64       `json:"stallCount" doc:"How many pitches it has."`
	IsActive    bool        `json:"isActive" doc:"Whether the festival is running now."`
}

// PublicFestivalMapList is the published festival maps.
type PublicFestivalMapList struct {
	Maps []PublicFestivalMapSummary `json:"maps" nullable:"false" doc:"Newest first."`
}

type festivalMapIDInput struct {
	ID int64 `path:"id" doc:"The festival map's id."`
}

type festivalMapSaveInput struct {
	ID   int64 `path:"id" doc:"The festival map's id."`
	Body FestivalMapInput
}

type festivalMapCreateInput struct{ Body FestivalMapInput }

type festivalMapChangeInput struct {
	ID   int64 `path:"id" doc:"The festival map's id."`
	Body FestivalMapChanges
}

type publicMapInput struct {
	IDOrSlug string `path:"idOrSlug" maxLength:"100" doc:"The map's shortcode or numeric id."`
}

type festivalMapOutput struct{ Body FestivalMap }

type festivalMapCreatedOutput struct {
	Status int
	Body   FestivalMap
}

type festivalMapListOutput struct{ Body FestivalMapList }

type publicMapOutput struct{ Body PublicFestivalMap }

type publicMapListOutput struct{ Body PublicFestivalMapList }

func (h handlers) registerFestivalMaps(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.FestivalMap)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	notFound := slices.Concat(guarded, []int{http.StatusNotFound})

	huma.Register(api, huma.Operation{
		OperationID: "list-festival-maps", Method: http.MethodGet, Path: "/festival-maps",
		Summary:     "Every festival map",
		Description: "Also open to the staff of the Ema page, whose board editor picks its festival from this list.",
		Tags:        festivalMapTags, Security: secured,
		Middlewares: huma.Middlewares{h.permitted(api, permissions.FestivalMap, permissions.FestivalEma)}, Errors: guarded,
	}, h.listFestivalMaps)
	huma.Register(api, huma.Operation{
		OperationID: "create-festival-map", Method: http.MethodPost, Path: "/festival-maps",
		Summary: "Make a festival map", Description: "It starts in progress; publishing is a separate step.",
		Tags: festivalMapTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: slices.Concat(guarded, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.createFestivalMap)
	huma.Register(api, huma.Operation{
		OperationID: "list-public-festival-maps", Method: http.MethodGet, Path: "/festival-maps/public",
		Summary: "The published festival maps", Tags: festivalMapTags, Middlewares: huma.Middlewares{public},
	}, h.listPublicFestivalMaps)
	huma.Register(api, huma.Operation{
		OperationID: "get-public-festival-map", Method: http.MethodGet, Path: "/festival-maps/public/{idOrSlug}",
		Summary: "One published festival map",
		Description: "Found by its shortcode, else its numeric id, so a link posted before a shortcode existed keeps " +
			"working. Each pitch is resolved for a visitor: the affiliate's card, whether it is open now, its " +
			"stamp-rally badge and a raffle running there. A map that isn't published is not found.",
		Tags: festivalMapTags, Middlewares: huma.Middlewares{public}, Errors: []int{http.StatusNotFound},
	}, h.getPublicFestivalMap)
	huma.Register(api, huma.Operation{
		OperationID: "get-festival-map", Method: http.MethodGet, Path: "/festival-maps/{id}",
		Summary: "One festival map with its pitches", Tags: festivalMapTags, Security: secured, Middlewares: page, Errors: notFound,
	}, h.getFestivalMap)
	huma.Register(api, huma.Operation{
		OperationID: "update-festival-map", Method: http.MethodPut, Path: "/festival-maps/{id}",
		Summary: "Replace a festival map's editable fields",
		Description: "Its status is kept. The pitches are reconciled by id: kept ids are updated in place, 0 adds one, " +
			"and one left out is deleted. A pitch or occupant id that isn't on this map is a 409, since the editor " +
			"is out of date; nothing is saved.",
		Tags: festivalMapTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.updateFestivalMap)
	huma.Register(api, huma.Operation{
		OperationID: "change-festival-map", Method: http.MethodPatch, Path: "/festival-maps/{id}",
		Summary:     "Publish, close or reopen a festival map",
		Description: "Publishing a map with no base image is a 409: the pitches are positioned as a share of that image.",
		Tags:        festivalMapTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.changeFestivalMap)
	huma.Register(api, huma.Operation{
		OperationID: "delete-festival-map", Method: http.MethodDelete, Path: "/festival-maps/{id}",
		Summary: "Delete a festival map",
		Description: "Its pitches go with it. The stamp rallies and raffles filed under it stay, unlinked, and the stamps " +
			"that named its occupants fall back to their affiliates. Its plaque board is deleted with the plaques still " +
			"waiting for review.",
		Tags: festivalMapTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteFestivalMap)
}

func noSuchFestivalMap() error { return huma.Error404NotFound("There is no such festival map.") }

// mapPath is the segment a map's public address uses: its shortcode, else its id.
func mapPath(id int64, slug string) string {
	if slug != "" {
		return slug
	}
	return strconv.FormatInt(id, 10)
}

// storedTime is an event time as the JSON column holds it.
type storedTime struct {
	Label string `json:"label"`
	Start string `json:"start"`
	End   string `json:"end"`
}

func encodeTimes(times []festival.EventTime) string {
	out := make([]storedTime, 0, len(times))
	for _, t := range times {
		out = append(out, storedTime{Label: t.Label, Start: t.Start, End: t.End})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// decodeTimes reads a JSON column's event times; one that doesn't read is none.
func decodeTimes(raw string) []festival.EventTime {
	var stored []storedTime
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return []festival.EventTime{}
	}
	out := make([]festival.EventTime, 0, len(stored))
	for _, t := range stored {
		out = append(out, festival.EventTime{Label: t.Label, Start: t.Start, End: t.End})
	}
	return out
}

func toEventTimes(times []festival.EventTime) []EventTime {
	out := make([]EventTime, 0, len(times))
	for _, t := range times {
		e := EventTime{Label: t.Label, Start: t.Start}
		if t.End != "" {
			end := t.End
			e.End = &end
		}
		out = append(out, e)
	}
	return out
}

func toFestivalMap(m *db.GetFestivalMapRow) FestivalMap {
	return FestivalMap{
		ID: m.ID, Title: m.Title, Slug: m.Slug, PublicPath: mapPath(m.ID, m.Slug), Description: m.Description,
		Times: toEventTimes(decodeTimes(m.Times)), MapImage: m.MapImage, Status: festival.NormalizeMapStatus(m.Status),
		CreatedAt: m.CreatedAt, Stalls: []FestivalStall{}, StallCount: m.StallCount,
	}
}

// findFestivalMap reads a map with its pitches and their occupants, or nil when there
// is none.
func (h handlers) findFestivalMap(ctx context.Context, q *db.Queries, id int64) (*FestivalMap, error) {
	row, err := q.GetFestivalMap(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, h.fail("read a festival map", err)
	}
	return h.withStalls(ctx, q, row)
}

// withStalls is a map row with its pitches read.
func (h handlers) withStalls(ctx context.Context, q *db.Queries, row *db.GetFestivalMapRow) (*FestivalMap, error) {
	m := toFestivalMap(row)
	stalls, err := q.ListFestivalStalls(ctx, row.ID)
	if err != nil {
		return nil, h.fail("read a map's pitches", err)
	}
	occupants, err := q.ListMapOccupants(ctx, row.ID)
	if err != nil {
		return nil, h.fail("read a map's occupants", err)
	}
	byStall := make(map[int64][]FestivalStallOccupant, len(stalls))
	for _, o := range occupants {
		byStall[o.StallID] = append(byStall[o.StallID], FestivalStallOccupant{
			ID: o.ID, AffiliateID: intPtr(o.AffiliateID), AffiliateName: o.AffiliateName, Title: o.Title,
			Description: o.Description, EventCarrd: o.EventCarrd, StallType: festival.NormalizeStallType(o.StallType),
			TypeLabel: o.TypeLabel, Times: toEventTimes(decodeTimes(o.Times)), SortOrder: o.SortOrder,
		})
	}
	for _, s := range stalls {
		os := byStall[s.ID]
		if os == nil {
			os = []FestivalStallOccupant{}
		}
		m.Stalls = append(m.Stalls, FestivalStall{
			ID: s.ID, Shape: festival.NormalizeShape(s.Shape), Color: s.Color, TextColor: s.TextColor,
			SelectionColor: s.SelectionColor, SortOrder: s.SortOrder, Occupants: os,
			Placement: Placement{X: s.PosX, Y: s.PosY, Width: s.Width, Height: s.Height, Rotation: s.Rotation},
		})
	}
	return &m, nil
}

func (h handlers) loadFestivalMap(ctx context.Context, q *db.Queries, id int64) (FestivalMap, error) {
	m, err := h.findFestivalMap(ctx, q, id)
	if err != nil {
		return FestivalMap{}, err
	}
	if m == nil {
		return FestivalMap{}, noSuchFestivalMap()
	}
	return *m, nil
}

func (h handlers) listFestivalMaps(ctx context.Context, _ *struct{}) (*festivalMapListOutput, error) {
	rows, err := h.read().ListFestivalMaps(ctx)
	if err != nil {
		return nil, h.fail("list festival maps", err)
	}
	out := FestivalMapList{Maps: make([]FestivalMap, 0, len(rows))}
	for _, r := range rows {
		out.Maps = append(out.Maps, toFestivalMap((*db.GetFestivalMapRow)(r)))
	}
	return &festivalMapListOutput{Body: out}, nil
}

func (h handlers) getFestivalMap(ctx context.Context, in *festivalMapIDInput) (*festivalMapOutput, error) {
	m, err := h.loadFestivalMap(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	return &festivalMapOutput{Body: m}, nil
}

// mapSave is a map's fields as they are stored.
type mapSave struct {
	title, slug, description, times, mapImage string
	stalls                                    []stallSave
}

type stallSave struct {
	id        int64
	stall     db.InsertFestivalStallParams
	occupants []occupantSave
}

type occupantSave struct {
	id int64
	db.InsertStallOccupantParams
}

// eventTimes puts the ranges a client sent in the stored form, naming the field of
// any that isn't a time.
func eventTimes(location string, in []EventTimeInput, add func(*huma.ErrorDetail)) []festival.EventTime {
	out := make([]festival.EventTime, 0, len(in))
	for i, t := range in {
		loc := location + "[" + strconv.Itoa(i) + "]"
		start, d := normalizeInput(loc+".start", &t.Start)
		add(d)
		end, d := normalizeInput(loc+".end", t.End)
		add(d)
		e := festival.EventTime{Label: t.Label}
		if start != nil {
			e.Start = *start
		}
		if end != nil {
			e.End = *end
		}
		out = append(out, e)
	}
	return festival.SanitizeEventTimes(out)
}

// mapColumns checks a map's fields the schema can't and puts them as they are
// stored: the shortcode folded and judged, the times normalized, colors reduced to a
// hex or nothing, placements clamped, and the pitches and occupants that say nothing
// dropped.
func mapColumns(in FestivalMapInput) (mapSave, error) {
	var problems []error
	add := func(d *huma.ErrorDetail) {
		if d != nil {
			problems = append(problems, d)
		}
	}
	slug := festival.NormalizeSlug(in.Slug)
	if msg := festival.SlugError(slug); msg != "" {
		problems = append(problems, &huma.ErrorDetail{Location: "body.slug", Value: in.Slug, Message: msg})
	}
	out := mapSave{
		title: strings.TrimSpace(in.Title), slug: slug, description: in.Description, mapImage: strings.TrimSpace(in.MapImage),
		times: encodeTimes(eventTimes("body.times", in.Times, add)),
	}
	for i, st := range in.Stalls {
		loc := "body.stalls[" + strconv.Itoa(i) + "]"
		s := stallSave{id: st.ID}
		for j, o := range st.Occupants {
			if !festival.KeepOccupant(o.ID, o.Title, o.AffiliateID != nil) {
				continue
			}
			s.occupants = append(s.occupants, occupantSave{id: o.ID, InsertStallOccupantParams: db.InsertStallOccupantParams{
				AffiliateID: nullInt(o.AffiliateID), Title: strings.TrimSpace(o.Title), Description: o.Description,
				EventCarrd: strings.TrimSpace(o.EventCarrd), StallType: festival.NormalizeStallType(o.StallType),
				TypeLabel: strings.TrimSpace(o.TypeLabel),
				Times:     encodeTimes(eventTimes(loc+".occupants["+strconv.Itoa(j)+"].times", o.Times, add)),
				SortOrder: int64(len(s.occupants)),
			}})
		}
		// A shape on the plan that names nobody is a box a visitor can open for an empty panel.
		if len(s.occupants) == 0 {
			continue
		}
		p := st.Placement.sanitized()
		s.stall = db.InsertFestivalStallParams{
			Shape: festival.NormalizeShape(st.Shape), Color: festival.SanitizeColor(st.Color),
			SelectionColor: festival.SanitizeColor(st.SelectionColor), TextColor: festival.SanitizeColor(st.TextColor),
			PosX: p.X, PosY: p.Y, Width: p.Width, Height: p.Height, Rotation: p.Rotation, SortOrder: int64(len(out.stalls)),
		}
		out.stalls = append(out.stalls, s)
	}
	if len(problems) > 0 {
		return mapSave{}, huma.Error422UnprocessableEntity("The festival map can't be saved as it is.", problems...)
	}
	return out, nil
}

var errStaleMap = huma.Error409Conflict("This map changed since you opened it. Reload the map and reapply your edit.")

// slugFree is a 409 when another map holds the shortcode.
func slugFree(ctx context.Context, q *db.Queries, slug string, mapID int64) error {
	if slug == "" {
		return nil
	}
	other, err := q.GetFestivalMapBySlug(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if other.ID != mapID {
		return huma.Error409Conflict("That shortcode is already used by another festival map.")
	}
	return nil
}

// saveStalls reconciles a map's pitches by id in the transaction q is bound to.
func saveStalls(ctx context.Context, q *db.Queries, mapID int64, stalls []stallSave) error {
	keep := make(map[int64]bool, len(stalls))
	for _, s := range stalls {
		id := s.id
		if id > 0 {
			n, err := q.UpdateFestivalStall(ctx, db.UpdateFestivalStallParams{
				Shape: s.stall.Shape, Color: s.stall.Color, SelectionColor: s.stall.SelectionColor, TextColor: s.stall.TextColor,
				PosX: s.stall.PosX, PosY: s.stall.PosY, Width: s.stall.Width, Height: s.stall.Height,
				Rotation: s.stall.Rotation, SortOrder: s.stall.SortOrder, ID: id, MapID: mapID,
			})
			if err != nil {
				return err
			}
			// Counted, because the occupants below are keyed on the pitch id alone: a save of
			// this map carrying another map's pitch id must not rewrite that pitch's occupants.
			if n == 0 {
				return errStaleMap
			}
		} else {
			s.stall.MapID = mapID
			var err error
			if id, err = q.InsertFestivalStall(ctx, s.stall); err != nil {
				return err
			}
		}
		keep[id] = true
		if err := saveOccupants(ctx, q, id, s.occupants); err != nil {
			return err
		}
	}
	existing, err := q.ListMapStallIDs(ctx, mapID)
	if err != nil {
		return err
	}
	for _, id := range existing {
		if keep[id] {
			continue
		}
		if err := q.ClearStallStampOccupants(ctx, id); err != nil {
			return err
		}
		if err := q.ClearStallRaffleOccupants(ctx, id); err != nil {
			return err
		}
		if err := q.DeleteFestivalStall(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// saveOccupants reconciles a pitch's occupants by id, so one that survives an edit
// keeps the id its stamps and raffles point at.
func saveOccupants(ctx context.Context, q *db.Queries, stallID int64, occupants []occupantSave) error {
	keep := make(map[int64]bool, len(occupants))
	for _, o := range occupants {
		id := o.id
		if id > 0 {
			n, err := q.UpdateStallOccupant(ctx, db.UpdateStallOccupantParams{
				AffiliateID: o.AffiliateID, Title: o.Title, Description: o.Description, EventCarrd: o.EventCarrd,
				StallType: o.StallType, TypeLabel: o.TypeLabel, Times: o.Times, SortOrder: o.SortOrder, ID: id, StallID: stallID,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return errStaleMap
			}
		} else {
			o.StallID = stallID
			var err error
			if id, err = q.InsertStallOccupant(ctx, o.InsertStallOccupantParams); err != nil {
				return err
			}
		}
		keep[id] = true
	}
	existing, err := q.ListStallOccupantIDs(ctx, stallID)
	if err != nil {
		return err
	}
	for _, id := range existing {
		if keep[id] {
			continue
		}
		// The stamps and raffles that named it keep what they are, minus the pitch.
		if err := q.ClearOccupantStamps(ctx, sql.NullInt64{Int64: id, Valid: true}); err != nil {
			return err
		}
		if err := q.ClearOccupantRaffles(ctx, sql.NullInt64{Int64: id, Valid: true}); err != nil {
			return err
		}
		if err := q.DeleteStallOccupant(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// problemOr answers a problem a transaction returned as it is, and any other error as
// the failure of what.
func (h handlers) problemOr(err error, what string) error {
	var problem huma.StatusError
	switch {
	case errors.As(err, &problem):
		return problem
	case uniqueViolation(err):
		return huma.Error409Conflict("That shortcode is already used by another festival map.")
	default:
		return h.fail(what, err)
	}
}

func (h handlers) createFestivalMap(ctx context.Context, in *festivalMapCreateInput) (*festivalMapCreatedOutput, error) {
	save, err := mapColumns(in.Body)
	if err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	var id int64
	err = h.transact(ctx, func(q *db.Queries) error {
		if err := slugFree(ctx, q, save.slug, 0); err != nil {
			return err
		}
		var err error
		id, err = q.CreateFestivalMap(ctx, db.CreateFestivalMapParams{
			Title: save.title, Slug: save.slug, Description: save.description, Times: save.times, MapImage: save.mapImage,
		})
		if err != nil {
			return err
		}
		return saveStalls(ctx, q, id, save.stalls)
	})
	if err != nil {
		return nil, h.problemOr(err, "create a festival map")
	}
	m, err := h.loadFestivalMap(ctx, h.read(), id)
	if err != nil {
		return nil, err
	}
	return &festivalMapCreatedOutput{Status: http.StatusCreated, Body: m}, nil
}

func (h handlers) updateFestivalMap(ctx context.Context, in *festivalMapSaveInput) (*festivalMapOutput, error) {
	save, err := mapColumns(in.Body)
	if err != nil {
		return nil, err
	}
	ctx = context.WithoutCancel(ctx)
	err = h.transact(ctx, func(q *db.Queries) error {
		if err := slugFree(ctx, q, save.slug, in.ID); err != nil {
			return err
		}
		n, err := q.UpdateFestivalMap(ctx, db.UpdateFestivalMapParams{
			Title: save.title, Slug: save.slug, Description: save.description, Times: save.times, MapImage: save.mapImage, ID: in.ID,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return noSuchFestivalMap()
		}
		return saveStalls(ctx, q, in.ID, save.stalls)
	})
	if err != nil {
		return nil, h.problemOr(err, "update a festival map")
	}
	m, err := h.loadFestivalMap(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	// The stall names show on the stamp cards and the raffles.
	ResourceChanged(h.Live, "stamp-rallies")
	ResourceChanged(h.Live, "raffles")
	return &festivalMapOutput{Body: m}, nil
}

func (h handlers) changeFestivalMap(ctx context.Context, in *festivalMapChangeInput) (*festivalMapOutput, error) {
	ctx = context.WithoutCancel(ctx)
	m, err := h.loadFestivalMap(ctx, h.read(), in.ID)
	if err != nil {
		return nil, err
	}
	if in.Body.Status == festival.MapPublished && strings.TrimSpace(m.MapImage) == "" {
		return nil, huma.Error409Conflict("Pick a base map image before publishing - the stalls are positioned against it.")
	}
	n, err := h.write().SetFestivalMapStatus(ctx, db.SetFestivalMapStatusParams{Status: in.Body.Status, ID: in.ID})
	if err != nil {
		return nil, h.fail("set a festival map's status", err)
	}
	if n == 0 {
		return nil, noSuchFestivalMap()
	}
	m.Status = in.Body.Status
	return &festivalMapOutput{Body: m}, nil
}

func (h handlers) deleteFestivalMap(ctx context.Context, in *festivalMapIDInput) (*struct{}, error) {
	// The festival's plaque board goes with it (the table cascades), so the images still
	// waiting for review are gathered first and removed after. Detached, so the rows and
	// the files go together.
	ctx = context.WithoutCancel(ctx)
	images, err := h.read().ListEmaMapImages(ctx, in.ID)
	if err != nil {
		return nil, h.fail("list a festival's plaque images", err)
	}
	var deleted int64
	err = h.transact(ctx, func(q *db.Queries) error {
		if err := q.ClearMapStampOccupants(ctx, in.ID); err != nil {
			return err
		}
		if err := q.ClearMapRaffles(ctx, sql.NullInt64{Int64: in.ID, Valid: true}); err != nil {
			return err
		}
		if err := q.ClearMapRallies(ctx, sql.NullInt64{Int64: in.ID, Valid: true}); err != nil {
			return err
		}
		var err error
		deleted, err = q.DeleteFestivalMap(ctx, in.ID)
		return err
	})
	if err != nil {
		return nil, h.fail("delete a festival map", err)
	}
	if deleted == 0 {
		return nil, noSuchFestivalMap()
	}
	for _, img := range images {
		h.removePlaqueImage(img)
	}
	// The middleware tells the maps; these pages showed what the delete unlinked or removed.
	for _, resource := range []string{"stamp-rallies", "raffles", "ema-boards"} {
		ResourceChanged(h.Live, resource)
	}
	return nil, nil
}

// festivalLink is the shortcode or id of the published festival a rally is filed
// under, or "" for none: a lookup that fails only leaves the link out.
func (h handlers) festivalLink(ctx context.Context, rallyID int64) string {
	m, err := h.read().GetRallyFestival(ctx, rallyID)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		h.Logger.Error("look up a rally's festival", "rallyId", rallyID, "error", err)
		return ""
	}
	return mapPath(m.ID, m.Slug)
}

// resolvePublicMap finds a map by its shortcode or, failing that, its numeric id. A
// purely numeric shortcode is refused on save, so the two forms never collide.
func (h handlers) resolvePublicMap(ctx context.Context, q *db.Queries, idOrSlug string) (*db.GetFestivalMapRow, error) {
	idOrSlug = strings.TrimSpace(idOrSlug)
	if slug := festival.NormalizeSlug(idOrSlug); slug != "" {
		row, err := q.GetFestivalMapBySlug(ctx, slug)
		if err == nil {
			return (*db.GetFestivalMapRow)(row), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, h.fail("read a festival map", err)
		}
	}
	id, err := strconv.ParseInt(idOrSlug, 10, 64)
	if err != nil {
		return nil, nil //nolint:nilerr // neither a known shortcode nor an id: not found
	}
	row, err := q.GetFestivalMap(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, h.fail("read a festival map", err)
	}
	return row, nil
}

// publishedMap is the published map a segment names, or the 404.
func (h handlers) publishedMap(ctx context.Context, q *db.Queries, idOrSlug string) (*db.GetFestivalMapRow, error) {
	row, err := h.resolvePublicMap(ctx, q, idOrSlug)
	if err != nil {
		return nil, err
	}
	// A draft's existence isn't something this answer should confirm.
	if row == nil || festival.NormalizeMapStatus(row.Status) != festival.MapPublished {
		return nil, noSuchFestivalMap()
	}
	return row, nil
}

func (h handlers) listPublicFestivalMaps(ctx context.Context, _ *struct{}) (*publicMapListOutput, error) {
	rows, err := h.read().ListPublishedFestivalMaps(ctx)
	if err != nil {
		return nil, h.fail("list published festival maps", err)
	}
	now := time.Now().UTC()
	out := PublicFestivalMapList{Maps: make([]PublicFestivalMapSummary, 0, len(rows))}
	for _, r := range rows {
		times := decodeTimes(r.Times)
		out.Maps = append(out.Maps, PublicFestivalMapSummary{
			ID: r.ID, Title: r.Title, Slug: r.Slug, PublicPath: mapPath(r.ID, r.Slug), Description: r.Description,
			Times: toEventTimes(times), MapImage: r.MapImage, StallCount: r.StallCount,
			IsActive: festival.WithinAnyEventTime(times, now),
		})
	}
	return &publicMapListOutput{Body: out}, nil
}

// publicAffiliates reads the public card of each affiliate a map's occupants name:
// one read per distinct affiliate.
func (h handlers) publicAffiliates(ctx context.Context, q *db.Queries, m *FestivalMap) (map[int64]*PublicStallAffiliate, error) {
	cards := make(map[int64]*PublicStallAffiliate)
	for _, s := range m.Stalls {
		for _, o := range s.Occupants {
			if o.AffiliateID == nil {
				continue
			}
			id := *o.AffiliateID
			if _, done := cards[id]; done {
				continue
			}
			a, err := q.GetAffiliateByID(ctx, id)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			cards[id] = &PublicStallAffiliate{
				Name: a.Name, Subtitle: a.Subtitle, Owners: decodeList[string](a.Owners), Logo: a.Logo,
				DiscordLink: a.DiscordLink, CarrdLink: a.CarrdLink,
			}
		}
	}
	return cards, nil
}

func (h handlers) getPublicFestivalMap(ctx context.Context, in *publicMapInput) (*publicMapOutput, error) {
	q := h.read()
	row, err := h.publishedMap(ctx, q, in.IDOrSlug)
	if err != nil {
		return nil, err
	}
	m, err := h.withStalls(ctx, q, row)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	// The stamps an open linked rally placed on this map's pitches, so each can be badged
	// without a second round trip. Not closed isn't running: the window is judged here
	// against the clock, as the raffles' are below.
	var rallyStamps map[int64]string
	var rallyID int64
	var rallyTitle string
	var rallySignup bool
	switch rally, err := q.GetOpenRallyForMap(ctx, sql.NullInt64{Int64: row.ID, Valid: true}); {
	case err == nil && festival.WithinWindow(deref(rally.AvailableFrom), deref(rally.AvailableTo), now):
		rallyID, rallyTitle, rallySignup = rally.ID, rally.Title, rally.PublicSignup != 0
		stamps, err := q.ListRallyStampImagesForMap(ctx, rally.ID)
		if err != nil {
			return nil, h.fail("list a rally's stamps for a festival map", err)
		}
		rallyStamps = make(map[int64]string, len(stamps))
		for _, s := range stamps {
			rallyStamps[s.OccupantID.Int64] = s.Image
		}
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return nil, h.fail("read a festival map's rally", err)
	}

	// Raffles pinned to this map's pitches, oldest first so the newest of a pitch wins.
	rows, err := q.ListStallRaffles(ctx, row.ID)
	if err != nil {
		return nil, h.fail("list a festival map's raffles", err)
	}
	stallRaffles := make(map[int64]*db.ListStallRafflesRow, len(rows))
	for _, r := range rows {
		stallRaffles[r.OccupantID.Int64] = r
	}

	affiliates, err := h.publicAffiliates(ctx, q, m)
	if err != nil {
		return nil, h.fail("read a festival map's affiliates", err)
	}

	times := make([]festival.EventTime, 0, len(m.Times))
	for _, t := range m.Times {
		times = append(times, festival.EventTime{Start: t.Start, End: deref(t.End)})
	}
	festivalOpen := festival.WithinAnyEventTime(times, now)
	out := PublicFestivalMap{
		ID: m.ID, Title: m.Title, Slug: m.Slug, PublicPath: m.PublicPath, Description: m.Description, Times: m.Times,
		MapImage: m.MapImage, IsActive: festivalOpen, Stalls: make([]PublicFestivalStall, 0, len(m.Stalls)),
		StampRallyTitle: rallyTitle, StampRallySignup: rallySignup,
	}
	if rallyID != 0 {
		out.StampRallyID = &rallyID
	}
	for _, s := range m.Stalls {
		ps := PublicFestivalStall{
			ID: s.ID, Shape: s.Shape, Color: s.Color, TextColor: s.TextColor, SelectionColor: s.SelectionColor,
			Placement: s.Placement, Occupants: make([]PublicStallOccupant, 0, len(s.Occupants)),
		}
		for _, o := range s.Occupants {
			ownTimes := make([]festival.EventTime, 0, len(o.Times))
			for _, t := range o.Times {
				ownTimes = append(ownTimes, festival.EventTime{Start: t.Start, End: deref(t.End)})
			}
			po := PublicStallOccupant{
				ID: o.ID, Title: o.Title, Description: o.Description, EventCarrd: o.EventCarrd, StallType: o.StallType,
				TypeLabel: o.TypeLabel, Times: o.Times,
				// An occupant that keeps no hours of its own follows the festival's.
				IsOpen: festival.WithinAnyEventTime(ownTimes, now) && festivalOpen,
			}
			if o.AffiliateID != nil {
				po.Affiliate = affiliates[*o.AffiliateID]
			}
			if image, found := rallyStamps[o.ID]; found && rallyID != 0 {
				id := rallyID
				po.InStampRally, po.RallyID, po.RallyTitle, po.StampImage = true, &id, rallyTitle, image
			}
			if r, found := stallRaffles[o.ID]; found && raffles.Running(r.Status, deref(r.AvailableFrom), deref(r.AvailableTo), now) {
				po.Raffle = &PublicStallRaffle{ID: r.ID, Title: r.Title, PrizeImage: r.PrizeImage}
			}
			ps.Occupants = append(ps.Occupants, po)
		}
		out.Stalls = append(out.Stalls, ps)
	}
	return &publicMapOutput{Body: out}, nil
}
