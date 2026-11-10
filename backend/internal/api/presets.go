package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/bingo"
	"app-suite/internal/db"
	"app-suite/internal/permissions"
)

// Game presets: a game set up once and started from again. Applying one on the Game
// page picks its patterns (those that still exist), fills in its details, and sets
// the automatic-draw controls; it never changes a game already running.

var presetTags = []string{"Presets"}

// GamePreset is a saved game set-up.
type GamePreset struct {
	ID           int64   `json:"id" doc:"The preset's id."`
	Name         string  `json:"name" doc:"The preset's name."`
	PatternIDs   []int64 `json:"patternIds" nullable:"false" doc:"The win patterns a game started from it picks. A pattern deleted since is left out when it is applied."`
	GameDetails  string  `json:"gameDetails" doc:"The game details (Markdown) it fills in."`
	Auto         bool    `json:"auto" doc:"Whether a game started from it draws numbers on its own."`
	AutoInterval int64   `json:"autoInterval" doc:"The seconds between automatic draws it fills in."`
	CreatedAt    *string `json:"createdAt" nullable:"true" doc:"When it was made, RFC 3339 in UTC."`
}

// GamePresetInput is a preset as a save sends it: all of it, since a save replaces it.
type GamePresetInput struct {
	Name         string  `json:"name" minLength:"1" maxLength:"100" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The preset's name."`
	PatternIDs   []int64 `json:"patternIds" minItems:"1" maxItems:"1000" nullable:"false" doc:"The win patterns a game started from it picks: at least one."`
	GameDetails  string  `json:"gameDetails" maxLength:"20000" doc:"The game details (Markdown) it fills in; empty for none."`
	Auto         bool    `json:"auto" doc:"Whether a game started from it draws numbers on its own."`
	AutoInterval int     `json:"autoInterval" doc:"The seconds between automatic draws, held between 5 and 600; 0 or less for the default, 30."`
}

// GamePresetList is every preset.
type GamePresetList struct {
	Presets []GamePreset `json:"presets" nullable:"false" doc:"Every preset, by name."`
}

type presetIDInput struct {
	ID int64 `path:"id" doc:"The preset's id."`
}

type presetCreateInput struct {
	Body GamePresetInput
}

type presetSaveInput struct {
	ID   int64 `path:"id" doc:"The preset's id."`
	Body GamePresetInput
}

type presetOutput struct{ Body GamePreset }

type presetCreatedOutput struct {
	Status int
	Body   GamePreset
}

type presetListOutput struct{ Body GamePresetList }

func (h handlers) registerPresets(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.BingoPresets)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}

	huma.Register(api, huma.Operation{
		OperationID: "list-presets", Method: http.MethodGet, Path: "/presets",
		Summary: "Every game preset", Tags: presetTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listPresets)
	huma.Register(api, huma.Operation{
		OperationID: "create-preset", Method: http.MethodPost, Path: "/presets",
		Summary: "Make a game preset", Tags: presetTags, DefaultStatus: http.StatusCreated,
		Security: secured, Middlewares: page, Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.createPreset)
	huma.Register(api, huma.Operation{
		OperationID: "update-preset", Method: http.MethodPut, Path: "/presets/{id}",
		Summary: "Replace a game preset", Tags: presetTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.updatePreset)
	huma.Register(api, huma.Operation{
		OperationID: "delete-preset", Method: http.MethodDelete, Path: "/presets/{id}",
		Summary: "Delete a game preset", Tags: presetTags, DefaultStatus: http.StatusNoContent,
		Security: secured, Middlewares: page, Errors: append(guarded, http.StatusNotFound),
	}, h.deletePreset)
}

func toPreset(p *db.GamePreset) GamePreset {
	ids := []int64{}
	// A list that doesn't read as ids picks nothing, rather than failing the page.
	if err := json.Unmarshal([]byte(p.PatternIds), &ids); err != nil || ids == nil {
		ids = []int64{}
	}
	return GamePreset{
		ID: p.ID, Name: p.Name, PatternIDs: ids, GameDetails: p.GameDetails,
		Auto: p.AutoCall == 1, AutoInterval: p.AutoInterval, CreatedAt: p.CreatedAt,
	}
}

// columns is the preset as it is stored.
func (in GamePresetInput) columns() (name, patternIDs string, auto, interval int64, err error) {
	ids, err := json.Marshal(in.PatternIDs)
	if err != nil {
		return "", "", 0, 0, err
	}
	return strings.TrimSpace(in.Name), string(ids), boolInt(in.Auto), int64(bingo.ClampAutoInterval(in.AutoInterval)), nil
}

func (h handlers) listPresets(ctx context.Context, _ *struct{}) (*presetListOutput, error) {
	rows, err := h.read().ListGamePresets(ctx)
	if err != nil {
		return nil, h.fail("list presets", err)
	}
	out := GamePresetList{Presets: make([]GamePreset, 0, len(rows))}
	for _, r := range rows {
		out.Presets = append(out.Presets, toPreset(r))
	}
	return &presetListOutput{Body: out}, nil
}

func (h handlers) createPreset(ctx context.Context, in *presetCreateInput) (*presetCreatedOutput, error) {
	name, ids, auto, interval, err := in.Body.columns()
	if err != nil {
		return nil, h.fail("encode a preset", err)
	}
	r, err := h.write().CreateGamePreset(ctx, db.CreateGamePresetParams{
		Name: name, PatternIds: ids, GameDetails: in.Body.GameDetails, AutoCall: auto, AutoInterval: interval,
	})
	if err != nil {
		return nil, h.fail("create a preset", err)
	}
	return &presetCreatedOutput{Status: http.StatusCreated, Body: toPreset(r)}, nil
}

func (h handlers) updatePreset(ctx context.Context, in *presetSaveInput) (*presetOutput, error) {
	name, ids, auto, interval, err := in.Body.columns()
	if err != nil {
		return nil, h.fail("encode a preset", err)
	}
	r, err := h.write().UpdateGamePreset(ctx, db.UpdateGamePresetParams{
		Name: name, PatternIds: ids, GameDetails: in.Body.GameDetails, AutoCall: auto, AutoInterval: interval, ID: in.ID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("There is no such preset.")
	}
	if err != nil {
		return nil, h.fail("update a preset", err)
	}
	return &presetOutput{Body: toPreset(r)}, nil
}

func (h handlers) deletePreset(ctx context.Context, in *presetIDInput) (*struct{}, error) {
	n, err := h.write().DeleteGamePreset(ctx, in.ID)
	if err != nil {
		return nil, h.fail("delete a preset", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound("There is no such preset.")
	}
	return nil, nil
}
