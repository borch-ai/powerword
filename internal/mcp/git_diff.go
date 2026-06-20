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
func ExtractGitDiff(ctx context.Context, cfg *config.Config) (string, error) {
	srvCfg, err := resolveGitServerConfig(cfg)
	if err != nil {
		return "", err
	}

	srv, err := NewServerProcess(ctx, "git", srvCfg)
	if err != nil {
		return "", fmt.Errorf("failed to start MCP git server (%s): %w", srvCfg.Command, err)
	}
	defer func() {
		_ = srv.GracefulShutdown(time.Second * 5)
	}()

	baseBranch := resolveBaseBranch(ctx)

	var diffBuilder strings.Builder

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

// resolveGitServerConfig returns the ServerConfig for the git MCP process.
func resolveGitServerConfig(cfg *config.Config) (config.ServerConfig, error) {
	if srvCfg, ok := cfg.Servers["git"]; ok {
		return srvCfg, nil
	}
	cmd := "pw-mcp-git"
	if _, err := statFile("bin/pw-mcp-git"); err == nil {
		cmd = "./bin/pw-mcp-git"
	} else if _, err := lookPath("pw-mcp-git"); err != nil {
		return config.ServerConfig{}, fmt.Errorf("pw-mcp-git not found in bin/ or PATH, run 'make all' first")
	}
	return config.ServerConfig{Command: cmd, Args: []string{}}, nil
}

// resolveBaseBranch determines the base branch for diffing: prefers the GitHub PR base,
// falls back to "main" or "origin/main" if no PR is active.
func resolveBaseBranch(ctx context.Context) string {
	ghCmd := execCommand(ctx, "gh", "pr", "view", "--json", "baseRefName", "--jq", ".baseRefName")
	if out, err := ghCmd.Output(); err == nil {
		if b := strings.TrimSpace(string(out)); b != "" {
			return b
		}
	}
	// Fallback: check if main exists locally, otherwise try origin/main
	if execCommand(ctx, "git", "rev-parse", "--verify", "-q", "main").Run() != nil {
		if execCommand(ctx, "git", "rev-parse", "--verify", "-q", "origin/main").Run() == nil {
			return "origin/main"
		}
	}
	return "main"
}
