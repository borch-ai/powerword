package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfig_Success(t *testing.T) {
	tmpDir := t.TempDir()

	tomlContent := `
verbose = true
model = "gemini-1.5-flash"

[api_keys]
gemini = "gemini-key-123"
openai = "openai-key-456"

[servers.filesystem]
command = "node"
args = ["/path/to/server.js"]
env = ["FOO=BAR"]
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

	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server config, got %d", len(cfg.Servers))
	}
	srvCfg := cfg.Servers["filesystem"]
	if srvCfg.Command != "node" {
		t.Errorf("expected server command 'node', got '%s'", srvCfg.Command)
	}
	if len(srvCfg.Args) != 1 || srvCfg.Args[0] != "/path/to/server.js" {
		t.Errorf("expected server args ['/path/to/server.js'], got %v", srvCfg.Args)
	}
	if len(srvCfg.Env) != 1 || srvCfg.Env[0] != "FOO=BAR" {
		t.Errorf("expected server env ['FOO=BAR'], got %v", srvCfg.Env)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate returned unexpected error: %v", err)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()

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

	// Set env overrides using t.Setenv (which cleans up automatically)
	t.Setenv("POWERWORD_GEMINI_API_KEY", "gemini-key-env")
	t.Setenv("POWERWORD_OPENAI_API_KEY", "openai-key-env")
	t.Setenv("POWERWORD_ANTHROPIC_API_KEY", "anthropic-key-env")
	t.Setenv("POWERWORD_MODEL", "openai-model-env")
	t.Setenv("POWERWORD_VERBOSE", "true")

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
	tmpDir := t.TempDir()

	invalidContent := `
verbose = "not-a-bool"
`
	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	if errWrite := os.WriteFile(cfgFilePath, []byte(invalidContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	_, err := LoadConfig(cfgFilePath)
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
	t.Setenv("POWERWORD_GEMINI_API_KEY", "env-key")

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

func TestLoadConfig_DotEnv(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()

	if errChdir := os.Chdir(tmpDir); errChdir != nil {
		t.Fatalf("failed to change directory: %v", errChdir)
	}
	defer func() {
		_ = os.Chdir(origWd)
	}()

	envContent := `
POWERWORD_GEMINI_API_KEY=dotenv-gemini-key
POWERWORD_MODEL=dotenv-model
`
	if errWrite := os.WriteFile(".env", []byte(envContent), 0600); errWrite != nil {
		t.Fatalf("failed to write .env: %v", errWrite)
	}

	// Explicitly unset variables to verify they are loaded from the .env file
	_ = os.Unsetenv("POWERWORD_GEMINI_API_KEY")
	_ = os.Unsetenv("POWERWORD_MODEL")

	defer func() {
		_ = os.Unsetenv("POWERWORD_GEMINI_API_KEY")
		_ = os.Unsetenv("POWERWORD_MODEL")
	}()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig returned unexpected error: %v", err)
	}

	if cfg.APIKeys.Gemini != "dotenv-gemini-key" {
		t.Errorf("expected Gemini API key 'dotenv-gemini-key', got '%s'", cfg.APIKeys.Gemini)
	}
	if cfg.Model != "dotenv-model" {
		t.Errorf("expected Model 'dotenv-model', got '%s'", cfg.Model)
	}
}

func TestLoadConfig_DotEnvReadError(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()

	if errChdir := os.Chdir(tmpDir); errChdir != nil {
		t.Fatalf("failed to change directory: %v", errChdir)
	}
	defer func() {
		_ = os.Chdir(origWd)
	}()

	// Create .env as a directory to trigger a read error
	if errMkdir := os.Mkdir(".env", 0750); errMkdir != nil {
		t.Fatalf("failed to create .env directory: %v", errMkdir)
	}

	_, err = LoadConfig("")
	if err == nil {
		t.Errorf("expected error when .env is a directory, got nil")
	}
}

func TestLoadConfig_InvalidTOML_DefaultPath(t *testing.T) {
	origPath := DefaultConfigPath
	defer func() { DefaultConfigPath = origPath }()

	tmpDir := t.TempDir()
	invalidContent := `
verbose = "not-a-bool"
`
	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	if errWrite := os.WriteFile(cfgFilePath, []byte(invalidContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}
	DefaultConfigPath = cfgFilePath

	_, err := LoadConfig("")
	if err == nil {
		t.Errorf("expected error for invalid TOML format in default path, got nil")
	}
}

func TestLoadConfig_DotEnv_SafeOverride(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	tmpDir := t.TempDir()

	if errChdir := os.Chdir(tmpDir); errChdir != nil {
		t.Fatalf("failed to change directory: %v", errChdir)
	}
	defer func() {
		_ = os.Chdir(origWd)
	}()

	envContent := `
POWERWORD_GEMINI_API_KEY=dotenv-gemini-key
POWERWORD_MODEL=dotenv-model
SOME_OTHER_VAR=dotenv-other-key
`
	if errWrite := os.WriteFile(".env", []byte(envContent), 0600); errWrite != nil {
		t.Fatalf("failed to write .env: %v", errWrite)
	}

	// Preset POWERWORD_GEMINI_API_KEY to test that it doesn't get overridden
	t.Setenv("POWERWORD_GEMINI_API_KEY", "preset-gemini-key")
	_ = os.Unsetenv("POWERWORD_MODEL")
	_ = os.Unsetenv("SOME_OTHER_VAR")

	defer func() {
		_ = os.Unsetenv("POWERWORD_GEMINI_API_KEY")
		_ = os.Unsetenv("POWERWORD_MODEL")
		_ = os.Unsetenv("SOME_OTHER_VAR")
	}()

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig returned unexpected error: %v", err)
	}

	// Should keep the preset key instead of using the dotenv one
	if cfg.APIKeys.Gemini != "preset-gemini-key" {
		t.Errorf("expected Gemini API key to keep its preset value 'preset-gemini-key', got '%s'", cfg.APIKeys.Gemini)
	}

	// Should load the model key which wasn't preset
	if cfg.Model != "dotenv-model" {
		t.Errorf("expected Model to load from dotenv as 'dotenv-model', got '%s'", cfg.Model)
	}

	// Should NOT load SOME_OTHER_VAR (doesn't start with POWERWORD_)
	if os.Getenv("SOME_OTHER_VAR") != "" {
		t.Errorf("expected SOME_OTHER_VAR to not be loaded, got '%s'", os.Getenv("SOME_OTHER_VAR"))
	}
}

func TestBindEnv_Error(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected bindEnv to panic with 0 arguments, but it did not")
		}
	}()

	v := viper.New()
	bindEnv(v) // 0 arguments triggers BindEnv error/panic
}
