//go:build integration

package main_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
)

var pluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-youtube-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-youtube")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-youtube: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_YouTubePlugin_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Mock server for YouTube API endpoints
	var mockServer *httptest.Server
	mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 0. OAuth2 Token refresh
		if strings.Contains(r.URL.Path, "/oauth2/token") {
			_, _ = w.Write([]byte(`{"access_token": "mock-access-token", "token_type": "Bearer", "expires_in": 3600}`))
			return
		}

		// 1. Upload video (POST/PUT)
		if strings.Contains(r.URL.Path, "/upload/youtube/v3/videos") {
			// Google Client SDK multipart upload request body contains the video data directly.
			// Let's return the final video resource JSON.
			resp := `{"id": "integration-video-999"}`
			_, _ = w.Write([]byte(resp))
			return
		}

		// 2. Videos endpoint (List / Update)
		if strings.Contains(r.URL.Path, "/youtube/v3/videos") {
			if r.Method == http.MethodGet {
				// videos.list returning details or statistics
				var resp string
				if strings.Contains(r.URL.RawQuery, "part=statistics") {
					resp = `{
						"items": [{
							"id": "integration-video-999",
							"statistics": {
								"viewCount": "7500",
								"likeCount": "300",
								"commentCount": "40",
								"favoriteCount": "10"
							}
						}]
					}`
				} else {
					resp = `{
						"items": [{
							"id": "integration-video-999",
							"snippet": {
								"title": "Old Title",
								"description": "Old Description",
								"tags": ["old"]
							}
						}]
					}`
				}
				_, _ = w.Write([]byte(resp))
				return
			}

			if r.Method == http.MethodPut {
				// videos.update
				_, _ = w.Write([]byte(`{"id": "integration-video-999"}`))
				return
			}
		}

		// 3. PlaylistItems endpoint (Insert)
		if strings.Contains(r.URL.Path, "/youtube/v3/playlistItems") {
			_, _ = w.Write([]byte(`{"id": "playlist-item-abc"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-youtube-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Create a dummy video file
	dummyVideo := filepath.Join(workspaceDir, "test.mp4")
	if err := os.WriteFile(dummyVideo, []byte("fake video bytes"), 0600); err != nil {
		t.Fatal(err)
	}

	// Write mock powerword.toml
	cfgTOML := `
[plugins.youtube]
client_id = "client-id-123"
client_secret = "client-secret-123"
refresh_token = "refresh-token-123"
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config with our endpoint override env var
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env: []string{
			"POWERWORD_WORKSPACE_ROOT=" + workspaceDir,
			"POWERWORD_YOUTUBE_API_ENDPOINT=" + mockServer.URL,
		},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-youtube", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() {
		_ = sp.GracefulShutdown(1 * time.Second)
	}()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	// 1. Verify Handshake lists all 3 tools
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	foundUpload := false
	foundUpdate := false
	foundMetrics := false
	for _, tool := range tools {
		if tool.Name == "youtube_upload_video" {
			foundUpload = true
		}
		if tool.Name == "youtube_update_metadata" {
			foundUpdate = true
		}
		if tool.Name == "youtube_get_metrics" {
			foundMetrics = true
		}
	}
	if !foundUpload || !foundUpdate || !foundMetrics {
		t.Errorf("missing tools in registration. got: %+v", tools)
	}

	// 2. Call tool: youtube_upload_video
	resUpload, err := client.CallTool(ctx, "youtube_upload_video", map[string]interface{}{
		"video_path":  dummyVideo,
		"title":       "Integration Title",
		"description": "Integration Desc",
		"privacy":     "public",
	})
	if err != nil {
		t.Fatalf("failed to call youtube_upload_video: %v", err)
	}
	if resUpload.IsError {
		resErrStr, _ := mcp.FormatToolResult(resUpload)
		t.Fatalf("youtube_upload_video failed: %s", resErrStr)
	}
	uploadStr, _ := mcp.FormatToolResult(resUpload)
	if !strings.Contains(uploadStr, "integration-video-999") {
		t.Errorf("expected upload response to contain video ID, got: %q", uploadStr)
	}

	// 3. Call tool: youtube_update_metadata
	resUpdate, err := client.CallTool(ctx, "youtube_update_metadata", map[string]interface{}{
		"video_id":    "integration-video-999",
		"title":       "Updated Integration Title",
		"playlist_id": "playlist-1234",
	})
	if err != nil {
		t.Fatalf("failed to call youtube_update_metadata: %v", err)
	}
	if resUpdate.IsError {
		resErrStr, _ := mcp.FormatToolResult(resUpdate)
		t.Fatalf("youtube_update_metadata failed: %s", resErrStr)
	}
	updateStr, _ := mcp.FormatToolResult(resUpdate)
	if !strings.Contains(updateStr, "updated successfully") {
		t.Errorf("expected success message, got: %q", updateStr)
	}

	// 4. Call tool: youtube_get_metrics
	resMetrics, err := client.CallTool(ctx, "youtube_get_metrics", map[string]interface{}{
		"video_id": "integration-video-999",
		"metrics":  []string{"views", "likes"},
	})
	if err != nil {
		t.Fatalf("failed to call youtube_get_metrics: %v", err)
	}
	if resMetrics.IsError {
		resErrStr, _ := mcp.FormatToolResult(resMetrics)
		t.Fatalf("youtube_get_metrics failed: %s", resErrStr)
	}
	metricsStr, _ := mcp.FormatToolResult(resMetrics)
	if !strings.Contains(metricsStr, `"views": 7500`) || !strings.Contains(metricsStr, `"likes": 300`) {
		t.Errorf("expected statistics in metrics, got: %q", metricsStr)
	}
}
