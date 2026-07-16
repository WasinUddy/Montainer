package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config configures AWS S3 or an S3-compatible object store. Endpoint may be
// left empty for AWS itself; custom endpoints use path-style addressing by
// default for compatibility with services such as MinIO.
type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	UsePathStyle    bool
}

// ObjectStore is the small contract needed by the backup service.
type ObjectStore interface {
	Upload(ctx context.Context, key string, body io.Reader, size int64) error
}

type S3Store struct {
	bucket   string
	uploader *transfermanager.Client
}

func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
			return nil, fmt.Errorf("both S3 access key ID and secret access key are required when either is set")
		}
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
			options.UsePathStyle = true
		}
		if cfg.UsePathStyle {
			options.UsePathStyle = true
		}
	})

	return &S3Store{
		bucket:   cfg.Bucket,
		uploader: transfermanager.New(client),
	}, nil
}

func (s *S3Store) Upload(ctx context.Context, key string, body io.Reader, size int64) error {
	if s == nil || s.uploader == nil {
		return fmt.Errorf("S3 object store is not configured")
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("S3 object key is required")
	}

	_, err := s.uploader.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String("application/zip"),
	})
	if err != nil {
		return fmt.Errorf("upload backup %q: %w", key, err)
	}
	return nil
}
