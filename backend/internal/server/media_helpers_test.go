package server_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
)

// filePart is one file of a multipart form.
type filePart struct {
	field, name string
	data        []byte
	// kind is the part's content type, as a browser labels it; the default is
	// application/octet-stream.
	kind string
}

// uploadBody builds a multipart form of text fields and file parts. Each part is
// labeled application/octet-stream, so what is judged is the bytes.
func uploadBody(fields map[string]string, parts ...filePart) (*bytes.Buffer, string) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	for _, p := range parts {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, p.field, p.name))
		kind := p.kind
		if kind == "" {
			kind = "application/octet-stream"
		}
		header.Set("Content-Type", kind)
		part, _ := mw.CreatePart(header)
		_, _ = part.Write(p.data)
	}
	_ = mw.Close()
	return &body, mw.FormDataContentType()
}

// upload posts a multipart form as the env's client.
func (e *testEnv) upload(t *testing.T, path string, fields map[string]string, parts ...filePart) *http.Response {
	t.Helper()
	return uploadAs(t, e.client, e, path, fields, parts...)
}

// uploadAs posts a multipart form as another client.
func uploadAs(t *testing.T, c *http.Client, e *testEnv, path string, fields map[string]string, parts ...filePart) *http.Response {
	t.Helper()
	body, kind := uploadBody(fields, parts...)
	resp, err := c.Post(e.url(path), kind, body)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// files names each file of a form after its field, "files".
func files(named map[string][]byte) []filePart {
	var parts []filePart
	for name, data := range named {
		parts = append(parts, filePart{field: "files", name: name, data: data})
	}
	return parts
}

// realPNG is a w x h PNG.
func realPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// exifJPEG is a w x h JPEG carrying an EXIF segment with the given orientation.
func exifJPEG(t *testing.T, w, h int, orientation uint16) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: 120, B: uint8(y * 255 / h), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	var tiff bytes.Buffer
	tiff.WriteString("II")
	for _, v := range []any{uint16(42), uint32(8), uint16(1), uint16(0x0112), uint16(3), uint32(1), orientation, uint16(0), uint32(0)} {
		_ = binary.Write(&tiff, binary.LittleEndian, v)
	}
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)
	raw := buf.Bytes()
	return append(append(append([]byte{}, raw[:2]...), seg...), raw[2:]...)
}

// smallGIF is a one-frame GIF.
func smallGIF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewPaletted(image.Rect(0, 0, 3, 3), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeFile writes a file under dir, making the folders.
func writeFile(t *testing.T, dir string, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// problemErrors are the field locations a 422 problem names.
func problemLocations(t *testing.T, resp *http.Response) []string {
	t.Helper()
	var p struct {
		Errors []struct {
			Location string `json:"location"`
		} `json:"errors"`
	}
	decodeInto(t, resp, http.StatusUnprocessableEntity, &p)
	var out []string
	for _, e := range p.Errors {
		out = append(out, e.Location)
	}
	return out
}
