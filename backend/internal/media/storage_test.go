package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/config"
)

func storageConfig(endpoint string) config.MediaConfig {
	return config.MediaConfig{
		Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1",
		Bucket: "shopping-manager-media-test", AccessKeyID: "synthetic-test-key",
		SecretKey: strings.Repeat("s", 32), RequestTimeout: time.Second,
	}
}

func TestStorageSignsRequestsAndKeepsFilesPrivate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("unsigned S3 request")
		}
		if r.URL.Path != "/shopping-manager-media-test/check.jpg" {
			t.Error("expected path-style bucket and key")
		}
		if r.Method == http.MethodPut {
			if r.Header.Get("Cache-Control") != "private, no-store" ||
				r.Header.Get("Content-Type") != "image/jpeg" || r.Header.Get("Content-Disposition") != "inline" {
				t.Error("unsafe object metadata")
			}
			data, _ := io.ReadAll(r.Body)
			if !bytes.Contains(data, []byte("test-object")) {
				t.Error("object body lost")
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store := NewStorage(storageConfig(server.URL))
	if err := store.Put(context.Background(), "check.jpg", strings.NewReader("test-object"), 11, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
}

func TestStorageDoesNotFollowRedirectsOrDiscloseDependencyErrors(t *testing.T) {
	var redirected bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected = true
	}))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", destination.URL+"/?token=synthetic-secret")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	store := NewStorage(storageConfig(server.URL))
	if err := store.Check(context.Background()); err == nil ||
		strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), server.URL) {
		t.Fatal("dependency failure suppressed or disclosed")
	}
	if redirected {
		t.Fatal("signed request followed a redirect")
	}
	cause := errors.New("synthetic-secret")
	err := storageError("put", cause)
	if err.Error() != "media storage put failed" || !errors.Is(err, cause) {
		t.Fatal("safe error lost its cause")
	}
}

func TestStorageRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewStorage(storageConfig("http://127.0.0.1:1")).Check(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}
