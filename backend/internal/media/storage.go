package media

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/logging"
)

var ErrObjectNotFound = errors.New("media object not found")

type Storage struct {
	client *s3.Client
	bucket string
}

// NewStorage uses explicit credentials only: no environment/profile/EC2 discovery,
// SDK request logs or redirects carrying signed requests to another endpoint.
func NewStorage(cfg config.MediaConfig) *Storage {
	client := s3.New(s3.Options{
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
	})
	return &Storage{client: client, bucket: cfg.Bucket}
}

func (s *Storage) Check(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	return storageError("check", err)
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
