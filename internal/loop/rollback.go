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
	// Check if inside a git repository
	gitCheck := execCommand(ctx, "git", "rev-parse", "--is-inside-work-tree")
	gitCheck.Dir = dir
	if err := gitCheck.Run(); err != nil {
		return nil, fmt.Errorf("workspace %q is not inside a git repository: %w", dir, err)
	}

	// Get original commit hash
	gitRev := execCommand(ctx, "git", "rev-parse", "HEAD")
	gitRev.Dir = dir
	out, err := gitRev.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get original HEAD commit hash: %w", err)
	}
	originalCommit := strings.TrimSpace(string(out))

	// Check if dirty
	gitStatus := execCommand(ctx, "git", "status", "--porcelain")
	gitStatus.Dir = dir
	statusOut, err := gitStatus.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to check git status: %w", err)
	}
	isDirty := len(strings.TrimSpace(string(statusOut))) > 0

	snap := &WorkspaceSnapshot{
		Dir:            dir,
		OriginalCommit: originalCommit,
	}

	if isDirty {
		// Create a unique stash message containing original commit to prevent collisions
		stashMsg := fmt.Sprintf("powerword-snapshot-%s", originalCommit)

		// Push to stash including untracked files
		stashPush := execCommand(ctx, "git", "stash", "push", "-u", "-m", stashMsg)
		stashPush.Dir = dir
		if err := stashPush.Run(); err != nil {
			return nil, fmt.Errorf("failed to stash uncommitted changes: %w", err)
		}
		snap.HasStash = true
		snap.StashMessage = stashMsg

		// Re-apply stash immediately so the agent can see and modify the changes
		stashApply := execCommand(ctx, "git", "stash", "apply", "stash@{0}")
		stashApply.Dir = dir
		if err := stashApply.Run(); err != nil {
			// Clean up stash if apply fails
			stashDrop := execCommand(ctx, "git", "stash", "drop", "stash@{0}")
			stashDrop.Dir = dir
			_ = stashDrop.Run()
			return nil, fmt.Errorf("failed to apply stashed changes: %w", err)
		}
	}

	return snap, nil
}

// Restore rolls back the workspace changes to the snapshot state.
func (s *WorkspaceSnapshot) Restore(ctx context.Context) error {
	// 1. Reset HEAD and hard reset to original commit
	resetCmd := execCommand(ctx, "git", "reset", "--hard", s.OriginalCommit)
	resetCmd.Dir = s.Dir
	if err := resetCmd.Run(); err != nil {
		return fmt.Errorf("failed to reset to original commit %s: %w", s.OriginalCommit, err)
	}

	// 2. Clean untracked files/directories
	cleanCmd := execCommand(ctx, "git", "clean", "-fd")
	cleanCmd.Dir = s.Dir
	if err := cleanCmd.Run(); err != nil {
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
		// Pop the stash to restore original uncommitted changes
		stashPop := execCommand(ctx, "git", "stash", "pop", stashRef)
		stashPop.Dir = s.Dir
		if err := stashPop.Run(); err != nil {
			return fmt.Errorf("failed to pop stash %s: %w", stashRef, err)
		}
	}

	return nil
}

// CleanUp cleans up the stash if it was created, without restoring it.
func (s *WorkspaceSnapshot) CleanUp(ctx context.Context) error {
	if s.HasStash {
		stashIndex, err := s.findStashIndex(ctx)
		if err != nil {
			return fmt.Errorf("failed to find snapshot stash for cleanup: %w", err)
		}

		stashRef := fmt.Sprintf("stash@{%d}", stashIndex)
		stashDrop := execCommand(ctx, "git", "stash", "drop", stashRef)
		stashDrop.Dir = s.Dir
		if err := stashDrop.Run(); err != nil {
			return fmt.Errorf("failed to drop stash %s: %w", stashRef, err)
		}
	}
	return nil
}

// findStashIndex returns the stash index matching the message.
func (s *WorkspaceSnapshot) findStashIndex(ctx context.Context) (int, error) {
	stashList := execCommand(ctx, "git", "stash", "list")
	stashList.Dir = s.Dir
	out, err := stashList.Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if strings.Contains(line, s.StashMessage) {
			return i, nil
		}
	}
	return 0, fmt.Errorf("stash with message %q not found", s.StashMessage)
}
