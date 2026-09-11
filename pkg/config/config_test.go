package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func clearEnv() func() {
	orig := os.Environ()
	hasPrefixOrMatch := func(name string) bool {
		if strings.HasPrefix(name, "POWERWORD_") {
			return true
		}
		switch name {
		case "GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "SERP_API_KEY", "AMAZON_API_KEY":
			return true
		}
		return false
	}
	for _, env := range orig {
		name := strings.SplitN(env, "=", 2)[0]
		if hasPrefixOrMatch(name) {
			_ = os.Unsetenv(name)
		}
	}
	return func() {
		for _, env := range orig {
			name := strings.SplitN(env, "=", 2)[0]
			if hasPrefixOrMatch(name) {
				kv := strings.SplitN(env, "=", 2)
				if len(kv) == 2 {
					_ = os.Setenv(kv[0], kv[1])
				}
			}
		}
	}
}

func TestLoadConfig_Success(t *testing.T) {
	defer clearEnv()()
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
	defer clearEnv()()
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
	t.Setenv("POWERWORD_CRITIC_PROVIDER", "ollama")
	t.Setenv("POWERWORD_CRITIC_MODEL", "llama3")
	t.Setenv("POWERWORD_CRITIC_ENDPOINT", "http://localhost:11434")
	t.Setenv("POWERWORD_GIT_ROLLBACK", "true")

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
	if cfg.CriticProvider != "ollama" {
		t.Errorf("expected CriticProvider overridden to 'ollama', got '%s'", cfg.CriticProvider)
	}
	if cfg.CriticModel != "llama3" {
		t.Errorf("expected CriticModel overridden to 'llama3', got '%s'", cfg.CriticModel)
	}
	if cfg.CriticEndpoint != "http://localhost:11434" {
		t.Errorf("expected CriticEndpoint overridden to 'http://localhost:11434', got '%s'", cfg.CriticEndpoint)
	}
	if !cfg.GitRollback {
		t.Errorf("expected GitRollback overridden to true, got false")
	}
}

func TestLoadConfig_MissingCustomConfig(t *testing.T) {
	defer clearEnv()()
	_, err := LoadConfig("non-existent-file.toml")
	if err == nil {
		t.Errorf("expected error for missing custom config file, got nil")
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	defer clearEnv()()
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
	defer clearEnv()()
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
	defer clearEnv()()
	origPath := DefaultConfigPath
	defer func() { DefaultConfigPath = origPath }()
	DefaultConfigPath = ""

	_, err := LoadConfig("")
	if err == nil {
		t.Error("expected error when LoadConfig is called with empty cfgFile and empty DefaultConfigPath, got nil")
	}
}

func TestLoadConfig_DotEnv(t *testing.T) {
	defer clearEnv()()
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
	defer clearEnv()()
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
	defer clearEnv()()
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
	defer clearEnv()()
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

func TestLoadConfig_LegacyDisableCritic(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	// 1. disable_critic = true in TOML (enable_critic not set) -> EnableCritic should be false
	tomlContent1 := `
disable_critic = true
[api_keys]
gemini = "key"
`
	cfgFilePath1 := filepath.Join(tmpDir, "config1.toml")
	_ = os.WriteFile(cfgFilePath1, []byte(tomlContent1), 0600)

	cfg1, err := LoadConfig(cfgFilePath1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg1.EnableCritic {
		t.Error("expected EnableCritic to be false when disable_critic is true")
	}

	// 2. disable_critic = false in TOML (enable_critic not set) -> EnableCritic should be true
	tomlContent2 := `
disable_critic = false
[api_keys]
gemini = "key"
`
	cfgFilePath2 := filepath.Join(tmpDir, "config2.toml")
	_ = os.WriteFile(cfgFilePath2, []byte(tomlContent2), 0600)

	cfg2, err := LoadConfig(cfgFilePath2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg2.EnableCritic {
		t.Error("expected EnableCritic to be true when disable_critic is false")
	}

	// 3. POWERWORD_DISABLE_CRITIC env var set to true -> EnableCritic should be false
	t.Setenv("POWERWORD_DISABLE_CRITIC", "true")
	cfg3, err := LoadConfig(cfgFilePath2) // use a clean config without enable_critic set
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg3.EnableCritic {
		t.Error("expected EnableCritic to be false when POWERWORD_DISABLE_CRITIC env is true")
	}
}

func TestLoadConfig_APIKeyFallbacks(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	tomlContent := `
[api_keys]
gemini = "gemini-toml-key"
`
	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// Scenario 1: OS-level GEMINI_API_KEY set, no POWERWORD_* set
	t.Setenv("GEMINI_API_KEY", "gemini-canonical-val")
	cfg, err := LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.APIKeys.Gemini != "gemini-canonical-val" {
		t.Errorf("expected Gemini key to be 'gemini-canonical-val', got '%s'", cfg.APIKeys.Gemini)
	}

	// Scenario 2: OS-level POWERWORD_GEMINI_API_KEY set alongside GEMINI_API_KEY
	t.Setenv("POWERWORD_GEMINI_API_KEY", "gemini-powerword-val")
	cfg, err = LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.APIKeys.Gemini != "gemini-powerword-val" {
		t.Errorf("expected Gemini key to be 'gemini-powerword-val' (POWERWORD_ prefix wins), got '%s'", cfg.APIKeys.Gemini)
	}

	// Scenario 3: OS-level GOOGLE_API_KEY set, neither GEMINI_API_KEY nor POWERWORD_GEMINI_API_KEY set
	t.Setenv("POWERWORD_GEMINI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "google-canonical-val")
	cfg, err = LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.APIKeys.Gemini != "google-canonical-val" {
		t.Errorf("expected Gemini key to be 'google-canonical-val', got '%s'", cfg.APIKeys.Gemini)
	}

	// Scenario 4: OS-level ANTHROPIC_API_KEY set
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-canonical-val")
	cfg, err = LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.APIKeys.Anthropic != "anthropic-canonical-val" {
		t.Errorf("expected Anthropic key to be 'anthropic-canonical-val', got '%s'", cfg.APIKeys.Anthropic)
	}

	// Scenario 5: OS-level OPENAI_API_KEY set
	t.Setenv("OPENAI_API_KEY", "openai-canonical-val")
	cfg, err = LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.APIKeys.OpenAI != "openai-canonical-val" {
		t.Errorf("expected OpenAI key to be 'openai-canonical-val', got '%s'", cfg.APIKeys.OpenAI)
	}

	// Scenario 6: OS-level SERP_API_KEY set
	t.Setenv("SERP_API_KEY", "serp-canonical-val")
	cfg, err = LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Plugins.Trends.SerpAPIKey != "serp-canonical-val" {
		t.Errorf("expected SerpAPI key to be 'serp-canonical-val', got '%s'", cfg.Plugins.Trends.SerpAPIKey)
	}
}

func TestLoadConfig_APIKeyFallbacksDotEnv(t *testing.T) {
	defer clearEnv()()
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
POWERWORD_GEMINI_API_KEY=dotenv-powerword-key
GEMINI_API_KEY=dotenv-gemini-key
OPENAI_API_KEY=dotenv-openai-key
ANTHROPIC_API_KEY=dotenv-anthropic-key
SERP_API_KEY=dotenv-serp-key
AMAZON_API_KEY=dotenv-amazon-key
`
	if errWrite := os.WriteFile(".env", []byte(envContent), 0600); errWrite != nil {
		t.Fatalf("failed to write .env: %v", errWrite)
	}

	// Preset none of them in OS env
	_ = os.Unsetenv("POWERWORD_GEMINI_API_KEY")
	_ = os.Unsetenv("POWERWORD_OPENAI_API_KEY")
	_ = os.Unsetenv("POWERWORD_ANTHROPIC_API_KEY")
	_ = os.Unsetenv("POWERWORD_SERP_API_KEY")
	_ = os.Unsetenv("POWERWORD_AMAZON_API_KEY")
	_ = os.Unsetenv("GEMINI_API_KEY")
	_ = os.Unsetenv("OPENAI_API_KEY")
	_ = os.Unsetenv("ANTHROPIC_API_KEY")
	_ = os.Unsetenv("SERP_API_KEY")
	_ = os.Unsetenv("AMAZON_API_KEY")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg.APIKeys.Gemini != "dotenv-powerword-key" {
		t.Errorf("expected Gemini API key 'dotenv-powerword-key' (POWERWORD_* takes precedence in same source), got '%s'", cfg.APIKeys.Gemini)
	}
	if cfg.APIKeys.OpenAI != "dotenv-openai-key" {
		t.Errorf("expected OpenAI API key 'dotenv-openai-key', got '%s'", cfg.APIKeys.OpenAI)
	}
	if cfg.APIKeys.Anthropic != "dotenv-anthropic-key" {
		t.Errorf("expected Anthropic API key 'dotenv-anthropic-key', got '%s'", cfg.APIKeys.Anthropic)
	}
	if cfg.Plugins.Trends.SerpAPIKey != "dotenv-serp-key" {
		t.Errorf("expected SerpAPI key 'dotenv-serp-key', got '%s'", cfg.Plugins.Trends.SerpAPIKey)
	}
	if cfg.Plugins.Amazon.APIKey != "dotenv-amazon-key" {
		t.Errorf("expected Amazon API key 'dotenv-amazon-key', got '%s'", cfg.Plugins.Amazon.APIKey)
	}

	// Test OS-level override wins over .env (including OS-level canonical overriding .env POWERWORD_ key)
	_ = os.Unsetenv("POWERWORD_GEMINI_API_KEY")
	_ = os.Unsetenv("POWERWORD_OPENAI_API_KEY")
	_ = os.Unsetenv("POWERWORD_ANTHROPIC_API_KEY")
	_ = os.Unsetenv("POWERWORD_SERP_API_KEY")
	_ = os.Unsetenv("POWERWORD_AMAZON_API_KEY")
	t.Setenv("GEMINI_API_KEY", "os-gemini-override")
	t.Setenv("AMAZON_API_KEY", "os-amazon-override")
	cfg, err = LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.APIKeys.Gemini != "os-gemini-override" {
		t.Errorf("expected OS environment variable to override .env, got '%s'", cfg.APIKeys.Gemini)
	}
	if cfg.Plugins.Amazon.APIKey != "os-amazon-override" {
		t.Errorf("expected OS environment variable to override .env, got '%s'", cfg.Plugins.Amazon.APIKey)
	}
}

func TestLoadFromWorkspace_WithToml(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	tomlContent := `
[api_keys]
gemini = "workspace-gemini-key"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "powerword.toml"), []byte(tomlContent), 0600); err != nil {
		t.Fatalf("failed to write powerword.toml: %v", err)
	}

	cfg, err := LoadFromWorkspace(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromWorkspace returned unexpected error: %v", err)
	}
	if cfg.APIKeys.Gemini != "workspace-gemini-key" {
		t.Errorf("expected Gemini key 'workspace-gemini-key', got '%s'", cfg.APIKeys.Gemini)
	}
}

func TestLoadFromWorkspace_NoToml_FallsBackToGlobal(t *testing.T) {
	defer clearEnv()()
	origPath := DefaultConfigPath
	defer func() { DefaultConfigPath = origPath }()

	// Point global config at a valid file
	globalTmpDir := t.TempDir()
	globalToml := filepath.Join(globalTmpDir, "global.toml")
	if err := os.WriteFile(globalToml, []byte("[api_keys]\ngemini = \"global-key\"\n"), 0600); err != nil {
		t.Fatalf("failed to write global toml: %v", err)
	}
	DefaultConfigPath = globalToml

	workspaceTmpDir := t.TempDir() // no powerword.toml here

	cfg, err := LoadFromWorkspace(workspaceTmpDir)
	if err != nil {
		t.Fatalf("LoadFromWorkspace returned unexpected error: %v", err)
	}
	if cfg.APIKeys.Gemini != "global-key" {
		t.Errorf("expected fallback Gemini key 'global-key', got '%s'", cfg.APIKeys.Gemini)
	}
}

func TestLoadFromWorkspace_NoToml_NoGlobal_ReturnsEmpty(t *testing.T) {
	defer clearEnv()()
	origPath := DefaultConfigPath
	defer func() { DefaultConfigPath = origPath }()
	DefaultConfigPath = filepath.Join(t.TempDir(), "nonexistent.toml")

	workspaceTmpDir := t.TempDir() // no powerword.toml

	cfg, err := LoadFromWorkspace(workspaceTmpDir)
	if err != nil {
		t.Fatalf("LoadFromWorkspace returned unexpected error: %v", err)
	}
	// Should return an empty config (not nil)
	if cfg == nil {
		t.Fatal("expected non-nil config, got nil")
	}
}

func TestLoadConfig_GDoc(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	tomlContent := `
[plugins.gdoc]
credentials_path = "/path/to/creds.json"
token_path = "/path/to/token.json"
service_account_path = "/path/to/sa.json"
`
	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// 1. Test TOML parsing
	cfg, err := LoadConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("LoadConfig returned unexpected error: %v", err)
	}

	if cfg.Plugins.GDoc.CredentialsPath != "/path/to/creds.json" {
		t.Errorf("expected credentials_path to be '/path/to/creds.json', got '%s'", cfg.Plugins.GDoc.CredentialsPath)
	}
	if cfg.Plugins.GDoc.TokenPath != "/path/to/token.json" {
		t.Errorf("expected token_path to be '/path/to/token.json', got '%s'", cfg.Plugins.GDoc.TokenPath)
	}
	if cfg.Plugins.GDoc.ServiceAccountPath != "/path/to/sa.json" {
		t.Errorf("expected service_account_path to be '/path/to/sa.json', got '%s'", cfg.Plugins.GDoc.ServiceAccountPath)
	}

	// 2. Test Env override
	t.Setenv("POWERWORD_GDOC_CREDENTIALS_PATH", "/env/creds.json")
	t.Setenv("POWERWORD_GDOC_TOKEN_PATH", "/env/token.json")
	t.Setenv("POWERWORD_GDOC_SERVICE_ACCOUNT_PATH", "/env/sa.json")

	cfgEnv, errEnv := LoadConfig(cfgFilePath)
	if errEnv != nil {
		t.Fatalf("LoadConfig returned unexpected error with env: %v", errEnv)
	}

	if cfgEnv.Plugins.GDoc.CredentialsPath != "/env/creds.json" {
		t.Errorf("expected env override credentials_path to be '/env/creds.json', got '%s'", cfgEnv.Plugins.GDoc.CredentialsPath)
	}
	if cfgEnv.Plugins.GDoc.TokenPath != "/env/token.json" {
		t.Errorf("expected env override token_path to be '/env/token.json', got '%s'", cfgEnv.Plugins.GDoc.TokenPath)
	}
	if cfgEnv.Plugins.GDoc.ServiceAccountPath != "/env/sa.json" {
		t.Errorf("expected env override service_account_path to be '/env/sa.json', got '%s'", cfgEnv.Plugins.GDoc.ServiceAccountPath)
	}
}

func TestLoadConfig_ImageGenRetryEnvOverrides(t *testing.T) {
	defer clearEnv()()
	t.Setenv("POWERWORD_GEMINI_API_KEY", "dummy-key")
	t.Setenv("POWERWORD_IMAGEGEN_MAX_RETRIES", "4")
	t.Setenv("POWERWORD_IMAGEGEN_RETRY_BACKOFF", "350ms")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig returned unexpected error: %v", err)
	}

	if cfg.Plugins.ImageGen.MaxRetries != 4 {
		t.Errorf("expected ImageGen.MaxRetries to be 4, got %d", cfg.Plugins.ImageGen.MaxRetries)
	}
	if cfg.Plugins.ImageGen.RetryBackoff != "350ms" {
		t.Errorf("expected ImageGen.RetryBackoff to be '350ms', got %q", cfg.Plugins.ImageGen.RetryBackoff)
	}
}
