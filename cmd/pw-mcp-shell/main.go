package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	workspaceRoot := os.Getenv("POWERWORD_WORKSPACE_ROOT")
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get cwd: %w", err)
		}
		workspaceRoot = cwd
	}

	srv, err := setupServer(workspaceRoot)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

func setupServer(workspaceRoot string) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-shell",
		Version: "1.0.0",
	}, nil)

	// Hardcoded deny-list for high-risk commands
	denyList := []string{
		"rm", "mkfs", "dd", "sudo", "su", "shutdown", "reboot",
		"poweroff", "halt", "format", "chmod", "chown",
	}

	isDenied := func(cmd string) bool {
		baseCmd := strings.ToLower(strings.TrimSpace(cmd))
		for _, denied := range denyList {
			if baseCmd == denied {
				return true
			}
		}
		return false
	}

	srv.AddTool(&mcp.Tool{
		Name:        "run_command",
		Description: "Executes a shell command. Subject to security checks.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"The executable command"},"args":{"type":"array","items":{"type":"string"},"description":"Arguments for the command"}},"required":["command"]}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return nil, err
		}

		if isDenied(input.Command) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Execution of '%s' is blocked by security policy.", input.Command)}}}, nil
		}

		//nolint:gosec // command is checked by deny-list
		cmd := exec.CommandContext(ctx, input.Command, input.Args...)
		cmd.Dir = workspaceRoot

		output, err := cmd.CombinedOutput()
		outStr := string(output)

		if err != nil {
			msg := fmt.Sprintf("Command failed: %v\nOutput: %s", err, outStr)
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: outStr}}}, nil
	})

	return srv, nil
}
