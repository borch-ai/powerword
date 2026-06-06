package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClient represents an active session with an MCP server.
type MCPClient struct {
	session *mcpsdk.ClientSession
}

// NewClient initializes a new MCPClient using the provided transport.
// It performs the SDK's initialization protocol.
func NewClient(ctx context.Context, transport mcpsdk.Transport) (*MCPClient, error) {
	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "powerword",
		Version: "v1.0.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect MCP client: %w", err)
	}

	return &MCPClient{
		session: session,
	}, nil
}

// ListTools queries the MCP server for available tools.
func (c *MCPClient) ListTools(ctx context.Context) ([]mcpsdk.Tool, error) {
	result, err := c.session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}
	var tools []mcpsdk.Tool
	for _, t := range result.Tools {
		tools = append(tools, *t)
	}
	return tools, nil
}

// CallTool executes a tool on the MCP server with the given arguments.
func (c *MCPClient) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcpsdk.CallToolResult, error) {
	result, err := c.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call tool %q: %w", name, err)
	}
	return result, nil
}

// Close closes the MCP session.
func (c *MCPClient) Close() error {
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}
