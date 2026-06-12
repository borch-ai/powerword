package loop

import (
	"context"
	"errors"
	"io"
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

func TestRunLoop_MaxTokensExact(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Hello",
				Usage:   &llm.TokenUsage{InputTokens: 100, OutputTokens: 50},
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
		MaxTokens:         150, // exactly equal to usage
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

func TestRunReActLoop_GenerateErrorPreservesHistory(t *testing.T) {
	mockClient := &mockLLMClient{
		genErr: errors.New("generation failed"),
	}
	ctx := context.Background()
	cfg := &config.Config{
		MaxLoopIterations: 3,
	}
	tracker := telemetry.NewUsageTracker()
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Initial user prompt"},
	}

	formatter := NewTerminalFormatter(io.Discard, 80)

	updated, err := runReActLoop(ctx, cfg, mockClient, nil, formatter, messages, tracker, "test-model")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(updated) != 1 {
		t.Errorf("expected updated messages to have length 1, got %d", len(updated))
	}
	if updated[0].Content != "Initial user prompt" {
		t.Errorf("expected original content to be preserved, got %q", updated[0].Content)
	}
}

type mockLLMClientMulti struct {
	calls  int
	genErr error
}

func (m *mockLLMClientMulti) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, opts ...llm.GenerateOption) (*llm.Message, error) {
	m.calls++
	if m.calls == 1 {
		return &llm.Message{
			Content: "First turn response",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call-1",
					Name: "test_tool",
				},
			},
		}, nil
	}
	return nil, m.genErr
}

func (m *mockLLMClientMulti) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockLLMClientMulti) ListModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

func TestRunReActLoop_GenerateErrorPreservesHistory_MultipleTurns(t *testing.T) {
	mockClient := &mockLLMClientMulti{
		genErr: errors.New("second turn generation failed"),
	}
	ctx := context.Background()
	cfg := &config.Config{
		MaxLoopIterations: 3,
	}
	tracker := telemetry.NewUsageTracker()
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "Initial user prompt"},
	}

	formatter := NewTerminalFormatter(io.Discard, 80)

	updated, err := runReActLoop(ctx, cfg, mockClient, nil, formatter, messages, tracker, "test-model")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Initial user prompt (1) + assistant tool call response (1) + tool response (1) = 3 messages total
	if len(updated) != 3 {
		t.Errorf("expected updated messages to have length 3, got %d", len(updated))
	}
	if updated[0].Content != "Initial user prompt" {
		t.Errorf("expected original content at index 0, got %q", updated[0].Content)
	}
	if updated[1].Content != "First turn response" {
		t.Errorf("expected first turn response at index 1, got %q", updated[1].Content)
	}
	if updated[2].Role != llm.RoleTool {
		t.Errorf("expected tool response at index 2, got role %s", updated[2].Role)
	}
}
