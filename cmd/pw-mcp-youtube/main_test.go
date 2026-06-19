package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/youtube"
	"github.com/borch-ai/powerword/pkg/config"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func assertResponse(t *testing.T, res *mcp.CallToolResult, wantError bool, wantSubstr string) {
	t.Helper()
	if res.IsError != wantError {
		t.Errorf("expected error: %v, got: %v (content: %v)", wantError, res.IsError, res.Content)
	}
	if len(res.Content) == 0 {
		t.Errorf("no content in response")
		return
	}
	var contentStr string
	if txt, ok := res.Content[0].(*mcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(res.Content[0])
	}
	if !strings.Contains(contentStr, wantSubstr) {
		t.Errorf("expected response to contain %q, got %q", wantSubstr, contentStr)
	}
}

func startTestServer(t *testing.T, workspaceRoot string, svc *youtube.YouTubeService) (*mcp.ClientSession, context.Context, func()) {
	t.Helper()
	cfg := &config.Config{}
	srv, err := setupServer(workspaceRoot, cfg, svc)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(fmt.Errorf("server run err: %v", runErr))
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		cancel()
		t.Fatalf("failed to connect client: %v", err)
	}

	cleanup := func() {
		_ = session.Close()
		cancel()
	}
	return session, ctx, cleanup
}

func TestYouTube_MCP_UploadVideo(t *testing.T) {
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

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/upload/youtube/v3/videos") {
				respBody := `{"id": "uploaded-video-456"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
					Header:     make(http.Header),
				}, nil
			}
			return nil, fmt.Errorf("unexpected url: %s", req.URL.String())
		}),
	}

	svc := youtube.NewYouTubeService(cfg, mockClient)
	session, ctx, cleanup := startTestServer(t, tmpDir, svc)
	defer cleanup()

	// Test 1: Upload Video Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "youtube_upload_video",
		Arguments: json.RawMessage(fmt.Sprintf(`{
			"video_path": %q,
			"title": "Test Title",
			"description": "Test Desc",
			"privacy": "private"
		}`, dummyVideo)),
	})
	if err != nil {
		t.Fatalf("CallTool youtube_upload_video failed: %v", err)
	}
	assertResponse(t, res, false, "uploaded-video-456")

	// Test 1.5: Upload Video Success with Relative Path
	relVideo := "relative_test.mp4"
	err = os.WriteFile(filepath.Join(tmpDir, relVideo), []byte("fake relative video content"), 0600)
	if err != nil {
		t.Fatalf("failed to write dummy relative video: %v", err)
	}

	resRel, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "youtube_upload_video",
		Arguments: json.RawMessage(fmt.Sprintf(`{
			"video_path": %q,
			"title": "Test Title Relative",
			"description": "Test Desc",
			"privacy": "private"
		}`, relVideo)),
	})
	if err != nil {
		t.Fatalf("CallTool youtube_upload_video with relative path failed: %v", err)
	}
	assertResponse(t, resRel, false, "uploaded-video-456")

	// Test 2: Upload Video Error (missing required video_path)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "youtube_upload_video",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool youtube_upload_video error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "video_path parameter is required")
}

func TestYouTube_MCP_UpdateMetadata(t *testing.T) {
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
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"items":[{"id":"video123","snippet":{"title":"Old"}}]}`)),
					Header:     make(http.Header),
				}, nil
			}
			if req.Method == http.MethodPut && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"id":"video123"}`)),
					Header:     make(http.Header),
				}, nil
			}
			return nil, fmt.Errorf("unexpected url: %s", req.URL.String())
		}),
	}

	svc := youtube.NewYouTubeService(cfg, mockClient)
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: Update Metadata Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "youtube_update_metadata",
		Arguments: json.RawMessage(`{
			"video_id": "video123",
			"title": "New Title",
			"description": "New Desc",
			"tags": ["t1", "t2"]
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool youtube_update_metadata failed: %v", err)
	}
	assertResponse(t, res, false, "updated successfully")

	// Test 2: Update Metadata Error (missing video_id)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "youtube_update_metadata",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool youtube_update_metadata error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "video_id parameter is required")
}

func TestYouTube_MCP_GetMetrics(t *testing.T) {
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
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/youtube/v3/videos") {
				body := `{"items":[{"id":"video123","statistics":{"viewCount":"5000","likeCount":"200"}}]}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     make(http.Header),
				}, nil
			}
			return nil, fmt.Errorf("unexpected url: %s", req.URL.String())
		}),
	}

	svc := youtube.NewYouTubeService(cfg, mockClient)
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: Get Metrics Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "youtube_get_metrics",
		Arguments: json.RawMessage(`{
			"video_id": "video123",
			"metrics": ["views", "likes"]
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool youtube_get_metrics failed: %v", err)
	}
	assertResponse(t, res, false, "views")
	assertResponse(t, res, false, "likes")

	// Test 2: Get Metrics Error (missing video_id)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "youtube_get_metrics",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool youtube_get_metrics error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "video_id parameter is required")
}

func TestYouTube_MCP_UnmarshalErrors(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, nil)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "youtube_upload_video",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "youtube_update_metadata",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "youtube_get_metrics",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}

func TestRun_ConfigParsing(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	srv, err := setupServer(tempDir, nil, nil)
	if err != nil {
		t.Fatalf("setupServer failed: %v", err)
	}
	if srv == nil {
		t.Error("expected setupServer to return a server")
	}

	// Write bad config to force run() error
	badConfigPath := filepath.Join(tempDir, "powerword.toml")
	_ = os.WriteFile(badConfigPath, []byte("bad config format"), 0600)
	err = run()
	if err == nil {
		t.Error("expected run to fail with invalid config file, got nil")
	}
}

func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "contains client_secret query param",
			input:    "failed: client_secret=foo123&other=bar",
			expected: "failed: client_secret=[REDACTED]&other=bar",
		},
		{
			name:     "contains refresh_token in string",
			input:    `{"error": "invalid_grant", "refresh_token": "abcde123"}`,
			expected: `{"error": "invalid_grant", "refresh_token": "[REDACTED]"}`,
		},
		{
			name:     "contains client_id in error msg",
			input:    "google api error: client_id=xyz-123.apps.googleusercontent.com, unauthorized",
			expected: "google api error: client_id=[REDACTED], unauthorized",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSecrets(tc.input)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
