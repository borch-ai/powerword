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
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var pluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-coverage-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-coverage")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-coverage: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_CoveragePlugin_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temp directory for profile files
	tmpDir, err := os.MkdirTemp("", "pw-cov-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write a mock LCOV profile
	lcovPath := filepath.Join(tmpDir, "lcov.info")
	lcovContent := "SF:index.ts\nLF:100\nLH:95\nend_of_record\n"
	if err := os.WriteFile(lcovPath, []byte(lcovContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config to spawn our compiled coverage plugin
	srvCfg := config.ServerConfig{
		Command: pluginPath,
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-coverage", srvCfg)
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

	// 1. Verify Tool Registration
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	foundCheckCoverage := false
	for _, tool := range tools {
		if tool.Name == "check_coverage" {
			foundCheckCoverage = true
			break
		}
	}
	if !foundCheckCoverage {
		t.Fatalf("expected to find 'check_coverage' tool, got tools: %+v", tools)
	}

	// 2. Call check_coverage tool for success (95.0% >= 90.0%)
	res, err := client.CallTool(ctx, "check_coverage", map[string]interface{}{
		"threshold":    90.0,
		"profile_path": lcovPath,
		"format":       "auto",
	})
	if err != nil {
		t.Fatalf("failed to call check_coverage: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %v", res)
	}
	text := res.Content[0].(*mcpsdk.TextContent).Text
	if !strings.Contains(text, "PASS") {
		t.Errorf("expected output to contain 'PASS', got: %s", text)
	}

	// 3. Call check_coverage tool for failure (95.0% < 98.0%)
	resFail, err := client.CallTool(ctx, "check_coverage", map[string]interface{}{
		"threshold":    98.0,
		"profile_path": lcovPath,
		"format":       "auto",
	})
	if err != nil {
		t.Fatalf("failed to call check_coverage: %v", err)
	}
	if !resFail.IsError {
		t.Errorf("expected error tool response due to low coverage, got success")
	}
	textFail := resFail.Content[0].(*mcpsdk.TextContent).Text
	if !strings.Contains(textFail, "FAIL") {
		t.Errorf("expected output to contain 'FAIL', got: %s", textFail)
	}
}
