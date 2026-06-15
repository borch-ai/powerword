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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
)

var pluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-git-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-git")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-git: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func setupTestRepo(t *testing.T, dir string) {
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello integration world"), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if _, err := wt.Add("test.txt"); err != nil {
		t.Fatalf("failed to add to worktree: %v", err)
	}

	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Integration Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	if err := os.WriteFile(testFile, []byte("this is completely new content"), 0600); err != nil {
		t.Fatalf("failed to modify test file: %v", err)
	}
}

func TestMCP_GitPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-git-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	setupTestRepo(t, workspaceDir)

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-git", srvCfg)
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

	foundGitStatus := false
	foundGitDiff := false
	foundGitLog := false
	for _, tool := range tools {
		if tool.Name == "git_status" {
			foundGitStatus = true
		}
		if tool.Name == "git_diff" {
			foundGitDiff = true
		}
		if tool.Name == "git_log" {
			foundGitLog = true
		}
	}
	if !foundGitStatus {
		t.Errorf("expected to find 'git_status' tool, got tools: %+v", tools)
	}
	if !foundGitDiff {
		t.Errorf("expected to find 'git_diff' tool, got tools: %+v", tools)
	}
	if !foundGitLog {
		t.Errorf("expected to find 'git_log' tool, got tools: %+v", tools)
	}

	// 2. CallTool git_status
	resultStatus, err := client.CallTool(ctx, "git_status", map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to call git_status tool: %v", err)
	}
	if resultStatus.IsError {
		t.Fatalf("tool execution git_status returned error: %v", resultStatus)
	}

	resStrStatus, err := mcp.FormatToolResult(resultStatus)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if !strings.Contains(resStrStatus, "test.txt") {
		t.Errorf("expected status to mention test.txt, got: %q", resStrStatus)
	}

	// 3. CallTool git_diff
	resultDiff, err := client.CallTool(ctx, "git_diff", map[string]interface{}{})
	if err != nil {
		t.Fatalf("failed to call git_diff tool: %v", err)
	}
	if resultDiff.IsError {
		t.Fatalf("tool execution git_diff returned error: %v", resultDiff)
	}

	resStrDiff, err := mcp.FormatToolResult(resultDiff)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if resStrDiff == "" || strings.TrimSpace(resStrDiff) == "No changes" {
		t.Errorf("expected diff to show changes, but got: %q", resStrDiff)
	}
	if !strings.Contains(resStrDiff, "+") || !strings.Contains(resStrDiff, "-") {
		t.Errorf("expected diff to show additions/deletions, but got: %q", resStrDiff)
	}
}
