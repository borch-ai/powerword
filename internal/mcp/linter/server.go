package linter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/linter"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const lintPlansSchema = `{
	"type": "object",
	"properties": {
		"workspace_root": {
			"type": "string",
			"description": "Optional absolute or relative path to the workspace root containing plans/. Defaults to POWERWORD_WORKSPACE_ROOT env var or current directory."
		},
		"plan_template_path": {
			"type": "string",
			"description": "Optional custom path to the plan template file"
		}
	}
}`

var (
	reLine   = regexp.MustCompile(`^(.*\.md):(\d+):\s*(.*)$`)
	reNoLine = regexp.MustCompile(`^(.*\.md):\s*(.*)$`)
)

type lintPlansArgs struct {
	WorkspaceRoot    string `json:"workspace_root"`
	PlanTemplatePath string `json:"plan_template_path"`
}

type lintPlansResult struct {
	Valid  bool        `json:"valid"`
	Errors []lintError `json:"errors"`
}

type lintError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
	Rule    string `json:"rule"`
}

// SetupServer initializes the MCP server for plan validation.
func SetupServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-linter",
		Version: "0.1.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "lint_plans",
		Description: "Statically validate workspace implementation plans against structured template constraints and file link accessibility.",
		InputSchema: json.RawMessage(lintPlansSchema),
	}, handleLintPlans)

	return srv
}

func handleLintPlans(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args lintPlansArgs
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	cfg := &config.Config{}
	if args.PlanTemplatePath != "" {
		cfg.PlanTemplate = args.PlanTemplatePath
	}

	workspaceRoot := args.WorkspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = os.Getenv("POWERWORD_WORKSPACE_ROOT")
	}
	if workspaceRoot == "" {
		workspaceRoot = "."
	}

	var validationErrs []string
	err := linter.ValidatePlans(workspaceRoot, cfg)
	if err != nil {
		var valErr *linter.PlanValidationError
		if errors.As(err, &valErr) {
			validationErrs = valErr.Errors
		} else {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("System validation error: %v", err)}},
			}, nil
		}
	}

	result := lintPlansResult{
		Valid:  len(validationErrs) == 0,
		Errors: make([]lintError, 0, len(validationErrs)),
	}

	for _, errStr := range validationErrs {
		result.Errors = append(result.Errors, parseValidationError(errStr))
	}

	data, jErr := json.MarshalIndent(result, "", "  ")
	if jErr != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", jErr)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil
}

func parseValidationError(errStr string) lintError {
	if matches := reLine.FindStringSubmatch(errStr); len(matches) > 3 {
		file := matches[1]
		line, _ := strconv.Atoi(matches[2])
		msg := matches[3]
		rule := "plan_structure"
		lowerMsg := strings.ToLower(msg)
		switch {
		case strings.Contains(lowerMsg, "relative"):
			rule = "relative_links"
		case strings.Contains(lowerMsg, "link label"):
			rule = "link_label"
		case strings.Contains(lowerMsg, "does not exist"):
			rule = "missing_files"
		}
		return lintError{
			File:    file,
			Line:    line,
			Message: msg,
			Rule:    rule,
		}
	}

	if matches := reNoLine.FindStringSubmatch(errStr); len(matches) > 2 {
		file := matches[1]
		msg := matches[2]
		return lintError{
			File:    file,
			Line:    0,
			Message: msg,
			Rule:    "plan_structure",
		}
	}

	return lintError{
		File:    "",
		Line:    0,
		Message: errStr,
		Rule:    "unknown",
	}
}
