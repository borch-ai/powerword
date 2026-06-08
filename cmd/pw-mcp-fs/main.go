package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func checkSandbox(absRoot, target string) (string, error) {
	absPath, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) && absPath != absRoot {
		return "", fmt.Errorf("path %s is outside of workspace root %s", absPath, absRoot)
	}
	return absPath, nil
}

func setupServer(workspaceRoot string) (*mcp.Server, error) {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute workspace root: %w", err)
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-fs",
		Version: "1.0.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "read_file",
		Description: "Reads the content of a file",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute or relative path to the file"}},"required":["path"]}`),
	}, handleReadFile(absRoot))

	srv.AddTool(&mcp.Tool{
		Name:        "write_file",
		Description: "Writes content to a file",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute or relative path to the file"},"content":{"type":"string","description":"Content to write to the file"}},"required":["path","content"]}`),
	}, handleWriteFile(absRoot))

	srv.AddTool(&mcp.Tool{
		Name:        "list_directory",
		Description: "Lists the contents of a directory",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute or relative path to the directory"}},"required":["path"]}`),
	}, handleListDirectory(absRoot))

	srv.AddTool(&mcp.Tool{
		Name:        "search_grep",
		Description: "Searches for a regular expression pattern within files in a directory",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Regular expression pattern to search for"},"path":{"type":"string","description":"Absolute or relative path to the directory to search"}},"required":["pattern","path"]}`),
	}, handleSearchGrep(absRoot))

	return srv, nil
}

func handleReadFile(absRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		absPath, err := checkSandbox(absRoot, args.Path)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		//nolint:gosec // path is validated by checkSandbox
		content, err := os.ReadFile(absPath)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(content)}}}, nil
	}
}

func handleWriteFile(absRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		absPath, err := checkSandbox(absRoot, args.Path)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0750); err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		if err := os.WriteFile(absPath, []byte(args.Content), 0600); err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully wrote to %s", absPath)}}}, nil
	}
}

func handleListDirectory(absRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		absPath, err := checkSandbox(absRoot, args.Path)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		var out strings.Builder
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			indicator := ""
			if entry.IsDir() {
				indicator = "/"
			}
			fmt.Fprintf(&out, "%s%s (size: %d bytes, modtime: %s)\n", entry.Name(), indicator, info.Size(), info.ModTime().Format("2006-01-02 15:04:05"))
		}
		if out.Len() == 0 {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Directory is empty"}}}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.String()}}}, nil
	}
}

func searchFile(p, absPath string, re *regexp.Regexp, results *strings.Builder, matchCount *int, maxMatches int) error {
	//nolint:gosec // path is generated by filepath.WalkDir
	file, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(io.LimitReader(file, 10*1024*1024))
	if err != nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if re.MatchString(line) {
			relPath, _ := filepath.Rel(absPath, p)
			fmt.Fprintf(results, "%s:%d: %s\n", relPath, i+1, strings.TrimSpace(line))
			*matchCount++
			if *matchCount >= maxMatches {
				fmt.Fprintf(results, "\n... Output truncated after %d matches ...", maxMatches)
				break
			}
		}
	}
	return nil
}

func walkDirFunc(absPath string, re *regexp.Regexp, results *strings.Builder, matchCount *int, maxMatches int) fs.WalkDirFunc {
	return func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if *matchCount >= maxMatches {
			return nil
		}
		return searchFile(p, absPath, re, results, matchCount, maxMatches)
	}
}

func handleSearchGrep(absRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid regular expression: %v", err)}}}, nil
		}

		absPath, err := checkSandbox(absRoot, args.Path)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		var results strings.Builder
		matchCount := 0
		maxMatches := 100

		err = filepath.WalkDir(absPath, walkDirFunc(absPath, re, &results, &matchCount, maxMatches))
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}

		if matchCount == 0 {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No matches found"}}}, nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: results.String()}}}, nil
	}
}
