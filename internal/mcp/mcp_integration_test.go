//go:build integration

package mcp_test

import (
	"archive/zip"
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

var (
	fsPluginPath   string
	epubPluginPath string
)

func TestMain(m *testing.M) {
	// Dynamically build the plugins
	tmpDir, err := os.MkdirTemp("", "pw-mcp-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	fsPluginPath = filepath.Join(tmpDir, "pw-mcp-fs")
	cmd := exec.Command("go", "build", "-o", fsPluginPath, "../../cmd/pw-mcp-fs")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-fs: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	epubPluginPath = filepath.Join(tmpDir, "pw-mcp-epub")
	cmdEpub := exec.Command("go", "build", "-o", epubPluginPath, "../../cmd/pw-mcp-epub")
	if err := cmdEpub.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-epub: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_FSPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Write a test file in the workspace
	testFilePath := filepath.Join(workspaceDir, "test.txt")
	testContent := "hello from mcp integration test!"
	if err := os.WriteFile(testFilePath, []byte(testContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: fsPluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-fs", srvCfg)
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

	// Verify we got the tools we expect (read_file, write_file, etc.)
	foundReadFile := false
	for _, tool := range tools {
		if tool.Name == "read_file" {
			foundReadFile = true
		}
	}
	if !foundReadFile {
		t.Errorf("expected to find 'read_file' tool, got tools: %+v", tools)
	}

	// 2. CallTool to read the test file
	args := map[string]interface{}{
		"path": "test.txt",
	}
	result, err := client.CallTool(ctx, "read_file", args)
	if err != nil {
		t.Fatalf("failed to call read_file tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	// Translate and format tool result
	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}

	if !strings.Contains(resStr, testContent) {
		t.Errorf("expected tool result to contain test content %q, got: %q", testContent, resStr)
	}
}

func TestMCP_EpubPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-epub-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	manuscriptPath := filepath.Join(workspaceDir, "manuscript.md")
	if err := os.WriteFile(manuscriptPath, []byte("# Chapter 1\nHello EPUB integration!"), 0600); err != nil {
		t.Fatal(err)
	}

	imagesDir := filepath.Join(workspaceDir, "images")
	if err := os.Mkdir(imagesDir, 0750); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(workspaceDir, "book.epub")

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: epubPluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-epub", srvCfg)
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

	foundCompileEpub := false
	for _, tool := range tools {
		if tool.Name == "compile_epub" {
			foundCompileEpub = true
		}
	}
	if !foundCompileEpub {
		t.Errorf("expected to find 'compile_epub' tool, got tools: %+v", tools)
	}

	// 2. CallTool to compile the EPUB
	args := map[string]interface{}{
		"manuscript_path": manuscriptPath,
		"images_dir":      imagesDir,
		"output_path":     outputPath,
		"title":           "Integration Book",
		"author":          "Integration Agent",
	}
	result, err := client.CallTool(ctx, "compile_epub", args)
	if err != nil {
		t.Fatalf("failed to call compile_epub tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	// Check if file exists and is valid EPUB
	rc, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("failed to open compiled EPUB zip: %v", err)
	}
	defer rc.Close()

	if len(rc.File) == 0 {
		t.Fatal("compiled EPUB zip is empty")
	}

	firstFile := rc.File[0]
	if firstFile.Name != "mimetype" {
		t.Errorf("expected first file in EPUB to be mimetype, got %s", firstFile.Name)
	}
	if firstFile.Method != zip.Store {
		t.Errorf("expected mimetype to be stored with no compression, got method %d", firstFile.Method)
	}
}
