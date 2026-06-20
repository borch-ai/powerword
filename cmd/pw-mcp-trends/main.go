package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/trends"
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
	scoreNicheSchema = `{
		"type": "object",
		"properties": {
			"keyword": {
				"type": "string",
				"description": "The seed keyword or niche term to score."
			},
			"limit": {
				"type": "integer",
				"description": "Optional limit on the number of candidates returned (default: 10)."
			},
			"sources": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Optional sources to query ('amazon', 'serp'). Defaults to both."
			}
		},
		"required": ["keyword"]
	}`

	getTrendVelocitySchema = `{
		"type": "object",
		"properties": {
			"keyword": {
				"type": "string",
				"description": "The keyword to get trend velocity for."
			},
			"period": {
				"type": "string",
				"description": "Optional period to fetch data for (e.g. 'today 3-m', 'today 1-m', 'today 12-m'). Defaults to 'today 3-m'."
			}
		},
		"required": ["keyword"]
	}`
)

func setupServer(workspaceRoot string, cfg *config.Config, svc *trends.TrendsService) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-trends",
		Version: "0.1.0",
	}, nil)

	trendsService := svc
	if trendsService == nil {
		trendsService = trends.NewTrendsService(cfg, nil)
	}

	srv.AddTool(&mcp.Tool{
		Name:        "score_niche",
		Description: "Returns ranked keyword candidates for a seed niche term.",
		InputSchema: json.RawMessage(scoreNicheSchema),
	}, handleScoreNiche(trendsService))

	srv.AddTool(&mcp.Tool{
		Name:        "get_trend_velocity",
		Description: "Returns a single keyword's trend velocity derived from Google Trends.",
		InputSchema: json.RawMessage(getTrendVelocitySchema),
	}, handleTrendVelocity(trendsService))

	return srv, nil
}

func handleScoreNiche(svc *trends.TrendsService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Keyword string   `json:"keyword"`
			Limit   int      `json:"limit"`
			Sources []string `json:"sources"`
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

		limit := args.Limit
		if limit <= 0 {
			limit = 10
		}

		res, err := svc.ScoreNiche(ctx, args.Keyword, limit, args.Sources)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to score niche: %v", err)}},
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

func handleTrendVelocity(svc *trends.TrendsService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Keyword string `json:"keyword"`
			Period  string `json:"period"`
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

		period := args.Period
		if period == "" {
			period = "today 3-m"
		}

		score, direction, err := svc.GetTrendVelocity(ctx, args.Keyword, period)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get trend velocity: %v", err)}},
			}, nil
		}

		res := struct {
			TrendScore float64 `json:"trend_score"`
			Direction  string  `json:"direction"`
		}{
			TrendScore: score,
			Direction:  direction,
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
