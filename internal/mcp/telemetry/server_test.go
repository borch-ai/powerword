package telemetry

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/pkg/telemetry"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestTelemetryServer_CalculateTokensCost_Simple(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(runErr)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	args := map[string]interface{}{
		"model_usages": map[string]interface{}{
			"gemini-1.5-pro": map[string]interface{}{
				"input_tokens":  2000000, // 2M input tokens
				"output_tokens": 1000000, // 1M output tokens
				"cached_tokens": 0,
			},
		},
	}
	res, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "calculate_tokens_cost",
		Arguments: args,
	})
	if callErr != nil {
		t.Fatalf("CallTool failed: %v", callErr)
	}

	if res.IsError {
		t.Fatalf("Expected no error, got result error: %v", res.Content)
	}

	if len(res.Content) != 1 {
		t.Fatalf("Expected exactly 1 content block, got %d", len(res.Content))
	}

	txtContent, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent block, got %T", res.Content[0])
	}

	var calculated struct {
		InputTokens   int64   `json:"input_tokens"`
		OutputTokens  int64   `json:"output_tokens"`
		CachedTokens  int64   `json:"cached_tokens"`
		EstimatedCost float64 `json:"estimated_cost"`
		DailyQuotaCap int64   `json:"daily_quota_cap"`
	}

	if parseErr := json.Unmarshal([]byte(txtContent.Text), &calculated); parseErr != nil {
		t.Fatalf("Failed to parse response: %v", parseErr)
	}

	// Cost of gemini-1.5-pro = Input: 1.25 / 1M, Output: 3.75 / 1M.
	// Cost = 2 * 1.25 + 1 * 3.75 = 2.50 + 3.75 = 6.25
	if !almostEqual(calculated.EstimatedCost, 6.25) {
		t.Errorf("Expected estimated cost 6.25, got %f", calculated.EstimatedCost)
	}
	if calculated.InputTokens != 2000000 {
		t.Errorf("Expected 2000000 input tokens, got %d", calculated.InputTokens)
	}
	if calculated.OutputTokens != 1000000 {
		t.Errorf("Expected 1000000 output tokens, got %d", calculated.OutputTokens)
	}
	if calculated.DailyQuotaCap != 1000000 {
		t.Errorf("Expected default daily quota cap 1000000, got %d", calculated.DailyQuotaCap)
	}
}

func TestTelemetryServer_CalculateTokensCost_Prefix(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(runErr)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	argsPrefix := map[string]interface{}{
		"model_usages": map[string]interface{}{
			"gemini-1.5-pro-latest": map[string]interface{}{
				"input_tokens":  2000000,
				"output_tokens": 1000000,
				"cached_tokens": 0,
			},
		},
	}
	resPrefix, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "calculate_tokens_cost",
		Arguments: argsPrefix,
	})
	if callErr != nil {
		t.Fatalf("CallTool prefix failed: %v", callErr)
	}

	var calculatedPrefix struct {
		EstimatedCost float64 `json:"estimated_cost"`
	}
	txtContentPrefix := resPrefix.Content[0].(*mcp.TextContent)
	if parseErr := json.Unmarshal([]byte(txtContentPrefix.Text), &calculatedPrefix); parseErr != nil {
		t.Fatalf("Failed to parse prefix response: %v", parseErr)
	}

	if !almostEqual(calculatedPrefix.EstimatedCost, 6.25) {
		t.Errorf("Expected estimated cost 6.25 for prefix matched model, got %f", calculatedPrefix.EstimatedCost)
	}
}

func TestTelemetryServer_CalculateTokensCost_Overrides(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(runErr)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	argsOverrides := map[string]interface{}{
		"model_usages": map[string]interface{}{
			"custom-model-foo": map[string]interface{}{
				"input_tokens":  100000,
				"output_tokens": 200000,
				"cached_tokens": 50000,
			},
		},
		"daily_quota_cap": 5000000,
		"pricing_overrides": map[string]interface{}{
			"custom-model": map[string]interface{}{
				"input":  2.0, // 2.0 per 1M input
				"output": 4.0, // 4.0 per 1M output
				"cached": 0.5, // 0.5 per 1M cached
			},
		},
	}
	resOverrides, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "calculate_tokens_cost",
		Arguments: argsOverrides,
	})
	if callErr != nil {
		t.Fatalf("CallTool overrides failed: %v", callErr)
	}

	var calculatedOverrides struct {
		InputTokens   int64   `json:"input_tokens"`
		OutputTokens  int64   `json:"output_tokens"`
		CachedTokens  int64   `json:"cached_tokens"`
		EstimatedCost float64 `json:"estimated_cost"`
		DailyQuotaCap int64   `json:"daily_quota_cap"`
	}
	txtContentOverrides := resOverrides.Content[0].(*mcp.TextContent)
	if parseErr := json.Unmarshal([]byte(txtContentOverrides.Text), &calculatedOverrides); parseErr != nil {
		t.Fatalf("Failed to parse overrides response: %v", parseErr)
	}

	// custom-model-foo matches prefix "custom-model".
	// Billed input = 100k - 50k = 50k input tokens.
	// Input cost = 50,000 * (2.0 / 1,000,000) = 0.10
	// Output cost = 200,000 * (4.0 / 1,000,000) = 0.80
	// Cached cost = 50,000 * (0.5 / 1,000,000) = 0.025
	// Total estimated cost = 0.10 + 0.80 + 0.025 = 0.925
	if !almostEqual(calculatedOverrides.EstimatedCost, 0.925) {
		t.Errorf("Expected overrides cost 0.925, got %f", calculatedOverrides.EstimatedCost)
	}
	if calculatedOverrides.DailyQuotaCap != 5000000 {
		t.Errorf("Expected overrides quota cap 5000000, got %d", calculatedOverrides.DailyQuotaCap)
	}
}

func TestTelemetryServer_GetModelPricing(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(runErr)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	// 1. Get all pricing
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_model_pricing",
		Arguments: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	txtContent := res.Content[0].(*mcp.TextContent)
	var pricing map[string]telemetry.ModelPricing
	if parseErr := json.Unmarshal([]byte(txtContent.Text), &pricing); parseErr != nil {
		t.Fatalf("Failed to parse all pricing response: %v", parseErr)
	}

	if len(pricing) == 0 {
		t.Errorf("Expected non-empty pricing database")
	}

	// 2. Get specific model pricing
	resModel, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_model_pricing",
		Arguments: map[string]interface{}{
			"model": "gemini-1.5-pro",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	txtContentModel := resModel.Content[0].(*mcp.TextContent)
	var singleModel telemetry.ModelPricing
	if parseErr := json.Unmarshal([]byte(txtContentModel.Text), &singleModel); parseErr != nil {
		t.Fatalf("Failed to parse single model pricing response: %v", parseErr)
	}

	if singleModel.Input != 1.25 || singleModel.Output != 3.75 {
		t.Errorf("Unexpected pricing values for gemini-1.5-pro: %+v", singleModel)
	}

	// 3. Test non-existent model error
	resErr, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_model_pricing",
		Arguments: map[string]interface{}{
			"model": "non-existent-model-name",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if !resErr.IsError {
		t.Errorf("Expected IsError to be true for non-existent model")
	}

	txtContentErr := resErr.Content[0].(*mcp.TextContent)
	if !strings.Contains(txtContentErr.Text, "no pricing found for model") {
		t.Errorf("Expected error message containing 'no pricing found for model', got %q", txtContentErr.Text)
	}
}

func TestTelemetryServer_ErrorsAndEdgeCases(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("Failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(runErr)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	// 1. Invalid Arguments for calculate_tokens_cost (should fail)
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "calculate_tokens_cost",
		Arguments: map[string]interface{}{"model_usages": "invalid-type"},
	})
	if err == nil {
		t.Errorf("Expected error for invalid arguments format, got nil")
	}

	// 2. Billed input less than 0 (cached > input)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "calculate_tokens_cost",
		Arguments: map[string]interface{}{
			"model_usages": map[string]interface{}{
				"gemini-1.5-pro": map[string]interface{}{
					"input_tokens":  50,
					"output_tokens": 10,
					"cached_tokens": 100, // cached > input
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	var calculated struct {
		EstimatedCost float64 `json:"estimated_cost"`
	}
	txtContent := res.Content[0].(*mcp.TextContent)
	if parseErr := json.Unmarshal([]byte(txtContent.Text), &calculated); parseErr != nil {
		t.Fatalf("Failed to parse response: %v", parseErr)
	}

	// Billed input = 0 (since 50 - 100 < 0).
	// Cost = 0 * InputPrice + 10 * OutputPrice + 100 * CachedPrice
	// gemini-1.5-pro pricing = Input: 1.25, Output: 3.75, Cached: 0.3125 (per 1M)
	expected := (10.0 * 3.75 / 1e6) + (100.0 * 0.3125 / 1e6)
	if !almostEqual(calculated.EstimatedCost, expected) {
		t.Errorf("Expected cost %f, got %f", expected, calculated.EstimatedCost)
	}

	// 3. Invalid Arguments for get_model_pricing (should fail)
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_model_pricing",
		Arguments: map[string]interface{}{"model": 123}, // invalid model type (expects string)
	})
	if err == nil {
		t.Errorf("Expected error for invalid get_model_pricing arguments, got nil")
	}

	// 4. Negative token count should fail
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "calculate_tokens_cost",
		Arguments: map[string]interface{}{
			"model_usages": map[string]interface{}{
				"gemini-1.5-pro": map[string]interface{}{
					"input_tokens":  -50,
					"output_tokens": 10,
					"cached_tokens": 0,
				},
			},
		},
	})
	if err == nil {
		t.Errorf("Expected error for negative token counts, got nil")
	}
}
