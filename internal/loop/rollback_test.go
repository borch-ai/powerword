package loop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	//nolint:gosec,noctx
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git command %v failed in %s: %v\nOutput: %s", args, dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.name", "Test User")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")

	// Create an initial commit
	testFile := filepath.Join(dir, "readme.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	runGitCmd(t, dir, "add", "readme.md")
	runGitCmd(t, dir, "commit", "-m", "initial commit")
}

func TestNewWorkspaceSnapshot_NonGit(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := NewWorkspaceSnapshot(context.Background(), tmpDir)
	if err == nil {
		t.Error("expected error for non-git workspace, got nil")
	}
}

func TestWorkspaceSnapshot_CleanRepo(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	ctx := context.Background()
	snap, err := NewWorkspaceSnapshot(ctx, tmpDir)
	if err != nil {
		t.Fatalf("NewWorkspaceSnapshot failed: %v", err)
	}

	if snap.HasStash {
		t.Error("expected clean repo to not have stash")
	}

	// Make a change and commit
	testFile := filepath.Join(tmpDir, "readme.md")
	if wErr := os.WriteFile(testFile, []byte("# Test Repo\nModified\n"), 0600); wErr != nil {
		t.Fatalf("failed to write file: %v", wErr)
	}
	runGitCmd(t, tmpDir, "add", "readme.md")
	runGitCmd(t, tmpDir, "commit", "-m", "agent commit")

	// Create an untracked file
	untrackedFile := filepath.Join(tmpDir, "newfile.txt")
	if wErr := os.WriteFile(untrackedFile, []byte("agent new file"), 0600); wErr != nil {
		t.Fatalf("failed to write file: %v", wErr)
	}

	// Restore
	if rErr := snap.Restore(ctx); rErr != nil {
		t.Fatalf("Restore failed: %v", rErr)
	}

	// Verify restored back to initial commit
	//nolint:gosec
	readmeContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read readme: %v", err)
	}
	if string(readmeContent) != "# Test Repo\n" {
		t.Errorf("expected original readme content, got %q", string(readmeContent))
	}

	// Verify untracked file was deleted
	if _, err := os.Stat(untrackedFile); !os.IsNotExist(err) {
		t.Errorf("expected untracked file to be deleted, but it exists")
	}
}

func TestWorkspaceSnapshot_DirtyRepo(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Make dirty uncommitted changes
	testFile := filepath.Join(tmpDir, "readme.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\nUser Dirty\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Create an untracked file before snapshot
	userUntracked := filepath.Join(tmpDir, "userfile.txt")
	if err := os.WriteFile(userUntracked, []byte("user untracked"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	ctx := context.Background()
	snap, err := NewWorkspaceSnapshot(ctx, tmpDir)
	if err != nil {
		t.Fatalf("NewWorkspaceSnapshot failed: %v", err)
	}

	if !snap.HasStash {
		t.Error("expected dirty repo to have stash")
	}

	// Make sure dirty changes are still in the working directory (as they were applied back)
	//nolint:gosec
	readmeContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read readme: %v", err)
	}
	if !strings.Contains(string(readmeContent), "User Dirty") {
		t.Errorf("expected worktree changes to remain applied after snapshot")
	}

	// Make agent changes and commit
	if wErr := os.WriteFile(testFile, []byte("# Test Repo\nUser Dirty\nAgent Changes\n"), 0600); wErr != nil {
		t.Fatalf("failed to write file: %v", wErr)
	}
	runGitCmd(t, tmpDir, "add", "readme.md")
	runGitCmd(t, tmpDir, "commit", "-m", "agent commit 1")

	// Create an untracked file
	agentUntracked := filepath.Join(tmpDir, "agentfile.txt")
	if wErr := os.WriteFile(agentUntracked, []byte("agent file"), 0600); wErr != nil {
		t.Fatalf("failed to write file: %v", wErr)
	}

	// Restore
	if rErr := snap.Restore(ctx); rErr != nil {
		t.Fatalf("Restore failed: %v", rErr)
	}

	// Verify readme content reverted to user dirty state (before agent changes)
	//nolint:gosec
	readmeContentRestored, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read readme: %v", err)
	}
	if string(readmeContentRestored) != "# Test Repo\nUser Dirty\n" {
		t.Errorf("expected readme to revert to user dirty, got %q", string(readmeContentRestored))
	}

	// Verify user file still exists
	//nolint:gosec
	userContent, err := os.ReadFile(userUntracked)
	if err != nil {
		t.Errorf("expected user file to exist: %v", err)
	}
	if string(userContent) != "user untracked" {
		t.Errorf("expected user file content, got %q", string(userContent))
	}

	// Verify agent file was deleted
	if _, err := os.Stat(agentUntracked); !os.IsNotExist(err) {
		t.Errorf("expected agent file to be deleted, but it exists")
	}
}

func TestWorkspaceSnapshot_CleanUp(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Make dirty changes
	testFile := filepath.Join(tmpDir, "readme.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\nUser Dirty\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	ctx := context.Background()
	snap, err := NewWorkspaceSnapshot(ctx, tmpDir)
	if err != nil {
		t.Fatalf("NewWorkspaceSnapshot failed: %v", err)
	}

	if !snap.HasStash {
		t.Error("expected dirty repo to have stash")
	}

	// Clean up snapshot stash (success case)
	if err := snap.CleanUp(ctx); err != nil {
		t.Fatalf("CleanUp failed: %v", err)
	}

	// Stash list should not contain our stash message
	stashList := runGitCmd(t, tmpDir, "stash", "list")
	if strings.Contains(stashList, snap.StashMessage) {
		t.Error("expected stash to be dropped, but it is still present")
	}
}

//nolint:gocognit,funlen
func TestWorkspaceSnapshot_MockedErrors(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	ctx := context.Background()

	// 1. failed to get original HEAD
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "rev-parse" && args[1] == "HEAD" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	_, err := NewWorkspaceSnapshot(ctx, tmpDir)
	if err == nil || !strings.Contains(err.Error(), "failed to get original HEAD") {
		t.Errorf("expected error getting HEAD, got: %v", err)
	}

	// 2. failed to check git status
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "status" && args[1] == "--porcelain" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	_, err = NewWorkspaceSnapshot(ctx, tmpDir)
	if err == nil || !strings.Contains(err.Error(), "failed to check git status") {
		t.Errorf("expected error checking status, got: %v", err)
	}

	// Make dirty changes for stash tests
	testFile := filepath.Join(tmpDir, "readme.md")
	if wErr := os.WriteFile(testFile, []byte("# Test Repo\nUser Dirty\n"), 0600); wErr != nil {
		t.Fatalf("failed to write file: %v", wErr)
	}

	// 3. failed to stash changes
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "stash" && args[1] == "push" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	_, err = NewWorkspaceSnapshot(ctx, tmpDir)
	if err == nil || !strings.Contains(err.Error(), "failed to stash uncommitted changes") {
		t.Errorf("expected error stashing, got: %v", err)
	}

	// Restore original exec to take a successful snapshot with stash
	execCommand = origExec
	snap, err := NewWorkspaceSnapshot(ctx, tmpDir)
	if err != nil {
		t.Fatalf("expected successful snapshot creation, got: %v", err)
	}
	if !snap.HasStash {
		t.Fatal("expected snap to have stash")
	}

	// 4. failed to reset to original commit in Restore
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "reset" && args[1] == "--hard" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	err = snap.Restore(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to reset to original commit") {
		t.Errorf("expected error resetting commit in Restore, got: %v", err)
	}

	// 5. failed to clean in Restore
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 0 && args[0] == "clean" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	err = snap.Restore(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to clean untracked files") {
		t.Errorf("expected error cleaning in Restore, got: %v", err)
	}

	// 6. failed to list stash in Restore (findStashIndex failure)
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "stash" && args[1] == "list" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	err = snap.Restore(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to find snapshot stash") {
		t.Errorf("expected error finding stash in Restore, got: %v", err)
	}

	// 7. stash message not found (findStashIndex not found)
	execCommand = origExec
	badSnap := &WorkspaceSnapshot{
		Dir:            tmpDir,
		OriginalCommit: snap.OriginalCommit,
		HasStash:       true,
		StashMessage:   "non-existent-stash-message-12345",
	}
	err = badSnap.Restore(ctx)
	if err == nil || !strings.Contains(err.Error(), "stash with message") {
		t.Errorf("expected stash not found error in Restore, got: %v", err)
	}

	// 8. failed to pop stash in Restore
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "stash" && args[1] == "pop" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	err = snap.Restore(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to pop stash") {
		t.Errorf("expected error popping stash in Restore, got: %v", err)
	}

	// 9. CleanUp: failed to list stash (findStashIndex failure)
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "stash" && args[1] == "list" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	err = snap.CleanUp(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to find snapshot stash for cleanup") {
		t.Errorf("expected error finding stash in CleanUp, got: %v", err)
	}

	// 10. CleanUp: failed to drop stash
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "stash" && args[1] == "drop" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}
	err = snap.CleanUp(ctx)
	if err == nil || !strings.Contains(err.Error(), "failed to drop stash") {
		t.Errorf("expected error dropping stash in CleanUp, got: %v", err)
	}
}

func TestNewWorkspaceSnapshot_StashApplyFails(t *testing.T) {
	origExec := execCommand
	defer func() { execCommand = origExec }()

	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Make dirty changes
	testFile := filepath.Join(tmpDir, "readme.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\nUser Dirty\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	ctx := context.Background()

	// Mock git stash apply failure
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "git" && len(args) > 1 && args[0] == "stash" && args[1] == "apply" {
			return exec.CommandContext(ctx, "false")
		}
		return origExec(ctx, command, args...)
	}

	_, err := NewWorkspaceSnapshot(ctx, tmpDir)
	if err == nil || !strings.Contains(err.Error(), "failed to apply stashed changes") {
		t.Errorf("expected stash apply failure error, got: %v", err)
	}
}

func TestWorkspaceSnapshot_GitBinaryNotFound(t *testing.T) {
	origPath := os.Getenv("PATH")
	defer func() { _ = os.Setenv("PATH", origPath) }()

	// Temporarily break PATH so git cannot be found
	_ = os.Setenv("PATH", "")

	ctx := context.Background()
	_, err := NewWorkspaceSnapshot(ctx, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "git binary not found") {
		t.Errorf("expected git binary not found error in NewWorkspaceSnapshot, got: %v", err)
	}

	snap := &WorkspaceSnapshot{
		Dir:            t.TempDir(),
		OriginalCommit: "abcdef",
		HasStash:       true,
		StashMessage:   "test-stash",
	}

	err = snap.Restore(ctx)
	if err == nil || !strings.Contains(err.Error(), "git binary not found") {
		t.Errorf("expected git binary not found error in Restore, got: %v", err)
	}

	err = snap.CleanUp(ctx)
	if err == nil || !strings.Contains(err.Error(), "git binary not found") {
		t.Errorf("expected git binary not found error in CleanUp, got: %v", err)
	}
}
