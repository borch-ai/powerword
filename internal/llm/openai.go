package llm

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client    *openai.Client
	modelName string
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(apiKey string, modelName string) (*OpenAIClient, error) {
	cfg := openai.DefaultConfig(apiKey)
	client := openai.NewClientWithConfig(cfg)
	return &OpenAIClient{
		client:    client,
		modelName: modelName,
	}, nil
}

// NewOpenAIClientWithConfig creates a new OpenAI client with custom configuration, useful for testing/mocking.
func NewOpenAIClientWithConfig(cfg openai.ClientConfig, modelName string) *OpenAIClient {
	client := openai.NewClientWithConfig(cfg)
	return &OpenAIClient{
		client:    client,
		modelName: modelName,
	}
}

func (o *OpenAIClient) prepareRequest(messages []Message, tools []ToolDefinition) (openai.ChatCompletionRequest, error) {
	var openAIMessages []openai.ChatCompletionMessage
	for _, msg := range messages {
		var role string
		switch msg.Role {
		case RoleSystem:
			role = openai.ChatMessageRoleSystem
		case RoleUser:
			role = openai.ChatMessageRoleUser
		case RoleAssistant:
			role = openai.ChatMessageRoleAssistant
		case RoleTool:
			role = openai.ChatMessageRoleTool
		default:
			return openai.ChatCompletionRequest{}, fmt.Errorf("unknown message role: %s", msg.Role)
		}

		var toolCalls []openai.ToolCall
		for _, tc := range msg.ToolCalls {
			toolCalls = append(toolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}

		openAIMessages = append(openAIMessages, openai.ChatCompletionMessage{
			Role:       role,
			Content:    msg.Content,
			ToolCalls:  toolCalls,
			ToolCallID: msg.ToolCallID,
		})
	}

	var openAITools []openai.Tool
	for _, t := range tools {
		openAITools = append(openAITools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	req := openai.ChatCompletionRequest{
		Model:    o.modelName,
		Messages: openAIMessages,
	}

	if len(openAITools) > 0 {
		req.Tools = openAITools
	}

	return req, nil
}

func (o *OpenAIClient) Generate(ctx context.Context, messages []Message, tools []ToolDefinition) (*Message, error) {
	req, err := o.prepareRequest(messages, tools)
	if err != nil {
		return nil, err
	}

	resp, err := o.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("empty response from OpenAI")
	}

	choice := resp.Choices[0]
	assistantMsg := &Message{
		Role:    RoleAssistant,
		Content: choice.Message.Content,
	}

	if resp.Usage.TotalTokens > 0 {
		assistantMsg.Usage = TokenUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		}
	}

	for _, tc := range choice.Message.ToolCalls {
		assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return assistantMsg, nil
}

func (o *OpenAIClient) Stream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	req, err := o.prepareRequest(messages, tools)
	if err != nil {
		return nil, err
	}
	req.StreamOptions = &openai.StreamOptions{
		IncludeUsage: true,
	}

	stream, err := o.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai chat completion stream error: %w", err)
	}

	out := make(chan StreamChunk, 10)

	go func() {
		defer func() {
			_ = stream.Close()
		}()
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				out <- StreamChunk{Error: ctx.Err()}
				return
			default:
				response, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return
				}
				if err != nil {
					out <- StreamChunk{Error: fmt.Errorf("openai stream error: %w", err)}
					return
				}

				if response.Usage != nil {
					out <- StreamChunk{
						Usage: &TokenUsage{
							InputTokens:  response.Usage.PromptTokens,
							OutputTokens: response.Usage.CompletionTokens,
						},
					}
				}

				if len(response.Choices) > 0 {
					out <- StreamChunk{Content: response.Choices[0].Delta.Content}
				}
			}
		}
	}()

	return out, nil
}

func (o *OpenAIClient) ListModels(ctx context.Context) ([]string, error) {
	modelsList, err := o.client.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("openai models list error: %w", err)
	}

	var models []string
	// Note: go-openai's ListModels response maps the JSON "data" field to the Models slice.
	for _, m := range modelsList.Models {
		models = append(models, m.ID)
	}
	return models, nil
}
