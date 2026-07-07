package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mockLinkIssueCmd(name string, args []string, diffOut string, prBody string, errCmd string) Cmd {
	return &mockCmd{
		outputFunc: func() ([]byte, error) {
			if errCmd == name {
				return nil, fmt.Errorf("mock error")
			}
			if name == "git" && len(args) > 0 && args[0] == "diff" {
				return []byte(diffOut), nil
			}
			if name == "gh" && len(args) > 1 && args[0] == "pr" && args[1] == "view" {
				return []byte(prBody), nil
			}
			return []byte("success"), nil
		},
		runFunc: func() error {
			if errCmd == name {
				return fmt.Errorf("mock error")
			}
			return nil
		},
	}
}

func TestLinkTaskIssue_NoPlansModified(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, name string, args ...string) Cmd {
		return mockLinkIssueCmd(name, args, "internal/review/link_issue.go", "", "")
	}

	err := LinkTaskIssue(context.Background(), "123", "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestLinkTaskIssue_NoIssuesInPlans(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	tmpDir, err := os.MkdirTemp("", "powerword_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a dummy plans file without issues
	plansDir := filepath.Join(tmpDir, "plans")
	if mkdirErr := os.MkdirAll(plansDir, 0750); mkdirErr != nil {
		t.Fatalf("failed to create plans dir: %v", mkdirErr)
	}
	planPath := filepath.Join(plansDir, "task_1.md")
	if writeErr := os.WriteFile(planPath, []byte("# No issues here"), 0600); writeErr != nil {
		t.Fatalf("failed to write plan file: %v", writeErr)
	}

	// Change working directory to temp dir for file reading
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if chdirErr := os.Chdir(tmpDir); chdirErr != nil {
		t.Fatalf("failed to chdir: %v", chdirErr)
	}
	defer func() { _ = os.Chdir(origWd) }()

	execCommand = func(ctx context.Context, name string, args ...string) Cmd {
		return mockLinkIssueCmd(name, args, "plans/task_1.md\nplans/non_existent.md", "", "")
	}

	err = LinkTaskIssue(context.Background(), "123", "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestLinkTaskIssue_AllIssuesAlreadyLinked(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	tmpDir, err := os.MkdirTemp("", "powerword_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	plansDir := filepath.Join(tmpDir, "plans")
	if mkdirErr := os.MkdirAll(plansDir, 0750); mkdirErr != nil {
		t.Fatalf("failed to create plans dir: %v", mkdirErr)
	}
	planPath := filepath.Join(plansDir, "task_1.md")
	if writeErr := os.WriteFile(planPath, []byte("# Status: Completed (Issue #45)"), 0600); writeErr != nil {
		t.Fatalf("failed to write plan file: %v", writeErr)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if chdirErr := os.Chdir(tmpDir); chdirErr != nil {
		t.Fatalf("failed to chdir: %v", chdirErr)
	}
	defer func() { _ = os.Chdir(origWd) }()

	execCommand = func(ctx context.Context, name string, args ...string) Cmd {
		return mockLinkIssueCmd(name, args, "plans/task_1.md", `{"body": "This fixes #45 description."}`, "")
	}

	err = LinkTaskIssue(context.Background(), "123", "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestLinkTaskIssue_LinksMissingIssues(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	tmpDir, err := os.MkdirTemp("", "powerword_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	plansDir := filepath.Join(tmpDir, "plans")
	if mkdirErr := os.MkdirAll(plansDir, 0750); mkdirErr != nil {
		t.Fatalf("failed to create plans dir: %v", mkdirErr)
	}
	planPath := filepath.Join(plansDir, "task_1.md")
	if writeErr := os.WriteFile(planPath, []byte("# Status: Completed (Issue #45) and Issue #67"), 0600); writeErr != nil {
		t.Fatalf("failed to write plan file: %v", writeErr)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if chdirErr := os.Chdir(tmpDir); chdirErr != nil {
		t.Fatalf("failed to chdir: %v", chdirErr)
	}
	defer func() { _ = os.Chdir(origWd) }()

	var capturedBody string
	var editCalled bool

	execCommand = func(ctx context.Context, name string, args ...string) Cmd {
		return mockExecForMissingIssues(ctx, name, args, &editCalled, &capturedBody)
	}

	err = LinkTaskIssue(context.Background(), "123", "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !editCalled {
		t.Error("expected gh pr edit to be called, but it was not")
	}

	if !strings.Contains(capturedBody, "Closes #67") {
		t.Errorf("expected updated body to contain Closes #67, got %s", capturedBody)
	}
	if strings.Contains(capturedBody, "Closes #45") {
		t.Error("expected updated body NOT to contain Closes #45 since it was already linked")
	}
}

// Helper to keep TestLinkTaskIssue_LinksMissingIssues cognitive complexity low
func mockExecForMissingIssues(ctx context.Context, name string, args []string, editCalled *bool, capturedBody *string) Cmd {
	return &mockCmd{
		outputFunc: func() ([]byte, error) {
			if name == "git" && len(args) > 0 && args[0] == "diff" {
				return []byte("plans/task_1.md"), nil
			}
			if name == "gh" && len(args) > 1 && args[0] == "pr" && args[1] == "view" {
				return []byte(`{"body": "This closes #45."}`), nil
			}
			return []byte("success"), nil
		},
		runFunc: func() error {
			if name == "gh" && len(args) > 1 && args[0] == "pr" && args[1] == "edit" {
				*editCalled = true
				for i, arg := range args {
					if arg == "--body-file" && i+1 < len(args) {
						content, _ := os.ReadFile(args[i+1])
						*capturedBody = string(content)
					}
				}
				return nil
			}
			return nil
		},
	}
}

func TestLinkTaskIssue_GitDiffError(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	execCommand = func(ctx context.Context, name string, args ...string) Cmd {
		return mockLinkIssueCmd(name, args, "", "", "git")
	}

	err := LinkTaskIssue(context.Background(), "123", "main")
	if err == nil {
		t.Error("expected error from git diff, got nil")
	}
}

func TestLinkTaskIssue_PRViewError(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	tmpDir, err := os.MkdirTemp("", "powerword_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	plansDir := filepath.Join(tmpDir, "plans")
	if mkdirErr := os.MkdirAll(plansDir, 0750); mkdirErr != nil {
		t.Fatalf("failed to create plans dir: %v", mkdirErr)
	}
	planPath := filepath.Join(plansDir, "task_1.md")
	if writeErr := os.WriteFile(planPath, []byte("# Status: Completed (Issue #45)"), 0600); writeErr != nil {
		t.Fatalf("failed to write plan file: %v", writeErr)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if chdirErr := os.Chdir(tmpDir); chdirErr != nil {
		t.Fatalf("failed to chdir: %v", chdirErr)
	}
	defer func() { _ = os.Chdir(origWd) }()

	execCommand = func(ctx context.Context, name string, args ...string) Cmd {
		if name == "gh" && len(args) > 1 && args[0] == "pr" && args[1] == "view" {
			return mockLinkIssueCmd(name, args, "plans/task_1.md", "", "gh")
		}
		return mockLinkIssueCmd(name, args, "plans/task_1.md", "", "")
	}

	err = LinkTaskIssue(context.Background(), "123", "main")
	if err == nil {
		t.Error("expected error from gh pr view failure, got nil")
	}
}

func TestLinkTaskIssue_PREditError(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	tmpDir, err := os.MkdirTemp("", "powerword_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	plansDir := filepath.Join(tmpDir, "plans")
	if mkdirErr := os.MkdirAll(plansDir, 0750); mkdirErr != nil {
		t.Fatalf("failed to create plans dir: %v", mkdirErr)
	}
	planPath := filepath.Join(plansDir, "task_1.md")
	if writeErr := os.WriteFile(planPath, []byte("# Status: Completed (Issue #45)"), 0600); writeErr != nil {
		t.Fatalf("failed to write plan file: %v", writeErr)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if chdirErr := os.Chdir(tmpDir); chdirErr != nil {
		t.Fatalf("failed to chdir: %v", chdirErr)
	}
	defer func() { _ = os.Chdir(origWd) }()

	execCommand = func(ctx context.Context, name string, args ...string) Cmd {
		if name == "gh" && len(args) > 1 && args[0] == "pr" && args[1] == "edit" {
			return mockLinkIssueCmd(name, args, "plans/task_1.md", `{"body": "This has nothing."}`, "gh")
		}
		return mockLinkIssueCmd(name, args, "plans/task_1.md", `{"body": "This has nothing."}`, "")
	}

	err = LinkTaskIssue(context.Background(), "123", "main")
	if err == nil {
		t.Error("expected error from gh pr edit failure, got nil")
	}
}
