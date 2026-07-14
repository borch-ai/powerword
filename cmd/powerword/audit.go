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
			return runAuditCommand(cmd, filePath, limit, strict, format)
		},
	}

	cmd.Flags().StringVar(&filePath, "file", ".agents/telemetry.json", "Path to telemetry JSON file")
	cmd.Flags().Float64Var(&limit, "limit", 2.0, "Financial limit/budget in USD")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail command if budget is exceeded")
	cmd.Flags().StringVar(&format, "format", "markdown", "Output format (markdown or text)")

	return cmd
}

func runAuditCommand(cmd *cobra.Command, filePath string, limit float64, strict bool, format string) error {
	if format != "markdown" && format != "text" {
		return fmt.Errorf("unsupported output format %q; must be either \"markdown\" or \"text\"", format)
	}
	if filePath == "" {
		return fmt.Errorf("telemetry file path cannot be empty")
	}
	if limit < 0 {
		return fmt.Errorf("limit cannot be negative: %.2f", limit)
	}

	// 1. Check file existence and type
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			cmd.Printf("No telemetry file found at %s. Skipping budget audit.\n", filePath)
			return nil
		}
		return fmt.Errorf("failed to access telemetry file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("telemetry path is a directory: %s", filePath)
	}

	// 2. Read and parse telemetry JSON
	tracker, err := loadTelemetryFile(filePath)
	if err != nil {
		return err
	}

	// 3. Resolve pricing map
	pricing := getActivePricing()

	// 4. Calculate cost
	cost := tracker.EstimatedCost(pricing)

	// Check for missing pricing
	missingPricingModels := findMissingPricingModels(tracker, pricing)

	// 5. Determine status
	status := getBudgetStatus(cost, limit, missingPricingModels)

	// 6. Output format
	if format == "markdown" {
		renderMarkdownAudit(cmd, tracker, pricing, limit, cost, status, missingPricingModels)
	} else {
		renderTextAudit(cmd, tracker, pricing, limit, status)
	}

	// 7. Exit code if strict and exceeded or missing pricing
	if strict {
		if len(missingPricingModels) > 0 {
			return fmt.Errorf("budget audit failed: missing pricing for models: %v", missingPricingModels)
		}
		if cost > limit {
			return fmt.Errorf("estimated cost of $%.5f exceeds limit of $%.2f", cost, limit)
		}
	}

	return nil
}

//nolint:gosec // G304: telemetry path is a user-configured path, input is trusted or local
func loadTelemetryFile(filePath string) (*telemetry.UsageTracker, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read telemetry file: %w", err)
	}
	var tracker telemetry.UsageTracker
	if err := json.Unmarshal(data, &tracker); err != nil {
		return nil, fmt.Errorf("failed to unmarshal telemetry JSON: %w", err)
	}
	return &tracker, nil
}

func getActivePricing() map[string]telemetry.ModelPricing {
	pricing := fallbackPricing
	cfg := config.Active
	if cfg != nil && len(cfg.Pricing) > 0 {
		pricing = cfg.Pricing
	}
	return pricing
}

func findMissingPricingModels(tracker *telemetry.UsageTracker, pricing map[string]telemetry.ModelPricing) []string {
	var missing []string
	for modelName := range tracker.ModelUsages {
		if telemetry.GetPricingForModel(modelName, pricing) == nil {
			missing = append(missing, modelName)
		}
	}
	sort.Strings(missing)
	return missing
}

func getBudgetStatus(cost float64, limit float64, missingPricingModels []string) string {
	if len(missingPricingModels) > 0 {
		if cost > limit {
			return "⚠️ Budget Exceeded (and Incomplete, missing pricing for some models)"
		}
		return "⚠️ Missing Pricing (Budget Incomplete)"
	}
	if cost > limit {
		return "⚠️ Budget Exceeded"
	}
	return "✅ Within Budget"
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

func renderMarkdownAudit(cmd *cobra.Command, tracker *telemetry.UsageTracker, pricing map[string]telemetry.ModelPricing, limit float64, cost float64, status string, missingPricingModels []string) {
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
		mPricing := telemetry.GetPricingForModel(modelName, pricing)
		if mPricing == nil {
			cmd.Printf("| `%s` | %d | %d | %d | N/A |\n", modelName, usage.InputTokens, usage.OutputTokens, usage.CachedTokens)
		} else {
			mCost := calculateModelCost(usage, mPricing)
			cmd.Printf("| `%s` | %d | %d | %d | $%.5f |\n", modelName, usage.InputTokens, usage.OutputTokens, usage.CachedTokens, mCost)
		}
		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		totalCached += usage.CachedTokens
	}
	cmd.Println("| --- | --- | --- | --- | --- |")
	totalCostStr := fmt.Sprintf("$%.5f", cost)
	if len(missingPricingModels) > 0 {
		totalCostStr = fmt.Sprintf("$%.5f (Incomplete)", cost)
	}
	cmd.Printf("| **Total** | **%d** | **%d** | **%d** | **%s** |\n", totalInput, totalOutput, totalCached, totalCostStr)
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
