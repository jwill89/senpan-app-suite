package api

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/media"
	"app-suite/internal/permissions"
	"app-suite/internal/uploads"
)

// The image library: categories (folders of pictures served at /images/<dir>/<file>)
// and the pictures in them. Every page with a picture field may read it; only the
// Images page changes it.

var imageTags = []string{"Images"}

const maxSVGBytes = 2 << 20

// ImageCategory is a folder of the image library.
type ImageCategory struct {
	Name      string `json:"name" doc:"What staff call it."`
	Dir       string `json:"dir" doc:"Its folder name: lowercase letters, digits and underscores. Pictures are served at /images/<dir>/<file>."`
	FileCount int    `json:"fileCount" doc:"How many pictures it holds."`
	TotalSize int64  `json:"totalSize" doc:"Their combined size in bytes."`
}

// ImageCategoryList is the library's categories.
type ImageCategoryList struct {
	Categories []ImageCategory `json:"categories" nullable:"false" doc:"The categories, in the order they were made."`
}

// ImageCategoryInput makes a category.
type ImageCategoryInput struct {
	Name string `json:"name" minLength:"1" maxLength:"100" doc:"What staff call it."`
	Dir  string `json:"dir,omitempty" maxLength:"100" doc:"Its folder name, normalized to lowercase letters, digits and underscores. Left out, it is made from the name."`
}

// ImageCategoryChanges are the edits to a category; what is left out stays.
type ImageCategoryChanges struct {
	Name *string `json:"name,omitempty" minLength:"1" maxLength:"100" doc:"A new name."`
	Dir  *string `json:"dir,omitempty" maxLength:"100" doc:"A new folder name; the folder is moved. Left out or blank, the folder stays, so every stored reference to a picture in it keeps working."`
}

// Image is a picture in a category.
type Image struct {
	Name     string `json:"name" doc:"The file's name."`
	URL      string `json:"url" doc:"Its absolute address, which a Discord embed needs."`
	Path     string `json:"path" doc:"Its address relative to the site's root: images/<dir>/<name>."`
	Size     int64  `json:"size" doc:"Its size in bytes."`
	Modified string `json:"modified" format:"date-time" doc:"When it was last changed, RFC 3339 in UTC."`
}

// ImageList is a category's pictures.
type ImageList struct {
	Dir    string  `json:"dir" doc:"The category's folder name."`
	Images []Image `json:"images" nullable:"false" doc:"Its pictures, newest first."`
}

// ImageUpload says which files of an upload were saved.
type ImageUpload struct {
	Uploaded []Image         `json:"uploaded" nullable:"false" doc:"The pictures saved."`
	Skipped  []SkippedUpload `json:"skipped" nullable:"false" doc:"The files that were not, each with why."`
}

// ImageUploadForm is the files of an upload to the library.
type ImageUploadForm struct {
	Files []huma.FormFile `form:"files" required:"false" contentType:"application/octet-stream" doc:"One to 50 pictures: JPEG, PNG, GIF, WebP or SVG, each judged from its bytes. A picture whose name is already in the category replaces it. A JPEG or PNG is saved again without its metadata (location, camera, text), a JPEG turned upright by its orientation."`
}

type imageCategoryInput struct {
	Dir string `path:"dir" doc:"The category's folder name."`
}

type imageCategoryCreateInput struct{ Body ImageCategoryInput }

type imageCategoryUpdateInput struct {
	Dir  string `path:"dir" doc:"The category's folder name."`
	Body ImageCategoryChanges
}

type imageUploadInput struct {
	Dir     string `path:"dir" doc:"The category's folder name."`
	RawBody huma.MultipartFormFiles[ImageUploadForm]
}

type imageDeleteInput struct {
	Dir  string `path:"dir" doc:"The category's folder name."`
	Name string `path:"name" doc:"The picture's file name."`
}

type imageCategoryListOutput struct{ Body ImageCategoryList }

type imageCategoryOutput struct{ Body ImageCategory }

type imageCategoryCreatedOutput struct {
	Status int
	Body   ImageCategory
}

type imageListOutput struct{ Body ImageList }

type imageUploadOutput struct {
	Status int
	Body   ImageUpload
}

func (h handlers) registerImages(api huma.API) {
	readers := huma.Middlewares{h.permitted(api, permissions.ImageBrowsers()...)}
	writers := huma.Middlewares{h.permitted(api, permissions.SystemImages)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	notFound := slices.Concat(guarded, []int{http.StatusNotFound})
	changes := slices.Concat(notFound, []int{http.StatusConflict, http.StatusUnprocessableEntity})

	huma.Register(api, huma.Operation{
		OperationID: "list-image-categories", Method: http.MethodGet, Path: "/image-categories",
		Summary: "The image library's categories", Tags: imageTags, Security: secured, Middlewares: readers, Errors: guarded,
	}, h.listImageCategories)
	huma.Register(api, huma.Operation{
		OperationID: "create-image-category", Method: http.MethodPost, Path: "/image-categories",
		Summary:     "Make a category",
		Description: "The folders bookclub, announcements and ema_plaques belong to other pages and can't be made.",
		Tags:        imageTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: writers,
		Errors: slices.Concat(guarded, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.createImageCategory)
	huma.Register(api, huma.Operation{
		OperationID: "update-image-category", Method: http.MethodPatch, Path: "/image-categories/{dir}",
		Summary: "Rename a category or move its folder", Tags: imageTags, Security: secured, Middlewares: writers, Errors: changes,
	}, h.updateImageCategory)
	huma.Register(api, huma.Operation{
		OperationID: "delete-image-category", Method: http.MethodDelete, Path: "/image-categories/{dir}",
		Summary: "Delete a category", Description: "Its folder and every picture in it are deleted.",
		Tags: imageTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: writers, Errors: notFound,
	}, h.deleteImageCategory)
	huma.Register(api, huma.Operation{
		OperationID: "list-images", Method: http.MethodGet, Path: "/image-categories/{dir}/images",
		Summary: "A category's pictures", Tags: imageTags, Security: secured, Middlewares: readers, Errors: notFound,
	}, h.listImages)
	huma.Register(api, huma.Operation{
		OperationID: "upload-images", Method: http.MethodPost, Path: "/image-categories/{dir}/images",
		Summary: "Add pictures to a category",
		Description: "Each file is judged on its own, so a batch with some bad files still saves the good ones; the answer " +
			"lists both. If none could be saved the answer is a 422 naming each file. A picture over 40 megapixels or " +
			"16384 pixels on a side is refused.",
		Tags: imageTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: writers, MaxBodyBytes: maxUploadBody,
		Errors: slices.Concat(notFound, []int{http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity}),
	}, h.uploadImages)
	huma.Register(api, huma.Operation{
		OperationID: "delete-image", Method: http.MethodDelete, Path: "/image-categories/{dir}/images/{name}",
		Summary: "Delete a picture", Tags: imageTags, DefaultStatus: http.StatusNoContent,
		Security: secured, Middlewares: writers, Errors: notFound,
	}, h.deleteImage)
}

func toImageCategory(c media.Category) ImageCategory {
	return ImageCategory{Name: c.Name, Dir: c.Dir, FileCount: c.FileCount, TotalSize: c.TotalSize}
}

// imageURL is a picture's absolute address, its name path-escaped so one with spaces
// or non-ASCII letters is a well-formed address.
func imageURL(site, dir, name string) string {
	return site + "/images/" + dir + "/" + url.PathEscape(name)
}

func toImage(site, dir, name string, size int64, modified time.Time) Image {
	return Image{
		Name: name, URL: imageURL(site, dir, name), Path: "images/" + dir + "/" + name,
		Size: size, Modified: modified.UTC().Format(time.RFC3339),
	}
}

func (h handlers) listImageCategories(_ context.Context, _ *struct{}) (*imageCategoryListOutput, error) {
	cats, err := h.Images.Categories()
	if err != nil {
		return nil, h.fail("list image categories", err)
	}
	out := ImageCategoryList{Categories: make([]ImageCategory, 0, len(cats))}
	for _, c := range cats {
		out.Categories = append(out.Categories, toImageCategory(c))
	}
	return &imageCategoryListOutput{Body: out}, nil
}

func (h handlers) createImageCategory(_ context.Context, in *imageCategoryCreateInput) (*imageCategoryCreatedOutput, error) {
	cat, err := h.Images.CreateCategory(in.Body.Name, in.Body.Dir)
	if err != nil {
		return nil, h.mediaProblem("create an image category", "body", err)
	}
	return &imageCategoryCreatedOutput{Status: http.StatusCreated, Body: toImageCategory(cat)}, nil
}

func (h handlers) updateImageCategory(_ context.Context, in *imageCategoryUpdateInput) (*imageCategoryOutput, error) {
	if in.Body.Name == nil && in.Body.Dir == nil {
		return nil, fieldProblem("body", nil, "Send a name or a folder to change.")
	}
	cat, err := h.Images.UpdateCategory(in.Dir, in.Body.Name, in.Body.Dir)
	if err != nil {
		return nil, h.mediaProblem("update an image category", "body", err)
	}
	return &imageCategoryOutput{Body: toImageCategory(cat)}, nil
}

func (h handlers) deleteImageCategory(_ context.Context, in *imageCategoryInput) (*struct{}, error) {
	if err := h.Images.DeleteCategory(in.Dir); err != nil {
		return nil, h.mediaProblem("delete an image category", "body", err)
	}
	return nil, nil
}

func (h handlers) listImages(ctx context.Context, in *imageCategoryInput) (*imageListOutput, error) {
	list, err := h.Images.List(in.Dir)
	if err != nil {
		return nil, h.mediaProblem("list images", "path", err)
	}
	site := requestOf(ctx).Site
	out := ImageList{Dir: in.Dir, Images: make([]Image, 0, len(list))}
	for _, im := range list {
		out.Images = append(out.Images, toImage(site, in.Dir, im.Name, im.Size, im.Modified))
	}
	return &imageListOutput{Body: out}, nil
}

func (h handlers) uploadImages(ctx context.Context, in *imageUploadInput) (*imageUploadOutput, error) {
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
	if !h.Images.Known(in.Dir) {
		return nil, huma.Error404NotFound("There is no such image category.")
	}

	site := requestOf(ctx).Site
	uploaded, skipped, refusal := eachUpload(files, func(f huma.FormFile) (Image, string, string) {
		name, ok := uploads.SafeName(f.Filename, uploads.IsLibraryExt)
		if !ok {
			return Image{}, f.Filename, "Unsupported type (allowed: .jpg, .jpeg, .png, .webp, .gif, .svg)"
		}
		isSVG := strings.EqualFold(filepath.Ext(name), ".svg")
		if isSVG && f.Size > maxSVGBytes {
			return Image{}, name, "Not a valid SVG"
		}
		data, err := readUpload(f)
		if err != nil {
			return Image{}, name, "Failed to read"
		}
		var stored []byte
		if isSVG {
			// An SVG can run script and is served from this origin, so what is stored is
			// the sanitized markup, never the upload.
			clean, valid := uploads.SanitizeSVG(data)
			if !valid {
				return Image{}, name, "Not a valid SVG"
			}
			stored = clean
		} else {
			checked, reason := h.checkPicture(data)
			if reason != "" {
				return Image{}, name, reason
			}
			stored = checked
		}
		if err := h.Images.Save(in.Dir, name, stored); err != nil {
			h.Logger.Warn("save an uploaded image", "dir", in.Dir, "error", err)
			return Image{}, name, "Failed to save"
		}
		return toImage(site, in.Dir, name, int64(len(stored)), time.Now()), name, ""
	})
	if refusal != nil {
		return nil, refusal
	}
	return &imageUploadOutput{Status: http.StatusCreated, Body: ImageUpload{Uploaded: uploaded, Skipped: skipped}}, nil
}

func (h handlers) deleteImage(_ context.Context, in *imageDeleteInput) (*struct{}, error) {
	if err := h.Images.Delete(in.Dir, in.Name); err != nil {
		return nil, h.mediaProblem("delete an image", "path", err)
	}
	return nil, nil
}
