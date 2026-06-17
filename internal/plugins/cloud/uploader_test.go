package cloud

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

func TestNewUploader(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantType string
	}{
		{"noop provider", "noop", "*cloud.NoOpUploader"},
		{"empty provider", "", "*cloud.NoOpUploader"},
		{"gcs provider", "gcs", "*cloud.GoogleStorageUploader"},
		{"gcp provider", "gcp", "*cloud.GoogleStorageUploader"},
		{"s3 provider", "s3", "*cloud.S3Uploader"},
		{"aws provider", "aws", "*cloud.S3Uploader"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Plugins.Cloud.Provider = tt.provider
			u := NewUploader(cfg)
			gotType := fmt.Sprintf("%T", u)
			if gotType != tt.wantType {
				t.Errorf("NewUploader(%q) type = %s; want %s", tt.provider, gotType, tt.wantType)
			}
		})
	}
}

func TestNoOpUploader_UploadFile(t *testing.T) {
	u := &NoOpUploader{}
	_, err := u.UploadFile(context.Background(), "some/file.txt")
	if err == nil {
		t.Error("expected NoOpUploader to return an error, got nil")
	}
}

func TestRealUploadersWithMockHTTP(t *testing.T) {
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "test.png")
	if err := os.WriteFile(testFilePath, []byte("fake image content"), 0600); err != nil {
		t.Fatal(err)
	}

	var s3Uploaded int32
	var gcsUploaded int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("--- MOCK UPLOADER REQUEST: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")

		// AWS S3 PutObject (PUT request to /<bucket>/<key>)
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/test-bucket/uploads/") {
			atomic.StoreInt32(&s3Uploaded, 1)
			w.WriteHeader(http.StatusOK)
			return
		}

		// GCP GCS Upload (POST to resumable upload URL or JSON API endpoint)
		if r.Method == "POST" && (strings.Contains(r.URL.Path, "/b/test-bucket/o") || strings.Contains(r.URL.Path, "/upload/storage/v1/b/test-bucket/o")) {
			atomic.StoreInt32(&gcsUploaded, 1)
			resp := `{
				"kind": "storage#object",
				"name": "uploads/mocked-object-name",
				"bucket": "test-bucket"
			}`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}

		// GCP GCS ACL Set (PUT to /b/test-bucket/o/<objectName>/acl/...)
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/acl") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("POWERWORD_CLOUD_MOCK_ENDPOINT", server.URL)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

	cfg := &config.Config{}
	cfg.Plugins.Cloud.Region = "us-east-1"
	cfg.Plugins.Cloud.Bucket = "test-bucket"

	ctx := context.Background()

	// 1. Test GCS Uploader
	cfg.Plugins.Cloud.Provider = "gcs"
	gcsUploader := NewUploader(cfg)
	urlGCS, err := gcsUploader.UploadFile(ctx, testFilePath)
	if err != nil {
		t.Fatalf("GoogleStorageUploader.UploadFile failed: %v", err)
	}
	if atomic.LoadInt32(&gcsUploaded) != 1 {
		t.Error("expected GCS mock upload to be triggered")
	}
	if !strings.Contains(urlGCS, "test-bucket/uploads/") {
		t.Errorf("unexpected GCS upload URL: %s", urlGCS)
	}

	// 2. Test S3 Uploader
	cfg.Plugins.Cloud.Provider = "s3"
	s3Uploader := NewUploader(cfg)
	urlS3, err := s3Uploader.UploadFile(ctx, testFilePath)
	if err != nil {
		t.Fatalf("S3Uploader.UploadFile failed: %v", err)
	}
	if atomic.LoadInt32(&s3Uploaded) != 1 {
		t.Error("expected S3 mock upload to be triggered")
	}
	if !strings.Contains(urlS3, "test-bucket/uploads/") {
		t.Errorf("unexpected S3 upload URL: %s", urlS3)
	}
}

func TestUploadFileErrors(t *testing.T) {
	ctx := context.Background()

	// 1. Missing GCS bucket name
	cfgGCS := &config.Config{}
	cfgGCS.Plugins.Cloud.Provider = "gcs"
	gcsUploader := NewUploader(cfgGCS)
	_, err := gcsUploader.UploadFile(ctx, "nonexistent.png")
	if err == nil || !strings.Contains(err.Error(), "bucket name is not configured") {
		t.Errorf("expected missing bucket error, got: %v", err)
	}

	// 2. Missing S3 bucket name
	cfgS3 := &config.Config{}
	cfgS3.Plugins.Cloud.Provider = "s3"
	s3Uploader := NewUploader(cfgS3)
	_, err = s3Uploader.UploadFile(ctx, "nonexistent.png")
	if err == nil || !strings.Contains(err.Error(), "bucket name is not configured") {
		t.Errorf("expected missing bucket error, got: %v", err)
	}

	// 3. Local file does not exist (GCS)
	cfgGCS.Plugins.Cloud.Bucket = "test-bucket"
	_, err = gcsUploader.UploadFile(ctx, "nonexistent.png")
	if err == nil || !strings.Contains(err.Error(), "failed to open local file") {
		t.Errorf("expected file not found error, got: %v", err)
	}

	// 4. Local file does not exist (S3)
	cfgS3.Plugins.Cloud.Bucket = "test-bucket"
	_, err = s3Uploader.UploadFile(ctx, "nonexistent.png")
	if err == nil || !strings.Contains(err.Error(), "failed to open local file") {
		t.Errorf("expected file not found error, got: %v", err)
	}
}

func TestNewUploader_NonexistentCredentialsPath(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.Cloud.Provider = "s3"
	cfg.Plugins.Cloud.CredentialsPath = "/nonexistent/credentials/file/path"

	u := NewUploader(cfg)
	if _, ok := u.(*NoOpUploader); !ok {
		t.Errorf("expected NoOpUploader when credentials path does not exist, got %T", u)
	}
}

func TestGenerateObjectPath_RandError(t *testing.T) {
	oldRandRead := randRead
	defer func() { randRead = oldRandRead }()
	randRead = func(b []byte) (int, error) {
		return 0, fmt.Errorf("mock random reader error")
	}

	path := generateObjectPath("/path/to/test.txt")
	if !strings.Contains(path, "uploads/fallback-") {
		t.Errorf("expected path to contain fallback prefix, got: %s", path)
	}
}

func TestGoogleStorageUBLAAndACLErrors(t *testing.T) {
	tempDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == "POST" && (strings.Contains(r.URL.Path, "/b/test-bucket/o") || strings.Contains(r.URL.Path, "/upload/storage/v1/b/test-bucket/o")) {
			resp := `{
				"kind": "storage#object",
				"name": "uploads/mocked-object-name",
				"bucket": "test-bucket"
			}`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}

		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/acl") {
			if strings.Contains(r.URL.Path, "ubla") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "Cannot use ACL API if uniform bucket-level access is enabled on the bucket"}}`))
				return
			}
			if strings.Contains(r.URL.Path, "acl-err") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error": {"code": 400, "message": "Permission denied setting ACL"}}`))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Setenv("POWERWORD_CLOUD_MOCK_ENDPOINT", server.URL)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project-123")

	cfg := &config.Config{}
	cfg.Plugins.Cloud.Region = "us-east-1"
	cfg.Plugins.Cloud.Bucket = "test-bucket"
	cfg.Plugins.Cloud.Provider = "gcs"
	gcsUploader := NewUploader(cfg)

	ctx := context.Background()

	// Case 1: Uniform Bucket-Level Access (swallowed error, should succeed)
	ublaPath := filepath.Join(tempDir, "test-ubla.png")
	_ = os.WriteFile(ublaPath, []byte("fake image content"), 0600)
	url, err := gcsUploader.UploadFile(ctx, ublaPath)
	if err != nil {
		t.Fatalf("expected GCS upload to succeed under Uniform Bucket-Level Access, got: %v", err)
	}
	if !strings.Contains(url, "test-bucket/uploads/") {
		t.Errorf("unexpected URL: %s", url)
	}

	// Case 2: Other GCS ACL error (should fail)
	aclErrPath := filepath.Join(tempDir, "test-acl-err.png")
	_ = os.WriteFile(aclErrPath, []byte("fake image content"), 0600)
	_, err = gcsUploader.UploadFile(ctx, aclErrPath)
	if err == nil || !strings.Contains(err.Error(), "failed to set GCS object ACL") {
		t.Errorf("expected GCS ACL set error, got: %v", err)
	}
}
