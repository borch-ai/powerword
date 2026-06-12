//go:build integration

package loop_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Dynamically build the powerword binary
	tmpDir, err := os.MkdirTemp("", "powerword-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(tmpDir, "powerword")
	// The binary is compiled from cmd/powerword
	cmd := exec.Command("go", "build", "-o", binaryPath, "../../cmd/powerword")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build powerword binary: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestCLI_HeadlessJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleAssistant,
						Content: "Hello from integration test mock LLM!",
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

	cmd := exec.Command(binaryPath, "test prompt", "--json", "--headless")
	cmd.Env = append(os.Environ(),
		"OPENAI_BASE_URL="+server.URL,
		"POWERWORD_OPENAI_API_KEY=dummy",
		"POWERWORD_MODEL=gpt-4",
	)

	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}

	// Verify output is valid JSON
	var payload struct {
		Response        string `json:"response"`
		ExecutionStatus string `json:"execution_status"`
		Usage           struct {
			ModelUsages map[string]struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"model_usages"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v, output: %s", err, string(out))
	}

	if payload.ExecutionStatus != "success" {
		t.Errorf("expected execution_status 'success', got %s", payload.ExecutionStatus)
	}

	if payload.Response != "Hello from integration test mock LLM!" {
		t.Errorf("unexpected response content: %s", payload.Response)
	}

	usage, ok := payload.Usage.ModelUsages["gpt-4"]
	if !ok {
		t.Fatalf("expected usage statistics for model 'gpt-4', got payload: %+v", payload)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 20 {
		t.Errorf("unexpected token usages: %+v", usage)
	}
}

func TestCLI_CustomTOMLConfig(t *testing.T) {
	// Create a temp config file
	tmpDir, err := os.MkdirTemp("", "pw-config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "powerword.toml")
	configContent := `
model = "gpt-4"
[api_keys]
openai = "dummy-api-key-from-config"

[pricing.gpt-4]
input = 0.50
output = 1.50
`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleAssistant,
						Content: "Config-based LLM response",
					},
				},
			},
			Usage: openai.Usage{
				PromptTokens:     100,
				CompletionTokens: 200,
				TotalTokens:      300,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cmd := exec.Command(binaryPath, "-c", configPath, "run custom config test", "--json", "--headless")
	cmd.Env = append(os.Environ(),
		"OPENAI_BASE_URL="+server.URL,
	)

	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to run CLI command with custom config: %v", err)
	}

	var payload struct {
		Response        string `json:"response"`
		ExecutionStatus string `json:"execution_status"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v, output: %s", err, string(out))
	}

	if payload.Response != "Config-based LLM response" {
		t.Errorf("unexpected response content: %s", payload.Response)
	}
}

func TestCLI_ExitCodes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pw-config-bad-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	badConfigPath := filepath.Join(tmpDir, "malformed.toml")
	if err := os.WriteFile(badConfigPath, []byte("this is not toml format"), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(binaryPath, "-c", badConfigPath, "prompt")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected failure exit code for malformed config, got success")
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 0 {
			t.Errorf("expected non-zero exit code for malformed config, got %d", exitErr.ExitCode())
		}
	} else {
		t.Fatalf("expected exec.ExitError, got %T", err)
	}

	if !strings.Contains(string(out), "failed to parse config file") && !strings.Contains(string(out), "toml") && !strings.Contains(string(out), "Error:") {
		t.Errorf("expected error output to mention parsing or file error, got: %s", string(out))
	}
}

func TestCLI_SessionResumability(t *testing.T) {
	tempHomeDir, err := os.MkdirTemp("", "pw-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempHomeDir)

	// Mock LLM server
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		resp := openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleAssistant,
						Content: fmt.Sprintf("Response turn %d", requestCount),
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// 1. Run first turn
	cmd1 := exec.Command(binaryPath, "prompt 1", "--session", "test-session", "--json", "--headless")
	cmd1.Env = append(os.Environ(),
		"HOME="+tempHomeDir,
		"OPENAI_BASE_URL="+server.URL,
		"POWERWORD_OPENAI_API_KEY=dummy",
		"POWERWORD_MODEL=gpt-4",
	)

	cmd1.Stderr = os.Stderr
	_, err = cmd1.Output()
	if err != nil {
		t.Fatalf("first turn failed: %v", err)
	}

	// Verify session file exists
	sessionFilePath := filepath.Join(tempHomeDir, ".local", "share", "powerword", "sessions", "test-session.json")
	if _, err := os.Stat(sessionFilePath); os.IsNotExist(err) {
		t.Fatalf("session file was not created at %s", sessionFilePath)
	}

	// Read and verify session contents
	data, err := os.ReadFile(sessionFilePath)
	if err != nil {
		t.Fatal(err)
	}

	var session struct {
		ID       string `json:"id"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("failed to parse session json: %v", err)
	}

	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(session.Messages))
	}
	if session.Messages[0].Content != "prompt 1" {
		t.Errorf("expected 'prompt 1', got %s", session.Messages[0].Content)
	}
	if session.Messages[1].Content != "Response turn 1" {
		t.Errorf("expected 'Response turn 1', got %s", session.Messages[1].Content)
	}

	// 2. Run second turn, which should resume and append to the session
	cmd2 := exec.Command(binaryPath, "prompt 2", "--session", "test-session", "--json", "--headless")
	cmd2.Env = append(os.Environ(),
		"HOME="+tempHomeDir,
		"OPENAI_BASE_URL="+server.URL,
		"POWERWORD_OPENAI_API_KEY=dummy",
		"POWERWORD_MODEL=gpt-4",
	)

	cmd2.Stderr = os.Stderr
	_, err = cmd2.Output()
	if err != nil {
		t.Fatalf("second turn failed: %v", err)
	}

	// Read and verify updated session contents
	data2, err := os.ReadFile(sessionFilePath)
	if err != nil {
		t.Fatal(err)
	}

	if err := json.Unmarshal(data2, &session); err != nil {
		t.Fatalf("failed to parse session json: %v", err)
	}

	if len(session.Messages) != 4 {
		t.Fatalf("expected 4 messages in resumed session, got %d", len(session.Messages))
	}
	if session.Messages[2].Content != "prompt 2" {
		t.Errorf("expected 'prompt 2', got %s", session.Messages[2].Content)
	}
	if session.Messages[3].Content != "Response turn 2" {
		t.Errorf("expected 'Response turn 2', got %s", session.Messages[3].Content)
	}
}
