package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type ImageProcessor struct{ FFmpegPath string }

func (p ImageProcessor) Process(ctx context.Context, purpose Purpose, source, dir string) ([]ProcessedVariant, error) {
	if purpose != Cover && purpose != StepImage {
		return nil, uploadError("unsupported_media_type")
	}

	h, err := readImageHeader(source)

	if err != nil {
		return nil, err
	}

	w, hgt := h.width, h.height

	if h.orientation >= 5 {
		w, hgt = hgt, w
	}

	filter := ""

	switch h.orientation {
	case 2:
		filter += "hflip,"
	case 3:
		filter += "hflip,vflip,"
	case 4:
		filter += "vflip,"
	case 5:
		filter += "transpose=clock,hflip,"
	case 6:
		filter += "transpose=clock,"
	case 7:
		filter += "transpose=clock,vflip,"
	case 8:
		filter += "transpose=cclock,"
	}

	if purpose == Cover {
		cw, ch := min(w, max(1, hgt*16/9)), min(hgt, max(1, w*9/16))

		filter += fmt.Sprintf("crop=%d:%d:exact=1,scale=640:360", cw, ch)

		w, hgt = 640, 360
	} else {
		if w > 1600 || hgt > 1600 {
			if w >= hgt {
				hgt = max(1, hgt*1600/w)
				w = 1600
			} else {
				w = max(1, w*1600/hgt)
				hgt = 1600
			}
		}
		filter += fmt.Sprintf("scale=%d:%d", w, hgt)
	}
	intermediate := filepath.Join(dir, "normalized.rgba")

	cmd := exec.CommandContext(ctx, p.FFmpegPath, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-xerror",
		"-max_alloc", "134217728", "-threads", "1", "-filter_threads", "1", "-noautorotate",
		"-max_pixels", "32000000", "-protocol_whitelist", "file",
		"-f", "image2", "-pattern_type", "none", "-c:v", h.codec, "-err_detect", "crccheck+explode", "-i", source,
		"-map", "0:v:0", "-vf", filter, "-frames:v", "1", "-c:v", "rawvideo", "-threads", "1", "-pix_fmt", "rgba", "-f", "rawvideo", intermediate)
	
	if err := runMediaCommand(ctx, cmd); err != nil {
		return nil, err
	}

	f, err := os.Open(intermediate)

	if err != nil {
		return nil, uploadError("internal_error")
	}
	defer f.Close()

	size := w * hgt * 4
	pixels, err := io.ReadAll(io.LimitReader(f, int64(size+1)))

	if err != nil {
		return nil, uploadError("internal_error")
	}

	if len(pixels) != size {
		return nil, uploadError("invalid_media")
	}

	decoded := &image.NRGBA{Pix: pixels, Stride: w * 4, Rect: image.Rect(0, 0, w, hgt)}

	opaque := image.NewRGBA(image.Rect(0, 0, w, hgt))

	draw.Draw(opaque, opaque.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(opaque, opaque.Bounds(), decoded, decoded.Bounds().Min, draw.Over)

	output, err := writeJPEG(ctx, opaque, filepath.Join(dir, "result.jpg"), variantNames(purpose)[0], outputBudget(purpose))
	
	if err != nil {
		return nil, err
	}

	return []ProcessedVariant{output}, nil
}

func writeJPEG(ctx context.Context, img image.Image, path, name string, limit int64) (ProcessedVariant, error) {
	var encoded bytes.Buffer
	for _, quality := range []int{82, 70, 55, 40} {
		if ctx.Err() != nil {
			return ProcessedVariant{}, uploadError("media_processing_timeout")
		}
		encoded.Reset()
		if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: quality}); err != nil {
			return ProcessedVariant{}, uploadError("internal_error")
		}
		if int64(encoded.Len()) <= limit {
			if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
				return ProcessedVariant{}, uploadError("internal_error")
			}
			return ProcessedVariant{Path: path, Variant: Variant{Name: name, ContentType: "image/jpeg",
				ByteSize: int64(encoded.Len()), Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}}, nil
		}
	}
	return ProcessedVariant{}, uploadError("media_output_too_large")
}
