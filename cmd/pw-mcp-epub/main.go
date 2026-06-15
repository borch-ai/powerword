package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/epub"
	"github.com/borch-ai/powerword/pkg/config"
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

	cfgPath := filepath.Join(workspaceRoot, "powerword.toml")
	var cfg *config.Config
	var err error

	//nolint:gosec // cfgPath is constructed from validated workspaceRoot
	if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
		cfg, err = config.LoadConfig("")
		if err != nil {
			cfg = &config.Config{}
		}
	} else {
		cfg, err = config.LoadConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to load configuration from %s: %w", cfgPath, err)
		}
	}

	srv, err := setupServer(workspaceRoot, cfg)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

const compileEpubSchema = `{
	"type": "object",
	"properties": {
		"manuscript_path": {
			"type": "string",
			"description": "Absolute path to the book manuscript (Markdown/text format)."
		},
		"images_dir": {
			"type": "string",
			"description": "Absolute path to the directory containing book illustration images (PNG/JPG)."
		},
		"output_path": {
			"type": "string",
			"description": "Absolute destination path for the compiled .epub file."
		},
		"title": {
			"type": "string",
			"description": "The title of the book."
		},
		"author": {
			"type": "string",
			"description": "The author name."
		},
		"language": {
			"type": "string",
			"description": "Language code (default: 'en')."
		},
		"stylesheet_path": {
			"type": "string",
			"description": "Optional path to a custom CSS file."
		}
	},
	"required": ["manuscript_path", "images_dir", "output_path", "title", "author"]
}`

func setupServer(workspaceRoot string, cfg *config.Config) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-epub",
		Version: "0.1.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "compile_epub",
		Description: "Compiles a book manuscript and illustration assets into a spec-compliant EPUB 3 document.",
		InputSchema: json.RawMessage(compileEpubSchema),
	}, handleCompileEPUB())

	return srv, nil
}

func handleCompileEPUB() func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			ManuscriptPath string `json:"manuscript_path"`
			ImagesDir      string `json:"images_dir"`
			OutputPath     string `json:"output_path"`
			Title          string `json:"title"`
			Author         string `json:"author"`
			Language       string `json:"language"`
			StylesheetPath string `json:"stylesheet_path"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.ManuscriptPath == "" || args.ImagesDir == "" || args.OutputPath == "" || args.Title == "" || args.Author == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "missing required parameters"}},
			}, nil
		}

		opts := epub.CompileOpts{
			ManuscriptPath: args.ManuscriptPath,
			ImagesDir:      args.ImagesDir,
			OutputPath:     args.OutputPath,
			Title:          args.Title,
			Author:         args.Author,
			Language:       args.Language,
			StylesheetPath: args.StylesheetPath,
		}

		if err := epub.CompileEPUB(ctx, opts); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to compile EPUB: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("successfully compiled EPUB to %s", args.OutputPath)}},
		}, nil
	}
}
