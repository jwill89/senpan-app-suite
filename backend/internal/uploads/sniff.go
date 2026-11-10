package uploads

import "net/http"

// SniffImage names the raster image type a file's leading bytes show, as a content
// type, or "" when they show none.
func SniffImage(head []byte) string {
	if len(head) > 512 {
		head = head[:512]
	}
	switch ct := http.DetectContentType(head); ct {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return ct
	}
	return ""
}

// IsMP3 reports whether b begins like an MP3: an ID3v2 tag, or an MPEG audio frame
// sync (0xFF, then the top three bits of the next byte set).
func IsMP3(b []byte) bool {
	if len(b) >= 3 && b[0] == 'I' && b[1] == 'D' && b[2] == '3' {
		return true
	}
	return len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0
}

// IsMP4 reports whether b is an ISO base media container: a box size, "ftyp", and a
// known audio or video brand. A file that merely contains "ftyp" doesn't pass.
func IsMP4(b []byte) bool {
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		return false
	}
	switch string(b[8:12]) {
	case "isom", "iso2", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1",
		"dash", "M4V ", "M4A ", "mmp4", "MSNV", "ndsc", "ndsm":
		return true
	}
	return false
}

// CarrdMedia judges a Carrd file's leading bytes against the type its extension
// (lowercase, with its dot) claims, answering a short reason when they don't match
// and "" when they do. The files are served publicly, so the extension isn't trusted:
// an HTML or script file renamed to .png or .mp3 must not land in a public folder.
func CarrdMedia(ext string, head []byte) string {
	switch {
	case IsRasterExt(ext):
		if SniffImage(head) == "" {
			return "Not a valid image"
		}
	case ext == ".mp3":
		if !IsMP3(head) {
			return "Not a valid audio file"
		}
	case ext == ".mp4":
		if !IsMP4(head) {
			return "Not a valid video file"
		}
	}
	return ""
}
