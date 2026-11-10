package uploads

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

// exifSegment is an APP1 segment holding one orientation entry in the given byte order.
func exifSegment(order binary.ByteOrder, tag string, orientation uint16) []byte {
	var tiff bytes.Buffer
	tiff.WriteString(tag)
	_ = binary.Write(&tiff, order, uint16(42))
	_ = binary.Write(&tiff, order, uint32(8))
	_ = binary.Write(&tiff, order, uint16(1)) // one entry
	_ = binary.Write(&tiff, order, uint16(0x0112))
	_ = binary.Write(&tiff, order, uint16(3)) // SHORT
	_ = binary.Write(&tiff, order, uint32(1))
	_ = binary.Write(&tiff, order, orientation)
	_ = binary.Write(&tiff, order, uint16(0))
	_ = binary.Write(&tiff, order, uint32(0)) // no next directory

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	return append(seg, payload...)
}

// jpegWith is a w x h JPEG with segment slipped in right after its start marker.
func jpegWith(t *testing.T, w, h int, segment []byte) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	return append(append(append([]byte{}, raw[:2]...), segment...), raw[2:]...)
}

func TestJPEGOrientation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order binary.ByteOrder
		tag   string
		value uint16
		want  int
	}{
		{"little endian", binary.LittleEndian, "II", 6, 6},
		{"big endian", binary.BigEndian, "MM", 8, 8},
		{"upright", binary.LittleEndian, "II", 1, 1},
		{"out of range", binary.LittleEndian, "II", 9, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := jpegWith(t, 8, 4, exifSegment(tc.order, tc.tag, tc.value))
			if got := jpegOrientation(data); got != tc.want {
				t.Errorf("orientation = %d; want %d", got, tc.want)
			}
		})
	}
	if got := jpegOrientation(jpegWith(t, 8, 4, nil)); got != 1 {
		t.Errorf("no EXIF: orientation = %d; want 1", got)
	}
	for _, junk := range [][]byte{nil, {0xFF}, {0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF}, []byte("not a jpeg")} {
		if got := jpegOrientation(junk); got != 1 {
			t.Errorf("orientation of %q = %d; want 1", junk, got)
		}
	}
}

func TestOrientMovesPixels(t *testing.T) {
	// A 3x2 image with a distinct red pixel at the top left.
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})
	for _, tc := range []struct {
		o        int
		w, h     int
		x, y     int // where the red pixel ends up
		describe string
	}{
		{2, 3, 2, 2, 0, "mirrored"},
		{3, 3, 2, 2, 1, "rotated half a turn"},
		{4, 3, 2, 0, 1, "flipped vertically"},
		{5, 2, 3, 0, 0, "transposed"},
		{6, 2, 3, 1, 0, "rotated a quarter clockwise"},
		{7, 2, 3, 1, 2, "transversed"},
		{8, 2, 3, 0, 2, "rotated a quarter counterclockwise"},
	} {
		got := orient(src, tc.o)
		b := got.Bounds()
		if b.Dx() != tc.w || b.Dy() != tc.h {
			t.Errorf("orientation %d (%s): size %dx%d; want %dx%d", tc.o, tc.describe, b.Dx(), b.Dy(), tc.w, tc.h)
			continue
		}
		if r, _, _, _ := got.At(tc.x, tc.y).RGBA(); r == 0 {
			t.Errorf("orientation %d (%s): red pixel isn't at (%d,%d)", tc.o, tc.describe, tc.x, tc.y)
		}
	}
}

func TestCheckImageJPEGUprightsAndDropsMetadata(t *testing.T) {
	data := jpegWith(t, 8, 4, exifSegment(binary.LittleEndian, "II", 6))
	got, err := CheckImage(data, ImageRule{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "image/jpeg" || got.Width != 4 || got.Height != 8 {
		t.Errorf("got %s %dx%d; want a 4x8 JPEG (turned upright)", got.Type, got.Width, got.Height)
	}
	if bytes.Contains(got.Data, []byte("Exif")) {
		t.Error("the re-encoded JPEG still carries its EXIF")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(got.Data))
	if err != nil || cfg.Width != 4 || cfg.Height != 8 {
		t.Errorf("stored JPEG = %+v, %v; want 4x8", cfg, err)
	}
}

func TestCheckImagePNGDropsTextChunks(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 5, 5))); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	// A tEXt chunk after the signature and IHDR (8 + 25 bytes).
	text := []byte("Comment\x00GPS 51.5N")
	chunk := make([]byte, 8, 8+len(text)+4)
	binary.BigEndian.PutUint32(chunk, uint32(len(text)))
	copy(chunk[4:], "tEXt")
	chunk = append(chunk, text...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	withText := append(append(append([]byte{}, raw[:33]...), chunk...), raw[33:]...)
	if !bytes.Contains(withText, []byte("GPS")) {
		t.Fatal("test setup lost the text chunk")
	}

	got, err := CheckImage(withText, ImageRule{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "image/png" || bytes.Contains(got.Data, []byte("GPS")) {
		t.Errorf("type %s, text chunk kept = %v; want a PNG without it", got.Type, bytes.Contains(got.Data, []byte("GPS")))
	}
}

func TestCheckImageRefusesWhatIsntAPicture(t *testing.T) {
	for name, data := range map[string][]byte{
		"html":       []byte("<html><script>alert(1)</script></html>"),
		"empty":      nil,
		"png header": []byte("\x89PNG\r\n\x1a\n and then nothing"),
		"svg":        []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"),
	} {
		if _, err := CheckImage(data, ImageRule{}); !errors.Is(err, ErrNotImage) {
			t.Errorf("%s: err = %v; want ErrNotImage", name, err)
		}
	}
}

func TestCheckImageRule(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 30, 10))); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckImage(buf.Bytes(), ImageRule{Types: []string{"image/png"}, MaxSide: 30}); err != nil {
		t.Errorf("a PNG at the side limit: %v", err)
	}
	if _, err := CheckImage(buf.Bytes(), ImageRule{MaxSide: 29}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a PNG over the side limit: err = %v; want ErrTooLarge", err)
	}
	if _, err := CheckImage(jpegWith(t, 8, 4, nil), ImageRule{Types: []string{"image/png"}}); !errors.Is(err, ErrNotImage) {
		t.Errorf("a JPEG where only PNG is taken: err = %v; want ErrNotImage", err)
	}
}

func TestCheckImageTooManyPixels(t *testing.T) {
	// 7000 x 7000 is 49 megapixels: refused from the header, before a pixel is decoded.
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, image.NewGray(image.Rect(0, 0, 7000, 7000))); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckImage(buf.Bytes(), ImageRule{}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v; want ErrTooLarge", err)
	}
}

func animatedGIF(frames int) []byte {
	g := &gif.GIF{}
	pal := color.Palette{color.Black, color.White}
	for range frames {
		g.Image = append(g.Image, image.NewPaletted(image.Rect(0, 0, 4, 4), pal))
		g.Delay = append(g.Delay, 5)
	}
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, g)
	return buf.Bytes()
}

func TestCheckImageGIFKeptAsSent(t *testing.T) {
	data := animatedGIF(3)
	got, err := CheckImage(data, ImageRule{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "image/gif" || !bytes.Equal(got.Data, data) {
		t.Error("a GIF should be kept byte for byte so its animation survives")
	}
	if n, _, ok := gifFrames(data); !ok || n != 3 {
		t.Errorf("gifFrames = %d, %v; want 3 frames", n, ok)
	}
	if _, err := CheckImage(animatedGIF(MaxGIFFrames+1), ImageRule{}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a GIF over the frame limit: err = %v; want ErrTooLarge", err)
	}
	if _, err := CheckImage(data[:len(data)-8], ImageRule{}); !errors.Is(err, ErrNotImage) {
		t.Errorf("a cut-off GIF: err = %v; want ErrNotImage", err)
	}
}

func TestCheckImageWebPKeptAsSent(t *testing.T) {
	// A 1x1 lossless WebP.
	data, err := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if err != nil {
		t.Fatal(err)
	}
	got, err := CheckImage(data, ImageRule{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "image/webp" || !bytes.Equal(got.Data, data) || got.Width != 1 {
		t.Errorf("got %s width %d; want the WebP as sent", got.Type, got.Width)
	}
}
