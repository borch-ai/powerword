package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/config"
	"github.com/borch-ai/powerword/internal/plugins/viral"
)

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

func startTestServer(t *testing.T, workspaceRoot string, svc *viral.ViralService) (*mcp.ClientSession, context.Context, func()) {
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

func createMockFFmpegScript(t *testing.T) string {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "mock_ffmpeg")
	scriptContent := `#!/bin/sh
for arg; do true; done
touch "$arg"
exit 0
`
	//nolint:gosec // mock script needs to be executable
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("failed to write mock ffmpeg: %v", err)
	}
	return scriptPath
}

func TestViral_MCP_GenerateVoiceover(t *testing.T) {
	tempDir := t.TempDir()
	mockFFmpegPath := createMockFFmpegScript(t)

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				TTSProvider: "mock",
				FFmpegPath:  mockFFmpegPath,
			},
		},
	}
	svc := viral.NewViralService(tempDir, cfg)
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// 1. Success voiceover
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "viral_generate_voiceover",
		Arguments: json.RawMessage(`{
			"script": "hello voiceover",
			"provider": "mock"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool viral_generate_voiceover failed: %v", err)
	}
	assertResponse(t, res, false, "Successfully generated voiceover")

	// 2. Missing script error
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "viral_generate_voiceover",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, errRes, true, "script parameter is required")

	// 3. Service generation error
	cfgErr := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				TTSProvider: "invalid-provider-trigger-error",
			},
		},
	}
	svcErr := viral.NewViralService(tempDir, cfgErr)
	sessionErr, ctxErr, cleanupErr := startTestServer(t, tempDir, svcErr)
	defer cleanupErr()

	resErr, err := sessionErr.CallTool(ctxErr, &mcp.CallToolParams{
		Name: "viral_generate_voiceover",
		Arguments: json.RawMessage(`{
			"script": "hello"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, resErr, true, "failed to generate voiceover")
}

func TestViral_MCP_GenerateVideo(t *testing.T) {
	tempDir := t.TempDir()
	mockFFmpegPath := createMockFFmpegScript(t)

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "mock",
				FFmpegPath:   mockFFmpegPath,
			},
		},
	}
	svc := viral.NewViralService(tempDir, cfg)
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// 1. Success video
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "viral_generate_video",
		Arguments: json.RawMessage(`{
			"prompt": "hello video clip"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, false, "Successfully generated background video")

	// 2. Missing prompt error
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "viral_generate_video",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, errRes, true, "prompt parameter is required")

	// 3. Backend error
	cfgErr := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "invalid-backend-trigger-error",
			},
		},
	}
	svcErr := viral.NewViralService(tempDir, cfgErr)
	sessionErr, ctxErr, cleanupErr := startTestServer(t, tempDir, svcErr)
	defer cleanupErr()

	resErr, err := sessionErr.CallTool(ctxErr, &mcp.CallToolParams{
		Name: "viral_generate_video",
		Arguments: json.RawMessage(`{
			"prompt": "hello"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, resErr, true, "failed to generate video")
}

func TestViral_MCP_StitchTrailer(t *testing.T) {
	tempDir := t.TempDir()
	mockFFmpegPath := createMockFFmpegScript(t)

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: mockFFmpegPath,
			},
		},
	}
	svc := viral.NewViralService(tempDir, cfg)
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	videoPath := filepath.Join(tempDir, "v.mp4")
	audioPath := filepath.Join(tempDir, "a.mp3")
	_ = os.WriteFile(videoPath, []byte("video"), 0600)
	_ = os.WriteFile(audioPath, []byte("audio"), 0600)

	// 1. Success stitch
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "viral_stitch_trailer",
		Arguments: json.RawMessage(fmt.Sprintf(`{
			"video_path": %q,
			"audio_path": %q
		}`, videoPath, audioPath)),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, false, "Successfully stitched trailer")

	// 2. Missing parameters error
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "viral_stitch_trailer",
		Arguments: json.RawMessage(fmt.Sprintf(`{
			"video_path": %q
		}`, videoPath)),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, errRes, true, "video_path and audio_path parameters are required")

	// 3. Ffmpeg run error
	cfgErr := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: "non-existent-ffmpeg",
			},
		},
	}
	svcErr := viral.NewViralService(tempDir, cfgErr)
	sessionErr, ctxErr, cleanupErr := startTestServer(t, tempDir, svcErr)
	defer cleanupErr()

	resErr, err := sessionErr.CallTool(ctxErr, &mcp.CallToolParams{
		Name: "viral_stitch_trailer",
		Arguments: json.RawMessage(fmt.Sprintf(`{
			"video_path": %q,
			"audio_path": %q
		}`, videoPath, audioPath)),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, resErr, true, "failed to stitch trailer")
}

func TestViral_MCP_UnmarshalErrors(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, nil)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "viral_generate_voiceover",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "viral_generate_video",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "viral_stitch_trailer",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}
