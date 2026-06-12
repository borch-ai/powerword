package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestMain(m *testing.M) {
	Runner = func(ctx context.Context, cfg *Config, prompt string) error {
		return nil
	}
	os.Exit(m.Run())
}

func TestRootCmd_Success(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
verbose = true
model = "toml-model"
[api_keys]
gemini = "gemini-key"
`
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// Reset Active
	Active = nil

	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--config", cfgFilePath, "test prompt"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("cmd.Execute returned unexpected error: %v", err)
	}

	if Active == nil {
		t.Fatalf("expected Active config to be loaded, got nil")
	}

	if Active.Model != "toml-model" {
		t.Errorf("expected Active.Model to be 'toml-model', got '%s'", Active.Model)
	}

	if !Active.Verbose {
		t.Errorf("expected Active.Verbose to be true, got false")
	}
}

func TestRootCmd_FlagOverrides(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
verbose = false
model = "toml-model"
[api_keys]
gemini = "gemini-key"
`
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// Reset Active
	Active = nil

	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	// Override model via flag
	cmd.SetArgs([]string{
		"--config", cfgFilePath,
		"--model", "flag-model",
		"--verbose",
		"--accept-all",
		"--headless",
		"--json",
		"--git-rollback",
		"--autonomous",
		"--issue", "456",
		"--classifier-model", "classifier",
		"--route", "foo=bar",
		"--max-cost", "5.0",
		"--max-tokens", "2000000",
		"--max-input-tokens", "1500000",
		"--max-output-tokens", "250000",
		"--max-cached-tokens", "100000",
		"test prompt",
	})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("cmd.Execute returned unexpected error: %v", err)
	}

	if Active == nil {
		t.Fatalf("expected Active config to be loaded, got nil")
	}

	verifyFlagOverrides(t, Active)
}

func verifyFlagOverrides(t *testing.T, active *Config) {
	if active.Model != "flag-model" {
		t.Errorf("expected Active.Model to be overridden to 'flag-model', got '%s'", active.Model)
	}

	if !active.Verbose {
		t.Errorf("expected Active.Verbose to be overridden to true, got false")
	}

	if !active.AutoConfirm {
		t.Errorf("expected Active.AutoConfirm to be overridden to true, got false")
	}

	if !active.Headless {
		t.Errorf("expected Active.Headless to be overridden to true, got false")
	}

	if !active.JSONOutput {
		t.Errorf("expected Active.JSONOutput to be overridden to true, got false")
	}

	if !active.GitRollback {
		t.Errorf("expected Active.GitRollback to be overridden to true, got false")
	}

	if !active.Autonomous {
		t.Errorf("expected Active.Autonomous to be overridden to true, got false")
	}

	if active.Issue != "456" {
		t.Errorf("expected Active.Issue to be overridden to '456', got '%s'", active.Issue)
	}

	if active.ClassifierModel != "classifier" {
		t.Errorf("expected Active.ClassifierModel to be overridden to 'classifier', got '%s'", active.ClassifierModel)
	}

	if active.Route == nil || active.Route["foo"] != "bar" {
		t.Errorf("expected Active.Route to be overridden, got %v", active.Route)
	}

	if active.MaxCost != 5.0 {
		t.Errorf("expected Active.MaxCost to be overridden to 5.0, got %f", active.MaxCost)
	}

	if active.MaxTokens != 2000000 {
		t.Errorf("expected Active.MaxTokens to be overridden to 2000000, got %d", active.MaxTokens)
	}

	if active.MaxInputTokens != 1500000 {
		t.Errorf("expected Active.MaxInputTokens to be overridden to 1500000, got %d", active.MaxInputTokens)
	}

	if active.MaxOutputTokens != 250000 {
		t.Errorf("expected Active.MaxOutputTokens to be overridden to 250000, got %d", active.MaxOutputTokens)
	}

	if active.MaxCachedTokens != 100000 {
		t.Errorf("expected Active.MaxCachedTokens to be overridden to 100000, got %d", active.MaxCachedTokens)
	}
}

func TestRootCmd_ValidationError(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
verbose = false
model = "toml-model"
`
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// Reset Active
	Active = nil

	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--config", cfgFilePath, "test prompt"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
}

func TestRootCmd_NoArgs(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
verbose = false
model = "toml-model"
[api_keys]
gemini = "gemini-key"
`
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// Reset Active
	Active = nil

	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	// Run without prompt args, should call help
	cmd.SetArgs([]string{"--config", cfgFilePath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("cmd.Execute returned unexpected error: %v", err)
	}
}

func TestExecute(t *testing.T) {
	defer clearEnv()()
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Mock command arguments for executing the test
	os.Args = []string{"powerword", "test prompt"}

	// Set API key via env so validation passes
	t.Setenv("POWERWORD_GEMINI_API_KEY", "env-gemini-key")

	errExec := Execute()
	if errExec != nil {
		t.Fatalf("Execute returned error: %v", errExec)
	}
}

func TestRootCmd_VersionFlag(t *testing.T) {
	Version = "v1.2.3"
	defer func() { Version = "dev" }()

	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("cmd.Execute returned unexpected error: %v", err)
	}

	expected := "powerword version v1.2.3\n"
	if buf.String() != expected {
		t.Errorf("expected version output %q, got %q", expected, buf.String())
	}
}

func TestRootCmd_SubcommandConfigLoading(t *testing.T) {
	defer clearEnv()()
	tmpDir := t.TempDir()

	cfgFilePath := filepath.Join(tmpDir, "config.toml")
	tomlContent := `
verbose = true
model = "toml-model"
[api_keys]
gemini = "gemini-key"
`
	if errWrite := os.WriteFile(cfgFilePath, []byte(tomlContent), 0600); errWrite != nil {
		t.Fatalf("failed to write temp config: %v", errWrite)
	}

	// Reset Active
	Active = nil

	cmd := NewRootCmd()

	// Create a mock subcommand
	subCmd := &cobra.Command{
		Use: "mocksub",
		RunE: func(c *cobra.Command, args []string) error {
			if Active == nil {
				t.Fatalf("expected Active config to be loaded in subcommand, got nil")
			}
			return nil
		},
	}
	cmd.AddCommand(subCmd)

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Execute subcommand without arguments
	cmd.SetArgs([]string{"mocksub", "--config", cfgFilePath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("cmd.Execute returned unexpected error: %v", err)
	}

	if Active == nil {
		t.Fatalf("expected Active config to be loaded, got nil")
	}
}
