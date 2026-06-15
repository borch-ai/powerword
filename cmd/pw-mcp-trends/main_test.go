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

	"github.com/borch-ai/powerword/internal/plugins/trends"
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

func startTestServer(t *testing.T, workspaceRoot string, svc *trends.TrendsService) (*mcp.ClientSession, context.Context, func()) {
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

func TestTrends_MCP_ScoreNiche(t *testing.T) {
	amazonTransport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.String(), "completion.amazon.com") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["radon",["radon detector","radon home"]]`)),
				Header:     make(http.Header),
			}, nil
		}
		return nil, errors.New("unexpected url")
	})

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.String(), "completion.amazon.com") {
				return amazonTransport.RoundTrip(req)
			}
			if strings.Contains(req.URL.String(), "serpapi.com") {
				respJSON := `{
					"interest_over_time": {
						"timeline_data": [
							{"values": [{"extracted_value": 100}]},
							{"values": [{"extracted_value": 100}]}
						]
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
	cfg.Plugins.Trends.SerpAPIKey = "dummy-key"
	svc := trends.NewTrendsService(cfg, client)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: Score Niche Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "score_niche",
		Arguments: json.RawMessage(`{
			"keyword": "radon",
			"limit": 5,
			"sources": ["amazon", "serp"]
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool score_niche failed: %v", err)
	}
	assertResponse(t, res, false, "radon detector")
	assertResponse(t, res, false, "radon home")
	assertResponse(t, res, false, "radon")

	// Test 2: Score Niche Error (missing required keyword)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "score_niche",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool score_niche error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "keyword parameter is required")
}

func TestTrends_MCP_GetTrendVelocity(t *testing.T) {
	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.String(), "serpapi.com") {
				respJSON := `{
					"interest_over_time": {
						"timeline_data": [
							{"values": [{"extracted_value": 80}]},
							{"values": [{"extracted_value": 80}]},
							{"values": [{"extracted_value": 80}]},
							{"values": [{"extracted_value": 80}]},
							{"values": [{"extracted_value": 80}]},
							{"values": [{"extracted_value": 80}]},
							{"values": [{"extracted_value": 80}]},
							{"values": [{"extracted_value": 80}]},
							{"values": [{"extracted_value": 20}]},
							{"values": [{"extracted_value": 20}]},
							{"values": [{"extracted_value": 20}]},
							{"values": [{"extracted_value": 20}]}
						]
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
	cfg.Plugins.Trends.SerpAPIKey = "dummy-key"
	svc := trends.NewTrendsService(cfg, client)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: Get Trend Velocity Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_trend_velocity",
		Arguments: json.RawMessage(`{
			"keyword": "radon",
			"period": "3-m"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool get_trend_velocity failed: %v", err)
	}
	assertResponse(t, res, false, "declining")
	assertResponse(t, res, false, "trend_score")

	// Test 2: Get Trend Velocity Error (missing required keyword)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_trend_velocity",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool get_trend_velocity error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "keyword parameter is required")
}

func TestTrends_MCP_UnmarshalErrors(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, nil)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "score_niche",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_trend_velocity",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}

func TestRun_ConfigParsing(t *testing.T) {
	// Let's test `run` error when custom config doesn't exist but has bad formatting, etc.
	// Or simply test setupServer initialization.
	tempDir := t.TempDir()
	errSet := os.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)
	if errSet != nil {
		t.Fatalf("failed to set env: %v", errSet)
	}
	defer func() {
		_ = os.Unsetenv("POWERWORD_WORKSPACE_ROOT")
	}()

	// powerword.toml doesn't exist, should load config("")
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
