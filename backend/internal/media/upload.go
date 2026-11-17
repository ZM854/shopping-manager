package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type uploadError string

func (e uploadError) Error() string { return string(e) }

type UploadMetadata interface {
	GetFile(context.Context, int64, int64) (File, error)
	BeginProcessing(context.Context, int64, int64, int64, string) (File, error)
	ReleaseSource(context.Context, int64, int64) error
	Complete(context.Context, int64, int64, []Variant) (File, error)
	Fail(context.Context, int64, int64, string) (File, error)
}
type ObjectWriter interface {
	Put(context.Context, string, io.ReadSeeker, int64, string) error
}
type Processor interface {
	Process(context.Context, Purpose, string, string) ([]ProcessedVariant, error)
}

type ProcessedVariant struct {
	Path string
	Variant
}

type Uploader struct {
	metadata                       UploadMetadata
	storage                        ObjectWriter
	processor                      Processor
	root                           string
	mu                             sync.Mutex
	active                         map[int64]Purpose
	receiveTimeout, processTimeout time.Duration
}

func NewUploader(metadata UploadMetadata, storage ObjectWriter, processor Processor, root string) (*Uploader, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, uploadError("internal_error")
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, uploadError("internal_error")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, uploadError("internal_error")
	}
	if err = os.Chmod(root, 0700); err != nil {
		return nil, uploadError("internal_error")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, uploadError("internal_error")
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "upload-") {
			continue
		}
		if err = os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return nil, uploadError("internal_error")
		}
	}
	return &Uploader{metadata: metadata, storage: storage, processor: processor, root: root,
		active: make(map[int64]Purpose), receiveTimeout: 60 * time.Second, processTimeout: 180 * time.Second}, nil
}

func (u *Uploader) acquire(id int64, purpose Purpose) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, exists := u.active[id]; exists {
		return ErrUploadInProgress
	}
	if len(u.active) >= 2 {
		return uploadError("media_busy")
	}
	if purpose == StepVideo {
		for _, active := range u.active {
			if active == StepVideo {
				return uploadError("media_busy")
			}
		}
	}
	u.active[id] = purpose
	return nil
}
func (u *Uploader) release(id int64) { u.mu.Lock(); delete(u.active, id); u.mu.Unlock() }

func (u *Uploader) Upload(ctx context.Context, owner, id, contentLength int64, body io.ReadCloser) (result File, err error) {
	ctx, cancelOverall := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelOverall()

	f, err := u.metadata.GetFile(ctx, owner, id)
	if err != nil {
		return File{}, err
	}

	limit := inputLimit(f.Purpose)
	if contentLength > limit {
		return File{}, uploadError("body_too_large")
	}

	if f.State == Processing {
		return File{}, ErrUploadInProgress
	}

	if f.State == DeletePending {
		return File{}, ErrMediaUnavailable
	}

	if err = u.acquire(id, f.Purpose); err != nil {
		return File{}, err
	}
	defer u.release(id)

	f, err = u.metadata.BeginProcessing(ctx, owner, id, f.ByteSize, f.SHA256)
	if err != nil {
		return File{}, err
	}

	isNew := f.State == Processing
	dir := ""

	defer func() {
		if !isNew {
			return
		}
		if dir != "" {
			if cleanupErr := os.RemoveAll(dir); cleanupErr != nil {
				result = File{}
				err = uploadError("internal_error")
				return
			}
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if releaseErr := u.metadata.ReleaseSource(cleanupCtx, owner, id); releaseErr != nil {
			result = File{}
			err = releaseErr
			return
		}
		if err != nil {
			_, _ = u.metadata.Fail(cleanupCtx, owner, id, failureCode(err))
		}
	}()

	var out *os.File

	if isNew {
		dir, err = os.MkdirTemp(u.root, "upload-"+strconv.FormatInt(id, 10)+"-")
		if err != nil {
			return File{}, uploadError("internal_error")
		}
		out, err = os.OpenFile(filepath.Join(dir, "source"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return File{}, uploadError("internal_error")
		}
	}

	receiveCtx, cancelReceive := context.WithTimeout(ctx, u.receiveTimeout)
	stop := context.AfterFunc(receiveCtx, func() { _ = body.Close() })

	hash := sha256.New()

	var writer io.Writer = hash

	if out != nil {
		writer = io.MultiWriter(out, hash)
	}

	n, readErr := io.Copy(writer, io.LimitReader(body, f.ByteSize+1))
	if out != nil {
		if closeErr := out.Close(); readErr == nil {
			readErr = closeErr
		}
	}

	stop()
	receiveErr := receiveCtx.Err()
	cancelReceive()

	if receiveErr != nil {
		return File{}, uploadError("media_processing_timeout")
	}

	if readErr != nil {
		var large *http.MaxBytesError
		if errors.As(readErr, &large) {
			return File{}, uploadError("body_too_large")
		}
		return File{}, uploadError("invalid_media")
	}

	if n > limit {
		return File{}, uploadError("body_too_large")
	}

	if n != f.ByteSize || hex.EncodeToString(hash.Sum(nil)) != f.SHA256 {
		return File{}, ErrContentMismatch
	}

	if !isNew {
		latest, err := u.metadata.GetFile(ctx, owner, id)
		if err != nil {
			return File{}, err
		}
		if latest.State != Ready && latest.State != Attached {
			return File{}, ErrMediaUnavailable
		}
		return latest, nil
	}

	processCtx, cancelProcess := context.WithTimeout(ctx, u.processTimeout)
	defer cancelProcess()

	outputs, err := u.processor.Process(processCtx, f.Purpose, filepath.Join(dir, "source"), dir)
	if err != nil {
		if processCtx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return File{}, uploadError("media_processing_timeout")
		}
		return File{}, err
	}

	variants := make([]Variant, len(outputs))
	for i, output := range outputs {
		variants[i] = output.Variant
	}

	objects, _, err := completedObjects(f, variants)
	if err != nil {
		return File{}, uploadError("internal_error")
	}

	for i, output := range outputs {
		if err = u.putVariant(processCtx, objects[i].Key, output); err != nil {
			return File{}, err
		}
	}

	if err = os.RemoveAll(dir); err != nil {
		return File{}, uploadError("internal_error")
	}

	dir = ""
	if err = u.metadata.ReleaseSource(processCtx, owner, id); err != nil {
		if processCtx.Err() != nil {
			return File{}, uploadError("media_processing_timeout")
		}
		return File{}, err
	}

	result, err = u.metadata.Complete(processCtx, owner, id, variants)
	if err != nil && processCtx.Err() != nil {
		err = uploadError("media_processing_timeout")
	}

	if err == nil {
		isNew = false
	}

	return result, err
}

func (u *Uploader) putVariant(ctx context.Context, key string, output ProcessedVariant) error {
	normalized, err := os.Open(output.Path)

	if err != nil {
		return uploadError("internal_error")
	}

	info, statErr := normalized.Stat()

	if statErr != nil || !info.Mode().IsRegular() || info.Size() != output.ByteSize {
		normalized.Close()
		return uploadError("internal_error")
	}

	putErr := u.storage.Put(ctx, key, normalized, output.ByteSize, output.ContentType)
	closeErr := normalized.Close()
	
	if putErr != nil {
		if ctx.Err() != nil {
			return uploadError("media_processing_timeout")
		}
		return uploadError("media_storage_unavailable")
	}
	if closeErr != nil {
		return uploadError("internal_error")
	}
	return nil
}

func failureCode(err error) string {
	var failure uploadError
	if errors.As(err, &failure) && validFailureCode(string(failure)) {
		return string(failure)
	}
	if errors.Is(err, ErrContentMismatch) {
		return "invalid_media"
	}
	if failure == "body_too_large" {
		return "media_constraints_exceeded"
	}
	return "media_processing_interrupted"
}
