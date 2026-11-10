package uploads

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"slices"

	"golang.org/x/image/webp"
)

// The limits every picture is held to before it is decoded in full.
const (
	// MaxPixels is the most pixels a picture may have (40 megapixels).
	MaxPixels = 40_000_000
	// MaxSide is the longest side a picture may have, in pixels.
	MaxSide = 16384
	// MaxGIFFrames is the most frames an animated GIF may have.
	MaxGIFFrames = 500
	// maxGIFPixels bounds the pixels all of a GIF's frames add up to, which is what
	// decoding it in full allocates.
	maxGIFPixels = 150_000_000
	// jpegQuality is the quality a JPEG is saved at again.
	jpegQuality = 90
)

// Errors an upload fails with.
var (
	// ErrNotImage means the bytes aren't a picture of an accepted type, or can't be
	// decoded.
	ErrNotImage = errors.New("not a valid image")
	// ErrTooLarge means the picture has too many pixels or a side that is too long.
	ErrTooLarge = errors.New("image is too large")
)

// ImageRule narrows what CheckImage accepts.
type ImageRule struct {
	// Types are the content types accepted; none means JPEG, PNG, GIF and WebP.
	Types []string
	// MaxSide is the longest side accepted; 0 means the package's MaxSide.
	MaxSide int
}

// Checked is a picture that passed the rule, as it should be stored.
type Checked struct {
	// Data is what to store: a JPEG or PNG decoded and encoded again, which drops its
	// metadata (EXIF, GPS, text chunks); a GIF or WebP as it was sent.
	Data []byte
	// Type is its content type, judged from its bytes.
	Type string
	// Width and Height are its size in pixels.
	Width, Height int
}

// CheckImage judges data from its bytes and checks it in full. Its size is read first
// and held to the limits before any pixel is decoded. A JPEG is decoded, turned
// upright by its EXIF orientation and encoded again at quality 90; a PNG is decoded
// and encoded again (an animated PNG keeps its first frame); a GIF is decoded to
// check every frame and kept; a WebP's header is read and kept, since Go writes no
// WebP.
func CheckImage(data []byte, rule ImageRule) (Checked, error) {
	typ := SniffImage(data)
	if typ == "" || (len(rule.Types) > 0 && !slices.Contains(rule.Types, typ)) {
		return Checked{}, ErrNotImage
	}
	side := rule.MaxSide
	if side <= 0 {
		side = MaxSide
	}

	var cfg image.Config
	var err error
	switch typ {
	case "image/jpeg":
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	case "image/png":
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	case "image/gif":
		cfg, err = gif.DecodeConfig(bytes.NewReader(data))
	case "image/webp":
		cfg, err = webp.DecodeConfig(bytes.NewReader(data))
	}
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return Checked{}, ErrNotImage
	}
	if cfg.Width > side || cfg.Height > side || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return Checked{}, ErrTooLarge
	}
	out := Checked{Data: data, Type: typ, Width: cfg.Width, Height: cfg.Height}

	switch typ {
	case "image/jpeg":
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return Checked{}, ErrNotImage
		}
		if o := jpegOrientation(data); o > 1 {
			img = orient(img, o)
			b := img.Bounds()
			out.Width, out.Height = b.Dx(), b.Dy()
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return Checked{}, fmt.Errorf("encode jpeg: %w", err)
		}
		out.Data = buf.Bytes()
	case "image/png":
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return Checked{}, ErrNotImage
		}
		var buf bytes.Buffer
		if err := (&png.Encoder{}).Encode(&buf, img); err != nil {
			return Checked{}, fmt.Errorf("encode png: %w", err)
		}
		out.Data = buf.Bytes()
	case "image/gif":
		frames, pixels, ok := gifFrames(data)
		if !ok || frames == 0 || frames > MaxGIFFrames || pixels > maxGIFPixels {
			if ok && frames > 0 {
				return Checked{}, ErrTooLarge
			}
			return Checked{}, ErrNotImage
		}
		if _, err := gif.DecodeAll(bytes.NewReader(data)); err != nil {
			return Checked{}, ErrNotImage
		}
	}
	return out, nil
}

// gifFrames walks a GIF's blocks without decoding any pixels and answers how many
// frames it has and how many pixels their rectangles add up to, so a GIF that would
// decode into too much memory is refused first. ok is false for a file that doesn't
// parse.
func gifFrames(b []byte) (frames int, pixels int64, ok bool) {
	if len(b) < 13 || (string(b[:6]) != "GIF87a" && string(b[:6]) != "GIF89a") {
		return 0, 0, false
	}
	i := 13
	if b[10]&0x80 != 0 {
		i += 3 * (1 << (int(b[10]&7) + 1))
	}
	// skipBlocks steps over a run of data sub-blocks to the byte after its terminator.
	skipBlocks := func(at int) int {
		for at < len(b) {
			n := int(b[at])
			at++
			if n == 0 {
				return at
			}
			at += n
		}
		return -1
	}
	for i < len(b) {
		switch b[i] {
		case 0x3B: // trailer
			return frames, pixels, true
		case 0x21: // extension: label, then sub-blocks
			if i+2 > len(b) {
				return 0, 0, false
			}
			if i = skipBlocks(i + 2); i < 0 {
				return 0, 0, false
			}
		case 0x2C: // image descriptor
			if i+10 > len(b) {
				return 0, 0, false
			}
			w := int64(b[i+5]) | int64(b[i+6])<<8
			h := int64(b[i+7]) | int64(b[i+8])<<8
			flags := b[i+9]
			frames++
			pixels += w * h
			if frames > MaxGIFFrames || pixels > maxGIFPixels {
				return frames, pixels, true
			}
			i += 10
			if flags&0x80 != 0 {
				i += 3 * (1 << (int(flags&7) + 1))
			}
			i++ // LZW minimum code size
			if i = skipBlocks(i); i < 0 {
				return 0, 0, false
			}
		default:
			return 0, 0, false
		}
	}
	return 0, 0, false
}

// orient turns img upright from an EXIF orientation value of 2 to 8.
func orient(img image.Image, o int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)

	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var nx, ny int
			switch o {
			case 2:
				nx, ny = w-1-x, y
			case 3:
				nx, ny = w-1-x, h-1-y
			case 4:
				nx, ny = x, h-1-y
			case 5:
				nx, ny = y, x
			case 6:
				nx, ny = h-1-y, x
			case 7:
				nx, ny = h-1-y, w-1-x
			case 8:
				nx, ny = y, w-1-x
			default:
				nx, ny = x, y
			}
			s := src.PixOffset(x, y)
			d := dst.PixOffset(nx, ny)
			copy(dst.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return dst
}
