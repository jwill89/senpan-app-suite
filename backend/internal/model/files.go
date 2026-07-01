package model

// FontFile is the JSON shape for a single font file in the directory listing.
type FontFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // RFC3339
}

// FontsResponse is the body of GET /api/fonts.
type FontsResponse struct {
	Fonts []FontFile `json:"fonts"`
}

// FontUploadResponse is the body of POST /api/fonts/upload.
type FontUploadResponse struct {
	Uploaded []string        `json:"uploaded"`
	Skipped  []SkippedUpload `json:"skipped"`
}

// CarrdProject is the JSON shape for one project in the listing. The counts and
// size are aggregated across the whole project tree (root + nested sub-folders).
type CarrdProject struct {
	Title          string `json:"title"`
	Folder         string `json:"folder"`
	FileCount      int    `json:"file_count"`      // media files, recursive
	SubfolderCount int    `json:"subfolder_count"` // nested sub-directories, recursive
	TotalSize      int64  `json:"total_size"`      // combined size of all media files, bytes
	Modified       string `json:"modified"`        // RFC3339 (folder mod time)
}

// CarrdProjectsResponse is the body of GET /api/carrd/projects.
type CarrdProjectsResponse struct {
	Projects []CarrdProject `json:"projects"`
}

// CarrdProjectCreateResponse is the body of POST /api/carrd/projects for the
// create/rename actions — JSON: {"ok": true, "project": {...}}.
type CarrdProjectCreateResponse struct {
	OK      bool         `json:"ok"`
	Project CarrdProject `json:"project"`
}

// CarrdImage is the JSON shape for one image in a project listing.
type CarrdImage struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // RFC3339
}

// CarrdImagesResponse is the body of GET /api/carrd/images: the resolved folder
// and subpath, the immediate sub-directory names, and the images at that path.
type CarrdImagesResponse struct {
	Folder string       `json:"folder"`
	Path   string       `json:"path"`
	Dirs   []string     `json:"dirs"`
	Images []CarrdImage `json:"images"`
}

// CarrdUploadResponse is the body of POST /api/carrd/upload.
type CarrdUploadResponse struct {
	Uploaded []string        `json:"uploaded"`
	Skipped  []SkippedUpload `json:"skipped"`
}

// ImageCategory is the JSON shape for one category in the listing. FileCount and
// TotalSize are populated for listings and omitted (zero) for action responses.
type ImageCategory struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Permanent bool   `json:"permanent"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
}

// ImageCategoriesResponse is the body of GET /api/image-categories.
type ImageCategoriesResponse struct {
	Categories []ImageCategory `json:"categories"`
}

// ImageCategoryActionResponse is the body of POST /api/image-categories for the
// create/rename actions — JSON: {"ok": true, "category": {...}}.
type ImageCategoryActionResponse struct {
	OK       bool          `json:"ok"`
	Category ImageCategory `json:"category"`
}

// ImageEntry is the JSON shape for one image in a category listing. URL is the
// absolute public URL (Discord embeds require absolute URLs); Path is the
// root-relative web path (raffles store the relative path in prize_image).
type ImageEntry struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // RFC3339
}

// ImagesResponse is the body of GET /api/images: the category dir and its images.
type ImagesResponse struct {
	Dir    string       `json:"dir"`
	Images []ImageEntry `json:"images"`
}

// ImagesUploadResponse is the body of POST /api/images/upload.
type ImagesUploadResponse struct {
	Uploaded []string        `json:"uploaded"`
	Skipped  []SkippedUpload `json:"skipped"`
}
