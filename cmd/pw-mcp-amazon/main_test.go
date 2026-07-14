package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/amazon"
	"github.com/borch-ai/powerword/pkg/config"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

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

func startTestServer(t *testing.T, workspaceRoot string, svc *amazon.AmazonService) (*mcp.ClientSession, context.Context, func()) {
	t.Helper()
	cfg := &config.Config{}
	srv, err := setupServer(workspaceRoot, cfg, svc)
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

func TestAmazon_MCP_GetListingCount(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.String(), "api.scaleserp.com") {
				respJSON := `{
					"search_information": {
						"total_results": 1234
					}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respJSON)),
					Header:     make(http.Header),
				}, nil
			}
			return nil, errors.New("unexpected url")
		}),
	}

	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "dummy-key"
	svc := amazon.NewAmazonService(cfg, client)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_amazon_listing_count",
		Arguments: json.RawMessage(`{
			"keyword": "radon detector"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool get_amazon_listing_count failed: %v", err)
	}
	assertResponse(t, res, false, "listing_count")
	assertResponse(t, res, false, "1234")

	// Test 2: Error (missing required keyword)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_amazon_listing_count",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool get_amazon_listing_count error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "keyword parameter is required")
}

func TestAmazon_MCP_GetListingCount_ServiceError(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("internal server error")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	cfg := &config.Config{}
	cfg.Plugins.Amazon.APIKey = "real-key"
	svc := amazon.NewAmazonService(cfg, client)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_amazon_listing_count",
		Arguments: json.RawMessage(`{
			"keyword": "test"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, true, "failed to get listing count")
}

func TestAmazon_MCP_UnmarshalErrors(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, nil)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_amazon_listing_count",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}

func TestRun_ConfigParsing(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	srv, err := setupServer(tempDir, nil, nil)
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
	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	// Write a valid empty config file
	configPath := tempDir + "/powerword.toml"
	_ = os.WriteFile(configPath, []byte(""), 0600)

	// Keep stdin closed to let Run exit immediately
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	_ = w.Close()
	os.Stdin = r

	// Run should exit immediately with no error or EOF
	err = run()
	if err != nil && !strings.Contains(err.Error(), "EOF") && !errors.Is(err, io.EOF) {
		t.Errorf("expected nil or EOF error, got %v", err)
	}
}
