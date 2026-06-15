//go:build integration

package main_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
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
	tmpDir, err := os.MkdirTemp("", "pw-mcp-imagegen-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-imagegen")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-imagegen: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func createTinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{0, 255, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode png: %v", err)
	}
	return buf.Bytes()
}

func TestMCP_ImageGenPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pngBytes := createTinyPNG(t)

	// Mock server for OpenAI DALL-E and file downloads
	var mockServer *httptest.Server
	mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/v1/images/generations") {
			w.Header().Set("Content-Type", "application/json")
			// Return URL pointing back to our mock server for download
			resp := fmt.Sprintf(`{"data": [{"url": "%s/image.png"}]}`, mockServer.URL)
			_, _ = w.Write([]byte(resp))
			return
		}

		if strings.Contains(r.URL.Path, "/image.png") {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-imagegen-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Write mock powerword.toml setting the backend and dummy key
	cfgTOML := `
[api_keys]
openai = "dummy-openai-key"
[plugins.imagegen]
backend = "openai"
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env: []string{
			"POWERWORD_WORKSPACE_ROOT=" + workspaceDir,
			"OPENAI_BASE_URL=" + mockServer.URL + "/v1",
		},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-imagegen", srvCfg)
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

	foundGenerate := false
	foundRegisterStyle := false
	foundListStyles := false
	for _, tool := range tools {
		if tool.Name == "imagegen_generate" {
			foundGenerate = true
		}
		if tool.Name == "imagegen_register_style" {
			foundRegisterStyle = true
		}
		if tool.Name == "imagegen_list_styles" {
			foundListStyles = true
		}
	}
	if !foundGenerate {
		t.Errorf("expected to find 'imagegen_generate' tool, got tools: %+v", tools)
	}
	if !foundRegisterStyle {
		t.Errorf("expected to find 'imagegen_register_style' tool, got tools: %+v", tools)
	}
	if !foundListStyles {
		t.Errorf("expected to find 'imagegen_list_styles' tool, got tools: %+v", tools)
	}

	// 2. CallTool to register style profile
	argsRegister := map[string]interface{}{
		"style_id":    "test_profile",
		"prompt_seed": "oil painting style, high resolution",
	}
	resultReg, err := client.CallTool(ctx, "imagegen_register_style", argsRegister)
	if err != nil {
		t.Fatalf("failed to call imagegen_register_style tool: %v", err)
	}
	if resultReg.IsError {
		t.Fatalf("tool execution imagegen_register_style returned error: %v", resultReg)
	}

	// 3. CallTool to generate image
	argsGen := map[string]interface{}{
		"prompt":   "An worried egg sitting on a shelf",
		"size":     "1024x1024",
		"style_id": "test_profile",
	}
	resultGen, err := client.CallTool(ctx, "imagegen_generate", argsGen)
	if err != nil {
		t.Fatalf("failed to call imagegen_generate tool: %v", err)
	}
	if resultGen.IsError {
		resStrErr, _ := mcp.FormatToolResult(resultGen)
		t.Fatalf("tool execution imagegen_generate returned error: %s (raw result: %+v)", resStrErr, resultGen)
	}

	resStr, err := mcp.FormatToolResult(resultGen)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if !strings.Contains(resStr, "Successfully generated image") {
		t.Errorf("expected output to indicate success, got: %q", resStr)
	}

	// Check that generated image file exists and is not empty
	imgFiles, err := filepath.Glob(filepath.Join(workspaceDir, "generated_images", "*.png"))
	if err != nil || len(imgFiles) == 0 {
		t.Fatalf("no generated image files found: %v", err)
	}

	info, err := os.Stat(imgFiles[0])
	if err != nil {
		t.Fatalf("stat generated image file failed: %v", err)
	}
	if info.Size() < 50 {
		t.Errorf("generated image size too small: %d bytes", info.Size())
	}
}
