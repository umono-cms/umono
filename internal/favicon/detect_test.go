package favicon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/png"
	"testing"
)

func TestDetectPNG(t *testing.T) {
	t.Parallel()

	got, err := Detect(validPNG(t))
	if err != nil {
		t.Fatalf("Detect() error = %v, want nil", err)
	}
	if got != TypePNG {
		t.Fatalf("Detect() = %q, want %q", got, TypePNG)
	}
}

func TestDetectICO(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
	}{
		{
			name: "ico with a png image",
			data: buildICO(0, 1, validPNG(t)),
		},
		{
			name: "ico with a bmp image",
			data: buildICO(0, 1, bmpImage()),
		},
		{
			name: "ico with a png and a bmp image",
			data: buildICO(0, 1, validPNG(t), bmpImage()),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Detect(tc.data)
			if err != nil {
				t.Fatalf("Detect() error = %v, want nil", err)
			}
			if got != TypeICO {
				t.Fatalf("Detect() = %q, want %q", got, TypeICO)
			}
		})
	}
}

func TestDetectIgnoresTheSizeOfTheImage(t *testing.T) {
	t.Parallel()

	padded := make([]byte, MaxSize)
	copy(padded, validPNG(t))

	got, err := Detect(padded)
	if err != nil {
		t.Fatalf("Detect() error = %v, want nil", err)
	}
	if got != TypePNG {
		t.Fatalf("Detect() = %q, want %q", got, TypePNG)
	}
}

func TestDetectRejectsATooLargeFile(t *testing.T) {
	t.Parallel()

	padded := make([]byte, MaxSize+1)
	copy(padded, validPNG(t))

	got, err := Detect(padded)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Detect() error = %v, want %v", err, ErrTooLarge)
	}
	if got != "" {
		t.Fatalf("Detect() = %q, want empty type", got)
	}
}

func TestDetectRejectsOtherFormats(t *testing.T) {
	t.Parallel()

	pngBytes := validPNG(t)

	brokenCRC := make([]byte, len(pngBytes))
	copy(brokenCRC, pngBytes)
	brokenCRC[29] ^= 0xFF

	idatChunk := make([]byte, len(pngBytes))
	copy(idatChunk, pngBytes)
	copy(idatChunk[12:16], "IDAT")

	cases := []struct {
		name string
		data []byte
	}{
		{
			name: "empty content",
			data: []byte{},
		},
		{
			name: "gif",
			data: append([]byte("GIF89a"), make([]byte, 20)...),
		},
		{
			name: "svg",
			data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		},
		{
			name: "png signature without a chunk",
			data: []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'},
		},
		{
			name: "png with a broken ihdr crc",
			data: brokenCRC,
		},
		{
			name: "png with an idat chunk instead of ihdr",
			data: idatChunk,
		},
		{
			name: "ico with a reserved field set",
			data: buildICO(1, 1, bmpImage()),
		},
		{
			name: "ico cursor",
			data: buildICO(0, 2, bmpImage()),
		},
		{
			name: "ico header without images",
			data: []byte{0, 0, 1, 0, 0, 0},
		},
		{
			name: "ico with a truncated directory",
			data: buildICO(0, 1, bmpImage())[:20],
		},
		{
			name: "ico with a size past the end of the content",
			data: setUint32(buildICO(0, 1, bmpImage()), 14, 41),
		},
		{
			name: "ico with an overflowing offset",
			data: setUint32(buildICO(0, 1, bmpImage()), 18, 0xFFFFFFF0),
		},
		{
			name: "ico with an unknown image",
			data: buildICO(0, 1, []byte("JUNKJUNK")),
		},
		{
			name: "ico with one unknown image",
			data: buildICO(0, 1, pngBytes, []byte("JUNKJUNK")),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Detect(tc.data)
			if !errors.Is(err, ErrFormat) {
				t.Fatalf("Detect() error = %v, want %v", err, ErrFormat)
			}
			if got != "" {
				t.Fatalf("Detect() = %q, want empty type", got)
			}
		})
	}
}

func validPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("png.Encode() error = %v, want nil", err)
	}
	return buf.Bytes()
}

func bmpImage() []byte {
	img := make([]byte, 40)
	binary.LittleEndian.PutUint32(img[:4], 40)
	return img
}

func buildICO(reserved, typ uint16, images ...[]byte) []byte {
	header := make([]byte, 6)
	binary.LittleEndian.PutUint16(header[0:2], reserved)
	binary.LittleEndian.PutUint16(header[2:4], typ)
	binary.LittleEndian.PutUint16(header[4:6], uint16(len(images)))

	offset := 6 + 16*len(images)
	directory := make([]byte, 16*len(images))
	for i, img := range images {
		entry := directory[i*16 : (i+1)*16]
		entry[0] = 0
		entry[1] = 0
		binary.LittleEndian.PutUint32(entry[8:12], uint32(len(img)))
		binary.LittleEndian.PutUint32(entry[12:16], uint32(offset))
		offset += len(img)
	}

	out := make([]byte, 0, offset)
	out = append(out, header...)
	out = append(out, directory...)
	for _, img := range images {
		out = append(out, img...)
	}
	return out
}

func setUint32(b []byte, at int, v uint32) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	binary.LittleEndian.PutUint32(out[at:at+4], v)
	return out
}
