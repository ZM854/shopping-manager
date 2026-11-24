//go:build integration

package media

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestVideoToolDeadlineStopsFFmpeg(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin",
		"-re", "-f", "lavfi", "-i", "color=size=16x16:rate=5", "-t", "10", "-f", "null", "-")
	if err := runMediaCommand(ctx, cmd); err != uploadError("media_processing_timeout") ||
		cmd.ProcessState == nil {
		t.Fatalf("FFmpeg was not stopped and reaped: %v", err)
	}
}
