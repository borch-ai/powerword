package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"powerword/internal/config"
	"powerword/internal/llm"
)

type mockLLMClient struct {
	genResps []*llm.Message
	genErr   error
	calls    int
}

func (m *mockLLMClient) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Message, error) {
	if m.genErr != nil {
		return nil, m.genErr
	}
	if m.calls < len(m.genResps) {
		resp := m.genResps[m.calls]
		m.calls++
		return resp, nil
	}
	return &llm.Message{Content: "default fallback"}, nil
}

func (m *mockLLMClient) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (<-chan llm.StreamChunk, error) {
	return nil, errors.New("not implemented")
}

func (m *mockLLMClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

func TestRunLoop_Success(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{Content: "Hello world!"},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Verbose: true,
		Model:   "test",
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRunLoop_ClientCreationError(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return nil, errors.New("creation error")
	}

	ctx := context.Background()
	cfg := &config.Config{}

	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected client creation error, got nil")
	}
	if !strings.Contains(err.Error(), "creation error") {
		t.Errorf("expected error to mention 'creation error', got: %v", err)
	}
}

func TestRunLoop_GenerateError(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genErr: errors.New("generate error"),
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		MaxLoopIterations: 5,
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected generate error, got nil")
	}
	if !strings.Contains(err.Error(), "generate error") {
		t.Errorf("expected error to mention 'generate error', got: %v", err)
	}
}

func TestRunLoop_ListSessions(t *testing.T) {
	setupTestSessions(t)

	// Create a dummy session to list
	session := &Session{
		ID:       "test-list-session",
		Model:    "test",
		Messages: []llm.Message{},
	}
	if err := SaveSession(session); err != nil {
		t.Fatalf("failed to save dummy session: %v", err)
	}

	ctx := context.Background()
	cfg := &config.Config{
		ListSessions: true,
	}

	err := RunLoop(ctx, cfg, "")
	if err != nil {
		t.Fatalf("expected no error for ListSessions, got: %v", err)
	}
}

func TestRunLoop_ListSessionsEmpty(t *testing.T) {
	setupTestSessions(t)

	ctx := context.Background()
	cfg := &config.Config{
		ListSessions: true,
	}

	err := RunLoop(ctx, cfg, "")
	if err != nil {
		t.Fatalf("expected no error for ListSessionsEmpty, got: %v", err)
	}
}

func TestRunLoop_WithSession(t *testing.T) {
	setupTestSessions(t)

	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{Content: "Assistant response"},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Session:           "existing-session",
		Model:             "test-model",
		MaxLoopIterations: 5,
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify session was saved with the new messages
	session, err := LoadSession("existing-session")
	if err != nil {
		t.Fatalf("expected no error loading session, got: %v", err)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages, got: %d", len(session.Messages))
	}
	if session.Messages[0].Content != "test prompt" {
		t.Errorf("expected user prompt, got: %s", session.Messages[0].Content)
	}
	if session.Messages[1].Content != "Assistant response" {
		t.Errorf("expected assistant response, got: %s", session.Messages[1].Content)
	}
}

func TestRunLoop_WithSessionLoadError(t *testing.T) {
	dir := setupTestSessions(t)

	// Need to manually create the invalid file
	importOSDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(importOSDir, 0750); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(importOSDir, "bad-session.json"), []byte("invalid json"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	ctx := context.Background()
	cfg := &config.Config{
		Session: "bad-session",
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to unmarshal session") {
		t.Errorf("expected error to contain 'failed to unmarshal session', got: %v", err)
	}
}

func TestRunLoop_WithServers(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{Content: "Hello "},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		MaxLoopIterations: 5,
		Servers: map[string]config.ServerConfig{
			"dummy": {
				Command: "echo",
				Args:    []string{"not mcp"},
			},
		},
	}

	err := RunLoop(ctx, cfg, "test")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRunLoop_ToolCall(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Wait, I will call a tool",
				ToolCalls: []llm.ToolCall{
					{
						ID:        "call_1",
						Name:      "unknown_tool",
						Arguments: `{"arg":"val"}`,
					},
					{
						ID:        "call_2",
						Name:      "bad_json_tool",
						Arguments: `{bad_json}`,
					},
				},
			},
			{
				Content: "Okay, I got an error",
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		MaxLoopIterations: 3,
		AutoConfirm:       true,
		Verbose:           true,
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRunLoop_ToolCall_NoAutoConfirm(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Wait, I will call a tool",
				ToolCalls: []llm.ToolCall{
					{
						ID:        "call_1",
						Name:      "test_tool",
						Arguments: `{"arg":"val"}`,
					},
				},
			},
			{
				Content: "Done",
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	// Mock stdin
	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	_, _ = w.WriteString("n\n")
	_ = w.Close()

	ctx := context.Background()
	cfg := &config.Config{
		MaxLoopIterations: 3,
		AutoConfirm:       false,
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
