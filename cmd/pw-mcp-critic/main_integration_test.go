//go:build integration

package main_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	tmpDir, err := os.MkdirTemp("", "pw-mcp-critic-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-critic")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-critic: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_CriticPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Mock server for OpenAI API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Review complete. VERDICT: ACCEPT"
				}
			}]
		}`))
	}))
	defer mockServer.Close()

	// Create a temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-critic-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Write mock powerword.toml
	cfgTOML := fmt.Sprintf(`
critic_provider = "openai"
critic_model = "gpt-4"
critic_endpoint = "%s"

[api_keys]
openai = "dummy-key"
`, mockServer.URL)
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	// We need a dummy Git diff in this workspace, but pw-mcp-critic runs `git diff` via ExtractGitDiff helper.
	// Since workspaceDir is a temp folder that is not a git repo, ExtractGitDiff would fail.
	// Let's initialize a git repo here to make sure git diff succeeds.
	gitInitCmd := exec.Command("git", "init")
	gitInitCmd.Dir = workspaceDir
	if err := gitInitCmd.Run(); err != nil {
		t.Fatalf("failed to git init temp workspace: %v", err)
	}

	// Write a file and stage it so there is a diff
	dummyFile := filepath.Join(workspaceDir, "test.go")
	if err := os.WriteFile(dummyFile, []byte("package main\n\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	gitAddCmd := exec.Command("git", "add", "test.go")
	gitAddCmd.Dir = workspaceDir
	if err := gitAddCmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-critic", srvCfg)
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

	foundReview := false
	for _, tool := range tools {
		if tool.Name == "review_workspace" {
			foundReview = true
		}
	}
	if !foundReview {
		t.Errorf("expected to find 'review_workspace' tool, got tools: %+v", tools)
	}

	// 2. CallTool to review the workspace
	args := map[string]interface{}{
		"plan_content": "### Goal\nAdd tests\n### Proposed Changes\n- test.go\n### Verification Plan\nRun tests",
	}
	result, err := client.CallTool(ctx, "review_workspace", args)
	if err != nil {
		t.Fatalf("failed to call review_workspace tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}

	if !strings.Contains(resStr, "VERDICT: ACCEPT") {
		t.Errorf("expected tool result to contain 'VERDICT: ACCEPT', got: %q", resStr)
	}
}
