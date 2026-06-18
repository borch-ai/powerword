package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

func TestCheckCoverageCmd_InvalidThreshold(t *testing.T) {
	cmd := newCheckCoverageCmd()
	err := cmd.RunE(cmd, []string{"invalid_pct"})
	if err == nil {
		t.Error("expected error for invalid threshold float parsing, got nil")
	} else if !strings.Contains(err.Error(), "invalid threshold percentage") {
		t.Errorf("expected invalid threshold error message, got: %v", err)
	}
}

func TestCheckCoverageCmd_Success(t *testing.T) {
	tmpDir := t.TempDir()
	covFile := filepath.Join(tmpDir, "coverage.out")

	// Write valid statement referencing real validator.go file
	content := "mode: set\n" +
		"github.com/borch-ai/powerword/pkg/linter/validator.go:37.52,41.2 3 1\n"

	if err := os.WriteFile(covFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write cover file: %v", err)
	}

	cmd := newCheckCoverageCmd()
	err := cmd.RunE(cmd, []string{"90.0", covFile})
	if err != nil {
		t.Errorf("expected CLI command to succeed, got error: %v", err)
	}
}

func TestCheckCoverageCmd_DetectFormat(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      string
		expectFormat string
	}{
		{
			name:         "lcov info extension",
			filename:     "lcov.info",
			content:      "",
			expectFormat: "lcov",
		},
		{
			name:         "cobertura xml extension",
			filename:     "coverage.xml",
			content:      "",
			expectFormat: "cobertura",
		},
		{
			name:         "go out extension",
			filename:     "coverage.out",
			content:      "",
			expectFormat: "go",
		},
		{
			name:         "go content mode",
			filename:     "custom_cov",
			content:      "mode: set\n",
			expectFormat: "go",
		},
		{
			name:         "lcov content SF",
			filename:     "custom_cov",
			content:      "SF:button.ts\n",
			expectFormat: "lcov",
		},
		{
			name:         "cobertura content xml",
			filename:     "custom_cov",
			content:      "<?xml version=\"1.0\"?>\n<coverage>",
			expectFormat: "cobertura",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, tc.filename)
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			format := detectFormat(path)
			if format != tc.expectFormat {
				t.Errorf("expected format %s, got %s", tc.expectFormat, format)
			}
		})
	}
}

func TestCheckCoverageCmd_SpawnFailure(t *testing.T) {
	// Configure Active with a dummy non-existent binary command to force a spawn error
	config.Active = &config.Config{
		Servers: map[string]config.ServerConfig{
			"coverage": {
				Command: "non_existent_binary_foo_bar_xyz",
			},
		},
	}
	defer func() { config.Active = nil }()

	tmpDir := t.TempDir()
	lcovPath := filepath.Join(tmpDir, "lcov.info")
	if err := os.WriteFile(lcovPath, []byte("SF:foo.ts\nLF:10\nLH:5\nend_of_record\n"), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd := newCheckCoverageCmd()
	err := cmd.RunE(cmd, []string{"90.0", lcovPath})
	if err == nil {
		t.Error("expected error for non-existent server command execution, got nil")
	} else if !strings.Contains(err.Error(), "failed to start MCP coverage server") {
		t.Errorf("expected start server error message, got: %v", err)
	}
}
