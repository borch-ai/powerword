package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/pdfcheck"
	"github.com/borch-ai/powerword/pkg/config"
)

var osExit = os.Exit

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		osExit(1)
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

const validatePDFSchema = `{
	"type": "object",
	"properties": {
		"pdf_path": {
			"type": "string",
			"description": "Absolute or relative path to the target PDF file to validate."
		},
		"expected_width_inches": {
			"type": "number",
			"description": "Expected width of the page in inches (trim width)."
		},
		"expected_height_inches": {
			"type": "number",
			"description": "Expected height of the page in inches (trim height)."
		},
		"bleed_inches": {
			"type": "number",
			"description": "Optional bleed offset. Defaults to 0.125 inches."
		},
		"min_dpi": {
			"type": "integer",
			"description": "Optional minimum image resolution required. Defaults to 300 DPI."
		},
		"enforce_embedded_fonts": {
			"type": "boolean",
			"description": "Optional enforcement that all referenced fonts are embedded. Defaults to true."
		},
		"enforce_cmyk": {
			"type": "boolean",
			"description": "Optional enforcement that all images use CMYK color space. Defaults to false."
		},
		"enforce_grayscale": {
			"type": "boolean",
			"description": "Optional enforcement that all elements and images use grayscale color spaces. Defaults to false."
		},
		"min_gutter_inches": {
			"type": "number",
			"minimum": 0,
			"description": "Optional minimum inside gutter margin in inches."
		},
		"min_margin_inches": {
			"type": "number",
			"minimum": 0,
			"description": "Optional minimum top, bottom, and outer margin in inches."
		}
	},
	"required": ["pdf_path", "expected_width_inches", "expected_height_inches"]
}`

func setupServer(workspaceRoot string, cfg *config.Config) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-pdfcheck",
		Version: "0.1.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "validate_pdf",
		Description: "Performs preflight validation checks on a compiled PDF book manuscript to ensure KDP paperback compliance.",
		InputSchema: json.RawMessage(validatePDFSchema),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleValidatePDF(ctx, req, workspaceRoot)
	})

	return srv, nil
}

func handleValidatePDF(ctx context.Context, req *mcp.CallToolRequest, workspaceRoot string) (*mcp.CallToolResult, error) {
	var args pdfcheck.ValidatePDFInput
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, err
	}

	if args.PDFPath == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "pdf_path parameter is required"}},
		}, nil
	}

	if !filepath.IsAbs(args.PDFPath) {
		args.PDFPath = filepath.Join(workspaceRoot, args.PDFPath)
	}

	if args.ExpectedWidthInches <= 0 || args.ExpectedHeightInches <= 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "expected_width_inches and expected_height_inches parameters must be positive"}},
		}, nil
	}

	if args.MinGutterInches != nil && *args.MinGutterInches < 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "min_gutter_inches parameter must be non-negative"}},
		}, nil
	}

	if args.MinMarginInches != nil && *args.MinMarginInches < 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "min_margin_inches parameter must be non-negative"}},
		}, nil
	}

	res, err := pdfcheck.ValidatePDFPreflight(ctx, args)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("preflight validation failed: %v", err)}},
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
