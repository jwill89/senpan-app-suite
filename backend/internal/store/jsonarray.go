package store

import "encoding/json"

// encodeJSONArray marshals a slice to a JSON array string for storage in a TEXT
// column, normalizing nil to "[]" and never failing (returns "[]" on the
// practically-impossible marshal error). Shared by the per-feature encoders.
func encodeJSONArray[T any](in []T) string {
	if in == nil {
		in = []T{}
	}
	data, err := json.Marshal(in)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// decodeJSONArray unmarshals a JSON array string from a TEXT column, returning an
// empty (non-nil) slice for empty or malformed input so callers never get nil.
func decodeJSONArray[T any](raw string) []T {
	if raw == "" {
		return []T{}
	}
	var out []T
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []T{}
	}
	return out
}
