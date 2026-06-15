//go:build integration

package main_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
)

var pluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-trends-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-trends")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-trends: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_TrendsPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Spin up mock server for Amazon autocomplete and SerpAPI Trends
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/search/complete") {
			// Amazon mock suggestion list
			resp := `["radon detector",["radon detector home","radon detector charcoal"],[],[],"12345"]`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}

		if strings.Contains(r.URL.Path, "/search") && r.URL.Query().Get("engine") == "google_trends" {
			// SerpAPI mock trends response
			resp := `{
				"interest_over_time": {
					"timeline_data": [
						{"values": [{"extracted_value": 20}]},
						{"values": [{"extracted_value": 20}]},
						{"values": [{"extracted_value": 20}]},
						{"values": [{"extracted_value": 20}]},
						{"values": [{"extracted_value": 20}]},
						{"values": [{"extracted_value": 20}]},
						{"values": [{"extracted_value": 20}]},
						{"values": [{"extracted_value": 20}]},
						{"values": [{"extracted_value": 80}]},
						{"values": [{"extracted_value": 80}]},
						{"values": [{"extracted_value": 80}]},
						{"values": [{"extracted_value": 80}]}
					]
				}
			}`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}

		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "not found"}`))
	}))
	defer mockServer.Close()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-trends-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Write mock powerword.toml containing the SerpAPI key config
	cfgTOML := `
[plugins.trends]
serp_api_key = "dummy-serp-key"
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env: []string{
			"POWERWORD_WORKSPACE_ROOT=" + workspaceDir,
			"POWERWORD_AMAZON_BASE_URL=" + mockServer.URL,
			"POWERWORD_SERPAPI_BASE_URL=" + mockServer.URL,
		},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-trends", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() {
		_ = sp.GracefulShutdown(1 * time.Second)
	}()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	// 1. Handshake / ListTools
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	foundScoreNiche := false
	foundGetTrendVelocity := false
	for _, tool := range tools {
		if tool.Name == "score_niche" {
			foundScoreNiche = true
		}
		if tool.Name == "get_trend_velocity" {
			foundGetTrendVelocity = true
		}
	}
	if !foundScoreNiche {
		t.Errorf("expected to find 'score_niche' tool, got tools: %+v", tools)
	}
	if !foundGetTrendVelocity {
		t.Errorf("expected to find 'get_trend_velocity' tool, got tools: %+v", tools)
	}

	// 2. CallTool to score niche
	argsScore := map[string]interface{}{
		"keyword": "radon detector",
		"limit":   10,
		"sources": []string{"amazon", "serp"},
	}
	result, err := client.CallTool(ctx, "score_niche", argsScore)
	if err != nil {
		t.Fatalf("failed to call score_niche tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool execution score_niche returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if !strings.Contains(resStr, "radon detector home") {
		t.Errorf("expected output to contain suggestions, got: %q", resStr)
	}

	// 3. CallTool to get trend velocity
	argsVelocity := map[string]interface{}{
		"keyword": "radon detector",
		"period":  "3-m",
	}
	resultVelocity, err := client.CallTool(ctx, "get_trend_velocity", argsVelocity)
	if err != nil {
		t.Fatalf("failed to call get_trend_velocity tool: %v", err)
	}
	if resultVelocity.IsError {
		t.Fatalf("tool execution get_trend_velocity returned error: %v", resultVelocity)
	}

	resStrVelocity, err := mcp.FormatToolResult(resultVelocity)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}
	if !strings.Contains(resStrVelocity, "rising") || !strings.Contains(resStrVelocity, "1") {
		t.Errorf("expected velocity response to contain rising and score 1.0, got: %q", resStrVelocity)
	}
}
