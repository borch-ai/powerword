package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var execCommand = exec.CommandContext
var statFile = os.Stat
var lookPath = exec.LookPath

// SetExecCommand sets the execCommand variable in the mcp package for mocking in tests.
func SetExecCommand(f func(context.Context, string, ...string) *exec.Cmd) {
	execCommand = f
}

// ExtractGitDiff starts the external mcp git server and calls its tools to extract working and committed changes.
//
//nolint:gocognit,nestif
func ExtractGitDiff(ctx context.Context, cfg *config.Config) (string, error) {
	srvCfg, ok := cfg.Servers["git"]
	if !ok {
		cmd := "pw-mcp-git"
		if _, err := statFile("bin/pw-mcp-git"); err == nil {
			cmd = "./bin/pw-mcp-git"
		} else if _, err := lookPath("pw-mcp-git"); err != nil {
			return "", fmt.Errorf("pw-mcp-git not found in bin/ or PATH, run 'make all' first")
		}
		srvCfg = config.ServerConfig{
			Command: cmd,
			Args:    []string{},
		}
	}

	srv, err := NewServerProcess(ctx, "git", srvCfg)
	if err != nil {
		return "", fmt.Errorf("failed to start MCP git server (%s): %w", srvCfg.Command, err)
	}
	defer func() {
		_ = srv.GracefulShutdown(time.Second * 5)
	}()

	var diffBuilder strings.Builder

	baseBranch := "main"
	// Check if there's an active PR and use its base
	ghCmd := execCommand(ctx, "gh", "pr", "view", "--json", "baseRefName", "--jq", ".baseRefName")
	out, ghErr := ghCmd.Output()
	if ghErr == nil {
		if b := strings.TrimSpace(string(out)); b != "" {
			baseBranch = b
		}
	} else {
		// Fallback to origin/main if main doesn't exist locally but origin/main does
		gitCmd := execCommand(ctx, "git", "rev-parse", "--verify", "-q", "main")
		if gitCmd.Run() != nil {
			gitCmd2 := execCommand(ctx, "git", "rev-parse", "--verify", "-q", "origin/main")
			if gitCmd2.Run() == nil {
				baseBranch = "origin/main"
			}
		}
	}

	// 1. Get committed changes against base branch
	resCommits, err := srv.Client().CallTool(ctx, "git_diff_commits", map[string]interface{}{"base": baseBranch, "head": "HEAD"})
	if err == nil && !resCommits.IsError && len(resCommits.Content) > 0 {
		if tc, ok := resCommits.Content[0].(*mcpsdk.TextContent); ok {
			text := strings.TrimSpace(tc.Text)
			if text != "No changes" && text != "" {
				fmt.Fprintf(&diffBuilder, "Committed changes (%s -> HEAD):\n", baseBranch)
				diffBuilder.WriteString(text)
				diffBuilder.WriteString("\n\n")
			}
		}
	}

	// 2. Get uncommitted working tree changes
	resWorking, err := srv.Client().CallTool(ctx, "git_diff", map[string]interface{}{})
	if err == nil && !resWorking.IsError && len(resWorking.Content) > 0 {
		if tc, ok := resWorking.Content[0].(*mcpsdk.TextContent); ok {
			text := strings.TrimSpace(tc.Text)
			if text != "No changes" && text != "" {
				diffBuilder.WriteString("Uncommitted changes (Working Tree):\n")
				diffBuilder.WriteString(text)
				diffBuilder.WriteString("\n\n")
			}
		}
	}

	if diffBuilder.Len() == 0 {
		return "", nil
	}

	return strings.TrimSpace(diffBuilder.String()), nil
}
