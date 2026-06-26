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

type orderedListTestCase struct {
	name        string
	content     string
	expectError bool
	errorMsg    string
}

func runOrderedListTest(t *testing.T, tc orderedListTestCase) {
	errors := LintMarkdown("test.md", tc.content)
	if !tc.expectError {
		if len(errors) > 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
		return
	}

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
}

func TestLintMarkdown_OrderedListPrefix(t *testing.T) {
	tests := []orderedListTestCase{
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runOrderedListTest(t, tc)
		})
	}
}

// TestLintMarkdown_OrderedListReset covers the resetListStack branch paths.
func TestLintMarkdown_OrderedListReset(t *testing.T) {
	tests := []orderedListTestCase{
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
		{
			name: "indented non-list text does not pop outer list",
			content: `
1. item 1
  Some indented text (more indent than outer list, less than would be a sublist)
2. item 2
`,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runOrderedListTest(t, tc)
		})
	}
}
