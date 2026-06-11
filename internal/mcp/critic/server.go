package critic

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

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
			"description": "The shell command to execute local validation (e.g. 'make all'). Optional."
		}
	},
	"required": ["plan_content"]
}`

// SetupServer creates and configures the critic MCP server.
func SetupServer(workspaceRoot string, cfg *config.Config) (*mcp.Server, error) {
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

//nolint:gocognit,nestif
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
		var validationOutput string
		if args.ValidationCommand != "" {
			fields := strings.Fields(args.ValidationCommand)
			if len(fields) > 0 {
				//nolint:gosec // execution is explicitly requested by the orchestrator/tool call
				cmd := execCommand(ctx, fields[0], fields[1:]...)
				cmd.Dir = workspaceRoot
				out, err := cmd.CombinedOutput()
				if err != nil {
					validationOutput = fmt.Sprintf("Validation command failed: %v\nOutput:\n%s", err, string(out))
				} else {
					validationOutput = string(out)
				}
			}
		}

		// 2. Extract git diff using mockable ExtractGitDiff variable
		diffStr, err := ExtractGitDiff(ctx, cfg)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to extract git diff: %v", err)}},
			}, nil
		}

		diffStr = strings.TrimSpace(diffStr)
		if len(diffStr) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "No local changes found to review.\nVERDICT: ACCEPT"}},
			}, nil
		}

		// 3. Construct prompt
		prompt := fmt.Sprintf(`You are a strict code reviewer. Review the following workspace diff against the implementation plan.

Implementation Plan:
%s

Local Validation Output:
%s

Git Diff:
%s

Check if ALL proposed changes are implemented in the diff. Check for any omissions, bugs, or missing tests.
If there are any missing changes or issues, clearly list them and end your response with exactly "VERDICT: REJECT".
If the diff fully implements the plan correctly, end your response with exactly "VERDICT: ACCEPT".`,
			args.PlanContent, validationOutput, diffStr)

		messages := []llm.Message{
			{Role: llm.RoleSystem, Content: "You are an automated pre-push code critic."},
			{Role: llm.RoleUser, Content: prompt},
		}

		// 4. Call LLM
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
}
