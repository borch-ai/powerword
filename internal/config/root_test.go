package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

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
