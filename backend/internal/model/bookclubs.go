package model

// BookclubUploadResponse is the shared single-image upload response (book-club
// cover uploads via saveSingleImageUpload) - the full URL of the stored image.
// JSON: {"url": <fullURL>}.
type BookclubUploadResponse struct {
	URL string `json:"url"`
}
