package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
)

func TestResolveTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	// Case 1: No local file and no config -> fallback to embedded
	cfg := &config.Config{}
	resolved, err := resolveTemplate(tmpDir, cfg)
	if err != nil {
		t.Fatalf("expected resolveTemplate to succeed, got %v", err)
	}
	if !strings.Contains(resolved, "# plan: Task") {
		t.Errorf("expected resolved template to contain embedded content, got: %s", resolved)
	}

	// Case 2: Config configured template
	globalTmplFile := filepath.Join(tmpDir, "global_template.md")
	globalContent := "# custom global template"
	if errWrite := os.WriteFile(globalTmplFile, []byte(globalContent), 0600); errWrite != nil {
		t.Fatalf("failed to write global template: %v", errWrite)
	}
	cfg.PlanTemplate = globalTmplFile
	resolved, err = resolveTemplate(tmpDir, cfg)
	if err != nil {
		t.Fatalf("expected resolveTemplate to succeed, got %v", err)
	}
	if resolved != globalContent {
		t.Errorf("expected global content %q, got %q", globalContent, resolved)
	}

	// Case 3: Local template override
	plansDir := filepath.Join(tmpDir, "plans")
	if errMkdir := os.Mkdir(plansDir, 0750); errMkdir != nil {
		t.Fatalf("failed to create plans dir: %v", errMkdir)
	}
	localTmplFile := filepath.Join(plansDir, "TEMPLATE.md")
	localContent := "# custom local template"
	if errWrite := os.WriteFile(localTmplFile, []byte(localContent), 0600); errWrite != nil {
		t.Fatalf("failed to write local template: %v", errWrite)
	}
	resolved, err = resolveTemplate(tmpDir, cfg)
	if err != nil {
		t.Fatalf("expected resolveTemplate to succeed, got %v", err)
	}
	if resolved != localContent {
		t.Errorf("expected local content %q, got %q", localContent, resolved)
	}
}

func TestValidatePlans_Success(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	// Create a dummy modified file that actually exists
	dummyFile := filepath.Join(tmpDir, "some_file.go")
	if err := os.WriteFile(dummyFile, []byte("package main"), 0600); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	// Write a valid plan
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

	cfg := &config.Config{}
	err := ValidatePlans(tmpDir, cfg)
	if err != nil {
		t.Errorf("expected no validation errors, got: %v", err)
	}
}

//nolint:funlen
func TestValidatePlans_Failures(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	tests := []struct {
		name         string
		planFilename string
		content      string
		expectError  string
	}{
		{
			name:         "missing title",
			planFilename: "task_missing_title.md",
			content: `## User Review Required
## Proposed Changes
## Verification Plan`,
			expectError: "missing top-level plan header",
		},
		{
			name:         "missing proposed changes",
			planFilename: "task_missing_proposed.md",
			content: `# plan: Task 1.1: Missing Proposed
## User Review Required
## Verification Plan`,
			expectError: "missing heading \"## Proposed Changes\"",
		},
		{
			name:         "missing verification plan",
			planFilename: "task_missing_verif.md",
			content: `# plan: Task 1.1: Missing Verification
## User Review Required
## Proposed Changes`,
			expectError: "missing heading \"## Verification Plan\"",
		},
		{
			name:         "completed plan missing metadata",
			planFilename: "task_completed_missing_meta.md",
			content: `# plan: Task 1.1: Completed Missing Meta
**Status:** Completed
## User Review Required
## Proposed Changes
## Verification Plan`,
			expectError: "Go Version is missing or a placeholder",
		},
		{
			name:         "completed plan missing Date Completed",
			planFilename: "task_completed_missing_date.md",
			content: `# plan: Task 1.1: Completed Missing Date
**Status:** Completed
**Go Version:** 1.26
**Unit Test Coverage:** 92%
## User Review Required
## Proposed Changes
## Verification Plan`,
			expectError: "Date Completed is missing or a placeholder",
		},
		{
			name:         "completed plan missing Unit Test Coverage",
			planFilename: "task_completed_missing_coverage.md",
			content: `# plan: Task 1.1: Completed Missing Coverage
**Status:** Completed
**Go Version:** 1.26
**Date Completed:** 2026-06-11
## User Review Required
## Proposed Changes
## Verification Plan`,
			expectError: "Unit Test Coverage is missing or a placeholder",
		},
		{
			name:         "non-matching title format",
			planFilename: "task_non_matching_title.md",
			content: `# not a matching title
**Status:** Open
## User Review Required
## Proposed Changes
## Verification Plan`,
			expectError: "missing top-level plan header",
		},
		{
			name:         "completed plan placeholder metadata",
			planFilename: "task_completed_placeholder.md",
			content: `# plan: Task 1.1: Completed Placeholder
**Status:** Completed
**Go Version:** [Go Version]
**Date Completed:** TBD
**Unit Test Coverage:** [TBD]
## User Review Required
## Proposed Changes
## Verification Plan`,
			expectError: "Go Version is missing or a placeholder",
		},
		{
			name:         "link label mismatch",
			planFilename: "task_label_mismatch.md",
			content: `# plan: Task 1.1: Label Mismatch
**Status:** Open
## User Review Required
## Proposed Changes
#### [MODIFY] [wrong_label.go](file://../correct_name.go)
## Verification Plan`,
			expectError: "link label \"wrong_label.go\" does not match actual file basename \"correct_name.go\"",
		},
		{
			name:         "modified file does not exist",
			planFilename: "task_file_not_exist.md",
			content: `# plan: Task 1.1: File Not Exist
**Status:** Open
## User Review Required
## Proposed Changes
#### [MODIFY] [non_existent.go](file://../non_existent.go)
## Verification Plan`,
			expectError: "modified file",
		},
		{
			name:         "completed new file does not exist",
			planFilename: "task_completed_new_not_exist.md",
			content: `# plan: Task 1.1: New File Completed Not Exist
**Status:** Completed
**Go Version:** 1.26
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 92%
## User Review Required
## Proposed Changes
#### [NEW] [non_existent_new.go](file://../non_existent_new.go)
## Verification Plan`,
			expectError: "completed new file",
		},
		{
			name:         "path outside workspace",
			planFilename: "task_outside_workspace.md",
			content: `# plan: Task 1.1: Outside Workspace
**Status:** Open
## User Review Required
## Proposed Changes
#### [MODIFY] [hosts](file://../../../../../../../../../../../../../../../../etc/hosts)
## Verification Plan`,
			expectError: "outside workspace root",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(plansDir, tc.planFilename)
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatalf("failed to write test plan: %v", err)
			}
			defer func() {
				_ = os.Remove(path)
			}()

			cfg := &config.Config{}
			err := ValidatePlans(tmpDir, cfg)
			if err == nil {
				t.Errorf("expected validation error, got nil")
			} else if !strings.Contains(err.Error(), tc.expectError) {
				t.Errorf("expected error containing %q, got: %v", tc.expectError, err)
			}
		})
	}
}

func TestValidatePlans_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	// Create a file in root
	dummyFile := filepath.Join(tmpDir, "some_file.go")
	if err := os.WriteFile(dummyFile, []byte("package main"), 0600); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	// Write a valid plan using relative path relative to plans/
	planContent := `# plan: Task 1.1: Test Plan
**Status:** Open

## User Review Required
None.

## Proposed Changes
#### [MODIFY] [some_file.go](../some_file.go)
- Edit it.

## Verification Plan
### Automated Tests
- Run tests.
`
	if err := os.WriteFile(filepath.Join(plansDir, "task_1_1.md"), []byte(planContent), 0600); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	// Create a subfolder inside plans/
	subDir := filepath.Join(plansDir, "phase_1")
	if err := os.Mkdir(subDir, 0750); err != nil {
		t.Fatalf("failed to create plans subfolder: %v", err)
	}

	// Write a valid plan inside the subfolder using relative path relative to the subfolder (i.e. ../../some_file.go)
	nestedPlanContent := `# plan: Task 1.2: Test Plan Nested
**Status:** Open

## User Review Required
None.

## Proposed Changes
#### [MODIFY] [some_file.go](../../some_file.go)
- Edit it.

## Verification Plan
### Automated Tests
- Run tests.
`
	if err := os.WriteFile(filepath.Join(subDir, "task_1_2.md"), []byte(nestedPlanContent), 0600); err != nil {
		t.Fatalf("failed to write nested plan file: %v", err)
	}

	cfg := &config.Config{}
	err := ValidatePlans(tmpDir, cfg)
	if err != nil {
		t.Errorf("expected relative path validation to succeed, got: %v", err)
	}
}

func TestValidatePlans_CustomTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	// Create global template file
	tmplFile := filepath.Join(tmpDir, "my_tmpl.md")
	tmplContent := `# Custom Plan Template Title
## User Review Required
## Custom Section
`
	if err := os.WriteFile(tmplFile, []byte(tmplContent), 0600); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	cfg := &config.Config{
		PlanTemplate: tmplFile,
	}

	// Case 1: Plan conforms to custom template
	conformingPlan := `# Custom Plan Template Title
**Status:** Open
## User Review Required
## Custom Section
`
	planPath := filepath.Join(plansDir, "task_1_1.md")
	if err := os.WriteFile(planPath, []byte(conformingPlan), 0600); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}
	if err := ValidatePlans(tmpDir, cfg); err != nil {
		t.Errorf("expected custom template validation to succeed, got: %v", err)
	}

	// Case 2: Plan does not conform (missing custom section)
	nonConformingPlan := `# Custom Plan Template Title
**Status:** Open
## User Review Required
`
	if err := os.WriteFile(planPath, []byte(nonConformingPlan), 0600); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}
	if err := ValidatePlans(tmpDir, cfg); err == nil {
		t.Error("expected custom template validation to fail due to missing Custom Section, got nil")
	}
}

func TestValidatePlans_AdditionalFailures(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	tests := []struct {
		name         string
		planFilename string
		content      string
		expectError  string
	}{
		{
			name:         "modify is a directory",
			planFilename: "task_modify_dir.md",
			content: `# plan: Task 1.1: Modify Dir
**Status:** Open
## User Review Required
## Proposed Changes
#### [MODIFY] [plans](file://../plans)
## Verification Plan`,
			expectError: "is a directory, not a file",
		},
		{
			name:         "completed new is a directory",
			planFilename: "task_new_dir.md",
			content: `# plan: Task 1.1: New Dir Completed
**Status:** Completed
**Go Version:** 1.26
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 92%
## User Review Required
## Proposed Changes
#### [NEW] [plans](file://../plans)
## Verification Plan`,
			expectError: "is a directory, not a file",
		},
		{
			name:         "empty metadata value",
			planFilename: "task_empty_meta.md",
			content: `# plan: Task 1.1: Empty Meta
**Status:** Completed
**Go Version:**
**Date Completed:** 2026-06-11
**Unit Test Coverage:** 92%
## User Review Required
## Proposed Changes
## Verification Plan`,
			expectError: "Go Version is missing or a placeholder",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(plansDir, tc.planFilename)
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatalf("failed to write test plan: %v", err)
			}
			defer func() {
				_ = os.Remove(path)
			}()

			cfg := &config.Config{}
			err := ValidatePlans(tmpDir, cfg)
			if err == nil {
				t.Errorf("expected validation error, got nil")
			} else if !strings.Contains(err.Error(), tc.expectError) {
				t.Errorf("expected error containing %q, got: %v", tc.expectError, err)
			}
		})
	}
}

func TestValidatePlans_CopilotComments(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{}

	// 1. Unreadable template file (e.g. is a directory) returns error
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}
	localTmplDir := filepath.Join(plansDir, "TEMPLATE.md")
	if err := os.Mkdir(localTmplDir, 0750); err != nil {
		t.Fatalf("failed to create TEMPLATE.md directory: %v", err)
	}
	_, errResolve := resolveTemplate(tmpDir, cfg)
	if errResolve == nil {
		t.Error("expected error resolving directory as template, got nil")
	}
	_ = os.Remove(localTmplDir)

	// 2. Compatibility normalization for /code/powerword/ and /powerword/ paths
	dummyFile := filepath.Join(tmpDir, "some_file.go")
	if errWrite := os.WriteFile(dummyFile, []byte("package main"), 0600); errWrite != nil {
		t.Fatalf("failed to write dummy file: %v", errWrite)
	}

	planContent := `# plan: Task 1.1: Compatibility Normalized Plan
**Status:** Open

## User Review Required
None

## Proposed Changes
#### [MODIFY] [some_file.go](file:///Users/human/code/powerword/some_file.go)
- Edit it.

## Verification Plan
### Automated Tests
- Run tests.
`
	planPath := filepath.Join(plansDir, "task_1_1.md")
	if errWritePlan := os.WriteFile(planPath, []byte(planContent), 0600); errWritePlan != nil {
		t.Fatalf("failed to write plan: %v", errWritePlan)
	}

	errValidate := ValidatePlans(tmpDir, cfg)
	if errValidate == nil {
		t.Error("expected validation to fail for absolute path link")
	} else if !strings.Contains(errValidate.Error(), "must be relative, not absolute") {
		t.Errorf("expected error to mention absolute path, got: %v", errValidate)
	}

	// Now fix it
	fixedCount, errFix := FixAbsolutePathsInPlans(tmpDir, cfg)
	if errFix != nil {
		t.Fatalf("expected FixAbsolutePathsInPlans to succeed, got %v", errFix)
	}
	if fixedCount != 1 {
		t.Errorf("expected 1 file to be modified, got %d", fixedCount)
	}

	// Verify that it now passes validation
	errValidatePost := ValidatePlans(tmpDir, cfg)
	if errValidatePost != nil {
		t.Errorf("expected validation to succeed after auto-fix, got error: %v", errValidatePost)
	}
	_ = os.Remove(planPath)

	// 3. Outside workspace path returns early
	outsidePlanContent := `# plan: Task 1.1: Outside Workspace early return
**Status:** Open

## User Review Required
None

## Proposed Changes
#### [MODIFY] [hosts](file://../../../../../../../../etc/hosts)
- Edit it.

## Verification Plan
`
	if errWriteOutside := os.WriteFile(planPath, []byte(outsidePlanContent), 0600); errWriteOutside != nil {
		t.Fatalf("failed to write outside plan: %v", errWriteOutside)
	}
	errValidateOutside := ValidatePlans(tmpDir, cfg)
	if errValidateOutside == nil {
		t.Error("expected validation to fail for outside path")
	} else {
		errMsg := errValidateOutside.Error()
		if !strings.Contains(errMsg, "outside workspace root") {
			t.Errorf("expected outside workspace root error, got: %s", errMsg)
		}
		// Ensure it didn't do basename checks on /etc/hosts or os.Stat checks
		if strings.Contains(errMsg, "link label") || strings.Contains(errMsg, "does not exist on disk") {
			t.Errorf("expected validation to return early on outside path, but got other errors: %s", errMsg)
		}
	}
	_ = os.Remove(planPath)
}

func TestValidatePlans_CoverageBoosters(t *testing.T) {
	// Test compileTitleRegex with invalid regex pattern
	r := compileTitleRegex("[invalid-regex")
	if r == nil {
		t.Error("expected compileTitleRegex to fall back to default regex, got nil")
	}
	if r.String() != `(?i)^#\s+(plan|feat):\s*Task\s+.*$` {
		t.Errorf("expected fallback regex, got: %s", r.String())
	}

	// Test compileTitleRegex with empty pattern
	rEmpty := compileTitleRegex("")
	if rEmpty == nil {
		t.Error("expected compileTitleRegex to fall back to default regex on empty pattern, got nil")
	}

	// Test scanPlanFiles when plans directory does not exist
	files, err := scanPlanFiles("/non-existent-directory-xyz-123")
	if err != nil {
		t.Errorf("expected scanPlanFiles to not return error for non-existent directory, got: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got: %d", len(files))
	}
}

func TestFixAbsolutePathsInPlans(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	if err := os.Mkdir(plansDir, 0750); err != nil {
		t.Fatalf("failed to create plans dir: %v", err)
	}

	// 1. Create a dummy file that actually exists
	dummyFile := filepath.Join(tmpDir, "my_file.go")
	if err := os.WriteFile(dummyFile, []byte("package main"), 0600); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	// 2. Create a go.mod file to test Go Version parsing
	goModContent := `module github.com/borch-ai/powerword
go 1.25.3
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0600); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// 3. Write a plan file with absolute paths, mismatched labels, missing file://, and completed status placeholders
	planContent := `# plan: Task 1.1: Fix Me Plan
**Status:** Completed
**Go Version:** [Go Version]
**Date Completed:** TBD
**Unit Test Coverage:** 92%

## Proposed Changes
#### [MODIFY] [wrong_label.go](file:///` + strings.ReplaceAll(dummyFile, "\\", "/") + `)
- Edit it.
#### [NEW] [my_file.go](../my_file.go)
- Edit it.

## Verification Plan
`
	planPath := filepath.Join(plansDir, "task_1_1.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0600); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	cfg := &config.Config{}
	fixed, err := FixAbsolutePathsInPlans(tmpDir, cfg)
	if err != nil {
		t.Fatalf("FixAbsolutePathsInPlans failed: %v", err)
	}
	if fixed != 1 {
		t.Errorf("expected 1 file to be modified, got %d", fixed)
	}

	// Read modified file content
	//nolint:gosec
	fixedContentBytes, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("failed to read plan: %v", err)
	}
	fixedContent := string(fixedContentBytes)

	// Verify Go Version is updated from go.mod
	if !strings.Contains(fixedContent, "**Go Version:** 1.25.3") {
		t.Errorf("expected Go Version to be populated with 1.25.3, got:\n%s", fixedContent)
	}

	// Verify Date Completed is populated with today's date
	todayStr := time.Now().Format("2006-01-02")
	if !strings.Contains(fixedContent, "**Date Completed:** "+todayStr) {
		t.Errorf("expected Date Completed to be populated with %s, got:\n%s", todayStr, fixedContent)
	}

	// Verify links:
	// - wrong_label.go -> my_file.go
	// - file:///absolute_path -> file://../my_file.go
	if !strings.Contains(fixedContent, "#### [MODIFY] [my_file.go](file://../my_file.go)") {
		t.Errorf("expected absolute link with mismatched label to be fixed, got:\n%s", fixedContent)
	}

	// - naked relative link -> prepended file://
	if !strings.Contains(fixedContent, "#### [NEW] [my_file.go](file://../my_file.go)") {
		t.Errorf("expected naked relative link to be normalized with file://, got:\n%s", fixedContent)
	}
}
