package loop

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/llm"
	"github.com/borch-ai/powerword/pkg/telemetry"
)

type mockLLMClient struct {
	genResps []*llm.Message
	genErr   error
	calls    int
}

func (m *mockLLMClient) Generate(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition, opts ...llm.GenerateOption) (*llm.Message, error) {
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
		Verbose:           true,
		Model:             "test",
		MaxLoopIterations: 3,
	}

	err := RunLoop(ctx, cfg, "test prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if mockClient.calls != 1 {
		t.Fatalf("expected Generate to be called 1 time, got: %d", mockClient.calls)
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
			{
				Content: "Assistant response",
				Usage:   &llm.TokenUsage{InputTokens: 10, OutputTokens: 20},
			},
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
						Name:      "write_tool",
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

func TestReadStdinPrompt(t *testing.T) {
	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	_, _ = w.WriteString("piped input\n")
	_ = w.Close()

	prompt, err := readStdinPrompt()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt != "piped input" {
		t.Errorf("expected 'piped input', got '%s'", prompt)
	}
}

func TestPrintJSONPayload(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	tracker := telemetry.NewUsageTracker()
	tracker.RecordUsage("test", telemetry.TokenUsage{InputTokens: 10, OutputTokens: 20})
	printJSONPayload(nil, []llm.Message{
		{Role: llm.RoleAssistant, Content: "Hello"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{Name: "test"}}},
	}, 0, tracker)

	_ = w.Close()
	os.Stdout = oldStdout

	outBytes, _ := io.ReadAll(r)
	_ = r.Close()
	output := string(outBytes)

	if !strings.Contains(output, "Hello") {
		t.Errorf("expected payload to contain 'Hello', got: %s", output)
	}
	if !strings.Contains(output, "success") {
		t.Errorf("expected payload to contain 'success', got: %s", output)
	}
	if !strings.Contains(output, `"usage":`) {
		t.Errorf("expected payload to contain 'usage', got: %s", output)
	}
}

func TestRunLoop_JSONOutput(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{Role: llm.RoleAssistant, Content: "JSON response"},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		MaxLoopIterations: 3,
		JSONOutput:        true,
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err := RunLoop(ctx, cfg, "test JSON prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	_ = w.Close()
	os.Stdout = oldStdout

	outBytes, _ := io.ReadAll(r)
	_ = r.Close()
	output := string(outBytes)

	if !strings.Contains(output, "JSON response") {
		t.Errorf("expected JSON payload to contain generated response, got: %s", output)
	}
}

func TestRunLoop_Headless(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Wait, I will call a tool",
				ToolCalls: []llm.ToolCall{
					{
						ID:        "call_1",
						Name:      "write_tool",
						Arguments: `{"arg":"val"}`,
					},
				},
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		MaxLoopIterations: 3,
		Headless:          true,
	}

	err := RunLoop(ctx, cfg, "test headless prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestGetOutputWriter(t *testing.T) {
	cfg := &config.Config{}

	// Default
	w := getOutputWriter(cfg)
	if w != os.Stdout {
		t.Errorf("expected os.Stdout, got %v", w)
	}

	// JSONOutput
	cfg.JSONOutput = true
	w = getOutputWriter(cfg)
	if w != io.Discard {
		t.Errorf("expected io.Discard, got %v", w)
	}

	// Custom OutputWriter
	custom := io.Discard
	cfg.OutputWriter = custom
	w = getOutputWriter(cfg)
	if w != custom {
		t.Errorf("expected custom writer, got %v", w)
	}
}

func TestRunLoop_TelemetrySummary(t *testing.T) {
	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Content: "Hello world!",
				Usage:   &llm.TokenUsage{InputTokens: 100000, OutputTokens: 200000},
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	ctx := context.Background()
	cfg := &config.Config{
		Verbose:           true,
		Model:             "test-model",
		MaxLoopIterations: 3,
		Pricing: map[string]telemetry.ModelPricing{
			"test-model": {Input: 1.0, Output: 2.0}, // $1.00 / 1M input, $2.00 / 1M output
		},
	}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := RunLoop(ctx, cfg, "test prompt")

	_ = w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	outBytes, _ := io.ReadAll(r)
	_ = r.Close()
	output := string(outBytes)

	// Estimated cost: 100k * 1 / 1M + 200k * 2 / 1M = 0.1 + 0.4 = $0.50
	if !strings.Contains(output, "Estimated Cost: $0.50000") {
		t.Errorf("expected telemetry summary to contain 'Estimated Cost: $0.50000', got output: %s", output)
	}
	if !strings.Contains(output, "Total Tokens: 300000 (100000 In, 200000 Out)") {
		t.Errorf("expected telemetry summary to contain correct token counts, got output: %s", output)
	}
}

func TestRunLoop_PauseSession_Interactive(t *testing.T) {
	setupTestSessions(t)

	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Role:    llm.RoleAssistant,
				Content: "Let me delete a file",
				ToolCalls: []llm.ToolCall{
					{
						ID:        "call_pause_1",
						Name:      "delete_tool",
						Arguments: `{"path":"somefile"}`,
					},
				},
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	// Mock stdin to input 'p' to pause
	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	_, _ = w.WriteString("p\n")
	_ = w.Close()

	ctx := context.Background()
	cfg := &config.Config{
		Session:           "paused-session",
		MaxLoopIterations: 3,
		AutoConfirm:       false,
	}

	err := RunLoop(ctx, cfg, "run delete and pause")
	if err != nil {
		t.Fatalf("expected no error (graceful pause), got: %v", err)
	}

	// Load session and verify state
	session, err := LoadSession("paused-session")
	if err != nil {
		t.Fatalf("expected session to exist, got: %v", err)
	}
	// The messages should contain the user prompt and the assistant tool call message,
	// but no tool response because it was paused.
	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages in paused session, got %d", len(session.Messages))
	}
	if session.Messages[1].Role != llm.RoleAssistant || len(session.Messages[1].ToolCalls) != 1 {
		t.Errorf("expected last message to be assistant with tool calls, got: %+v", session.Messages[1])
	}
}

func TestRunLoop_ResumeSession_Success(t *testing.T) {
	setupTestSessions(t)

	// Pre-create a paused session
	pausedSession := &Session{
		ID:    "resumable-session",
		Model: "test-model",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "run delete and pause"},
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{
						ID:        "call_pause_1",
						Name:      "delete_tool",
						Arguments: `{"path":"somefile"}`,
					},
				},
			},
		},
	}
	if err := SaveSession(pausedSession); err != nil {
		t.Fatalf("failed to save paused session: %v", err)
	}

	oldNewClient := newClient
	defer func() { newClient = oldNewClient }()

	mockClient := &mockLLMClient{
		genResps: []*llm.Message{
			{
				Role:    llm.RoleAssistant,
				Content: "Deletion completed successfully!",
			},
		},
	}
	newClient = func(cfg *config.Config) (llm.LLMClient, error) {
		return mockClient, nil
	}

	// Mock stdin to input 'y' to allow the tool execution on resume
	r, w, _ := os.Pipe()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	_, _ = w.WriteString("y\n")
	_ = w.Close()

	ctx := context.Background()
	cfg := &config.Config{
		Resume:            "resumable-session",
		Model:             "test-model",
		MaxLoopIterations: 3,
		AutoConfirm:       false,
	}

	err := RunLoop(ctx, cfg, "") // No prompt
	if err != nil {
		t.Fatalf("expected no error on resume, got: %v", err)
	}

	// Load session and verify it completed
	session, err := LoadSession("resumable-session")
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	// Expected messages:
	// 0: User: run delete and pause
	// 1: Assistant: delete_tool (ToolCalls)
	// 2: Tool: (result of delete_tool)
	// 3: Assistant: Deletion completed successfully!
	if len(session.Messages) != 4 {
		t.Fatalf("expected 4 messages in resumed session, got %d", len(session.Messages))
	}
	if session.Messages[2].Role != llm.RoleTool || session.Messages[2].ToolCallID != "call_pause_1" {
		t.Errorf("expected third message to be tool response for call_pause_1, got: %+v", session.Messages[2])
	}
	if session.Messages[3].Content != "Deletion completed successfully!" {
		t.Errorf("expected final message to be completion message, got: %q", session.Messages[3].Content)
	}
}

func TestRunLoop_ResumeSession_EmptyOrNonExistent(t *testing.T) {
	setupTestSessions(t)

	ctx := context.Background()
	cfg := &config.Config{
		Resume: "non-existent-session",
	}

	err := RunLoop(ctx, cfg, "")
	if err == nil {
		t.Fatal("expected error resuming non-existent session, got nil")
	}
	if !strings.Contains(err.Error(), "cannot resume empty or non-existent session") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunLoop_ResumeSession_WithPromptError(t *testing.T) {
	setupTestSessions(t)

	ctx := context.Background()
	cfg := &config.Config{
		Resume: "some-session",
	}

	err := RunLoop(ctx, cfg, "some prompt")
	if err == nil {
		t.Fatal("expected error when prompt is provided to resume, got nil")
	}
	if !strings.Contains(err.Error(), "cannot provide a prompt when resuming a session") {
		t.Errorf("unexpected error: %v", err)
	}
}
