package uploads

import "testing"

func TestIsMP3(t *testing.T) {
	for in, want := range map[string]bool{
		"ID3\x04\x00":      true,
		"\xff\xfb\x90\x00": true, // an MPEG frame sync
		"\xff\x10":         false,
		"<html>":           false,
		"":                 false,
		"RIFF....WAVEfmt ": false, //nolint:gocritic // the trailing space is part of the chunk id
	} {
		if got := IsMP3([]byte(in)); got != want {
			t.Errorf("IsMP3(%q) = %v; want %v", in, got, want)
		}
	}
}

func TestIsMP4(t *testing.T) {
	for in, want := range map[string]bool{
		"\x00\x00\x00\x18ftypmp42": true,
		"\x00\x00\x00\x20ftypisom": true,
		"\x00\x00\x00\x18ftypxxxx": false, // an unknown brand
		"\x00\x00\x00\x18moovmp42": false, // no ftyp box
		"ftyp":                     false,
		"<html><!-- ftyp -->":      false,
	} {
		if got := IsMP4([]byte(in)); got != want {
			t.Errorf("IsMP4(%q) = %v; want %v", in, got, want)
		}
	}
}

func TestCarrdMedia(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	for _, tc := range []struct {
		ext  string
		head []byte
		want string
	}{
		{".png", png, ""},
		{".png", []byte("<html>"), "Not a valid image"},
		{".mp3", []byte("ID3\x03"), ""},
		{".mp3", png, "Not a valid audio file"},
		{".mp4", []byte("\x00\x00\x00\x18ftypmp42"), ""},
		{".mp4", []byte("ID3\x03"), "Not a valid video file"},
	} {
		if got := CarrdMedia(tc.ext, tc.head); got != tc.want {
			t.Errorf("CarrdMedia(%s, %q) = %q; want %q", tc.ext, tc.head, got, tc.want)
		}
	}
}
