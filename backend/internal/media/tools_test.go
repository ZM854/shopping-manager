package media

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/config"
)

func TestToolsFailSafely(t *testing.T) {
	err := CheckTools(context.Background(), config.MediaConfig{
		FFmpegPath: "synthetic-secret-nonexistent-program", FFprobePath: "ffprobe",
	})
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal("missing tool accepted or disclosed")
	}
}

func TestToolOutputIsBounded(t *testing.T) {
	var output boundedOutput
	_, err := io.Copy(&output, strings.NewReader(strings.Repeat("x", 1024*1024)))
	if err != nil || len(output.String()) != 256*1024 {
		t.Fatal("tool output is not bounded")
	}
	if !hasCodec(" V....D libx264 H.264 encoder\n A..... aac AAC\n", "libx264") ||
		hasCodec(" V..... h264_nvenc NVIDIA H.264", "libx264") {
		t.Fatal("hardware encoder mistaken for required software codec")
	}
}
