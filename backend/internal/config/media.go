package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type MediaConfig struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	Bucket         string
	AccessKeyID    string
	SecretKey      string
	RequestTimeout time.Duration
	FFmpegPath     string
	FFprobePath    string
}

var (
	mediaBucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	mediaRegionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	mediaKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
)

func loadMedia(appEnv string) (MediaConfig, error) {
	cfg := MediaConfig{
		Endpoint:       strings.TrimRight(os.Getenv("S3_ENDPOINT"), "/"),
		PublicEndpoint: strings.TrimRight(os.Getenv("S3_PUBLIC_ENDPOINT"), "/"),
		Region:         os.Getenv("S3_REGION"),
		Bucket:         os.Getenv("S3_BUCKET"),
		AccessKeyID:    os.Getenv("S3_ACCESS_KEY_ID"),
		SecretKey:      os.Getenv("S3_SECRET_ACCESS_KEY"),
		FFmpegPath:     os.Getenv("FFMPEG_PATH"),
		FFprobePath:    os.Getenv("FFPROBE_PATH"),
	}
	for _, entry := range []struct{ name, value string }{
		{"S3_ENDPOINT", cfg.Endpoint}, {"S3_PUBLIC_ENDPOINT", cfg.PublicEndpoint},
	} {
		u, err := url.Parse(entry.value)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" ||
			u.Fragment != "" || u.Path != "" || (u.Scheme != "http" && u.Scheme != "https") ||
			(u.Scheme == "http" && appEnv != "dev") {
			return MediaConfig{}, fmt.Errorf("%s must be an HTTP(S) origin without credentials, path, query or fragment; HTTP requires dev", entry.name)
		}
	}
	if !mediaRegionPattern.MatchString(cfg.Region) {
		return MediaConfig{}, fmt.Errorf("S3_REGION must be a nonempty region code")
	}
	if !mediaBucketPattern.MatchString(cfg.Bucket) || net.ParseIP(cfg.Bucket) != nil ||
		strings.Contains(cfg.Bucket, "..") || strings.Contains(cfg.Bucket, ".-") || strings.Contains(cfg.Bucket, "-.") {
		return MediaConfig{}, fmt.Errorf("S3_BUCKET must be a DNS-compatible bucket name")
	}
	if !mediaKeyPattern.MatchString(cfg.AccessKeyID) || strings.HasPrefix(cfg.AccessKeyID, "GENERATE_") {
		return MediaConfig{}, fmt.Errorf("S3_ACCESS_KEY_ID must contain 16..128 letters, digits, underscores or hyphens")
	}
	if len(cfg.SecretKey) < 32 || len(cfg.SecretKey) > 128 ||
		strings.TrimSpace(cfg.SecretKey) != cfg.SecretKey || strings.IndexFunc(cfg.SecretKey, unicode.IsControl) >= 0 ||
		strings.HasPrefix(cfg.SecretKey, "GENERATE_") {
		return MediaConfig{}, fmt.Errorf("S3_SECRET_ACCESS_KEY must contain 32..128 bytes without surrounding whitespace or control characters")
	}
	var err error
	cfg.RequestTimeout, err = parseDuration("S3_REQUEST_TIMEOUT", os.Getenv("S3_REQUEST_TIMEOUT"), 30*time.Second)
	if err != nil || cfg.RequestTimeout > 2*time.Minute {
		return MediaConfig{}, fmt.Errorf("S3_REQUEST_TIMEOUT must be between 1s and 2m")
	}
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.FFprobePath == "" {
		cfg.FFprobePath = "ffprobe"
	}
	return cfg, nil
}
