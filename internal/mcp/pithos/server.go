package pithos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var execCommand = exec.CommandContext

// SetExecCommand sets the execCommand variable for mocking in tests.
func SetExecCommand(f func(context.Context, string, ...string) *exec.Cmd) {
	execCommand = f
}

const (
	projectPathSchema = `{
		"type": "object",
		"properties": {
			"project_path": {
				"type": "string",
				"description": "The absolute path to the Pithos project directory."
			}
		},
		"required": ["project_path"]
	}`

	initiateSchema = `{
		"type": "object",
		"properties": {
			"project_path": {
				"type": "string",
				"description": "The absolute path to the Pithos project directory."
			},
			"theme": {
				"type": "string",
				"description": "The theme for the new project. Optional."
			}
		},
		"required": ["project_path"]
	}`
)

// SetupServer creates and configures the pithos MCP server.
func SetupServer() (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-pithos",
		Version: "1.0.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "pithos_initiate",
		Description: "Initializes a new Pithos project at the specified path.",
		InputSchema: json.RawMessage(initiateSchema),
	}, handleInitiate())

	srv.AddTool(&mcp.Tool{
		Name:        "pithos_brew",
		Description: "Runs the Pithos brew stage to generate book content.",
		InputSchema: json.RawMessage(projectPathSchema),
	}, handleStage("brew"))

	srv.AddTool(&mcp.Tool{
		Name:        "pithos_assemble",
		Description: "Runs the Pithos assemble stage to compile PDF/EPUB.",
		InputSchema: json.RawMessage(projectPathSchema),
	}, handleStage("assemble"))

	srv.AddTool(&mcp.Tool{
		Name:        "pithos_deploy",
		Description: "Runs the Pithos deploy stage.",
		InputSchema: json.RawMessage(projectPathSchema),
	}, handleStage("deploy"))

	return srv, nil
}

func handleInitiate() func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			ProjectPath string `json:"project_path"`
			Theme       string `json:"theme"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.ProjectPath == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "project_path is required"}},
			}, nil
		}

		if !filepath.IsAbs(args.ProjectPath) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "project_path must be an absolute path"}},
			}, nil
		}

		if err := checkSandbox(args.ProjectPath); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}

		cmdArgs := []string{"initiate", "--dir", args.ProjectPath}
		if args.Theme != "" {
			cmdArgs = append(cmdArgs, "--theme", args.Theme)
		}

		return runPithosCommand(ctx, cmdArgs)
	}
}

func handleStage(stage string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			ProjectPath string `json:"project_path"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.ProjectPath == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "project_path is required"}},
			}, nil
		}

		if !filepath.IsAbs(args.ProjectPath) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "project_path must be an absolute path"}},
			}, nil
		}

		if err := checkSandbox(args.ProjectPath); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}

		info, err := os.Stat(args.ProjectPath)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("project_path does not exist or is inaccessible: %v", err)}},
			}, nil
		}
		if !info.IsDir() {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "project_path must be a directory"}},
			}, nil
		}

		cmdArgs := []string{stage, "--dir", args.ProjectPath}
		return runPithosCommand(ctx, cmdArgs)
	}
}

func runPithosCommand(ctx context.Context, args []string) (*mcp.CallToolResult, error) {
	//nolint:gosec // G204: Pithos is expected to be a trusted executable in the environment PATH
	cmd := execCommand(ctx, "pithos", args...)
	out, err := cmd.CombinedOutput()

	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("pithos command failed: %v\nOutput:\n%s", err, string(out))}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("pithos command succeeded.\nOutput:\n%s", string(out))}},
	}, nil
}

func checkSandbox(requestedPath string) error {
	workspaceRoot := os.Getenv("POWERWORD_WORKSPACE_ROOT")
	if workspaceRoot == "" {
		return fmt.Errorf("access denied: POWERWORD_WORKSPACE_ROOT environment variable is not set")
	}

	cleanRoot, err := resolvePath(workspaceRoot)
	if err != nil {
		return fmt.Errorf("access denied: could not resolve workspace root: %v", err)
	}

	evalReq, err := resolvePath(requestedPath)
	if err != nil {
		return fmt.Errorf("access denied: could not resolve path for %s: %v", requestedPath, err)
	}

	// Add trailing separator to root to prevent prefix matching issues (e.g. /my/workspace matching /my/workspace2)
	rootWithSep := cleanRoot
	if !strings.HasSuffix(rootWithSep, string(filepath.Separator)) {
		rootWithSep += string(filepath.Separator)
	}

	if !strings.HasPrefix(evalReq, rootWithSep) && evalReq != cleanRoot {
		return fmt.Errorf("access denied: path %s is outside of workspace root %s", requestedPath, workspaceRoot)
	}
	return nil
}

func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	current := abs
	var parts []string
	for {
		eval, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(parts) - 1; i >= 0; i-- {
				eval = filepath.Join(eval, parts[i])
			}
			return filepath.Clean(eval), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		parts = append(parts, filepath.Base(current))
		current = parent
	}

	return filepath.Clean(abs), nil
}
