package config

import (
	"strings"
	"testing"
	"time"
)

func TestMediaSettings(t *testing.T) {
	configTestEnv(t)
	t.Setenv("S3_ENDPOINT", "http://localhost:8333/")
	t.Setenv("S3_PUBLIC_ENDPOINT", "http://127.0.0.1:8333")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Media.Endpoint != "http://localhost:8333" ||
		cfg.Media.PublicEndpoint != "http://127.0.0.1:8333" ||
		cfg.Media.RequestTimeout != 30*time.Second || cfg.Media.FFmpegPath != "ffmpeg" ||
		cfg.Media.FFprobePath != "ffprobe" {
		t.Fatal("media settings were lost")
	}
	t.Setenv("S3_REQUEST_TIMEOUT", "12s")
	t.Setenv("FFMPEG_PATH", `C:\media tools\ffmpeg.exe`)
	t.Setenv("FFPROBE_PATH", `C:\media tools\ffprobe.exe`)
	cfg, err = Load()
	if err != nil || cfg.Media.RequestTimeout != 12*time.Second ||
		cfg.Media.FFmpegPath != `C:\media tools\ffmpeg.exe` {
		t.Fatal("media overrides were lost")
	}
}

func TestMediaRejectsUnsafeSettingsWithoutDisclosingValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"S3_ENDPOINT", ""},
		{"S3_ENDPOINT", "https://synthetic-secret:password@storage.example.test"},
		{"S3_PUBLIC_ENDPOINT", "https://storage.example.test/?token=synthetic-secret"},
		{"S3_PUBLIC_ENDPOINT", "https://storage.example.test/#synthetic-secret"},
		{"S3_PUBLIC_ENDPOINT", "https://storage.example.test/path"},
		{"S3_ENDPOINT", "ftp://storage.example.test"},
		{"S3_BUCKET", "127.0.0.1"},
		{"S3_BUCKET", "wrong..bucket"},
		{"S3_BUCKET", "Wrong-Bucket"},
		{"S3_REGION", ""},
		{"S3_ACCESS_KEY_ID", ""},
		{"S3_ACCESS_KEY_ID", "GENERATE_S3_ACCESS_KEY"},
		{"S3_SECRET_ACCESS_KEY", "synthetic-secret"},
		{"S3_SECRET_ACCESS_KEY", strings.Repeat("a", 32) + "\nsynthetic-secret"},
		{"S3_REQUEST_TIMEOUT", "0"},
		{"S3_REQUEST_TIMEOUT", "121s"},
	} {
		t.Run(tc.key+"/"+strings.ReplaceAll(tc.value, "\n", "_"), func(t *testing.T) {
			configTestEnv(t)
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("unsafe media configuration accepted or disclosed")
			}
		})
	}
	configTestEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("S3_ENDPOINT", "http://localhost:8333")
	if _, err := loadMedia("production"); err == nil {
		t.Fatal("HTTP storage accepted outside dev")
	}
}
