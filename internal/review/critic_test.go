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
func handleMcpCritic() {
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

func handleGit(args []string) {
	if len(args) > 2 && args[1] == "status" && args[2] == "--porcelain" {
		os.Exit(0)
	}
	if len(args) > 2 && args[1] == "rev-parse" {
		if args[2] == "--show-toplevel" {
			_, _ = fmt.Fprintln(os.Stdout, ".")
			os.Exit(0)
		}
		if args[2] == "--is-inside-work-tree" {
			_, _ = fmt.Fprintln(os.Stdout, "true")
			os.Exit(0)
		}
	}
	if len(args) > 1 {
		_, _ = fmt.Fprintln(os.Stdout, strings.Join(args[1:], " "))
	}
	os.Exit(0)
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
	switch cmd {
	case "pw-mcp-critic":
		handleMcpCritic()
	case "git":
		handleGit(args)
	case "make":
		_, _ = fmt.Fprint(os.Stdout, "mock make all success")
		os.Exit(0)
	case "gh":
		_, _ = fmt.Fprint(os.Stdout, `{"body": "### Goal\ntest\n### Proposed Changes\nchanges\n### Verification Plan\nverif"}`)
		os.Exit(0)
	case "gh_invalid":
		_, _ = fmt.Fprint(os.Stdout, `invalid json`)
		os.Exit(0)
	case "echo":
		if len(args) > 1 {
			_, _ = fmt.Fprintln(os.Stdout, strings.Join(args[1:], " "))
		}
		os.Exit(0)
	case "true":
		os.Exit(0)
	case "fail":
		os.Exit(1)
	}
	os.Exit(1)
}

func TestVerifyWorkspace_InvalidConfig(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
	}
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec("fail", args)
	}
	defer func() { execCommand = origExec }()

	_, err := LoadIssuePlan(context.Background(), "123")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestVerifyWorkspace_NoChanges(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
	}
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
	}
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		if command == "make" {
			return mockReviewExec("make", []string{"fail"})
		}
		return mockReviewExec(command, args)
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
	}
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec("gh", []string{"gh_invalid"})
	}
	defer func() { execCommand = origExec }()

	_, err := LoadIssuePlan(context.Background(), "123")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestVerifyWorkspace_Success(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
	}
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
	}
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		return mockReviewExec(command, args)
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
	execCommand = func(ctx context.Context, command string, args ...string) Cmd {
		if command == "make" {
			return mockReviewExec("make", []string{"fail"})
		}
		return mockReviewExec(command, args)
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

func TestResolveCriticServerConfig_FromServerMap(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"critic": {Command: "custom-critic", Args: []string{"--verbose"}},
		},
	}
	srvCfg, err := resolveCriticServerConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srvCfg.Command != "custom-critic" {
		t.Errorf("expected command 'custom-critic', got '%s'", srvCfg.Command)
	}
}

func TestResolveCriticServerConfig_NotInPathOrBin(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{}, // no critic key
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tmp, err := os.MkdirTemp("", "critic-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if chdirErr := os.Chdir(tmp); chdirErr != nil {
		t.Fatal(chdirErr)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	t.Setenv("PATH", "")

	_, err = resolveCriticServerConfig(cfg)
	if err == nil {
		t.Error("expected error when pw-mcp-critic is not in PATH or bin/")
	} else if !strings.Contains(err.Error(), "pw-mcp-critic not found") {
		t.Errorf("expected 'pw-mcp-critic not found' error, got: %v", err)
	}
}

func TestExtractTextContent_WithTextContent(t *testing.T) {
	content := []mcp.Content{
		&mcp.TextContent{Text: "hello world"},
	}
	got := extractTextContent(content)
	if got != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", got)
	}
}

func TestExtractTextContent_Empty(t *testing.T) {
	got := extractTextContent(nil)
	if got != "" {
		t.Errorf("expected empty string, got '%s'", got)
	}
}

func TestExtractTextContent_NonTextContent(t *testing.T) {
	// Pass a non-TextContent item — should return ""
	content := []mcp.Content{
		&mcp.ImageContent{Data: []byte("base64data"), MIMEType: "image/png"},
	}
	got := extractTextContent(content)
	if got != "" {
		t.Errorf("expected empty string for non-text content, got '%s'", got)
	}
}

func TestParseCriticVerdict_Accept(t *testing.T) {
	err := parseCriticVerdict("Everything looks good. VERDICT: ACCEPT")
	if err != nil {
		t.Errorf("expected nil for ACCEPT verdict, got: %v", err)
	}
}

func TestParseCriticVerdict_Reject(t *testing.T) {
	err := parseCriticVerdict("Missing tests. VERDICT: REJECT")
	if err == nil {
		t.Error("expected error for REJECT verdict, got nil")
	}
}

func TestParseCriticVerdict_WithMarkdownDecorations(t *testing.T) {
	// Should strip surrounding markdown decorators before checking suffix
	err := parseCriticVerdict("**VERDICT: ACCEPT**")
	if err != nil {
		t.Errorf("expected nil after stripping markdown, got: %v", err)
	}
}

func TestRunLocalValidation_Empty(t *testing.T) {
	err := runLocalValidation(context.Background(), "")
	if err != nil {
		t.Errorf("expected nil for empty validation command, got: %v", err)
	}
}

func TestResolveValidationCmd_NoMakefile(t *testing.T) {
	origCheck := checkMakefileExists
	checkMakefileExists = func() bool { return false }
	defer func() { checkMakefileExists = origCheck }()

	cfg := &config.Config{EnableCritic: false}
	got := resolveValidationCmd(cfg)
	if got != "" {
		t.Errorf("expected empty string when no Makefile, got '%s'", got)
	}
}

func TestResolveValidationCmd_WithMakefile_CriticEnabled(t *testing.T) {
	origCheck := checkMakefileExists
	checkMakefileExists = func() bool { return true }
	defer func() { checkMakefileExists = origCheck }()

	cfg := &config.Config{EnableCritic: true}
	got := resolveValidationCmd(cfg)
	if got != "make all" {
		t.Errorf("expected 'make all', got '%s'", got)
	}
}
