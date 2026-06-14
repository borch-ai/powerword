package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

func setupMockWorkspace(t *testing.T, tmpDir string) string {
	t.Helper()

	// Write a mock go.mod so getGoVersionFromMod works
	goModPath := filepath.Join(tmpDir, "go.mod")
	err := os.WriteFile(goModPath, []byte("module testmodule\ngo 1.26.4\n"), 0600)
	if err != nil {
		t.Fatalf("failed to write mock go.mod: %v", err)
	}

	// Create plans directory and a plan file with an absolute path that needs fixing
	plansDir := filepath.Join(tmpDir, "plans")
	err = os.MkdirAll(plansDir, 0700)
	if err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	// Create target file so it exists under workspace root (just to be safe)
	err = os.MkdirAll(filepath.Join(tmpDir, "pkg", "config"), 0700)
	if err != nil {
		t.Fatalf("failed to create pkg/config: %v", err)
	}
	err = os.WriteFile(filepath.Join(tmpDir, "pkg", "config", "root.go"), []byte("package config"), 0600)
	if err != nil {
		t.Fatalf("failed to write mock root.go: %v", err)
	}

	// The plan validator checks title regex and headings, so let's make it conform
	planContent := `# plan: Task Test Fix Only

**Status:** Open
**Go Version:** —
**Date Completed:** —
**Unit Test Coverage:** —

## Proposed Changes
- [MODIFY] [root.go](file:///Users/human/code/powerword/pkg/config/root.go)

## Verification Plan
`
	planPath := filepath.Join(plansDir, "task_test_fix.md")
	err = os.WriteFile(planPath, []byte(planContent), 0600)
	if err != nil {
		t.Fatalf("failed to write mock plan: %v", err)
	}

	return planPath
}

func TestReviewCmd_FixOnlyNoAPIKey(t *testing.T) {
	origConfig := config.Active
	defer func() { config.Active = origConfig }()

	tmpDir := t.TempDir()

	// Write a mock config file with no API keys
	cfgFilePath := filepath.Join(tmpDir, "powerword.toml")
	err := os.WriteFile(cfgFilePath, []byte("[api_keys]\n"), 0600)
	if err != nil {
		t.Fatalf("failed to write empty config: %v", err)
	}

	planPath := setupMockWorkspace(t, tmpDir)

	// Save current working directory and change to tmpDir so review looks for plans in "."
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("failed to chdir to tmpDir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// 3. Create the root command, register the review command, and execute review --fix
	rootCmd := config.NewRootCmd()
	rootCmd.AddCommand(newReviewCmd())

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"review", "--fix", "--config", cfgFilePath})

	err = rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing review --fix: %v", err)
	}

	// Verify it printed the fix logs
	output := buf.String()
	if !strings.Contains(output, "Checking and auto-fixing") {
		t.Errorf("expected output to contain fix logs, got %q", output)
	}
	if !strings.Contains(output, "Auto-fix complete") {
		t.Errorf("expected output to contain complete log, got %q", output)
	}

	// Verify the file was modified to relative path
	//nolint:gosec
	fixedContent, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("failed to read fixed plan: %v", err)
	}
	if !strings.Contains(string(fixedContent), "[root.go](file://../pkg/config/root.go)") {
		t.Errorf("expected absolute path to be fixed to relative, got:\n%s", string(fixedContent))
	}
}

func TestReviewCmd_LocalRequiresAPIKey(t *testing.T) {
	origConfig := config.Active
	defer func() { config.Active = origConfig }()

	tmpDir := t.TempDir()
	cfgFilePath := filepath.Join(tmpDir, "powerword.toml")
	err := os.WriteFile(cfgFilePath, []byte("[api_keys]\n"), 0600)
	if err != nil {
		t.Fatalf("failed to write empty config: %v", err)
	}

	rootCmd := config.NewRootCmd()
	rootCmd.AddCommand(newReviewCmd())
	rootCmd.SetArgs([]string{"review", "--local", "--config", cfgFilePath})

	err = rootCmd.Execute()
	if err == nil {
		t.Fatalf("expected error due to missing API keys, got nil")
	}
	if !strings.Contains(err.Error(), "no API keys found") {
		t.Errorf("expected no API keys found error, got: %v", err)
	}
}

func TestReviewCmd_LocalWithAPIKey(t *testing.T) {
	origConfig := config.Active
	defer func() { config.Active = origConfig }()

	tmpDir := t.TempDir()
	cfgFilePath := filepath.Join(tmpDir, "powerword.toml")
	err := os.WriteFile(cfgFilePath, []byte("[api_keys]\ngemini = \"fake-key\"\n"), 0600)
	if err != nil {
		t.Fatalf("failed to write config with api key: %v", err)
	}

	rootCmd := config.NewRootCmd()
	rootCmd.AddCommand(newReviewCmd())
	rootCmd.SetArgs([]string{"review", "--local", "--config", cfgFilePath})

	err = rootCmd.Execute()
	// It should NOT fail on config validation ("no API keys found")
	if err != nil && strings.Contains(err.Error(), "no API keys found") {
		t.Errorf("did not expect API key validation error, got: %v", err)
	}
}

func TestReviewCmd_InvalidFlags(t *testing.T) {
	origConfig := config.Active
	defer func() { config.Active = origConfig }()

	tmpDir := t.TempDir()
	cfgFilePath := filepath.Join(tmpDir, "powerword.toml")
	err := os.WriteFile(cfgFilePath, []byte("[api_keys]\ngemini = \"fake-key\"\n"), 0600)
	if err != nil {
		t.Fatalf("failed to write config with api key: %v", err)
	}

	// 1. Missing both local and issue
	rootCmd1 := config.NewRootCmd()
	rootCmd1.AddCommand(newReviewCmd())
	rootCmd1.SetArgs([]string{"review", "--config", cfgFilePath})
	err1 := rootCmd1.Execute()
	if err1 == nil || !strings.Contains(err1.Error(), "either --issue or --local must be provided") {
		t.Errorf("expected 'either --issue or --local must be provided', got %v", err1)
	}

	// 2. Both local and issue provided
	rootCmd2 := config.NewRootCmd()
	rootCmd2.AddCommand(newReviewCmd())
	rootCmd2.SetArgs([]string{"review", "--local", "--issue", "123", "--config", cfgFilePath})
	err2 := rootCmd2.Execute()
	if err2 == nil || !strings.Contains(err2.Error(), "cannot provide both --local and --issue flags") {
		t.Errorf("expected 'cannot provide both --local and --issue flags', got %v", err2)
	}
}
