package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/gdoc"
	"github.com/borch-ai/powerword/pkg/config"
	"golang.org/x/oauth2"
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

func startTestServer(t *testing.T, svc *gdoc.GDocService) (*mcp.ClientSession, context.Context, func()) {
	t.Helper()
	cfg := &config.Config{}
	srv, err := setupServer(cfg, svc)
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

func TestGDoc_MCP_Create(t *testing.T) {
	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/v1/documents") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"documentId": "new-doc-456", "title": "New Doc"}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error": "bad"}`)),
			}, nil
		}),
	}

	cfg := &config.Config{}
	svc := gdoc.NewGDocService(cfg, mockClient)
	session, ctx, cleanup := startTestServer(t, svc)
	defer cleanup()

	// 1. Success case
	args := `{"title": "New Doc"}`
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gdoc_create",
		Arguments: json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	assertResponse(t, res, false, "new-doc-456")

	// 2. Error case (missing title)
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gdoc_create",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	assertResponse(t, res, true, "title parameter is required")
}

func TestGDoc_MCP_Read(t *testing.T) {
	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/v1/documents/doc123") {
				respBody := `{
					"documentId": "doc123",
					"body": {
						"content": [
							{
								"paragraph": {
									"elements": [
										{"textRun": {"content": "Google Doc Content"}}
									]
								}
							}
						]
					}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error": "bad"}`)),
			}, nil
		}),
	}

	cfg := &config.Config{}
	svc := gdoc.NewGDocService(cfg, mockClient)
	session, ctx, cleanup := startTestServer(t, svc)
	defer cleanup()

	// 1. Success case
	args := `{"document_id": "doc123"}`
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gdoc_read",
		Arguments: json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	assertResponse(t, res, false, "Google Doc Content")

	// 2. Error case (missing document_id)
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gdoc_read",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	assertResponse(t, res, true, "document_id parameter is required")
}

func TestGDoc_MCP_Update(t *testing.T) {
	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/v1/documents/doc123") {
				respBody := `{"documentId": "doc123", "body": {"content": [{"endIndex": 5}]}}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/v1/documents/doc123:batchUpdate") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error": "bad"}`)),
			}, nil
		}),
	}

	cfg := &config.Config{}
	svc := gdoc.NewGDocService(cfg, mockClient)
	session, ctx, cleanup := startTestServer(t, svc)
	defer cleanup()

	// Overwrite case
	args := `{"document_id": "doc123", "content": "Updated Content"}`
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gdoc_update",
		Arguments: json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	assertResponse(t, res, false, "Document updated successfully")

	// Append case
	argsAppend := `{"document_id": "doc123", "content": "Appended Content", "append": true}`
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gdoc_update",
		Arguments: json.RawMessage(argsAppend),
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	assertResponse(t, res, false, "Document content appended successfully")

	// Missing document_id case
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gdoc_update",
		Arguments: json.RawMessage(`{"content": "something"}`),
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	assertResponse(t, res, true, "document_id parameter is required")

	// Missing content case
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gdoc_update",
		Arguments: json.RawMessage(`{"document_id": "doc123"}`),
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	assertResponse(t, res, true, "content parameter is required")
}

func TestGDoc_Main_RunErrors(t *testing.T) {
	// 1. runAuthFlow missing credentials_path
	cfg := &config.Config{}
	err := runAuthFlow(cfg)
	if err == nil {
		t.Error("expected error from runAuthFlow with missing config, got nil")
	}

	// 2. runAuthFlow nonexistent credentials file
	cfg2 := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				CredentialsPath: "/nonexistent/creds.json",
				TokenPath:       "/nonexistent/token.json",
			},
		},
	}
	err = runAuthFlow(cfg2)
	if err == nil {
		t.Error("expected error from runAuthFlow on nonexistent credentials, got nil")
	}

	// 3. runAuthFlow credentials file with invalid JSON
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "creds.json")
	err = os.WriteFile(credFile, []byte("invalid json"), 0600)
	if err != nil {
		t.Fatalf("failed to write invalid json: %v", err)
	}
	cfg3 := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				CredentialsPath: credFile,
				TokenPath:       filepath.Join(tmpDir, "token.json"),
			},
		},
	}
	err = runAuthFlow(cfg3)
	if err == nil {
		t.Error("expected error from runAuthFlow on invalid credentials JSON, got nil")
	}
}

func TestGDoc_Main_OpenBrowser_SuccessCases(t *testing.T) {
	// Mock out hooks to avoid spawning real browser processes in tests
	oldBrowserHook := runBrowserCmdHook
	oldWindowsHook := runWindowsBrowserCmdHook
	defer func() {
		runBrowserCmdHook = oldBrowserHook
		runWindowsBrowserCmdHook = oldWindowsHook
	}()

	var browserCmdCalled bool
	var windowsCmdCalled bool

	runBrowserCmdHook = func(name, url string) error {
		browserCmdCalled = true
		return nil
	}
	runWindowsBrowserCmdHook = func(url string) error {
		windowsCmdCalled = true
		return nil
	}

	openBrowser("https://accounts.google.com/o/oauth2/auth")

	// Verify that at least one hook was invoked based on the target OS
	switch runtime.GOOS {
	case "darwin", "linux":
		if !browserCmdCalled {
			t.Error("expected browserCmdHook to be called on non-windows platform")
		}
	case "windows":
		if !windowsCmdCalled {
			t.Error("expected windowsCmdHook to be called on windows platform")
		}
	}

	// Test safety check with port in host
	browserCmdCalled = false
	windowsCmdCalled = false
	openBrowser("https://accounts.google.com:443/o/oauth2/auth")
	switch runtime.GOOS {
	case "darwin", "linux":
		if !browserCmdCalled {
			t.Error("expected browserCmdHook to be called for accounts.google.com:443 URL")
		}
	case "windows":
		if !windowsCmdCalled {
			t.Error("expected windowsCmdHook to be called for accounts.google.com:443 URL")
		}
	}
}

func TestGDoc_Main_OpenBrowser_FailureCases(t *testing.T) {
	// Mock out hooks to avoid spawning real browser processes in tests
	oldBrowserHook := runBrowserCmdHook
	oldWindowsHook := runWindowsBrowserCmdHook
	defer func() {
		runBrowserCmdHook = oldBrowserHook
		runWindowsBrowserCmdHook = oldWindowsHook
	}()

	var browserCmdCalled bool
	var windowsCmdCalled bool

	runBrowserCmdHook = func(name, url string) error {
		browserCmdCalled = true
		return nil
	}
	runWindowsBrowserCmdHook = func(url string) error {
		windowsCmdCalled = true
		return nil
	}

	// 5. Test command execution error returns
	runBrowserCmdHook = func(name, url string) error {
		return errors.New("browser open failed")
	}
	runWindowsBrowserCmdHook = func(url string) error {
		return errors.New("windows browser open failed")
	}
	openBrowser("https://accounts.google.com/o/oauth2/auth")

	// 2. HTTP (invalid scheme)
	browserCmdCalled = false
	windowsCmdCalled = false
	openBrowser("http://accounts.google.com/o/oauth2/auth")
	if browserCmdCalled || windowsCmdCalled {
		t.Error("expected browser hooks not to be called for HTTP URL")
	}

	// 3. Malicious domain
	browserCmdCalled = false
	windowsCmdCalled = false
	openBrowser("https://malicious.com/o/oauth2/auth")
	if browserCmdCalled || windowsCmdCalled {
		t.Error("expected browser hooks not to be called for malicious host URL")
	}

	// 4. Invalid URL parsing
	browserCmdCalled = false
	windowsCmdCalled = false
	openBrowser(":%")
	if browserCmdCalled || windowsCmdCalled {
		t.Error("expected browser hooks not to be called for unparseable URL")
	}
}

func TestGDoc_Main_SaveTokenConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "subdir", "token.json")
	tok := &oauth2.Token{
		AccessToken: "test-access-token",
	}
	err := saveTokenConfigured(tokenPath, tok)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}

	// Verify file content
	//nolint:gosec // G304: test file read is safe
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("failed to read saved token file: %v", err)
	}
	var loaded oauth2.Token
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal token: %v", err)
	}
	if loaded.AccessToken != "test-access-token" {
		t.Errorf("expected access token 'test-access-token', got %q", loaded.AccessToken)
	}
}

func TestGDoc_Main_StartCallbackServer(t *testing.T) {
	stateToken := "test-state-token"
	codeChan := make(chan string, 1)

	server, redirectURL, err := startCallbackServer(stateToken, codeChan)
	if err != nil {
		t.Fatalf("failed to start callback server: %v", err)
	}
	defer func() {
		_ = server.Shutdown(context.Background())
	}()

	// 1. Success case: hit the callback endpoint with matching state and code
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, redirectURL+"?state=test-state-token&code=auth-code-123", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make HTTP request to callback server: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status OK, got %v", resp.Status)
	}

	select {
	case code := <-codeChan:
		if code != "auth-code-123" {
			t.Errorf("expected code 'auth-code-123', got %q", code)
		}
	default:
		t.Error("expected auth code to be sent on channel, but channel was empty")
	}

	// 2. State mismatch case
	req2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, redirectURL+"?state=bad-state&code=auth-code-123", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("failed to make HTTP request to callback server: %v", err)
	}
	defer func() {
		_ = resp2.Body.Close()
	}()

	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status BadRequest for state mismatch, got %v", resp2.Status)
	}

	// 3. Missing code case
	req3, err := http.NewRequestWithContext(context.Background(), http.MethodGet, redirectURL+"?state=test-state-token", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("failed to make HTTP request to callback server: %v", err)
	}
	defer func() {
		_ = resp3.Body.Close()
	}()

	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status BadRequest for missing code, got %v", resp3.Status)
	}
}
