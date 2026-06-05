package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicClient struct {
	client    *anthropic.Client
	modelName string
}

// NewAnthropicClient creates a new Anthropic client.
func NewAnthropicClient(apiKey string, modelName string) (*AnthropicClient, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &AnthropicClient{
		client:    &client,
		modelName: modelName,
	}, nil
}

// NewAnthropicClientWithOpts creates a new Anthropic client with custom options, useful for testing/mocking.
func NewAnthropicClientWithOpts(modelName string, opts ...option.RequestOption) (*AnthropicClient, error) {
	client := anthropic.NewClient(opts...)
	return &AnthropicClient{
		client:    &client,
		modelName: modelName,
	}, nil
}

func (g *AnthropicClient) prepareParams(messages []Message, tools []ToolDefinition) (anthropic.MessageNewParams, error) {
	var anthropicMessages []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemPrompt += msg.Content
			continue
		}

		var role anthropic.MessageParamRole
		switch msg.Role {
		case RoleUser, RoleTool:
			role = anthropic.MessageParamRoleUser
		case RoleAssistant:
			role = anthropic.MessageParamRoleAssistant
		default:
			return anthropic.MessageNewParams{}, fmt.Errorf("invalid message role: %s", msg.Role)
		}

		var content []anthropic.ContentBlockParamUnion

		if msg.Content != "" && msg.Role != RoleTool {
			content = append(content, anthropic.NewTextBlock(msg.Content))
		}

		for _, tc := range msg.ToolCalls {
			var input any
			if tc.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
					return anthropic.MessageNewParams{}, fmt.Errorf("failed to unmarshal arguments for tool call %s: %w", tc.Name, err)
				}
			}
			content = append(content, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				},
			})
		}

		if msg.Role == RoleTool {
			content = append(content, anthropic.NewToolResultBlock(
				msg.ToolCallID,
				msg.Content,
				false,
			))
		}

		anthropicMessages = append(anthropicMessages, anthropic.MessageParam{
			Role:    role,
			Content: content,
		})
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(g.modelName),
		MaxTokens: int64(4096),
		Messages:  anthropicMessages,
	}

	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{
				Text: systemPrompt,
			},
		}
	}

	if len(tools) > 0 {
		var anthropicTools []anthropic.ToolUnionParam
		for _, t := range tools {
			schema, err := convertAnthropicSchema(t.InputSchema)
			if err != nil {
				return anthropic.MessageNewParams{}, err
			}
			anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        t.Name,
					Description: anthropic.String(t.Description),
					InputSchema: schema,
				},
			})
		}
		params.Tools = anthropicTools
	}

	return params, nil
}

func (g *AnthropicClient) Generate(ctx context.Context, messages []Message, tools []ToolDefinition) (*Message, error) {
	params, err := g.prepareParams(messages, tools)
	if err != nil {
		return nil, err
	}

	msg, err := g.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic messages error: %w", err)
	}

	assistantMsg := &Message{
		Role: RoleAssistant,
	}

	var textBuilder strings.Builder
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textBuilder.WriteString(block.Text)
		case "tool_use":
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(block.Input),
			})
		}
	}
	assistantMsg.Content = textBuilder.String()
	return assistantMsg, nil
}

func (g *AnthropicClient) Stream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	params, err := g.prepareParams(messages, tools)
	if err != nil {
		return nil, err
	}

	stream := g.client.Messages.NewStreaming(ctx, params)
	out := make(chan StreamChunk, 10)

	go func() {
		defer func() {
			_ = stream.Close()
		}()
		defer close(out)

		for stream.Next() {
			select {
			case <-ctx.Done():
				out <- StreamChunk{Error: ctx.Err()}
				return
			default:
				event := stream.Current()
				if eventVariant, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
					if deltaVariant, ok := eventVariant.Delta.AsAny().(anthropic.TextDelta); ok {
						out <- StreamChunk{Content: deltaVariant.Text}
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			out <- StreamChunk{Error: fmt.Errorf("anthropic stream error: %w", err)}
		}
	}()

	return out, nil
}

func convertAnthropicSchema(input any) (anthropic.ToolInputSchemaParam, error) {
	var s anthropic.ToolInputSchemaParam
	if input == nil {
		return s, nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	if s.Type == "" {
		s.Type = "object"
	}
	return s, nil
}
