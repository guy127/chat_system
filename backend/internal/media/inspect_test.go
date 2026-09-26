package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"smalltalk/internal/apperr"
)

func testImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	return img
}

// jpegWithExif inserts an APP1 "Exif" segment carrying a fake GPS marker after SOI.
func jpegWithExif(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(4, 3), nil); err != nil {
		t.Fatal(err)
	}
	payload := []byte("Exif\x00\x00GPS-SECRET-LOCATION")
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2)) //nolint:gosec // fixed small fixture
	seg = append(seg, payload...)
	b := buf.Bytes()
	return append(append(append([]byte{}, b[:2]...), seg...), b[2:]...)
}

func pngWithText(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage(5, 2)); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	data := []byte("Comment\x00GPS-SECRET-LOCATION")
	chunk := make([]byte, 8, 12+len(data))
	binary.BigEndian.PutUint32(chunk, uint32(len(data))) //nolint:gosec // fixed small fixture
	copy(chunk[4:], "tEXt")
	chunk = append(chunk, data...)
	crc := crc32.ChecksumIEEE(chunk[4:])
	chunk = binary.BigEndian.AppendUint32(chunk, crc)
	// Insert right after the IHDR chunk (8 signature + 25 IHDR bytes).
	return append(append(append([]byte{}, b[:33]...), chunk...), b[33:]...)
}

func TestInspectStripsMetadata(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		wantCT string
		w, h   int
	}{
		{"jpeg with exif", jpegWithExif(t), "image/jpeg", 4, 3},
		{"png with text chunk", pngWithText(t), "image/png", 5, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Contains(tt.data, []byte("GPS-SECRET")) {
				t.Fatal("fixture lacks metadata")
			}
			info, clean, err := Inspect(tt.data)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if info.ContentType != tt.wantCT || info.Width != tt.w || info.Height != tt.h {
				t.Fatalf("info = %+v", info)
			}
			if bytes.Contains(clean, []byte("GPS-SECRET")) {
				t.Fatal("metadata survived")
			}
			if _, _, err := image.Decode(bytes.NewReader(clean)); err != nil {
				t.Fatalf("cleaned image no longer decodes: %v", err)
			}
		})
	}
}

func TestInspectRejects(t *testing.T) {
	var gifBuf bytes.Buffer
	pal := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	if err := gif.Encode(&gifBuf, pal, nil); err != nil {
		t.Fatal(err)
	}
	if info, _, err := Inspect(gifBuf.Bytes()); err != nil || info.ContentType != "image/gif" {
		t.Fatalf("gif should pass, got %+v %v", info, err)
	}

	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), apperr.ImageInvalid},
		{"html", []byte("<html><body>hi</body></html>"), apperr.ImageInvalid},
		{"truncated jpeg", jpegWithExif(t)[:40], apperr.ImageInvalid},
		{"too large", make([]byte, MaxBytes+1), apperr.ImageTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := Inspect(tt.data); !errors.Is(err, tt.want) {
				t.Fatalf("Inspect() = %v, want %v", err, tt.want)
			}
		})
	}
}
