package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

func TestGDoc_Bootstrap(t *testing.T) {
	tmpDir := t.TempDir()

	cfgContent := `
[plugins.gdoc]
credentials_path = "/nonexistent/creds.json"
token_path = "/nonexistent/token.json"
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

	if cfg.Plugins.GDoc.CredentialsPath != "/nonexistent/creds.json" {
		t.Errorf("expected credentials path to be /nonexistent/creds.json, got %s", cfg.Plugins.GDoc.CredentialsPath)
	}

	srv, err := setupServer(cfg, nil)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	if srv == nil {
		t.Error("expected server to be initialized, got nil")
	}
}
