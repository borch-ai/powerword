package review

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/borch-ai/powerword/internal/config"
	"github.com/borch-ai/powerword/internal/llm"
	"github.com/borch-ai/powerword/internal/loop"
)

func TestRunAutonomousLoop_MissingIssue(t *testing.T) {
	cfg := &config.Config{}
	err := RunAutonomousLoop(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "--issue flag is required") {
		t.Errorf("expected error about missing issue, got: %v", err)
	}
}

func TestBuildPrompt(t *testing.T) {
	plan := &Plan{
		Goal:         "goal",
		Changes:      "changes",
		Verification: "verify",
	}

	prompt := buildPrompt(plan, "", nil, nil)
	if !strings.Contains(prompt, "No changes yet") {
		t.Errorf("expected 'No changes yet', got: %s", prompt)
	}
	if !strings.Contains(prompt, "Local Validation Passed") {
		t.Errorf("expected 'Local Validation Passed', got: %s", prompt)
	}

	promptDiff := buildPrompt(plan, "diff content", []byte("make error"), fmt.Errorf("error"))
	if !strings.Contains(promptDiff, "diff content") {
		t.Errorf("expected diff content, got: %s", promptDiff)
	}
	if !strings.Contains(promptDiff, "Local Validation Failed") {
		t.Errorf("expected validation failed, got: %s", promptDiff)
	}
}

func TestProcessTurnCompletion_NoSession(t *testing.T) {
	// Setup isolated sessions dir
	tmpDir := t.TempDir()
	origSessionsDir := loop.SessionsBaseDir
	loop.SessionsBaseDir = tmpDir
	defer func() { loop.SessionsBaseDir = origSessionsDir }()

	cfg := &config.Config{Issue: "123"}
	autoCfg := &config.Config{Session: "non-existent"}

	completed, err := processTurnCompletion(context.Background(), cfg, autoCfg, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if completed {
		t.Errorf("expected not completed")
	}
}

func TestProcessTurnCompletion_Complete(t *testing.T) {
	origExec := execCommand
	execCommand = mockExecCommandContext
	defer func() { execCommand = origExec }()

	tmpDir := t.TempDir()
	origSessionsDir := loop.SessionsBaseDir
	loop.SessionsBaseDir = tmpDir
	defer func() { loop.SessionsBaseDir = origSessionsDir }()

	autoCfg := &config.Config{Session: "test-session"}
	cfg := &config.Config{Issue: "123"}

	session := &loop.Session{
		ID:        "test-session",
		Timestamp: time.Now(),
		Messages: []llm.Message{
			{Role: llm.RoleAssistant, Content: "I am done. TASK COMPLETE"},
		},
	}
	_ = loop.SaveSession(session)

	completed, err := processTurnCompletion(context.Background(), cfg, autoCfg, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !completed {
		t.Errorf("expected completed to be true")
	}
}

func TestRunAutonomousLoop_SuccessOnFirstTurn(t *testing.T) {
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

	// Overwrite RunLoop to mock agent behavior
	origRunLoop := loopRunLoop
	loopRunLoop = func(ctx context.Context, cfg *config.Config, prompt string) error {
		// Mock agent saving TASK COMPLETE
		session := &loop.Session{
			ID:        cfg.Session,
			Timestamp: time.Now(),
			Messages: []llm.Message{
				{Role: llm.RoleAssistant, Content: "TASK COMPLETE"},
			},
		}
		_ = loop.SaveSession(session)
		return nil
	}
	defer func() { loopRunLoop = origRunLoop }()

	tmpDir := t.TempDir()
	origSessionsDir := loop.SessionsBaseDir
	loop.SessionsBaseDir = tmpDir
	defer func() { loop.SessionsBaseDir = origSessionsDir }()

	cfg := &config.Config{
		Issue: "123",
	}

	err := RunAutonomousLoop(context.Background(), cfg)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestRunAutonomousLoop_LoadIssuePlanError(t *testing.T) {
	origExec := execCommand
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		return mockExecCommandContext(ctx, "fail", args...)
	}
	defer func() { execCommand = origExec }()

	cfg := &config.Config{Issue: "123"}
	err := RunAutonomousLoop(context.Background(), cfg)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func TestRunAutonomousLoop_MaxIterations(t *testing.T) {
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

	origRunLoop := loopRunLoop
	loopRunLoop = func(ctx context.Context, cfg *config.Config, prompt string) error {
		return nil
	}
	defer func() { loopRunLoop = origRunLoop }()

	tmpDir := t.TempDir()
	origSessionsDir := loop.SessionsBaseDir
	loop.SessionsBaseDir = tmpDir
	defer func() { loop.SessionsBaseDir = origSessionsDir }()

	cfg := &config.Config{Issue: "123"}
	err := RunAutonomousLoop(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "reached maximum autonomous iterations") {
		t.Errorf("expected max iterations error, got %v", err)
	}
}

func TestRunAutonomousLoop_ExtractDiffError(t *testing.T) {
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
		return "", fmt.Errorf("diff error")
	}
	defer func() { ExtractGitDiff = origExtract }()

	tmpDir := t.TempDir()
	origSessionsDir := loop.SessionsBaseDir
	loop.SessionsBaseDir = tmpDir
	defer func() { loop.SessionsBaseDir = origSessionsDir }()

	cfg := &config.Config{Issue: "123"}
	err := RunAutonomousLoop(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "failed to extract git diff") {
		t.Errorf("expected extract diff error, got %v", err)
	}
}
