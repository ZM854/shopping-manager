package media

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
)

type VideoProcessor struct {
	FFmpegPath  string
	FFprobePath string
}

type MediaProcessor struct {
	Image ImageProcessor
	Video VideoProcessor
}

func (p MediaProcessor) Process(ctx context.Context, purpose Purpose, source, dir string) ([]ProcessedVariant, error) {
	if purpose == StepVideo {
		return p.Video.Process(ctx, source, dir)
	}
	return p.Image.Process(ctx, purpose, source, dir)
}

type videoProbe struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []videoStream `json:"streams"`
}

type videoStream struct {
	Index         int    `json:"index"`
	Type          string `json:"codec_type"`
	Codec         string `json:"codec_name"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	PixelFormat   string `json:"pix_fmt"`
	ColorTransfer string `json:"color_transfer"`
	AverageRate   string `json:"avg_frame_rate"`
	ReportedRate  string `json:"r_frame_rate"`
	Disposition   struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
}

func (p VideoProcessor) probe(ctx context.Context, demuxer, source string) (videoProbe, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := append(videoInput(demuxer, source), "-show_entries",
		"format=duration:stream=index,codec_type,codec_name,width,height,pix_fmt,color_transfer,avg_frame_rate,r_frame_rate:stream_disposition=attached_pic", "-of", "json")
	cmd := exec.CommandContext(ctx, p.FFprobePath, append([]string{"-v", "error"}, args...)...)
	var out boundedOutput
	cmd.Stdout = &out
	if err := runMediaCommand(ctx, cmd); err != nil {
		return videoProbe{}, err
	}
	var info videoProbe
	if json.Unmarshal(out.buffer.Bytes(), &info) != nil {
		return videoProbe{}, uploadError("invalid_media")
	}
	return info, nil
}

func videoInput(demuxer, source string) []string {
	args := []string{"-max_alloc", "134217728", "-threads", "1", "-max_pixels", "10000000",
		"-protocol_whitelist", "file", "-f", demuxer}
	if demuxer == "mov" {
		args = append(args, "-enable_drefs", "0")
	}
	return append(args, "-i", source)
}

func videoDemuxer(source string) (string, error) {
	f, err := os.Open(source)
	if err != nil {
		return "", uploadError("internal_error")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", uploadError("internal_error")
	}
	if info.Size() > VideoInputLimit {
		return "", uploadError("body_too_large")
	}
	kind, err := mimetype.DetectReader(io.LimitReader(f, 4096))
	if err != nil {
		return "", uploadError("internal_error")
	}
	switch kind.String() {
	case "video/mp4", "video/quicktime":
		return "mov", nil
	case "video/webm":
		return "matroska", nil
	default:
		return "", uploadError("unsupported_media_type")
	}
}

func (p videoProbe) tracks() (video, audio *videoStream) {
	for i := range p.Streams {
		s := &p.Streams[i]
		if video == nil && s.Type == "video" && s.Disposition.AttachedPic == 0 {
			video = s
		}
		if audio == nil && s.Type == "audio" {
			audio = s
		}
	}
	return
}

func durationSeconds(raw string) float64 {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	return n
}

func rate(raw string) float64 {
	num, den, ok := strings.Cut(raw, "/")
	if !ok {
		return 0
	}
	n, d := durationSeconds(num), durationSeconds(den)
	if n <= 0 || d <= 0 {
		return 0
	}
	return n / d
}

func (s videoStream) fps() float64 {
	if fps := rate(s.AverageRate); fps > 0 {
		return fps
	}
	return rate(s.ReportedRate)
}

func validateVideoInput(s *videoStream, duration float64) error {
	if s == nil || s.Codec == "" {
		return uploadError("unsupported_media_type")
	}
	switch s.PixelFormat {
	case "yuv420p", "yuv422p", "yuv444p", "yuv440p", "yuvj420p", "yuvj422p", "yuvj444p",
		"yuvj440p", "yuva420p", "nv12", "gray", "rgb24", "bgr24", "gbrp":
	default:
		return uploadError("unsupported_media_type")
	}
	if s.ColorTransfer == "smpte2084" || s.ColorTransfer == "arib-std-b67" {
		return uploadError("unsupported_media_type")
	}
	fps := s.fps()
	if duration <= 0 || fps <= 0 || s.Width <= 0 || s.Height <= 0 {
		return uploadError("invalid_media")
	}
	if duration > 60 || fps > 60 || max(s.Width, s.Height) > 3840 || min(s.Width, s.Height) > 2160 {
		return uploadError("media_constraints_exceeded")
	}
	return nil
}

const fitVideo = "scale=w='min(iw,if(gte(iw,ih),1280,720))':h='min(ih,if(gte(iw,ih),720,1280))':force_original_aspect_ratio=decrease:force_divisible_by=2"

func (p VideoProcessor) Process(ctx context.Context, source, dir string) ([]ProcessedVariant, error) {
	demuxer, err := videoDemuxer(source)
	if err != nil {
		return nil, err
	}
	input, err := p.probe(ctx, demuxer, source)
	if err != nil {
		return nil, err
	}
	video, audio := input.tracks()
	if err := validateVideoInput(video, durationSeconds(input.Format.Duration)); err != nil {
		return nil, err
	}

	mp4 := filepath.Join(dir, "video.mp4")
	args := append([]string{"-v", "error", "-nostdin", "-xerror", "-filter_threads", "1"}, videoInput(demuxer, source)...)
	args = append(args, "-map", fmt.Sprintf("0:%d", video.Index))
	if audio != nil {
		args = append(args, "-map", fmt.Sprintf("0:%d", audio.Index), "-c:a", "aac", "-b:a", "96k", "-ac", "2")
	}
	
	args = append(args, "-map_metadata", "-1", "-map_metadata:s", "-1", "-map_chapters", "-1",
		"-vf", fitVideo, "-fpsmax", "30", "-fps_mode", "cfr", "-t", "60",
		"-c:v", "libx264", "-threads:v", "2", "-preset", "fast", "-crf", "25", "-maxrate", "2200k", "-bufsize", "4400k",
		"-pix_fmt", "yuv420p", "-g", "60", "-movflags", "+faststart", "-fs", "22020096", "-f", "mp4", mp4)
	if err := runMediaCommand(ctx, exec.CommandContext(ctx, p.FFmpegPath, args...)); err != nil {
		return nil, err
	}

	file, err := os.Stat(mp4)
	if err != nil {
		return nil, uploadError("internal_error")
	}
	if file.Size() > videoOutputLimit {
		return nil, uploadError("media_output_too_large")
	}
	result, err := p.probe(ctx, "mov", mp4)
	if err != nil {
		return nil, err
	}
	normalized, sound := result.tracks()
	if normalized == nil || normalized.Codec != "h264" || normalized.PixelFormat != "yuv420p" ||
		normalized.fps() <= 0 || normalized.fps() > 30 || (audio != nil && (sound == nil || sound.Codec != "aac")) {
		return nil, uploadError("invalid_media")
	}
	ms := int64(math.Round(durationSeconds(result.Format.Duration) * 1000))
	output := ProcessedVariant{Path: mp4, Variant: Variant{Name: "video", ContentType: "video/mp4", ByteSize: file.Size(),
		Width: normalized.Width, Height: normalized.Height, DurationMilliseconds: &ms}}
	if !validVariant(output.Variant) {
		return nil, uploadError("media_constraints_exceeded")
	}
	poster, err := p.poster(ctx, output, dir)
	if err != nil {
		return nil, err
	}
	return []ProcessedVariant{output, poster}, nil
}

func (p VideoProcessor) poster(ctx context.Context, video ProcessedVariant, dir string) (ProcessedVariant, error) {
	w, h := video.Width, video.Height
	maxW, maxH := 640, 360
	if h > w {
		maxW, maxH = maxH, maxW
	}
	if w > maxW || h > maxH {
		if w*maxH > h*maxW {
			w, h = maxW, max(1, h*maxW/w)
		} else {
			w, h = max(1, w*maxH/h), maxH
		}
	}
	raw := filepath.Join(dir, "poster.rgba")
	args := append([]string{"-v", "error", "-nostdin", "-xerror", "-filter_threads", "1"}, videoInput("mov", video.Path)...)
	args = append(args, "-map", "0:v:0", "-vf", fmt.Sprintf("scale=%d:%d", w, h),
		"-frames:v", "1", "-c:v", "rawvideo", "-threads:v", "1", "-pix_fmt", "rgba", "-f", "rawvideo", raw)
	if err := runMediaCommand(ctx, exec.CommandContext(ctx, p.FFmpegPath, args...)); err != nil {
		return ProcessedVariant{}, err
	}
	pixels, err := os.ReadFile(raw)
	if err != nil {
		return ProcessedVariant{}, uploadError("internal_error")
	}
	if len(pixels) != w*h*4 {
		return ProcessedVariant{}, uploadError("invalid_media")
	}
	img := &image.RGBA{Pix: pixels, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	return writeJPEG(ctx, img, filepath.Join(dir, "poster.jpg"), "poster", previewLimit)
}
