package critic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/pkg/config"
)

func TestSetupServer(t *testing.T) {
	cfg := &config.Config{}
	srv, err := SetupServer(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}
	if srv == nil {
		t.Fatal("expected server to be non-nil")
	}
}

func TestReviewWorkspace_ValidationCommandAndLLM(t *testing.T) {
	// 1. Mock ExtractGitDiff
	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "mock diff content", nil
	}
	defer func() { ExtractGitDiff = origExtract }()

	// 2. Mock execCommand inside critic package
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		// Just run echo to succeed
		return exec.CommandContext(ctx, "echo", "validation success")
	}
	defer func() { execCommand = origExec }()

	// 3. Mock LLM server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices": [{"message": {"role": "assistant", "content": "All good. VERDICT: ACCEPT"}}]}`)
	}))
	defer ts.Close()

	cfg := &config.Config{
		CriticProvider: "openai",
		CriticModel:    "gpt-4",
		CriticEndpoint: ts.URL,
		APIKeys: config.APIKeys{
			OpenAI: "dummy-key",
		},
	}

	srv, err := SetupServer(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(fmt.Errorf("server run err: %v", runErr))
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "review_workspace",
		Arguments: map[string]interface{}{
			"plan_content":       "My plan content",
			"validation_command": "make all",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if res.IsError {
		t.Fatalf("expected no tool error, got: %v", res.Content)
	}

	if len(res.Content) == 0 {
		t.Fatal("expected content in result")
	}

	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "VERDICT: ACCEPT") {
		t.Errorf("expected response to contain VERDICT: ACCEPT, got: %q", text)
	}
}

func TestReviewWorkspace_ValidationCommandFails(t *testing.T) {
	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "mock diff content", nil
	}
	defer func() { ExtractGitDiff = origExtract }()

	// Mock execCommand to run a command that exits with error code 1
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		// On Unix/Mac, executing false will exit with 1
		return exec.CommandContext(ctx, "false")
	}
	defer func() { execCommand = origExec }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The prompt should have validation command failure output.
		_, _ = fmt.Fprintln(w, `{"choices": [{"message": {"role": "assistant", "content": "Failed compilation. VERDICT: REJECT"}}]}`)
	}))
	defer ts.Close()

	cfg := &config.Config{
		CriticProvider: "openai",
		CriticModel:    "gpt-4",
		CriticEndpoint: ts.URL,
		APIKeys: config.APIKeys{
			OpenAI: "dummy-key",
		},
	}

	srv, err := SetupServer(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, t1)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "review_workspace",
		Arguments: map[string]interface{}{
			"plan_content":       "My plan content",
			"validation_command": "make all",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if res.IsError {
		t.Fatalf("expected no tool error, got: %v", res.Content)
	}

	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "VERDICT: REJECT") {
		t.Errorf("expected response to contain VERDICT: REJECT, got: %q", text)
	}
}

func TestReviewWorkspace_NoPlanContent(t *testing.T) {
	cfg := &config.Config{}
	srv, err := SetupServer(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, t1)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "review_workspace",
		Arguments: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if !res.IsError {
		t.Error("expected error for empty plan_content")
	}
}

func TestReviewWorkspace_NoDiff(t *testing.T) {
	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "", nil // no diff
	}
	defer func() { ExtractGitDiff = origExtract }()

	cfg := &config.Config{}
	srv, err := SetupServer(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, t1)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "review_workspace",
		Arguments: map[string]interface{}{
			"plan_content": "some plan",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if res.IsError {
		t.Fatalf("expected no tool error, got: %v", res.Content)
	}

	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No local changes found to review") {
		t.Errorf("expected no local changes message, got: %q", text)
	}
}

func TestReviewWorkspace_GitDiffError(t *testing.T) {
	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "", errors.New("git command failed")
	}
	defer func() { ExtractGitDiff = origExtract }()

	cfg := &config.Config{}
	srv, err := SetupServer(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, t1)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "review_workspace",
		Arguments: map[string]interface{}{
			"plan_content": "some plan",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if !res.IsError {
		t.Error("expected error for git diff failure")
	}
}
