package gitutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// ExecCommand is a package-level function variable that defaults to exec.CommandContext.
// It can be overridden in unit tests to mock command execution.
var ExecCommand = exec.CommandContext

// LookPath is a package-level function variable that defaults to exec.LookPath.
// It can be overridden in unit tests to mock PATH lookup.
var LookPath = exec.LookPath

func setupCmdEnv(cmd *exec.Cmd) {
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	filtered := make([]string, 0, len(cmd.Env))
	for _, env := range cmd.Env {
		if !strings.HasPrefix(env, "GIT_TERMINAL_PROMPT=") {
			filtered = append(filtered, env)
		}
	}
	filtered = append(filtered, "GIT_TERMINAL_PROMPT=0")
	cmd.Env = filtered
}

var credentialRegex = regexp.MustCompile(`(https?://)([^@\s]+)(@)`)

// SanitizeGitOutput removes sensitive credentials (like basic auth tokens in URLs
// or DAEDALUS_GITHUB_TOKEN) from git output.
func SanitizeGitOutput(out []byte) string {
	s := strings.TrimSpace(string(out))
	s = credentialRegex.ReplaceAllString(s, "${1}[REDACTED]${3}")
	if token := os.Getenv("DAEDALUS_GITHUB_TOKEN"); token != "" {
		s = strings.ReplaceAll(s, token, "[REDACTED]")
	}
	return s
}

// RunGitCommand executes a git command and captures combined stdout/stderr for detailed error reporting.
// It filters out coverage warning lines to avoid contaminating output, and scrubs credentials.
func RunGitCommand(ctx context.Context, dir string, args ...string) (string, error) {
	// Robustness check: Ensure git executable is in the PATH
	if _, err := LookPath("git"); err != nil {
		return "", fmt.Errorf("git binary not found in PATH: %w", err)
	}

	cmd := ExecCommand(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	setupCmdEnv(cmd)
	out, err := cmd.CombinedOutput()

	outStr := SanitizeGitOutput(out)
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

// Clone clones a repository into the specified directory.
func Clone(ctx context.Context, url, dir, branch string, depth int) error {
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	if depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", depth), "--single-branch")
	}
	args = append(args, "--", url, dir)

	_, err := RunGitCommand(ctx, "", args...)
	return err
}

// Fetch fetches the latest changes from the remote repository.
func Fetch(ctx context.Context, dir, branch string) error {
	args := []string{"fetch"}
	if branch != "" {
		args = append(args, "origin", branch)
	}
	_, err := RunGitCommand(ctx, dir, args...)
	return err
}

// Checkout checks out a specific branch. It does not create it.
func Checkout(ctx context.Context, dir, branch string) error {
	_, err := RunGitCommand(ctx, dir, "checkout", branch)
	return err
}

// CheckoutBranch creates and checks out a new branch, optionally resetting it to a start point (e.g., origin/branch).
func CheckoutBranch(ctx context.Context, dir, branch, startPoint string) error {
	args := []string{"checkout", "-B", branch}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	_, err := RunGitCommand(ctx, dir, args...)
	return err
}

// Branch creates a new branch without checking it out.
func Branch(ctx context.Context, dir, branch, startPoint string) error {
	args := []string{"branch"}
	if startPoint != "" {
		// Set upstream automatically if we are starting from a remote branch
		if strings.HasPrefix(startPoint, "origin/") {
			args = append(args, "--set-upstream-to="+startPoint)
		}
	}
	args = append(args, branch)
	if startPoint != "" {
		args = append(args, startPoint)
	}
	_, err := RunGitCommand(ctx, dir, args...)
	return err
}

// Pull fetches and integrates changes. If branch is not empty, it acts like Daedalus's Pull with specific branch handling.
func Pull(ctx context.Context, dir, branch string) error {
	if branch != "" {
		if err := Fetch(ctx, dir, branch); err != nil {
			return err
		}
		if err := CheckoutBranch(ctx, dir, branch, "origin/"+branch); err != nil {
			return err
		}
		// Set upstream
		_, err := RunGitCommand(ctx, dir, "branch", "--set-upstream-to=origin/"+branch, branch)
		return err
	}

	// Default fetch and pull
	if err := Fetch(ctx, dir, ""); err != nil {
		return err
	}
	_, err := RunGitCommand(ctx, dir, "pull", "--ff-only")
	return err
}

// BranchList lists all local branches.
func BranchList(ctx context.Context, dir string) ([]string, error) {
	out, err := RunGitCommand(ctx, dir, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
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
