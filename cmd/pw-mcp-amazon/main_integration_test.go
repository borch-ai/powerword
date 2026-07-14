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

var amazonPluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-amazon-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	amazonPluginPath = filepath.Join(tmpDir, "pw-mcp-amazon")
	//nolint:gosec // G204: go build command is safe to run in tests with controlled arguments
	cmd := exec.Command("go", "build", "-o", amazonPluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-amazon: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_AmazonPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-amazon-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: amazonPluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-amazon", srvCfg)
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

	foundTool := false
	for _, tool := range tools {
		if tool.Name == "get_amazon_listing_count" {
			foundTool = true
			break
		}
	}
	if !foundTool {
		t.Errorf("expected to find get_amazon_listing_count tool, got tools: %+v", tools)
	}

	// 2. CallTool: get_amazon_listing_count
	args := map[string]interface{}{
		"keyword": "radon detector",
	}
	result, err := client.CallTool(ctx, "get_amazon_listing_count", args)
	if err != nil {
		t.Fatalf("failed to call get_amazon_listing_count tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}

	if !strings.Contains(resStr, "listing_count") || !strings.Contains(resStr, "4200") {
		t.Errorf("expected listing_count and fallback 4200, got: %q", resStr)
	}
}
