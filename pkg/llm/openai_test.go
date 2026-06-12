package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
			Usage: openai.Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
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

	if msg.Usage == nil || msg.Usage.InputTokens != 10 || msg.Usage.OutputTokens != 20 {
		t.Errorf("unexpected usage: %+v", msg.Usage)
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
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty response") {
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
	client, err := NewOpenAIClient("dummy-key", "gpt-4")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to not be nil")
	}
}

func TestOpenAIClient_ListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4"},
				{"id": "gpt-3.5-turbo"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("dummy")
	cfg.BaseURL = server.URL
	client := NewOpenAIClientWithConfig(cfg, "gpt-4")

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 || models[0] != "gpt-4" || models[1] != "gpt-3.5-turbo" {
		t.Errorf("unexpected models: %v", models)
	}
}

func TestOpenAIClient_ListModels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := openai.DefaultConfig("dummy")
	cfg.BaseURL = server.URL
	client := NewOpenAIClientWithConfig(cfg, "gpt-4")

	_, err := client.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOpenAIClient_Generate_JSONMode(t *testing.T) {
	var capturedRequest openai.ChatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read raw request body to capture serialization
		var raw map[string]interface{}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		_ = json.Unmarshal(bodyBytes, &raw)
		_ = json.Unmarshal(bodyBytes, &capturedRequest)

		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleAssistant,
						Content: `{"status": "ok"}`,
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

	messages := []Message{
		{Role: RoleUser, Content: "Hello!"},
	}

	_, err := client.Generate(context.Background(), messages, nil, WithResponseMIMEType("application/json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedRequest.ResponseFormat == nil {
		t.Fatal("expected ResponseFormat to be set, got nil")
	}
	if capturedRequest.ResponseFormat.Type != openai.ChatCompletionResponseFormatTypeJSONObject {
		t.Errorf("expected ResponseFormat type %s, got %s",
			openai.ChatCompletionResponseFormatTypeJSONObject, capturedRequest.ResponseFormat.Type)
	}
}
