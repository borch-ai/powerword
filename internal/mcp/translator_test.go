package mcp

import (
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTranslateTools(t *testing.T) {
	mcpTools := []mcpsdk.Tool{
		{
			Name:        "test-tool",
			Description: "A test tool",
			InputSchema: map[string]interface{}{"type": "object"},
		},
	}

	defs, err := TranslateTools(mcpTools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Name != "test-tool" {
		t.Errorf("expected name 'test-tool', got %s", defs[0].Name)
	}
}

func TestFormatToolResult(t *testing.T) {
	// Test success with text
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "Success"},
		},
	}
	formatted, err := FormatToolResult(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if formatted != "Success" {
		t.Errorf("expected 'Success', got '%s'", formatted)
	}

	// Test pointer
	result2 := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "Success Ptr"},
		},
	}
	formatted2, _ := FormatToolResult(result2)
	if formatted2 != "Success Ptr" {
		t.Errorf("expected 'Success Ptr', got '%s'", formatted2)
	}

	// Test error flag
	errResult := &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "File not found"},
		},
	}
	formattedErr, _ := FormatToolResult(errResult)
	if formattedErr != "Error:\nFile not found" {
		t.Errorf("expected 'Error:\nFile not found', got '%s'", formattedErr)
	}

	// Test empty error
	emptyErrResult := &mcpsdk.CallToolResult{
		IsError: true,
	}
	formattedEmptyErr, _ := FormatToolResult(emptyErrResult)
	if formattedEmptyErr != "Error: Tool execution failed with an unknown error" {
		t.Errorf("expected 'Error: Tool execution failed with an unknown error', got '%s'", formattedEmptyErr)
	}
}
