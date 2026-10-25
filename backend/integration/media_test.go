//go:build integration

package integration_test

import (
	"context"
	"errors"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/media"
	"github.com/google/uuid"
)

func testMediaConfig(t *testing.T) config.MediaConfig {
	t.Helper()
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" ||
		u.Scheme != "http" || (u.Host != "storage-test:8333" && u.Host != "localhost:58333" && u.Host != "127.0.0.1:58333") ||
		os.Getenv("TEST_S3_BUCKET") != "shopping-manager-media-test" ||
		os.Getenv("TEST_S3_ACCESS_KEY_ID") != "shopping-manager-test-key" ||
		os.Getenv("TEST_S3_SECRET_ACCESS_KEY") != "local-integration-storage-secret-only" {
		t.Fatal("Нужен отдельный тестовый S3; рабочие endpoint/bucket/ключи запрещены, .env не загружается")
	}
	return config.MediaConfig{
		Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1",
		Bucket: os.Getenv("TEST_S3_BUCKET"), AccessKeyID: os.Getenv("TEST_S3_ACCESS_KEY_ID"),
		SecretKey: os.Getenv("TEST_S3_SECRET_ACCESS_KEY"), RequestTimeout: 10 * time.Second,
		FFmpegPath: "ffmpeg", FFprobePath: "ffprobe",
	}
}

func TestMediaStorage(t *testing.T) {
	cfg := testMediaConfig(t)
	store := media.NewStorage(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := store.Check(ctx); err != nil {
		t.Fatal("Тестовое S3 недоступно")
	}
	key := "checks/" + uuid.NewString() + ".txt"
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := store.Delete(cleanupCtx, key); err != nil {
			t.Error("Не удалось убрать тестовый объект")
		}
	})
	const payload = "private integration object"
	if err := store.Put(ctx, key, strings.NewReader(payload), int64(len(payload)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	readMedia := func(key, want string) {
		t.Helper()
		body, err := store.Get(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()
		data, err := io.ReadAll(io.LimitReader(body, 1024))
		if err != nil || string(data) != want {
			t.Fatal("Содержимое S3 объекта не сохранилось")
		}
	}
	readMedia(key, payload)
	client := &http.Client{Timeout: 5 * time.Second}
	for _, path := range []string{"/" + cfg.Bucket, "/" + cfg.Bucket + "/" + key} {
		response, err := client.Get(cfg.Endpoint + path)
		if err != nil {
			t.Fatal("Не удалось проверить анонимный доступ")
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusUnauthorized {
			t.Fatal("Анонимный доступ к bucket/объекту не закрыт")
		}
	}
	wrongCfg := cfg
	wrongCfg.SecretKey = strings.Repeat("x", 32)
	if err := media.NewStorage(wrongCfg).Check(ctx); err == nil {
		t.Fatal("S3 приняло неверный ключ")
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal("Повтор удаления отсутствующего объекта не идемпотентен")
	}
	missingBody, missingErr := store.Get(ctx, key)
	if missingBody != nil {
		missingBody.Close()
	}
	if !errors.Is(missingErr, media.ErrObjectNotFound) {
		t.Fatal("Объект после удаления не исчез")
	}

	// The media-check script recreates only its own storage container between
	// these two independent runs. Full integration runs have no phase/marker.
	phase := os.Getenv("TEST_MEDIA_PERSISTENCE_PHASE")
	if phase != "" {
		id, err := uuid.Parse(os.Getenv("TEST_MEDIA_PERSISTENCE_ID"))
		if err != nil {
			t.Fatal("Некорректный ID проверки сохранности")
		}
		marker := "checks/persistence/" + id.String()
		switch phase {
		case "write":
			if err := store.Put(ctx, marker, strings.NewReader(payload), int64(len(payload)), "text/plain"); err != nil {
				t.Fatal(err)
			}
		case "read":
			readMedia(marker, payload)
			if err := store.Delete(ctx, marker); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("Неизвестная фаза проверки сохранности")
		}
	}
}

func TestMediaToolchain(t *testing.T) {
	cfg := config.MediaConfig{FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if err := media.CheckTools(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	imagePath := filepath.Join(directory, "sample.jpg")
	videoPath := filepath.Join(directory, "sample.mp4")
	run := func(program string, args ...string) string {
		t.Helper()
		output, err := exec.CommandContext(ctx, program, args...).Output()
		if err != nil {
			t.Fatal("Не удалось выполнить тестовое преобразование/проверку медиа")
		}
		return string(output)
	}
	run("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=red:s=64x64:d=1",
		"-frames:v", "1", "-threads", "1", "-c:v", "mjpeg", imagePath)
	file, err := os.Open(imagePath)
	if err != nil {
		t.Fatal("Нет тестового JPEG")
	}
	image, decodeErr := jpeg.Decode(file)
	file.Close()
	if decodeErr != nil || image.Bounds().Dx() != 64 || image.Bounds().Dy() != 64 {
		t.Fatal("FFmpeg не создал корректный JPEG")
	}
	run("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=64x64:r=10:d=0.3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=0.3", "-c:v", "libx264",
		"-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", "-movflags", "+faststart",
		"-threads", "1", videoPath)
	codecs := run("ffprobe", "-v", "error", "-show_entries", "stream=codec_name", "-of", "csv=p=0", videoPath)
	if !strings.Contains(codecs, "h264") || !strings.Contains(codecs, "aac") {
		t.Fatal("FFprobe не подтвердил H.264/AAC")
	}
}
