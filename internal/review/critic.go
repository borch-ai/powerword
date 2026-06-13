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

	internalmcp "github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/linter"

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
var ExtractGitDiff = internalmcp.ExtractGitDiff

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

//nolint:gocognit,funlen,nestif
func VerifyWorkspace(ctx context.Context, plan *Plan, cfg *config.Config) error {
	if plan == nil || cfg == nil {
		return errors.New("VerifyWorkspace requires non-nil plan and cfg")
	}

	if err := linter.ValidatePlans(".", cfg); err != nil {
		return err
	}

	validationCmd := ""
	if checkMakefileExists() {
		validationCmd = "make all"
		if !cfg.EnableCritic {
			fmt.Println("Running local validation (make all) directly...")
		} else {
			fmt.Println("Running local validation (make all) via MCP pw-mcp-critic...")
		}
	} else {
		fmt.Println("No Makefile found, skipping local validation")
	}

	if !cfg.EnableCritic {
		fmt.Println("Critic LLM review is disabled in config. Skipping LLM review.")
		if validationCmd != "" {
			fields := strings.Fields(validationCmd)
			if len(fields) > 0 {
				valCtx, valCancel := context.WithTimeout(ctx, 3*time.Minute)
				defer valCancel()
				//nolint:gosec // execution is explicitly requested by the CLI configuration
				cmd := execCommand(valCtx, fields[0], fields[1:]...)
				cmd.Dir = "."
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("local validation command failed: %w\nOutput:\n%s", err, string(out))
				}
				fmt.Println(string(out))
			}
		}
		return nil
	}

	srvCfg, ok := cfg.Servers["critic"]
	if !ok {
		cmd := "pw-mcp-critic"
		if _, err := os.Stat("bin/pw-mcp-critic"); err == nil {
			cmd = "./bin/pw-mcp-critic"
		} else if _, err := exec.LookPath("pw-mcp-critic"); err != nil {
			return fmt.Errorf("pw-mcp-critic not found in bin/ or PATH, run 'make all' first")
		}
		srvCfg = config.ServerConfig{
			Command: cmd,
			Args:    []string{},
		}
	}

	srv, err := internalmcp.NewServerProcess(ctx, "critic", srvCfg)
	if err != nil {
		return fmt.Errorf("failed to start MCP critic server (%s): %w", srvCfg.Command, err)
	}
	defer func() {
		_ = srv.GracefulShutdown(time.Second * 5)
	}()

	planContent := fmt.Sprintf("## Goal\n%s\n\n## Proposed Changes\n%s\n\n## Verification Plan\n%s\n",
		plan.Goal, plan.Changes, plan.Verification)

	fmt.Println("Analyzing changes with local critic via MCP...")
	res, err := srv.Client().CallTool(ctx, "review_workspace", map[string]interface{}{
		"plan_content":       planContent,
		"validation_command": validationCmd,
	})
	if err != nil {
		return fmt.Errorf("critic analysis tool call failed: %w", err)
	}
	if res.IsError {
		var errMsg string
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcpsdk.TextContent); ok {
				errMsg = tc.Text
			}
		}
		if errMsg == "" {
			errMsg = "critic server returned error"
		}
		_ = os.WriteFile(".powerword-critic.md", []byte(fmt.Sprintf("# Critic Error\n\n%s\n", errMsg)), 0600)
		return fmt.Errorf("critic analysis failed: %s", errMsg)
	}

	var criticOutput string
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcpsdk.TextContent); ok {
			criticOutput = tc.Text
		}
	}

	fmt.Println("\nCritic Feedback:")
	fmt.Println(criticOutput)

	_ = os.WriteFile(".powerword-critic.md", []byte(fmt.Sprintf("# Critic Feedback\n\n%s\n", criticOutput)), 0600)

	trimmedOutput := strings.TrimSpace(criticOutput)
	trimmedOutput = strings.Trim(trimmedOutput, "*_`\"'\n\r\t")

	if !strings.HasSuffix(trimmedOutput, "VERDICT: ACCEPT") {
		return fmt.Errorf("critic rejected the workspace changes")
	}

	_ = os.Remove(".powerword-critic.md")
	return nil
}
