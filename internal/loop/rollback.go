package loop

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/borch-ai/powerword/pkg/gitutil"
)

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

	var (
		out       string
		err       error
		targetDir string
		absDir    string
	)

	// Resolve and pin the directory path to be stable
	targetDir = dir
	if targetDir == "" {
		// Run in current directory to find the top level of the git repo
		out, err = gitutil.RunGitCommand(ctx, "", "rev-parse", "--show-toplevel")
		if err != nil {
			return nil, fmt.Errorf("failed to get git repository root: %w", err)
		}
		targetDir = strings.TrimSpace(out)
	}

	absDir, absErr := filepath.Abs(targetDir)
	if absErr != nil {
		return nil, fmt.Errorf("failed to get absolute path for target directory: %w", absErr)
	}
	targetDir = absDir

	// Check if inside a git repository
	inside, err := gitutil.IsInsideWorkTree(ctx, targetDir)
	if err != nil {
		return nil, fmt.Errorf("workspace %q is not inside a git repository: %w", targetDir, err)
	}
	if !inside {
		return nil, fmt.Errorf("workspace %q is not inside a git repository", targetDir)
	}

	// Get original commit hash
	originalCommit, err := gitutil.GetHeadCommit(ctx, targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get original HEAD commit hash: %w", err)
	}

	// Check if dirty
	statusOut, err := gitutil.RunGitCommand(ctx, targetDir, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to check git status: %w", err)
	}
	isDirty := len(strings.TrimSpace(statusOut)) > 0

	snap := &WorkspaceSnapshot{
		Dir:            targetDir,
		OriginalCommit: originalCommit,
	}

	if isDirty {
		// Create a unique stash message containing original commit to prevent collisions
		stashMsg := fmt.Sprintf("powerword-snapshot-%s", originalCommit)

		// Push to stash including untracked files
		if err = gitutil.StashPush(ctx, targetDir, stashMsg); err != nil {
			return nil, fmt.Errorf("failed to stash uncommitted changes: %w", err)
		}
		snap.HasStash = true
		snap.StashMessage = stashMsg

		// Re-apply stash immediately so the agent can see and modify the changes, preserving index state
		if err = gitutil.StashApply(ctx, targetDir, 0); err != nil {
			// Clean up stash if apply fails
			//nolint:errcheck
			_ = gitutil.StashDrop(ctx, targetDir, 0)
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
	if err := gitutil.ResetHard(ctx, s.Dir, s.OriginalCommit); err != nil {
		return fmt.Errorf("failed to reset to original commit %s: %w", s.OriginalCommit, err)
	}

	// 2. Clean untracked files/directories
	if err := gitutil.Clean(ctx, s.Dir); err != nil {
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
		if err := gitutil.StashPop(ctx, s.Dir, stashIndex); err != nil {
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
		if err := gitutil.StashDrop(ctx, s.Dir, stashIndex); err != nil {
			return fmt.Errorf("failed to drop stash %s: %w", stashRef, err)
		}
	}
	return nil
}

// findStashIndex returns the stash index matching the message.
func (s *WorkspaceSnapshot) findStashIndex(ctx context.Context) (int, error) {
	out, err := gitutil.StashList(ctx, s.Dir)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// A strict check to ensure we match our stash message exactly.
		// The list entry is formatted as: stash@{N}: On <branch>: <message>
		// or stash@{N}: WIP on <branch>: <hash> <message>
		if !strings.HasSuffix(line, ": "+s.StashMessage) {
			continue
		}

		var n int
		if _, err := fmt.Sscanf(line, "stash@{%d}", &n); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("stash with message %q not found", s.StashMessage)
}
