package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func createMockClient(t *testing.T, serverName string, toolName string) (*MCPClient, func()) {
	t.Helper()
	ctx := context.Background()
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: serverName, Version: "v1.0.0"}, nil)
	server.AddTool(&mcpsdk.Tool{
		Name:        toolName,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: toolName + " called"}},
		}, nil
	})

	t1, t2 := mcpsdk.NewInMemoryTransports()
	go func() {
		_, _ = server.Connect(ctx, t1, nil)
	}()

	client, err := NewClient(ctx, t2)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return client, func() {
		_ = client.Close()
	}
}

func TestRegistry_AddRemoveListCall(t *testing.T) {
	ctx := context.Background()
	registry := NewRegistry()

	c1, cleanup1 := createMockClient(t, "s1", "read")
	defer cleanup1()

	c2, cleanup2 := createMockClient(t, "s2", "read")
	defer cleanup2()

	// Test AddClient validations
	if err := registry.AddClient("", c1); err == nil {
		t.Errorf("Expected error for empty client name")
	}
	if err := registry.AddClient("bad__name", c1); err == nil {
		t.Errorf("Expected error for client name containing __")
	}
	if err := registry.AddClient("nilClient", nil); err == nil {
		t.Errorf("Expected error for nil client")
	}

	// Add clients
	if err := registry.AddClient("fs", c1); err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}
	if err := registry.AddClient("git", c2); err != nil {
		t.Fatalf("AddClient failed: %v", err)
	}
	if err := registry.AddClient("fs", c1); err == nil {
		t.Errorf("Expected error adding duplicate client")
	}

	// List all tools (should aggregate properly)
	tools, err := registry.ListAllTools(ctx)
	if err != nil {
		t.Fatalf("ListAllTools failed: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("Expected 2 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}

	if !names["fs__read"] || !names["git__read"] {
		t.Errorf("Missing expected aggregated names, got %v", names)
	}

	// Call tool
	res, err := registry.CallTool(ctx, "fs__read", map[string]interface{}{})
	if err != nil {
		t.Fatalf("CallTool fs__read failed: %v", err)
	}

	if res.Content[0].(*mcpsdk.TextContent).Text != "read called" {
		t.Errorf("Unexpected CallTool result: %+v", res)
	}

	// Try to call invalid tool format
	_, err = registry.CallTool(ctx, "invalidformat", map[string]interface{}{})
	if err == nil {
		t.Errorf("Expected error for invalid format")
	}

	// Try to call nonexistent client
	_, err = registry.CallTool(ctx, "nonexistent__read", map[string]interface{}{})
	if err == nil {
		t.Errorf("Expected error for nonexistent client")
	}

	// Remove client
	if err := registry.RemoveClient("fs"); err != nil {
		t.Fatalf("RemoveClient failed: %v", err)
	}
	if err := registry.RemoveClient("fs"); err == nil {
		t.Errorf("Expected error removing non-existent client")
	}
}
