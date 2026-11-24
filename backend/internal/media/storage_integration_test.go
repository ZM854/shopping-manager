//go:build integration

package media

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSignedLinkExpiresInSeaweedFS(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	cfg := storageConfig(endpoint)
	cfg.AccessKeyID = os.Getenv("TEST_S3_ACCESS_KEY_ID")
	cfg.SecretKey = os.Getenv("TEST_S3_SECRET_ACCESS_KEY")
	if (endpoint != "http://storage-test:8333" && endpoint != "http://127.0.0.1:58333" && endpoint != "http://localhost:58333") ||
		os.Getenv("TEST_S3_BUCKET") != cfg.Bucket || cfg.AccessKeyID != "shopping-manager-test-key" ||
		cfg.SecretKey != "local-integration-storage-secret-only" {
		t.Fatal("Use the isolated test S3 bucket")
	}
	cfg.RequestTimeout = 8 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := NewStorage(cfg)
	key := "checks/" + uuid.NewString() + ".jpg"
	const data = "test expiring object"
	if err := store.Put(ctx, key, strings.NewReader(data), int64(len(data)), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := store.Delete(ctx, key); err != nil {
			t.Error(err)
		}
	})
	link, expires, err := store.signGet(ctx, key, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	status := func() int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
		if err != nil {
			t.Fatal("Invalid signed URL")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("Failed to retrieve signed object")
		}
		res.Body.Close()
		return res.StatusCode
	}
	if got := status(); got != http.StatusOK {
		t.Fatalf("Fresh signed link rejected with %d", got)
	}
	timer := time.NewTimer(time.Until(expires.Add(time.Second)))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}
	if got := status(); got != http.StatusForbidden {
		t.Fatalf("Expired signed link returned %d", got)
	}
}
