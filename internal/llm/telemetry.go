package llm

import (
	"fmt"
	"strings"

	"powerword/internal/config"
)

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
func (u *UsageTracker) EstimatedCost(cfg *config.Config) float64 {
	if cfg == nil || len(cfg.Pricing) == 0 {
		return 0
	}

	var totalCost float64
	for model, usage := range u.ModelUsages {
		var pricing *config.ModelPricing
		if p, ok := cfg.Pricing[model]; ok {
			pricing = &p
		} else {
			// Find the best prefix match
			var bestPrefix string
			for prefix := range cfg.Pricing {
				if strings.HasPrefix(model, prefix) {
					if len(prefix) > len(bestPrefix) {
						bestPrefix = prefix
					}
				}
			}
			if bestPrefix != "" {
				p := cfg.Pricing[bestPrefix]
				pricing = &p
			}
		}

		if pricing == nil {
			continue
		}

		costInput := float64(usage.InputTokens) * (pricing.Input / 1_000_000.0)
		costOutput := float64(usage.OutputTokens) * (pricing.Output / 1_000_000.0)
		costCached := float64(usage.CachedTokens) * (pricing.Cached / 1_000_000.0)
		totalCost += costInput + costOutput + costCached
	}

	return totalCost
}

// FormatSummary returns a formatted string detailing token usage and estimated cost.
func (u *UsageTracker) FormatSummary(cfg *config.Config) string {
	cost := u.EstimatedCost(cfg)

	var totalInput, totalOutput, totalCached int
	for _, usage := range u.ModelUsages {
		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
		totalCached += usage.CachedTokens
	}
	total := totalInput + totalOutput

	var sb strings.Builder
	sb.WriteString("Session Metrics:\n")
	sb.WriteString(fmt.Sprintf("- Total Tokens: %d (%d In, %d Out", total, totalInput, totalOutput))
	if totalCached > 0 {
		sb.WriteString(fmt.Sprintf(", %d Cached", totalCached))
	}
	sb.WriteString(")\n")

	if cost > 0 {
		sb.WriteString(fmt.Sprintf("- Estimated Cost: $%.5f\n", cost))
	} else if cfg != nil && len(cfg.Pricing) > 0 && total > 0 {
		sb.WriteString("- Estimated Cost: $0.00000 (Check pricing config)\n")
	}
	sb.WriteString(fmt.Sprintf("- Turns: %d\n", u.Turns))

	return sb.String()
}
