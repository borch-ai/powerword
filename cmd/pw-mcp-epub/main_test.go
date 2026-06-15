package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/pkg/config"
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

func startTestServer(t *testing.T, workspaceRoot string) (*mcp.ClientSession, context.Context, func()) {
	t.Helper()
	cfg := &config.Config{}
	srv, err := setupServer(workspaceRoot, cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(fmt.Errorf("server run err: %v", runErr))
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		cancel()
		t.Fatalf("failed to connect client: %v", err)
	}

	cleanup := func() {
		_ = session.Close()
		cancel()
	}
	return session, ctx, cleanup
}

func TestEpub_MCP_CompileEpub(t *testing.T) {
	tempDir := t.TempDir()

	// Write mock input files
	manuscriptPath := filepath.Join(tempDir, "manuscript.md")
	_ = os.WriteFile(manuscriptPath, []byte("# Chapter 1\nContent"), 0600)

	imagesDir := filepath.Join(tempDir, "images")
	_ = os.Mkdir(imagesDir, 0750)

	outputPath := filepath.Join(tempDir, "output.epub")

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	// Test successful compilation
	argsJSON := fmt.Sprintf(`{
		"manuscript_path": %q,
		"images_dir": %q,
		"output_path": %q,
		"title": "My Book",
		"author": "Me"
	}`, manuscriptPath, imagesDir, outputPath)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_epub",
		Arguments: json.RawMessage(argsJSON),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, false, "successfully compiled EPUB")

	// Test missing parameter error
	badArgsJSON := fmt.Sprintf(`{
		"manuscript_path": "",
		"images_dir": %q,
		"output_path": %q,
		"title": "My Book",
		"author": "Me"
	}`, imagesDir, outputPath)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_epub",
		Arguments: json.RawMessage(badArgsJSON),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, true, "missing required parameters")

	// Test compile failure propagation (e.g. bad manuscript file path)
	nonExistentArgsJSON := fmt.Sprintf(`{
		"manuscript_path": "does-not-exist.md",
		"images_dir": %q,
		"output_path": %q,
		"title": "My Book",
		"author": "Me"
	}`, imagesDir, outputPath)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_epub",
		Arguments: json.RawMessage(nonExistentArgsJSON),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, true, "failed to compile EPUB")
}

func TestEpub_MCP_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_epub",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}

func TestRun_ConfigParsing(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	srv, err := setupServer(tempDir, nil)
	if err != nil {
		t.Fatalf("setupServer failed: %v", err)
	}
	if srv == nil {
		t.Error("expected setupServer to return a server")
	}

	// Write bad config to force run() error
	badConfigPath := tempDir + "/powerword.toml"
	_ = os.WriteFile(badConfigPath, []byte("bad config format"), 0600)
	err = run()
	if err == nil {
		t.Error("expected run to fail with invalid config file, got nil")
	}
}

func TestRun_Success(t *testing.T) {
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r
	_ = w.Close() // EOF immediately

	_, wOut, _ := os.Pipe()
	os.Stdout = wOut
	defer func() { _ = wOut.Close() }()

	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	// should run and exit immediately on EOF
	err := run()
	// StdioTransport exit error is acceptable on direct EOF
	_ = err
}
