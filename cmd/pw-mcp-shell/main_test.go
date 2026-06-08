package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func assertResponse(t *testing.T, res *mcp.CallToolResult, wantError bool, wantSubstr string) {
	t.Helper()
	if res.IsError != wantError {
		t.Errorf("expected error: %v, got: %v (content: %v)", wantError, res.IsError, res.Content)
	}
	if len(res.Content) == 0 {
		t.Errorf("no content in response")
		return
	}
	var contentStr string
	if txt, ok := res.Content[0].(*mcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(res.Content[0])
	}
	if !strings.Contains(contentStr, wantSubstr) {
		t.Errorf("expected response to contain %q, got %q", wantSubstr, contentStr)
	}
}

func TestShellMCP(t *testing.T) {
	tempDir := t.TempDir()

	srv, err := setupServer(tempDir)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(fmt.Errorf("server run err: %v", runErr))
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	tests := []struct {
		name       string
		command    string
		args       []string
		wantError  bool
		wantSubstr string
	}{
		{
			name:       "safe command",
			command:    "echo",
			args:       []string{"hello world"},
			wantError:  false,
			wantSubstr: "hello world",
		},
		{
			name:       "denied command",
			command:    "rm",
			args:       []string{"-rf", "/"},
			wantError:  true,
			wantSubstr: "blocked by security policy",
		},
		{
			name:       "invalid command",
			command:    "this_command_does_not_exist_xyz123",
			args:       []string{},
			wantError:  true,
			wantSubstr: "Command failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "run_command",
				Arguments: map[string]interface{}{"command": tc.command, "args": tc.args},
			})
			if err != nil {
				t.Fatalf("CallTool failed: %v", err)
			}
			assertResponse(t, res, tc.wantError, tc.wantSubstr)
		})
	}
}
