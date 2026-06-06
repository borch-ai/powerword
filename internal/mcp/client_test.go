package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClient_ListAndCallTools(t *testing.T) {
	ctx := context.Background()

	// Create test server
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-server", Version: "v1.0.0"}, nil)

	// Add a tool
	server.AddTool(&mcpsdk.Tool{
		Name:        "echo",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}}}`),
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var args struct{ Message string }
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: args.Message}},
		}, nil
	})

	t1, t2 := mcpsdk.NewInMemoryTransports()

	// Connect server
	errCh := make(chan error, 1)
	go func() {
		_, err := server.Connect(ctx, t1, nil)
		if err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	// Connect client
	client, err := NewClient(ctx, t2)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Test ListTools
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("Expected tool 'echo', got %+v", tools)
	}

	// Test CallTool
	res, err := client.CallTool(ctx, "echo", map[string]interface{}{"message": "hello"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if len(res.Content) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(res.Content))
	}
	textContent, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", res.Content[0])
	}
	if textContent.Text != "hello" {
		t.Errorf("Expected 'hello', got %q", textContent.Text)
	}
}
