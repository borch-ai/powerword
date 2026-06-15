package typst

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Page represents a parsed page from the manuscript.
type Page struct {
	Number int
	Text   string
	Prompt string
}

var pageHeaderRegex = regexp.MustCompile(`^(?i)#\s*Page\s*(\d+)`)

// ParseManuscript parses a Markdown manuscript containing # Page N, ## Text, and ## Prompt sections.
func ParseManuscript(filePath string) ([]Page, error) {
	//nolint:gosec // filePath is dynamic and verified by checkSandbox
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open manuscript file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var pages []Page
	var currentPage *Page
	state := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if shouldSkip(line) {
			continue
		}

		if matches := pageHeaderRegex.FindStringSubmatch(line); len(matches) > 0 {
			pageNum, err := strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("invalid page number: %w", err)
			}
			if currentPage != nil {
				pages = append(pages, *currentPage)
			}
			currentPage = &Page{Number: pageNum}
			state = 0
			continue
		}

		if currentPage == nil {
			continue
		}

		state = updateState(line, state)
		if strings.HasPrefix(line, "##") {
			continue
		}

		appendContent(currentPage, line, state)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading manuscript file: %w", err)
	}

	if currentPage != nil {
		pages = append(pages, *currentPage)
	}

	cleanPages(pages)
	return pages, nil
}

func shouldSkip(line string) bool {
	return strings.HasPrefix(line, "<!--")
}

func updateState(line string, currentState int) int {
	if strings.EqualFold(line, "## Text") {
		return 1
	}
	if strings.EqualFold(line, "## Prompt") {
		return 2
	}
	return currentState
}

func appendContent(page *Page, line string, state int) {
	if state == 1 && page.Text == "" {
		page.Text = line
		return
	}
	if state == 1 {
		page.Text += "\n" + line
		return
	}
	if state == 2 && page.Prompt == "" {
		page.Prompt = line
		return
	}
	if state == 2 {
		page.Prompt += "\n" + line
	}
}

func cleanPages(pages []Page) {
	for i := range pages {
		pages[i].Text = strings.TrimSpace(pages[i].Text)
		pages[i].Text = strings.ReplaceAll(pages[i].Text, "\n", " \\\n")
		pages[i].Prompt = strings.TrimSpace(pages[i].Prompt)
	}
}
