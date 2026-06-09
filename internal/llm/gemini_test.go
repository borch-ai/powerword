package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func TestGeminiClient_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify endpoint is called
		if !strings.Contains(strings.ToLower(r.URL.Path), "content") {
			t.Errorf("unexpected endpoint path: %s", r.URL.Path)
		}

		resp := []any{
			map[string]any{
				"candidates": []any{
					map[string]any{
						"content": map[string]any{
							"parts": []any{
								map[string]any{
									"text": "Hello from Gemini mock!",
								},
							},
							"role": "model",
						},
					},
				},
				"usageMetadata": map[string]any{
					"promptTokenCount":        10,
					"candidatesTokenCount":    20,
					"cachedContentTokenCount": 5,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	opts := []option.ClientOption{
		option.WithEndpoint(server.URL),
		option.WithAPIKey("dummy-key"),
	}

	client, err := NewGeminiClientWithOpts("gemini-1.5-pro", opts...)
	if err != nil {
		t.Fatal(err)
	}

	messages := []Message{
		{Role: RoleSystem, Content: "system instruction"},
		{Role: RoleUser, Content: "Hello"},
	}

	msg, err := client.Generate(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if msg.Content != "Hello from Gemini mock!" {
		t.Errorf("unexpected content: %s", msg.Content)
	}

	if msg.Usage == nil || msg.Usage.InputTokens != 10 || msg.Usage.OutputTokens != 20 || msg.Usage.CachedTokens != 5 {
		t.Errorf("unexpected usage: %+v", msg.Usage)
	}
}

func TestGeminiClient_Generate_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []any{
			map[string]any{
				"candidates": []any{
					map[string]any{
						"content": map[string]any{
							"parts": []any{
								map[string]any{
									"functionCall": map[string]any{
										"name": "test_tool",
										"args": map[string]any{
											"arg1": "val1",
										},
									},
								},
							},
							"role": "model",
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	opts := []option.ClientOption{
		option.WithEndpoint(server.URL),
		option.WithAPIKey("dummy-key"),
	}

	client, err := NewGeminiClientWithOpts("gemini-1.5-pro", opts...)
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
				"required": []string{"arg1"},
			},
		},
	}

	messages := []Message{
		{Role: RoleUser, Content: "Call tool"},
		{
			Role:       RoleTool,
			ToolCallID: "test_tool-0",
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
	if tc.Name != "test_tool" || tc.ID != "test_tool-0" || tc.Arguments != `{"arg1":"val1"}` {
		t.Errorf("unexpected tool call values: %+v", tc)
	}
}

func TestGeminiClient_Generate_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid request"}}`))
	}))
	defer server.Close()

	opts := []option.ClientOption{
		option.WithEndpoint(server.URL),
		option.WithAPIKey("dummy-key"),
	}

	client, err := NewGeminiClientWithOpts("gemini-1.5-pro", opts...)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Generate(context.Background(), []Message{{Role: RoleUser, Content: "Hello"}}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGeminiClient_Stream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Write streamed chunks as a JSON array of responses
		_, _ = w.Write([]byte(`[`))
		w.(http.Flusher).Flush()
		time.Sleep(10 * time.Millisecond)

		_, _ = w.Write([]byte(`{"candidates": [{"content": {"parts": [{"text": "Hello "}]}}]},`))
		w.(http.Flusher).Flush()
		time.Sleep(10 * time.Millisecond)

		_, _ = w.Write([]byte(`{"candidates": [{"content": {"parts": [{"text": "Gemini!"}]}}]}`))
		w.(http.Flusher).Flush()
		time.Sleep(10 * time.Millisecond)

		_, _ = w.Write([]byte(`]`))
		w.(http.Flusher).Flush()
	}))
	defer server.Close()

	opts := []option.ClientOption{
		option.WithEndpoint(server.URL),
		option.WithAPIKey("dummy-key"),
	}

	client, err := NewGeminiClientWithOpts("gemini-1.5-pro", opts...)
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

	expected := []string{"Hello ", "Gemini!"}
	if len(results) != len(expected) || results[0] != expected[0] || results[1] != expected[1] {
		t.Errorf("unexpected stream results: %+v", results)
	}
}

func TestGemini_ConvertSchema(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"str":  map[string]any{"type": "string", "description": "a string"},
			"num":  map[string]any{"type": "number"},
			"int":  map[string]any{"type": "integer"},
			"bool": map[string]any{"type": "boolean"},
			"arr": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"enumVal": map[string]any{
				"type": "string",
				"enum": []any{"a", "b"},
			},
		},
		"required": []any{"str"},
	}

	schema, err := convertSchema(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if schema.Type != genai.TypeObject {
		t.Errorf("expected TypeObject, got %v", schema.Type)
	}

	// Test nil input
	nilSchema, err := convertSchema(nil)
	if err != nil || nilSchema != nil {
		t.Errorf("expected nil schema, got %v, error: %v", nilSchema, err)
	}
}

func TestNewGeminiClient_Direct(t *testing.T) {
	// Call NewGeminiClient directly to cover that branch.
	client, err := NewGeminiClient("dummy-key", "gemini-1.5-pro")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to not be nil")
	}
}

func TestGemini_CoverageBonus(t *testing.T) {
	// 1. NewGeminiClientWithOpts error
	_, err := NewGeminiClientWithOpts("model", option.WithGRPCConnectionPool(-1))
	if err == nil {
		t.Error("expected error for invalid option")
	}

	client := &GeminiClient{modelName: "gemini-1.5-pro"}

	// 2. convertSchema error using channel (unmarshalable type)
	_, err = convertSchema(make(chan int))
	if err == nil {
		t.Error("expected error for unmarshalable schema")
	}

	// 3. msgToParts tool call invalid JSON args
	invalidMsg := Message{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{
			{ID: "1", Name: "tool", Arguments: "{invalid"},
		},
	}
	_, err = client.msgToParts(invalidMsg, nil)
	if err == nil {
		t.Error("expected error for invalid tool call arguments")
	}

	// 4. msgToContent error propagation
	_, err = client.msgToContent(invalidMsg, nil)
	if err == nil {
		t.Error("expected error in msgToContent")
	}

	// 5. msgToParts RoleTool non-JSON content
	toolMsg := Message{
		Role:       RoleTool,
		ToolCallID: "tool",
		Content:    "non-json",
	}
	parts, err := client.msgToParts(toolMsg, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}

	// 6. findFunctionName fallback
	name := client.findFunctionName(nil, "fallback")
	if name != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", name)
	}
}

func TestGeminiClient_Stream_Cancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"candidates": [{"content": {"parts": [{"text": "Hello "}]}}]}]`))
	}))
	defer server.Close()

	opts := []option.ClientOption{
		option.WithEndpoint(server.URL),
		option.WithAPIKey("dummy-key"),
	}

	client, err := NewGeminiClientWithOpts("gemini-1.5-pro", opts...)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	ch, err := client.Stream(ctx, []Message{{Role: RoleUser, Content: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected stream error: %v", err)
	}

	// Drain the channel and check for error
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

func TestGeminiClient_ListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"models": []map[string]any{
				{"name": "models/gemini-pro"},
				{"name": "models/gemini-1.5-pro"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	opts := []option.ClientOption{
		option.WithEndpoint(server.URL),
		option.WithAPIKey("dummy-key"),
	}

	client, err := NewGeminiClientWithOpts("gemini-1.5-pro", opts...)
	if err != nil {
		t.Fatal(err)
	}

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 || models[0] != "models/gemini-pro" || models[1] != "models/gemini-1.5-pro" {
		t.Errorf("unexpected models: %v", models)
	}
}

func TestGeminiClient_ListModels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	opts := []option.ClientOption{
		option.WithEndpoint(server.URL),
		option.WithAPIKey("dummy-key"),
	}

	client, err := NewGeminiClientWithOpts("gemini-1.5-pro", opts...)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
