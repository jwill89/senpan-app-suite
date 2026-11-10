package api

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"app-suite/internal/media"
	"app-suite/internal/permissions"
	"app-suite/internal/uploads"
)

// Carrd projects: folders of pictures, audio and video that outside Carrd sites embed
// from the carrd host. A project has a title apart from its folder name.

var carrdTags = []string{"Carrd"}

// CarrdProject is a Carrd project with counts across its whole tree.
type CarrdProject struct {
	Title          string `json:"title" doc:"What staff call it; unique among projects."`
	Folder         string `json:"folder" doc:"Its folder name, which is in the address of every file: lowercase letters, digits and hyphens."`
	FileCount      int    `json:"fileCount" doc:"The media files in it, in sub-folders too."`
	SubfolderCount int    `json:"subfolderCount" doc:"The sub-folders in it, nested ones too."`
	TotalSize      int64  `json:"totalSize" doc:"The media files' combined size in bytes."`
	Modified       string `json:"modified" format:"date-time" doc:"When the folder last changed, RFC 3339 in UTC."`
}

// CarrdProjectList is the Carrd projects.
type CarrdProjectList struct {
	Projects []CarrdProject `json:"projects" nullable:"false" doc:"The projects, by title."`
}

// CarrdProjectInput makes a project.
type CarrdProjectInput struct {
	Title  string `json:"title" minLength:"1" maxLength:"200" doc:"What staff call it; unique among projects."`
	Folder string `json:"folder,omitempty" maxLength:"100" doc:"Its folder name, normalized to lowercase letters, digits and hyphens. Left out, it is made from the title."`
}

// CarrdProjectChanges are the edits to a project; what is left out or blank stays.
type CarrdProjectChanges struct {
	Title  *string `json:"title,omitempty" maxLength:"200" doc:"A new title."`
	Folder *string `json:"folder,omitempty" maxLength:"100" doc:"A new folder name; the folder is renamed, which changes every file's address."`
}

// CarrdFile is a media file in a project.
type CarrdFile struct {
	Name     string `json:"name" doc:"The file's name."`
	Size     int64  `json:"size" doc:"Its size in bytes."`
	Modified string `json:"modified" format:"date-time" doc:"When it last changed, RFC 3339 in UTC."`
}

// CarrdFolder is what one folder of a project holds.
type CarrdFolder struct {
	Folder  string      `json:"folder" doc:"The project's folder name."`
	Path    string      `json:"path" doc:"The folder within the project, empty for its root."`
	Folders []string    `json:"folders" nullable:"false" doc:"Its sub-folders' names, by name."`
	Files   []CarrdFile `json:"files" nullable:"false" doc:"Its media files, by name."`
}

// CarrdSubfolderInput makes a sub-folder.
type CarrdSubfolderInput struct {
	Path string `json:"path,omitempty" maxLength:"500" doc:"The folder to make it in, within the project; empty for the project's root."`
	Name string `json:"name" minLength:"1" maxLength:"100" doc:"Its name, normalized to lowercase letters, digits and hyphens."`
}

// CarrdSubfolder is a sub-folder that was made.
type CarrdSubfolder struct {
	Name string `json:"name" doc:"The name it got."`
}

// CarrdUpload says which files of an upload were saved.
type CarrdUpload struct {
	Uploaded []string        `json:"uploaded" nullable:"false" doc:"The names saved."`
	Skipped  []SkippedUpload `json:"skipped" nullable:"false" doc:"The files that were not, each with why."`
}

// CarrdUploadForm is the files of an upload to a project folder.
type CarrdUploadForm struct {
	Path  string          `form:"path" required:"false" maxLength:"500" doc:"The folder to save into, within the project; empty for its root. It must exist."`
	Files []huma.FormFile `form:"files" required:"false" contentType:"application/octet-stream" doc:"One to 50 files: JPEG, PNG, GIF or WebP pictures, MP3 audio or MP4 video, each judged from its bytes. A file whose name is already there replaces it. A JPEG or PNG is saved again without its metadata."`
}

type carrdProjectInput struct {
	Folder string `path:"folder" doc:"The project's folder name."`
}

type carrdProjectCreateInput struct{ Body CarrdProjectInput }

type carrdProjectUpdateInput struct {
	Folder string `path:"folder" doc:"The project's folder name."`
	Body   CarrdProjectChanges
}

type carrdFilesInput struct {
	Folder string `path:"folder" doc:"The project's folder name."`
	Path   string `query:"path" required:"false" maxLength:"500" doc:"The folder within the project; empty for its root."`
}

type carrdUploadInput struct {
	Folder  string `path:"folder" doc:"The project's folder name."`
	RawBody huma.MultipartFormFiles[CarrdUploadForm]
}

type carrdFileDeleteInput struct {
	Folder string `path:"folder" doc:"The project's folder name."`
	Path   string `query:"path" required:"false" maxLength:"500" doc:"The folder within the project; empty for its root."`
	Name   string `query:"name" minLength:"1" maxLength:"255" doc:"The file's name."`
}

type carrdFolderCreateInput struct {
	Folder string `path:"folder" doc:"The project's folder name."`
	Body   CarrdSubfolderInput
}

type carrdFolderDeleteInput struct {
	Folder string `path:"folder" doc:"The project's folder name."`
	Path   string `query:"path" required:"false" maxLength:"500" doc:"The sub-folder within the project. The project's root can't be deleted here."`
}

type carrdProjectListOutput struct{ Body CarrdProjectList }

type carrdProjectOutput struct{ Body CarrdProject }

type carrdProjectCreatedOutput struct {
	Status int
	Body   CarrdProject
}

type carrdFolderOutput struct{ Body CarrdFolder }

type carrdUploadOutput struct {
	Status int
	Body   CarrdUpload
}

type carrdSubfolderOutput struct {
	Status int
	Body   CarrdSubfolder
}

func (h handlers) registerCarrd(api huma.API) {
	page := huma.Middlewares{h.permitted(api, permissions.AtelierCarrd)}
	guarded := []int{http.StatusUnauthorized, http.StatusForbidden}
	notFound := slices.Concat(guarded, []int{http.StatusNotFound})
	invalid := slices.Concat(notFound, []int{http.StatusUnprocessableEntity})

	huma.Register(api, huma.Operation{
		OperationID: "list-carrd-projects", Method: http.MethodGet, Path: "/carrd-projects",
		Summary: "The Carrd projects", Tags: carrdTags, Security: secured, Middlewares: page, Errors: guarded,
	}, h.listCarrdProjects)
	huma.Register(api, huma.Operation{
		OperationID: "create-carrd-project", Method: http.MethodPost, Path: "/carrd-projects",
		Summary: "Make a project", Tags: carrdTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: slices.Concat(guarded, []int{http.StatusConflict, http.StatusUnprocessableEntity}),
	}, h.createCarrdProject)
	huma.Register(api, huma.Operation{
		OperationID: "update-carrd-project", Method: http.MethodPatch, Path: "/carrd-projects/{folder}",
		Summary: "Retitle a project or rename its folder", Tags: carrdTags, Security: secured, Middlewares: page,
		Errors: slices.Concat(invalid, []int{http.StatusConflict}),
	}, h.updateCarrdProject)
	huma.Register(api, huma.Operation{
		OperationID: "delete-carrd-project", Method: http.MethodDelete, Path: "/carrd-projects/{folder}",
		Summary: "Delete a project", Description: "Its folder and everything in it are deleted.",
		Tags: carrdTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: notFound,
	}, h.deleteCarrdProject)
	huma.Register(api, huma.Operation{
		OperationID: "list-carrd-files", Method: http.MethodGet, Path: "/carrd-projects/{folder}/files",
		Summary: "A folder of a project", Tags: carrdTags, Security: secured, Middlewares: page, Errors: invalid,
	}, h.listCarrdFiles)
	huma.Register(api, huma.Operation{
		OperationID: "upload-carrd-files", Method: http.MethodPost, Path: "/carrd-projects/{folder}/files",
		Summary: "Add files to a project folder",
		Description: "Each file is judged on its own, so a batch with some bad files still saves the good ones; the answer " +
			"lists both. If none could be saved the answer is a 422 naming each file.",
		Tags: carrdTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page, MaxBodyBytes: maxUploadBody,
		Errors: slices.Concat(invalid, []int{http.StatusRequestEntityTooLarge}),
	}, h.uploadCarrdFiles)
	huma.Register(api, huma.Operation{
		OperationID: "delete-carrd-file", Method: http.MethodDelete, Path: "/carrd-projects/{folder}/files",
		Summary: "Delete a file", Tags: carrdTags, DefaultStatus: http.StatusNoContent,
		Security: secured, Middlewares: page, Errors: invalid,
	}, h.deleteCarrdFile)
	huma.Register(api, huma.Operation{
		OperationID: "create-carrd-folder", Method: http.MethodPost, Path: "/carrd-projects/{folder}/folders",
		Summary: "Make a sub-folder", Tags: carrdTags, DefaultStatus: http.StatusCreated, Security: secured, Middlewares: page,
		Errors: slices.Concat(invalid, []int{http.StatusConflict}),
	}, h.createCarrdFolder)
	huma.Register(api, huma.Operation{
		OperationID: "delete-carrd-folder", Method: http.MethodDelete, Path: "/carrd-projects/{folder}/folders",
		Summary: "Delete a sub-folder", Description: "It and everything in it are deleted. A project's root is deleted with the project.",
		Tags: carrdTags, DefaultStatus: http.StatusNoContent, Security: secured, Middlewares: page, Errors: invalid,
	}, h.deleteCarrdFolder)
}

func toCarrdProject(p media.Project) CarrdProject {
	return CarrdProject{
		Title: p.Title, Folder: p.Folder, FileCount: p.FileCount, SubfolderCount: p.SubfolderCount,
		TotalSize: p.TotalSize, Modified: p.Modified.UTC().Format(time.RFC3339),
	}
}

func (h handlers) listCarrdProjects(_ context.Context, _ *struct{}) (*carrdProjectListOutput, error) {
	projects, err := h.Carrd.Projects()
	if err != nil {
		return nil, h.fail("list carrd projects", err)
	}
	out := CarrdProjectList{Projects: make([]CarrdProject, 0, len(projects))}
	for _, p := range projects {
		out.Projects = append(out.Projects, toCarrdProject(p))
	}
	return &carrdProjectListOutput{Body: out}, nil
}

func (h handlers) createCarrdProject(_ context.Context, in *carrdProjectCreateInput) (*carrdProjectCreatedOutput, error) {
	p, err := h.Carrd.CreateProject(in.Body.Title, in.Body.Folder)
	if err != nil {
		return nil, h.mediaProblem("create a carrd project", "body", err)
	}
	return &carrdProjectCreatedOutput{Status: http.StatusCreated, Body: toCarrdProject(p)}, nil
}

func (h handlers) updateCarrdProject(_ context.Context, in *carrdProjectUpdateInput) (*carrdProjectOutput, error) {
	if in.Body.Title == nil && in.Body.Folder == nil {
		return nil, fieldProblem("body", nil, "Send a title or a folder to change.")
	}
	p, err := h.Carrd.UpdateProject(in.Folder, in.Body.Title, in.Body.Folder)
	if err != nil {
		return nil, h.mediaProblem("update a carrd project", "body", err)
	}
	return &carrdProjectOutput{Body: toCarrdProject(p)}, nil
}

func (h handlers) deleteCarrdProject(_ context.Context, in *carrdProjectInput) (*struct{}, error) {
	if err := h.Carrd.DeleteProject(in.Folder); err != nil {
		return nil, h.mediaProblem("delete a carrd project", "path", err)
	}
	return nil, nil
}

func (h handlers) listCarrdFiles(_ context.Context, in *carrdFilesInput) (*carrdFolderOutput, error) {
	l, err := h.Carrd.Folder(in.Folder, in.Path)
	if err != nil {
		return nil, h.mediaProblem("list carrd files", "query", err)
	}
	out := CarrdFolder{Folder: l.Folder, Path: l.Path, Folders: l.Folders, Files: make([]CarrdFile, 0, len(l.Files))}
	for _, f := range l.Files {
		out.Files = append(out.Files, CarrdFile{Name: f.Name, Size: f.Size, Modified: f.Modified.UTC().Format(time.RFC3339)})
	}
	return &carrdFolderOutput{Body: out}, nil
}

func (h handlers) uploadCarrdFiles(_ context.Context, in *carrdUploadInput) (*carrdUploadOutput, error) {
	form := in.RawBody.Data()
	defer func() {
		for _, f := range form.Files {
			if f.File != nil {
				_ = f.Close() // closed again by eachUpload for the files it reached; nothing to report
			}
		}
	}()
	if err := checkUploadCount(len(form.Files)); err != nil {
		return nil, err
	}
	dest, err := h.Carrd.Dest(in.Folder, form.Path)
	if err != nil {
		return nil, h.mediaProblem("find a carrd folder", "body", err)
	}

	uploaded, skipped, refusal := eachUpload(form.Files, func(f huma.FormFile) (string, string, string) {
		name, ok := uploads.SafeName(f.Filename, uploads.IsCarrdExt)
		if !ok {
			return "", f.Filename, "Unsupported type (allowed: .jpg, .jpeg, .png, .webp, .gif, .mp3, .mp4)"
		}
		ext := strings.ToLower(filepath.Ext(name))
		var stored []byte
		if uploads.IsRasterExt(ext) {
			data, err := readUpload(f)
			if err != nil {
				return "", name, "Failed to read"
			}
			checked, reason := h.checkPicture(data)
			if reason != "" {
				return "", name, reason
			}
			stored = checked
		} else {
			// Audio and video are served as they came, once the bytes show the format
			// their name claims.
			data, err := io.ReadAll(f.File)
			if err != nil {
				return "", name, "Failed to read"
			}
			head := data[:min(len(data), 512)]
			if reason := uploads.CarrdMedia(ext, head); reason != "" {
				return "", name, reason
			}
			stored = data
		}
		if err := h.Carrd.Save(dest, name, stored); err != nil {
			h.Logger.Warn("save a carrd file", "error", err)
			return "", name, "Failed to save"
		}
		return name, name, ""
	})
	if refusal != nil {
		return nil, refusal
	}
	return &carrdUploadOutput{Status: http.StatusCreated, Body: CarrdUpload{Uploaded: uploaded, Skipped: skipped}}, nil
}

func (h handlers) deleteCarrdFile(_ context.Context, in *carrdFileDeleteInput) (*struct{}, error) {
	if err := h.Carrd.DeleteFile(in.Folder, in.Path, in.Name); err != nil {
		return nil, h.mediaProblem("delete a carrd file", "query", err)
	}
	return nil, nil
}

func (h handlers) createCarrdFolder(_ context.Context, in *carrdFolderCreateInput) (*carrdSubfolderOutput, error) {
	name, err := h.Carrd.CreateFolder(in.Folder, in.Body.Path, in.Body.Name)
	if err != nil {
		return nil, h.mediaProblem("create a carrd folder", "body", err)
	}
	return &carrdSubfolderOutput{Status: http.StatusCreated, Body: CarrdSubfolder{Name: name}}, nil
}

func (h handlers) deleteCarrdFolder(_ context.Context, in *carrdFolderDeleteInput) (*struct{}, error) {
	if err := h.Carrd.DeleteFolder(in.Folder, in.Path); err != nil {
		return nil, h.mediaProblem("delete a carrd folder", "query", err)
	}
	return nil, nil
}
