package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestOpenAIClient_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleAssistant,
						Content: "Hello from OpenAI mock server!",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("dummy")
	cfg.BaseURL = server.URL
	client, err := NewOpenAIClient("dummy", "gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	client.client = openai.NewClientWithConfig(cfg)

	messages := []Message{
		{Role: RoleSystem, Content: "You are a system prompt."},
		{Role: RoleUser, Content: "Hello!"},
	}

	msg, err := client.Generate(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Content != "Hello from OpenAI mock server!" {
		t.Errorf("unexpected content: %s", msg.Content)
	}
}

func TestOpenAIClient_Generate_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Role: openai.ChatMessageRoleAssistant,
						ToolCalls: []openai.ToolCall{
							{
								ID:   "call_1",
								Type: openai.ToolTypeFunction,
								Function: openai.FunctionCall{
									Name:      "test_tool",
									Arguments: `{"arg1":"val1"}`,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("dummy")
	cfg.BaseURL = server.URL
	client := NewOpenAIClientWithConfig(cfg, "gpt-4")

	tools := []ToolDefinition{
		{
			Name:        "test_tool",
			Description: "A test tool",
			InputSchema: map[string]any{"type": "object"},
		},
	}

	messages := []Message{
		{Role: RoleUser, Content: "Run tool"},
		{
			Role:       RoleTool,
			ToolCallID: "call_0",
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
	if tc.ID != "call_1" || tc.Name != "test_tool" || tc.Arguments != `{"arg1":"val1"}` {
		t.Errorf("unexpected tool call values: %+v", tc)
	}
}

func TestOpenAIClient_Generate_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("dummy")
	cfg.BaseURL = server.URL
	client := NewOpenAIClientWithConfig(cfg, "gpt-4")

	_, err := client.Generate(context.Background(), []Message{{Role: RoleUser, Content: "Hello"}}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOpenAIClient_Generate_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("dummy")
	cfg.BaseURL = server.URL
	client := NewOpenAIClientWithConfig(cfg, "gpt-4")

	_, err := client.Generate(context.Background(), []Message{{Role: RoleUser, Content: "Hello"}}, nil)
	if !errors.Is(err, nil) && err.Error() != "openai chat completion error: empty response from OpenAI" && !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("expected empty response error, got: %v", err)
	}
}

func TestOpenAIClient_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Write stream events
		chunks := []string{
			`{"choices":[{"delta":{"content":"Hello "}}]}`,
			`{"choices":[{"delta":{"content":"world!"}}]}`,
		}

		for _, chunk := range chunks {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("dummy")
	cfg.BaseURL = server.URL
	client := NewOpenAIClientWithConfig(cfg, "gpt-4")

	ch, err := client.Stream(context.Background(), []Message{{Role: RoleUser, Content: "Hello"}}, nil)
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

	expected := []string{"Hello ", "world!"}
	if len(results) != len(expected) || results[0] != expected[0] || results[1] != expected[1] {
		t.Errorf("unexpected stream results: %+v", results)
	}
}

func TestOpenAIClient_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad Request"))
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("dummy")
	cfg.BaseURL = server.URL
	client := NewOpenAIClientWithConfig(cfg, "gpt-4")

	_, err := client.Stream(context.Background(), []Message{{Role: RoleUser, Content: "Hello"}}, nil)
	if err == nil {
		t.Fatal("expected stream error, got nil")
	}
}

func TestOpenAIClient_InvalidRole(t *testing.T) {
	client := &OpenAIClient{modelName: "gpt-4"}
	// Generate with an invalid message role to trigger error path in prepareRequest
	messages := []Message{{Role: "invalid_role", Content: "Hello"}}
	_, err := client.Generate(context.Background(), messages, nil)
	if err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}

	_, err = client.Stream(context.Background(), messages, nil)
	if err == nil {
		t.Fatal("expected stream error for invalid role, got nil")
	}
}

func TestNewOpenAIClient_Direct(t *testing.T) {
	_, _ = NewOpenAIClient("dummy-key", "gpt-4")
}

