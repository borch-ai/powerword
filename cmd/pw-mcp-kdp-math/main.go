package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"powerword/internal/config"
	"powerword/internal/plugins/kdpmath"
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

func setupServer(workspaceRoot string, cfg *config.Config) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-kdp-math",
		Version: "1.0.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "kdp_calculate_geometry",
		Description: "Computes exact book cover, interior page dimensions, safety margins, and spine thickness based on page count, binding type, paper type, and trim size.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"page_count": {
					"type": "integer",
					"description": "Total page count of the book"
				},
				"binding_type": {
					"type": "string",
					"description": "Binding type: 'paperback' or 'hardcover'"
				},
				"paper_type": {
					"type": "string",
					"description": "Paper type: 'white', 'cream', 'standard_color', or 'premium_color'"
				},
				"trim_size": {
					"type": "string",
					"description": "Trim size in inches (e.g. '6x9' or '5.5x8.5')"
				}
			},
			"required": ["page_count", "binding_type", "paper_type", "trim_size"]
		}`),
	}, handleCalculateGeometry())

	srv.AddTool(&mcp.Tool{
		Name:        "kdp_validate_pdf",
		Description: "Parses an existing PDF to inspect its page count, dimensions in points/inches, and validates target bleed line layout compliance for covers or interiors.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pdf_path": {
					"type": "string",
					"description": "Absolute or workspace-relative path to the PDF file"
				},
				"binding_type": {
					"type": "string",
					"description": "Binding type: 'paperback' or 'hardcover'"
				},
				"paper_type": {
					"type": "string",
					"description": "Paper type: 'white', 'cream', 'standard_color', or 'premium_color'"
				},
				"trim_size": {
					"type": "string",
					"description": "Expected trim size in inches (e.g. '6x9')"
				},
				"expected_page_count": {
					"type": "integer",
					"description": "Expected total page count (optional, but highly recommended for interior validation, required for cover validation)"
				},
				"is_cover": {
					"type": "boolean",
					"description": "Set to true if validating a cover PDF spread, false if validating interior pages"
				},
				"has_bleed": {
					"type": "boolean",
					"description": "Set to true if interior pages should include bleed margins"
				}
			},
			"required": ["pdf_path", "binding_type", "paper_type", "trim_size"]
		}`),
	}, handleValidatePDF(workspaceRoot))

	srv.AddTool(&mcp.Tool{
		Name:        "kdp_generate_manifest",
		Description: "Generates structured JSON cover layout templates compatible with BookBolt/Inkfluence configurations.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"page_count": {
					"type": "integer",
					"description": "Total page count of the book"
				},
				"binding_type": {
					"type": "string",
					"description": "Binding type: 'paperback' or 'hardcover'"
				},
				"paper_type": {
					"type": "string",
					"description": "Paper type: 'white', 'cream', 'standard_color', or 'premium_color'"
				},
				"trim_size": {
					"type": "string",
					"description": "Trim size in inches (e.g. '6x9')"
				}
			},
			"required": ["page_count", "binding_type", "paper_type", "trim_size"]
		}`),
	}, handleGenerateManifest())

	return srv, nil
}

func handleCalculateGeometry() func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			PageCount   int    `json:"page_count"`
			BindingType string `json:"binding_type"`
			PaperType   string `json:"paper_type"`
			TrimSize    string `json:"trim_size"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		res, err := kdpmath.CalculateGeometry(args.PageCount, args.BindingType, args.PaperType, args.TrimSize)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to calculate geometry: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleValidatePDF(workspaceRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			PDFPath           string `json:"pdf_path"`
			BindingType       string `json:"binding_type"`
			PaperType         string `json:"paper_type"`
			TrimSize          string `json:"trim_size"`
			ExpectedPageCount int    `json:"expected_page_count"`
			IsCover           bool   `json:"is_cover"`
			HasBleed          bool   `json:"has_bleed"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		path := args.PDFPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(workspaceRoot, path)
		}

		res, err := kdpmath.ValidatePDF(path, args.BindingType, args.PaperType, args.TrimSize, args.ExpectedPageCount, args.IsCover, args.HasBleed)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to validate PDF: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleGenerateManifest() func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			PageCount   int    `json:"page_count"`
			BindingType string `json:"binding_type"`
			PaperType   string `json:"paper_type"`
			TrimSize    string `json:"trim_size"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		manifest, err := kdpmath.GenerateManifest(args.PageCount, args.BindingType, args.PaperType, args.TrimSize)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to generate manifest: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: manifest}},
		}, nil
	}
}
