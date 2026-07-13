//go:build integration

package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupMockRemote creates a local file-based Git repository to serve as a remote.
// It initializes the repo, adds a dummy file, and creates an initial commit on "main".
func setupMockRemote(t *testing.T) string {
	t.Helper()
	remoteDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init", "--initial-branch=main")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init remote repo: %v", err)
	}

	// Configure git for committing
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = remoteDir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = remoteDir
	_ = cmd.Run()

	// Create a dummy file and commit
	dummyFile := filepath.Join(remoteDir, "README.md")
	if err := os.WriteFile(dummyFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = remoteDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	return remoteDir
}

func TestIntegration_Clone(t *testing.T) {
	remoteURL := setupMockRemote(t)
	cloneDir := t.TempDir()

	// Perform clone
	err := Clone(context.Background(), remoteURL, cloneDir, "main", 1)
	if err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(cloneDir, "README.md")); os.IsNotExist(err) {
		t.Errorf("expected README.md to exist in cloned repo")
	}
}

func TestIntegration_BranchAndCheckout(t *testing.T) {
	remoteURL := setupMockRemote(t)
	cloneDir := t.TempDir()

	err := Clone(context.Background(), remoteURL, cloneDir, "main", 1)
	if err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	// Create and checkout new branch
	err = Branch(context.Background(), cloneDir, "feature-branch", "")
	if err != nil {
		t.Fatalf("Branch failed: %v", err)
	}

	err = CheckoutBranch(context.Background(), cloneDir, "feature-branch", "")
	if err != nil {
		t.Fatalf("CheckoutBranch failed: %v", err)
	}

	// Verify branch list
	branches, err := BranchList(context.Background(), cloneDir)
	if err != nil {
		t.Fatalf("BranchList failed: %v", err)
	}

	found := false
	for _, b := range branches {
		if b == "feature-branch" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'feature-branch' in branch list, got: %v", branches)
	}
}

func TestIntegration_FetchAndPull(t *testing.T) {
	remoteURL := setupMockRemote(t)
	cloneDir := t.TempDir()

	err := Clone(context.Background(), remoteURL, cloneDir, "main", 1)
	if err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	// Add a new commit to the remote
	newFile := filepath.Join(remoteURL, "update.txt")
	if err := os.WriteFile(newFile, []byte("update"), 0644); err != nil {
		t.Fatalf("failed to write new file: %v", err)
	}
	cmd := exec.Command("git", "add", "update.txt")
	cmd.Dir = remoteURL
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Update commit")
	cmd.Dir = remoteURL
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to commit update: %v", err)
	}

	// Fetch and pull in clone
	err = Fetch(context.Background(), cloneDir, "")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	err = Pull(context.Background(), cloneDir, "main")
	if err != nil {
		t.Fatalf("Pull failed: %v", err)
	}

	// Verify new file exists in clone
	if _, err := os.Stat(filepath.Join(cloneDir, "update.txt")); os.IsNotExist(err) {
		t.Errorf("expected update.txt to exist after pull")
	}
}
