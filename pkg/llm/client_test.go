package llm

import (
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "gemini config success",
			cfg: &config.Config{
				Model: "gemini-1.5-pro",
				APIKeys: config.APIKeys{
					Gemini: "key",
				},
			},
			wantErr: false,
		},
		{
			name: "gemini config missing key",
			cfg: &config.Config{
				Model: "gemini-1.5-pro",
			},
			wantErr: true,
		},
		{
			name: "anthropic config success",
			cfg: &config.Config{
				Model: "claude-3-5-sonnet",
				APIKeys: config.APIKeys{
					Anthropic: "key",
				},
			},
			wantErr: false,
		},
		{
			name: "anthropic config missing key",
			cfg: &config.Config{
				Model: "claude-3-5-sonnet",
			},
			wantErr: true,
		},
		{
			name: "openai config success",
			cfg: &config.Config{
				Model: "gpt-4",
				APIKeys: config.APIKeys{
					OpenAI: "key",
				},
			},
			wantErr: false,
		},
		{
			name: "openai config missing key",
			cfg: &config.Config{
				Model: "gpt-4",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewClient(tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("expected error: %v, got error: %v", tc.wantErr, err)
			}
		})
	}
}

func TestNewCriticClient_Success(t *testing.T) {
	cfg := &config.Config{
		Model: "openai-test",
		APIKeys: config.APIKeys{
			OpenAI: "key",
		},
	}

	client, err := NewCriticClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatalf("expected client, got nil")
	}

	cfg.CriticProvider = "ollama"
	cfg.CriticModel = "llama3"
	cfg.CriticEndpoint = "http://localhost"
	client, err = NewCriticClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatalf("expected client, got nil")
	}

	cfg.CriticProvider = "gemini"
	cfg.CriticModel = "gemini-1.5-pro"
	cfg.APIKeys.Gemini = "key"
	_, err = NewCriticClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg.CriticProvider = "claude"
	cfg.CriticModel = "claude-3-5-sonnet"
	cfg.APIKeys.Anthropic = "key"
	_, err = NewCriticClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg.CriticProvider = "openai"
	cfg.CriticModel = ""
	cfg.APIKeys.OpenAI = "key"
	client, err = NewCriticClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatalf("expected client, got nil")
	}

	cfg.CriticProvider = "anthropic"
	cfg.APIKeys.Anthropic = "key"
	client, err = NewCriticClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatalf("expected client, got nil")
	}
}

func TestNewCriticClient_Failure(t *testing.T) {
	_, err := NewCriticClient(nil)
	if err == nil {
		t.Error("expected error for nil config, got nil")
	}

	cfg := &config.Config{
		Model: "openai-test",
		APIKeys: config.APIKeys{
			OpenAI: "key",
		},
	}

	cfg.CriticProvider = "gemini"
	cfg.APIKeys.Gemini = ""
	_, err = NewCriticClient(cfg)
	if err == nil {
		t.Error("expected error for missing gemini key, got nil")
	}

	cfg.CriticProvider = "claude"
	cfg.APIKeys.Anthropic = ""
	_, err = NewCriticClient(cfg)
	if err == nil {
		t.Error("expected error for missing anthropic key, got nil")
	}

	cfg.CriticProvider = "unsupported"
	_, err = NewCriticClient(cfg)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}
