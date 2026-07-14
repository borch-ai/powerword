package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

func TestAmazon_Bootstrap(t *testing.T) {
	tmpDir := t.TempDir()

	cfgContent := `
[plugins.amazon]
api_key = "test-bootstrap-key"
base_url = "https://example.com/api"
`
	err := os.WriteFile(filepath.Join(tmpDir, "powerword.toml"), []byte(cfgContent), 0600)
	if err != nil {
		t.Fatalf("failed to write dummy config: %v", err)
	}

	t.Setenv("POWERWORD_WORKSPACE_ROOT", tmpDir)

	cfg, err := config.LoadFromWorkspace(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Plugins.Amazon.APIKey != "test-bootstrap-key" {
		t.Errorf("expected API key to be test-bootstrap-key, got %s", cfg.Plugins.Amazon.APIKey)
	}
	if cfg.Plugins.Amazon.BaseURL != "https://example.com/api" {
		t.Errorf("expected BaseURL to be https://example.com/api, got %s", cfg.Plugins.Amazon.BaseURL)
	}

	srv, err := setupServer(tmpDir, cfg, nil)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	if srv == nil {
		t.Error("expected server to be initialized, got nil")
	}
}
