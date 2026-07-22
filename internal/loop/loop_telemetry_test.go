package loop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	internalmcp "github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/telemetry"
)

func TestGetCostAndUsage_Fallback(t *testing.T) {
	tracker := telemetry.NewUsageTracker()
	tracker.RecordUsage("model-a", telemetry.TokenUsage{InputTokens: 100000, OutputTokens: 200000})

	pricing := map[string]telemetry.ModelPricing{
		"model-a": {Input: 1.0, Output: 2.0},
	}

	// 1. Registry is nil
	in, out, cached, cost, qCap := getCostAndUsage(context.Background(), nil, tracker, pricing, 500000)
	if in != 100000 || out != 200000 || cached != 0 || cost != 0.5 || qCap != 500000 {
		t.Errorf("Expected fallback cost 0.5 and qCap 500000, got: in=%d, out=%d, cached=%d, cost=%f, qCap=%d", in, out, cached, cost, qCap)
	}

	// 2. Registry exists but has no telemetry client
	registry := internalmcp.NewRegistry()
	in, out, cached, cost, qCap = getCostAndUsage(context.Background(), registry, tracker, pricing, 500000)
	if in != 100000 || out != 200000 || cached != 0 || cost != 0.5 || qCap != 500000 {
		t.Errorf("Expected fallback cost 0.5 and qCap 500000, got: in=%d, out=%d, cached=%d, cost=%f, qCap=%d", in, out, cached, cost, qCap)
	}
}

func TestGetCostAndUsage_ViaServer(t *testing.T) {
	ctx := context.Background()

	// Setup mock server responding to calculate_tokens_cost
	srv := mcp.NewServer(&mcp.Implementation{Name: "test-telemetry", Version: "1.0"}, nil)
	srv.AddTool(&mcp.Tool{
		Name:        "calculate_tokens_cost",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp := struct {
			InputTokens   int64   `json:"input_tokens"`
			OutputTokens  int64   `json:"output_tokens"`
			CachedTokens  int64   `json:"cached_tokens"`
			EstimatedCost float64 `json:"estimated_cost"`
			DailyQuotaCap int64   `json:"daily_quota_cap"`
		}{
			InputTokens:   999,
			OutputTokens:  888,
			CachedTokens:  777,
			EstimatedCost: 12.345,
			DailyQuotaCap: 999999,
		}
		respBytes, _ := json.Marshal(resp)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(respBytes)}},
		}, nil
	})

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if err := srv.Run(ctx, t1); err != nil && err != context.Canceled {
			panic(err)
		}
	}()

	mcpClient, err := internalmcp.NewClient(ctx, t2)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = mcpClient.Close() }()

	registry := internalmcp.NewRegistry()
	err = registry.AddClient("telemetry", mcpClient)
	if err != nil {
		t.Fatalf("Failed to add client: %v", err)
	}

	tracker := telemetry.NewUsageTracker()
	in, out, cached, cost, qCap := getCostAndUsage(ctx, registry, tracker, nil, 1000)

	if in != 999 || out != 888 || cached != 777 || cost != 12.345 || qCap != 999999 {
		t.Errorf("Expected values from server, got: in=%d, out=%d, cached=%d, cost=%f, qCap=%d", in, out, cached, cost, qCap)
	}
}

func TestGetCostAndUsage_ServerErrorFallback(t *testing.T) {
	ctx := context.Background()

	// Setup mock server that returns error
	srv := mcp.NewServer(&mcp.Implementation{Name: "test-telemetry", Version: "1.0"}, nil)
	srv.AddTool(&mcp.Tool{
		Name:        "calculate_tokens_cost",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, errors.New("calculation error")
	})

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if err := srv.Run(ctx, t1); err != nil && err != context.Canceled {
			panic(err)
		}
	}()

	mcpClient, err := internalmcp.NewClient(ctx, t2)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = mcpClient.Close() }()

	registry := internalmcp.NewRegistry()
	err = registry.AddClient("telemetry", mcpClient)
	if err != nil {
		t.Fatalf("Failed to add client: %v", err)
	}

	tracker := telemetry.NewUsageTracker()
	tracker.RecordUsage("model-a", telemetry.TokenUsage{InputTokens: 100000, OutputTokens: 200000})

	pricing := map[string]telemetry.ModelPricing{
		"model-a": {Input: 1.0, Output: 2.0},
	}

	// Should fallback to local calculation because server returned error
	in, out, cached, cost, qCap := getCostAndUsage(ctx, registry, tracker, pricing, 500000)
	if in != 100000 || out != 200000 || cached != 0 || cost != 0.5 || qCap != 500000 {
		t.Errorf("Expected fallback cost 0.5 and qCap 500000, got: in=%d, out=%d, cached=%d, cost=%f, qCap=%d", in, out, cached, cost, qCap)
	}
}

func TestCheckBudget_Exceeded(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		MaxCost: 1.0,
		Pricing: map[string]telemetry.ModelPricing{
			"model-a": {Input: 5.0},
		},
	}

	tracker := telemetry.NewUsageTracker()
	// 500,000 * 5.0 / 1,000,000 = 2.50 USD (exceeds 1.0)
	tracker.RecordUsage("model-a", telemetry.TokenUsage{InputTokens: 500000})

	err := checkBudget(ctx, cfg, nil, tracker)
	if err == nil {
		t.Fatal("Expected budget exceeded error, got nil")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("Expected BudgetExceededError, got %T", err)
	}
}

func TestHandleSessionSaveAndOutput_WithTelemetryServer(t *testing.T) {
	ctx := context.Background()

	// Setup mock server
	srv := mcp.NewServer(&mcp.Implementation{Name: "test-telemetry", Version: "1.0"}, nil)
	srv.AddTool(&mcp.Tool{
		Name:        "calculate_tokens_cost",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp := struct {
			InputTokens   int64   `json:"input_tokens"`
			OutputTokens  int64   `json:"output_tokens"`
			CachedTokens  int64   `json:"cached_tokens"`
			EstimatedCost float64 `json:"estimated_cost"`
			DailyQuotaCap int64   `json:"daily_quota_cap"`
		}{
			InputTokens:   100,
			OutputTokens:  200,
			CachedTokens:  50,
			EstimatedCost: 1.23,
			DailyQuotaCap: 1000000,
		}
		respBytes, _ := json.Marshal(resp)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(respBytes)}},
		}, nil
	})

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if err := srv.Run(ctx, t1); err != nil && err != context.Canceled {
			panic(err)
		}
	}()

	mcpClient, err := internalmcp.NewClient(ctx, t2)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = mcpClient.Close() }()

	registry := internalmcp.NewRegistry()
	err = registry.AddClient("telemetry", mcpClient)
	if err != nil {
		t.Fatalf("Failed to add client: %v", err)
	}

	tracker := telemetry.NewUsageTracker()
	tracker.RecordUsage("model-a", telemetry.TokenUsage{InputTokens: 100, OutputTokens: 200, CachedTokens: 50})

	cfg := &config.Config{
		Verbose: true,
	}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err = handleSessionSaveAndOutput(ctx, cfg, registry, nil, "model-a", nil, false, nil, nil, 0, tracker)

	_ = w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	outBytes, _ := io.ReadAll(r)
	_ = r.Close()
	output := string(outBytes)

	if !strings.Contains(output, "Estimated Cost: $1.23000") {
		t.Errorf("Expected output to contain 'Estimated Cost: $1.23000', got: %q", output)
	}
	if !strings.Contains(output, "- Total Tokens: 300 (100 In, 200 Out)") {
		t.Errorf("Expected output to contain correct token count, got: %q", output)
	}
	if !strings.Contains(output, "- Cached Tokens: 50") {
		t.Errorf("Expected output to contain Cached Tokens: 50, got: %q", output)
	}
}
