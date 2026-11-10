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
	"app-suite/internal/permissions"
	"app-suite/internal/themes"
)

// activeStyleKey is the settings key holding the active theme's id, "" for none.
const activeStyleKey = "active_style_id"

// StyleSummary is a theme as the theme list shows it.
type StyleSummary struct {
	ID        int64   `json:"id" doc:"The theme's id."`
	Name      string  `json:"name" doc:"The theme's name."`
	IsPublic  bool    `json:"isPublic" doc:"Whether players may pick it in the theme picker."`
	CreatedAt *string `json:"createdAt" doc:"When it was made, RFC 3339 in UTC."`
}

// StyleList is every theme, and which one the site wears.
type StyleList struct {
	Styles        []StyleSummary `json:"styles" nullable:"false" doc:"Every theme, by name."`
	ActiveStyleID *int64         `json:"activeStyleId" doc:"The theme the site wears; null for the default."`
}

// StyleInput is a theme as a save sends it.
type StyleInput struct {
	Name           string            `json:"name" minLength:"1" maxLength:"100" doc:"The theme's name."`
	Tokens         map[string]string `json:"tokens" doc:"Design-token overrides, by token name (without its leading --), each a CSS value of at most 64 characters. An unknown token is dropped, and a value is stripped of anything that could break out of its declaration."`
	BoardFlourish  string            `json:"boardFlourish" maxLength:"300" doc:"The flourish drawn in the board's corners: an images/<category>/<file>.svg path, or empty for none."`
	NumberFlourish string            `json:"numberFlourish" maxLength:"300" doc:"The flourish drawn behind the called number: an images/<category>/<file>.svg path, or empty for none."`
	IsPublic       bool              `json:"isPublic" doc:"Whether players may pick it in the theme picker."`
}

// Style is a theme in full, with the stylesheet its tokens make.
type Style struct {
	ID             int64             `json:"id" doc:"The theme's id."`
	Name           string            `json:"name" doc:"The theme's name."`
	Tokens         map[string]string `json:"tokens" doc:"Its design-token overrides, by token name."`
	CSS            string            `json:"css" doc:"The stylesheet its tokens make: one :root block, or empty when it overrides nothing."`
	BoardFlourish  string            `json:"boardFlourish" doc:"The board's corner flourish, an images/ path, or empty."`
	NumberFlourish string            `json:"numberFlourish" doc:"The called number's flourish, an images/ path, or empty."`
	IsPublic       bool              `json:"isPublic" doc:"Whether players may pick it in the theme picker."`
	CreatedAt      *string           `json:"createdAt" doc:"When it was made, RFC 3339 in UTC."`
}

// ThemeCSS is what a page applies to wear a theme.
type ThemeCSS struct {
	StyleID        *int64 `json:"styleId" doc:"The theme's id; null for the default look."`
	CSS            string `json:"css" doc:"Its stylesheet; empty for the default look."`
	BoardFlourish  string `json:"boardFlourish" doc:"The board's corner flourish, an images/ path, or empty."`
	NumberFlourish string `json:"numberFlourish" doc:"The called number's flourish, an images/ path, or empty."`
}

// PublicStyle is a theme a player may pick.
type PublicStyle struct {
	ID   int64  `json:"id" doc:"The theme's id."`
	Name string `json:"name" doc:"The theme's name."`
}

// PublicStyleList is the themes players may pick.
type PublicStyleList struct {
	Styles []PublicStyle `json:"styles" nullable:"false" doc:"The themes players may pick, by name."`
}

type styleIDInput struct {
	ID int64 `path:"id" minimum:"1" doc:"The theme's id."`
}

type styleSaveInput struct {
	ID   int64 `path:"id" minimum:"1" doc:"The theme's id."`
	Body StyleInput
}

type styleCreateInput struct {
	Body StyleInput
}

type styleOutput struct {
	Body Style
}

type styleCreatedOutput struct {
	Status int
	Body   Style
}

type styleListOutput struct {
	Body StyleList
}

type themeCSSOutput struct {
	Body ThemeCSS
}

type publicStyleListOutput struct {
	Body PublicStyleList
}

// themeChanged tells every board and console that the theme id was written (nil:
// the site went back to the default look), so a page wearing it reads it again.
func (h handlers) themeChanged(id *int64) {
	h.publish(&ThemeChangedEvent{Payload: ThemeChangedPayload{StyleID: id}}, everyone)
}

func (h handlers) registerStyles(api huma.API) {
	themesPage := huma.Middlewares{h.permitted(api, permissions.SystemThemes)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}

	huma.Register(api, huma.Operation{
		OperationID: "list-styles", Method: http.MethodGet, Path: "/styles",
		Summary: "Every theme", Tags: []string{"Themes"},
		Security: secured, Middlewares: themesPage, Errors: guarded,
	}, h.listStyles)

	huma.Register(api, huma.Operation{
		OperationID: "create-style", Method: http.MethodPost, Path: "/styles",
		Summary: "Make a theme", Description: "Every theme written is told on the live streams (theme_changed), " +
			"so the Themes page and anyone wearing it read it again.",
		Tags: []string{"Themes"}, DefaultStatus: http.StatusCreated,
		Security: secured, Middlewares: themesPage,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.createStyle)

	huma.Register(api, huma.Operation{
		OperationID: "get-style", Method: http.MethodGet, Path: "/styles/{id}",
		Summary: "A theme in full", Tags: []string{"Themes"},
		Security: secured, Middlewares: themesPage,
		Errors: append(guarded, http.StatusNotFound),
	}, h.getStyle)

	huma.Register(api, huma.Operation{
		OperationID: "update-style", Method: http.MethodPut, Path: "/styles/{id}",
		Summary: "Replace a theme", Description: "When it is the site's theme, every client wears the change at once.",
		Tags: []string{"Themes"}, Security: secured, Middlewares: themesPage,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.updateStyle)

	huma.Register(api, huma.Operation{
		OperationID: "delete-style", Method: http.MethodDelete, Path: "/styles/{id}",
		Summary: "Delete a theme", Description: "When it is the site's theme, every client goes back to the default look.",
		Tags: []string{"Themes"}, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: themesPage,
		Errors: append(guarded, http.StatusNotFound),
	}, h.deleteStyle)

	huma.Register(api, huma.Operation{
		OperationID: "activate-style", Method: http.MethodPost, Path: "/styles/{id}/activate",
		Summary: "Make a theme the site's", Description: "Every client wears it at once.",
		Tags: []string{"Themes"}, Security: secured, Middlewares: themesPage,
		Errors: append(guarded, http.StatusNotFound),
	}, h.activateStyle)

	huma.Register(api, huma.Operation{
		OperationID: "deactivate-style", Method: http.MethodPost, Path: "/styles/deactivate",
		Summary: "Go back to the default look", Description: "Every client goes back to it at once.",
		Tags: []string{"Themes"}, Security: secured, Middlewares: themesPage, Errors: guarded,
	}, h.deactivateStyle)

	huma.Register(api, huma.Operation{
		OperationID: "get-active-style", Method: http.MethodGet, Path: "/styles/active",
		Summary: "The site's theme", Description: "Public: every page wears it.",
		Tags: []string{"Themes"}, Middlewares: huma.Middlewares{public},
	}, h.activeStyle)

	huma.Register(api, huma.Operation{
		OperationID: "list-public-styles", Method: http.MethodGet, Path: "/styles/public",
		Summary: "The themes players may pick", Tags: []string{"Themes"}, Middlewares: huma.Middlewares{public},
	}, h.listPublicStyles)

	huma.Register(api, huma.Operation{
		OperationID: "get-public-style", Method: http.MethodGet, Path: "/styles/public/{id}",
		Summary: "A theme a player may pick", Description: "Its stylesheet, for the player's own view. A theme players may not pick answers 404.",
		Tags: []string{"Themes"}, Middlewares: huma.Middlewares{public}, Errors: []int{http.StatusNotFound},
	}, h.publicStyle)
}

func toStyle(id int64, name, tokens, board, number string, isPublic int64, createdAt *string) Style {
	t := themes.Unmarshal(tokens)
	return Style{
		ID: id, Name: name, Tokens: t, CSS: themes.CSS(t), BoardFlourish: board, NumberFlourish: number,
		IsPublic: isPublic == 1, CreatedAt: createdAt,
	}
}

// check applies the rules the schema can't express, each as a 422 naming its field.
func (in StyleInput) check() []error {
	var problems []error
	for field, path := range map[string]string{"boardFlourish": in.BoardFlourish, "numberFlourish": in.NumberFlourish} {
		if !themes.ValidFlourish(path) {
			problems = append(problems, &huma.ErrorDetail{Location: "body." + field, Value: path,
				Message: "A flourish is an images/<category>/<file>.svg path, or empty for none."})
		}
	}
	if strings.TrimSpace(in.Name) == "" {
		problems = append(problems, &huma.ErrorDetail{Location: "body.name", Value: in.Name, Message: "A theme needs a name."})
	}
	return problems
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// activeStyleID is the site's theme's id, or nil for the default look.
func activeStyleID(ctx context.Context, q *db.Queries) (*int64, error) {
	raw, err := q.GetSetting(ctx, activeStyleKey)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && raw == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, nil //nolint:nilerr // a value that isn't an id is no theme
	}
	return &id, nil
}

func (h handlers) listStyles(ctx context.Context, _ *struct{}) (*styleListOutput, error) {
	q := h.read()
	rows, err := q.ListStyles(ctx)
	if err != nil {
		return nil, h.fail("list styles", err)
	}
	active, err := activeStyleID(ctx, q)
	if err != nil {
		return nil, h.fail("read the active style", err)
	}
	out := StyleList{Styles: make([]StyleSummary, 0, len(rows)), ActiveStyleID: active}
	for _, r := range rows {
		out.Styles = append(out.Styles, StyleSummary{ID: r.ID, Name: r.Name, IsPublic: r.IsPublic == 1, CreatedAt: r.CreatedAt})
	}
	return &styleListOutput{Body: out}, nil
}

func (h handlers) getStyle(ctx context.Context, in *styleIDInput) (*styleOutput, error) {
	r, err := h.read().GetStyle(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("There is no such theme.")
	}
	if err != nil {
		return nil, h.fail("load a style", err)
	}
	return &styleOutput{Body: toStyle(r.ID, r.Name, r.Tokens, r.BoardFlourish, r.NumberFlourish, r.IsPublic, r.CreatedAt)}, nil
}

func (h handlers) createStyle(ctx context.Context, in *styleCreateInput) (*styleCreatedOutput, error) {
	if problems := in.Body.check(); len(problems) > 0 {
		return nil, huma.Error422UnprocessableEntity("The theme can't be saved as it is.", problems...)
	}
	tokens, err := themes.Marshal(in.Body.Tokens)
	if err != nil {
		return nil, h.fail("encode a style's tokens", err)
	}
	r, err := h.write().CreateStyle(ctx, db.CreateStyleParams{
		Name: strings.TrimSpace(in.Body.Name), Tokens: tokens, BoardFlourish: in.Body.BoardFlourish,
		NumberFlourish: in.Body.NumberFlourish, IsPublic: boolInt(in.Body.IsPublic),
	})
	if err != nil {
		return nil, h.fail("create a style", err)
	}
	h.themeChanged(&r.ID)
	return &styleCreatedOutput{Status: http.StatusCreated,
		Body: toStyle(r.ID, r.Name, r.Tokens, r.BoardFlourish, r.NumberFlourish, r.IsPublic, r.CreatedAt)}, nil
}

func (h handlers) updateStyle(ctx context.Context, in *styleSaveInput) (*styleOutput, error) {
	if problems := in.Body.check(); len(problems) > 0 {
		return nil, huma.Error422UnprocessableEntity("The theme can't be saved as it is.", problems...)
	}
	tokens, err := themes.Marshal(in.Body.Tokens)
	if err != nil {
		return nil, h.fail("encode a style's tokens", err)
	}
	r, err := h.write().UpdateStyle(ctx, db.UpdateStyleParams{
		ID: in.ID, Name: strings.TrimSpace(in.Body.Name), Tokens: tokens, BoardFlourish: in.Body.BoardFlourish,
		NumberFlourish: in.Body.NumberFlourish, IsPublic: boolInt(in.Body.IsPublic),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("There is no such theme.")
	}
	if err != nil {
		return nil, h.fail("update a style", err)
	}
	h.themeChanged(&r.ID)
	return &styleOutput{Body: toStyle(r.ID, r.Name, r.Tokens, r.BoardFlourish, r.NumberFlourish, r.IsPublic, r.CreatedAt)}, nil
}

func (h handlers) deleteStyle(ctx context.Context, in *styleIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error {
		n, err := q.DeleteStyle(ctx, in.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		active, err := activeStyleID(ctx, q)
		if err != nil || active == nil || *active != in.ID {
			return err
		}
		return q.SetSetting(ctx, db.SetSettingParams{Key: activeStyleKey, Value: ""})
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("There is no such theme.")
	}
	if err != nil {
		return nil, h.fail("delete a style", err)
	}
	h.themeChanged(&in.ID)
	return nil, nil
}

func (h handlers) activateStyle(ctx context.Context, in *styleIDInput) (*themeCSSOutput, error) {
	ctx = context.WithoutCancel(ctx)
	r, err := h.read().GetStyle(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("There is no such theme.")
	}
	if err != nil {
		return nil, h.fail("load a style", err)
	}
	if err := h.write().SetSetting(ctx, db.SetSettingParams{Key: activeStyleKey, Value: strconv.FormatInt(r.ID, 10)}); err != nil {
		return nil, h.fail("set the active style", err)
	}
	style := toStyle(r.ID, r.Name, r.Tokens, r.BoardFlourish, r.NumberFlourish, r.IsPublic, r.CreatedAt)
	h.themeChanged(&style.ID)
	return &themeCSSOutput{Body: ThemeCSS{StyleID: &style.ID, CSS: style.CSS, BoardFlourish: style.BoardFlourish, NumberFlourish: style.NumberFlourish}}, nil
}

func (h handlers) deactivateStyle(ctx context.Context, _ *struct{}) (*themeCSSOutput, error) {
	ctx = context.WithoutCancel(ctx)
	if err := h.write().SetSetting(ctx, db.SetSettingParams{Key: activeStyleKey, Value: ""}); err != nil {
		return nil, h.fail("clear the active style", err)
	}
	h.themeChanged(nil)
	return &themeCSSOutput{Body: ThemeCSS{}}, nil
}

func (h handlers) activeStyle(ctx context.Context, _ *struct{}) (*themeCSSOutput, error) {
	q := h.read()
	id, err := activeStyleID(ctx, q)
	if err != nil {
		return nil, h.fail("read the active style", err)
	}
	if id == nil {
		return &themeCSSOutput{Body: ThemeCSS{}}, nil
	}
	r, err := q.GetStyle(ctx, *id)
	if errors.Is(err, sql.ErrNoRows) {
		return &themeCSSOutput{Body: ThemeCSS{}}, nil // a theme deleted another way is no theme
	}
	if err != nil {
		return nil, h.fail("load the active style", err)
	}
	style := toStyle(r.ID, r.Name, r.Tokens, r.BoardFlourish, r.NumberFlourish, r.IsPublic, r.CreatedAt)
	return &themeCSSOutput{Body: ThemeCSS{StyleID: &style.ID, CSS: style.CSS, BoardFlourish: style.BoardFlourish, NumberFlourish: style.NumberFlourish}}, nil
}

func (h handlers) listPublicStyles(ctx context.Context, _ *struct{}) (*publicStyleListOutput, error) {
	rows, err := h.read().ListPublicStyles(ctx)
	if err != nil {
		return nil, h.fail("list public styles", err)
	}
	out := PublicStyleList{Styles: make([]PublicStyle, 0, len(rows))}
	for _, r := range rows {
		out.Styles = append(out.Styles, PublicStyle{ID: r.ID, Name: r.Name})
	}
	return &publicStyleListOutput{Body: out}, nil
}

func (h handlers) publicStyle(ctx context.Context, in *styleIDInput) (*themeCSSOutput, error) {
	r, err := h.read().GetPublicStyle(ctx, in.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("There is no such theme to pick.")
	}
	if err != nil {
		return nil, h.fail("load a public style", err)
	}
	style := toStyle(r.ID, r.Name, r.Tokens, r.BoardFlourish, r.NumberFlourish, r.IsPublic, r.CreatedAt)
	return &themeCSSOutput{Body: ThemeCSS{StyleID: &style.ID, CSS: style.CSS, BoardFlourish: style.BoardFlourish, NumberFlourish: style.NumberFlourish}}, nil
}
