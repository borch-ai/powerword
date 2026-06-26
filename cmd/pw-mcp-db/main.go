package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/db"
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

	svc, err := db.NewDBService(&cfg.Plugins.DB)
	if err != nil {
		return fmt.Errorf("failed to initialise db service: %w", err)
	}
	defer func() { _ = svc.Close() }()

	srv, err := setupServer(svc)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

// ---------------------------------------------------------------------------
// JSON schemas
// ---------------------------------------------------------------------------

const (
	listTablesSchema = `{
		"type": "object",
		"properties": {}
	}`

	describeTableSchema = `{
		"type": "object",
		"properties": {
			"table": {
				"type": "string",
				"description": "The name of the table or view to describe."
			}
		},
		"required": ["table"]
	}`

	queryReadSchema = `{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "A read-only SQL SELECT query to execute. Write operations (INSERT, UPDATE, DELETE, DROP, ALTER, TRUNCATE) are always rejected."
			}
		},
		"required": ["query"]
	}`

	showLocksSchema = `{
		"type": "object",
		"properties": {}
	}`
)

// ---------------------------------------------------------------------------
// Server setup
// ---------------------------------------------------------------------------

func setupServer(svc *db.DBService) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-db",
		Version: "0.1.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "db_list_tables",
		Description: "Returns all table and view names visible in the configured database schema or BigQuery dataset.",
		InputSchema: json.RawMessage(listTablesSchema),
	}, handleListTables(svc))

	srv.AddTool(&mcp.Tool{
		Name:        "db_describe_table",
		Description: "Returns column names, data types, nullability, and key information for a named table or view.",
		InputSchema: json.RawMessage(describeTableSchema),
	}, handleDescribeTable(svc))

	srv.AddTool(&mcp.Tool{
		Name:        "db_query_read",
		Description: "Executes a read-only SQL SELECT query and returns results as JSON. Results are capped at the configured max_rows limit. Write operations are always rejected.",
		InputSchema: json.RawMessage(queryReadSchema),
	}, handleQueryRead(svc))

	srv.AddTool(&mcp.Tool{
		Name:        "db_show_locks",
		Description: "Lists currently active queries and locking sessions. Returns an advisory message for backends that do not expose lock information (SQLite, DuckDB, BigQuery).",
		InputSchema: json.RawMessage(showLocksSchema),
	}, handleShowLocks(svc))

	return srv, nil
}

// ---------------------------------------------------------------------------
// Tool handlers
// ---------------------------------------------------------------------------

func handleListTables(svc *db.DBService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tables, err := svc.ListTables(ctx)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("db_list_tables failed: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(tables, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleDescribeTable(svc *db.DBService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Table string `json:"table"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		if args.Table == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "table parameter is required"}},
			}, nil
		}

		cols, err := svc.DescribeTable(ctx, args.Table)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("db_describe_table failed: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(cols, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleQueryRead(svc *db.DBService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		if args.Query == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "query parameter is required"}},
			}, nil
		}

		result, err := svc.QueryRead(ctx, args.Query)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("db_query_read failed: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleShowLocks(svc *db.DBService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		locks, err := svc.ShowLocks(ctx)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("db_show_locks failed: %v", err)}},
			}, nil
		}

		data, err := json.MarshalIndent(locks, "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}
