package media

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/logging"
)

var ErrObjectNotFound = errors.New("media object not found")

type Storage struct {
	client *s3.Client
	signer *s3.PresignClient
	bucket string
}

func NewStorage(cfg config.MediaConfig) *Storage {
	options := s3.Options{
		Region:       cfg.Region,
		BaseEndpoint: aws.String(cfg.Endpoint),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, ""),
		HTTPClient: &http.Client{
			Timeout: cfg.RequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		Logger:           logging.NewStandardLogger(io.Discard),
		RetryMaxAttempts: 2,
	}
	client := s3.New(options)
	options.BaseEndpoint = aws.String(cfg.PublicEndpoint)
	return &Storage{client: client, signer: s3.NewPresignClient(s3.New(options)), bucket: cfg.Bucket}
}

func (s *Storage) Check(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	return storageError("check", err)
}

func (s *Storage) ConfigureBrowserAccess(ctx context.Context, origins []string) error {
	_, err := s.client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(s.bucket),
		CORSConfiguration: &types.CORSConfiguration{CORSRules: []types.CORSRule{{
			AllowedOrigins: origins,
			AllowedMethods: []string{"GET", "HEAD"},
			AllowedHeaders: []string{"Range"},
			ExposeHeaders:  []string{"Accept-Ranges", "Content-Range"},
			MaxAgeSeconds:  aws.Int32(300),
		}}},
	})
	return storageError("cors", err)
}

func (s *Storage) SignGet(ctx context.Context, key string) (string, time.Time, error) {
	return s.signGet(ctx, key, AccessTTL)
}

func (s *Storage) signGet(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	signed, err := s.signer.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", time.Time{}, storageError("sign", err)
	}
	u, err := url.Parse(signed.URL)
	if err != nil {
		return "", time.Time{}, storageError("sign", err)
	}
	at, err := time.Parse("20060102T150405Z", u.Query().Get("X-Amz-Date"))
	if err != nil {
		return "", time.Time{}, storageError("sign", err)
	}
	return signed.URL, at.Add(ttl), nil
}

func (s *Storage) Put(ctx context.Context, key string, body io.ReadSeeker, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:             aws.String(s.bucket),
		Key:                aws.String(key),
		Body:               body,
		ContentLength:      aws.Int64(size),
		ContentType:        aws.String(contentType),
		CacheControl:       aws.String("private, no-store"),
		ContentDisposition: aws.String("inline"),
	})
	return storageError("put", err)
}

// The caller must close the body. HTTPClient.Timeout also bounds body reads.
func (s *Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		return nil, storageError("get", err)
	}
	return result.Body, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	return storageError("delete", err)
}

type safeStorageError struct {
	op    string
	cause error
}

func (e *safeStorageError) Error() string { return "media storage " + e.op + " failed" }
func (e *safeStorageError) Unwrap() error { return e.cause }

func storageError(op string, err error) error {
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchKey" {
		return ErrObjectNotFound
	}
	return &safeStorageError{op: op, cause: err}
}
