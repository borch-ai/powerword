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
	chunks []llm.StreamChunk
	err    error
}

func (m *mockLLMClient) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Message, error) {
	return nil, nil
}

func (m *mockLLMClient) Stream(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (<-chan llm.StreamChunk, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan llm.StreamChunk, len(m.chunks))
	for _, chunk := range m.chunks {
		ch <- chunk
	}
	close(ch)
	return ch, nil
}

func (m *mockLLMClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

func TestRunLoop_Success(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		chunks: []llm.StreamChunk{
			{Content: "Hello "},
			{Content: "world!"},
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

func TestRunLoop_StreamStartError(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		err: errors.New("stream start error"),
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{}

	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected stream start error, got nil")
	}
	if !strings.Contains(err.Error(), "stream start error") {
		t.Errorf("expected error to mention 'stream start error', got: %v", err)
	}
}

func TestRunLoop_StreamChunkError(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		chunks: []llm.StreamChunk{
			{Content: "Hello "},
			{Error: errors.New("chunk error")},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{}

	err := RunLoop(ctx, cfg, "test prompt")
	if err == nil {
		t.Fatal("expected stream chunk error, got nil")
	}
	if !strings.Contains(err.Error(), "chunk error") {
		t.Errorf("expected error to mention 'chunk error', got: %v", err)
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
	_ = SaveSession(session)

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
		chunks: []llm.StreamChunk{
			{Content: "Assistant response"},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Session: "existing-session",
		Model:   "test-model",
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

	importOSAndFilepath := func() {
		// Just to ensure os and filepath are imported if not used elsewhere, but they are.
	}
	_ = importOSAndFilepath

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
}
