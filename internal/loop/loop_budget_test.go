package loop

import (
	"context"
	"errors"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/llm"
	"github.com/borch-ai/powerword/pkg/telemetry"
)

func TestRunLoop_MaxCostExceeded(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Hello",
				Usage:   &llm.TokenUsage{InputTokens: 500000, OutputTokens: 500000},
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model:             "test-model",
		MaxLoopIterations: 3,
		MaxCost:           2.0, // $2.00 limit
		Pricing: map[string]telemetry.ModelPricing{
			"test-model": {Input: 5.0, Output: 5.0}, // $5.00/1M tokens
		},
	}

	// First turn cost: 0.5M * $5 + 0.5M * $5 = $5.00. This exceeds $2.00.
	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected budget exceeded error, got nil")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got: %T (%v)", err, err)
	}

	if !testing.Short() {
		t.Logf("Got expected error: %v", err)
	}
}

func TestRunLoop_MaxTokensExceeded(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Hello",
				Usage:   &llm.TokenUsage{InputTokens: 100, OutputTokens: 100},
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model:             "test-model",
		MaxLoopIterations: 3,
		MaxTokens:         150, // 150 total tokens limit
	}

	// 200 total tokens exceeds 150
	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected budget exceeded error, got nil")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got: %T (%v)", err, err)
	}
}

func TestRunLoop_MaxInputTokensExceeded(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Hello",
				Usage:   &llm.TokenUsage{InputTokens: 100, OutputTokens: 10},
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model:             "test-model",
		MaxLoopIterations: 3,
		MaxInputTokens:    50, // 50 input tokens limit
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected budget exceeded error, got nil")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got: %T (%v)", err, err)
	}
}

func TestRunLoop_MaxOutputTokensExceeded(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Hello",
				Usage:   &llm.TokenUsage{InputTokens: 10, OutputTokens: 100},
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model:             "test-model",
		MaxLoopIterations: 3,
		MaxOutputTokens:   50, // 50 output tokens limit
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected budget exceeded error, got nil")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got: %T (%v)", err, err)
	}
}

func TestRunLoop_MaxCachedTokensExceeded(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Hello",
				Usage:   &llm.TokenUsage{InputTokens: 100, OutputTokens: 10, CachedTokens: 80},
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model:             "test-model",
		MaxLoopIterations: 3,
		MaxCachedTokens:   50, // 50 cached tokens limit
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected budget exceeded error, got nil")
	}

	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got: %T (%v)", err, err)
	}
}
