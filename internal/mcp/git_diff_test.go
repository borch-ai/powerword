package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestHelperProcess is used to mock the pw-mcp-git server process and git commands.
//
//nolint:gocognit
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	cmd := args[0]
	if cmd == "pw-mcp-git-mock" {
		srv := mcp.NewServer(&mcp.Implementation{Name: "pw-mcp-git", Version: "1.0.0"}, nil)
		srv.AddTool(&mcp.Tool{
			Name:        "git_diff_commits",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			val := os.Getenv("MOCK_COMMITS_DIFF")
			if val == "" {
				val = "No changes"
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: val}},
			}, nil
		})
		srv.AddTool(&mcp.Tool{
			Name:        "git_diff",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			val := os.Getenv("MOCK_WORKING_DIFF")
			if val == "" {
				val = "No changes"
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: val}},
			}, nil
		})
		transport := &mcp.StdioTransport{}
		if err := srv.Run(context.Background(), transport); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if cmd == "gh" {
		if os.Getenv("MOCK_GH_BASE") == "fail" {
			os.Exit(1)
		}
		_, _ = fmt.Fprint(os.Stdout, "main")
		os.Exit(0)
	}
	if cmd == "git" {
		// Mock logic: succeed if verifying origin/main, fail otherwise
		for _, arg := range args {
			if arg == "origin/main" {
				os.Exit(0)
			}
		}
		os.Exit(1)
	}
	os.Exit(1)
}

func TestExtractGitDiff_Success(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		//nolint:gosec
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
	defer func() { execCommand = origExec }()

	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"git": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestHelperProcess", "--", "pw-mcp-git-mock"},
				Env: []string{
					"GO_WANT_HELPER_PROCESS=1",
					"MOCK_COMMITS_DIFF=committed diff",
					"MOCK_WORKING_DIFF=working tree diff",
				},
			},
		},
	}

	diff, err := ExtractGitDiff(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(diff, "committed diff") {
		t.Errorf("expected diff to contain committed diff, got: %q", diff)
	}
	if !strings.Contains(diff, "working tree diff") {
		t.Errorf("expected diff to contain working tree diff, got: %q", diff)
	}
}

func TestExtractGitDiff_Empty(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		//nolint:gosec
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
	defer func() { execCommand = origExec }()

	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"git": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestHelperProcess", "--", "pw-mcp-git-mock"},
				Env: []string{
					"GO_WANT_HELPER_PROCESS=1",
					"MOCK_COMMITS_DIFF=No changes",
					"MOCK_WORKING_DIFF=No changes",
				},
			},
		},
	}

	diff, err := ExtractGitDiff(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff != "" {
		t.Errorf("expected empty diff, got: %q", diff)
	}
}

func TestExtractGitDiff_Fallback(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		//nolint:gosec
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "MOCK_GH_BASE=fail")
		return cmd
	}
	defer func() { execCommand = origExec }()

	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"git": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestHelperProcess", "--", "pw-mcp-git-mock"},
				Env: []string{
					"GO_WANT_HELPER_PROCESS=1",
					"MOCK_COMMITS_DIFF=diff",
				},
			},
		},
	}

	diff, err := ExtractGitDiff(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(diff, "diff") {
		t.Errorf("expected diff, got: %q", diff)
	}
}

func TestExtractGitDiff_ServerStartFails(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"git": {
				Command: "invalid-command-does-not-exist-xyz123",
			},
		},
	}

	_, err := ExtractGitDiff(context.Background(), cfg)
	if err == nil {
		t.Error("expected error when server fails to start")
	}
}

func TestExtractGitDiff_NoConfig(t *testing.T) {
	cfg := &config.Config{}
	_, _ = ExtractGitDiff(context.Background(), cfg)
}
