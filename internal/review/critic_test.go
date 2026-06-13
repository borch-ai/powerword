package review

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/internal/mcp/critic"
	"github.com/borch-ai/powerword/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func init() {
	checkMakefileExists = func() bool { return true }
}

func TestParseIssueBody(t *testing.T) {
	body := `### Goal
To build a great feature.
### Proposed Changes
- File A
- File B
### Verification Plan
Run tests.`

	plan, err := parseIssueBody(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := &Plan{
		Goal:         "To build a great feature.",
		Changes:      "- File A\n- File B",
		Verification: "Run tests.",
	}

	if !reflect.DeepEqual(plan, expected) {
		t.Errorf("expected %+v, got %+v", expected, plan)
	}
}

func TestParseIssueBody_Empty(t *testing.T) {
	_, err := parseIssueBody("This is just some random text without any headers.")
	if err == nil {
		t.Errorf("Expected error for empty plan, got nil")
	}
}

func newTestConfig(endpoint string, gitDiffVal string, openAIKey string) *config.Config {
	return &config.Config{
		EnableCritic:   true,
		CriticProvider: "openai",
		CriticModel:    "gpt-4",
		CriticEndpoint: endpoint,
		APIKeys: config.APIKeys{
			OpenAI: openAIKey,
		},
		Servers: map[string]config.ServerConfig{
			"critic": {
				Command: os.Args[0],
				Args:    []string{"-test.run=TestHelperProcess", "--", "pw-mcp-critic"},
				Env: []string{
					"GO_WANT_HELPER_PROCESS=1",
					"CRITIC_ENDPOINT=" + endpoint,
					"MOCK_GIT_DIFF=" + gitDiffVal,
					"MOCK_OPENAI_API_KEY=" + openAIKey,
				},
			},
		},
	}
}

// TestHelperProcess is used to mock exec.Command
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	cmd := args[0]
	if cmd == "pw-mcp-critic" {
		cfg := &config.Config{
			CriticProvider: "openai",
			CriticModel:    "gpt-4",
			CriticEndpoint: os.Getenv("CRITIC_ENDPOINT"),
			APIKeys: config.APIKeys{
				OpenAI: os.Getenv("MOCK_OPENAI_API_KEY"),
			},
		}
		critic.SetExecCommand(func(ctx context.Context, name string, args ...string) *exec.Cmd {
			cs := []string{"-test.run=TestHelperProcess", "--", name}
			cs = append(cs, args...)
			//nolint:gosec // this is a test helper, subprocess with dynamic arguments is safe
			cmd := exec.CommandContext(ctx, os.Args[0], cs...)
			cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
			return cmd
		})
		critic.ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
			val := os.Getenv("MOCK_GIT_DIFF")
			if val == "error" {
				return "", fmt.Errorf("git command failed")
			}
			return val, nil
		}

		srv, err := critic.SetupServer(os.TempDir(), cfg)
		if err != nil {
			os.Exit(1)
		}
		transport := &mcp.StdioTransport{}
		if err := srv.Run(context.Background(), transport); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if cmd == "make" {
		_, _ = fmt.Fprint(os.Stdout, "mock make all success")
		os.Exit(0)
	}
	if cmd == "gh" {
		_, _ = fmt.Fprint(os.Stdout, `{"body": "### Goal\ntest\n### Proposed Changes\nchanges\n### Verification Plan\nverif"}`)
		os.Exit(0)
	}
	if cmd == "gh_invalid" {
		_, _ = fmt.Fprint(os.Stdout, `invalid json`)
		os.Exit(0)
	}
	if cmd == "echo" {
		os.Exit(0)
	}
	if cmd == "fail" {
		os.Exit(1)
	}
	os.Exit(1)
}

func mockExecCommandContext(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	//nolint:gosec // this is a test helper, subprocess with dynamic arguments is safe
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)

	// Clean GOCOVERDIR to prevent helper subprocess from corrupting/writing to coverage profile
	env := os.Environ()
	var cleanEnv []string
	for _, e := range env {
		if strings.HasPrefix(e, "GOCOVERDIR=") {
			continue
		}
		cleanEnv = append(cleanEnv, e)
	}
	cleanEnv = append(cleanEnv, "GO_WANT_HELPER_PROCESS=1")
	cmd.Env = cleanEnv
	return cmd
}

func TestVerifyWorkspace_InvalidConfig(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "make" {
			return mockExecCommandContext(ctx, "make", args...)
		}
		return mockExecCommandContext(ctx, command, args...)
	}
	defer func() { execCommand = origExec }()

	plan := &Plan{Goal: "test"}
	// Missing API key in critic server config should fail client init
	cfg := newTestConfig("http://invalid", "diff", "")

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err == nil {
		t.Errorf("expected error due to invalid LLM config, got nil")
	}
}

func TestLoadIssuePlan(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	plan, err := LoadIssuePlan(context.Background(), "123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Goal != "test" {
		t.Errorf("expected Goal 'test', got '%s'", plan.Goal)
	}
}

func TestLoadIssuePlan_Error(t *testing.T) {
	// Create a mock that returns error
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		cmd := mockExecCommandContext(ctx, "fail")
		return cmd
	}
	defer func() { execCommand = origExec }()

	_, err := LoadIssuePlan(context.Background(), "123")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestVerifyWorkspace_NoChanges(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	plan := &Plan{Goal: "test"}
	cfg := newTestConfig("http://invalid", "", "dummy-key")

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err != nil {
		t.Errorf("expected nil error for no changes, got: %v", err)
	}
}

func TestVerifyWorkspace_NoMakefile(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	origCheck := checkMakefileExists
	checkMakefileExists = func() bool { return false }
	defer func() { checkMakefileExists = origCheck }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices": [{"message": {"role": "assistant", "content": "All good. VERDICT: ACCEPT"}}]}`)
	}))
	defer ts.Close()

	plan := &Plan{Goal: "test"}
	cfg := newTestConfig(ts.URL, "diff", "dummy-key")
	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err != nil {
		t.Errorf("expected nil error for no changes, got: %v", err)
	}
}

func TestVerifyWorkspace_MakeFails(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "make" {
			cmd := mockExecCommandContext(ctx, "fail")
			return cmd
		}
		return mockExecCommandContext(ctx, command, args...)
	}
	defer func() { execCommand = origExec }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices": [{"message": {"role": "assistant", "content": "Failed validation. VERDICT: REJECT"}}]}`)
	}))
	defer ts.Close()

	plan := &Plan{Goal: "test"}
	cfg := newTestConfig(ts.URL, "diff", "dummy-key")

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err == nil {
		t.Errorf("expected error for make fail, got nil")
	}
}

func TestVerifyWorkspace_GitFails(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	plan := &Plan{Goal: "test"}
	cfg := newTestConfig("http://invalid", "error", "dummy-key")

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err == nil {
		t.Errorf("expected error for git fail, got nil")
	}
}

func TestExtractGitDiff_Fallback(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"git": {
				Command: "invalid-command-does-not-exist",
			},
		},
	}
	_, err := ExtractGitDiff(context.Background(), cfg)
	if err == nil {
		t.Errorf("expected error for invalid git server, got nil")
	}
}

func TestLoadIssuePlan_InvalidJSON(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		return mockExecCommandContext(ctx, "gh_invalid")
	}
	defer func() { execCommand = origExec }()

	_, err := LoadIssuePlan(context.Background(), "123")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestVerifyWorkspace_Success(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "make" {
			return mockExecCommandContext(ctx, "make", args...) // mock make success
		}
		return mockExecCommandContext(ctx, command, args...)
	}
	defer func() { execCommand = origExec }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices": [{"message": {"role": "assistant", "content": "All good. VERDICT: ACCEPT"}}]}`)
	}))
	defer ts.Close()

	plan := &Plan{Goal: "test"}
	cfg := newTestConfig(ts.URL, "diff", "dummy-key")

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err != nil {
		t.Errorf("expected success, got error: %v", err)
	}
}

func TestVerifyWorkspace_Reject(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "make" {
			return mockExecCommandContext(ctx, "make", args...) // mock make success
		}
		return mockExecCommandContext(ctx, command, args...)
	}
	defer func() { execCommand = origExec }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices": [{"message": {"role": "assistant", "content": "Issues found. VERDICT: REJECT"}}]}`)
	}))
	defer ts.Close()

	plan := &Plan{Goal: "test"}
	cfg := newTestConfig(ts.URL, "diff", "dummy-key")

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err == nil {
		t.Errorf("expected error for reject, got nil")
	}
}

func TestVerifyWorkspace_NilInputs(t *testing.T) {
	err := VerifyWorkspace(context.Background(), nil, &config.Config{})
	if err == nil {
		t.Error("expected error for nil plan")
	}
	err = VerifyWorkspace(context.Background(), &Plan{}, nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestVerifyWorkspace_ToolError(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	plan := &Plan{Goal: "test"}
	cfg := newTestConfig(ts.URL, "diff", "dummy-key")

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err == nil {
		t.Error("expected error for tool/LLM failure")
	}
}

func TestVerifyWorkspace_CriticServerConfigFallback(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	// Create a dummy bin/pw-mcp-critic to satisfy os.Stat
	if err := os.MkdirAll("bin", 0750); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	dummyPath := "bin/pw-mcp-critic"
	if err := os.WriteFile(dummyPath, []byte(""), 0600); err != nil {
		t.Fatalf("failed to write dummy critic: %v", err)
	}
	defer func() {
		_ = os.Remove(dummyPath)
		_ = os.Remove("bin")
	}()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{}, // No critic config
	}

	// This will cover the fallback config check
	_ = VerifyWorkspace(context.Background(), plan, cfg)
}

func TestVerifyWorkspace_EnableCriticFalse(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "make" {
			return mockExecCommandContext(ctx, "make", args...) // mock make success
		}
		return mockExecCommandContext(ctx, command, args...)
	}
	defer func() { execCommand = origExec }()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{
		EnableCritic: false,
	}

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err != nil {
		t.Errorf("expected success for disabled critic, got: %v", err)
	}
}

func TestVerifyWorkspace_EnableCriticFalse_ValidationFails(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "make" {
			cmd := mockExecCommandContext(ctx, "fail") // mock make failure
			return cmd
		}
		return mockExecCommandContext(ctx, command, args...)
	}
	defer func() { execCommand = origExec }()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{
		EnableCritic: false,
	}

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err == nil {
		t.Error("expected validation command failure error, got nil")
	}
}
