package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
)

func TestReviewCmd_FixOnlyNoAPIKey(t *testing.T) {
	// 1. Set config.Active to a config with no API keys
	origConfig := config.Active
	config.Active = &config.Config{
		APIKeys: config.APIKeys{},
	}
	defer func() { config.Active = origConfig }()

	// 2. Create a temporary workspace directory to simulate a repo
	tmpDir := t.TempDir()

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

	// 3. Create the command and run review --fix
	cmd := newReviewCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--fix"})

	err = cmd.Execute()
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
	fixedContent, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("failed to read fixed plan: %v", err)
	}
	if !strings.Contains(string(fixedContent), "[root.go](file://../pkg/config/root.go)") {
		t.Errorf("expected absolute path to be fixed to relative, got:\n%s", string(fixedContent))
	}
}

func TestReviewCmd_LocalRequiresAPIKey(t *testing.T) {
	// Set config.Active to a config with no API keys
	origConfig := config.Active
	config.Active = &config.Config{
		APIKeys: config.APIKeys{},
	}
	defer func() { config.Active = origConfig }()

	cmd := newReviewCmd()
	cmd.SetArgs([]string{"--local"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error due to missing API keys, got nil")
	}
	if !strings.Contains(err.Error(), "no API keys found") {
		t.Errorf("expected no API keys found error, got: %v", err)
	}
}

func TestReviewCmd_LocalWithAPIKey(t *testing.T) {
	// Set config.Active to a config with API keys
	origConfig := config.Active
	config.Active = &config.Config{
		APIKeys: config.APIKeys{
			Gemini: "fake-key",
		},
	}
	defer func() { config.Active = origConfig }()

	// Assert that calling review --local fails on something other than API key validation
	cmd := newReviewCmd()
	cmd.SetArgs([]string{"--local"})

	err := cmd.Execute()
	// It should NOT fail on config validation ("no API keys found")
	if err != nil && strings.Contains(err.Error(), "no API keys found") {
		t.Errorf("did not expect API key validation error, got: %v", err)
	}
}

func TestReviewCmd_InvalidFlags(t *testing.T) {
	// Set config.Active with valid API keys to pass config validation
	origConfig := config.Active
	config.Active = &config.Config{
		APIKeys: config.APIKeys{
			Gemini: "fake-key",
		},
	}
	defer func() { config.Active = origConfig }()

	// 1. Missing both local and issue
	cmd1 := newReviewCmd()
	cmd1.SetArgs([]string{})
	err1 := cmd1.Execute()
	if err1 == nil || !strings.Contains(err1.Error(), "either --issue or --local must be provided") {
		t.Errorf("expected 'either --issue or --local must be provided', got %v", err1)
	}

	// 2. Both local and issue provided
	cmd2 := newReviewCmd()
	cmd2.SetArgs([]string{"--local", "--issue", "123"})
	err2 := cmd2.Execute()
	if err2 == nil || !strings.Contains(err2.Error(), "cannot provide both --local and --issue flags") {
		t.Errorf("expected 'cannot provide both --local and --issue flags', got %v", err2)
	}
}
