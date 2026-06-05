package linter

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	headerRegex = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	listRegex   = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)$`)
	uListRegex  = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	planTitleRx = regexp.MustCompile(`^# Task \d+(\.\d+)*:.*$`)
)

// LintMarkdown checks a markdown content for basic formatting rules.
// Rules enforced:
// - MD024: No duplicate headers in the same file.
// - MD029: Ordered list prefix consistency.
func LintMarkdown(filename string, content string) []string {
	var errors []string
	scanner := bufio.NewScanner(strings.NewReader(content))

	headers := make(map[string]int) // text -> line number
	type listState struct {
		indent    string
		prevNum   int
		listStyle string
	}
	var listStack []listState

	inCodeBlock := false
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for code block toggle
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			// Reset list context on code block
			listStack = nil
			continue
		}

		if inCodeBlock {
			continue
		}

		// Check headers
		if headerRegex.MatchString(line) {
			listStack = nil // headers end list blocks
			matches := headerRegex.FindStringSubmatch(line)
			headerText := strings.TrimSpace(matches[2])
			if prevLine, exists := headers[headerText]; exists {
				errors = append(errors, fmt.Sprintf("%s:%d: MD024 duplicate header %q (previously seen on line %d)", filename, lineNum, headerText, prevLine))
			} else {
				headers[headerText] = lineNum
			}
			continue
		}

		// Check ordered lists
		if listRegex.MatchString(line) {
			matches := listRegex.FindStringSubmatch(line)
			indent := matches[1]
			numStr := matches[2]
			num, _ := strconv.Atoi(numStr)

			// Find if indent already exists in the stack
			matchingIdx := -1
			for i := len(listStack) - 1; i >= 0; i-- {
				if listStack[i].indent == indent {
					matchingIdx = i
					break
				}
			}

			if matchingIdx == -1 {
				// Start a new list block at this indentation
				if num != 1 {
					errors = append(errors, fmt.Sprintf("%s:%d: MD029 ordered list must start with 1, got %d", filename, lineNum, num))
				}
				listStack = append(listStack, listState{
					indent:    indent,
					prevNum:   1,
					listStyle: "",
				})
			} else {
				// Pop deeper nested lists
				listStack = listStack[:matchingIdx+1]
				state := &listStack[matchingIdx]

				switch state.listStyle {
				case "":
					switch num {
					case 1:
						state.listStyle = "one"
					case state.prevNum + 1:
						state.listStyle = "sequential"
						state.prevNum = num
					default:
						errors = append(errors, fmt.Sprintf("%s:%d: MD029 invalid ordered list prefix %d; expected 1 or sequential %d", filename, lineNum, num, state.prevNum+1))
					}
				case "one":
					if num != 1 {
						errors = append(errors, fmt.Sprintf("%s:%d: MD029 expected ordered list prefix '1.', got '%d.' (style is 'one')", filename, lineNum, num))
					}
				case "sequential":
					expected := state.prevNum + 1
					if num != expected {
						errors = append(errors, fmt.Sprintf("%s:%d: MD029 expected sequential ordered list prefix '%d.', got '%d.'", filename, lineNum, expected, num))
					}
					state.prevNum = num
				}
			}
			continue
		}

		// Reset list block under specific conditions:
		switch {
		case trimmed == "":
			listStack = nil
		case uListRegex.MatchString(line):
			listStack = nil
		case len(listStack) > 0:
			// If we are in a list, check if the line indentation is smaller than any active indentations
			lineIndent := ""
			for _, char := range line {
				if char == ' ' || char == '\t' {
					lineIndent += string(char)
				} else {
					break
				}
			}
			// Pop from listStack if the current line has less indentation than a stack level
			for i := len(listStack) - 1; i >= 0; i-- {
				if len(lineIndent) < len(listStack[i].indent) {
					listStack = listStack[:i]
				}
			}
		}
	}

	return errors
}

// LintPlan checks if a plan file contains the required headings:
// - # Task <number>: <title>
// - ## User Review Required
// - ## Proposed Changes
// - ## Verification Plan
func LintPlan(filename string, content string) []string {
	var errors []string
	scanner := bufio.NewScanner(strings.NewReader(content))

	hasTitle := false
	hasReview := false
	hasChanges := false
	hasVerification := false
	inCodeBlock := false

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}

		if inCodeBlock {
			continue
		}

		switch {
		case strings.HasPrefix(line, "# "):
			if planTitleRx.MatchString(line) {
				hasTitle = true
			}
		case line == "## User Review Required":
			hasReview = true
		case line == "## Proposed Changes":
			hasChanges = true
		case line == "## Verification Plan":
			hasVerification = true
		}
	}

	if !hasTitle {
		errors = append(errors, fmt.Sprintf("%s: missing top-level plan header matching '# Task [Task Number]: [Task Title]'", filename))
	}
	if !hasReview {
		errors = append(errors, fmt.Sprintf("%s: missing heading '## User Review Required'", filename))
	}
	if !hasChanges {
		errors = append(errors, fmt.Sprintf("%s: missing heading '## Proposed Changes'", filename))
	}
	if !hasVerification {
		errors = append(errors, fmt.Sprintf("%s: missing heading '## Verification Plan'", filename))
	}

	return errors
}
