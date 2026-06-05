package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRootCmd_Success(t *testing.T) {
	// Create a temp directory and write config
	tmpDir, err := os.MkdirTemp("", "pw-root-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

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

	// Reset Active and flag variables
	Active = nil
	cfgFile = ""
	model = ""
	verbose = false

	// Prepare buffers
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--config", cfgFilePath, "test prompt"})

	err = RootCmd.Execute()
	if err != nil {
		t.Fatalf("RootCmd.Execute returned unexpected error: %v", err)
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
	// Create a temp directory and write config
	tmpDir, err := os.MkdirTemp("", "pw-root-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

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
	cfgFile = ""
	model = ""
	verbose = false

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	// Override model via flag
	RootCmd.SetArgs([]string{"--config", cfgFilePath, "--model", "flag-model", "--verbose", "test prompt"})

	err = RootCmd.Execute()
	if err != nil {
		t.Fatalf("RootCmd.Execute returned unexpected error: %v", err)
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
	// Create config with no API keys
	tmpDir, err := os.MkdirTemp("", "pw-root-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

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
	cfgFile = ""
	model = ""
	verbose = false

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--config", cfgFilePath, "test prompt"})

	err = RootCmd.Execute()
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
}

func TestRootCmd_NoArgs(t *testing.T) {
	// Create config with gemini key
	tmpDir, err := os.MkdirTemp("", "pw-root-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

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
	cfgFile = ""
	model = ""
	verbose = false

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	// Run without prompt args, should call help
	RootCmd.SetArgs([]string{"--config", cfgFilePath})

	err = RootCmd.Execute()
	if err != nil {
		t.Fatalf("RootCmd.Execute returned unexpected error: %v", err)
	}
}
