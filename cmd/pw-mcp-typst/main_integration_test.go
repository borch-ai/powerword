//go:build integration

package main_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
)

var pluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-typst-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-typst")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-typst: %v\n", err)
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
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode png: %v", err)
	}
	return buf.Bytes()
}

func TestMCP_TypstPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Verify if host has typst installed, skip if not
	if _, lookErr := exec.LookPath("typst"); lookErr != nil {
		t.Skip("skipping typst subprocess integration test: typst binary not found on PATH")
	}

	// Create a temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-typst-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	manuscriptPath := filepath.Join(workspaceDir, "manuscript.md")
	manuscriptContent := `
# Page 1
## Text
Stanza 1 with some text to compile.

## Prompt
Prompt 1
`
	if err := os.WriteFile(manuscriptPath, []byte(manuscriptContent), 0600); err != nil {
		t.Fatal(err)
	}

	imagesDir := filepath.Join(workspaceDir, "images")
	if err := os.Mkdir(imagesDir, 0750); err != nil {
		t.Fatal(err)
	}

	realPNG := createTinyPNG(t)
	if err := os.WriteFile(filepath.Join(imagesDir, "page_1.png"), realPNG, 0600); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(workspaceDir, "interior.pdf")
	coverPath := filepath.Join(workspaceDir, "cover.pdf")

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-typst", srvCfg)
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

	foundCompileInterior := false
	foundCompileCover := false
	for _, tool := range tools {
		if tool.Name == "compile_interior" {
			foundCompileInterior = true
		}
		if tool.Name == "compile_cover" {
			foundCompileCover = true
		}
	}
	if !foundCompileInterior {
		t.Errorf("expected to find 'compile_interior' tool, got tools: %+v", tools)
	}
	if !foundCompileCover {
		t.Errorf("expected to find 'compile_cover' tool, got tools: %+v", tools)
	}

	// 2. CallTool to compile interior
	argsInterior := map[string]interface{}{
		"manuscript_path": manuscriptPath,
		"images_dir":      imagesDir,
		"output_path":     outputPath,
		"page_size":       "8.5in,8.5in",
		"margin_inside":   "0.5in",
		"margin_outside":  "0.5in",
		"bleed":           "0.125in",
	}
	result, err := client.CallTool(ctx, "compile_interior", argsInterior)
	if err != nil {
		t.Fatalf("failed to call compile_interior tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution compile_interior returned error: %v", result)
	}

	// Check output interior PDF exists and has content
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("compiled interior PDF does not exist: %v", err)
	}
	if info.Size() < 100 {
		t.Errorf("compiled interior PDF size is too small: %d bytes", info.Size())
	}

	// 3. CallTool to compile cover
	argsCover := map[string]interface{}{
		"front_image_path": filepath.Join(imagesDir, "page_1.png"),
		"title":            "Mock Cover Book",
		"subtitle":         "For testing purposes",
		"author":           "Test Suite",
		"page_count":       24,
		"output_path":      coverPath,
	}
	resultCover, err := client.CallTool(ctx, "compile_cover", argsCover)
	if err != nil {
		t.Fatalf("failed to call compile_cover tool: %v", err)
	}

	if resultCover.IsError {
		t.Fatalf("tool execution compile_cover returned error: %v", resultCover)
	}

	// Check output cover PDF exists and has content
	infoCover, err := os.Stat(coverPath)
	if err != nil {
		t.Fatalf("compiled cover PDF does not exist: %v", err)
	}
	if infoCover.Size() < 100 {
		t.Errorf("compiled cover PDF size is too small: %d bytes", infoCover.Size())
	}
}

func TestMCP_TypstPlugin_LayoutOverrides(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Verify if host has typst installed, skip if not
	if _, lookErr := exec.LookPath("typst"); lookErr != nil {
		t.Skip("skipping typst subprocess integration test: typst binary not found on PATH")
	}

	// Create a temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-typst-workspace-overrides-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	manuscriptPath := filepath.Join(workspaceDir, "manuscript.md")
	manuscriptContent := `
# Page 1
<!-- Layout: facing-pages -->
## Text
Stanza 1 with some text.

## Prompt
Prompt 1

# Page 2
<!-- Layout: full-bleed -->
## Text
Stanza 2 with some text.

## Prompt
Prompt 2
`
	if err := os.WriteFile(manuscriptPath, []byte(manuscriptContent), 0600); err != nil {
		t.Fatal(err)
	}

	imagesDir := filepath.Join(workspaceDir, "images")
	if err := os.Mkdir(imagesDir, 0750); err != nil {
		t.Fatal(err)
	}

	realPNG := createTinyPNG(t)
	if err := os.WriteFile(filepath.Join(imagesDir, "page_1.png"), realPNG, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imagesDir, "page_2.png"), realPNG, 0600); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(workspaceDir, "interior.pdf")

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-typst", srvCfg)
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

	// Handshake
	_, err = client.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	// CallTool to compile interior with per-page layout overrides
	argsInterior := map[string]interface{}{
		"manuscript_path": manuscriptPath,
		"images_dir":      imagesDir,
		"output_path":     outputPath,
		"page_size":       "8.5in,8.5in",
		"margin_inside":   "0.5in",
		"margin_outside":  "0.5in",
		"bleed":           "0.125in",
		"layout":          "full-bleed", // Default global layout
	}
	result, err := client.CallTool(ctx, "compile_interior", argsInterior)
	if err != nil {
		t.Fatalf("failed to call compile_interior tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution compile_interior returned error: %v", result)
	}

	// Check output interior PDF exists and has content
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("compiled interior PDF does not exist: %v", err)
	}
	if info.Size() < 100 {
		t.Errorf("compiled interior PDF size is too small: %d bytes", info.Size())
	}
}
