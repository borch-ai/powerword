package telemetry

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/pkg/telemetry"
)

// defaultPricing specifies the fallback pricing configuration for common models.
var defaultPricing = map[string]telemetry.ModelPricing{
	"gemini-1.5-pro":    {Input: 1.25, Output: 3.75, Cached: 0.3125},
	"gemini-1.5-flash":  {Input: 0.075, Output: 0.30, Cached: 0.01875},
	"gemini-2.5-flash":  {Input: 0.075, Output: 0.30, Cached: 0.01875},
	"gpt-4o":            {Input: 5.00, Output: 15.00},
	"gpt-4o-mini":       {Input: 0.150, Output: 0.600},
	"claude-3-5-sonnet": {Input: 3.00, Output: 15.00},
}

//nolint:gosec // G101: JSON schema containing 'tokens' is not a hardcoded credential
const calculateTokensCostSchema = `{
	"type": "object",
	"properties": {
		"model_usages": {
			"type": "object",
			"description": "Map of model names to their token counts (input_tokens, output_tokens, cached_tokens).",
			"additionalProperties": {
				"type": "object",
				"properties": {
					"input_tokens": { "type": "integer", "minimum": 0 },
					"output_tokens": { "type": "integer", "minimum": 0 },
					"cached_tokens": { "type": "integer", "minimum": 0 }
				}
			}
		},
		"daily_quota_cap": {
			"type": "integer",
			"description": "Optional daily quota cap. Defaults to 1,000,000."
		},
		"pricing_overrides": {
			"type": "object",
			"description": "Optional custom pricing overrides mapping model prefix to pricing.",
			"additionalProperties": {
				"type": "object",
				"properties": {
					"input": { "type": "number" },
					"output": { "type": "number" },
					"cached": { "type": "number" }
				}
			}
		}
	},
	"required": ["model_usages"]
}`

const getModelPricingSchema = `{
	"type": "object",
	"properties": {
		"model": {
			"type": "string",
			"description": "Optional specific model name to query."
		}
	}
}`

// SetupServer creates and configures the telemetry MCP server.
func SetupServer() (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-telemetry",
		Version: "1.0.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "calculate_tokens_cost",
		Description: "Aggregates token usage across models and calculates the estimated cost.",
		InputSchema: json.RawMessage(calculateTokensCostSchema),
	}, handleCalculateTokensCost(defaultPricing))

	srv.AddTool(&mcp.Tool{
		Name:        "get_model_pricing",
		Description: "Retrieves the active pricing database or details for a specific model.",
		InputSchema: json.RawMessage(getModelPricingSchema),
	}, handleGetModelPricing(defaultPricing))

	return srv, nil
}

func calculateModelCost(model string, usage telemetry.ModelUsage, pricing map[string]telemetry.ModelPricing) (float64, error) {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CachedTokens < 0 {
		return 0, fmt.Errorf("negative token counts are not allowed: model=%s, input=%d, output=%d, cached=%d", model, usage.InputTokens, usage.OutputTokens, usage.CachedTokens)
	}

	p, ok := telemetry.GetPricingForModel(model, pricing)
	if !ok {
		return 0, nil
	}

	billedInput := int64(usage.InputTokens) - int64(usage.CachedTokens)
	if billedInput < 0 {
		billedInput = 0
	}
	costInput := float64(billedInput) * (p.Input / 1_000_000.0)
	costOutput := float64(usage.OutputTokens) * (p.Output / 1_000_000.0)
	costCached := float64(usage.CachedTokens) * (p.Cached / 1_000_000.0)
	return costInput + costOutput + costCached, nil
}

func handleCalculateTokensCost(defaultPricing map[string]telemetry.ModelPricing) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			ModelUsages      map[string]telemetry.ModelUsage   `json:"model_usages"`
			DailyQuotaCap    *int64                            `json:"daily_quota_cap,omitempty"`
			PricingOverrides map[string]telemetry.ModelPricing `json:"pricing_overrides,omitempty"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		// Resolve pricing map
		pricing := defaultPricing
		if args.PricingOverrides != nil {
			pricing = args.PricingOverrides
		}

		var totalInput, totalOutput, totalCached int64
		var estimatedCost float64

		for model, usage := range args.ModelUsages {
			cost, err := calculateModelCost(model, usage, pricing)
			if err != nil {
				return nil, err
			}
			estimatedCost += cost
			totalInput += int64(usage.InputTokens)
			totalOutput += int64(usage.OutputTokens)
			totalCached += int64(usage.CachedTokens)
		}

		capVal := int64(1000000)
		if args.DailyQuotaCap != nil {
			capVal = *args.DailyQuotaCap
		}

		resp := struct {
			InputTokens   int64   `json:"input_tokens"`
			OutputTokens  int64   `json:"output_tokens"`
			CachedTokens  int64   `json:"cached_tokens"`
			EstimatedCost float64 `json:"estimated_cost"`
			DailyQuotaCap int64   `json:"daily_quota_cap"`
		}{
			InputTokens:   totalInput,
			OutputTokens:  totalOutput,
			CachedTokens:  totalCached,
			EstimatedCost: estimatedCost,
			DailyQuotaCap: capVal,
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(respBytes)}},
		}, nil
	}
}

func handleGetModelPricing(defaultPricing map[string]telemetry.ModelPricing) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Model string `json:"model,omitempty"`
		}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
		}

		if args.Model != "" {
			p, ok := telemetry.GetPricingForModel(args.Model, defaultPricing)
			if !ok {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("no pricing found for model %q", args.Model)}},
				}, nil
			}
			respBytes, err := json.Marshal(p)
			if err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: string(respBytes)}},
			}, nil
		}

		respBytes, err := json.Marshal(defaultPricing)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(respBytes)}},
		}, nil
	}
}
