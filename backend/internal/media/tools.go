package media

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/config"
)

// CheckTools is a startup dependency check
func CheckTools(ctx context.Context, cfg config.MediaConfig) error {
	for _, check := range []struct {
		program string
		args    []string
		names   []string
	}{
		{cfg.FFmpegPath, []string{"-hide_banner", "-encoders"}, []string{"mjpeg", "libx264", "aac"}},
		{cfg.FFmpegPath, []string{"-hide_banner", "-decoders"}, []string{"mjpeg", "png", "webp", "h264", "hevc", "vp8", "vp9", "aac", "opus", "vorbis"}},
		{cfg.FFprobePath, []string{"-version"}, nil},
	} {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(checkCtx, check.program, check.args...)
		cmd.WaitDelay = time.Second
		var output boundedOutput
		cmd.Stdout = &output
		cmd.Stderr = nil
		err := cmd.Run()
		cancel()
		if err != nil {
			return errors.New("media tools unavailable")
		}
		if check.program == cfg.FFprobePath && len(check.names) == 0 &&
			!strings.HasPrefix(output.String(), "ffprobe version ") {
			return errors.New("media probe unavailable")
		}
		for _, name := range check.names {
			if !hasCodec(output.String(), name) {
				return errors.New("required media codec unavailable: " + name)
			}
		}
	}
	return nil
}

func hasCodec(output, name string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[1] == name {
			return true
		}
	}
	return false
}

type boundedOutput struct{ buffer bytes.Buffer }

func (b *boundedOutput) String() string { return b.buffer.String() }

func (b *boundedOutput) Write(data []byte) (int, error) {
	const maxOutput = 256 * 1024
	if remaining := maxOutput - b.buffer.Len(); remaining > 0 {
		_, _ = b.buffer.Write(data[:min(len(data), remaining)])
	}
	return len(data), nil
}
