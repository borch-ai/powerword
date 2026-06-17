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
	tmpDir, err := os.MkdirTemp("", "pw-mcp-cloud-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-cloud")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-cloud: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_CloudPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-cloud-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Write mock powerword.toml containing the cloud configuration
	cfgTOML := `
[plugins.cloud]
provider = "noop"
bucket = "test-integration-bucket"
credentials_path = "/tmp/dummy-credentials.json"
region = "us-east-1"
`
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
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-cloud", srvCfg)
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

	foundListInstances := false
	foundGetLogs := false
	foundCheckBucket := false
	for _, tool := range tools {
		if tool.Name == "cloud_list_instances" {
			foundListInstances = true
		}
		if tool.Name == "cloud_get_logs" {
			foundGetLogs = true
		}
		if tool.Name == "cloud_check_bucket" {
			foundCheckBucket = true
		}
	}

	if !foundListInstances {
		t.Errorf("expected to find 'cloud_list_instances' tool, got tools: %+v", tools)
	}
	if !foundGetLogs {
		t.Errorf("expected to find 'cloud_get_logs' tool, got tools: %+v", tools)
	}
	if !foundCheckBucket {
		t.Errorf("expected to find 'cloud_check_bucket' tool, got tools: %+v", tools)
	}

	// 2. CallTool to check bucket (which calls noop/mock backend)
	args := map[string]interface{}{
		"provider":    "noop",
		"bucket_name": "test-integration-bucket",
	}
	result, err := client.CallTool(ctx, "cloud_check_bucket", args)
	if err != nil {
		t.Fatalf("failed to call cloud_check_bucket tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool execution cloud_check_bucket returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if !strings.Contains(resStr, "test-integration-bucket") || !strings.Contains(resStr, "noop") {
		t.Errorf("expected output to contain bucket name and provider, got: %q", resStr)
	}
}
