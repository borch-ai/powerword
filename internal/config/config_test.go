package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	// Create a temporary directory for config
	tmpDir, err := os.MkdirTemp("", "pw-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tomlContent := `
verbose = true
model = "gemini-1.5-flash"

[api_keys]
gemini = "gemini-key-123"
openai = "openai-key-456"
`
	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// Load config
	cfg, err := LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig returned unexpected error: %v", err)
	}

	if !cfg.Verbose {
		t.Errorf("expected Verbose to be true, got false")
	}
	if cfg.Model != "gemini-1.5-flash" {
		t.Errorf("expected Model to be 'gemini-1.5-flash', got '%s'", cfg.Model)
	}
	if cfg.APIKeys.Gemini != "gemini-key-123" {
		t.Errorf("expected Gemini API key to be 'gemini-key-123', got '%s'", cfg.APIKeys.Gemini)
	}
	if cfg.APIKeys.OpenAI != "openai-key-456" {
		t.Errorf("expected OpenAI API key to be 'openai-key-456', got '%s'", cfg.APIKeys.OpenAI)
	}
	if cfg.APIKeys.Anthropic != "" {
		t.Errorf("expected Anthropic API key to be empty, got '%s'", cfg.APIKeys.Anthropic)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate returned unexpected error: %v", err)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pw-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tomlContent := `
verbose = false
model = "gemini-1.5-pro"

[api_keys]
gemini = "gemini-key-toml"
`
	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// Set env overrides
	_ = os.Setenv("POWERWORD_GEMINI_API_KEY", "gemini-key-env")
	_ = os.Setenv("POWERWORD_OPENAI_API_KEY", "openai-key-env")
	_ = os.Setenv("POWERWORD_ANTHROPIC_API_KEY", "anthropic-key-env")
	_ = os.Setenv("POWERWORD_MODEL", "openai-model-env")
	_ = os.Setenv("POWERWORD_VERBOSE", "true")

	defer func() {
		_ = os.Unsetenv("POWERWORD_GEMINI_API_KEY")
		_ = os.Unsetenv("POWERWORD_OPENAI_API_KEY")
		_ = os.Unsetenv("POWERWORD_ANTHROPIC_API_KEY")
		_ = os.Unsetenv("POWERWORD_MODEL")
		_ = os.Unsetenv("POWERWORD_VERBOSE")
	}()

	cfg, err := LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig returned unexpected error: %v", err)
	}

	if !cfg.Verbose {
		t.Errorf("expected Verbose overridden to true, got false")
	}
	if cfg.Model != "openai-model-env" {
		t.Errorf("expected Model overridden to 'openai-model-env', got '%s'", cfg.Model)
	}
	if cfg.APIKeys.Gemini != "gemini-key-env" {
		t.Errorf("expected Gemini API key overridden to 'gemini-key-env', got '%s'", cfg.APIKeys.Gemini)
	}
	if cfg.APIKeys.OpenAI != "openai-key-env" {
		t.Errorf("expected OpenAI API key overridden to 'openai-key-env', got '%s'", cfg.APIKeys.OpenAI)
	}
	if cfg.APIKeys.Anthropic != "anthropic-key-env" {
		t.Errorf("expected Anthropic API key overridden to 'anthropic-key-env', got '%s'", cfg.APIKeys.Anthropic)
	}
}

func TestLoadConfig_MissingCustomConfig(t *testing.T) {
	_, err := LoadConfig("non-existent-file.toml")
	if err == nil {
		t.Errorf("expected error for missing custom config file, got nil")
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pw-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	invalidContent := `
verbose = "not-a-bool"
`
	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	if errWrite := os.WriteFile(cfgFilePath, []byte(invalidContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	_, err = LoadConfig(cfgFilePath)
	if err == nil {
		t.Errorf("expected error for invalid TOML format, got nil")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid - Gemini key only",
			config: Config{
				APIKeys: APIKeys{Gemini: "key"},
			},
			wantErr: false,
		},
		{
			name: "valid - OpenAI key only",
			config: Config{
				APIKeys: APIKeys{OpenAI: "key"},
			},
			wantErr: false,
		},
		{
			name: "valid - Anthropic key only",
			config: Config{
				APIKeys: APIKeys{Anthropic: "key"},
			},
			wantErr: false,
		},
		{
			name: "invalid - no keys",
			config: Config{
				APIKeys: APIKeys{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfig_DefaultConfigNotFound(t *testing.T) {
	// Temporarily redirect DefaultConfigPath to a non-existent file
	origPath := DefaultConfigPath
	defer func() { DefaultConfigPath = origPath }()
	DefaultConfigPath = filepath.Join(t.TempDir(), "non-existent-dir", "config.toml")

	// Set API key via env so validation passes
	_ = os.Setenv("POWERWORD_GEMINI_API_KEY", "env-key")
	defer func() {
		_ = os.Unsetenv("POWERWORD_GEMINI_API_KEY")
	}()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("expected LoadConfig to succeed even if default config is missing, got: %v", err)
	}

	if cfg.APIKeys.Gemini != "env-key" {
		t.Errorf("expected Gemini API key to be 'env-key', got '%s'", cfg.APIKeys.Gemini)
	}
}

func TestLoadConfig_EmptyDefaultConfigPath(t *testing.T) {
	origPath := DefaultConfigPath
	defer func() { DefaultConfigPath = origPath }()
	DefaultConfigPath = ""

	_, err := LoadConfig("")
	if err == nil {
		t.Error("expected error when LoadConfig is called with empty cfgFile and empty DefaultConfigPath, got nil")
	}
}
