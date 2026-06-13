package linter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHandleLintPlans_Success(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	dummyFile := filepath.Join(tmpDir, "some_file.go")
	if err := os.WriteFile(dummyFile, []byte("package main"), 0600); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	planContent := `# plan: Task 1.1: Test Plan
**Status:** Open

## User Review Required
None.

## Proposed Changes
#### [MODIFY] [some_file.go](file://../some_file.go)
- Edit it.

## Verification Plan
### Automated Tests
- Run tests.
`
	if err := os.WriteFile(filepath.Join(plansDir, "task_1_1.md"), []byte(planContent), 0600); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "lint_plans",
			Arguments: json.RawMessage(`{"workspace_root": "` + strings.ReplaceAll(tmpDir, "\\", "\\\\") + `"}`),
		},
	}

	res, err := handleLintPlans(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error calling tool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error flag: %v", res.Content[0])
	}

	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content output")
	}

	var result lintPlansResult
	if err := json.Unmarshal([]byte(tc.Text), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected valid=true, got false")
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got: %v", result.Errors)
	}
}

func TestHandleLintPlans_Failures(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	planContent := `# plan: Task 1.1: Mismatched Label
**Status:** Open

## User Review Required
None.

## Proposed Changes
#### [MODIFY] [wrong.go](file://../correct.go)
- Edit it.

## Verification Plan
`
	if err := os.WriteFile(filepath.Join(plansDir, "task_1_1.md"), []byte(planContent), 0600); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "lint_plans",
			Arguments: json.RawMessage(`{"workspace_root": "` + strings.ReplaceAll(tmpDir, "\\", "\\\\") + `"}`),
		},
	}

	res, err := handleLintPlans(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error calling tool: %v", err)
	}

	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content output")
	}

	var result lintPlansResult
	if err := json.Unmarshal([]byte(tc.Text), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if result.Valid {
		t.Errorf("expected valid=false, got true")
	}
	if len(result.Errors) == 0 {
		t.Errorf("expected validation errors, got none")
	}

	foundLabelError := false
	for _, e := range result.Errors {
		if e.Rule == "link_label" {
			foundLabelError = true
		}
	}
	if !foundLabelError {
		t.Errorf("expected link_label rule violation in errors, got: %+v", result.Errors)
	}
}

func TestSetupServer(t *testing.T) {
	srv := SetupServer()
	if srv == nil {
		t.Fatal("expected SetupServer to return a valid server")
	}
}

func TestParseValidationError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected lintError
	}{
		{
			name:  "standard relative Unix path with line",
			input: "plans/task.md:12: path must be relative, not absolute",
			expected: lintError{
				File:    "plans/task.md",
				Line:    12,
				Message: "path must be relative, not absolute",
				Rule:    "relative_links",
			},
		},
		{
			name:  "standard relative Unix path without line",
			input: "plans/task.md: missing heading \"## Proposed Changes\"",
			expected: lintError{
				File:    "plans/task.md",
				Line:    0,
				Message: "missing heading \"## Proposed Changes\"",
				Rule:    "plan_structure",
			},
		},
		{
			name:  "Windows path with drive letter and line number",
			input: `C:\repo\plans\task.md:12: path must be relative, not absolute`,
			expected: lintError{
				File:    `C:\repo\plans\task.md`,
				Line:    12,
				Message: "path must be relative, not absolute",
				Rule:    "relative_links",
			},
		},
		{
			name:  "Windows path with drive letter without line number",
			input: `D:\code\powerword\plans\task.md: missing heading "## Verification Plan"`,
			expected: lintError{
				File:    `D:\code\powerword\plans\task.md`,
				Line:    0,
				Message: `missing heading "## Verification Plan"`,
				Rule:    "plan_structure",
			},
		},
		{
			name:  "fallback structureless error",
			input: "some unexpected error string without structure",
			expected: lintError{
				File:    "",
				Line:    0,
				Message: "some unexpected error string without structure",
				Rule:    "unknown",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseValidationError(tc.input)
			if got.File != tc.expected.File {
				t.Errorf("expected File=%q, got %q", tc.expected.File, got.File)
			}
			if got.Line != tc.expected.Line {
				t.Errorf("expected Line=%d, got %d", tc.expected.Line, got.Line)
			}
			if got.Message != tc.expected.Message {
				t.Errorf("expected Message=%q, got %q", tc.expected.Message, got.Message)
			}
			if got.Rule != tc.expected.Rule {
				t.Errorf("expected Rule=%q, got %q", tc.expected.Rule, got.Rule)
			}
		})
	}
}

func TestHandleLintPlans_EnvVarFallback(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	dummyFile := filepath.Join(tmpDir, "some_file.go")
	if err := os.WriteFile(dummyFile, []byte("package main"), 0600); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	planContent := `# plan: Task 1.1: Test Plan
**Status:** Open

## User Review Required
None.

## Proposed Changes
#### [MODIFY] [some_file.go](file://../some_file.go)
- Edit it.

## Verification Plan
### Automated Tests
- Run tests.
`
	if err := os.WriteFile(filepath.Join(plansDir, "task_1_1.md"), []byte(planContent), 0600); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	t.Setenv("POWERWORD_WORKSPACE_ROOT", tmpDir)

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "lint_plans",
			Arguments: json.RawMessage(`{}`),
		},
	}

	res, err := handleLintPlans(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error calling tool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned error flag: %v", res.Content[0])
	}

	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content output")
	}

	var result lintPlansResult
	if err := json.Unmarshal([]byte(tc.Text), &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected valid=true, got false")
	}
}
