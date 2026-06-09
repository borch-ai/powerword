package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestAnthropicClient_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":   "msg_123",
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "Hello from Anthropic mock!",
				},
			},
			"usage": map[string]any{
				"input_tokens":            10,
				"output_tokens":           20,
				"cache_read_input_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewAnthropicClientWithOpts("claude-3-5-sonnet",
		option.WithBaseURL(server.URL),
		option.WithAPIKey("dummy-key"),
	)
	if err != nil {
		t.Fatal(err)
	}

	messages := []Message{
		{Role: RoleSystem, Content: "You are Claude"},
		{Role: RoleUser, Content: "Hello"},
	}

	msg, err := client.Generate(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Content != "Hello from Anthropic mock!" {
		t.Errorf("unexpected content: %s", msg.Content)
	}

	if msg.Usage == nil || msg.Usage.InputTokens != 10 || msg.Usage.OutputTokens != 20 || msg.Usage.CachedTokens != 5 {
		t.Errorf("unexpected usage: %+v", msg.Usage)
	}
}

func TestAnthropicClient_Generate_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":   "msg_123",
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type": "tool_use",
					"id":   "toolu_1",
					"name": "test_tool",
					"input": map[string]any{
						"arg1": "val1",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewAnthropicClientWithOpts("claude-3-5-sonnet",
		option.WithBaseURL(server.URL),
		option.WithAPIKey("dummy-key"),
	)
	if err != nil {
		t.Fatal(err)
	}

	tools := []ToolDefinition{
		{
			Name:        "test_tool",
			Description: "A test tool",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"arg1": map[string]any{"type": "string"},
				},
			},
		},
	}

	messages := []Message{
		{Role: RoleUser, Content: "Use tool"},
		{
			Role:       RoleTool,
			ToolCallID: "toolu_0",
			Content:    `{"result":"ok"}`,
		},
	}

	msg, err := client.Generate(context.Background(), messages, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}

	tc := msg.ToolCalls[0]
	if tc.ID != "toolu_1" || tc.Name != "test_tool" || tc.Arguments != `{"arg1":"val1"}` {
		t.Errorf("unexpected tool call values: %+v", tc)
	}
}

func TestAnthropicClient_Generate_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"Invalid API key"}}`))
	}))
	defer server.Close()

	client, err := NewAnthropicClientWithOpts("claude-3-5-sonnet",
		option.WithBaseURL(server.URL),
		option.WithAPIKey("dummy-key"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Generate(context.Background(), []Message{{Role: RoleUser, Content: "Hello"}}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAnthropicClient_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		chunks := []string{
			`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello "}}`,
			`{"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Claude!"}}`,
		}

		for _, chunk := range chunks {
			_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", chunk)
			w.(http.Flusher).Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	client, err := NewAnthropicClientWithOpts("claude-3-5-sonnet",
		option.WithBaseURL(server.URL),
		option.WithAPIKey("dummy-key"),
	)
	if err != nil {
		t.Fatal(err)
	}

	ch, err := client.Stream(context.Background(), []Message{{Role: RoleUser, Content: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	var results []string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("unexpected stream chunk error: %v", chunk.Error)
		}
		results = append(results, chunk.Content)
	}

	expected := []string{"Hello ", "Claude!"}
	if len(results) != len(expected) || results[0] != expected[0] || results[1] != expected[1] {
		t.Errorf("unexpected stream results: %+v", results)
	}
}

func TestAnthropic_ConvertSchema(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"str": map[string]any{"type": "string"},
		},
	}

	schema, err := convertAnthropicSchema(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if schema.Type != "object" {
		t.Errorf("expected object, got %s", schema.Type)
	}

	// Test nil input
	nilSchema, err := convertAnthropicSchema(nil)
	if err != nil || nilSchema.Type != "" {
		t.Errorf("expected empty schema, got %+v, error: %v", nilSchema, err)
	}
}

func TestNewAnthropicClient_Direct(t *testing.T) {
	client, err := NewAnthropicClient("dummy-key", "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to not be nil")
	}
}

func TestAnthropic_CoverageBonus(t *testing.T) {
	client := &AnthropicClient{modelName: "claude"}

	// 1. convertAnthropicSchema error
	_, err := convertAnthropicSchema(make(chan int))
	if err == nil {
		t.Error("expected error for unmarshalable schema")
	}

	// 2. prepareParams invalid role
	_, err = client.prepareParams([]Message{{Role: "invalid", Content: "x"}}, nil)
	if err == nil {
		t.Error("expected error for invalid role")
	}

	// 3. prepareParams with system prompt, tool call, and tool result
	messages := []Message{
		{Role: RoleSystem, Content: "system"},
		{Role: RoleUser, Content: "user"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "1", Name: "t", Arguments: `{"a":1}`}}},
		{Role: RoleTool, ToolCallID: "1", Content: "result"},
	}
	tools := []ToolDefinition{
		{Name: "t", Description: "t", InputSchema: map[string]any{"type": "object"}},
	}
	params, err := client.prepareParams(messages, tools)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(params.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(params.Messages))
	}

	// 4. prepareParams tool schema error
	badTools := []ToolDefinition{
		{Name: "t", Description: "t", InputSchema: make(chan int)},
	}
	_, err = client.prepareParams(messages, badTools)
	if err == nil {
		t.Error("expected error for bad tool schema")
	}
}

func TestAnthropicClient_Stream_Cancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\": \"content_block_delta\", \"index\": 0, \"delta\": {\"type\": \"text_delta\", \"text\": \"Hello \"}}\n\n"))
	}))
	defer server.Close()

	client, err := NewAnthropicClientWithOpts("claude-3-5-sonnet",
		option.WithBaseURL(server.URL),
		option.WithAPIKey("dummy-key"),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	ch, err := client.Stream(ctx, []Message{{Role: RoleUser, Content: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	var gotCancelErr bool
	for chunk := range ch {
		if chunk.Error != nil && errors.Is(chunk.Error, context.Canceled) {
			gotCancelErr = true
		}
	}
	if !gotCancelErr {
		t.Error("expected stream to terminate with context.Canceled error")
	}
}

func TestAnthropicClient_ListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": []map[string]any{
				{"id": "claude-3-5-sonnet", "type": "model"},
				{"id": "claude-3-opus", "type": "model"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewAnthropicClientWithOpts("claude",
		option.WithBaseURL(server.URL),
		option.WithAPIKey("dummy-key"),
	)
	if err != nil {
		t.Fatal(err)
	}

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 || models[0] != "claude-3-5-sonnet" || models[1] != "claude-3-opus" {
		t.Errorf("unexpected models: %v", models)
	}
}

func TestAnthropicClient_ListModels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewAnthropicClientWithOpts("claude",
		option.WithBaseURL(server.URL),
		option.WithAPIKey("dummy-key"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
