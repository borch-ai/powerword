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

func (a *AnthropicClient) prepareParams(messages []Message, tools []ToolDefinition) (anthropic.MessageNewParams, error) {
	var anthropicMessages []anthropic.MessageParam
	var systemPrompt string

	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemPrompt += msg.Content
			continue
		}

		param, err := convertMessageToParam(msg)
		if err != nil {
			return anthropic.MessageNewParams{}, err
		}
		anthropicMessages = append(anthropicMessages, param)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.modelName),
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

func (a *AnthropicClient) Generate(ctx context.Context, messages []Message, tools []ToolDefinition) (*Message, error) {
	params, err := a.prepareParams(messages, tools)
	if err != nil {
		return nil, err
	}

	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic messages error: %w", err)
	}

	assistantMsg := &Message{
		Role: RoleAssistant,
	}
	assistantMsg.Usage = TokenUsage{
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
		CachedTokens: int(msg.Usage.CacheReadInputTokens),
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

func (a *AnthropicClient) Stream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	params, err := a.prepareParams(messages, tools)
	if err != nil {
		return nil, err
	}

	stream := a.client.Messages.NewStreaming(ctx, params)
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
				handleAnthropicStreamEvent(stream.Current(), out)
			}
		}

		if err := stream.Err(); err != nil {
			out <- StreamChunk{Error: fmt.Errorf("anthropic stream error: %w", err)}
		}
	}()

	return out, nil
}

func handleAnthropicStreamEvent(event anthropic.MessageStreamEventUnion, out chan<- StreamChunk) {
	if messageStart, ok := event.AsAny().(anthropic.MessageStartEvent); ok {
		out <- StreamChunk{
			Usage: &TokenUsage{
				InputTokens:  int(messageStart.Message.Usage.InputTokens),
				CachedTokens: int(messageStart.Message.Usage.CacheReadInputTokens),
			},
		}
	}

	if messageDelta, ok := event.AsAny().(anthropic.MessageDeltaEvent); ok {
		out <- StreamChunk{
			Usage: &TokenUsage{
				OutputTokens: int(messageDelta.Usage.OutputTokens),
			},
		}
	}

	if eventVariant, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
		if deltaVariant, ok := eventVariant.Delta.AsAny().(anthropic.TextDelta); ok {
			out <- StreamChunk{Content: deltaVariant.Text}
		}
	}
}
func (a *AnthropicClient) ListModels(ctx context.Context) ([]string, error) {
	page, err := a.client.Models.List(ctx, anthropic.ModelListParams{})
	if err != nil {
		return nil, fmt.Errorf("anthropic models list error: %w", err)
	}

	var models []string
	for _, m := range page.Data {
		models = append(models, m.ID)
	}
	return models, nil
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

func convertMessageToParam(msg Message) (anthropic.MessageParam, error) {
	var role anthropic.MessageParamRole
	switch msg.Role {
	case RoleUser, RoleTool:
		role = anthropic.MessageParamRoleUser
	case RoleAssistant:
		role = anthropic.MessageParamRoleAssistant
	default:
		return anthropic.MessageParam{}, fmt.Errorf("invalid message role: %s", msg.Role)
	}

	var content []anthropic.ContentBlockParamUnion

	if msg.Content != "" && msg.Role != RoleTool {
		content = append(content, anthropic.NewTextBlock(msg.Content))
	}

	for _, tc := range msg.ToolCalls {
		var input any
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
				return anthropic.MessageParam{}, fmt.Errorf("failed to unmarshal arguments for tool call %s: %w", tc.Name, err)
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

	return anthropic.MessageParam{
		Role:    role,
		Content: content,
	}, nil
}
