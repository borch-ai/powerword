package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		"github.com/borch-ai/powerword/internal/review/validator.go:37.52,41.2 3 1\n"

	if err := os.WriteFile(covFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write cover file: %v", err)
	}

	cmd := newCheckCoverageCmd()
	err := cmd.RunE(cmd, []string{"90.0", covFile})
	if err != nil {
		t.Errorf("expected CLI command to succeed, got error: %v", err)
	}
}
