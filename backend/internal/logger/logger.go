package logger

import (
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

func levelByEnv(env string) slog.Level {
	switch env {
	case "dev":
		return slog.LevelDebug
	default: 
		return slog.LevelInfo
	}
	
}

func New(env string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: levelByEnv(env),
	}

	var handler slog.Handler

	if env == "dev" {
		handler = tint.NewTextHandler(os.Stdout, &tint.Options{
			Level: levelByEnv(env),
			TimeFormat: time.TimeOnly,
			NoColor: false,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}