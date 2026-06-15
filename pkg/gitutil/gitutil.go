package gitutil

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// ExecCommand is a package-level function variable that defaults to exec.CommandContext.
// It can be overridden in unit tests to mock command execution.
var ExecCommand = exec.CommandContext

// LookPath is a package-level function variable that defaults to exec.LookPath.
// It can be overridden in unit tests to mock PATH lookup.
var LookPath = exec.LookPath

var (
	gitBinaryCached string
	gitBinaryMu     sync.RWMutex
)

// RunGitCommand executes a git command and captures combined stdout/stderr for detailed error reporting.
// It filters out coverage warning lines to avoid contaminating output.
func RunGitCommand(ctx context.Context, dir string, args ...string) (string, error) {
	// Robustness check: Ensure git executable is in the PATH (using cached lookup)
	gitBinaryMu.RLock()
	cached := gitBinaryCached
	gitBinaryMu.RUnlock()

	if cached == "" {
		path, err := LookPath("git")
		if err != nil {
			return "", fmt.Errorf("git binary not found in PATH: %w", err)
		}
		gitBinaryMu.Lock()
		gitBinaryCached = path
		gitBinaryMu.Unlock()
	}

	cmd := ExecCommand(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	outStr := string(out)
	if strings.Contains(outStr, "warning: GOCOVERDIR not set") {
		var cleanLines []string
		for _, line := range strings.Split(outStr, "\n") {
			if strings.Contains(line, "warning: GOCOVERDIR not set") {
				continue
			}
			cleanLines = append(cleanLines, line)
		}
		outStr = strings.Join(cleanLines, "\n")
	}

	if err != nil {
		return "", fmt.Errorf("git command %v failed: %w (output: %q)", args, err, strings.TrimSpace(outStr))
	}
	return outStr, nil
}

// IsInsideWorkTree checks if the directory is inside a Git working tree.
func IsInsideWorkTree(ctx context.Context, dir string) (bool, error) {
	out, err := RunGitCommand(ctx, dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "true", nil
}

// GetHeadCommit returns the HEAD commit hash.
func GetHeadCommit(ctx context.Context, dir string) (string, error) {
	out, err := RunGitCommand(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Init initializes a new Git repository in the target directory.
func Init(ctx context.Context, dir string) error {
	_, err := RunGitCommand(ctx, dir, "init")
	return err
}

// AddAll adds all files in the directory to the Git staging area.
func AddAll(ctx context.Context, dir string) error {
	_, err := RunGitCommand(ctx, dir, "add", "-A")
	return err
}

// Commit commits currently staged changes with the provided message.
func Commit(ctx context.Context, dir string, message string) error {
	_, err := RunGitCommand(ctx, dir, "commit", "-m", message)
	return err
}

// Clean cleans untracked files and directories.
func Clean(ctx context.Context, dir string) error {
	_, err := RunGitCommand(ctx, dir, "clean", "-fd")
	return err
}

// ResetHard resets the worktree and index to the specified commit.
func ResetHard(ctx context.Context, dir string, commit string) error {
	_, err := RunGitCommand(ctx, dir, "reset", "--hard", commit)
	return err
}

// StashPush pushes uncommitted changes (including untracked files) to the stash stack with a message.
func StashPush(ctx context.Context, dir string, message string) error {
	_, err := RunGitCommand(ctx, dir, "stash", "push", "-u", "-m", message)
	return err
}

// StashApply applies the stash entry at the specified index, preserving index state.
func StashApply(ctx context.Context, dir string, index int) error {
	_, err := RunGitCommand(ctx, dir, "stash", "apply", "--index", fmt.Sprintf("stash@{%d}", index))
	return err
}

// StashPop pops (applies and drops) the stash entry at the specified index, preserving index state.
func StashPop(ctx context.Context, dir string, index int) error {
	_, err := RunGitCommand(ctx, dir, "stash", "pop", "--index", fmt.Sprintf("stash@{%d}", index))
	return err
}

// StashDrop drops the stash entry at the specified index.
func StashDrop(ctx context.Context, dir string, index int) error {
	_, err := RunGitCommand(ctx, dir, "stash", "drop", fmt.Sprintf("stash@{%d}", index))
	return err
}

// StashList lists all stash entries.
func StashList(ctx context.Context, dir string) (string, error) {
	return RunGitCommand(ctx, dir, "stash", "list")
}
