package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/mcp/critic"
	"github.com/borch-ai/powerword/pkg/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func setupWorkspaceRoot(workspaceRoot string) (string, error) {
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get cwd: %w", err)
		}
		return cwd, nil
	}

	absPath, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path of workspace root: %w", err)
	}

	if err := os.Chdir(absPath); err != nil {
		return "", fmt.Errorf("failed to change working directory to %s: %w", absPath, err)
	}

	return absPath, nil
}

func run() error {
	workspaceRoot, err := setupWorkspaceRoot(os.Getenv("POWERWORD_WORKSPACE_ROOT"))
	if err != nil {
		return err
	}

	cfg, err := config.LoadFromWorkspace(workspaceRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	srv, err := critic.SetupServer(workspaceRoot, cfg)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}
