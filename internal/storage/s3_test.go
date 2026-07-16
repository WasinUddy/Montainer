package storage

import (
	"context"
	"strings"
	"testing"
)

func TestNewS3StoreRequiresBucket(t *testing.T) {
	t.Parallel()

	_, err := NewS3Store(context.Background(), S3Config{})
	if err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Fatalf("expected bucket validation error, got %v", err)
	}
}

func TestNewS3StoreRejectsPartialStaticCredentials(t *testing.T) {
	t.Parallel()

	_, err := NewS3Store(context.Background(), S3Config{
		Bucket:      "backups",
		AccessKeyID: "access-only",
	})
	if err == nil || !strings.Contains(err.Error(), "both S3 access key") {
		t.Fatalf("expected partial credential validation error, got %v", err)
	}
}
