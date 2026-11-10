package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/db"
	"app-suite/internal/permissions"
)

// Win patterns and the categories that group them: the Patterns page's, which the
// game's new-game picker and the presets read too. A game copies the patterns it is
// started with, so changing one never changes a game already running.

var patternTags = []string{"Patterns"}

// PatternGrid is a pattern's 5x5 grid, row by row from the top: true where a cell
// must be called for a card to win.
type PatternGrid [][]bool

// Schema is exactly five rows of exactly five booleans, none of them null.
func (PatternGrid) Schema(huma.Registry) *huma.Schema {
	five := 5
	return &huma.Schema{
		Type: huma.TypeArray, MinItems: &five, MaxItems: &five,
		Items: &huma.Schema{
			Type: huma.TypeArray, MinItems: &five, MaxItems: &five,
			Items: &huma.Schema{Type: huma.TypeBoolean},
		},
	}
}

// Pattern is a win pattern.
type Pattern struct {
	ID           int64       `json:"id" doc:"The pattern's id."`
	Name         string      `json:"name" doc:"The pattern's name."`
	PatternData  PatternGrid `json:"patternData" doc:"Its 5x5 grid, row by row from the top: true where a cell must be called."`
	SortOrder    int64       `json:"sortOrder" doc:"Its place within its category, lowest first."`
	CategoryID   int64       `json:"categoryId" doc:"The id of the category it is in."`
	CategoryName string      `json:"categoryName" doc:"The name of the category it is in."`
}

// PatternCategory is a group of patterns.
type PatternCategory struct {
	ID        int64  `json:"id" doc:"The category's id."`
	Name      string `json:"name" doc:"The category's name."`
	SortOrder int64  `json:"sortOrder" doc:"Its place among the categories, lowest first."`
}

// PatternList is every pattern and every category, each in order.
type PatternList struct {
	Patterns   []Pattern         `json:"patterns" nullable:"false" doc:"Every pattern, by its category's order and then its own."`
	Categories []PatternCategory `json:"categories" nullable:"false" doc:"Every category, in order."`
}

// PatternInput is a new pattern.
type PatternInput struct {
	Name        string      `json:"name" minLength:"1" maxLength:"100" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The pattern's name."`
	PatternData PatternGrid `json:"patternData" doc:"Its 5x5 grid, row by row from the top: true where a cell must be called. No two patterns share a grid."`
	CategoryID  int64       `json:"categoryId" minimum:"1" doc:"The category it goes in, last."`
}

// PatternChanges is what changes on a pattern: only what is sent.
type PatternChanges struct {
	Name       *string `json:"name,omitempty" minLength:"1" maxLength:"100" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"A new name."`
	CategoryID *int64  `json:"categoryId,omitempty" minimum:"1" doc:"A category to move it to, last. Its own category leaves it where it is."`
}

// PatternMove is a step up or down.
type PatternMove struct {
	Direction string `json:"direction" enum:"up,down" doc:"Which way it moves: up (earlier) or down (later). At the end already, it stays."`
}

// PatternOrder is a category's patterns in their new order.
type PatternOrder struct {
	CategoryID int64   `json:"categoryId" minimum:"1" doc:"The category they are in from now on: a pattern from another one moves into it."`
	OrderedIDs []int64 `json:"orderedIds" minItems:"1" maxItems:"1000" uniqueItems:"true" nullable:"false" doc:"The patterns' ids, in their new order."`
}

// PatternCategoryInput is a category's name.
type PatternCategoryInput struct {
	Name string `json:"name" minLength:"1" maxLength:"100" pattern:"[\\p{L}\\p{N}\\p{P}\\p{S}]" patternDescription:"more than spaces" doc:"The category's name."`
}

// PatternCategoryOrder is every category in its new order.
type PatternCategoryOrder struct {
	OrderedIDs []int64 `json:"orderedIds" minItems:"1" maxItems:"1000" uniqueItems:"true" nullable:"false" doc:"The categories' ids, in their new order."`
}

type patternIDInput struct {
	ID int64 `path:"id" doc:"The pattern's id."`
}

type patternCreateInput struct {
	Body PatternInput
}

type patternUpdateInput struct {
	ID   int64 `path:"id" doc:"The pattern's id."`
	Body PatternChanges
}

type patternMoveInput struct {
	ID   int64 `path:"id" doc:"The pattern's id."`
	Body PatternMove
}

type patternOrderInput struct {
	Body PatternOrder
}

type patternCategoryIDInput struct {
	ID int64 `path:"id" doc:"The category's id."`
}

type patternCategoryCreateInput struct {
	Body PatternCategoryInput
}

type patternCategoryUpdateInput struct {
	ID   int64 `path:"id" doc:"The category's id."`
	Body PatternCategoryInput
}

type patternCategoryMoveInput struct {
	ID   int64 `path:"id" doc:"The category's id."`
	Body PatternMove
}

type patternCategoryOrderInput struct {
	Body PatternCategoryOrder
}

type patternOutput struct{ Body Pattern }

type patternCreatedOutput struct {
	Status int
	Body   Pattern
}

type patternListOutput struct{ Body PatternList }

type patternCategoryOutput struct{ Body PatternCategory }

type patternCategoryCreatedOutput struct {
	Status int
	Body   PatternCategory
}

func (h handlers) registerPatterns(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.BingoPatterns)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	writes := "Staff with the page hear of it on their live stream (patterns_changed)."

	huma.Register(api, huma.Operation{
		OperationID: "list-patterns", Method: http.MethodGet, Path: "/patterns",
		Summary: "Every pattern and category", Tags: patternTags,
		Security: secured, Middlewares: page, Errors: guarded,
	}, h.listPatterns)
	huma.Register(api, huma.Operation{
		OperationID: "create-pattern", Method: http.MethodPost, Path: "/patterns",
		Summary: "Make a pattern", Description: "A grid another pattern has already is refused (409). " + writes,
		Tags: patternTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusConflict, http.StatusUnprocessableEntity),
	}, h.createPattern)
	huma.Register(api, huma.Operation{
		OperationID: "update-pattern", Method: http.MethodPatch, Path: "/patterns/{id}",
		Summary: "Rename a pattern or move it to another category", Description: writes,
		Tags: patternTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.updatePattern)
	huma.Register(api, huma.Operation{
		OperationID: "move-pattern", Method: http.MethodPost, Path: "/patterns/{id}/move",
		Summary: "Move a pattern up or down its category", Description: "Answers every pattern and category, in their new order. " + writes,
		Tags: patternTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.movePattern)
	huma.Register(api, huma.Operation{
		OperationID: "reorder-patterns", Method: http.MethodPost, Path: "/patterns/reorder",
		Summary: "Put a category's patterns in order", Description: "Answers every pattern and category, in their new order. " + writes,
		Tags: patternTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.reorderPatterns)
	huma.Register(api, huma.Operation{
		OperationID: "delete-pattern", Method: http.MethodDelete, Path: "/patterns/{id}",
		Summary: "Delete a pattern", Description: "Presets that pick it pick the rest. " + writes,
		Tags: patternTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound),
	}, h.deletePattern)

	huma.Register(api, huma.Operation{
		OperationID: "create-pattern-category", Method: http.MethodPost, Path: "/pattern-categories",
		Summary: "Make a pattern category", Description: "It goes last. " + writes,
		Tags: patternTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.createPatternCategory)
	huma.Register(api, huma.Operation{
		OperationID: "update-pattern-category", Method: http.MethodPatch, Path: "/pattern-categories/{id}",
		Summary: "Rename a pattern category", Description: writes,
		Tags: patternTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.updatePatternCategory)
	huma.Register(api, huma.Operation{
		OperationID: "move-pattern-category", Method: http.MethodPost, Path: "/pattern-categories/{id}/move",
		Summary: "Move a pattern category up or down", Description: "Answers every pattern and category, in their new order. " + writes,
		Tags: patternTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusUnprocessableEntity),
	}, h.movePatternCategory)
	huma.Register(api, huma.Operation{
		OperationID: "reorder-pattern-categories", Method: http.MethodPost, Path: "/pattern-categories/reorder",
		Summary: "Put the pattern categories in order", Description: "Answers every pattern and category, in their new order. " + writes,
		Tags: patternTags, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusUnprocessableEntity),
	}, h.reorderPatternCategories)
	huma.Register(api, huma.Operation{
		OperationID: "delete-pattern-category", Method: http.MethodDelete, Path: "/pattern-categories/{id}",
		Summary: "Delete a pattern category",
		Description: "Its patterns move to the end of the first remaining category. The last category can't be " +
			"deleted (409). " + writes,
		Tags: patternTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page,
		Errors: append(guarded, http.StatusNotFound, http.StatusConflict),
	}, h.deletePatternCategory)
}

// errNoPattern and errNoCategory say a pattern or category isn't there, so the
// transaction that met it rolls back.
var (
	errNoPattern  = errors.New("no such pattern")
	errNoCategory = errors.New("no such category")
)

// missingError is an id in a list that isn't there: errNoPattern or errNoCategory, and
// where in the list it was.
type missingError struct {
	err   error
	index int
}

func (m missingError) Error() string { return fmt.Sprintf("%v at %d", m.err, m.index) }

func (m missingError) Unwrap() error { return m.err }

// unknown is a 422 naming a field that refers to something that isn't there.
func unknown(location string, value any, what string) error {
	return huma.Error422UnprocessableEntity("There is no such "+what+".", &huma.ErrorDetail{
		Location: location, Value: value, Message: "There is no such " + what + ".",
	})
}

func toPattern(id int64, name, grid string, sortOrder, categoryID int64, categoryName string) (Pattern, error) {
	p := Pattern{ID: id, Name: name, SortOrder: sortOrder, CategoryID: categoryID, CategoryName: categoryName}
	if err := json.Unmarshal([]byte(grid), &p.PatternData); err != nil {
		return Pattern{}, fmt.Errorf("pattern %d's grid: %w", id, err)
	}
	return p, nil
}

func toPatternCategory(c *db.PatternCategory) PatternCategory {
	return PatternCategory{ID: c.ID, Name: c.Name, SortOrder: c.SortOrder}
}

// patternList reads every pattern and category.
func patternList(ctx context.Context, q *db.Queries) (PatternList, error) {
	rows, err := q.ListPatterns(ctx)
	if err != nil {
		return PatternList{}, err
	}
	cats, err := q.ListPatternCategories(ctx)
	if err != nil {
		return PatternList{}, err
	}
	out := PatternList{Patterns: make([]Pattern, 0, len(rows)), Categories: make([]PatternCategory, 0, len(cats))}
	for _, r := range rows {
		p, err := toPattern(r.ID, r.Name, r.PatternData, r.SortOrder, r.CategoryID, r.CategoryName)
		if err != nil {
			return PatternList{}, err
		}
		out.Patterns = append(out.Patterns, p)
	}
	for _, c := range cats {
		out.Categories = append(out.Categories, toPatternCategory(c))
	}
	return out, nil
}

// pattern reads one pattern, or a 404.
func (h handlers) pattern(ctx context.Context, id int64) (Pattern, error) {
	r, err := h.read().GetPattern(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Pattern{}, huma.Error404NotFound("There is no such pattern.")
	}
	if err != nil {
		return Pattern{}, h.fail("read a pattern", err)
	}
	p, err := toPattern(r.ID, r.Name, r.PatternData, r.SortOrder, r.CategoryID, r.CategoryName)
	if err != nil {
		return Pattern{}, h.fail("read a pattern", err)
	}
	return p, nil
}

// patternsChanged tells the staff with the Patterns page that the patterns changed,
// and they read them again.
func (h handlers) patternsChanged() {
	h.publish(&PatternsChangedEvent{}, toPage(permissions.BingoPatterns))
}

// patternListAfter answers a reorder: the list as it is now. The staff hear of it.
func (h handlers) patternListAfter(ctx context.Context) (*patternListOutput, error) {
	h.patternsChanged()
	list, err := patternList(ctx, h.read())
	if err != nil {
		return nil, h.fail("list patterns", err)
	}
	return &patternListOutput{Body: list}, nil
}

func (h handlers) listPatterns(ctx context.Context, _ *struct{}) (*patternListOutput, error) {
	list, err := patternList(ctx, h.read())
	if err != nil {
		return nil, h.fail("list patterns", err)
	}
	return &patternListOutput{Body: list}, nil
}

// duplicateError is a grid another pattern already has.
type duplicateError struct{ name, category string }

func (d duplicateError) Error() string { return "duplicate pattern" }

func (h handlers) createPattern(ctx context.Context, in *patternCreateInput) (*patternCreatedOutput, error) {
	grid, err := json.Marshal(in.Body.PatternData)
	if err != nil {
		return nil, h.fail("encode a pattern's grid", err)
	}
	ctx = context.WithoutCancel(ctx) // a pattern made is told of, whether or not the caller waits
	var id int64
	err = h.transact(ctx, func(q *db.Queries) error {
		if _, err := q.GetPatternCategory(ctx, in.Body.CategoryID); errors.Is(err, sql.ErrNoRows) {
			return errNoCategory
		} else if err != nil {
			return err
		}
		dup, err := q.FindPatternByGrid(ctx, string(grid))
		if err == nil {
			return duplicateError{name: dup.Name, category: dup.CategoryName}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		id, err = q.CreatePattern(ctx, db.CreatePatternParams{
			Name: strings.TrimSpace(in.Body.Name), PatternData: string(grid), CategoryID: in.Body.CategoryID,
		})
		return err
	})
	var dup duplicateError
	switch {
	case errors.Is(err, errNoCategory):
		return nil, unknown("body.categoryId", in.Body.CategoryID, "category")
	case errors.As(err, &dup):
		return nil, huma.Error409Conflict(fmt.Sprintf("Duplicate pattern: that grid is %q, in the category %q.", dup.name, dup.category))
	case err != nil:
		return nil, h.fail("create a pattern", err)
	}
	p, err := h.pattern(ctx, id)
	if err != nil {
		return nil, err
	}
	h.patternsChanged()
	return &patternCreatedOutput{Status: http.StatusCreated, Body: p}, nil
}

func (h handlers) updatePattern(ctx context.Context, in *patternUpdateInput) (*patternOutput, error) {
	change := in.Body
	ctx = context.WithoutCancel(ctx) // a rename and a move land together or not at all
	err := h.transact(ctx, func(q *db.Queries) error {
		current, err := q.GetPattern(ctx, in.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return errNoPattern
		}
		if err != nil {
			return err
		}
		if change.Name != nil {
			if _, err := q.RenamePattern(ctx, db.RenamePatternParams{Name: strings.TrimSpace(*change.Name), ID: in.ID}); err != nil {
				return err
			}
		}
		if change.CategoryID == nil || *change.CategoryID == current.CategoryID {
			return nil
		}
		if _, err := q.GetPatternCategory(ctx, *change.CategoryID); errors.Is(err, sql.ErrNoRows) {
			return errNoCategory
		} else if err != nil {
			return err
		}
		last, err := q.LastPatternOrder(ctx, *change.CategoryID)
		if err != nil {
			return err
		}
		_, err = q.SetPatternOrder(ctx, db.SetPatternOrderParams{CategoryID: *change.CategoryID, SortOrder: last + 1, ID: in.ID})
		return err
	})
	switch {
	case errors.Is(err, errNoPattern):
		return nil, huma.Error404NotFound("There is no such pattern.")
	case errors.Is(err, errNoCategory):
		return nil, unknown("body.categoryId", *change.CategoryID, "category")
	case err != nil:
		return nil, h.fail("change a pattern", err)
	}
	p, err := h.pattern(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	h.patternsChanged()
	return &patternOutput{Body: p}, nil
}

// stepped is ids with id moved one place up or down, or ids as they were when it is
// at that end already.
func stepped(ids []int64, id int64, direction string) []int64 {
	i := slices.Index(ids, id)
	j := i - 1
	if direction == "down" {
		j = i + 1
	}
	if i < 0 || j < 0 || j >= len(ids) {
		return ids
	}
	out := slices.Clone(ids)
	out[i], out[j] = out[j], out[i]
	return out
}

// placePatterns puts the patterns ids into category in that order, errNoPattern when
// one isn't there.
func placePatterns(ctx context.Context, q *db.Queries, category int64, ids []int64) error {
	for i, id := range ids {
		n, err := q.SetPatternOrder(ctx, db.SetPatternOrderParams{CategoryID: category, SortOrder: int64(i), ID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			return missingError{err: errNoPattern, index: i}
		}
	}
	return nil
}

func (h handlers) movePattern(ctx context.Context, in *patternMoveInput) (*patternListOutput, error) {
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error {
		current, err := q.GetPattern(ctx, in.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return errNoPattern
		}
		if err != nil {
			return err
		}
		ids, err := q.ListPatternIDsInCategory(ctx, current.CategoryID)
		if err != nil {
			return err
		}
		return placePatterns(ctx, q, current.CategoryID, stepped(ids, in.ID, in.Body.Direction))
	})
	if errors.Is(err, errNoPattern) {
		return nil, huma.Error404NotFound("There is no such pattern.")
	}
	if err != nil {
		return nil, h.fail("move a pattern", err)
	}
	return h.patternListAfter(ctx)
}

func (h handlers) reorderPatterns(ctx context.Context, in *patternOrderInput) (*patternListOutput, error) {
	order := in.Body
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error {
		if _, err := q.GetPatternCategory(ctx, order.CategoryID); errors.Is(err, sql.ErrNoRows) {
			return errNoCategory
		} else if err != nil {
			return err
		}
		return placePatterns(ctx, q, order.CategoryID, order.OrderedIDs)
	})
	var miss missingError
	switch {
	case errors.Is(err, errNoCategory):
		return nil, unknown("body.categoryId", order.CategoryID, "category")
	case errors.As(err, &miss):
		return nil, unknown(fmt.Sprintf("body.orderedIds[%d]", miss.index), order.OrderedIDs[miss.index], "pattern")
	case err != nil:
		return nil, h.fail("reorder patterns", err)
	}
	return h.patternListAfter(ctx)
}

func (h handlers) deletePattern(ctx context.Context, in *patternIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	n, err := h.write().DeletePattern(ctx, in.ID)
	if err != nil {
		return nil, h.fail("delete a pattern", err)
	}
	if n == 0 {
		return nil, huma.Error404NotFound("There is no such pattern.")
	}
	h.patternsChanged()
	return nil, nil
}

func (h handlers) createPatternCategory(ctx context.Context, in *patternCategoryCreateInput) (*patternCategoryCreatedOutput, error) {
	ctx = context.WithoutCancel(ctx)
	c, err := h.write().CreatePatternCategory(ctx, strings.TrimSpace(in.Body.Name))
	if err != nil {
		return nil, h.fail("create a pattern category", err)
	}
	h.patternsChanged()
	return &patternCategoryCreatedOutput{Status: http.StatusCreated, Body: toPatternCategory(c)}, nil
}

func (h handlers) updatePatternCategory(ctx context.Context, in *patternCategoryUpdateInput) (*patternCategoryOutput, error) {
	ctx = context.WithoutCancel(ctx)
	c, err := h.write().RenamePatternCategory(ctx, db.RenamePatternCategoryParams{Name: strings.TrimSpace(in.Body.Name), ID: in.ID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("There is no such pattern category.")
	}
	if err != nil {
		return nil, h.fail("rename a pattern category", err)
	}
	h.patternsChanged()
	return &patternCategoryOutput{Body: toPatternCategory(c)}, nil
}

// placeCategories puts the categories ids in that order, errNoCategory when one
// isn't there.
func placeCategories(ctx context.Context, q *db.Queries, ids []int64) error {
	for i, id := range ids {
		n, err := q.SetPatternCategoryOrder(ctx, db.SetPatternCategoryOrderParams{SortOrder: int64(i), ID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			return missingError{err: errNoCategory, index: i}
		}
	}
	return nil
}

func (h handlers) movePatternCategory(ctx context.Context, in *patternCategoryMoveInput) (*patternListOutput, error) {
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error {
		cats, err := q.ListPatternCategories(ctx)
		if err != nil {
			return err
		}
		ids := make([]int64, 0, len(cats))
		for _, c := range cats {
			ids = append(ids, c.ID)
		}
		if !slices.Contains(ids, in.ID) {
			return errNoCategory
		}
		return placeCategories(ctx, q, stepped(ids, in.ID, in.Body.Direction))
	})
	if errors.Is(err, errNoCategory) {
		return nil, huma.Error404NotFound("There is no such pattern category.")
	}
	if err != nil {
		return nil, h.fail("move a pattern category", err)
	}
	return h.patternListAfter(ctx)
}

func (h handlers) reorderPatternCategories(ctx context.Context, in *patternCategoryOrderInput) (*patternListOutput, error) {
	ids := in.Body.OrderedIDs
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error { return placeCategories(ctx, q, ids) })
	if miss := (missingError{}); errors.As(err, &miss) {
		return nil, unknown(fmt.Sprintf("body.orderedIds[%d]", miss.index), ids[miss.index], "pattern category")
	}
	if err != nil {
		return nil, h.fail("reorder pattern categories", err)
	}
	return h.patternListAfter(ctx)
}

// errLastCategory is a delete that would leave no category for patterns to be in.
var errLastCategory = errors.New("the last category")

func (h handlers) deletePatternCategory(ctx context.Context, in *patternCategoryIDInput) (*struct{}, error) {
	ctx = context.WithoutCancel(ctx)
	err := h.transact(ctx, func(q *db.Queries) error {
		if _, err := q.GetPatternCategory(ctx, in.ID); errors.Is(err, sql.ErrNoRows) {
			return errNoCategory
		} else if err != nil {
			return err
		}
		fallback, err := q.FallbackPatternCategory(ctx, in.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return errLastCategory
		}
		if err != nil {
			return err
		}
		last, err := q.LastPatternOrder(ctx, fallback)
		if err != nil {
			return err
		}
		if err := q.RehomePatterns(ctx, db.RehomePatternsParams{ToCategory: fallback, After: last + 1, FromCategory: in.ID}); err != nil {
			return err
		}
		_, err = q.DeletePatternCategory(ctx, in.ID)
		return err
	})
	switch {
	case errors.Is(err, errNoCategory):
		return nil, huma.Error404NotFound("There is no such pattern category.")
	case errors.Is(err, errLastCategory):
		return nil, huma.Error409Conflict("The last pattern category can't be deleted: every pattern needs one to be in.")
	case err != nil:
		return nil, h.fail("delete a pattern category", err)
	}
	h.patternsChanged()
	return nil, nil
}
