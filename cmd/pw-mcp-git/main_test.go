package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func setupTestRepo(t *testing.T, dir string) *git.Worktree {
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	testFile := filepath.Join(dir, "test.txt")
	_ = os.WriteFile(testFile, []byte("hello world"), 0600)

	if _, addErr := wt.Add("test.txt"); addErr != nil {
		t.Fatalf("failed to add to worktree: %v", addErr)
	}

	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	_ = os.WriteFile(testFile, []byte("hello universe"), 0600)
	return wt
}

func testGitStatus(t *testing.T, ctx context.Context, session *mcp.ClientSession) {
	t.Run("git_status", func(t *testing.T) {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "git_status",
			Arguments: map[string]interface{}{},
		})
		if err != nil || res.IsError {
			t.Fatalf("git_status failed: %v", err)
		}
		if res.Content[0].(*mcp.TextContent).Text == "" {
			t.Errorf("expected status output, got empty string")
		}
	})
}

func testGitLog(t *testing.T, ctx context.Context, session *mcp.ClientSession) {
	t.Run("git_log", func(t *testing.T) {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "git_log",
			Arguments: map[string]interface{}{"limit": 1},
		})
		if err != nil || res.IsError {
			t.Fatalf("git_log failed: %v", err)
		}
		if res.Content[0].(*mcp.TextContent).Text == "" {
			t.Errorf("expected log output, got empty string")
		}
	})
}

func testGitDiff(t *testing.T, ctx context.Context, session *mcp.ClientSession) {
	t.Run("git_diff", func(t *testing.T) {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "git_diff",
			Arguments: map[string]interface{}{},
		})
		if err != nil || res.IsError {
			t.Fatalf("git_diff failed: %v", err)
		}
		content := res.Content[0].(*mcp.TextContent).Text
		if content == "" || content == "No changes" {
			t.Errorf("expected diff output, got %q", content)
		}
	})
}

func testGitCommit(t *testing.T, ctx context.Context, session *mcp.ClientSession, wt *git.Worktree) {
	t.Run("git_commit", func(t *testing.T) {
		if _, addErr := wt.Add("test.txt"); addErr != nil {
			t.Fatalf("failed to add to worktree: %v", addErr)
		}
		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "git_commit",
			Arguments: map[string]interface{}{"message": "test commit"},
		})
		if err != nil || res.IsError {
			t.Fatalf("git_commit failed: %v", err)
		}
		if res.Content[0].(*mcp.TextContent).Text == "" {
			t.Errorf("expected commit output, got empty string")
		}
	})
}

func TestGitMCP(t *testing.T) {
	tempDir := t.TempDir()
	wt := setupTestRepo(t, tempDir)

	srv, err := setupServer(tempDir)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(fmt.Errorf("server run err: %v", runErr))
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	testGitStatus(t, ctx, session)
	testGitLog(t, ctx, session)
	testGitDiff(t, ctx, session)
	testGitCommit(t, ctx, session, wt)
}
