// Package media accepts image uploads: it validates them, strips metadata
// such as GPS location, stores the bytes and serves them to room members.
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/gif" // register decoders for DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"net/http"

	"smalltalk/internal/apperr"
)

const (
	MaxBytes     = 10 << 20 // 10 MB
	MaxDimension = 8192     // per side; keeps decoded images a sane size for phones
)

type Info struct {
	ContentType string
	Width       int
	Height      int
}

// Inspect checks that data is a JPEG, PNG or GIF judged by its bytes (never
// by the client's claimed type) and returns a copy without metadata.
func Inspect(data []byte) (Info, []byte, error) {
	if len(data) > MaxBytes {
		return Info{}, nil, apperr.ImageTooLarge
	}
	ct := http.DetectContentType(data)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Info{}, nil, apperr.ImageInvalid
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxDimension || cfg.Height > MaxDimension {
		return Info{}, nil, apperr.ImageInvalid
	}

	var clean []byte
	switch ct {
	case "image/jpeg":
		clean, err = stripJPEG(data)
	case "image/png":
		clean, err = stripPNG(data)
	case "image/gif":
		clean = data // GIF has no EXIF/GPS blocks
	default:
		return Info{}, nil, apperr.ImageInvalid
	}
	if err != nil {
		return Info{}, nil, apperr.ImageInvalid
	}
	return Info{ContentType: ct, Width: cfg.Width, Height: cfg.Height}, clean, nil
}

var errMalformed = errors.New("malformed image")

// stripJPEG drops APP1 (EXIF, XMP — where GPS lives), APP13 (IPTC) and
// comment segments. Pixel data is copied untouched.
func stripJPEG(data []byte) ([]byte, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, errMalformed
	}
	out := make([]byte, 0, len(data))
	out = append(out, 0xFF, 0xD8)
	i := 2
	for i < len(data) {
		if data[i] != 0xFF {
			return nil, errMalformed
		}
		// Skip fill bytes.
		for i < len(data) && data[i] == 0xFF {
			i++
		}
		if i >= len(data) {
			return nil, errMalformed
		}
		marker := data[i]
		i++
		switch {
		case marker == 0xD9: // EOI
			return append(out, 0xFF, marker), nil
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7): // no length
			out = append(out, 0xFF, marker)
			continue
		}
		if i+2 > len(data) {
			return nil, errMalformed
		}
		n := int(binary.BigEndian.Uint16(data[i:]))
		if n < 2 || i+n > len(data) {
			return nil, errMalformed
		}
		segment := data[i : i+n]
		i += n
		if marker == 0xDA { // SOS: the rest is entropy-coded data up to EOI
			out = append(out, 0xFF, marker)
			out = append(out, segment...)
			return append(out, data[i:]...), nil
		}
		if marker == 0xE1 || marker == 0xED || marker == 0xFE {
			continue
		}
		out = append(out, 0xFF, marker)
		out = append(out, segment...)
	}
	return nil, errMalformed
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// stripPNG drops text, EXIF and timestamp chunks.
func stripPNG(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, pngSignature) {
		return nil, errMalformed
	}
	out := make([]byte, 0, len(data))
	out = append(out, pngSignature...)
	i := len(pngSignature)
	for i < len(data) {
		if i+8 > len(data) {
			return nil, errMalformed
		}
		n := int(binary.BigEndian.Uint32(data[i:]))
		end := i + 12 + n // length + type + data + CRC
		if n < 0 || end > len(data) || end < i {
			return nil, errMalformed
		}
		switch string(data[i+4 : i+8]) {
		case "tEXt", "zTXt", "iTXt", "eXIf", "tIME":
		default:
			out = append(out, data[i:end]...)
		}
		if string(data[i+4:i+8]) == "IEND" {
			return out, nil
		}
		i = end
	}
	return nil, errMalformed
}
