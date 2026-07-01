package model

// Pattern represents a reusable win pattern template.
// PatternData is a 5×5 boolean grid where true means the cell must be
// called (matched) for a card to win with this pattern.
type Pattern struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	PatternData  [][]bool `json:"pattern_data"`  // 5×5 grid; true = required cell
	SortOrder    int      `json:"sort_order"`    // display order within category
	CategoryID   int64    `json:"category_id"`   // FK to pattern_categories
	CategoryName string   `json:"category_name"` // denormalized for display
}

// PatternCategory groups win patterns into named, ordered categories
// for organizational display in the admin UI.
type PatternCategory struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"` // lower values appear first
}

// PatternsResponse is the body of GET /api/patterns (and the reorder/bulk_reorder
// action replies): all patterns plus all categories.
type PatternsResponse struct {
	Patterns   []Pattern         `json:"patterns"`
	Categories []PatternCategory `json:"categories"`
}

// CreatedPattern is the freshly-created pattern echoed back by
// POST /api/patterns {action:"create"}. It is a subset of Pattern (no sort_order
// or category_name), so it is its own shape rather than the full domain type.
type CreatedPattern struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	PatternData [][]bool `json:"pattern_data"`
	CategoryID  int64    `json:"category_id"`
}

// PatternCreateResponse wraps the created pattern: {"pattern": {...}}.
type PatternCreateResponse struct {
	Pattern CreatedPattern `json:"pattern"`
}

// CategoriesResponse is the body of GET /api/pattern-categories (and the
// reorder/bulk_reorder action replies): all categories.
type CategoriesResponse struct {
	Categories []PatternCategory `json:"categories"`
}

// CategoryCreateResponse is the body of POST /api/pattern-categories
// {action:"create"}: the new category's id and name.
type CategoryCreateResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
