package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/telemetry"
	"github.com/spf13/cobra"
)

// Fallback pricing config for common models in case config.Active.Pricing is empty.
var fallbackPricing = map[string]telemetry.ModelPricing{
	"gemini-1.5-pro":    {Input: 1.25, Output: 3.75, Cached: 0.3125},
	"gemini-1.5-flash":  {Input: 0.075, Output: 0.30, Cached: 0.01875},
	"gemini-2.5-flash":  {Input: 0.075, Output: 0.30, Cached: 0.01875},
	"gpt-4o":            {Input: 5.00, Output: 15.00},
	"gpt-4o-mini":       {Input: 0.150, Output: 0.600},
	"claude-3-5-sonnet": {Input: 3.00, Output: 15.00},
}

func newAuditCmd() *cobra.Command {
	var filePath string
	var limit float64
	var strict bool
	var format string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit token usage and estimated cost from a telemetry file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Check file existence
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				cmd.Printf("No telemetry file found at %s. Skipping budget audit.\n", filePath)
				return nil
			}

			// 2. Read and parse telemetry JSON
			//nolint:gosec // G304: telemetry path is a user-configured path, input is trusted or local
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read telemetry file: %w", err)
			}

			var tracker telemetry.UsageTracker
			if err := json.Unmarshal(data, &tracker); err != nil {
				return fmt.Errorf("failed to unmarshal telemetry JSON: %w", err)
			}

			// 3. Resolve pricing map
			pricing := fallbackPricing
			cfg := config.Active
			if cfg != nil && len(cfg.Pricing) > 0 {
				// Override with config pricing
				pricing = cfg.Pricing
			}

			// 4. Calculate cost
			cost := tracker.EstimatedCost(pricing)

			// 5. Determine status
			status := "✅ Within Budget"
			if cost > limit {
				status = "⚠️ Budget Exceeded"
			}

			// 6. Output format
			if format == "markdown" {
				renderMarkdownAudit(cmd, &tracker, pricing, limit, cost, status)
			} else {
				renderTextAudit(cmd, &tracker, pricing, limit, status)
			}

			// 7. Exit code if strict and exceeded
			if strict && cost > limit {
				return fmt.Errorf("estimated cost of $%.5f exceeds limit of $%.2f", cost, limit)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&filePath, "file", ".agents/telemetry.json", "Path to telemetry JSON file")
	cmd.Flags().Float64Var(&limit, "limit", 2.0, "Financial limit/budget in USD")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail command if budget is exceeded")
	cmd.Flags().StringVar(&format, "format", "markdown", "Output format (markdown or text)")

	return cmd
}

func getModelPricing(modelName string, pricing map[string]telemetry.ModelPricing) *telemetry.ModelPricing {
	if p, ok := pricing[modelName]; ok {
		return &p
	}
	// Simple prefix matching
	var bestPrefix string
	for prefix := range pricing {
		if len(prefix) > 0 && len(modelName) >= len(prefix) && modelName[:len(prefix)] == prefix {
			if len(prefix) > len(bestPrefix) {
				bestPrefix = prefix
			}
		}
	}
	if bestPrefix != "" {
		p := pricing[bestPrefix]
		return &p
	}
	return nil
}

func calculateModelCost(usage *telemetry.ModelUsage, mPricing *telemetry.ModelPricing) float64 {
	if mPricing == nil || usage == nil {
		return 0.0
	}
	billedInput := usage.InputTokens - usage.CachedTokens
	if billedInput < 0 {
		billedInput = 0
	}
	return float64(billedInput)*(mPricing.Input/1_000_000.0) +
		float64(usage.OutputTokens)*(mPricing.Output/1_000_000.0) +
		float64(usage.CachedTokens)*(mPricing.Cached/1_000_000.0)
}

func renderMarkdownAudit(cmd *cobra.Command, tracker *telemetry.UsageTracker, pricing map[string]telemetry.ModelPricing, limit float64, cost float64, status string) {
	cmd.Println("<!-- powerword-budget-auditor-marker -->")
	cmd.Println("### ⚡ Powerword Token & Budget Audit")
	cmd.Println()
	cmd.Println("| Model | Input Tokens | Output Tokens | Cached Tokens | Estimated Cost |")
	cmd.Println("| :--- | :---: | :---: | :---: | :---: |")

	// Sort model names for deterministic output
	var modelNames []string
	for k := range tracker.ModelUsages {
		modelNames = append(modelNames, k)
	}
	sort.Strings(modelNames)

	var totalInput, totalOutput, totalCached int
	for _, modelName := range modelNames {
		usage := tracker.ModelUsages[modelName]
		mPricing := getModelPricing(modelName, pricing)
		mCost := calculateModelCost(usage, mPricing)

		cmd.Printf("| `%s` | %d | %d | %d | $%.5f |\n", modelName, usage.InputTokens, usage.OutputTokens, usage.CachedTokens, mCost)
		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		totalCached += usage.CachedTokens
	}
	cmd.Println("| --- | --- | --- | --- | --- |")
	cmd.Printf("| **Total** | **%d** | **%d** | **%d** | **$%.5f** |\n", totalInput, totalOutput, totalCached, cost)
	cmd.Println()
	cmd.Printf("- **Turns:** %d\n", tracker.Turns)
	cmd.Printf("- **Cost Limit:** $%.2f\n", limit)
	cmd.Printf("- **Status:** %s\n", status)
}

func renderTextAudit(cmd *cobra.Command, tracker *telemetry.UsageTracker, pricing map[string]telemetry.ModelPricing, limit float64, status string) {
	cmd.Print(tracker.FormatSummary(pricing))
	cmd.Printf("Cost Limit: $%.2f\n", limit)
	cmd.Printf("Status: %s\n", status)
}
