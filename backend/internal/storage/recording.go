package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// RecordingStorage abstracts where call recordings are persisted and how
// public/pre-signed URLs are generated. Implementations may use OCI, S3, or
// local filesystem.
type RecordingStorage interface {
	// Store persists the recording data under the given key and returns the
	// storage-level identifier.
	Store(ctx context.Context, key string, data io.Reader) (string, error)

	// GetURL returns a URL that can be used to download the recording. If the
	// implementation supports pre-signed URLs, expiry controls the lifetime.
	GetURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// HealthCheck returns an error if the storage backend is not reachable.
	HealthCheck(ctx context.Context) error

	// Name returns a short identifier for logs/metrics.
	Name() string
}

// recordingKey sanitizes and prefixes a recording path so all objects live in
// a single namespace (e.g. "recordings/").
func recordingKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		key = "recording"
	}
	if !strings.HasPrefix(key, "recordings/") {
		key = "recordings/" + key
	}
	return key
}

// OCIRecordingStorage wraps OCIClient to satisfy RecordingStorage.
type OCIRecordingStorage struct {
	client *OCIClient
}

// NewOCIRecordingStorage creates a RecordingStorage backed by OCI.
func NewOCIRecordingStorage(client *OCIClient) *OCIRecordingStorage {
	return &OCIRecordingStorage{client: client}
}

// Name returns "oci".
func (s *OCIRecordingStorage) Name() string { return "oci" }

// Store reads all data and delegates to the OCI client.
func (s *OCIRecordingStorage) Store(ctx context.Context, key string, data io.Reader) (string, error) {
	b, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("read recording data: %w", err)
	}
	storedKey := recordingKey(key)
	url, err := s.client.UploadPublic(ctx, storedKey, b)
	if err != nil {
		return "", fmt.Errorf("oci store: %w", err)
	}
	return url, nil
}

// GetURL returns the public native OCI URL for the recording. OCI public
// buckets do not require pre-signing; expiry is ignored.
func (s *OCIRecordingStorage) GetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	_ = ctx
	_ = expiry
	storedKey := recordingKey(key)
	return s.client.PublicURL(storedKey), nil
}

// HealthCheck attempts a lightweight metadata operation. For OCI this is a
// HeadObject on a known sentinel key; if the bucket does not exist or the
// credentials are invalid, the call fails.
func (s *OCIRecordingStorage) HealthCheck(ctx context.Context) error {
	return s.client.HealthCheck(ctx)
}
