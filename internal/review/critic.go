package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"powerword/internal/config"
	"powerword/internal/llm"
	internalmcp "powerword/internal/mcp"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Plan struct {
	Goal         string
	Changes      string
	Verification string
}

var execCommand = exec.CommandContext

var checkMakefileExists = func() bool {
	_, err := os.Stat("Makefile")
	return err == nil
}

// ExtractGitDiff is a variable so it can be mocked in tests.
var ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
	srvCfg, ok := cfg.Servers["git"]
	if !ok {
		cmd := "pw-mcp-git"
		if _, err := os.Stat("bin/pw-mcp-git"); err == nil {
			cmd = "./bin/pw-mcp-git"
		} else if _, err := exec.LookPath("pw-mcp-git"); err != nil {
			return "", fmt.Errorf("pw-mcp-git not found in bin/ or PATH, run 'make all' first")
		}
		srvCfg = config.ServerConfig{
			Command: cmd,
			Args:    []string{},
		}
	}

	srv, err := internalmcp.NewServerProcess(ctx, "git", srvCfg)
	if err != nil {
		return "", fmt.Errorf("failed to start MCP git server (%s): %w", srvCfg.Command, err)
	}
	defer func() {
		_ = srv.GracefulShutdown(time.Second * 5)
	}()

	var diffBuilder strings.Builder

	baseBranch := "main"
	// Check if there's an active PR and use its base
	ghCmd := execCommand(ctx, "gh", "pr", "view", "--json", "baseRefName", "--jq", ".baseRefName")
	out, ghErr := ghCmd.Output()
	if ghErr == nil {
		if b := strings.TrimSpace(string(out)); b != "" {
			baseBranch = b
		}
	} else {
		// Fallback to origin/main if main doesn't exist locally but origin/main does
		gitCmd := execCommand(ctx, "git", "rev-parse", "--verify", "-q", "main")
		if gitCmd.Run() != nil {
			gitCmd2 := execCommand(ctx, "git", "rev-parse", "--verify", "-q", "origin/main")
			if gitCmd2.Run() == nil {
				baseBranch = "origin/main"
			}
		}
	}

	fmt.Printf("Using base branch %q for diff extraction...\n", baseBranch)

	// 1. Get committed changes against base branch
	resCommits, err := srv.Client().CallTool(ctx, "git_diff_commits", map[string]interface{}{"base": baseBranch, "head": "HEAD"})
	if err == nil && !resCommits.IsError && len(resCommits.Content) > 0 {
		if tc, ok := resCommits.Content[0].(*mcpsdk.TextContent); ok {
			text := strings.TrimSpace(tc.Text)
			if text != "No changes" && text != "" {
				fmt.Fprintf(&diffBuilder, "Committed changes (%s -> HEAD):\n", baseBranch)
				diffBuilder.WriteString(text)
				diffBuilder.WriteString("\n\n")
			}
		}
	}

	// 2. Get uncommitted working tree changes
	resWorking, err := srv.Client().CallTool(ctx, "git_diff", map[string]interface{}{})
	if err == nil && !resWorking.IsError && len(resWorking.Content) > 0 {
		if tc, ok := resWorking.Content[0].(*mcpsdk.TextContent); ok {
			text := strings.TrimSpace(tc.Text)
			if text != "No changes" && text != "" {
				diffBuilder.WriteString("Uncommitted changes (Working Tree):\n")
				diffBuilder.WriteString(text)
				diffBuilder.WriteString("\n\n")
			}
		}
	}

	if diffBuilder.Len() == 0 {
		return "", nil
	}

	return strings.TrimSpace(diffBuilder.String()), nil
}

func LoadIssuePlan(ctx context.Context, issueID string) (*Plan, error) {
	//nolint:gosec // this CLI is intended to be run by the user locally with explicit issue IDs
	cmd := execCommand(ctx, "gh", "issue", "view", issueID, "--json", "body")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issue (is gh installed and authenticated?): %w", err)
	}

	var data struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("failed to parse gh issue json: %w", err)
	}

	return parseIssueBody(data.Body)
}

func parseIssueBody(body string) (*Plan, error) {
	// Simple regex parsing assuming standard markdown headers. Support both ## and ###
	goalMatches := regexp.MustCompile(`(?is)(?:^|\n)#+\s*Goal\s*(.*?)(?:\n#+|\z)`).FindStringSubmatch(body)
	changesMatches := regexp.MustCompile(`(?is)(?:^|\n)#+\s*Proposed Changes\s*(.*?)(?:\n#+|\z)`).FindStringSubmatch(body)
	verifMatches := regexp.MustCompile(`(?is)(?:^|\n)#+\s*Verification Plan\s*(.*?)(?:\n#+|\z)`).FindStringSubmatch(body)

	plan := &Plan{}

	sanitize := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "_No response_" {
			return ""
		}
		return s
	}

	if len(goalMatches) > 1 {
		plan.Goal = sanitize(goalMatches[1])
	} else {
		// Fallback: Goal is everything before the first header if no explicit Goal header
		fallbackGoal := regexp.MustCompile(`(?is)^(.*?)(?:\n#+|\z)`).FindStringSubmatch(body)
		if len(fallbackGoal) > 1 {
			plan.Goal = sanitize(fallbackGoal[1])
		}
	}

	if len(changesMatches) > 1 {
		plan.Changes = sanitize(changesMatches[1])
	}
	if len(verifMatches) > 1 {
		plan.Verification = sanitize(verifMatches[1])
	}

	if plan.Goal == "" || plan.Changes == "" || plan.Verification == "" {
		return nil, fmt.Errorf("could not parse all required sections from issue body")
	}

	return plan, nil
}

func VerifyWorkspace(ctx context.Context, plan *Plan, cfg *config.Config) error {
	if plan == nil || cfg == nil {
		return errors.New("VerifyWorkspace requires non-nil plan and cfg")
	}

	var validationOutput string
	if checkMakefileExists() {
		fmt.Println("Running local validation (make all)...")

		makeCtx, makeCancel := context.WithTimeout(ctx, 3*time.Minute)
		defer makeCancel()

		makeCmd := execCommand(makeCtx, "make", "all")
		makeOut, err := makeCmd.CombinedOutput()
		if err != nil {
			_ = os.WriteFile(".powerword-critic.md", []byte(fmt.Sprintf("# Local Validation Failed\n\n```\n%s\n```\n", string(makeOut))), 0600)
			return fmt.Errorf("local validation failed: %w", err)
		}
		validationOutput = string(makeOut)
	} else {
		fmt.Println("No Makefile found, skipping local validation")
	}

	fmt.Println("Extracting git diff via MCP pw-mcp-git...")
	diffStr, err := ExtractGitDiff(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to extract git diff: %w", err)
	}

	diffStr = strings.TrimSpace(diffStr)
	if len(diffStr) == 0 {
		fmt.Println("No local changes found to review.")
		_ = os.Remove(".powerword-critic.md") // clean up old feedback
		return nil
	}

	client, err := llm.NewCriticClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize critic LLM client: %w", err)
	}

	prompt := fmt.Sprintf(`You are a strict code reviewer. Review the following workspace diff against the implementation plan.

Implementation Plan Goal:
%s

Implementation Plan Proposed Changes:
%s

Implementation Plan Verification:
%s

Local Validation Output:
%s

Git Diff:
%s

Check if ALL proposed changes are implemented in the diff. Check for any omissions, bugs, or missing tests.
If there are any missing changes or issues, clearly list them and end your response with exactly "VERDICT: REJECT".
If the diff fully implements the plan correctly, end your response with exactly "VERDICT: ACCEPT".`,
		plan.Goal, plan.Changes, plan.Verification, validationOutput, diffStr)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are an automated pre-push code critic."},
		{Role: llm.RoleUser, Content: prompt},
	}

	fmt.Println("Analyzing changes with local critic...")
	resp, err := client.Generate(ctx, messages, nil)
	if err != nil {
		return fmt.Errorf("critic analysis failed: %w", err)
	}

	fmt.Println("\nCritic Feedback:")
	fmt.Println(resp.Content)

	_ = os.WriteFile(".powerword-critic.md", []byte(fmt.Sprintf("# Critic Feedback\n\n%s\n", resp.Content)), 0600)

	if !strings.HasSuffix(strings.TrimSpace(resp.Content), "VERDICT: ACCEPT") {
		return fmt.Errorf("critic rejected the workspace changes")
	}

	_ = os.Remove(".powerword-critic.md")
	return nil
}
