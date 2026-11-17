package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type uploadMetadataStub struct {
	file                File
	root                string
	released, completed bool
	failure             string
	testing             *testing.T
}

func (m *uploadMetadataStub) GetFile(context.Context, int64, int64) (File, error) { return m.file, nil }
func (m *uploadMetadataStub) BeginProcessing(context.Context, int64, int64, int64, string) (File, error) {
	f := m.file
	f.State = Processing
	return f, nil
}
func (m *uploadMetadataStub) ReleaseSource(context.Context, int64, int64) error {
	entries, err := os.ReadDir(m.root)
	if err != nil || len(entries) != 0 {
		m.testing.Error("Source reservation released before tmp removal", err)
	}
	m.released = true
	return nil
}
func (m *uploadMetadataStub) Complete(context.Context, int64, int64, []Variant) (File, error) {
	m.completed = true
	return File{}, errors.New("unexpected completion")
}
func (m *uploadMetadataStub) Fail(_ context.Context, _, _ int64, code string) (File, error) {
	m.failure = code
	return File{}, nil
}

type timeoutProcessor struct{}

func (timeoutProcessor) Process(ctx context.Context, _ Purpose, _, _ string) ([]ProcessedVariant, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type unusedStorage struct{ testing *testing.T }

func (s unusedStorage) Put(context.Context, string, io.ReadSeeker, int64, string) error {
	s.testing.Error("Timeout wrote an S3 object")
	return nil
}

func TestUploadTimeoutsCleanTmpAndPreserveFailure(t *testing.T) {
	for _, receive := range []bool{false, true} {
		t.Run(map[bool]string{true: "receive", false: "process"}[receive], func(t *testing.T) {
			root := t.TempDir()
			sum := sha256.Sum256([]byte("x"))
			m := &uploadMetadataStub{file: File{ID: 1, Purpose: StepImage, ByteSize: 1, SHA256: hex.EncodeToString(sum[:]), State: AwaitingUpload}, root: root, testing: t}
			u, err := NewUploader(m, unusedStorage{t}, timeoutProcessor{}, root)
			if err != nil {
				t.Fatal(err)
			}
			u.receiveTimeout = 15 * time.Millisecond
			u.processTimeout = 15 * time.Millisecond
			var body io.ReadCloser = io.NopCloser(strings.NewReader("x"))
			if receive {
				r, w := io.Pipe()
				defer w.Close()
				body = r
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := u.Upload(ctx, 1, 1, -1, body)
			if err != uploadError("media_processing_timeout") || got.ID != 0 || m.completed || !m.released || m.failure != "media_processing_timeout" {
				t.Fatal("Timeout produced success or lost cleanup", err, m.failure)
			}
		})
	}
}

func TestUploadStartupOnlyRemovesOwnedTemporaryEntries(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "upload-old")
	if err := os.Mkdir(stale, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "source"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewUploader(nil, nil, nil, root); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "keep.txt" {
		t.Fatal("Startup cleanup crossed its namespace", err)
	}
}

func TestUploadAdmissionAllowsOneVideoAndOnePhoto(t *testing.T) {
	u, err := NewUploader(nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := u.acquire(1, StepVideo); err != nil {
		t.Fatal(err)
	}
	if err := u.acquire(2, StepVideo); err != uploadError("media_busy") {
		t.Fatalf("Second video should wait: %v", err)
	}
	if err := u.acquire(3, StepImage); err != nil {
		t.Fatalf("Photo should fit alongside video: %v", err)
	}
	if err := u.acquire(1, StepVideo); err != ErrUploadInProgress {
		t.Fatalf("Same ID should get an in-progress conflict: %v", err)
	}
	u.release(1)
	if err := u.acquire(2, StepVideo); err != nil {
		t.Fatalf("New video should fit after release: %v", err)
	}
}
