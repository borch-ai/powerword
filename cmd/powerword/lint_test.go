package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintPlansCmd_Success(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	planContent := `# plan: Task 1.1: Test Plan
**Status:** Open

## User Review Required
None.

## Proposed Changes
#### [MODIFY] [powerword.toml](file://../powerword.toml)
- Edit it.

## Verification Plan
### Automated Tests
- Run tests.
`
	if err := os.WriteFile(filepath.Join(plansDir, "task_1_1.md"), []byte(planContent), 0600); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	cmd := newLintPlansCmd()
	_ = cmd.Flags().Set("path", tmpDir)

	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Errorf("expected success, got error: %v", err)
	}
}

func TestLintPlansCmd_Failure(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	planContent := `# plan: Task 1.1: Test Plan
**Status:** Open

## User Review Required
None.

## Proposed Changes
#### [MODIFY] [powerword.toml](file://../powerword.toml)
- Edit it.
`
	if err := os.WriteFile(filepath.Join(plansDir, "task_1_1.md"), []byte(planContent), 0600); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	cmd := newLintPlansCmd()
	_ = cmd.Flags().Set("path", tmpDir)

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestLintGoCmd_Execution(t *testing.T) {
	tmpDir := t.TempDir()
	goModContent := "module github.com/borch-ai/test-module\ngo 1.26\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0600); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldCwd)
	}()

	cmd := newLintGoCmd()
	_ = cmd.Flags().Set("config", "nonexistent-config-file.yml")

	err = cmd.RunE(cmd, []string{})
	if err != nil && !strings.Contains(err.Error(), "nonexistent-config-file.yml") && !strings.Contains(err.Error(), "no such file") {
		t.Logf("got error (expected if golangci-lint fails to read config): %v", err)
	}
}
