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
	tmpDir, err := os.MkdirTemp("", "pw-mcp-memory-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-memory")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-memory: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_MemoryPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use a mock config to skip network calls during integration tests if possible
	// However, we need embeddings. We might just rely on real credentials from env
	// or mock out HOME to use a local config. We'll set HOME to a temp directory
	// and let it either fail gracefully or succeed.
	homeDir, err := os.MkdirTemp("", "pw-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(homeDir)

	// Since we need an LLM client, we'll configure a mock endpoint by putting a config.toml in the home directory
	cfgPath := filepath.Join(homeDir, ".config", "powerword")
	if err := os.MkdirAll(cfgPath, 0755); err != nil {
		t.Fatal(err)
	}
	
	cfgContent := "[api_keys]\ngemini = \"dummy\"\n"
	if err := os.WriteFile(filepath.Join(cfgPath, "config.toml"), []byte(cfgContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"HOME=" + homeDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-memory", srvCfg)
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

	foundAdd := false
	foundSearch := false
	for _, tool := range tools {
		if tool.Name == "memory_add" {
			foundAdd = true
		}
		if tool.Name == "memory_search" {
			foundSearch = true
		}
	}
	if !foundAdd || !foundSearch {
		t.Errorf("expected to find memory_add and memory_search, got tools: %+v", tools)
	}

	// We won't test CallTool with memory_add here because it requires a real API key to generate embeddings,
	// unless we setup an HTTP server mock here.
	// Since we just want to verify stdio integration, Listing tools is sufficient.

	// Let's test the error path for memory_add empty text, which does not require API key.
	args := map[string]interface{}{
		"text": "",
	}
	result, err := client.CallTool(ctx, "memory_add", args)
	if err != nil {
		t.Fatalf("failed to call memory_add tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for empty text, got success")
	}
	resStr, _ := mcp.FormatToolResult(result)
	if !strings.Contains(resStr, "Text cannot be empty") {
		t.Errorf("expected 'Text cannot be empty', got: %s", resStr)
	}
}
