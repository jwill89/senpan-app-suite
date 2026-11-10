package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"app-suite/internal/fonts"
	"app-suite/internal/media"
	"app-suite/internal/permissions"
)

// Fonts: uploaded font files, grouped into fonts by base name, each with a CSS family
// name, a served format and the outside sites that may load it. Fonts are licensed, so
// they are never static files: the public operations serve them through tokens that
// rotate, gated per font by the site asking.

var fontTags = []string{"Fonts"}

// FontVariant is one format of a font: an uploaded file or the converted WOFF2 copy.
type FontVariant struct {
	Name      string  `json:"name" doc:"The file's name; the converted copy is named <base>.woff2."`
	Type      string  `json:"type" enum:"TTF,OTF,WOFF,WOFF2,EOT" doc:"Its format."`
	Converted bool    `json:"converted" doc:"True for the WOFF2 copy the server made, false for an upload."`
	Size      int64   `json:"size" doc:"Its size in bytes."`
	Modified  *string `json:"modified" nullable:"true" format:"date-time" doc:"When it was last changed, RFC 3339 in UTC; null for a copy that can't be read."`
	Token     string  `json:"token" doc:"The token its public address ends in. It changes about weekly, so read it afresh and never store it."`
}

// Font is one font with its variants and the format it serves.
type Font struct {
	Base        string        `json:"base" doc:"The file name without its extension: the font's name here, and what the edit and delete operations address it by."`
	Family      string        `json:"family" doc:"The CSS font-family name the kit, the app and the header-font picker use; the base name unless set."`
	Serve       string        `json:"serve" doc:"The format chosen to be served, or empty for automatic (WOFF2 when there is one)."`
	ServedType  string        `json:"servedType" doc:"The format actually served."`
	ServedToken string        `json:"servedToken" doc:"The token of the served format; it changes about weekly."`
	Origins     []string      `json:"origins" nullable:"false" doc:"The outside sites allowed to load it, as bare origins. The app's own origin is always allowed and never listed."`
	Modified    *string       `json:"modified" nullable:"true" format:"date-time" doc:"When its newest file was changed, RFC 3339 in UTC; null for a font with no readable file."`
	Variants    []FontVariant `json:"variants" nullable:"false" doc:"Its files and its converted copy."`
}

// FontList is the uploaded fonts.
type FontList struct {
	Fonts []Font `json:"fonts" nullable:"false" doc:"The fonts, by name."`
}

// FontChanges are the edits to a font; what is left out stays.
type FontChanges struct {
	Family  *string   `json:"family,omitempty" maxLength:"100" doc:"The CSS family name. Blank resets it to the base name. It may not hold quotes, backslashes, control characters or any of { } ; < >, and no other font may have it."`
	Serve   *string   `json:"serve,omitempty" doc:"The format to serve: TTF, OTF, WOFF, WOFF2 or EOT, one the font has. Blank means automatic."`
	Origins *[]string `json:"origins,omitempty" maxItems:"100" doc:"The outside sites allowed to load the font, replacing the list. Each is an origin such as https://mysite.carrd.co, with no path."`
}

// FontUpload says which files of an upload were saved.
type FontUpload struct {
	Uploaded []string        `json:"uploaded" nullable:"false" doc:"The names saved."`
	Skipped  []SkippedUpload `json:"skipped" nullable:"false" doc:"The files that were not, each with why. A name that is already there is skipped: delete the file first."`
	Warnings []string        `json:"warnings" nullable:"false" doc:"Files saved whose WOFF2 conversion failed, so an uploaded format is served for them."`
}

// FontUploadForm is the files of a font upload.
type FontUploadForm struct {
	Files []huma.FormFile `form:"files" required:"false" contentType:"application/octet-stream" doc:"One to 50 font files: .ttf, .otf, .woff, .woff2 or .eot. Files sharing a base name are formats of one font."`
}

// FontFileRename is a font file's name.
type FontFileRename struct {
	Name string `json:"name" minLength:"1" maxLength:"255" doc:"The file's name, with one of the font extensions."`
}

type fontBaseInput struct {
	Base string `path:"base" doc:"The font's base name."`
}

type fontUpdateInput struct {
	Base string `path:"base" doc:"The font's base name."`
	Body FontChanges
}

type fontUploadInput struct {
	RawBody huma.MultipartFormFiles[FontUploadForm]
}

type fontFileInput struct {
	Name string `path:"name" doc:"The font file's name."`
}

type fontFileRenameInput struct {
	Name string `path:"name" doc:"The font file's current name."`
	Body FontFileRename
}

type fontKitInput struct {
	Referer string `header:"Referer" required:"false" doc:"The embedding page. The kit holds only the fonts that allow its site; with no Referer, or this app's own, it holds all of them."`
}

type fontPublicInput struct {
	Token        string `path:"token" maxLength:"100" doc:"A font file's token, from the kit or the fonts list."`
	Origin       string `header:"Origin" required:"false" doc:"The requesting site. It must be on the font's list, or be this app. It is echoed back in Access-Control-Allow-Origin."`
	Referer      string `header:"Referer" required:"false" doc:"Used when there is no Origin, as Safari sends none for fonts: the embedding page's site must be on the font's list."`
	SecFetchSite string `header:"Sec-Fetch-Site" required:"false" doc:"same-origin lets the app's own pages load a font."`
}

type fontListOutput struct{ Body FontList }

type fontOutput struct{ Body Font }

type fontUploadOutput struct {
	Status int
	Body   FontUpload
}

type fontFileRenameOutput struct{ Body FontFileRename }

// fontStreamOutput is a public font answer the handler writes itself, so it can set
// its own cache headers and answer ranges and conditional requests.
type fontStreamOutput struct {
	Body func(ctx huma.Context)
}

func (h handlers) registerFonts(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.AtelierFonts)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	notFound := slices.Concat(guarded, []int{http.StatusNotFound})

	huma.Register(api, huma.Operation{
		OperationID: "list-fonts", Method: http.MethodGet, Path: "/fonts",
		Summary: "The uploaded fonts", Tags: fontTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listFonts)
	huma.Register(api, huma.Operation{
		OperationID: "upload-fonts", Method: http.MethodPost, Path: "/fonts",
		Summary: "Upload font files",
		Description: "Each file is judged on its own. A name already there is skipped, so a font in use is never " +
			"overwritten. A font with no WOFF2 gets one converted; if that fails the upload stays and a warning says so. " +
			"If none could be saved the answer is a 422 naming each file.",
		Tags: fontTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page, MaxBodyBytes: maxUploadBody,
		Errors: slices.Concat(guarded, []int{http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity}),
	}, h.uploadFonts)
	huma.Register(api, huma.Operation{
		OperationID: "update-font", Method: http.MethodPatch, Path: "/fonts/{base}",
		Summary: "Edit a font's family, served format or allowed sites", Tags: fontTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.updateFont)
	huma.Register(api, huma.Operation{
		OperationID: "delete-font", Method: http.MethodDelete, Path: "/fonts/{base}",
		Summary: "Delete a font", Description: "Every file, the converted copy and the settings of the font are deleted.",
		Tags: fontTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteFont)
	huma.Register(api, huma.Operation{
		OperationID: "rename-font-file", Method: http.MethodPatch, Path: "/font-files/{name}",
		Summary: "Rename a font file",
		Description: "A file of the same base name is another format of the same font. Its settings follow it when " +
			"the rename empties its old font.",
		Tags: fontTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.renameFontFile)
	huma.Register(api, huma.Operation{
		OperationID: "delete-font-file", Method: http.MethodDelete, Path: "/font-files/{name}",
		Summary: "Delete a font file", Tags: fontTags, DefaultStatus: http.StatusNoContent,
		Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteFontFile)

	huma.Register(api, huma.Operation{
		OperationID: "get-font-kit", Method: http.MethodGet, Path: "/fonts/pub/kit.css",
		Summary: "The font stylesheet outside sites embed",
		Description: "@font-face rules for the fonts the requesting site (by its Referer) is allowed to load, every font " +
			"for this app's own pages. Sources are relative (f/<token>) so they resolve on the host that served it. " +
			"Kept private for five minutes, and varied by Referer, since it differs by site.",
		Tags: fontTags,
		Responses: map[string]*huma.Response{"200": {
			Description: "The stylesheet.",
			Content:     map[string]*huma.MediaType{"text/css": {Schema: &huma.Schema{Type: "string"}}},
		}},
	}, h.getFontKit)
	huma.Register(api, huma.Operation{
		OperationID: "get-font-file", Method: http.MethodGet, Path: "/fonts/pub/f/{token}",
		Summary: "A font file",
		Description: "The font's bytes, for a token from the kit or the list. This app's own pages may always load it; " +
			"an outside site only when the font's list names it, by Origin or, with none, by Referer. Ranges and " +
			"conditional requests are answered. Kept private for a week, the length of a token's validity.",
		Tags: fontTags, Errors: []int{http.StatusForbidden, http.StatusNotFound},
		Responses: map[string]*huma.Response{
			"200": {Description: "The font.", Content: fontContent()},
			"206": {Description: "The part of the font a Range header asked for.", Content: fontContent()},
			"304": {Description: "The font is unchanged since the If-Modified-Since date."},
		},
	}, h.getFontFile)
}

// fontContent declares the content types a font file is served as.
func fontContent() map[string]*huma.MediaType {
	content := map[string]*huma.MediaType{}
	for _, ct := range fonts.ContentTypes {
		content[ct] = &huma.MediaType{Schema: &huma.Schema{Type: "string", Format: "binary"}}
	}
	return content
}

func fontTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func toFont(f media.Font) Font {
	out := Font{
		Base: f.Base, Family: f.Family, Serve: f.Serve, ServedType: f.ServedType, ServedToken: f.ServedToken,
		Origins: f.Origins, Variants: make([]FontVariant, 0, len(f.Variants)),
	}
	out.Modified = fontTime(f.Modified)
	for _, v := range f.Variants {
		out.Variants = append(out.Variants, FontVariant{
			Name: v.Name, Type: v.Type, Converted: v.Converted, Size: v.Size,
			Modified: fontTime(v.Modified), Token: v.Token,
		})
	}
	return out
}

// fontsChanged tells everyone the settings changed: the public settings carry the
// uploaded fonts.
func (h handlers) fontsChanged() { h.publish(&SettingsChangedEvent{}, everyone) }

func (h handlers) listFonts(ctx context.Context, _ *struct{}) (*fontListOutput, error) {
	list, err := h.Fonts.List(ctx, time.Now())
	if err != nil {
		return nil, h.fail("list fonts", err)
	}
	out := FontList{Fonts: make([]Font, 0, len(list))}
	for _, f := range list {
		out.Fonts = append(out.Fonts, toFont(f))
	}
	return &fontListOutput{Body: out}, nil
}

func (h handlers) uploadFonts(_ context.Context, in *fontUploadInput) (*fontUploadOutput, error) {
	files := in.RawBody.Data().Files
	defer func() {
		for _, f := range files {
			if f.File != nil {
				_ = f.Close() // closed again by eachUpload for the files it reached; nothing to report
			}
		}
	}()
	if err := checkUploadCount(len(files)); err != nil {
		return nil, err
	}

	var warnings []string
	uploaded, skipped, refusal := eachUpload(files, func(f huma.FormFile) (string, string, string) {
		name, ok := fonts.SafeName(f.Filename)
		if !ok {
			return "", f.Filename, "Unsupported type (allowed: .ttf, .otf, .woff, .woff2, .eot)"
		}
		data, err := io.ReadAll(f.File)
		if err != nil {
			return "", name, "Failed to read"
		}
		warning, err := h.Fonts.Add(name, data)
		switch {
		case errors.Is(err, media.ErrExists):
			return "", name, "Already exists - delete the existing file first"
		case err != nil:
			h.Logger.Warn("save an uploaded font", "error", err)
			return "", name, "Failed to save"
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		return name, name, ""
	})
	if refusal != nil {
		return nil, refusal
	}
	h.fontsChanged()
	if warnings == nil {
		warnings = []string{}
	}
	return &fontUploadOutput{Status: http.StatusCreated, Body: FontUpload{Uploaded: uploaded, Skipped: skipped, Warnings: warnings}}, nil
}

func (h handlers) updateFont(ctx context.Context, in *fontUpdateInput) (*fontOutput, error) {
	if in.Body.Family == nil && in.Body.Serve == nil && in.Body.Origins == nil {
		return nil, fieldProblem("body", nil, "Send a family, a served format or a list of sites to change.")
	}
	ctx = context.WithoutCancel(ctx)
	err := h.Fonts.Update(ctx, in.Base, media.Changes{Family: in.Body.Family, Serve: in.Body.Serve, Origins: in.Body.Origins})
	if err != nil {
		return nil, h.mediaProblem("update a font", "body", err)
	}
	f, err := h.Fonts.Get(ctx, in.Base, time.Now())
	if err != nil {
		return nil, h.mediaProblem("read a font", "body", err)
	}
	h.fontsChanged()
	return &fontOutput{Body: toFont(f)}, nil
}

func (h handlers) deleteFont(ctx context.Context, in *fontBaseInput) (*struct{}, error) {
	if err := h.Fonts.DeleteFont(ctx, in.Base); err != nil {
		return nil, h.mediaProblem("delete a font", "path", err)
	}
	h.fontsChanged()
	return nil, nil
}

func (h handlers) renameFontFile(ctx context.Context, in *fontFileRenameInput) (*fontFileRenameOutput, error) {
	name, ok := fonts.SafeName(in.Name)
	if !ok {
		return nil, huma.Error404NotFound("There is no such font file.")
	}
	newName, ok := fonts.SafeName(in.Body.Name)
	if !ok {
		return nil, fieldProblem("body.name", in.Body.Name, "Invalid new file name (allowed: .ttf, .otf, .woff, .woff2, .eot)")
	}
	if err := h.Fonts.RenameFile(ctx, name, newName); err != nil {
		return nil, h.mediaProblem("rename a font file", "body", err)
	}
	h.fontsChanged()
	return &fontFileRenameOutput{Body: FontFileRename{Name: newName}}, nil
}

func (h handlers) deleteFontFile(ctx context.Context, in *fontFileInput) (*struct{}, error) {
	name, ok := fonts.SafeName(in.Name)
	if !ok {
		return nil, huma.Error404NotFound("There is no such font file.")
	}
	if err := h.Fonts.DeleteFile(ctx, name); err != nil {
		return nil, h.mediaProblem("delete a font file", "path", err)
	}
	h.fontsChanged()
	return nil, nil
}

// -- Public serving ----------------------------------------------------------------

// requestHost is the host a request was made to, from the site the session layer
// recorded.
func requestHost(ctx context.Context) string {
	if u, err := url.Parse(requestOf(ctx).Site); err == nil {
		return u.Host
	}
	return ""
}

func (h handlers) getFontKit(ctx context.Context, in *fontKitInput) (*fontStreamOutput, error) {
	list, err := h.Fonts.Kit(ctx, fonts.KitSite(in.Referer, requestHost(ctx)))
	if err != nil {
		return nil, h.fail("build the font kit", err)
	}
	css := fonts.KitCSS(list)
	return &fontStreamOutput{Body: func(c huma.Context) {
		c.SetHeader("Content-Type", "text/css; charset=utf-8")
		c.SetHeader("X-Content-Type-Options", "nosniff")
		// Private: Cloudflare fronts the site and ignores Vary on non-image answers, so
		// a shared cache could keep one site's kit, tokens included, for every other.
		c.SetHeader("Cache-Control", "private, max-age=300")
		c.SetHeader("Vary", "Referer")
		_, _ = c.BodyWriter().Write([]byte(css))
	}}, nil
}

func (h handlers) getFontFile(ctx context.Context, in *fontPublicInput) (*fontStreamOutput, error) {
	file, ok, err := h.Fonts.FileForToken(ctx, in.Token)
	if err != nil {
		return nil, h.fail("resolve a font token", err)
	}
	if !ok {
		return nil, huma.Error404NotFound("Unknown or expired font link")
	}
	origin, allowed := fonts.RequestAllowed(fonts.Request{
		Host: requestHost(ctx), Origin: in.Origin, Referer: in.Referer, SecFetchSite: in.SecFetchSite,
	}, file.Meta)
	if !allowed {
		// The answer varies by Origin (its CORS header, and this refusal), so caches
		// must key on it. The Referer can decide it too but stays out of Vary: no
		// shared cache keeps this answer, and keying the browser's on a Referer would
		// fetch every font again on each page.
		return nil, huma.ErrorWithHeaders(huma.Error403Forbidden("This font may only be used by approved sites"),
			http.Header{"Vary": {"Origin"}})
	}
	f, err := os.Open(file.Path) //nolint:gosec // resolved from a token to a file the fonts folder holds
	if err != nil {
		return nil, huma.Error404NotFound("Unknown or expired font link")
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, h.fail("stat a font file", err)
	}
	return &fontStreamOutput{Body: func(c huma.Context) {
		defer func() { _ = f.Close() }() // read-only
		r, w := humago.Unwrap(c)
		hdr := w.Header()
		hdr.Set("Vary", "Origin")
		if origin != "" {
			hdr.Set("Access-Control-Allow-Origin", origin)
		}
		if ct := fonts.ContentTypes[strings.ToLower(extOf(file.Path))]; ct != "" {
			hdr.Set("Content-Type", ct)
		}
		hdr.Set("X-Content-Type-Options", "nosniff")
		// Private, for one token width, so a browser's copy never outlives its token.
		// Never public: Cloudflare caches fonts by default and ignores Vary: Origin, so
		// a copy primed by an allowed request would be served to any bare one,
		// bypassing the gate.
		hdr.Set("Cache-Control", "private, max-age=604800")
		http.ServeContent(w, r, "", info.ModTime(), f)
	}}, nil
}

func extOf(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i:]
	}
	return ""
}
