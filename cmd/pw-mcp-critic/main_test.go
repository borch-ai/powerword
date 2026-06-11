package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRun_WithConfig(t *testing.T) {
	tempDir := t.TempDir()

	// Create a dummy powerword.toml
	configPath := filepath.Join(tempDir, "powerword.toml")
	err := os.WriteFile(configPath, []byte(`
critic_provider = "openai"
critic_model = "gpt-4"

[api_keys]
openai = "test-openai-key"
`), 0600)
	if err != nil {
		t.Fatalf("failed to write dummy config: %v", err)
	}

	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	// Create a pipe to mock Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() {
		_ = r.Close()
		_ = w.Close()
	}()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
	}()

	// Close the write end immediately so the StdioTransport reads EOF and terminates
	_ = w.Close()

	err = run()
	if err != nil {
		t.Errorf("expected no error from run() on Stdin EOF, got: %v", err)
	}
}

func TestRun_NoConfig(t *testing.T) {
	tempDir := t.TempDir()
	// No powerword.toml in tempDir

	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() {
		_ = r.Close()
		_ = w.Close()
	}()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
	}()

	_ = w.Close()

	err = run()
	if err != nil {
		t.Errorf("expected no error from run() on missing config, got: %v", err)
	}
}

func TestRun_InvalidConfig(t *testing.T) {
	tempDir := t.TempDir()

	// Write malformed TOML configuration
	configPath := filepath.Join(tempDir, "powerword.toml")
	err := os.WriteFile(configPath, []byte(`
[api_keys
openai = "test-openai-key"
`), 0600)
	if err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	err = run()
	if err == nil {
		t.Error("expected error from run() on malformed config, got nil")
	}
}
