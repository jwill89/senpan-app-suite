package model

// FontVariant is one format variant of a font: an uploaded file, or the
// auto-converted WOFF2 copy (Converted=true). Token serves this variant; it
// rotates on a schedule, so it must be re-read, never persisted.
type FontVariant struct {
	Name      string `json:"name"`      // file name (converted copies use "<base>.woff2")
	Type      string `json:"type"`      // "TTF" | "OTF" | "WOFF" | "WOFF2" | "EOT"
	Converted bool   `json:"converted"` // auto-generated WOFF2 copy, not an upload
	Size      int64  `json:"size"`
	Modified  string `json:"modified"` // RFC3339 ("" for converted copies)
	Token     string `json:"token"`
}

// Font is one logical font in GET /api/fonts: all uploaded files sharing a
// base name (plus the converted WOFF2 copy) grouped as variants. Base is the
// group's identity for the /api/fonts/families/{base} endpoints. Family is the
// effective CSS family name (admin-customizable; defaults to Base). Serve is
// the admin's chosen served variant type ("" = auto: WOFF2 when available);
// ServedType/ServedToken describe the variant actually served publicly.
// Origins is THIS font's external-site allowlist (the app's own origin is
// always allowed and never listed).
type Font struct {
	Base        string        `json:"base"`
	Family      string        `json:"family"`
	Serve       string        `json:"serve"`
	ServedType  string        `json:"served_type"`
	ServedToken string        `json:"served_token"`
	Origins     []string      `json:"origins"`
	Modified    string        `json:"modified"` // newest variant, RFC3339
	Variants    []FontVariant `json:"variants"`
}

// FontsResponse is the body of GET /api/fonts.
type FontsResponse struct {
	Fonts []Font `json:"fonts"`
}

// FontUploadResponse is the body of POST /api/fonts/upload. Warnings covers
// files that uploaded fine but whose WOFF2 conversion failed (the original is
// served for those).
type FontUploadResponse struct {
	Uploaded []string        `json:"uploaded"`
	Skipped  []SkippedUpload `json:"skipped"`
	Warnings []string        `json:"warnings,omitempty"`
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
