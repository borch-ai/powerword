package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

type testCase struct {
	name       string
	toolName   string
	arguments  map[string]interface{}
	wantError  bool
	wantSubstr string
}

func getTestCases(tempDir, innerFile, outerDir, outerFile string) []testCase {
	return []testCase{
		{
			name:       "read_file inside sandbox",
			toolName:   "read_file",
			arguments:  map[string]interface{}{"path": innerFile},
			wantError:  false,
			wantSubstr: "inner",
		},
		{
			name:       "read_file outside sandbox absolute",
			toolName:   "read_file",
			arguments:  map[string]interface{}{"path": outerFile},
			wantError:  true,
			wantSubstr: "no such file",
		},
		{
			name:       "read_file outside sandbox relative",
			toolName:   "read_file",
			arguments:  map[string]interface{}{"path": filepath.Join(tempDir, "..", "etc", "passwd")},
			wantError:  true,
			wantSubstr: "no such file",
		},
		{
			name:       "write_file inside sandbox",
			toolName:   "write_file",
			arguments:  map[string]interface{}{"path": filepath.Join(tempDir, "new.txt"), "content": "new"},
			wantError:  false,
			wantSubstr: "Successfully wrote",
		},
		{
			name:       "write_file outside sandbox",
			toolName:   "write_file",
			arguments:  map[string]interface{}{"path": outerFile, "content": "new"},
			wantError:  false,
			wantSubstr: "Successfully wrote",
		},
		{
			name:       "search_grep inside sandbox",
			toolName:   "search_grep",
			arguments:  map[string]interface{}{"pattern": "in+", "path": tempDir},
			wantError:  false,
			wantSubstr: "inner",
		},
		{
			name:       "search_grep outside sandbox",
			toolName:   "search_grep",
			arguments:  map[string]interface{}{"pattern": "out+", "path": outerDir},
			wantError:  true,
			wantSubstr: "no such file",
		},
		{
			name:       "list_directory inside sandbox",
			toolName:   "list_directory",
			arguments:  map[string]interface{}{"path": tempDir},
			wantError:  false,
			wantSubstr: "inner.txt",
		},
		{
			name:       "list_directory outside sandbox",
			toolName:   "list_directory",
			arguments:  map[string]interface{}{"path": outerDir},
			wantError:  true,
			wantSubstr: "no such file",
		},
	}
}

func TestFS_Sandbox(t *testing.T) {
	tempDir := t.TempDir()

	innerFile := filepath.Join(tempDir, "inner.txt")
	if err := os.WriteFile(innerFile, []byte("inner"), 0600); err != nil {
		t.Fatalf("failed to write inner file: %v", err)
	}

	outerDir := t.TempDir()
	outerFile := filepath.Join(outerDir, "outer.txt")
	if err := os.WriteFile(outerFile, []byte("outer"), 0600); err != nil {
		t.Fatalf("failed to write outer file: %v", err)
	}

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

	tests := getTestCases(tempDir, innerFile, outerDir, outerFile)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tc.toolName,
				Arguments: tc.arguments,
			})
			if err != nil {
				t.Fatalf("CallTool failed: %v", err)
			}
			assertResponse(t, res, tc.wantError, tc.wantSubstr)
		})
	}
}
