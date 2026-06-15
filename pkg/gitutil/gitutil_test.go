package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:errcheck
func initRealRepo(t *testing.T, dir string) {
	t.Helper()
	ctx := context.Background()
	if err := Init(ctx, dir); err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	_, _ = RunGitCommand(ctx, dir, "config", "user.name", "Test User")
	_, _ = RunGitCommand(ctx, dir, "config", "user.email", "test@example.com")
}

//nolint:gocognit,gosec,funlen,errcheck
func TestGitUtil_RealOperations(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := context.Background()

	// 1. IsInsideWorkTree should return false or error in a non-git directory
	inside, _ := IsInsideWorkTree(ctx, tmpDir)
	if inside {
		t.Log("Note: temp directory is inside a git worktree of parent directory")
	}

	// 2. Init
	initRealRepo(t, tmpDir)

	insideAfter, isErr := IsInsideWorkTree(ctx, tmpDir)
	if isErr != nil {
		t.Fatalf("IsInsideWorkTree failed: %v", isErr)
	}
	if !insideAfter {
		t.Errorf("expected inside worktree to be true after init")
	}

	// 3. GetHeadCommit before any commit should fail
	if _, headErr := GetHeadCommit(ctx, tmpDir); headErr == nil {
		t.Error("expected GetHeadCommit to fail when there are no commits")
	}

	// 4. Create a file, add it, commit it
	filePath := filepath.Join(tmpDir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello world"), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if err := AddAll(ctx, tmpDir); err != nil {
		t.Fatalf("AddAll failed: %v", err)
	}

	if err := Commit(ctx, tmpDir, "initial commit"); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// 5. GetHeadCommit after commit should succeed
	commit1, getErr := GetHeadCommit(ctx, tmpDir)
	if getErr != nil {
		t.Fatalf("GetHeadCommit failed: %v", getErr)
	}
	if len(commit1) == 0 {
		t.Error("expected non-empty commit hash")
	}

	// 6. Test Stashing
	// Modify the file to make it dirty
	if err := os.WriteFile(filePath, []byte("hello dirty"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Create an untracked file
	untrackedPath := filepath.Join(tmpDir, "untracked.txt")
	if err := os.WriteFile(untrackedPath, []byte("untracked content"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	stashMsg := "test-stash-message"
	if err := StashPush(ctx, tmpDir, stashMsg); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}

	// Stash list should contain the message
	list, listErr := StashList(ctx, tmpDir)
	if listErr != nil {
		t.Fatalf("StashList failed: %v", listErr)
	}
	if !strings.Contains(list, stashMsg) {
		t.Errorf("expected stash list to contain %q, got: %q", stashMsg, list)
	}

	// Apply stash index 0
	if err := StashApply(ctx, tmpDir, 0); err != nil {
		t.Fatalf("StashApply failed: %v", err)
	}

	// Check that dirty content is back
	content, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatalf("failed to read file: %v", readErr)
	}
	if string(content) != "hello dirty" {
		t.Errorf("expected 'hello dirty', got %q", string(content))
	}

	// Drop stash index 0
	if err := StashDrop(ctx, tmpDir, 0); err != nil {
		t.Fatalf("StashDrop failed: %v", err)
	}

	// Verify stash list is empty or doesn't contain message
	listAfterDrop, _ := StashList(ctx, tmpDir)
	if strings.Contains(listAfterDrop, stashMsg) {
		t.Error("expected stash to be dropped")
	}

	// Create another stash to test Pop
	if err := os.WriteFile(filePath, []byte("hello pop"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := StashPush(ctx, tmpDir, "pop-stash"); err != nil {
		t.Fatalf("StashPush failed: %v", err)
	}

	// Pop stash index 0
	if err := StashPop(ctx, tmpDir, 0); err != nil {
		t.Fatalf("StashPop failed: %v", err)
	}

	contentPop, _ := os.ReadFile(filePath)
	if string(contentPop) != "hello pop" {
		t.Errorf("expected 'hello pop', got %q", string(contentPop))
	}

	// 7. Clean and ResetHard
	// Create an untracked file
	untrackedClean := filepath.Join(tmpDir, "untracked_clean.txt")
	if err := os.WriteFile(untrackedClean, []byte("untracked clean"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if err := Clean(ctx, tmpDir); err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	if _, err := os.Stat(untrackedClean); !os.IsNotExist(err) {
		t.Error("expected untracked file to be cleaned up")
	}

	// Modify tracked file
	if err := os.WriteFile(filePath, []byte("hello modified"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if err := ResetHard(ctx, tmpDir, commit1); err != nil {
		t.Fatalf("ResetHard failed: %v", err)
	}

	contentReset, _ := os.ReadFile(filePath)
	if string(contentReset) != "hello world" {
		t.Errorf("expected reset to restore 'hello world', got %q", string(contentReset))
	}
}

func TestGitUtil_MockedErrors(t *testing.T) {
	origExec := ExecCommand
	defer func() { ExecCommand = origExec }()

	ctx := context.Background()

	// Mock git command failure
	ExecCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}

	_, err := RunGitCommand(ctx, "/tmp", "status")
	if err == nil {
		t.Error("expected RunGitCommand to return error when git command fails")
	}
	if !strings.Contains(err.Error(), "git command [status] failed") {
		t.Errorf("expected error message to contain failure info, got: %v", err)
	}

	// Test coverage warning filtering with mocked output
	ExecCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		// Return a command that prints GOCOVERDIR warning and exits with non-zero (since it's fake)
		return exec.CommandContext(ctx, "sh", "-c", "echo 'warning: GOCOVERDIR not set\nactual output'; exit 1")
	}

	_, err = RunGitCommand(ctx, "/tmp", "status")
	if err == nil {
		t.Fatal("expected command to fail")
	}
	if strings.Contains(err.Error(), "warning: GOCOVERDIR not set") {
		t.Errorf("expected warning to be filtered out of error message, got error: %v", err)
	}
	if !strings.Contains(err.Error(), "actual output") {
		t.Errorf("expected actual output to be preserved in error message, got: %v", err)
	}
}

//nolint:errcheck
func TestGitUtil_GitBinaryNotFound(t *testing.T) {
	gitBinaryMu.Lock()
	gitBinaryCached = ""
	gitBinaryMu.Unlock()

	origPath := os.Getenv("PATH")
	defer func() {
		_ = os.Setenv("PATH", origPath)
		gitBinaryMu.Lock()
		gitBinaryCached = ""
		gitBinaryMu.Unlock()
	}()

	// Temporarily break PATH so git cannot be found
	_ = os.Setenv("PATH", "")

	ctx := context.Background()
	_, err := RunGitCommand(ctx, "/tmp", "status")
	if err == nil || !strings.Contains(err.Error(), "git binary not found in PATH") {
		t.Errorf("expected git binary not found error, got: %v", err)
	}
}

func TestGitUtil_MockedLookPath(t *testing.T) {
	gitBinaryMu.Lock()
	gitBinaryCached = ""
	gitBinaryMu.Unlock()

	origLookPath := LookPath
	LookPath = func(file string) (string, error) {
		return "mocked-git", nil
	}
	origExec := ExecCommand
	ExecCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	}
	defer func() {
		LookPath = origLookPath
		ExecCommand = origExec
		gitBinaryMu.Lock()
		gitBinaryCached = ""
		gitBinaryMu.Unlock()
	}()

	ctx := context.Background()
	_, err := RunGitCommand(ctx, "/tmp", "status")
	if err != nil {
		t.Errorf("expected success with mocked LookPath, got: %v", err)
	}
}
