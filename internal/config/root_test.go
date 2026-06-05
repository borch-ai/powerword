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
	cmd.SetArgs([]string{"--config", cfgFilePath, "--model", "flag-model", "--verbose", "test prompt"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("cmd.Execute returned unexpected error: %v", err)
	}

	if Active == nil {
		t.Fatalf("expected Active config to be loaded, got nil")
	}

	if Active.Model != "flag-model" {
		t.Errorf("expected Active.Model to be overridden to 'flag-model', got '%s'", Active.Model)
	}

	if !Active.Verbose {
		t.Errorf("expected Active.Verbose to be overridden to true, got false")
	}
}

func TestRootCmd_ValidationError(t *testing.T) {
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
