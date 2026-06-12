package telemetry

import (
	"math"
	"strings"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-5
}

func TestUsageTracker_RecordUsage(t *testing.T) {
	tracker := NewUsageTracker()

	// Empty usage should still increment turn but not add model usage
	tracker.RecordUsage("test-model", TokenUsage{})
	if len(tracker.ModelUsages) != 0 {
		t.Errorf("Expected 0 model usages, got %d", len(tracker.ModelUsages))
	}
	if tracker.Turns != 1 {
		t.Errorf("Expected 1 turn, got %d", tracker.Turns)
	}

	tracker.RecordUsage("test-model", TokenUsage{
		InputTokens:  10,
		OutputTokens: 20,
		CachedTokens: 5,
	})

	if len(tracker.ModelUsages) != 1 {
		t.Errorf("Expected 1 model usage, got %d", len(tracker.ModelUsages))
	}
	if tracker.Turns != 2 {
		t.Errorf("Expected 2 turns, got %d", tracker.Turns)
	}

	mu := tracker.ModelUsages["test-model"]
	if mu.InputTokens != 10 || mu.OutputTokens != 20 || mu.CachedTokens != 5 {
		t.Errorf("Incorrect usage values: %+v", mu)
	}

	// Add more usage to the same model
	tracker.RecordUsage("test-model", TokenUsage{
		InputTokens: 5,
	})
	if mu.InputTokens != 15 {
		t.Errorf("Expected 15 input tokens, got %d", mu.InputTokens)
	}
}

func TestUsageTracker_EstimatedCost(t *testing.T) {
	tracker := NewUsageTracker()

	// Test nil/empty pricing config
	if cost := tracker.EstimatedCost(nil); cost != 0 {
		t.Errorf("Expected cost 0 for nil pricing, got %f", cost)
	}

	pricing := map[string]ModelPricing{
		"test-model": {Input: 1.0, Output: 2.0, Cached: 0.5},
		"prefix-":    {Input: 10.0, Output: 20.0, Cached: 5.0},
	}

	// Empty tracker
	if cost := tracker.EstimatedCost(pricing); cost != 0 {
		t.Errorf("Expected cost 0 for empty tracker, got %f", cost)
	}

	// Exact match
	tracker.RecordUsage("test-model", TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		CachedTokens: 1_000_000,
	})
	// Expected cost: 0.0 + 2.0 + 0.5 = 2.5
	if cost := tracker.EstimatedCost(pricing); !almostEqual(cost, 2.5) {
		t.Errorf("Expected cost 2.5, got %f", cost)
	}

	// Prefix match
	tracker.RecordUsage("prefix-abc", TokenUsage{
		InputTokens:  500_000,
		OutputTokens: 500_000,
		CachedTokens: 0,
	})
	// Expected cost: 2.5 + (5.0 + 10.0) = 17.5
	if cost := tracker.EstimatedCost(pricing); !almostEqual(cost, 17.5) {
		t.Errorf("Expected cost 17.5, got %f", cost)
	}

	// No match
	tracker.RecordUsage("unknown-model", TokenUsage{
		InputTokens: 1_000_000,
	})
	// Expected cost: 17.5
	if cost := tracker.EstimatedCost(pricing); !almostEqual(cost, 17.5) {
		t.Errorf("Expected cost 17.5, got %f", cost)
	}
}

func TestUsageTracker_EstimatedCostPrefixLength(t *testing.T) {
	tracker := NewUsageTracker()

	pricing := map[string]ModelPricing{
		"gpt":   {Input: 1.0, Output: 2.0},
		"gpt-4": {Input: 10.0, Output: 20.0},
	}

	// Should match gpt-4 because it's longer
	tracker.RecordUsage("gpt-4-turbo", TokenUsage{
		InputTokens: 1_000_000,
	})

	if cost := tracker.EstimatedCost(pricing); !almostEqual(cost, 10.0) {
		t.Errorf("Expected cost 10.0, got %f", cost)
	}
}

func TestUsageTracker_FormatSummary(t *testing.T) {
	tracker := NewUsageTracker()

	pricing := map[string]ModelPricing{
		"test-model": {Input: 1.0, Output: 2.0, Cached: 0.5},
	}

	tracker.RecordUsage("test-model", TokenUsage{
		InputTokens:  10,
		OutputTokens: 20,
		CachedTokens: 5,
	})

	summary := tracker.FormatSummary(pricing)
	if !strings.Contains(summary, "Total Tokens: 30 (10 In, 20 Out)") {
		t.Errorf("Summary missing total tokens: %s", summary)
	}
	if !strings.Contains(summary, "Cached Tokens: 5") {
		t.Errorf("Summary missing cached tokens: %s", summary)
	}
	if !strings.Contains(summary, "Turns: 1") {
		t.Errorf("Summary missing turns: %s", summary)
	}
	if !strings.Contains(summary, "Estimated Cost: $0.00005") {
		t.Errorf("Summary missing estimated cost: %s", summary)
	}

	// Test formatting with no cost but pricing configured (should show $0.00000)
	tracker2 := NewUsageTracker()
	tracker2.RecordUsage("unknown-model", TokenUsage{
		InputTokens: 10,
	})
	summary2 := tracker2.FormatSummary(pricing)
	if !strings.Contains(summary2, "$0.00000") {
		t.Errorf("Summary missing 0 cost format: %s", summary2)
	}

	// Test formatting with no cost and no pricing configured
	tracker3 := NewUsageTracker()
	tracker3.RecordUsage("test-model", TokenUsage{
		InputTokens: 10,
	})
	summary3 := tracker3.FormatSummary(nil)
	if strings.Contains(summary3, "Estimated Cost") {
		t.Errorf("Summary should not contain estimated cost: %s", summary3)
	}
}

func TestUsageTracker_Totals(t *testing.T) {
	tracker := NewUsageTracker()
	tracker.RecordUsage("model1", TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
		CachedTokens: 20,
	})
	tracker.RecordUsage("model2", TokenUsage{
		InputTokens:  200,
		OutputTokens: 150,
		CachedTokens: 30,
	})

	if got := tracker.TotalTokens(); got != 500 {
		t.Errorf("TotalTokens() = %d; want 500", got)
	}
	if got := tracker.TotalInputTokens(); got != 300 {
		t.Errorf("TotalInputTokens() = %d; want 300", got)
	}
	if got := tracker.TotalOutputTokens(); got != 200 {
		t.Errorf("TotalOutputTokens() = %d; want 200", got)
	}
	if got := tracker.TotalCachedTokens(); got != 50 {
		t.Errorf("TotalCachedTokens() = %d; want 50", got)
	}
}
