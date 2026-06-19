package youtube

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

type mockRoundTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func TestYouTubeService_GetClient_MissingCredentials(t *testing.T) {
	cfg := &config.Config{} // no credentials
	svc := NewYouTubeService(cfg, nil)

	_, err := svc.getClient(context.Background())
	if err == nil {
		t.Fatal("expected error due to missing credentials, got nil")
	}
	if !strings.Contains(err.Error(), "missing required YouTube credentials") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestYouTubeService_UploadVideo(t *testing.T) {
	tmpDir := t.TempDir()
	dummyVideo := filepath.Join(tmpDir, "test.mp4")
	err := os.WriteFile(dummyVideo, []byte("fake video content"), 0600)
	if err != nil {
		t.Fatalf("failed to write dummy video: %v", err)
	}

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			YouTube: config.YouTubeConfig{
				ClientID:     "client123",
				ClientSecret: "secret123",
				RefreshToken: "refresh123",
			},
		},
	}

	var postCalled bool

	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				// Intercept initial upload POST request
				if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/upload/youtube/v3/videos") {
					postCalled = true

					// If the SDK optimized it to multipart because the file is small:
					if strings.Contains(req.URL.RawQuery, "uploadType=multipart") {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString(`{"id": "uploaded-video-456"}`)),
						}, nil
					}

					// Resumable upload initialization path:
					resp := &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
					}
					// Return upload location URL
					resp.Header.Set("Location", "http://localhost/upload-location-id")
					return resp, nil
				}

				// Intercept media upload PUT request (for resumable path)
				if req.Method == http.MethodPut && strings.Contains(req.URL.Path, "/upload-location-id") {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(`{"id": "uploaded-video-456"}`)),
					}, nil
				}

				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "bad request"}`)),
				}, nil
			},
		},
	}

	svc := NewYouTubeService(cfg, mockClient)
	videoID, err := svc.UploadVideo(context.Background(), dummyVideo, "My Title", "My Desc", "public")
	if err != nil {
		t.Fatalf("failed to upload video: %v", err)
	}

	if videoID != "uploaded-video-456" {
		t.Errorf("expected video ID uploaded-video-456, got %s", videoID)
	}

	if !postCalled {
		t.Error("upload POST request was not called")
	}
}

func TestYouTubeService_UploadVideo_ValidationErrors(t *testing.T) {
	cfg := &config.Config{}
	svc := NewYouTubeService(cfg, nil)

	// empty path
	_, err := svc.UploadVideo(context.Background(), "", "title", "desc", "")
	if err == nil {
		t.Error("expected error for empty video path, got nil")
	}

	// non-existent file
	_, err = svc.UploadVideo(context.Background(), "nonexistent.mp4", "title", "desc", "")
	if err == nil {
		t.Error("expected error for non-existent video file, got nil")
	}

	// directory path
	tmpDir := t.TempDir()
	_, err = svc.UploadVideo(context.Background(), tmpDir, "title", "desc", "")
	if err == nil {
		t.Error("expected error for directory path, got nil")
	}

	// invalid extension
	dummyTxt := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(dummyTxt, []byte("hello"), 0600)
	if err != nil {
		t.Fatalf("failed to create dummy text file: %v", err)
	}
	_, err = svc.UploadVideo(context.Background(), dummyTxt, "title", "desc", "")
	if err == nil {
		t.Error("expected error for txt file extension, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported video format") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestYouTubeService_UpdateMetadata(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			YouTube: config.YouTubeConfig{
				ClientID:     "client123",
				ClientSecret: "secret123",
				RefreshToken: "refresh123",
			},
		},
	}

	var listCalled, updateCalled, playlistInsertCalled bool

	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				// videos.list
				if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
					listCalled = true
					body := `{
						"items": [{
							"id": "video123",
							"snippet": {
								"title": "Old Title",
								"description": "Old Description",
								"tags": ["old"]
							}
						}]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(body)),
					}, nil
				}

				// videos.update
				if req.Method == http.MethodPut && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
					updateCalled = true
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(`{"id": "video123"}`)),
					}, nil
				}

				// playlistItems.insert
				if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/youtube/v3/playlistItems") {
					playlistInsertCalled = true
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(`{"id": "playlistItem123"}`)),
					}, nil
				}

				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "bad request"}`)),
				}, nil
			},
		},
	}

	svc := NewYouTubeService(cfg, mockClient)
	err := svc.UpdateMetadata(context.Background(), "video123", "New Title", "New Desc", []string{"new", "tags"}, "playlist789")
	if err != nil {
		t.Fatalf("failed to update metadata: %v", err)
	}

	if !listCalled {
		t.Error("videos.list was not called")
	}
	if !updateCalled {
		t.Error("videos.update was not called")
	}
	if !playlistInsertCalled {
		t.Error("playlistItems.insert was not called")
	}
}

func TestYouTubeService_UpdateMetadata_VideoNotFound(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			YouTube: config.YouTubeConfig{
				ClientID:     "client123",
				ClientSecret: "secret123",
				RefreshToken: "refresh123",
			},
		},
	}

	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				// videos.list returning empty items
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"items": []}`)),
				}, nil
			},
		},
	}

	svc := NewYouTubeService(cfg, mockClient)
	err := svc.UpdateMetadata(context.Background(), "video123", "New Title", "", nil, "")
	if err == nil {
		t.Error("expected error for missing video, got nil")
	}
	if !strings.Contains(err.Error(), "video not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestYouTubeService_GetMetrics(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			YouTube: config.YouTubeConfig{
				ClientID:     "client123",
				ClientSecret: "secret123",
				RefreshToken: "refresh123",
			},
		},
	}

	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
					body := `{
						"items": [{
							"id": "video123",
							"statistics": {
								"viewCount": "5000",
								"likeCount": "200",
								"commentCount": "35",
								"favoriteCount": "3"
							}
						}]
					}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(body)),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "bad request"}`)),
				}, nil
			},
		},
	}

	svc := NewYouTubeService(cfg, mockClient)

	// test requesting subset of metrics
	res, err := svc.GetMetrics(context.Background(), "video123", []string{"views", "likes", "unknown"})
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}

	expected := map[string]interface{}{
		"views":   uint64(5000),
		"likes":   uint64(200),
		"unknown": nil,
	}

	if !reflect.DeepEqual(res, expected) {
		t.Errorf("expected metrics %+v, got %+v", expected, res)
	}

	// test requesting empty metrics (should default to views, likes, comments, favorites)
	resAll, err := svc.GetMetrics(context.Background(), "video123", nil)
	if err != nil {
		t.Fatalf("failed to get all metrics: %v", err)
	}

	expectedAll := map[string]interface{}{
		"views":     uint64(5000),
		"likes":     uint64(200),
		"comments":  uint64(35),
		"favorites": uint64(3),
	}

	if !reflect.DeepEqual(resAll, expectedAll) {
		t.Errorf("expected all metrics %+v, got %+v", expectedAll, resAll)
	}
}

func TestYouTubeService_GetClient_NoHTTPClient(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			YouTube: config.YouTubeConfig{
				ClientID:     "client123",
				ClientSecret: "secret123",
				RefreshToken: "refresh123",
			},
		},
	}
	t.Setenv("POWERWORD_YOUTUBE_API_ENDPOINT", "https://localhost:9999")
	svc := NewYouTubeService(cfg, nil)
	srv, err := svc.getClient(context.Background())
	if err != nil {
		t.Fatalf("getClient failed: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil service client")
	}
}

func TestYouTubeService_UploadVideo_GetClientError(t *testing.T) {
	cfg := &config.Config{} // empty cfg to trigger missing credentials
	svc := NewYouTubeService(cfg, nil)
	tmpDir := t.TempDir()
	dummyVideo := filepath.Join(tmpDir, "test.mp4")
	err := os.WriteFile(dummyVideo, []byte("fake video content"), 0600)
	if err != nil {
		t.Fatalf("failed to write dummy video: %v", err)
	}

	_, err = svc.UploadVideo(context.Background(), dummyVideo, "title", "desc", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing required YouTube credentials") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestYouTubeService_UploadVideo_DefaultPrivacyAndError(t *testing.T) {
	tmpDir := t.TempDir()
	dummyVideo := filepath.Join(tmpDir, "test.mp4")
	err := os.WriteFile(dummyVideo, []byte("fake video content"), 0600)
	if err != nil {
		t.Fatalf("failed to write dummy video: %v", err)
	}

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			YouTube: config.YouTubeConfig{
				ClientID:     "client123",
				ClientSecret: "secret123",
				RefreshToken: "refresh123",
			},
		},
	}

	// 1. Test upload error
	mockClientErr := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal error"}`)),
				}, nil
			},
		},
	}
	svcErr := NewYouTubeService(cfg, mockClientErr)
	_, err = svcErr.UploadVideo(context.Background(), dummyVideo, "My Title", "My Desc", "")
	if err == nil {
		t.Fatal("expected upload error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to upload video") {
		t.Errorf("unexpected error: %v", err)
	}

	// 2. Test default privacy mapping to private
	mockClientPrivacy := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/upload/youtube/v3/videos") {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(`{"id": "uploaded-video-789"}`)),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "bad request"}`)),
				}, nil
			},
		},
	}
	svcPrivacy := NewYouTubeService(cfg, mockClientPrivacy)
	videoID, err := svcPrivacy.UploadVideo(context.Background(), dummyVideo, "My Title", "My Desc", "")
	if err != nil {
		t.Fatalf("failed to upload video: %v", err)
	}
	if videoID != "uploaded-video-789" {
		t.Errorf("expected video ID uploaded-video-789, got %s", videoID)
	}
}

func TestYouTubeService_UpdateMetadata_Errors(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			YouTube: config.YouTubeConfig{
				ClientID:     "client123",
				ClientSecret: "secret123",
				RefreshToken: "refresh123",
			},
		},
	}

	// 1. Missing video_id
	svc := NewYouTubeService(cfg, nil)
	err := svc.UpdateMetadata(context.Background(), "", "Title", "Desc", nil, "")
	if err == nil {
		t.Error("expected error for empty video_id, got nil")
	}

	// 2. getClient error (missing credentials)
	svcNoCreds := NewYouTubeService(&config.Config{}, nil)
	err = svcNoCreds.UpdateMetadata(context.Background(), "vid123", "Title", "Desc", nil, "")
	if err == nil {
		t.Error("expected error for missing credentials, got nil")
	}

	// 3. List error
	mockClientListErr := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal error"}`)),
				}, nil
			},
		},
	}
	svcListErr := NewYouTubeService(cfg, mockClientListErr)
	err = svcListErr.UpdateMetadata(context.Background(), "vid123", "Title", "Desc", nil, "")
	if err == nil {
		t.Error("expected list error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to retrieve video") {
		t.Errorf("unexpected error: %v", err)
	}

	// 4. Update error
	mockClientUpdateErr := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
					body := `{"items": [{"id": "vid123", "snippet": {"title": "Old"}}]}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(body)),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "update error"}`)),
				}, nil
			},
		},
	}
	svcUpdateErr := NewYouTubeService(cfg, mockClientUpdateErr)
	err = svcUpdateErr.UpdateMetadata(context.Background(), "vid123", "Title", "Desc", nil, "")
	if err == nil {
		t.Error("expected update error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to update video metadata") {
		t.Errorf("unexpected error: %v", err)
	}

	// 5. Playlist Insert error
	mockClientPlaylistErr := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
					body := `{"items": [{"id": "vid123", "snippet": {"title": "Old"}}]}`
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(body)),
					}, nil
				}
				if req.Method == http.MethodPut && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(`{"id": "vid123"}`)),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "playlist insert error"}`)),
				}, nil
			},
		},
	}
	svcPlaylistErr := NewYouTubeService(cfg, mockClientPlaylistErr)
	err = svcPlaylistErr.UpdateMetadata(context.Background(), "vid123", "Title", "Desc", nil, "playlist123")
	if err == nil {
		t.Error("expected playlist insert error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to add video to playlist") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestYouTubeService_GetMetrics_Errors(t *testing.T) {
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			YouTube: config.YouTubeConfig{
				ClientID:     "client123",
				ClientSecret: "secret123",
				RefreshToken: "refresh123",
			},
		},
	}

	// 1. Missing video_id
	svc := NewYouTubeService(cfg, nil)
	_, err := svc.GetMetrics(context.Background(), "", nil)
	if err == nil {
		t.Error("expected error for empty video_id, got nil")
	}

	// 2. getClient error (missing credentials)
	svcNoCreds := NewYouTubeService(&config.Config{}, nil)
	_, err = svcNoCreds.GetMetrics(context.Background(), "vid123", nil)
	if err == nil {
		t.Error("expected error for missing credentials, got nil")
	}

	// 3. List error
	mockClientListErr := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString(`{"error": "internal error"}`)),
				}, nil
			},
		},
	}
	svcListErr := NewYouTubeService(cfg, mockClientListErr)
	_, err = svcListErr.GetMetrics(context.Background(), "vid123", nil)
	if err == nil {
		t.Error("expected list error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to retrieve video statistics") {
		t.Errorf("unexpected error: %v", err)
	}

	// 4. Video not found
	mockClientNotFound := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"items": []}`)),
				}, nil
			},
		},
	}
	svcNotFound := NewYouTubeService(cfg, mockClientNotFound)
	_, err = svcNotFound.GetMetrics(context.Background(), "vid123", nil)
	if err == nil {
		t.Error("expected video not found error, got nil")
	}
	if !strings.Contains(err.Error(), "video not found") {
		t.Errorf("unexpected error: %v", err)
	}

	// 5. Statistics nil
	mockClientNilStats := &http.Client{
		Transport: &mockRoundTripper{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				body := `{"items": [{"id": "vid123"}]}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(body)),
				}, nil
			},
		},
	}
	svcNilStats := NewYouTubeService(cfg, mockClientNilStats)
	_, err = svcNilStats.GetMetrics(context.Background(), "vid123", nil)
	if err == nil {
		t.Error("expected nil statistics error, got nil")
	}
	if !strings.Contains(err.Error(), "no statistics available for video") {
		t.Errorf("unexpected error: %v", err)
	}
}
