package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type GeminiClient struct {
	client    *genai.Client
	modelName string
	opts      []option.ClientOption
}

// NewGeminiClient creates a new Gemini client.
func NewGeminiClient(apiKey string, modelName string) (*GeminiClient, error) {
	ctx := context.Background()
	opts := []option.ClientOption{option.WithAPIKey(apiKey)}
	client, err := genai.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	return &GeminiClient{
		client:    client,
		modelName: modelName,
		opts:      opts,
	}, nil
}

// NewGeminiClientWithOpts creates a new Gemini client with custom options, useful for testing/mocking.
func NewGeminiClientWithOpts(modelName string, opts ...option.ClientOption) (*GeminiClient, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client with opts: %w", err)
	}

	return &GeminiClient{
		client:    client,
		modelName: modelName,
		opts:      opts,
	}, nil
}

func (g *GeminiClient) prepareModel(messages []Message, tools []ToolDefinition) (*genai.GenerativeModel, []*genai.Content, []genai.Part, error) {
	model := g.client.GenerativeModel(g.modelName)

	// Convert tools
	var decls []*genai.FunctionDeclaration
	for _, t := range tools {
		schema, err := convertSchema(t.InputSchema)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to convert schema for tool %s: %w", t.Name, err)
		}
		decls = append(decls, &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		})
	}
	if len(decls) > 0 {
		model.Tools = []*genai.Tool{{
			FunctionDeclarations: decls,
		}}
	}

	// Convert messages
	var history []*genai.Content
	var systemInstruction strings.Builder

	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemInstruction.WriteString(msg.Content)
			continue
		}
	}

	if systemInstruction.Len() > 0 {
		model.SystemInstruction = &genai.Content{
			Parts: []genai.Part{genai.Text(systemInstruction.String())},
		}
	}

	// Filter out system messages and construct history
	var chatMessages []Message
	for _, msg := range messages {
		if msg.Role != RoleSystem {
			chatMessages = append(chatMessages, msg)
		}
	}

	if len(chatMessages) == 0 {
		return nil, nil, nil, errors.New("no messages provided after filtering system instruction")
	}

	// We separate the last message because chat.SendMessage takes it as an argument.
	historyMsgs := chatMessages[:len(chatMessages)-1]
	lastMsg := chatMessages[len(chatMessages)-1]

	for _, msg := range historyMsgs {
		content, err := g.msgToContent(msg, chatMessages)
		if err != nil {
			return nil, nil, nil, err
		}
		history = append(history, content)
	}

	lastParts, err := g.msgToParts(lastMsg, chatMessages)
	if err != nil {
		return nil, nil, nil, err
	}

	return model, history, lastParts, nil
}

func (g *GeminiClient) Generate(ctx context.Context, messages []Message, tools []ToolDefinition) (*Message, error) {
	model, history, lastParts, err := g.prepareModel(messages, tools)
	if err != nil {
		return nil, err
	}

	chat := model.StartChat()
	chat.History = history

	resp, err := chat.SendMessage(ctx, lastParts...)
	if err != nil {
		return nil, fmt.Errorf("gemini message error: %w", err)
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, errors.New("empty response from Gemini")
	}

	candidate := resp.Candidates[0]
	assistantMsg := &Message{
		Role: RoleAssistant,
	}

	if resp.UsageMetadata != nil {
		assistantMsg.Usage = TokenUsage{
			InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
			OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			CachedTokens: int(resp.UsageMetadata.CachedContentTokenCount),
		}
	}

	var textBuilder strings.Builder
	for _, part := range candidate.Content.Parts {
		switch p := part.(type) {
		case genai.Text:
			textBuilder.WriteString(string(p))
		case genai.FunctionCall:
			argsData, err := json.Marshal(p.Args)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal function call args: %w", err)
			}
			callID := fmt.Sprintf("%s-%d", p.Name, len(assistantMsg.ToolCalls))
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, ToolCall{
				ID:        callID,
				Name:      p.Name,
				Arguments: string(argsData),
			})
		}
	}
	assistantMsg.Content = textBuilder.String()
	return assistantMsg, nil
}

func (g *GeminiClient) Stream(ctx context.Context, messages []Message, tools []ToolDefinition) (<-chan StreamChunk, error) {
	model, history, lastParts, err := g.prepareModel(messages, tools)
	if err != nil {
		return nil, err
	}

	chat := model.StartChat()
	chat.History = history

	iter := chat.SendMessageStream(ctx, lastParts...)

	out := make(chan StreamChunk, 10)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				out <- StreamChunk{Error: ctx.Err()}
				return
			default:
				resp, err := iter.Next()
				if errors.Is(err, iterator.Done) {
					return
				}
				if err != nil {
					out <- StreamChunk{Error: fmt.Errorf("gemini stream error: %w", err)}
					return
				}
				handleStreamChunk(resp, out)
			}
		}
	}()

	return out, nil
}

func handleStreamChunk(resp *genai.GenerateContentResponse, out chan<- StreamChunk) {
	var chunk StreamChunk
	if resp.UsageMetadata != nil {
		chunk.Usage = &TokenUsage{
			InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
			OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			CachedTokens: int(resp.UsageMetadata.CachedContentTokenCount),
		}
	}

	if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if t, ok := part.(genai.Text); ok {
				chunk.Content += string(t)
			}
		}
	}

	if chunk.Content != "" || chunk.Usage != nil {
		out <- chunk
	}
}

func (g *GeminiClient) msgToContent(msg Message, allMsgs []Message) (*genai.Content, error) {
	role := "user"
	if msg.Role == RoleAssistant {
		role = "model"
	}

	parts, err := g.msgToParts(msg, allMsgs)
	if err != nil {
		return nil, err
	}

	return &genai.Content{
		Role:  role,
		Parts: parts,
	}, nil
}

func (g *GeminiClient) msgToParts(msg Message, allMsgs []Message) ([]genai.Part, error) {
	var parts []genai.Part

	if msg.Content != "" && msg.Role != RoleTool {
		parts = append(parts, genai.Text(msg.Content))
	}

	for _, tc := range msg.ToolCalls {
		var args map[string]any
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool call arguments: %w", err)
			}
		}
		parts = append(parts, genai.FunctionCall{
			Name: tc.Name,
			Args: args,
		})
	}

	if msg.Role == RoleTool {
		funcName := g.findFunctionName(allMsgs, msg.ToolCallID)
		var result map[string]any
		if err := json.Unmarshal([]byte(msg.Content), &result); err != nil {
			result = map[string]any{"result": msg.Content}
		}
		parts = append(parts, genai.FunctionResponse{
			Name:     funcName,
			Response: result,
		})
	}

	return parts, nil
}

func (g *GeminiClient) findFunctionName(messages []Message, callID string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		for _, tc := range messages[i].ToolCalls {
			if tc.ID == callID {
				return tc.Name
			}
		}
	}
	return callID
}

func convertSchema(input any) (*genai.Schema, error) {
	if input == nil {
		return nil, nil
	}
	if s, ok := input.(*genai.Schema); ok {
		return s, nil
	}

	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var rawMap map[string]any
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, err
	}

	return parseMapToSchema(rawMap)
}

func parseMapToSchema(raw map[string]any) (*genai.Schema, error) {
	if raw == nil {
		return nil, nil
	}
	s := &genai.Schema{}

	if val, ok := raw["type"].(string); ok {
		switch strings.ToLower(val) {
		case "string":
			s.Type = genai.TypeString
		case "number":
			s.Type = genai.TypeNumber
		case "integer":
			s.Type = genai.TypeInteger
		case "boolean":
			s.Type = genai.TypeBoolean
		case "array":
			s.Type = genai.TypeArray
		case "object":
			s.Type = genai.TypeObject
		}
	}

	if val, ok := raw["description"].(string); ok {
		s.Description = val
	}

	if err := parseProperties(s, raw); err != nil {
		return nil, err
	}
	parseRequired(s, raw)
	parseEnum(s, raw)

	if val, ok := raw["items"].(map[string]any); ok {
		itemSchema, err := parseMapToSchema(val)
		if err != nil {
			return nil, err
		}
		s.Items = itemSchema
	}

	return s, nil
}

func parseProperties(s *genai.Schema, raw map[string]any) error {
	if val, ok := raw["properties"].(map[string]any); ok {
		s.Properties = make(map[string]*genai.Schema)
		for k, v := range val {
			if propMap, ok := v.(map[string]any); ok {
				propSchema, err := parseMapToSchema(propMap)
				if err != nil {
					return err
				}
				s.Properties[k] = propSchema
			}
		}
	}
	return nil
}

func parseRequired(s *genai.Schema, raw map[string]any) {
	if val, ok := raw["required"].([]any); ok {
		s.Required = make([]string, 0, len(val))
		for _, item := range val {
			if str, ok := item.(string); ok {
				s.Required = append(s.Required, str)
			}
		}
	}
}

func parseEnum(s *genai.Schema, raw map[string]any) {
	if val, ok := raw["enum"].([]any); ok {
		s.Enum = make([]string, 0, len(val))
		for _, item := range val {
			if str, ok := item.(string); ok {
				s.Enum = append(s.Enum, str)
			}
		}
	}
}

func (g *GeminiClient) ListModels(ctx context.Context) ([]string, error) {
	var models []string
	iter := g.client.ListModels(ctx)
	for {
		m, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("gemini models list error: %w", err)
		}
		models = append(models, m.Name)
	}
	return models, nil
}
