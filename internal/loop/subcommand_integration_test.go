//go:build integration

package loop_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_ReviewSubcommand(t *testing.T) {
	// Create a temp workspace
	workspaceDir, err := os.MkdirTemp("", "pw-review-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	plansDir := filepath.Join(workspaceDir, "plans")
	if err := os.MkdirAll(plansDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create a mock go.mod to allow Go version lookup
	if err := os.WriteFile(filepath.Join(workspaceDir, "go.mod"), []byte("module testmod\ngo 1.26.4\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create target file so it exists under workspace root for validation
	if err := os.MkdirAll(filepath.Join(workspaceDir, "pkg", "config"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "pkg", "config", "root.go"), []byte("package config\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create a plan with absolute path (needs auto-fixing)
	badPlan := `# plan: Task Check Path Normalize

**Status:** Open
**Go Version:** —
**Date Completed:** —
**Unit Test Coverage:** —

## User Review Required

## Proposed Changes
- [MODIFY] [root.go](file:///Users/human/code/powerword/pkg/config/root.go)

## Verification Plan
`
	planPath := filepath.Join(plansDir, "task_check.md")
	if err := os.WriteFile(planPath, []byte(badPlan), 0600); err != nil {
		t.Fatal(err)
	}

	// Create config file with critic disabled so it skips LLM validation
	cfgPath := filepath.Join(workspaceDir, "powerword.toml")
	cfgContent := `
enable_critic = false
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binaryPath, "review", "--local", "--fix", "--config", cfgPath)
	cmd.Dir = workspaceDir
	cmd.Env = append(os.Environ(), "POWERWORD_GEMINI_API_KEY=dummy")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("powerword review --local --fix failed: %v. Output:\n%s", err, string(out))
	}

	// Verify plan file was updated to relative path
	fixedContent, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	expectedRelLink := "[root.go](file://../pkg/config/root.go)"
	if !strings.Contains(string(fixedContent), expectedRelLink) {
		t.Errorf("expected plan file to be auto-fixed with relative link %q, got:\n%s", expectedRelLink, string(fixedContent))
	}
}

func TestCLI_LinkIssueSubcommand(t *testing.T) {
	// Create a temp directory to compile stub gh binary
	tmpDir, err := os.MkdirTemp("", "pw-stub-gh-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write stub gh source code
	stubGoSource := `package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) >= 3 && args[0] == "pr" && args[1] == "view" {
		fmt.Print("{\"body\": \"This is the PR description body without issues\\n\"}")
		return
	}
	if len(args) >= 3 && args[0] == "pr" && args[1] == "edit" {
		return
	}
	fmt.Fprintf(os.Stderr, "unexpected mock arguments: %v\n", args)
	os.Exit(1)
}
`
	stubSrcPath := filepath.Join(tmpDir, "gh_stub.go")
	if err := os.WriteFile(stubSrcPath, []byte(stubGoSource), 0600); err != nil {
		t.Fatal(err)
	}

	stubGhPath := filepath.Join(tmpDir, "gh")
	compileCmd := exec.Command("go", "build", "-o", stubGhPath, stubSrcPath)
	if err := compileCmd.Run(); err != nil {
		t.Fatalf("failed to compile stub gh binary: %v", err)
	}

	// Create a temp workspace for git repository
	workspaceDir, err := os.MkdirTemp("", "pw-link-issue-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Git init
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = workspaceDir
	if err := gitCmd.Run(); err != nil {
		t.Fatalf("failed to git init: %v", err)
	}

	for key, val := range map[string]string{"user.email": "test@test.com", "user.name": "Test User"} {
		configCmd := exec.Command("git", "config", key, val)
		configCmd.Dir = workspaceDir
		if err := configCmd.Run(); err != nil {
			t.Fatalf("failed to config git %s: %v", key, err)
		}
	}

	// Create initial plans file
	plansDir := filepath.Join(workspaceDir, "plans")
	if err := os.MkdirAll(plansDir, 0700); err != nil {
		t.Fatal(err)
	}
	planFile := filepath.Join(plansDir, "task_issue.md")
	if err := os.WriteFile(planFile, []byte("# plan: Task Issue Linker\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Stage and commit initial file
	addCmd := exec.Command("git", "add", "plans/task_issue.md")
	addCmd.Dir = workspaceDir
	if err := addCmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}
	commitCmd := exec.Command("git", "commit", "-m", "initial commit")
	commitCmd.Dir = workspaceDir
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("failed to git commit: %v", err)
	}

	// Setup local tracking branch origin/main pointing to main branch commit
	originSetupCmd := exec.Command("git", "update-ref", "refs/remotes/origin/main", "HEAD")
	originSetupCmd.Dir = workspaceDir
	if err := originSetupCmd.Run(); err != nil {
		t.Fatalf("failed to setup mock origin/main tracking branch: %v", err)
	}

	// Modify the plan file to reference an issue ID (Issue #789)
	modifiedPlanContent := `# plan: Task Issue Linker

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-20
**Unit Test Coverage:** 91.0%

This implements Task Issue Linker.
Issue #789

## Proposed Changes
- [NEW] [test.go](file:///Users/human/code/powerword/test.go)

## Verification Plan
`
	if err := os.WriteFile(planFile, []byte(modifiedPlanContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Stage and commit the modified plan file so that git diff origin/main...HEAD sees it!
	addCmd2 := exec.Command("git", "add", "plans/task_issue.md")
	addCmd2.Dir = workspaceDir
	if err := addCmd2.Run(); err != nil {
		t.Fatalf("failed to git add modified plan: %v", err)
	}
	commitCmd2 := exec.Command("git", "commit", "-m", "add issue reference")
	commitCmd2.Dir = workspaceDir
	if err := commitCmd2.Run(); err != nil {
		t.Fatalf("failed to git commit modified plan: %v", err)
	}

	// Verify path modification via link-issue subcommand
	cmd := exec.Command(binaryPath, "link-issue", "--pr", "123", "--base", "main")
	cmd.Dir = workspaceDir

	// Prepend stub gh directory to PATH environment variable
	newPath := tmpDir + string(os.PathListSeparator) + os.Getenv("PATH")
	cmd.Env = append(os.Environ(), "PATH="+newPath, "POWERWORD_GEMINI_API_KEY=dummy")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("link-issue subcommand failed: %v. Output:\n%s", err, string(out))
	}

	// Verify that the link-issue printed success logs
	outputStr := string(out)
	if !strings.Contains(outputStr, "Found Issue ID #789 inside plan file") {
		t.Errorf("expected logs to show found Issue ID, got:\n%s", outputStr)
	}
	if !strings.Contains(outputStr, "Successfully updated the PR description") {
		t.Errorf("expected logs to show successful PR update, got:\n%s", outputStr)
	}
}
