//go:build integration

package main_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
	foundGetCapabilities := false
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
		if tool.Name == "imagegen_get_capabilities" {
			foundGetCapabilities = true
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
	if !foundGetCapabilities {
		t.Errorf("expected to find 'imagegen_get_capabilities' tool, got tools: %+v", tools)
	}

	// 1.5 CallTool to get capabilities
	resultCaps, err := client.CallTool(ctx, "imagegen_get_capabilities", map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to call imagegen_get_capabilities tool: %v", err)
	}
	if resultCaps.IsError {
		t.Fatalf("tool execution imagegen_get_capabilities returned error: %v", resultCaps)
	}
	capsStr, err := mcp.FormatToolResult(resultCaps)
	if err != nil {
		t.Fatalf("failed to format capabilities result: %v", err)
	}
	if !strings.Contains(capsStr, `"backend": "openai"`) {
		t.Errorf("expected capabilities to contain backend 'openai', got: %q", capsStr)
	}
	if !strings.Contains(capsStr, `"supports_cref": false`) {
		t.Errorf("expected capabilities to contain supports_cref false, got: %q", capsStr)
	}
	if !strings.Contains(capsStr, `"output_type": "image"`) {
		t.Errorf("expected capabilities to contain output_type 'image', got: %q", capsStr)
	}

	// 1.6 CallTool to generate with unsupported cref_url on openai backend
	argsGenBad := map[string]interface{}{
		"prompt":   "An worried egg sitting on a shelf",
		"size":     "1024x1024",
		"cref_url": "http://example.com/cref.png",
	}
	resultGenBad, err := client.CallTool(ctx, "imagegen_generate", argsGenBad)
	if err != nil {
		t.Fatalf("failed to call imagegen_generate tool: %v", err)
	}
	if !resultGenBad.IsError {
		t.Fatalf("expected generation to fail with unsupported cref_url, but it succeeded")
	}
	genBadStr, err := mcp.FormatToolResult(resultGenBad)
	if err != nil {
		t.Fatalf("failed to format generate result: %v", err)
	}
	if !strings.Contains(genBadStr, "character reference (cref_url) is not supported by the active imagegen backend") {
		t.Errorf("expected validation error in output, got: %q", genBadStr)
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

func TestMCP_ImageGenPlugin_MidjourneyCrefAndCw(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pngBytes := createTinyPNG(t)

	var receivedPrompt string

	// Mock server for Midjourney backend
	var mockServer *httptest.Server
	mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/midjourney") {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" {
				var req struct {
					Prompt string `json:"prompt"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
					receivedPrompt = req.Prompt
				}
				// Return a status URL pointing to GET status endpoint
				resp := fmt.Sprintf(`{"id": "mj-task-123", "status": "pending", "status_url": "%s/api/midjourney/status/mj-task-123"}`, mockServer.URL)
				_, _ = w.Write([]byte(resp))
				return
			}
			if r.Method == "GET" && strings.Contains(r.URL.Path, "/status/") {
				resp := fmt.Sprintf(`{"status": "completed", "image_url": "%s/image.png"}`, mockServer.URL)
				_, _ = w.Write([]byte(resp))
				return
			}
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

	// Write mock powerword.toml setting Midjourney backend
	cfgTOML := fmt.Sprintf(`
[plugins.imagegen]
backend = "midjourney"
midjourney_api_url = "%s/api/midjourney"
midjourney_polling_interval = "1ms"
midjourney_polling_timeout = "1s"
`, mockServer.URL)
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env: []string{
			"POWERWORD_WORKSPACE_ROOT=" + workspaceDir,
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

	// 1. Register style profile
	argsRegister := map[string]interface{}{
		"style_id":    "retro_profile",
		"prompt_seed": "1980s retro style",
		"sref_url":    "http://example.com/sref.png",
	}
	resultReg, err := client.CallTool(ctx, "imagegen_register_style", argsRegister)
	if err != nil {
		t.Fatalf("failed to call imagegen_register_style tool: %v", err)
	}
	if resultReg.IsError {
		t.Fatalf("tool execution imagegen_register_style returned error: %v", resultReg)
	}

	// 2. CallTool to generate image with cref and cw parameters
	argsGen := map[string]interface{}{
		"prompt":           "A futuristic car",
		"size":             "1024x1024",
		"style_id":         "retro_profile",
		"cref_url":         "http://example.com/cref.png",
		"character_weight": 75,
	}
	resultGen, err := client.CallTool(ctx, "imagegen_generate", argsGen)
	if err != nil {
		t.Fatalf("failed to call imagegen_generate tool: %v", err)
	}
	if resultGen.IsError {
		resStrErr, _ := mcp.FormatToolResult(resultGen)
		t.Fatalf("tool execution imagegen_generate returned error: %s (raw result: %+v)", resStrErr, resultGen)
	}

	// Assert that prompt constructed includes sref, cref, and cw parameters
	expectedPrompt := "A futuristic car, in the style of 1980s retro style --sref http://example.com/sref.png --cref http://example.com/cref.png --cw 75"
	if receivedPrompt != expectedPrompt {
		t.Errorf("expected constructed prompt to be %q, got: %q", expectedPrompt, receivedPrompt)
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

func TestMCP_ImageGenPlugin_RequestTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Mock server that sleeps for 200ms before responding
	var mockServer *httptest.Server
	mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		if strings.Contains(r.URL.Path, "/v1/images/generations") {
			w.Header().Set("Content-Type", "application/json")
			resp := fmt.Sprintf(`{"data": [{"url": "%s/image.png"}]}`, mockServer.URL)
			_, _ = w.Write([]byte(resp))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-imagegen-timeout-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Write mock powerword.toml setting the backend, key, and a very short request timeout
	cfgTOML := `
[api_keys]
openai = "dummy-openai-key"
[plugins.imagegen]
backend = "openai"
request_timeout = "50ms"
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

	// CallTool to generate image - should fail due to HTTP timeout (50ms limit vs 200ms sleep)
	argsGen := map[string]interface{}{
		"prompt": "An worried egg sitting on a shelf",
		"size":   "1024x1024",
	}
	resultGen, err := client.CallTool(ctx, "imagegen_generate", argsGen)
	if err != nil {
		t.Fatalf("failed to call imagegen_generate tool: %v", err)
	}
	if !resultGen.IsError {
		t.Fatalf("expected tool execution to fail due to request timeout, but it succeeded")
	}

	resStr, _ := mcp.FormatToolResult(resultGen)
	if !strings.Contains(resStr, "timeout") && !strings.Contains(resStr, "deadline exceeded") && !strings.Contains(resStr, "exceeded") {
		t.Errorf("expected timeout/deadline error in tool response, got: %q", resStr)
	}
}

func TestMCP_ImageGenPlugin_GoogleBackend_CrefNotSupported(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workspaceDir, err := os.MkdirTemp("", "pw-imagegen-google-caps-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Configure google (Imagen) backend — no real API calls needed since validation fires first.
	cfgTOML := `
[api_keys]
gemini = "dummy-google-key"
[plugins.imagegen]
backend = "google"
google_model = "imagen-4.0-generate-001"
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-imagegen", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() { _ = sp.GracefulShutdown(1 * time.Second) }()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	// Verify capabilities: google backend owns SupportsCref=false.
	resultCaps, err := client.CallTool(ctx, "imagegen_get_capabilities", map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to call imagegen_get_capabilities: %v", err)
	}
	if resultCaps.IsError {
		t.Fatalf("imagegen_get_capabilities returned error: %v", resultCaps)
	}
	capsStr, err := mcp.FormatToolResult(resultCaps)
	if err != nil {
		t.Fatalf("failed to format capabilities result: %v", err)
	}
	if !strings.Contains(capsStr, `"backend": "google"`) {
		t.Errorf("expected backend 'google' in capabilities, got: %q", capsStr)
	}
	if !strings.Contains(capsStr, `"supports_cref": false`) {
		t.Errorf("expected supports_cref=false for google Imagen backend, got: %q", capsStr)
	}
	if !strings.Contains(capsStr, `"output_type": "image"`) {
		t.Errorf("expected capabilities to contain output_type 'image' for google backend, got: %q", capsStr)
	}

	// Verify that calling generate with cref_url returns a validation error.
	resultGen, err := client.CallTool(ctx, "imagegen_generate", map[string]interface{}{
		"prompt":   "A majestic mountain",
		"cref_url": "http://example.com/cref.png",
	})
	if err != nil {
		t.Fatalf("failed to call imagegen_generate: %v", err)
	}
	if !resultGen.IsError {
		t.Fatal("expected imagegen_generate to fail on google backend with cref_url, but it succeeded")
	}
	genStr, _ := mcp.FormatToolResult(resultGen)
	if !strings.Contains(genStr, "character reference (cref_url) is not supported by the active imagegen backend") {
		t.Errorf("expected capability validation error, got: %q", genStr)
	}
}

func TestMCP_ImageGenPlugin_ForceCref_BypassesValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pngBytes := createTinyPNG(t)
	b64Data := base64.StdEncoding.EncodeToString(pngBytes)

	// Mock server for Google Imagen endpoint — returns a valid base64 image response.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := fmt.Sprintf(
			`{"predictions": [{"bytesBase64Encoded": "%s", "mimeType": "image/png"}]}`,
			b64Data,
		)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	defer mockServer.Close()

	workspaceDir, err := os.MkdirTemp("", "pw-imagegen-force-cref-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Configure google (Imagen) backend with force_cref = true.
	cfgTOML := `
[api_keys]
gemini = "dummy-google-key"
[plugins.imagegen]
backend = "google"
google_model = "imagen-4.0-generate-001"
force_cref = true
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env: []string{
			"POWERWORD_WORKSPACE_ROOT=" + workspaceDir,
			// Route all Google Imagen API calls to the mock server.
			"GOOGLE_BASE_URL=" + mockServer.URL,
		},
	}

	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-imagegen", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() { _ = sp.GracefulShutdown(1 * time.Second) }()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	// With force_cref=true the capability check is bypassed: supports_cref should be true.
	resultCaps, err := client.CallTool(ctx, "imagegen_get_capabilities", map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to call imagegen_get_capabilities: %v", err)
	}
	capsStr, err := mcp.FormatToolResult(resultCaps)
	if err != nil {
		t.Fatalf("failed to format capabilities result: %v", err)
	}
	if !strings.Contains(capsStr, `"supports_cref": true`) {
		t.Errorf("expected supports_cref=true with force_cref override, got: %q", capsStr)
	}

	// Generate with cref_url should bypass capability validation and succeed via the mock server.
	resultGen, err := client.CallTool(ctx, "imagegen_generate", map[string]interface{}{
		"prompt":   "A majestic mountain",
		"cref_url": "http://example.com/cref.png",
	})
	if err != nil {
		t.Fatalf("failed to call imagegen_generate: %v", err)
	}
	genStr, _ := mcp.FormatToolResult(resultGen)
	if strings.Contains(genStr, "character reference (cref_url) is not supported by the active imagegen backend") {
		t.Errorf("expected force_cref to bypass capability validation, but got capability error: %q", genStr)
	}
	if resultGen.IsError {
		t.Errorf("expected imagegen_generate to succeed with ForceCref+mock server, got error: %q", genStr)
	}
}

func TestMCP_ImageGenPlugin_VeoBackend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workspaceDir, err := os.MkdirTemp("", "pw-imagegen-veo-caps-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Configure google veo backend
	cfgTOML := `
[api_keys]
gemini = "dummy-google-key"
[plugins.imagegen]
backend = "veo"
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-imagegen", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() { _ = sp.GracefulShutdown(1 * time.Second) }()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	// Verify capabilities: veo backend owns SupportsCref=true, OutputType=video.
	resultCaps, err := client.CallTool(ctx, "imagegen_get_capabilities", map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to call imagegen_get_capabilities: %v", err)
	}
	if resultCaps.IsError {
		t.Fatalf("imagegen_get_capabilities returned error: %v", resultCaps)
	}
	capsStr, err := mcp.FormatToolResult(resultCaps)
	if err != nil {
		t.Fatalf("failed to format capabilities result: %v", err)
	}
	if !strings.Contains(capsStr, `"backend": "veo"`) {
		t.Errorf("expected backend 'veo' in capabilities, got: %q", capsStr)
	}
	if !strings.Contains(capsStr, `"supports_cref": true`) {
		t.Errorf("expected supports_cref=true for veo backend, got: %q", capsStr)
	}
	if !strings.Contains(capsStr, `"output_type": "video"`) {
		t.Errorf("expected output_type='video' for veo backend, got: %q", capsStr)
	}
}

