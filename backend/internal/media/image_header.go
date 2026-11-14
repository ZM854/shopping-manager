package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
)

const (
	ImageInputLimit int64 = 15 << 20
	maxImageSide          = 12000
	maxImagePixels        = 25_000_000
)

type imageHeader struct {
	width, height, orientation int
	codec                      string
}

func readImageHeader(path string) (imageHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return imageHeader{}, uploadError("internal_error")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, ImageInputLimit+1))
	if err != nil {
		return imageHeader{}, uploadError("internal_error")
	}
	if int64(len(data)) > ImageInputLimit {
		return imageHeader{}, uploadError("body_too_large")
	}

	h := imageHeader{orientation: 1}
	switch http.DetectContentType(data) {
	case "image/jpeg":
		h.codec = "mjpeg"
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			var unsupported jpeg.UnsupportedError
			if errors.As(err, &unsupported) {
				return h, uploadError("unsupported_media_type")
			}
			return h, uploadError("invalid_media")
		}
		h.width, h.height = cfg.Width, cfg.Height
		// MJPEG can recover a missing end marker even with strict decoding.
		if !bytes.HasSuffix(data, []byte{0xff, 0xd9}) {
			return h, uploadError("invalid_media")
		}
		err = readJPEGMetadata(data[2:], &h)
		if err != nil {
			return h, err
		}
	case "image/png":
		h.codec = "png"
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return h, uploadError("invalid_media")
		}
		switch cfg.ColorModel {
		case color.Gray16Model, color.RGBA64Model, color.NRGBA64Model:
			return h, uploadError("unsupported_media_type")
		}
		h.width, h.height = cfg.Width, cfg.Height
		if err := readPNGMetadata(data[8:], &h); err != nil {
			return h, err
		}
	case "image/webp":
		h.codec = "webp"
		if err := readWebPHeader(data, &h); err != nil {
			return h, err
		}
	default:
		// DetectContentType requires a complete JPEG/WebP signature. Preserve
		// the corrupt-file response for a recognizable but truncated header.
		if bytes.HasPrefix(data, []byte{0xff, 0xd8}) ||
			(len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP") {
			return h, uploadError("invalid_media")
		}
		return h, uploadError("unsupported_media_type")
	}
	return h, checkImageSize(h.width, h.height)
}

func checkImageSize(w, h int) error {
	if w < 1 || h < 1 {
		return uploadError("invalid_media")
	}
	if w > maxImageSide || h > maxImageSide || w*h > maxImagePixels {
		return uploadError("media_constraints_exceeded")
	}
	return nil
}

// Read APP metadata before the first scan. Go checks the JPEG frame header;
// FFmpeg decodes the scan data. EXIF thumbnails are opaque to this loop.
func readJPEGMetadata(data []byte, h *imageHeader) error {
	seenFrame := false
	for len(data) > 0 && data[0] == 0xff {
		for len(data) > 0 && data[0] == 0xff {
			data = data[1:]
		}
		if len(data) < 3 || data[0] == 0xda {
			break
		}
		marker := data[0]
		n := int(binary.BigEndian.Uint16(data[1:3]))
		if n < 2 || n >= len(data) {
			return uploadError("invalid_media")
		}
		payload := data[3 : 1+n]
		switch marker {
		case 0xc0, 0xc1, 0xc2: // SOF markers supported by jpeg.DecodeConfig.
			if seenFrame {
				return uploadError("invalid_media")
			}
			seenFrame = true
		case 0xe1:
			if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				h.orientation = exifOrientation(payload)
			}
		case 0xe2:
			if bytes.HasPrefix(payload, []byte("MPF\x00")) {
				return uploadError("unsupported_media_type")
			}
		}
		data = data[1+n:]
	}
	return nil
}

// Look for animation and EXIF at chunk boundaries; FFmpeg checks PNG CRCs
// and decodes pixels. Go's PNG decoder deliberately ignores APNG chunks.
func readPNGMetadata(data []byte, h *imageHeader) error {
	first := true
	for len(data) >= 12 {
		n := binary.BigEndian.Uint32(data[:4])
		if n > uint32(len(data)-12) {
			break
		}
		switch string(data[4:8]) {
		case "IHDR":
			if !first {
				return uploadError("invalid_media")
			}
		case "acTL", "fcTL", "fdAT":
			return uploadError("unsupported_media_type")
		case "eXIf":
			h.orientation = exifOrientation(data[8 : 8+int(n)])
		case "IEND":
			if n == 0 && len(data) == 12 {
				return nil
			}
			return uploadError("invalid_media")
		}
		first = false
		data = data[12+int(n):]
	}
	return uploadError("invalid_media")
}

// Go has no standard WebP decoder. Only read RIFF chunk boundaries,
// dimensions and metadata here; leave VP8/VP8L validation to FFmpeg.
func readWebPHeader(data []byte, h *imageHeader) error {
	if len(data) < 12 || binary.LittleEndian.Uint32(data[4:8]) != uint32(len(data)-8) {
		return uploadError("invalid_media")
	}
	var canvasW, canvasH int
	for data = data[12:]; len(data) >= 8; {
		n := binary.LittleEndian.Uint32(data[4:8])
		if n > uint32(len(data)-8) || n+(n&1) > uint32(len(data)-8) {
			return uploadError("invalid_media")
		}
		p := data[8 : 8+int(n)]
		switch string(data[:4]) {
		case "ANIM", "ANMF":
			return uploadError("unsupported_media_type")
		case "VP8X":
			if len(p) != 10 || canvasW != 0 || h.width != 0 {
				return uploadError("invalid_media")
			}
			if p[0]&2 != 0 {
				return uploadError("unsupported_media_type")
			}
			canvasW, canvasH = 1+uint24(p[4:7]), 1+uint24(p[7:10])
			if err := checkImageSize(canvasW, canvasH); err != nil {
				return err
			}
		case "VP8 ":
			if len(p) < 10 || h.width != 0 {
				return uploadError("invalid_media")
			}
			h.width = int(binary.LittleEndian.Uint16(p[6:8]) & 0x3fff)
			h.height = int(binary.LittleEndian.Uint16(p[8:10]) & 0x3fff)
		case "VP8L":
			if len(p) < 5 || h.width != 0 {
				return uploadError("invalid_media")
			}
			v := binary.LittleEndian.Uint32(p[1:5])
			h.width, h.height = 1+int(v&0x3fff), 1+int(v>>14&0x3fff)
		case "EXIF":
			h.orientation = exifOrientation(p)
		}
		data = data[8+int(n+(n&1)):]
	}
	if len(data) != 0 || (canvasW != 0 && (canvasW != h.width || canvasH != h.height)) {
		return uploadError("invalid_media")
	}
	return nil
}

func uint24(p []byte) int { return int(p[0]) | int(p[1])<<8 | int(p[2])<<16 }

// Read just the orientation tag from TIFF IFD0. FFmpeg 5.1/6.1 do
// not apply EXIF orientation for all supported still-image formats.
func exifOrientation(data []byte) int {
	data = bytes.TrimPrefix(data, []byte("Exif\x00\x00"))
	if len(data) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	offset := order.Uint32(data[4:8])
	if order.Uint16(data[2:4]) != 42 || offset < 8 || offset > uint32(len(data)-2) {
		return 1
	}
	n := int(order.Uint16(data[offset:]))
	data = data[offset+2:]
	for ; n > 0 && len(data) >= 12; n-- {
		if order.Uint16(data[:2]) == 0x112 && order.Uint16(data[2:4]) == 3 && order.Uint32(data[4:8]) == 1 {
			if v := int(order.Uint16(data[8:10])); v >= 1 && v <= 8 {
				return v
			}
		}
		data = data[12:]
	}
	return 1
}
