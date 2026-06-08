package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	workspaceRoot := os.Getenv("POWERWORD_WORKSPACE_ROOT")
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get cwd: %w", err)
		}
		workspaceRoot = cwd
	}

	srv, err := setupServer(workspaceRoot)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

func getRepo(workspaceRoot string) (*git.Repository, error) {
	repo, err := git.PlainOpen(workspaceRoot)
	if err != nil {
		repo, err = git.PlainOpenWithOptions(workspaceRoot, &git.PlainOpenOptions{DetectDotGit: true})
		if err != nil {
			return nil, fmt.Errorf("failed to open git repo at %s: %w", workspaceRoot, err)
		}
	}
	return repo, nil
}

func handleGitStatus(workspaceRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := getRepo(workspaceRoot)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		wt, err := repo.Worktree()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		status, err := wt.Status()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: status.String()}}}, nil
	}
}

func handleGitLog(workspaceRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Limit int `json:"limit"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		repo, err := getRepo(workspaceRoot)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		cIter, err := repo.Log(&git.LogOptions{})
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		var output strings.Builder
		count := 0
		err = cIter.ForEach(func(c *object.Commit) error {
			if args.Limit > 0 && count >= args.Limit {
				return storer.ErrStop
			}
			fmt.Fprintf(&output, "commit %s\nAuthor: %s <%s>\nDate:   %s\n\n    %s\n",
				c.Hash, c.Author.Name, c.Author.Email, c.Author.When.Format(time.RFC3339), strings.TrimSpace(c.Message))
			count++
			return nil
		})

		if err != nil && err != storer.ErrStop {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: output.String()}}}, nil
	}
}

func generateDiff(status git.Status, tree *object.Tree, wt *git.Worktree) string {
	var output strings.Builder
	dmp := diffmatchpatch.New()

	for path, fileStatus := range status {
		isModified := fileStatus.Worktree == git.Modified || fileStatus.Staging == git.Modified
		isAdded := fileStatus.Worktree == git.Added || fileStatus.Staging == git.Added
		isDeleted := fileStatus.Worktree == git.Deleted || fileStatus.Staging == git.Deleted

		if !isModified && !isAdded && !isDeleted {
			continue
		}

		var originalContent string
		if !isAdded {
			file, err := tree.File(path)
			if err == nil {
				originalContent, _ = file.Contents()
			}
		}

		var newContent string
		if !isDeleted {
			//nolint:gosec // path is generated from git status
			b, err := os.ReadFile(filepath.Join(wt.Filesystem.Root(), path))
			if err == nil {
				newContent = string(b)
			}
		}

		diffs := dmp.DiffMain(originalContent, newContent, false)
		patch := dmp.PatchMake(diffs)
		patchText := dmp.PatchToText(patch)

		if patchText != "" {
			fmt.Fprintf(&output, "--- a/%s\n+++ b/%s\n%s\n", path, path, patchText)
		}
	}
	return output.String()
}

func handleGitDiff(workspaceRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		repo, err := getRepo(workspaceRoot)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		wt, err := repo.Worktree()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		status, err := wt.Status()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		head, err := repo.Head()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		commit, err := repo.CommitObject(head.Hash())
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		tree, err := commit.Tree()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		diffText := generateDiff(status, tree, wt)
		if diffText == "" {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No changes"}}}, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: diffText}}}, nil
	}
}

func handleGitCommit(workspaceRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		repo, err := getRepo(workspaceRoot)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		wt, err := repo.Worktree()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		cfg, err := repo.Config()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		name := cfg.User.Name
		email := cfg.User.Email
		if name == "" {
			name = "Powerword Agent"
		}
		if email == "" {
			email = "agent@powerword.dev"
		}

		commit, err := wt.Commit(args.Message, &git.CommitOptions{
			Author: &object.Signature{
				Name:  name,
				Email: email,
				When:  time.Now(),
			},
		})
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Committed successfully: %s", commit.String())}}}, nil
	}
}

func setupServer(workspaceRoot string) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-git",
		Version: "1.0.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "git_status",
		Description: "Returns the working tree status",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, handleGitStatus(workspaceRoot))

	srv.AddTool(&mcp.Tool{
		Name:        "git_log",
		Description: "Returns the commit history",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","description":"Max number of commits to return"}},"required":["limit"]}`),
	}, handleGitLog(workspaceRoot))

	srv.AddTool(&mcp.Tool{
		Name:        "git_diff",
		Description: "Returns the diff of the working tree",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, handleGitDiff(workspaceRoot))

	srv.AddTool(&mcp.Tool{
		Name:        "git_commit",
		Description: "Commits staged changes. You must stage files first, or this will only commit already staged files.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string","description":"Commit message"}},"required":["message"]}`),
	}, handleGitCommit(workspaceRoot))

	return srv, nil
}
