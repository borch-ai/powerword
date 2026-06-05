package linter

import (
	"strings"
	"testing"
)

func TestLintMarkdown_Success(t *testing.T) {
	content := `# Heading 1

## Heading 2

Some text here.

1. First item
2. Second item
3. Third item

And another list:

1. First
1. Second
1. Third

` + "```" + `
# Comment in code block
1. Not a list item
## Not a header
` + "```"

	errors := LintMarkdown("test.md", content)
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %d: %v", len(errors), errors)
	}
}

func TestLintMarkdown_DuplicateHeaders(t *testing.T) {
	content := `# Header

## SubHeader

## SubHeader

# Header
`
	errors := LintMarkdown("test.md", content)
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}
	if !strings.Contains(errors[0], `duplicate header "SubHeader"`) {
		t.Errorf("unexpected error: %s", errors[0])
	}
	if !strings.Contains(errors[1], `duplicate header "Header"`) {
		t.Errorf("unexpected error: %s", errors[1])
	}
}

func TestLintMarkdown_OrderedListPrefix(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectError bool
		errorMsg    string
	}{
		{
			name: "starts with 2",
			content: `
2. item 1
3. item 2
`,
			expectError: true,
			errorMsg:    "ordered list must start with 1",
		},
		{
			name: "one style error",
			content: `
1. item 1
1. item 2
2. item 3
`,
			expectError: true,
			errorMsg:    "expected ordered list prefix '1.', got '2.'",
		},
		{
			name: "sequential style error",
			content: `
1. item 1
2. item 2
4. item 3
`,
			expectError: true,
			errorMsg:    "expected sequential ordered list prefix '3.', got '4.'",
		},
		{
			name: "indentation reset valid",
			content: `
1. item 1
   1. subitem 1
   2. subitem 2
2. item 2
`,
			expectError: false,
		},
		{
			name: "nested list error",
			content: `
1. item 1
   2. subitem 1
`,
			expectError: true,
			errorMsg:    "ordered list must start with 1, got 2",
		},
		{
			name: "blank line does not reset list (loose list valid)",
			content: `
1. item 1

2. item 2
`,
			expectError: false,
		},
		{
			name: "unordered list resets list",
			content: `
1. item 1
- unordered item
2. item 2
`,
			expectError: true,
			errorMsg:    "ordered list must start with 1, got 2",
		},
		{
			name: "invalid sequential step on initialization",
			content: `
1. item 1
3. item 2
`,
			expectError: true,
			errorMsg:    "invalid ordered list prefix 3; expected 1 or sequential 2",
		},
		{
			name: "smaller indentation resets nested list",
			content: `
1. item 1
   1. subitem 1
Paragraph text at outer level
   2. subitem 2
`,
			expectError: true,
			errorMsg:    "ordered list must start with 1, got 2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errors := LintMarkdown("test.md", tc.content)
			if tc.expectError {
				if len(errors) == 0 {
					t.Fatalf("expected error containing %q, got none", tc.errorMsg)
				}
				found := false
				for _, err := range errors {
					if strings.Contains(err, tc.errorMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got errors: %v", tc.errorMsg, errors)
				}
			} else if len(errors) > 0 {
				t.Errorf("expected no errors, got %v", errors)
			}
		})
	}
}

func TestLintPlan_Success(t *testing.T) {
	content := `# Task 1.5: Markdown Linting

## User Review Required
Some review text.

## Proposed Changes
Some changes.

## Verification Plan
Verification plan details.
`
	errors := LintPlan("plan.md", content)
	if len(errors) != 0 {
		t.Errorf("expected no errors, got: %v", errors)
	}
}

func TestLintPlan_Failures(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name: "missing title",
			content: `
## User Review Required
## Proposed Changes
## Verification Plan
`,
			expected: []string{"missing top-level plan header"},
		},
		{
			name: "invalid title format",
			content: `
# Task ABC: Invalid Title
## User Review Required
## Proposed Changes
## Verification Plan
`,
			expected: []string{"missing top-level plan header"},
		},
		{
			name: "missing other headings",
			content: `
# Task 1.2: Title
## User Review Required
`,
			expected: []string{
				"missing heading '## Proposed Changes'",
				"missing heading '## Verification Plan'",
			},
		},
		{
			name: "headings in code blocks",
			content: `
# Task 1.1: Title
## User Review Required
` + "```" + `
## Proposed Changes
## Verification Plan
` + "```" + `
`,
			expected: []string{
				"missing heading '## Proposed Changes'",
				"missing heading '## Verification Plan'",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errors := LintPlan("plan.md", tc.content)
			if len(errors) != len(tc.expected) {
				t.Fatalf("expected %d errors, got %d: %v", len(tc.expected), len(errors), errors)
			}
			for i, expectedText := range tc.expected {
				if !strings.Contains(errors[i], expectedText) {
					t.Errorf("expected error %d to contain %q, got %q", i, expectedText, errors[i])
				}
			}
		})
	}
}
