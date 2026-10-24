package storage_test

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"thehivesociety/api/internal/storage"
)

// Runs against a local S3-compatible server (CI: SeaweedFS) when
// TEST_S3_ENDPOINT, TEST_S3_ACCESS_KEY and TEST_S3_SECRET_KEY are set.
func s3Config(t *testing.T) storage.Config {
	t.Helper()
	ep := os.Getenv("TEST_S3_ENDPOINT")
	if ep == "" {
		t.Skip("TEST_S3_ENDPOINT not set; skipping S3 integration test")
	}
	return storage.Config{Endpoint: ep, Bucket: "hive-test-photos", AccessKeyID: os.Getenv("TEST_S3_ACCESS_KEY"),
		SecretAccessKey: os.Getenv("TEST_S3_SECRET_KEY"), Region: "us-east-1"}
}

func TestS3RoundTrip(t *testing.T) {
	cfg := s3Config(t)
	s, err := storage.NewS3(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.EnsureBucket(ctx); err != nil {
		t.Fatalf("bucket: %v", err)
	}
	if err := s.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := s.Put(ctx, "avatars/test.jpg", []byte("jpeg-bytes"), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	body, ct, err := s.Get(ctx, "avatars/test.jpg")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	if string(got) != "jpeg-bytes" || ct != "image/jpeg" {
		t.Fatalf("round trip: %q %q", got, ct)
	}
	if err := s.Delete(ctx, "avatars/test.jpg"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, err := s.Get(ctx, "avatars/test.jpg"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("deleted object still readable: %v", err)
	}
	if err := s.Delete(ctx, "avatars/never-existed.jpg"); err != nil {
		t.Fatalf("deleting a missing object must succeed: %v", err)
	}
}

func TestS3RejectsWrongCredentials(t *testing.T) {
	cfg := s3Config(t)
	cfg.SecretAccessKey = "wrong-secret"
	s, err := storage.NewS3(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), "avatars/x.jpg", []byte("x"), "image/jpeg"); err == nil {
		t.Fatal("write with a wrong secret succeeded")
	}
}

func TestS3ConfigValidation(t *testing.T) {
	for _, c := range []storage.Config{{}, {Endpoint: "not a url", Bucket: "b", AccessKeyID: "a", SecretAccessKey: "s"}, {Endpoint: "https://x", AccessKeyID: "a", SecretAccessKey: "s"}} {
		if _, err := storage.NewS3(c); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
}
