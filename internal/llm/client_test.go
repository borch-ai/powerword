package llm

import (
	"testing"

	"powerword/internal/config"
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
