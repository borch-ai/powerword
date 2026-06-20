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

var seoPluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-seo-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	seoPluginPath = filepath.Join(tmpDir, "pw-mcp-seo")
	cmd := exec.Command("go", "build", "-o", seoPluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-seo: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_SEOPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Mock server for KDP SEO scraping endpoints
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		u := r.URL.Path

		if strings.Contains(u, "/search/complete") {
			_, _ = w.Write([]byte(`["existential",["existential book"]]`))
			return
		}
		if strings.Contains(u, "/s") {
			resp := `<html><body><a href="/dp/B08X5Z8N21">Link</a></body></html>`
			_, _ = w.Write([]byte(resp))
			return
		}
		if strings.Contains(u, "/dp/B08X5Z8N21") {
			resp := `<html><body><span id="productTitle">Test Title</span></body></html>`
			_, _ = w.Write([]byte(resp))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Create temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-seo-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Setup server config with RateLimitMS = 1 to keep integration tests fast
	cfgTOML := `
[plugins.seo]
rate_limit_ms = 1
cache_ttl_hours = -1
`
	if err := os.WriteFile(filepath.Join(workspaceDir, "powerword.toml"), []byte(cfgTOML), 0600); err != nil {
		t.Fatal(err)
	}

	srvCfg := config.ServerConfig{
		Command: seoPluginPath,
		Env: []string{
			"POWERWORD_WORKSPACE_ROOT=" + workspaceDir,
			"POWERWORD_SEO_API_ENDPOINT=" + mockServer.URL,
		},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-seo", srvCfg)
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

	foundAnalyze := false
	foundGen := false
	for _, tool := range tools {
		if tool.Name == "seo_analyze_niche" {
			foundAnalyze = true
		}
		if tool.Name == "seo_generate_listing" {
			foundGen = true
		}
	}
	if !foundAnalyze || !foundGen {
		t.Errorf("expected to find SEO tools, got tools: %+v", tools)
	}

	// 2. CallTool: seo_analyze_niche
	args := map[string]interface{}{
		"query": "existential",
	}
	result, err := client.CallTool(ctx, "seo_analyze_niche", args)
	if err != nil {
		t.Fatalf("failed to call seo_analyze_niche tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	resStr, err := mcp.FormatToolResult(result)
	if err != nil {
		t.Fatalf("failed to format tool result: %v", err)
	}

	if !strings.Contains(resStr, "existential book") || !strings.Contains(resStr, "Test Title") {
		t.Errorf("expected suggestions and competitor title in result, got: %q", resStr)
	}
}
