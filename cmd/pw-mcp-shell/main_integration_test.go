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
	tmpDir, err := os.MkdirTemp("", "pw-mcp-shell-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-shell")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-shell: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_ShellPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-shell-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-shell", srvCfg)
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

	foundRunCommand := false
	for _, tool := range tools {
		if tool.Name == "run_command" {
			foundRunCommand = true
		}
	}
	if !foundRunCommand {
		t.Errorf("expected to find 'run_command' tool, got tools: %+v", tools)
	}

	// 2. CallTool to run a safe command
	argsEcho := map[string]interface{}{
		"command": "echo",
		"args":    []string{"integration test success"},
	}
	result, err := client.CallTool(ctx, "run_command", argsEcho)
	if err != nil {
		t.Fatalf("failed to call run_command tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if !strings.Contains(resStr, "integration test success") {
		t.Errorf("expected output to contain 'integration test success', got: %q", resStr)
	}

	// 3. CallTool to run a denied command
	argsRm := map[string]interface{}{
		"command": "rm",
		"args":    []string{"-rf", "/"},
	}
	resultRm, err := client.CallTool(ctx, "run_command", argsRm)
	if err != nil {
		t.Fatalf("failed to call run_command tool: %v", err)
	}
	if !resultRm.IsError {
		t.Errorf("expected command 'rm' to fail with error, but got success: %v", resultRm)
	}

	resStrRm, err := mcp.FormatToolResult(resultRm)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if !strings.Contains(resStrRm, "blocked by security policy") {
		t.Errorf("expected block message, got: %q", resStrRm)
	}
}
