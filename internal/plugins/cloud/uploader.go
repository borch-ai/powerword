package cloud

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/borch-ai/powerword/pkg/config"
)

// Uploader defines the interface for uploading local files to cloud storage.
type Uploader interface {
	UploadFile(ctx context.Context, localPath string) (string, error)
}

// NewUploader instantiates the concrete uploader based on the configuration provider.
func NewUploader(cfg *config.Config) Uploader {
	if cfg == nil {
		return &NoOpUploader{}
	}
	provider := strings.ToLower(cfg.Plugins.Cloud.Provider)
	switch provider {
	case "gcs", "gcp":
		return &GoogleStorageUploader{cfg: cfg}
	case "s3", "aws":
		return &S3Uploader{cfg: cfg}
	default:
		return &NoOpUploader{}
	}
}

// NoOpUploader fallback when uploader is disabled or not configured.
type NoOpUploader struct{}

// UploadFile returns an error for NoOpUploader.
func (u *NoOpUploader) UploadFile(ctx context.Context, localPath string) (string, error) {
	return "", fmt.Errorf("cloud storage uploader is not configured (provider is set to 'noop')")
}

// GoogleStorageUploader uploads files to Google Cloud Storage.
type GoogleStorageUploader struct {
	cfg *config.Config
}

// UploadFile uploads the file to the configured GCS bucket.
func (u *GoogleStorageUploader) UploadFile(ctx context.Context, localPath string) (string, error) {
	bucketName := u.cfg.Plugins.Cloud.Bucket
	if bucketName == "" {
		return "", fmt.Errorf("GCS bucket name is not configured")
	}

	//nolint:gosec // path is provided by client call
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer func() { _ = file.Close() }()

	opts, err := getGCPOptions(u.cfg)
	if err != nil {
		return "", err
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to initialize GCS client: %w", err)
	}
	defer func() { _ = client.Close() }()

	objectName := generateObjectPath(localPath)

	bkt := client.Bucket(bucketName)
	obj := bkt.Object(objectName)
	wc := obj.NewWriter(ctx)

	if _, err := io.Copy(wc, file); err != nil {
		_ = wc.Close()
		return "", fmt.Errorf("failed to upload object to GCS: %w", err)
	}
	if err := wc.Close(); err != nil {
		return "", fmt.Errorf("failed to close GCS writer: %w", err)
	}

	// Try setting object ACL to public-read.
	// Gracefully swallow Uniform Bucket-Level Access (UBLA) errors where ACL operations are forbidden.
	acl := obj.ACL()
	_ = acl.Set(ctx, storage.AllUsers, storage.RoleReader)

	mockEndpoint := os.Getenv("POWERWORD_CLOUD_MOCK_ENDPOINT")
	if mockEndpoint != "" {
		return fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(mockEndpoint, "/"), bucketName, objectName), nil
	}
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, objectName), nil
}

// S3Uploader uploads files to AWS S3.
type S3Uploader struct {
	cfg *config.Config
}

// UploadFile uploads the file to the configured AWS S3 bucket.
func (u *S3Uploader) UploadFile(ctx context.Context, localPath string) (string, error) {
	bucketName := u.cfg.Plugins.Cloud.Bucket
	if bucketName == "" {
		return "", fmt.Errorf("S3 bucket name is not configured")
	}
	region := u.cfg.Plugins.Cloud.Region
	if region == "" {
		region = "us-east-1"
	}

	//nolint:gosec // path is provided by client call
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open local file %s: %w", localPath, err)
	}
	defer func() { _ = file.Close() }()

	awsCfg, err := getAWSConfig(ctx, u.cfg, region)
	if err != nil {
		return "", err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if mockEndpoint := os.Getenv("POWERWORD_CLOUD_MOCK_ENDPOINT"); mockEndpoint != "" {
			o.BaseEndpoint = aws.String(mockEndpoint)
			o.UsePathStyle = true
		}
	})

	objectName := generateObjectPath(localPath)

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectName),
		Body:   file,
		ACL:    s3types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return "", fmt.Errorf("failed to put S3 object: %w", err)
	}

	mockEndpoint := os.Getenv("POWERWORD_CLOUD_MOCK_ENDPOINT")
	if mockEndpoint != "" {
		return fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(mockEndpoint, "/"), bucketName, objectName), nil
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, objectName), nil
}

// generateObjectPath helper creates a unique path for the object to prevent collision.
func generateObjectPath(localPath string) string {
	fileName := filepath.Base(localPath)
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	randomToken := fmt.Sprintf("%x-%d", randomBytes, time.Now().Unix())
	return fmt.Sprintf("uploads/%s-%s", randomToken, fileName)
}
