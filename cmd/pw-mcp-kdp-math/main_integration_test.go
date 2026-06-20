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

var mathPluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-kdp-math-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	mathPluginPath = filepath.Join(tmpDir, "pw-mcp-kdp-math")
	cmd := exec.Command("go", "build", "-o", mathPluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-kdp-math: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_KDPMathPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-math-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: mathPluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-kdp-math", srvCfg)
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

	foundCalc := false
	foundGen := false
	for _, tool := range tools {
		if tool.Name == "kdp_calculate_geometry" {
			foundCalc = true
		}
		if tool.Name == "kdp_generate_manifest" {
			foundGen = true
		}
	}
	if !foundCalc || !foundGen {
		t.Errorf("expected to find math tools, got tools: %+v", tools)
	}

	// 2. CallTool: kdp_calculate_geometry
	args := map[string]interface{}{
		"page_count":   100,
		"binding_type": "paperback",
		"paper_type":   "white",
		"trim_size":    "6x9",
	}
	result, err := client.CallTool(ctx, "kdp_calculate_geometry", args)
	if err != nil {
		t.Fatalf("failed to call kdp_calculate_geometry tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}

	if !strings.Contains(resStr, "cover_width_inches") || !strings.Contains(resStr, "spine_width_inches") {
		t.Errorf("expected cover & spine dimensions, got: %q", resStr)
	}

	// 3. CallTool: kdp_generate_manifest
	args2 := map[string]interface{}{
		"page_count":   120,
		"binding_type": "paperback",
		"paper_type":   "white",
		"trim_size":    "6x9",
	}
	result2, err := client.CallTool(ctx, "kdp_generate_manifest", args2)
	if err != nil {
		t.Fatalf("failed to call kdp_generate_manifest tool: %v", err)
	}

	if result2.IsError {
		t.Fatalf("tool execution returned error: %v", result2)
	}

	resStr2, err := mcp.FormatToolResult(result2)
	if err != nil {
		t.Fatalf("failed to format second tool result: %v", err)
	}

	if !strings.Contains(resStr2, "trim_width_inches") {
		t.Errorf("expected trim_width_inches in manifest, got: %q", resStr2)
	}
}
