//go:build integration

package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
)

var pluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-telemetry-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-telemetry")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-telemetry: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestMCP_TelemetryPlugin_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-telemetry", srvCfg)
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

	foundCalc := false
	foundPricing := false
	for _, tool := range tools {
		if tool.Name == "calculate_tokens_cost" {
			foundCalc = true
		}
		if tool.Name == "get_model_pricing" {
			foundPricing = true
		}
	}
	if !foundCalc || !foundPricing {
		t.Errorf("expected to find calculate_tokens_cost and get_model_pricing tools, got: %+v", tools)
	}

	// 2. Call calculate_tokens_cost
	args := map[string]interface{}{
		"model_usages": map[string]interface{}{
			"gemini-1.5-pro": map[string]interface{}{
				"input_tokens":  2000000,
				"output_tokens": 1000000,
				"cached_tokens": 0,
			},
		},
	}
	result, err := client.CallTool(ctx, "calculate_tokens_cost", args)
	if err != nil {
		t.Fatalf("failed to call calculate_tokens_cost: %v", err)
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}

	txtContent, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}

	var calculated struct {
		EstimatedCost float64 `json:"estimated_cost"`
	}
	if err := json.Unmarshal([]byte(txtContent.Text), &calculated); err != nil {
		t.Fatalf("failed to unmarshal result text: %v", err)
	}

	if calculated.EstimatedCost != 6.25 {
		t.Errorf("expected estimated cost 6.25, got %f", calculated.EstimatedCost)
	}
}
