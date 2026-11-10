// Package media does the file work behind the image library, the Carrd projects and
// the uploaded fonts, over a web root folder. Its rules about names and bytes are in
// internal/uploads and internal/fonts; this package decides what to do with a folder.
package media

import "errors"

// Kind says how a Problem should be answered.
type Kind int

const (
	// NotFound is a folder or file that isn't there.
	NotFound Kind = iota + 1
	// Conflict is a name or folder that is already taken.
	Conflict
	// Invalid is input the rules refuse.
	Invalid
)

// ProblemError is a failure a person can act on, with the words to tell them.
type ProblemError struct {
	Kind Kind
	// Field names the input at fault ("name", "dir"), "" when it isn't one.
	Field string
	// Msg is the explanation.
	Msg string
}

func (p *ProblemError) Error() string { return p.Msg }

func notFound(msg string) error { return &ProblemError{Kind: NotFound, Msg: msg} }

func conflict(msg string) error { return &ProblemError{Kind: Conflict, Msg: msg} }

func invalid(field, msg string) error { return &ProblemError{Kind: Invalid, Field: field, Msg: msg} }

// AsProblem returns err as a ProblemError, or nil when it isn't one.
func AsProblem(err error) *ProblemError {
	var p *ProblemError
	if errors.As(err, &p) {
		return p
	}
	return nil
}

// ErrExists says a file with that name is already there and may not be replaced.
var ErrExists = errors.New("already exists")
