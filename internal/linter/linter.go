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
)

type listState struct {
	indent    string
	prevNum   int
	listStyle string
}

type markdownLinter struct {
	filename    string
	lineNum     int
	errors      []string
	headers     map[string]int
	listStack   []listState
	inCodeBlock bool
}

func newMarkdownLinter(filename string) *markdownLinter {
	return &markdownLinter{
		filename: filename,
		headers:  make(map[string]int),
	}
}

func (l *markdownLinter) checkHeader(line string) bool {
	if !headerRegex.MatchString(line) {
		return false
	}
	l.listStack = nil // headers end list blocks
	matches := headerRegex.FindStringSubmatch(line)
	headerText := strings.TrimSpace(matches[2])
	if prevLine, exists := l.headers[headerText]; exists {
		l.errors = append(l.errors, fmt.Sprintf("%s:%d: MD024 duplicate header %q (previously seen on line %d)", l.filename, l.lineNum, headerText, prevLine))
	} else {
		l.headers[headerText] = l.lineNum
	}
	return true
}

func (l *markdownLinter) checkOrderedList(line string) bool {
	if !listRegex.MatchString(line) {
		return false
	}
	matches := listRegex.FindStringSubmatch(line)
	indent := matches[1]
	numStr := matches[2]
	num, _ := strconv.Atoi(numStr)

	matchingIdx := -1
	for i := len(l.listStack) - 1; i >= 0; i-- {
		if l.listStack[i].indent == indent {
			matchingIdx = i
			break
		}
	}

	if matchingIdx == -1 {
		l.startNewList(num, indent)
	} else {
		l.continueList(matchingIdx, num)
	}
	return true
}

func (l *markdownLinter) startNewList(num int, indent string) {
	if num != 1 {
		l.errors = append(l.errors, fmt.Sprintf("%s:%d: MD029 ordered list must start with 1, got %d", l.filename, l.lineNum, num))
	}
	l.listStack = append(l.listStack, listState{
		indent:    indent,
		prevNum:   1,
		listStyle: "",
	})
}

func (l *markdownLinter) continueList(matchingIdx int, num int) {
	l.listStack = l.listStack[:matchingIdx+1]
	state := &l.listStack[matchingIdx]

	switch state.listStyle {
	case "":
		switch num {
		case 1:
			state.listStyle = "one"
		case state.prevNum + 1:
			state.listStyle = "sequential"
			state.prevNum = num
		default:
			l.errors = append(l.errors, fmt.Sprintf("%s:%d: MD029 invalid ordered list prefix %d; expected 1 or sequential %d", l.filename, l.lineNum, num, state.prevNum+1))
		}
	case "one":
		if num != 1 {
			l.errors = append(l.errors, fmt.Sprintf("%s:%d: MD029 expected ordered list prefix '1.', got '%d.' (style is 'one')", l.filename, l.lineNum, num))
		}
	case "sequential":
		expected := state.prevNum + 1
		if num != expected {
			l.errors = append(l.errors, fmt.Sprintf("%s:%d: MD029 expected sequential ordered list prefix '%d.', got '%d.'", l.filename, l.lineNum, expected, num))
		}
		state.prevNum = num
	}
}

func (l *markdownLinter) resetListStack(trimmed, line string) {
	if trimmed == "" || len(l.listStack) == 0 {
		return
	}
	lineIndent := ""
	for _, char := range line {
		if char == ' ' || char == '\t' {
			lineIndent += string(char)
		} else {
			break
		}
	}
	for i := len(l.listStack) - 1; i >= 0; i-- {
		if len(lineIndent) <= len(l.listStack[i].indent) {
			l.listStack = l.listStack[:i]
		}
	}
}

// LintMarkdown checks a markdown content for basic formatting rules.
func LintMarkdown(filename string, content string) []string {
	l := newMarkdownLinter(filename)
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		l.lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			l.inCodeBlock = !l.inCodeBlock
			continue
		}

		if l.inCodeBlock {
			continue
		}

		if l.checkHeader(line) {
			continue
		}

		if l.checkOrderedList(line) {
			continue
		}

		l.resetListStack(trimmed, line)
	}

	if err := scanner.Err(); err != nil {
		l.errors = append(l.errors, fmt.Sprintf("%s: scanning error: %v", filename, err))
	}
	return l.errors
}
