package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/amazon"
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

	cfg, err := config.LoadFromWorkspace(workspaceRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	srv, err := setupServer(workspaceRoot, cfg, nil)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

const (
	getListingCountSchema = `{
		"type": "object",
		"properties": {
			"keyword": {
				"type": "string",
				"description": "The keyword to search on Amazon to retrieve the result count."
			}
		},
		"required": ["keyword"]
	}`
)

func setupServer(workspaceRoot string, cfg *config.Config, svc *amazon.AmazonService) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-amazon",
		Version: "0.1.0",
	}, nil)

	amazonService := svc
	if amazonService == nil {
		amazonService = amazon.NewAmazonService(cfg, nil)
	}

	srv.AddTool(&mcp.Tool{
		Name:        "get_amazon_listing_count",
		Description: "Returns the total product listing count for a search keyword on Amazon.",
		InputSchema: json.RawMessage(getListingCountSchema),
	}, handleGetListingCount(amazonService))

	return srv, nil
}

func handleGetListingCount(svc *amazon.AmazonService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Keyword string `json:"keyword"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Keyword == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "keyword parameter is required"}},
			}, nil
		}

		count, err := svc.GetListingCount(ctx, args.Keyword)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get listing count: %v", err)}},
			}, nil
		}

		res := struct {
			ListingCount int `json:"listing_count"`
		}{
			ListingCount: count,
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
