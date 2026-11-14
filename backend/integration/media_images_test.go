//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/media"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/recipe"
	"github.com/ZM854/shopping-manager/backend/internal/router"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
)

func imageSource(t *testing.T, format string, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{R: 220, A: 255}
			if y >= h/2 {
				c = color.NRGBA{B: 220, A: 255}
			}
			if x >= w/2 && format != "jpeg" {
				c.A = 0
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	} else {
		err = png.Encode(&b, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(format, "webp") {
		dir := t.TempDir()
		src := filepath.Join(dir, "fixture.png")
		dst := filepath.Join(dir, "fixture.webp")
		if err := os.WriteFile(src, b.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		lossless := "1"
		if format == "webp-lossy" {
			lossless = "0"
		}
		cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", src, "-frames:v", "1", "-c:v", "libwebp", "-lossless", lossless, dst)
		if err := cmd.Run(); err != nil {
			t.Fatal("FFmpeg fixture conversion failed", err)
		}
		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return b.Bytes()
}

func orientationEXIF(value byte) []byte {
	return append([]byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, value, 0, 0, 0, 0, 0, 0, 0},
		[]byte("synthetic-private-geotag")...)
}

func pngFixtureChunk(kind string, payload []byte) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(len(payload)))
	b = append(b, kind...)
	b = append(b, payload...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(b[4:]))
}

func orientedImage(t *testing.T, format string, orientation byte) []byte {
	b := imageSource(t, format, 80, 40)
	exif := orientationEXIF(orientation)
	switch format {
	case "png":
		return bytes.Join([][]byte{b[:33], pngFixtureChunk("eXIf", exif), b[33:]}, nil)
	case "webp-lossy":
		// Lossy WebP with alpha uses an extended container.
		if string(b[12:16]) != "VP8X" {
			t.Fatal("Expected extended WebP fixture")
		}
		b[20] |= 0x08 // EXIF present
		b = append(b, "EXIF"...)
		b = binary.LittleEndian.AppendUint32(b, uint32(len(exif)))
		b = append(b, exif...)
		if len(exif)%2 != 0 {
			b = append(b, 0)
		}
		binary.LittleEndian.PutUint32(b[4:8], uint32(len(b)-8))
		return b
	}
	exif = append([]byte("Exif\x00\x00"), exif...)
	segment := []byte{0xff, 0xe1, byte((len(exif) + 2) >> 8), byte(len(exif) + 2)}
	result := append(append(append([]byte{}, b[:2]...), segment...), exif...)
	return append(result, b[2:]...)
}

func TestMediaImageRealNormalization(t *testing.T) {
	p := media.ImageProcessor{FFmpegPath: "ffmpeg"}
	for _, tc := range []struct {
		name    string
		data    []byte
		purpose media.Purpose
		w, h    int
	}{
		{"jpeg", imageSource(t, "jpeg", 220, 110), media.Cover, 640, 360},
		{"png", imageSource(t, "png", 1800, 900), media.StepImage, 1600, 800},
		{"webp", imageSource(t, "webp", 80, 40), media.StepImage, 80, 40},
		{"webp-lossy", imageSource(t, "webp-lossy", 80, 40), media.StepImage, 80, 40},
		{"orientation-jpeg", orientedImage(t, "jpeg", 6), media.StepImage, 40, 80},
		{"orientation-png", orientedImage(t, "png", 6), media.StepImage, 40, 80},
		{"orientation-webp", orientedImage(t, "webp-lossy", 6), media.StepImage, 40, 80},
		{"mirror-jpeg", orientedImage(t, "jpeg", 5), media.StepImage, 40, 80},
		{"tiny", imageSource(t, "png", 1, 3), media.StepImage, 1, 3},
		{"narrow-cover", imageSource(t, "jpeg", 2, 12000), media.Cover, 640, 360},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "source")
			if err := os.WriteFile(src, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			output, v, err := p.Process(ctx, tc.purpose, src, dir)
			if err != nil {
				t.Fatal("Normalization failed", err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := jpeg.Decode(bytes.NewReader(data))
			if err != nil || decoded.Bounds().Dx() != tc.w || decoded.Bounds().Dy() != tc.h || len(v) != 1 || v[0].ByteSize != int64(len(data)) {
				t.Fatal("Incorrect output", v, err)
			}
			limit := int64(1500 << 10)
			if tc.purpose == media.Cover {
				limit = 200 << 10
			}
			if int64(len(data)) > limit {
				t.Fatal("Oversized output")
			}
			if bytes.Contains(data, []byte("Exif")) || bytes.Contains(data, []byte("synthetic-private-geotag")) {
				t.Fatal("EXIF leaked")
			}
			if tc.name == "png" || strings.HasPrefix(tc.name, "webp") {
				r, g, b, _ := decoded.At(tc.w*3/4, tc.h/4).RGBA()
				if min(r, g, b) < 60000 {
					t.Fatal("Alpha not composited onto white", r, g, b)
				}
			}
			if strings.HasPrefix(tc.name, "orientation-") {
				r, _, b, _ := decoded.At(5, 5).RGBA()
				if b < r {
					t.Fatal("EXIF rotation lost")
				}
			}
			if tc.name == "mirror-jpeg" {
				r, _, b, _ := decoded.At(5, 5).RGBA()
				if r < b {
					t.Fatal("EXIF transpose lost its reflection")
				}
			}
		})
	}
}

func TestMediaImagePixelLimitAllowsDecoderPadding(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// A real file on the inclusive 25 MP boundary, with a width FFmpeg
	// must round up internally for its decoder buffer.
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi",
		"-i", "color=red:size=5000x5000", "-frames:v", "1", "-threads", "1", "-c:v", "png", "-f", "image2", "-update", "1", src)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Could not make 25 MP PNG: %v: %s", err, output)
	}
	_, v, err := (media.ImageProcessor{FFmpegPath: "ffmpeg"}).Process(ctx, media.StepImage, src, dir)
	if err != nil || len(v) != 1 || v[0].Width != 1600 || v[0].Height != 1600 {
		t.Fatalf("Valid 25 MP image did not normalize: %v, %v", v, err)
	}
}

func TestMediaImageLargeNoiseIsCompressedWithinBudget(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1600, 1600))
	state := uint32(1)
	for i := 0; i < len(img.Pix); i += 4 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		img.Pix[i] = byte(state)
		img.Pix[i+1] = byte(state >> 8)
		img.Pix[i+2] = byte(state >> 16)
		img.Pix[i+3] = 255
	}
	var source bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	if err := enc.Encode(&source, img); err != nil {
		t.Fatal(err)
	}
	if source.Len() < 1500<<10 {
		t.Fatal("Fixture did not exercise a large source")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	if err := os.WriteFile(src, source.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, v, err := (media.ImageProcessor{FFmpegPath: "ffmpeg"}).Process(ctx, media.StepImage, src, dir)
	if err != nil || len(v) != 1 || v[0].ByteSize > 1500<<10 || v[0].ByteSize >= int64(source.Len()) {
		t.Fatal("Large source did not fit output budget", err, v)
	}
}

type imageWriter struct {
	store  *media.Storage
	mu     sync.Mutex
	writes int
	fail   bool
}

func (w *imageWriter) Put(ctx context.Context, key string, body io.ReadSeeker, size int64, typ string) error {
	w.mu.Lock()
	w.writes++
	fail := w.fail
	w.mu.Unlock()
	if err := w.store.Put(ctx, key, body, size, typ); err != nil {
		return err
	}
	if fail {
		return errors.New("synthetic-private-S3-path")
	}
	return nil
}

type imageHTTPFixture struct {
	*recipeHTTPFixture
	service *media.Service
	storage *media.Storage
	writer  *imageWriter
	root    string
	ids     []int64
}

func newImageHTTPFixture(t *testing.T, processor media.Processor) *imageHTTPFixture {
	t.Helper()
	base := newRecipeHTTPFixture(t)
	s := media.NewService(media.NewRepository(base.pool))
	storage := media.NewStorage(testMediaConfig(t))
	w := &imageWriter{store: storage}
	f := &imageHTTPFixture{recipeHTTPFixture: base, service: s, storage: storage, writer: w, root: t.TempDir()}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	migrationExec(t, f.pool, `SELECT setval('recipe_media_id_seq',$1)`, int64(binary.LittleEndian.Uint64(nonce[:])&((1<<52)-1))+(1<<53))
	u, err := media.NewUploader(s, w, processor, f.root)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewJSONHandler(&f.logs, nil))
	tokens := auth.NewTokenService(log, nil, "images-access-key-at-least-thirty-two-bytes", "images-refresh-key-at-least-thirty-two-bytes", time.Hour, time.Hour)
	for _, owner := range []int64{f.first, f.second} {
		pair, err := tokens.GenerateTokens(owner)
		if err != nil {
			t.Fatal(err)
		}
		f.tokens[owner] = pair.AccessToken
	}
	f.engine = router.New(log, shopping.NewHandler(nil, log), recipe.NewHandler(recipe.NewService(f.repo), log), media.NewHandler(s, u, log), auth.NewAuthHandler(log, nil, false), middleware.NewAuthMiddleware(tokens), config.Config{AllowedOrigins: []string{"http://client.example.test"}})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, id := range f.ids {
			file, err := s.GetFile(ctx, f.first, id)
			if err != nil {
				continue
			}
			for _, o := range file.Objects {
				if err := storage.Delete(ctx, o.Key); err != nil {
					t.Error("Test object cleanup failed", err)
				}
			}
		}
	})
	return f
}
func (f *imageHTTPFixture) reserve(t *testing.T, data []byte, purpose media.Purpose, key string) media.Reservation {
	t.Helper()
	sum := sha256.Sum256(data)
	req := media.ReserveRequest{Purpose: purpose, ByteSize: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	b, _ := json.Marshal(req)
	r := recipeHTTPResponse[media.Reservation](t, f.request(f.first, "POST", "/recipe-media", string(b), key), 201)
	f.ids = append(f.ids, r.ID)
	return r
}
func (f *imageHTTPFixture) put(owner, id int64, data []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/recipe-media/%d/content", id), bytes.NewReader(data))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Origin", "http://client.example.test")
	if owner != 0 {
		req.Header.Set("Authorization", "Bearer "+f.tokens[owner])
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func TestMediaImagesHTTPStorageRetryAndBoundaries(t *testing.T) {
	f := newImageHTTPFixture(t, media.ImageProcessor{FFmpegPath: "ffmpeg"})
	data := orientedImage(t, "jpeg", 6)
	r := f.reserve(t, data, media.Cover, "images-http-reserve-0001")
	path := fmt.Sprintf("/recipe-media/%d", r.ID)
	before := recipeHTTPResponse[media.StatusDTO](t, f.request(f.first, "GET", path, "", ""), 200)
	if before.Media != nil || before.State != media.AwaitingUpload {
		t.Fatal("Unexpected initial status")
	}
	recipeHTTPError(t, f.put(f.second, r.ID, data), 404, "not_found")
	recipeHTTPError(t, f.put(0, r.ID, data), 401, "unauthorized")
	bad := append([]byte{}, data...)
	bad[len(bad)/2] ^= 1
	recipeHTTPError(t, f.put(f.first, r.ID, bad), 409, "upload_content_mismatch")
	f.writer.fail = true
	recipeHTTPError(t, f.put(f.first, r.ID, data), 503, "media_storage_unavailable")
	f.writer.fail = false
	failed := recipeHTTPResponse[media.StatusDTO](t, f.request(f.first, "GET", path, "", ""), 200)
	if failed.State != media.Failed || failed.Media != nil || failed.ErrorCode == nil || *failed.ErrorCode != "media_storage_unavailable" {
		t.Fatal("Partial S3 write became ready")
	}
	ready := recipeHTTPResponse[media.MediaDTO](t, f.put(f.first, r.ID, data), 200)
	if ready.ID != r.ID || ready.Purpose != media.Cover || ready.Variants[0].Width != 640 || ready.Variants[0].Height != 360 {
		t.Fatal("Invalid media DTO")
	}
	writes := f.writer.writes
	recipeHTTPResponse[media.MediaDTO](t, f.put(f.first, r.ID, data), 200)
	if f.writer.writes != writes {
		t.Fatal("Ready retry rewrote S3")
	}
	recipeHTTPError(t, f.put(f.first, r.ID, bad), 409, "upload_content_mismatch")
	file, err := f.service.GetFile(context.Background(), f.first, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := f.storage.Get(context.Background(), file.Objects[0].Key)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(stored)); err != nil {
		t.Fatal("S3 did not contain real JPEG", err)
	}
	if bytes.Contains(stored, []byte("synthetic-private-geotag")) {
		t.Fatal("Metadata retained in S3")
	}
	if entries, err := os.ReadDir(f.root); err != nil || len(entries) != 0 {
		t.Fatal("Temporary files retained", err)
	}
	for _, invalid := range []string{`{"purpose":null}`, `{"purpose":"cover","purpose":"cover"}`, `{"purpose":"cover","\u0070urpose":"cover"}`, `{"ownerId":"1"}`, `[]`, `{} {}`, `{"byteSize":1.5}`, string([]byte{0xff})} {
		recipeHTTPError(t, f.request(f.first, "POST", "/recipe-media", invalid, "images-invalid-json-0001"), 400, "invalid_request")
	}
	recipeHTTPError(t, f.request(f.first, "POST", "/recipe-media", strings.Repeat(" ", 4097), "images-too-large-json"), 413, "body_too_large")
	for _, id := range []string{"0", "01", "9223372036854775808"} {
		recipeHTTPError(t, f.request(f.first, "GET", "/recipe-media/"+id, "", ""), 400, "validation_failed")
	}
	videoBody := `{"purpose":"step-video","byteSize":1,"sha256":"` + strings.Repeat("a", 64) + `"}`
	recipeHTTPError(t, f.request(f.first, "POST", "/recipe-media", videoBody, "images-video-not-yet"), 400, "validation_failed")
	logs := f.logs.String()
	for _, secret := range []string{f.tokens[f.first], f.tokens[f.second], "synthetic-private-geotag", "synthetic-private-S3-path"} {
		if strings.Contains(logs, secret) {
			t.Fatal("Private data leaked into logs")
		}
	}
}

func TestMediaImagesHTTPCommitFailureAndRecovery(t *testing.T) {
	f := newImageHTTPFixture(t, media.ImageProcessor{FFmpegPath: "ffmpeg"})
	data := imageSource(t, "png", 100, 60)
	r := f.reserve(t, data, media.StepImage, "images-commit-failure-0001")
	migrationExec(t, f.pool, `CREATE FUNCTION reject_image_ready() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private image SQL'; END $$;
		CREATE CONSTRAINT TRIGGER reject_image_ready AFTER UPDATE ON recipe_media DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN(NEW.state='ready') EXECUTE FUNCTION reject_image_ready()`)
	w := f.put(f.first, r.ID, data)
	recipeHTTPError(t, w, 500, "internal_error")
	if strings.Contains(w.Body.String(), "private") {
		t.Fatal("SQL disclosed")
	}
	file, err := f.service.GetFile(context.Background(), f.first, r.ID)
	if err != nil || file.State != media.Failed || file.SourceReservedBytes != 0 || file.Objects[0].Metadata != nil {
		t.Fatal("Commit failure became success or lost cleanup", err)
	}
	migrationExec(t, f.pool, `DROP TRIGGER reject_image_ready ON recipe_media`)
	recipeHTTPResponse[media.MediaDTO](t, f.put(f.first, r.ID, data), 200)
	interrupted := f.reserve(t, data, media.StepImage, "images-interrupted-0001")
	if _, err := f.service.BeginProcessing(context.Background(), f.first, interrupted.ID, interrupted.ByteSize, interrupted.SHA256); err != nil {
		t.Fatal(err)
	}
	repo := media.NewRepository(f.pool)
	if err := repo.RecoverInterrupted(context.Background()); err != nil {
		t.Fatal(err)
	}
	file, err = f.service.GetFile(context.Background(), f.first, interrupted.ID)
	if err != nil || file.State != media.Failed || file.SourceReservedBytes != 0 || file.ErrorCode == nil || *file.ErrorCode != "media_processing_interrupted" {
		t.Fatal("Interrupted processing not recovered", err)
	}
	recipeHTTPResponse[media.MediaDTO](t, f.put(f.first, interrupted.ID, data), 200)
}

type blockedImageProcessor struct {
	entered chan struct{}
	release chan struct{}
}

func (p *blockedImageProcessor) Process(ctx context.Context, purpose media.Purpose, source, dir string) (string, []media.Variant, error) {
	p.entered <- struct{}{}
	select {
	case <-p.release:
		return (media.ImageProcessor{FFmpegPath: "ffmpeg"}).Process(ctx, purpose, source, dir)
	case <-ctx.Done():
		return "", nil, ctx.Err()
	}
}
func TestMediaImagesAdmissionAndCancellation(t *testing.T) {
	p := &blockedImageProcessor{entered: make(chan struct{}, 3), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(p.release) })
	f := newImageHTTPFixture(t, p)
	data := imageSource(t, "png", 30, 20)
	var reservations []media.Reservation
	for i := 0; i < 3; i++ {
		reservations = append(reservations, f.reserve(t, data, media.Cover, fmt.Sprintf("images-admission-%08d", i)))
	}
	var wg sync.WaitGroup
	defer func() { release(); wg.Wait() }()
	run := func(id int64) chan *httptest.ResponseRecorder {
		ch := make(chan *httptest.ResponseRecorder, 1)
		wg.Add(1)
		go func() { defer wg.Done(); ch <- f.put(f.first, id, data) }()
		return ch
	}
	first, second := run(reservations[0].ID), run(reservations[1].ID)
	for i := 0; i < 2; i++ {
		select {
		case <-p.entered:
		case <-time.After(20 * time.Second):
			t.Fatal("Processing did not start")
		}
	}
	same := f.put(f.first, reservations[0].ID, data)
	recipeHTTPError(t, same, 409, "upload_in_progress")
	if same.Header().Get("Retry-After") == "" {
		t.Fatal("Missing retry delay")
	}
	third := run(reservations[2].ID)
	select {
	case w := <-third:
		recipeHTTPError(t, w, 429, "media_busy")
		if w.Header().Get("Retry-After") == "" {
			t.Fatal("Missing busy retry delay")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Unbounded processing queue")
	}
	cancelled := f.request(f.first, "DELETE", fmt.Sprintf("/recipe-media/%d", reservations[0].ID), "", "")
	recipeHTTPError(t, cancelled, 409, "upload_in_progress")
	release()
	recipeHTTPResponse[media.MediaDTO](t, <-first, 200)
	recipeHTTPResponse[media.MediaDTO](t, <-second, 200)
	w := f.request(f.first, "DELETE", fmt.Sprintf("/recipe-media/%d", reservations[2].ID), "", "")
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal("Cancel did not return empty 204")
	}
}

func animatedImage(t *testing.T, format string) []byte {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "animation."+format)
	encoder := format
	if format == "webp" {
		encoder = "libwebp_anim"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi",
		"-i", "testsrc=size=32x24:rate=2", "-frames:v", "2", "-threads", "1", "-c:v", encoder, "-f", format, dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Could not make %s fixture: %v: %s", format, err, output)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMediaImagesInvalidInputAndStreamingLimit(t *testing.T) {
	f := newImageHTTPFixture(t, media.ImageProcessor{FFmpegPath: "ffmpeg"})
	p := imageSource(t, "png", 20, 12)
	j := imageSource(t, "jpeg", 20, 12)
	withDimensions := func(w, h uint32) []byte {
		header := bytes.Clone(p[16:29])
		binary.BigEndian.PutUint32(header, w)
		binary.BigEndian.PutUint32(header[4:], h)
		return bytes.Join([][]byte{p[:8], pngFixtureChunk("IHDR", header), p[33:]}, nil)
	}
	corrupt := bytes.Clone(p)
	corrupt[len(corrupt)-1] ^= 1 // IEND CRC; past DecodeConfig's stopping point.
	var deep bytes.Buffer
	if err := png.Encode(&deep, image.NewNRGBA64(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		name   string
		data   []byte
		status int
		code   string
	}{
		{"svg", []byte("<svg>synthetic-private-geotag</svg>"), 415, "unsupported_media_type"},
		{"apng", animatedImage(t, "apng"), 415, "unsupported_media_type"},
		{"animated-webp", animatedImage(t, "webp"), 415, "unsupported_media_type"},
		{"16-bit-png", deep.Bytes(), 415, "unsupported_media_type"},
		{"truncated-png", p[:len(p)-2], 422, "invalid_media"},
		{"truncated-jpeg", j[:len(j)-2], 422, "invalid_media"},
		{"png-crc", corrupt, 422, "invalid_media"},
		{"side-limit", withDimensions(12001, 2), 422, "media_constraints_exceeded"},
		{"pixel-limit", withDimensions(5001, 5000), 422, "media_constraints_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := f.reserve(t, tc.data, media.Cover, fmt.Sprintf("images-invalid-%08d", i))
			recipeHTTPError(t, f.put(f.first, r.ID, tc.data), tc.status, tc.code)
		})
	}
	request := media.ReserveRequest{Purpose: media.Cover, ByteSize: media.ImageInputLimit, SHA256: strings.Repeat("a", 64)}
	b, _ := json.Marshal(request)
	r := recipeHTTPResponse[media.Reservation](t, f.request(f.first, "POST", "/recipe-media", string(b), "images-streaming-limit-0001"), 201)
	f.ids = append(f.ids, r.ID)
	recipeHTTPError(t, f.put(f.first, r.ID, bytes.Repeat([]byte{'x'}, int(media.ImageInputLimit)+1)), 413, "body_too_large")
	if f.writer.writes != 0 {
		t.Fatal("Invalid source reached S3")
	}
	entries, err := os.ReadDir(f.root)
	if err != nil || len(entries) != 0 {
		t.Fatal("Rejected input retained tmp", err)
	}
}
