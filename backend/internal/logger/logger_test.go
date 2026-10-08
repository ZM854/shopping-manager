package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestLoggerLevels(t *testing.T) {
	for _, env := range []string{"dev", "prod", "", "unknown"} {
		log := New(env)
		want := slog.LevelInfo
		if env == "dev" {
			want = slog.LevelDebug
		}
		if levelByEnv(env) != want || log.Enabled(context.Background(), slog.LevelDebug) != (env == "dev") || !log.Enabled(context.Background(), slog.LevelError) {
			t.Fatalf("Wrong log level for %q", env)
		}
	}
}
