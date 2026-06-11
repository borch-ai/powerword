package mcp

import (
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/pkg/llm"
)

// TranslateTools converts MCP tools into generic LLM ToolDefinitions.
func TranslateTools(mcpTools []mcpsdk.Tool) ([]llm.ToolDefinition, error) {
	var defs []llm.ToolDefinition
	for _, t := range mcpTools {
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return defs, nil
}

// FormatToolResult formats the tool result into a string that can be placed in llm.Message.Content.
func FormatToolResult(result *mcpsdk.CallToolResult) (string, error) {
	if result == nil {
		return "", nil
	}

	var contents []string

	// Prepend an error prefix if the result is flagged as an error
	if result.IsError {
		contents = append(contents, "Error:")
	}

	for _, c := range result.Content {
		switch v := c.(type) {
		case *mcpsdk.TextContent:
			contents = append(contents, v.Text)
		case *mcpsdk.ImageContent:
			contents = append(contents, fmt.Sprintf("[Image data: %s]", v.MIMEType))
		case *mcpsdk.EmbeddedResource:
			contents = append(contents, fmt.Sprintf("[Embedded Resource: %s]", v.Resource.URI))
		default:
			// best effort string format
			contents = append(contents, fmt.Sprintf("%v", v))
		}
	}

	if len(contents) == 1 && result.IsError {
		return "Error: Tool execution failed with an unknown error", nil
	}

	return strings.Join(contents, "\n"), nil
}
