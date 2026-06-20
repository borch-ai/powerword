package critic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	internalmcp "github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/llm"
)

var execCommand = exec.CommandContext

// ExtractGitDiff is a variable so it can be mocked in tests.
var ExtractGitDiff = internalmcp.ExtractGitDiff

// SetExecCommand sets the execCommand variable for mocking in tests.
func SetExecCommand(f func(context.Context, string, ...string) *exec.Cmd) {
	execCommand = f
}

const reviewWorkspaceSchema = `{
	"type": "object",
	"properties": {
		"plan_content": {
			"type": "string",
			"description": "The markdown content of the implementation plan to review the changes against."
		},
		"validation_command": {
			"type": "string",
			"description": "The command to execute local validation (split by whitespace, executed directly without a shell, e.g. 'make all'). Optional."
		}
	},
	"required": ["plan_content"]
}`

// SetupServer creates and configures the critic MCP server.
func SetupServer(workspaceRoot string, cfg *config.Config) (*mcp.Server, error) {
	if cfg == nil {
		return nil, errors.New("SetupServer requires non-nil configuration")
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-critic",
		Version: "1.0.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "review_workspace",
		Description: "Reviews workspace changes against an implementation plan, running optional validation commands.",
		InputSchema: json.RawMessage(reviewWorkspaceSchema),
	}, handleReviewWorkspace(workspaceRoot, cfg))

	return srv, nil
}

func handleReviewWorkspace(workspaceRoot string, cfg *config.Config) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			PlanContent       string `json:"plan_content"`
			ValidationCommand string `json:"validation_command"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.PlanContent == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "plan_content is required"}},
			}, nil
		}

		// 1. Run validation command if provided
		validationOutput, valErr := runValidationCommand(ctx, workspaceRoot, args.ValidationCommand)
		if valErr != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: valErr.Error()}},
			}, nil
		}

		// 2. Extract git diff
		diffStr, diffResult := extractWorkspaceDiff(ctx, cfg)
		if diffResult != nil {
			return diffResult, nil
		}

		// 3. Call LLM critic
		return callCriticLLM(ctx, cfg, args.PlanContent, validationOutput, diffStr)
	}
}

// runValidationCommand executes the optional validation command and returns its output.
// Returns an error result string if validation failed (not a Go error — caller returns it as a tool result).
func runValidationCommand(ctx context.Context, workspaceRoot, validationCommand string) (string, error) {
	if validationCommand == "" {
		return "", nil
	}
	fields := strings.Fields(validationCommand)
	if len(fields) == 0 {
		return "", nil
	}
	valCtx, valCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer valCancel()
	//nolint:gosec // G204: validationCommand is "make all" requested explicitly by the orchestrator/CLI configuration
	cmd := execCommand(valCtx, fields[0], fields[1:]...)
	cmd.Dir = workspaceRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("validation command failed: %v\nOutput:\n%s\nVERDICT: REJECT", err, string(out))
	}
	return string(out), nil
}

// extractWorkspaceDiff extracts the current git diff and returns it.
// If an early-exit tool result should be returned, the second return value is non-nil.
func extractWorkspaceDiff(ctx context.Context, cfg *config.Config) (string, *mcp.CallToolResult) {
	diffStr, err := ExtractGitDiff(ctx, cfg)
	if err != nil {
		return "", &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to extract git diff: %v", err)}},
		}
	}

	diffStr = strings.TrimSpace(diffStr)
	if len(diffStr) == 0 {
		return "", &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "No local changes found to review.\nVERDICT: ACCEPT"}},
		}
	}

	// Prevent 413 Payload Too Large errors by truncating extremely large diffs
	const maxDiffLen = 100000 // roughly 25k tokens
	if len(diffStr) > maxDiffLen {
		diffStr = diffStr[:maxDiffLen] + "\n\n... [diff truncated due to size limits]"
	}
	return diffStr, nil
}

// callCriticLLM builds the review prompt and invokes the critic LLM.
func callCriticLLM(ctx context.Context, cfg *config.Config, planContent, validationOutput, diffStr string) (*mcp.CallToolResult, error) {
	prompt := fmt.Sprintf(`You are a strict code reviewer. Review the following workspace diff against the implementation plan.

Implementation Plan:
%s

Local Validation Output:
%s

Git Diff:
%s

Check if ALL proposed changes are implemented in the diff. Check for any omissions, bugs, or missing tests.
Important: The Git Diff may contain new or modified plan files under the "plans/" directory. If a plan file in the diff has "Status: Open" or is not marked as "Completed", do not expect the code changes described in that plan to be implemented in this diff. Only expect code changes to be implemented for plans that are marked as "Completed".
If the Local Validation Output indicates a failure (e.g. compile or test errors), you MUST reject the changes.
If there are any missing changes or issues, clearly list them and end your response with exactly "VERDICT: REJECT".
If the diff fully implements the plan correctly and all validations pass, end your response with exactly "VERDICT: ACCEPT".`,
		planContent, validationOutput, diffStr)

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are an automated pre-push code critic."},
		{Role: llm.RoleUser, Content: prompt},
	}

	client, err := llm.NewCriticClient(cfg)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to initialize critic LLM client: %v", err)}},
		}, nil
	}

	resp, err := client.Generate(ctx, messages, nil)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("critic analysis failed: %v", err)}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: resp.Content}},
	}, nil
}
