package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/config"
	"github.com/borch-ai/powerword/internal/plugins/seo"
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

	srv, err := setupServer(workspaceRoot, cfg, nil)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

const (
	analyzeNicheSchema = `{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The target search query or book niche to analyze (e.g. 'existential nursery rhymes')."
			},
			"asins": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Optional list of specific competitor ASINs to retrieve details for."
			}
		}
	}`

	generateListingSchema = `{
		"type": "object",
		"properties": {
			"niche": {
				"type": "string",
				"description": "High level niche or topic of the book (e.g. 'existential nursery rhymes')."
			},
			"target_audience": {
				"type": "string",
				"description": "Optional target reader age group or persona (e.g. 'toddlers and parents')."
			},
			"keywords": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Optional seed keywords to weave into the generated metadata."
			},
			"book_type": {
				"type": "string",
				"description": "Optional book format or style (e.g. 'notebook', 'journal', 'coloring book', 'poetry')."
			},
			"competitor_data": {
				"type": "string",
				"description": "Optional competitor text/context to guide optimization (e.g., descriptions or metadata retrieved from seo_analyze_niche)."
			}
		},
		"required": ["niche"]
	}`
)

func setupServer(workspaceRoot string, cfg *config.Config, svc *seo.SEOService) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-seo",
		Version: "1.0.0",
	}, nil)

	seoService := svc
	if seoService == nil {
		seoService = seo.NewSEOService(cfg)
	}

	srv.AddTool(&mcp.Tool{
		Name:        "seo_analyze_niche",
		Description: "Queries search suggestion networks to retrieve trending search terms and competitor metadata.",
		InputSchema: json.RawMessage(analyzeNicheSchema),
	}, handleAnalyzeNiche(seoService))

	srv.AddTool(&mcp.Tool{
		Name:        "seo_generate_listing",
		Description: "Generates title, subtitle, seven search keywords, and description copy optimized for Amazon index algorithms.",
		InputSchema: json.RawMessage(generateListingSchema),
	}, handleGenerateListing(seoService))

	return srv, nil
}

func handleAnalyzeNiche(svc *seo.SEOService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Query string   `json:"query"`
			ASINs []string `json:"asins"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Query == "" && len(args.ASINs) == 0 {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "either query or asins must be specified"}},
			}, nil
		}

		res, err := svc.AnalyzeNiche(ctx, args.Query, args.ASINs)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to analyze niche: %v", err)}},
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

func handleGenerateListing(svc *seo.SEOService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Niche          string   `json:"niche"`
			TargetAudience string   `json:"target_audience"`
			Keywords       []string `json:"keywords"`
			BookType       string   `json:"book_type"`
			CompetitorData string   `json:"competitor_data"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Niche == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "niche parameter is required"}},
			}, nil
		}

		res, err := svc.GenerateListing(ctx, args.Niche, args.TargetAudience, args.Keywords, args.BookType, args.CompetitorData)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to generate listing: %v", err)}},
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
