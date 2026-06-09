package loop

import (
	"context"
	"errors"
	"testing"

	"powerword/internal/config"
	"powerword/internal/llm"
)

func TestRunLoop_ClassifierCreationError(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		if cfg.Model == "classifier-model" {
			return nil, errors.New("classifier creation error")
		}
		return &mockLLMClient{
			genResps: []*llm.Message{{Content: "default"}},
		}, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model:           "default-model",
		ClassifierModel: "classifier-model",
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRunLoop_RoutingAndClientSwap(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		if cfg.Model == "target-model" {
			return &mockLLMClient{
				genResps: []*llm.Message{{Content: "target response"}},
			}, nil
		}
		return &mockLLMClient{
			genResps: []*llm.Message{{Content: "default response"}},
		}, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model:   "default-model",
		Route:   map[string]string{"target": "target-model"},
		Verbose: true,
	}

	err := RunLoop(ctx, cfg, "test target")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRunLoop_TargetClientCreationErrorFallback(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		if cfg.Model == "bad-target-model" {
			return nil, errors.New("creation error")
		}
		return &mockLLMClient{
			genResps: []*llm.Message{{Content: "default response"}},
		}, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model: "default-model",
		Route: map[string]string{"target": "bad-target-model"},
	}

	err := RunLoop(ctx, cfg, "test target")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRunLoop_ClassifierSuccess(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		if cfg.Model == "classifier-model" {
			return &mockLLMClient{
				genResps: []*llm.Message{{Content: "classified-model"}},
			}, nil
		}
		if cfg.Model == "classified-model" {
			return &mockLLMClient{
				genResps: []*llm.Message{{Content: "classified response"}},
			}, nil
		}
		return &mockLLMClient{
			genResps: []*llm.Message{{Content: "default response"}},
		}, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Model:           "default-model",
		ClassifierModel: "classifier-model",
	}

	err := RunLoop(ctx, cfg, "test classifier")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
