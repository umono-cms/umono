// Package favicon checks the file the site owner uploads as the favicon.
// It reads only the structure of the file and does not decode pixels.
package favicon

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

// MaxSize is the largest favicon in bytes.
const MaxSize = 256 << 10

const (
	// TypePNG is the media type of a PNG favicon.
	TypePNG = "image/png"
	// TypeICO is the media type of an ICO favicon.
	TypeICO = "image/x-icon"
)

var (
	// ErrTooLarge is returned when the uploaded file is bigger than MaxSize.
	ErrTooLarge = errors.New("favicon is larger than 256 KiB")
	// ErrFormat is returned when the uploaded file is neither a PNG nor an ICO.
	ErrFormat = errors.New("favicon is not a PNG or an ICO")
)

// pngSignature is the eight byte header every PNG file starts with.
const pngSignature = "\x89PNG\r\n\x1a\n"

// Detect reports the media type of the uploaded favicon.
//
// The file extension and the media type sent by the client are not used:
// only the content of the file decides the result.
func Detect(content []byte) (string, error) {
	if len(content) > MaxSize {
		return "", ErrTooLarge
	}

	if isPNG(content) {
		return TypePNG, nil
	}

	if isICO(content) {
		return TypeICO, nil
	}

	return "", ErrFormat
}

// isPNG reports whether b is a PNG file.
//
// The IHDR chunk follows the signature and is complete, so its length and
// its CRC are checked.
func isPNG(b []byte) bool {
	if len(b) < 33 {
		return false
	}

	if string(b[:8]) != pngSignature {
		return false
	}

	if binary.BigEndian.Uint32(b[8:12]) != 13 {
		return false
	}

	if string(b[12:16]) != "IHDR" {
		return false
	}

	return binary.BigEndian.Uint32(b[29:33]) == crc32.ChecksumIEEE(b[12:29])
}

// isICO reports whether b is an ICO file.
//
// The header, every directory entry inside the file and every image it
// points at must be there, and every image must be a PNG or a BMP.
func isICO(b []byte) bool {
	if len(b) < 6 {
		return false
	}

	if binary.LittleEndian.Uint16(b[0:2]) != 0 {
		return false
	}

	if binary.LittleEndian.Uint16(b[2:4]) != 1 {
		return false
	}

	count := int(binary.LittleEndian.Uint16(b[4:6]))
	if count < 1 {
		return false
	}

	if len(b) < 6+16*count {
		return false
	}

	for i := 0; i < count; i++ {
		entry := b[6+16*i : 6+16*i+16]

		size := uint64(binary.LittleEndian.Uint32(entry[8:12]))
		offset := uint64(binary.LittleEndian.Uint32(entry[12:16]))
		if offset+size > uint64(len(b)) {
			return false
		}

		img := b[offset : offset+size]
		if !isImageInICO(img) {
			return false
		}
	}

	return true
}

// isImageInICO reports whether img is an image stored inside an ICO file.
//
// A PNG image starts with the PNG signature. A BMP image in an ICO starts
// with a BITMAPINFOHEADER, whose size is 40.
func isImageInICO(img []byte) bool {
	if len(img) >= 8 && string(img[:8]) == pngSignature {
		return true
	}

	return len(img) >= 4 && binary.LittleEndian.Uint32(img[:4]) == 40
}
