package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"powerword/internal/config"
	"powerword/internal/llm"
	"powerword/internal/plugins/seo"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

type mockLLM struct {
	response *llm.Message
	err      error
}

func (m *mockLLM) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Message, error) {
	return m.response, m.err
}

func (m *mockLLM) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockLLM) ListModels(ctx context.Context) ([]string, error) {
	return []string{"mock"}, nil
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

func startTestServer(t *testing.T, workspaceRoot string, svc *seo.SEOService) (*mcp.ClientSession, context.Context, func()) {
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

func TestSEO_MCP_AnalyzeNiche(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	transport := mockRoundTripper(func(req *http.Request) (*http.Response, error) {
		u := req.URL.String()
		if strings.Contains(u, "completion.amazon.com") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["existential",["existential book"]]`)),
				Header:     make(http.Header),
			}, nil
		}
		if strings.Contains(u, "amazon.com/s?") {
			resp := `<html><body><a href="/dp/B08X5Z8N21">Link</a></body></html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(resp)),
				Header:     make(http.Header),
			}, nil
		}
		if strings.Contains(u, "amazon.com/dp/") {
			resp := `<html><body><span id="productTitle">Test Title</span></body></html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(resp)),
				Header:     make(http.Header),
			}, nil
		}
		return nil, errors.New("unexpected call")
	})

	cfg := &config.Config{}
	cfg.Plugins.SEO.CacheTTLHours = -1
	cfg.Plugins.SEO.RateLimitMS = 1

	svc := seo.NewSEOService(cfg)
	svc.SetHTTPClient(&http.Client{Transport: transport})

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: Analyze Niche Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "seo_analyze_niche",
		Arguments: json.RawMessage(`{
			"query": "existential"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool seo_analyze_niche failed: %v", err)
	}
	assertResponse(t, res, false, "existential book")
	assertResponse(t, res, false, "Test Title")

	// Test 2: Analyze Niche Error (missing parameters)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "seo_analyze_niche",
		Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("CallTool seo_analyze_niche error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "either query or asins must be specified")
}

func TestSEO_MCP_GenerateListing(t *testing.T) {
	llmResponse := `
	{
		"title": "Optimized Title",
		"subtitle": "Optimized Subtitle",
		"keywords": ["kw1", "kw2", "kw3", "kw4", "kw5", "kw6", "kw7"],
		"description": "Best book description"
	}
	`
	mockL := &mockLLM{
		response: &llm.Message{
			Role:    llm.RoleAssistant,
			Content: llmResponse,
		},
	}

	cfg := &config.Config{}
	svc := seo.NewSEOService(cfg)
	svc.SetLLMClient(mockL)

	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, svc)
	defer cleanup()

	// Test 1: Generate Listing Success
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "seo_generate_listing",
		Arguments: json.RawMessage(`{
			"niche": "Existential Nursery Rhymes",
			"target_audience": "Toddlers"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool seo_generate_listing failed: %v", err)
	}
	assertResponse(t, res, false, "Optimized Title")
	assertResponse(t, res, false, "Best book description")

	// Test 2: Generate Listing Error (missing required niche)
	errRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "seo_generate_listing",
		Arguments: json.RawMessage(`{
			"target_audience": "Toddlers"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool seo_generate_listing error call failed: %v", err)
	}
	assertResponse(t, errRes, true, "niche parameter is required")
}

func TestSEO_MCP_UnmarshalErrors(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir, nil)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "seo_analyze_niche",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "seo_generate_listing",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}
