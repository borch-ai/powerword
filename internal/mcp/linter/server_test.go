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

func TestParseValidationError_Fallback(t *testing.T) {
	err := parseValidationError("some unexpected error string without structure")
	if err.Rule != "unknown" {
		t.Errorf("expected unknown rule, got: %s", err.Rule)
	}
}
