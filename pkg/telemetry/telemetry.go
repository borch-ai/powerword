package telemetry

import (
	"fmt"
	"strings"
)

// ModelPricing specifies the cost per 1M tokens.
type ModelPricing struct {
	Input  float64 `mapstructure:"input"`
	Output float64 `mapstructure:"output"`
	Cached float64 `mapstructure:"cached"`
}

// TokenUsage represents the token usage for a single request.
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CachedTokens int `json:"cached_tokens"`
}

// ModelUsage stores token counts for a specific model.
type ModelUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CachedTokens int `json:"cached_tokens"`
}

// UsageTracker tracks the token usage for a session across multiple loops and models.
type UsageTracker struct {
	ModelUsages map[string]*ModelUsage `json:"model_usages"`
	Turns       int                    `json:"turns"`
}

// NewUsageTracker initializes a new UsageTracker.
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{
		ModelUsages: make(map[string]*ModelUsage),
	}
}

// RecordUsage records token usage for a specific model.
func (u *UsageTracker) RecordUsage(model string, usage TokenUsage) {
	u.Turns++
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.CachedTokens == 0 {
		return
	}
	if u.ModelUsages[model] == nil {
		u.ModelUsages[model] = &ModelUsage{}
	}
	u.ModelUsages[model].InputTokens += usage.InputTokens
	u.ModelUsages[model].OutputTokens += usage.OutputTokens
	u.ModelUsages[model].CachedTokens += usage.CachedTokens
}

// EstimatedCost calculates the total estimated cost across all used models.
func (u *UsageTracker) EstimatedCost(pricing map[string]ModelPricing) float64 {
	if len(pricing) == 0 {
		return 0
	}

	var totalCost float64
	for model, usage := range u.ModelUsages {
		p := getPricingForModel(model, pricing)
		if p == nil {
			continue
		}

		billedInput := usage.InputTokens - usage.CachedTokens
		if billedInput < 0 {
			billedInput = 0
		}
		costInput := float64(billedInput) * (p.Input / 1_000_000.0)
		costOutput := float64(usage.OutputTokens) * (p.Output / 1_000_000.0)
		costCached := float64(usage.CachedTokens) * (p.Cached / 1_000_000.0)
		totalCost += costInput + costOutput + costCached
	}

	return totalCost
}

func getPricingForModel(model string, pricing map[string]ModelPricing) *ModelPricing {
	if p, ok := pricing[model]; ok {
		return &p
	}
	var bestPrefix string
	for prefix := range pricing {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
		}
	}
	if bestPrefix != "" {
		p := pricing[bestPrefix]
		return &p
	}
	return nil
}

// FormatSummary returns a formatted string detailing token usage and estimated cost.
func (u *UsageTracker) FormatSummary(pricing map[string]ModelPricing) string {
	cost := u.EstimatedCost(pricing)

	var totalInput, totalOutput, totalCached int
	for _, usage := range u.ModelUsages {
		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		totalCached += usage.CachedTokens
	}
	total := totalInput + totalOutput

	var sb strings.Builder
	sb.WriteString("Session Metrics:\n")
	fmt.Fprintf(&sb, "- Total Tokens: %d (%d In, %d Out)\n", total, totalInput, totalOutput)
	if totalCached > 0 {
		fmt.Fprintf(&sb, "- Cached Tokens: %d\n", totalCached)
	}

	var hasMissingPricing bool
	for model := range u.ModelUsages {
		if getPricingForModel(model, pricing) == nil {
			hasMissingPricing = true
			break
		}
	}

	sb.WriteString(formatCostSummary(cost, hasMissingPricing, len(pricing), total))
	fmt.Fprintf(&sb, "- Turns: %d\n", u.Turns)

	return sb.String()
}

func formatCostSummary(cost float64, hasMissingPricing bool, lenPricing int, total int) string {
	if total == 0 {
		return ""
	}
	if cost > 0 {
		if hasMissingPricing {
			return fmt.Sprintf("- Estimated Cost: $%.5f (Incomplete, missing pricing for some models)\n", cost)
		}
		return fmt.Sprintf("- Estimated Cost: $%.5f\n", cost)
	}
	if hasMissingPricing {
		return "- Estimated Cost: N/A (Incomplete, missing pricing for some models)\n"
	}
	return "- Estimated Cost: $0.00000 (Check pricing config)\n"
}

// TotalTokens returns the total input and output tokens.
func (u *UsageTracker) TotalTokens() int {
	var total int
	for _, usage := range u.ModelUsages {
		total += usage.InputTokens + usage.OutputTokens
	}
	return total
}

// TotalInputTokens returns the sum of input tokens across all models.
func (u *UsageTracker) TotalInputTokens() int {
	var total int
	for _, usage := range u.ModelUsages {
		total += usage.InputTokens
	}
	return total
}

// TotalOutputTokens returns the sum of output tokens across all models.
func (u *UsageTracker) TotalOutputTokens() int {
	var total int
	for _, usage := range u.ModelUsages {
		total += usage.OutputTokens
	}
	return total
}

// TotalCachedTokens returns the sum of cached tokens across all models.
func (u *UsageTracker) TotalCachedTokens() int {
	var total int
	for _, usage := range u.ModelUsages {
		total += usage.CachedTokens
	}
	return total
}
