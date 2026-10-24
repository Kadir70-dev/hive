// Package storage keeps private objects (profile photos) in an S3-compatible
// bucket such as Cloudflare R2. Objects are never public: the API streams
// them to authorised users, so bucket credentials never reach a browser.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("object not found")

// Store is the minimal object API the application needs.
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

// Config for an S3-compatible endpoint. For R2: Endpoint
// "https://<account-id>.r2.cloudflarestorage.com", Region "auto".
type Config struct {
	Endpoint, Bucket, AccessKeyID, SecretAccessKey, Region string
}

// S3 talks to R2 (or any S3-compatible service) with path-style requests.
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 validates configuration and builds a client. It does not contact the
// service; call Ping to check reachability and credentials.
func NewS3(c Config) (*S3, error) {
	if c.Endpoint == "" || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return nil, errors.New("R2_ENDPOINT, R2_BUCKET, R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY are all required")
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("R2_ENDPOINT must be an absolute URL")
	}
	if c.Region == "" {
		c.Region = "auto"
	}
	client, err := minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, ""),
		Secure:       u.Scheme == "https",
		Region:       c.Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, err
	}
	return &S3{client: client, bucket: c.Bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType, CacheControl: "private, no-store"})
	return err
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", err
	}
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	return obj, info.ContentType, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil && minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return nil
	}
	return err
}

// Ping checks credentials and that the bucket exists.
func (s *S3) Ping(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bucket %q does not exist", s.bucket)
	}
	return nil
}

// EnsureBucket creates the bucket if missing (used by tests and local dev only).
func (s *S3) EnsureBucket(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil || ok {
		return err
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

// Memory is an in-process Store for tests.
type Memory struct {
	mu         sync.Mutex
	objects    map[string]memObject
	FailPut    error
	FailDelete error
}

type memObject struct {
	data []byte
	ct   string
}

func NewMemory() *Memory { return &Memory{objects: map[string]memObject{}} }

func (m *Memory) Put(_ context.Context, key string, data []byte, ct string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailPut != nil {
		return m.FailPut
	}
	m.objects[key] = memObject{append([]byte(nil), data...), ct}
	return nil
}

func (m *Memory) Get(_ context.Context, key string) (io.ReadCloser, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objects[key]
	if !ok {
		return nil, "", ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(o.data)), o.ct, nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailDelete != nil {
		return m.FailDelete
	}
	delete(m.objects, key)
	return nil
}

func (m *Memory) Ping(context.Context) error { return nil }

// Keys lists stored keys (tests).
func (m *Memory) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k := range m.objects {
		out = append(out, k)
	}
	return out
}

// SafeKey reports whether a key was produced by this application.
func SafeKey(key string) bool {
	return strings.HasPrefix(key, "avatars/") && !strings.Contains(key, "..") && len(key) < 100
}
