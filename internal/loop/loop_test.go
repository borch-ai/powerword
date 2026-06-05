package loop

import (
	"context"
	"errors"
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
