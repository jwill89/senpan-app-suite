package uploads

import (
	"bytes"
	"encoding/binary"
)

// jpegOrientation reads the EXIF orientation (tag 0x0112, 1 to 8) from a JPEG's APP1
// segment, or 1 (upright) when there is none or it can't be read.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xFF { // fill byte
			i++
			continue
		}
		// Start of scan or end of image: the metadata segments come before either.
		if marker == 0xDA || marker == 0xD9 {
			return 1
		}
		// Standalone markers carry no length.
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2
			continue
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		segment := data[i+4 : i+2+size]
		if marker == 0xE1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			return tiffOrientation(segment[6:])
		}
		i += 2 + size
	}
	return 1
}

// tiffOrientation reads the orientation from an EXIF TIFF block's first directory.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(t[2:]) != 42 {
		return 1
	}
	dir := int(order.Uint32(t[4:]))
	if dir < 8 || dir+2 > len(t) {
		return 1
	}
	count := int(order.Uint16(t[dir:]))
	for n := range count {
		at := dir + 2 + n*12
		if at+12 > len(t) {
			return 1
		}
		if order.Uint16(t[at:]) != 0x0112 {
			continue
		}
		// A SHORT with one value, stored in the entry itself.
		if order.Uint16(t[at+2:]) != 3 || order.Uint32(t[at+4:]) != 1 {
			return 1
		}
		if v := int(order.Uint16(t[at+8:])); v >= 1 && v <= 8 {
			return v
		}
		return 1
	}
	return 1
}
