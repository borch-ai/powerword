//go:build integration

package main_test

import (
	"context"
	"fmt"
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
	tmpDir, err := os.MkdirTemp("", "pw-mcp-viral-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-viral")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-viral: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func createMockFFmpegScript(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "mock_ffmpeg")
	scriptContent := `#!/bin/sh
for arg; do true; done
touch "$arg"
exit 0
`
	//nolint:gosec
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("failed to write mock ffmpeg: %v", err)
	}
	return scriptPath
}

func TestMCP_ViralPlugin_StitchSlideshow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockFFmpeg := createMockFFmpegScript(t)

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-viral-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Write mock powerword.toml setting ffmpeg path
	cfgTOML := fmt.Sprintf(`
[plugins.viral]
ffmpeg_path = %q
`, mockFFmpeg)
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	// Create mock image and audio files
	imagePath := filepath.Join(workspaceDir, "test_image.png")
	audioPath := filepath.Join(workspaceDir, "test_audio.mp3")
	bgAudioPath := filepath.Join(workspaceDir, "bg_audio.mp3")

	_ = os.WriteFile(imagePath, []byte("image"), 0600)
	_ = os.WriteFile(audioPath, []byte("audio"), 0600)
	_ = os.WriteFile(bgAudioPath, []byte("bg-audio"), 0600)

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env: []string{
			"POWERWORD_WORKSPACE_ROOT=" + workspaceDir,
		},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-viral", srvCfg)
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

	// 1. Handshake / ListTools
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	foundStitchSlideshow := false
	for _, tool := range tools {
		if tool.Name == "viral_stitch_slideshow" {
			foundStitchSlideshow = true
		}
	}
	if !foundStitchSlideshow {
		t.Errorf("expected to find 'viral_stitch_slideshow' tool, got tools: %+v", tools)
	}

	// 2. CallTool to stitch slideshow (with background music)
	args := map[string]interface{}{
		"slides": []map[string]string{
			{
				"image_path": imagePath,
				"audio_path": audioPath,
			},
		},
		"background_audio_path": bgAudioPath,
		"output_name":           "out_integration.mp4",
	}

	result, err := client.CallTool(ctx, "viral_stitch_slideshow", args)
	if err != nil {
		t.Fatalf("failed to call viral_stitch_slideshow: %v", err)
	}
	if result.IsError {
		resStrErr, _ := mcp.FormatToolResult(result)
		t.Fatalf("tool execution viral_stitch_slideshow returned error: %s", resStrErr)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if !strings.Contains(resStr, "Successfully stitched slideshow") {
		t.Errorf("expected output to indicate success, got: %q", resStr)
	}
}
