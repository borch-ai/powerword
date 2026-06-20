//go:build integration

package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
)

var linterPluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-linter-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	linterPluginPath = filepath.Join(tmpDir, "pw-mcp-linter")
	cmd := exec.Command("go", "build", "-o", linterPluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-linter: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_LinterPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-linter-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	plansDir := filepath.Join(workspaceDir, "plans")
	if err := os.MkdirAll(plansDir, 0700); err != nil {
		t.Fatal(err)
	}

	// 1. Create a valid plan
	validPlan := `# plan: Task Valid Integration Test Plan

**Status:** Open
**Go Version:** —
**Date Completed:** —
**Unit Test Coverage:** —

## User Review Required

## Proposed Changes
No changes.

## Verification Plan
`
	if err := os.WriteFile(filepath.Join(plansDir, "task_valid.md"), []byte(validPlan), 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: linterPluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-linter", srvCfg)
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

	foundLint := false
	for _, tool := range tools {
		if tool.Name == "lint_plans" {
			foundLint = true
		}
	}
	if !foundLint {
		t.Errorf("expected to find 'lint_plans' tool, got tools: %+v", tools)
	}

	// 2. CallTool to lint plans (should be valid)
	args := map[string]interface{}{
		"workspace_root": workspaceDir,
	}
	result, err := client.CallTool(ctx, "lint_plans", args)
	if err != nil {
		t.Fatalf("failed to call lint_plans tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}

	var resData struct {
		Valid  bool          `json:"valid"`
		Errors []interface{} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(resStr), &resData); err != nil {
		t.Fatalf("failed to unmarshal linter result %q: %v", resStr, err)
	}

	if !resData.Valid {
		t.Errorf("expected valid = true, got data: %s", resStr)
	}

	// 3. Create an invalid plan (missing required sections)
	invalidPlan := `# plan: Task Invalid Test Plan
Missing Proposed Changes and Verification Plan headers completely.
`
	if err := os.WriteFile(filepath.Join(plansDir, "task_invalid.md"), []byte(invalidPlan), 0600); err != nil {
		t.Fatal(err)
	}

	// Call again, should be invalid
	result2, err := client.CallTool(ctx, "lint_plans", args)
	if err != nil {
		t.Fatalf("failed to call lint_plans second time: %v", err)
	}

	if result2.IsError {
		t.Fatalf("tool execution returned error: %v", result2)
	}

	resStr2, err := mcp.FormatToolResult(result2)
	if err != nil {
		t.Fatalf("failed to format second tool result: %v", err)
	}

	if err := json.Unmarshal([]byte(resStr2), &resData); err != nil {
		t.Fatalf("failed to unmarshal second result %q: %v", resStr2, err)
	}

	if resData.Valid {
		t.Errorf("expected valid = false, got data: %s", resStr2)
	}
	if len(resData.Errors) == 0 {
		t.Errorf("expected validation errors, got none: %s", resStr2)
	}
}
