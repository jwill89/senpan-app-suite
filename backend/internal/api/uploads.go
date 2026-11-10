package api

import (
	"errors"
	"fmt"
	"io"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/media"
	"app-suite/internal/uploads"
)

// The upload operations (the image library, Carrd, fonts) take up to maxUploadFiles
// files in one form of at most maxUploadBody bytes; a larger body is a 413 before it
// is read. Each file is judged on its own, so a batch with some bad files still saves
// the good ones.
const (
	maxUploadFiles = 50
	maxUploadBody  = 64<<20 + multipartOverhead
)

// SkippedUpload is a file of an upload that was not saved.
type SkippedUpload struct {
	Name   string `json:"name" doc:"The file's name as it was sent."`
	Reason string `json:"reason" doc:"Why it was not saved."`
}

// readUpload reads an uploaded file in full. The form's size is capped before it is
// read, so this holds at most the cap.
func readUpload(f huma.FormFile) ([]byte, error) {
	return io.ReadAll(f.File)
}

// checkPicture runs the image rule on an upload and answers the bytes to store, or the
// reason the picture was refused.
func (h handlers) checkPicture(data []byte) (stored []byte, reason string) {
	checked, err := uploads.CheckImage(data, uploads.ImageRule{})
	switch {
	case errors.Is(err, uploads.ErrNotImage):
		return nil, "Not a valid image"
	case errors.Is(err, uploads.ErrTooLarge):
		return nil, "Image is too large (over 40 megapixels or 16384 pixels on a side)"
	case err != nil:
		h.Logger.Warn("check an uploaded image", "error", err)
		return nil, "Failed to read"
	}
	return checked.Data, ""
}

// mediaProblem turns a media.ProblemError into its answer, naming an invalid input by its
// location ("body" or "query"); anything else is an unexpected failure.
func (h handlers) mediaProblem(where, location string, err error) error {
	p := media.AsProblem(err)
	if p == nil {
		return h.fail(where, err)
	}
	switch p.Kind {
	case media.NotFound:
		return huma.Error404NotFound(p.Msg)
	case media.Conflict:
		return huma.Error409Conflict(p.Msg)
	case media.Invalid:
		if p.Field == "" {
			return huma.Error422UnprocessableEntity(p.Msg)
		}
		return fieldProblem(location+"."+p.Field, nil, p.Msg)
	}
	return h.fail(where, err)
}

// checkUploadCount refuses a form with no files, or too many.
func checkUploadCount(n int) error {
	switch {
	case n == 0:
		return fieldProblem("body.files", nil, "Choose at least one file.")
	case n > maxUploadFiles:
		return fieldProblem("body.files", n, fmt.Sprintf("Send at most %d files at a time.", maxUploadFiles))
	}
	return nil
}

// eachUpload runs save on every file of a form and closes it. save answers what it
// stored, the file's name and, when it refused the file, the reason. The files saved
// and the files skipped come back, and when nothing was saved, the 422 that names each
// refused file (body.files[i]).
func eachUpload[T any](files []huma.FormFile, save func(huma.FormFile) (T, string, string)) (saved []T, skipped []SkippedUpload, refusal error) {
	saved = []T{}
	skipped = []SkippedUpload{}
	var details []error
	for i, f := range files {
		v, name, reason := save(f)
		if f.File != nil {
			_ = f.Close() // read in full; nothing to report on close
		}
		if reason != "" {
			skipped = append(skipped, SkippedUpload{Name: name, Reason: reason})
			details = append(details, &huma.ErrorDetail{
				Location: fmt.Sprintf("body.files[%d]", i), Message: reason, Value: name,
			})
			continue
		}
		saved = append(saved, v)
	}
	if len(saved) == 0 {
		return nil, skipped, huma.Error422UnprocessableEntity("None of the files could be saved.", details...)
	}
	return saved, skipped, nil
}
