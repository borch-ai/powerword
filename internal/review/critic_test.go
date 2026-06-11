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

	"powerword/internal/config"
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

	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "diff", nil
	}
	defer func() { ExtractGitDiff = origExtract }()

	plan := &Plan{Goal: "test"}
	// Missing API key should fail LLM client init
	cfg := &config.Config{Model: "openai"}

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

	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "", nil // mock no changes
	}
	defer func() { ExtractGitDiff = origExtract }()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{Model: "openai"}

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err != nil {
		t.Errorf("expected nil error for no changes, got: %v", err)
	}
}

func TestVerifyWorkspace_NoMakefile(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "", nil
	}
	defer func() { ExtractGitDiff = origExtract }()

	origCheck := checkMakefileExists
	checkMakefileExists = func() bool { return false }
	defer func() { checkMakefileExists = origCheck }()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{Model: "openai"}
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

	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "diff", nil
	}
	defer func() { ExtractGitDiff = origExtract }()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{Model: "openai"}

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err == nil {
		t.Errorf("expected error for make fail, got nil")
	}
}

func TestVerifyWorkspace_GitFails(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "", fmt.Errorf("mock git fail")
	}
	defer func() { ExtractGitDiff = origExtract }()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{Model: "openai"}

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

	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "diff", nil // mock git diff returns changes
	}
	defer func() { ExtractGitDiff = origExtract }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices": [{"message": {"role": "assistant", "content": "All good. VERDICT: ACCEPT"}}]}`)
	}))
	defer ts.Close()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{
		CriticProvider: "openai",
		CriticModel:    "gpt-4",
		CriticEndpoint: ts.URL, // Use our mock server
		APIKeys: config.APIKeys{
			OpenAI: "dummy-key",
		},
	}

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

	origExtract := ExtractGitDiff
	ExtractGitDiff = func(ctx context.Context, cfg *config.Config) (string, error) {
		return "diff", nil // mock git diff returns changes
	}
	defer func() { ExtractGitDiff = origExtract }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"choices": [{"message": {"role": "assistant", "content": "Issues found. VERDICT: REJECT"}}]}`)
	}))
	defer ts.Close()

	plan := &Plan{Goal: "test"}
	cfg := &config.Config{
		CriticProvider: "openai",
		CriticModel:    "gpt-4",
		CriticEndpoint: ts.URL, // Use our mock server
		APIKeys: config.APIKeys{
			OpenAI: "dummy-key",
		},
	}

	err := VerifyWorkspace(context.Background(), plan, cfg)
	if err == nil {
		t.Errorf("expected error for reject, got nil")
	}
}
