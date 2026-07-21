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

	// Exact match with CachedTokens == InputTokens
	tracker.RecordUsage("test-model", TokenUsage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		CachedTokens: 1_000_000,
	})
	// Expected cost: 0.0 + 2.0 + 0.5 = 2.5
	if cost := tracker.EstimatedCost(pricing); !almostEqual(cost, 2.5) {
		t.Errorf("Expected cost 2.5, got %f", cost)
	}

	// Case where CachedTokens > InputTokens (triggers billedInput = 0 branch)
	{
		clampedTracker := NewUsageTracker()
		clampedTracker.RecordUsage("test-model", TokenUsage{
			InputTokens:  1_000_000,
			OutputTokens: 1_000_000,
			CachedTokens: 2_000_000,
		})
		// Expected cost: 0.0 (clamped to 0) + 2.0 (output) + 1.0 (cached) = 3.0
		if cost := clampedTracker.EstimatedCost(pricing); !almostEqual(cost, 3.0) {
			t.Errorf("Expected cost 3.0 for CachedTokens > InputTokens, got %f", cost)
		}
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

	// Test formatting with missing pricing (should show N/A)
	tracker2 := NewUsageTracker()
	tracker2.RecordUsage("unknown-model", TokenUsage{
		InputTokens: 10,
	})
	summary2 := tracker2.FormatSummary(pricing)
	if !strings.Contains(summary2, "N/A (Incomplete, missing pricing for some models)") {
		t.Errorf("Summary missing N/A cost format: %s", summary2)
	}

	// Test formatting with mixed models (some missing pricing)
	trackerMixed := NewUsageTracker()
	trackerMixed.RecordUsage("test-model", TokenUsage{
		InputTokens: 10,
	})
	trackerMixed.RecordUsage("unknown-model", TokenUsage{
		InputTokens: 10,
	})
	summaryMixed := trackerMixed.FormatSummary(pricing)
	if !strings.Contains(summaryMixed, "Incomplete, missing pricing for some models") {
		t.Errorf("Summary missing incomplete cost format: %s", summaryMixed)
	}

	// Test formatting with no cost and no pricing configured (pricing map nil)
	tracker3 := NewUsageTracker()
	tracker3.RecordUsage("test-model", TokenUsage{
		InputTokens: 10,
	})
	summary3 := tracker3.FormatSummary(nil)
	if !strings.Contains(summary3, "Estimated Cost: N/A") || strings.Contains(summary3, "Incomplete") {
		t.Errorf("Summary should contain N/A but not Incomplete: %s", summary3)
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

func TestUsageTracker_NilModelUsage(t *testing.T) {
	tracker := NewUsageTracker()
	// Manually insert a nil entry into the map to trigger nil-pointer checks
	tracker.ModelUsages["nil-model"] = nil

	pricing := map[string]ModelPricing{
		"nil-model": {Input: 1.0, Output: 2.0},
	}

	// 1. EstimatedCost
	if cost := tracker.EstimatedCost(pricing); cost != 0 {
		t.Errorf("Expected 0 cost for nil model usage, got %f", cost)
	}

	// 2. FormatSummary
	summary := tracker.FormatSummary(pricing)
	if !strings.Contains(summary, "Total Tokens: 0") {
		t.Errorf("Expected 0 total tokens in summary, got: %s", summary)
	}

	// 3. Totals
	if got := tracker.TotalTokens(); got != 0 {
		t.Errorf("TotalTokens() = %d; want 0", got)
	}
	if got := tracker.TotalInputTokens(); got != 0 {
		t.Errorf("TotalInputTokens() = %d; want 0", got)
	}
	if got := tracker.TotalOutputTokens(); got != 0 {
		t.Errorf("TotalOutputTokens() = %d; want 0", got)
	}
	if got := tracker.TotalCachedTokens(); got != 0 {
		t.Errorf("TotalCachedTokens() = %d; want 0", got)
	}
}
