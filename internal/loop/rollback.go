package loop

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

var execCommand = exec.CommandContext

// SetExecCommand sets the execCommand variable in the loop package for mocking in tests.
func SetExecCommand(f func(context.Context, string, ...string) *exec.Cmd) {
	execCommand = f
}

// runGitCommand executes a git command and captures combined stdout/stderr for detailed error reporting.
func runGitCommand(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := execCommand(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command %v failed: %w (output: %q)", args, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// WorkspaceSnapshot represents a snapshot of the workspace state before an agent run.
type WorkspaceSnapshot struct {
	Dir            string
	OriginalCommit string
	HasStash       bool
	StashMessage   string
}

// NewWorkspaceSnapshot creates and returns a snapshot of the current workspace state.
// If it fails to run git commands, it returns an error.
func NewWorkspaceSnapshot(ctx context.Context, dir string) (*WorkspaceSnapshot, error) {
	// Robustness check: Ensure git executable is in the PATH
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git binary not found in PATH: %w", err)
	}

	// Check if inside a git repository
	if _, err := runGitCommand(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("workspace %q is not inside a git repository: %w", dir, err)
	}

	// Get original commit hash
	out, err := runGitCommand(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get original HEAD commit hash: %w", err)
	}
	originalCommit := strings.TrimSpace(out)

	// Check if dirty
	statusOut, err := runGitCommand(ctx, dir, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to check git status: %w", err)
	}
	isDirty := len(strings.TrimSpace(statusOut)) > 0

	snap := &WorkspaceSnapshot{
		Dir:            dir,
		OriginalCommit: originalCommit,
	}

	if isDirty {
		// Create a unique stash message containing original commit to prevent collisions
		stashMsg := fmt.Sprintf("powerword-snapshot-%s", originalCommit)

		// Push to stash including untracked files
		if _, err := runGitCommand(ctx, dir, "stash", "push", "-u", "-m", stashMsg); err != nil {
			return nil, fmt.Errorf("failed to stash uncommitted changes: %w", err)
		}
		snap.HasStash = true
		snap.StashMessage = stashMsg

		// Re-apply stash immediately so the agent can see and modify the changes, preserving index state
		if _, err := runGitCommand(ctx, dir, "stash", "apply", "--index", "stash@{0}"); err != nil {
			// Clean up stash if apply fails
			_, _ = runGitCommand(ctx, dir, "stash", "drop", "stash@{0}")
			return nil, fmt.Errorf("failed to apply stashed changes: %w", err)
		}
	}

	return snap, nil
}

// Restore rolls back the workspace changes to the snapshot state.
func (s *WorkspaceSnapshot) Restore(ctx context.Context) error {
	// Robustness check: Ensure git executable is in the PATH
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git binary not found in PATH: %w", err)
	}

	// 1. Reset HEAD and hard reset to original commit
	if _, err := runGitCommand(ctx, s.Dir, "reset", "--hard", s.OriginalCommit); err != nil {
		return fmt.Errorf("failed to reset to original commit %s: %w", s.OriginalCommit, err)
	}

	// 2. Clean untracked files/directories
	if _, err := runGitCommand(ctx, s.Dir, "clean", "-fd"); err != nil {
		return fmt.Errorf("failed to clean untracked files: %w", err)
	}

	// 3. Restore stash if we created one
	if s.HasStash {
		// Find the stash index matching our stash message
		stashIndex, err := s.findStashIndex(ctx)
		if err != nil {
			return fmt.Errorf("failed to find snapshot stash: %w", err)
		}

		stashRef := fmt.Sprintf("stash@{%d}", stashIndex)
		// Pop the stash to restore original uncommitted changes, preserving index state
		if _, err := runGitCommand(ctx, s.Dir, "stash", "pop", "--index", stashRef); err != nil {
			return fmt.Errorf("failed to pop stash %s: %w", stashRef, err)
		}
	}

	return nil
}

// CleanUp cleans up the stash if it was created, without restoring it.
func (s *WorkspaceSnapshot) CleanUp(ctx context.Context) error {
	if s.HasStash {
		// Robustness check: Ensure git executable is in the PATH
		if _, err := exec.LookPath("git"); err != nil {
			return fmt.Errorf("git binary not found in PATH: %w", err)
		}

		stashIndex, err := s.findStashIndex(ctx)
		if err != nil {
			return fmt.Errorf("failed to find snapshot stash for cleanup: %w", err)
		}

		stashRef := fmt.Sprintf("stash@{%d}", stashIndex)
		if _, err := runGitCommand(ctx, s.Dir, "stash", "drop", stashRef); err != nil {
			return fmt.Errorf("failed to drop stash %s: %w", stashRef, err)
		}
	}
	return nil
}

// findStashIndex returns the stash index matching the message.
func (s *WorkspaceSnapshot) findStashIndex(ctx context.Context) (int, error) {
	out, err := runGitCommand(ctx, s.Dir, "stash", "list")
	if err != nil {
		return 0, err
	}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, s.StashMessage) {
			return i, nil
		}
	}
	return 0, fmt.Errorf("stash with message %q not found", s.StashMessage)
}
