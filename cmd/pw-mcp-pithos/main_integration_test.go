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
	tmpDir, err := os.MkdirTemp("", "pw-mcp-pithos-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-pithos")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-pithos: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_PithosPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workspaceDir, err := os.MkdirTemp("", "pw-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Create a mock pithos script so the subprocess call succeeds
	mockPithosPath := filepath.Join(workspaceDir, "pithos")
	scriptContent := "#!/bin/sh\necho \"mock pithos output\"\n"
	if err := os.WriteFile(mockPithosPath, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env: []string{
			"POWERWORD_WORKSPACE_ROOT=" + workspaceDir,
			"PATH=" + workspaceDir + ":" + os.Getenv("PATH"),
		},
	}

	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-pithos", srvCfg)
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

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	foundTool := false
	for _, tool := range tools {
		if tool.Name == "pithos_initiate" {
			foundTool = true
		}
	}
	if !foundTool {
		t.Errorf("expected to find 'pithos_initiate' tool, got tools: %+v", tools)
	}

	args := map[string]interface{}{
		"project_path": workspaceDir,
	}
	result, err := client.CallTool(ctx, "pithos_initiate", args)
	if err != nil {
		t.Fatalf("failed to call pithos_initiate tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}

	if !strings.Contains(resStr, "mock pithos output") {
		t.Errorf("expected tool result to contain mock output, got: %q", resStr)
	}
}
