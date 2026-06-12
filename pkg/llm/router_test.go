package llm

import (
	"context"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

type mockClassifierClient struct {
	response string
}

func (m *mockClassifierClient) Generate(ctx context.Context, messages []Message, tools []ToolDefinition, opts ...GenerateOption) (*Message, error) {
	return &Message{
		Role:    RoleAssistant,
		Content: m.response,
	}, nil
}

func (m *mockClassifierClient) Stream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	return nil, nil
}

func (m *mockClassifierClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

var routerTestCases = []struct {
	name           string
	cfg            *config.Config
	prompt         string
	classifierResp string
	expectedModel  string
	expectedPrompt string
}{
	{
		name: "default model",
		cfg: &config.Config{
			Model: "default-model",
		},
		prompt:         "hello",
		expectedModel:  "default-model",
		expectedPrompt: "hello",
	},
	{
		name: "explicit prefix override",
		cfg: &config.Config{
			Model: "default-model",
		},
		prompt:         "@fast-model list files",
		expectedModel:  "fast-model",
		expectedPrompt: "list files",
	},
	{
		name: "explicit prefix with no trailing prompt",
		cfg: &config.Config{
			Model: "default-model",
		},
		prompt:         "@fast-model",
		expectedModel:  "fast-model",
		expectedPrompt: "",
	},
	{
		name: "rule-based routing match",
		cfg: &config.Config{
			Model: "default-model",
			Route: map[string]string{
				"git.*": "git-model",
			},
		},
		prompt:         "git status",
		expectedModel:  "git-model",
		expectedPrompt: "git status",
	},
	{
		name: "rule-based routing no match",
		cfg: &config.Config{
			Model: "default-model",
			Route: map[string]string{
				"git.*": "git-model",
			},
		},
		prompt:         "ls -la",
		expectedModel:  "default-model",
		expectedPrompt: "ls -la",
	},
	{
		name: "classifier routing",
		cfg: &config.Config{
			Model:           "default-model",
			ClassifierModel: "classifier",
		},
		prompt:         "complex task",
		classifierResp: "smart-model",
		expectedModel:  "smart-model",
		expectedPrompt: "complex task",
	},
	{
		name: "classifier returns invalid string",
		cfg: &config.Config{
			Model:           "default-model",
			ClassifierModel: "classifier",
		},
		prompt:         "complex task",
		classifierResp: "I think you should use smart-model",
		expectedModel:  "default-model", // Should fallback because it has spaces
		expectedPrompt: "complex task",
	},
	{
		name: "rule-based routing invalid regex",
		cfg: &config.Config{
			Model: "default-model",
			Route: map[string]string{
				"[invalid": "git-model",
			},
		},
		prompt:         "git status",
		expectedModel:  "default-model",
		expectedPrompt: "git status",
	},
}

func TestRouter_Route(t *testing.T) {
	for _, tt := range routerTestCases {
		t.Run(tt.name, func(t *testing.T) {
			var client LLMClient
			if tt.cfg.ClassifierModel != "" {
				client = &mockClassifierClient{response: tt.classifierResp}
			}

			router := NewRouter(tt.cfg, client)
			model, prompt, err := router.Route(context.Background(), tt.prompt)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if model != tt.expectedModel {
				t.Errorf("expected model %q, got %q", tt.expectedModel, model)
			}

			if prompt != tt.expectedPrompt {
				t.Errorf("expected prompt %q, got %q", tt.expectedPrompt, prompt)
			}
		})
	}
}

func TestRouter_GetAvailableModelsStr(t *testing.T) {
	cfg := &config.Config{
		Model: "default-model",
		Route: map[string]string{
			"a": "model-1",
			"b": "model-1",
			"c": "default-model",
		},
	}
	router := NewRouter(cfg, nil)
	str := router.getAvailableModelsStr()
	if str == "" {
		t.Error("expected non-empty string")
	}
}
