package review

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
)

//go:embed default_template.md
var defaultTemplate string

var (
	linkRegex             = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	statusRegex           = regexp.MustCompile(`(?i)^(?:\s*[-*+]?\s*)?(?:\*\*|\*)?Status\b[^:]*:\s*(?:\*\*|\*|)?\s*([a-zA-Z]+)`)
	goVersionRegex        = regexp.MustCompile(`(?i)^(?:\s*[-*+]?\s*)?(?:\*\*|\*)?Go Version\b[^:]*:\s*(?:\*\*|\*|)?\s*(.*)`)
	dateCompletedRegex    = regexp.MustCompile(`(?i)^(?:\s*[-*+]?\s*)?(?:\*\*|\*)?Date Completed\b[^:]*:\s*(?:\*\*|\*|)?\s*(.*)`)
	unitTestCoverageRegex = regexp.MustCompile(`(?i)^(?:\s*[-*+]?\s*)?(?:\*\*|\*)?Unit Test Coverage\b[^:]*:\s*(?:\*\*|\*|)?\s*(.*)`)
)

// PlanValidationError represents one or more plan conformance errors.
type PlanValidationError struct {
	Errors []string
}

func (e *PlanValidationError) Error() string {
	return fmt.Sprintf("plan validation failed:\n- %s", strings.Join(e.Errors, "\n- "))
}

// ValidatePlans scans the plans/ directory in the workspaceRoot and validates all plan files
// against the resolved template.
func ValidatePlans(workspaceRoot string, cfg *config.Config) error {
	templateText, err := resolveTemplate(workspaceRoot, cfg)
	if err != nil {
		return fmt.Errorf("failed to resolve plan template: %w", err)
	}

	templateHeadings, titlePatternStr := parseTemplateHeaders(templateText)
	titleRegex := compileTitleRegex(titlePatternStr)

	plansDir := filepath.Join(workspaceRoot, "plans")
	planFiles, err := scanPlanFiles(plansDir)
	if err != nil {
		return fmt.Errorf("failed to scan for plan files: %w", err)
	}

	var allErrors []string

	for _, planFile := range planFiles {
		if filepath.Base(planFile) == "TEMPLATE.md" {
			continue
		}

		fileErrs := validateSinglePlan(workspaceRoot, planFile, titleRegex, templateHeadings)
		if len(fileErrs) > 0 {
			allErrors = append(allErrors, fileErrs...)
		}
	}

	if len(allErrors) > 0 {
		return &PlanValidationError{Errors: allErrors}
	}

	return nil
}

func compileTitleRegex(pattern string) *regexp.Regexp {
	if pattern == "" {
		return regexp.MustCompile(`(?i)^#\s+(plan|feat):\s*Task\s+.*$`)
	}
	if r, err := regexp.Compile(pattern); err == nil {
		return r
	}
	return regexp.MustCompile(`(?i)^#\s+(plan|feat):\s*Task\s+.*$`)
}

func scanPlanFiles(plansDir string) ([]string, error) {
	if _, err := os.Stat(plansDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var planFiles []string
	err := filepath.WalkDir(plansDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, "task_") && strings.HasSuffix(name, ".md") {
			planFiles = append(planFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return planFiles, nil
}

func resolveTemplate(workspaceRoot string, cfg *config.Config) (string, error) {
	// 1. Workspace-specific
	workspacePath := filepath.Join(workspaceRoot, "plans", "TEMPLATE.md")
	//nolint:gosec
	if _, err := os.Stat(workspacePath); err == nil {
		//nolint:gosec
		content, err := os.ReadFile(workspacePath)
		if err != nil {
			return "", fmt.Errorf("failed to read workspace plan template %s: %w", workspacePath, err)
		}
		return string(content), nil
	}

	// 2. Global Config
	if cfg != nil && cfg.PlanTemplate != "" {
		//nolint:gosec
		if _, err := os.Stat(cfg.PlanTemplate); err == nil {
			//nolint:gosec
			content, err := os.ReadFile(cfg.PlanTemplate)
			if err != nil {
				return "", fmt.Errorf("failed to read global plan template %s: %w", cfg.PlanTemplate, err)
			}
			return string(content), nil
		}
	}

	// 3. Embedded Default
	return defaultTemplate, nil
}

func parseTemplateHeaders(templateText string) ([]string, string) {
	var headings []string
	var titlePattern string

	scanner := bufio.NewScanner(strings.NewReader(templateText))
	inCodeBlock := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(line[2:])
			if strings.Contains(strings.ToLower(title), "plan: task") || strings.Contains(strings.ToLower(title), "feat: task") {
				titlePattern = `(?i)^#\s+(plan|feat):\s*Task\s+.*$`
			} else {
				titlePattern = `^#\s+` + regexp.QuoteMeta(title) + `$`
			}
		} else if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimSpace(line))
		}
	}
	return headings, titlePattern
}

//nolint:gocognit,funlen
func validateSinglePlan(workspaceRoot string, planFile string, titleRegex *regexp.Regexp, templateHeadings []string) []string {
	var fileErrs []string

	//nolint:gosec
	content, err := os.ReadFile(planFile)
	if err != nil {
		return []string{fmt.Sprintf("%s: failed to read file: %v", planFile, err)}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lineNum := 0
	inCodeBlock := false

	hasTitle := false
	seenHeadings := make(map[string]bool)
	status := ""

	foundGoVersion := false
	foundDateCompleted := false
	foundUnitTestCoverage := false

	goVersionVal := ""
	dateCompletedVal := ""
	unitTestCoverageVal := ""

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

		// Title check
		if strings.HasPrefix(line, "# ") {
			if titleRegex.MatchString(line) {
				hasTitle = true
			}
		}

		// Heading tracking (only store level 2 headings)
		if strings.HasPrefix(line, "## ") {
			seenHeadings[strings.TrimSpace(line)] = true
		}

		// Status parsing
		if statusMatch := statusRegex.FindStringSubmatch(line); len(statusMatch) > 1 {
			status = statusMatch[1]
		}

		// Metadata parsing
		if gvMatch := goVersionRegex.FindStringSubmatch(line); len(gvMatch) > 1 {
			foundGoVersion = true
			goVersionVal = gvMatch[1]
		}
		if dcMatch := dateCompletedRegex.FindStringSubmatch(line); len(dcMatch) > 1 {
			foundDateCompleted = true
			dateCompletedVal = dcMatch[1]
		}
		if utcMatch := unitTestCoverageRegex.FindStringSubmatch(line); len(utcMatch) > 1 {
			foundUnitTestCoverage = true
			unitTestCoverageVal = utcMatch[1]
		}

		// Links extraction
		links := linkRegex.FindAllStringSubmatch(line, -1)
		for _, match := range links {
			label := match[1]
			linkPath := match[2]

			isActionHeader := strings.Contains(line, "[NEW]") || strings.Contains(line, "[MODIFY]") || strings.Contains(line, "[DELETE]")
			isFileScheme := strings.HasPrefix(linkPath, "file://")

			if isActionHeader || isFileScheme {
				linkErrs := validateLink(workspaceRoot, planFile, lineNum, line, label, linkPath, &status)
				if len(linkErrs) > 0 {
					fileErrs = append(fileErrs, linkErrs...)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fileErrs = append(fileErrs, fmt.Sprintf("%s: scanning error: %v", planFile, err))
		return fileErrs
	}

	if !hasTitle {
		fileErrs = append(fileErrs, fmt.Sprintf("%s: missing top-level plan header matching title pattern", planFile))
	}

	for _, reqHeading := range templateHeadings {
		if !seenHeadings[reqHeading] {
			fileErrs = append(fileErrs, fmt.Sprintf("%s: missing heading %q", planFile, reqHeading))
		}
	}

	if len(templateHeadings) == 0 {
		coreHeadings := []string{"## Proposed Changes", "## Verification Plan"}
		for _, ch := range coreHeadings {
			if !seenHeadings[ch] {
				fileErrs = append(fileErrs, fmt.Sprintf("%s: missing heading %q", planFile, ch))
			}
		}
	}

	if strings.ToLower(status) == "completed" {
		if !foundGoVersion || isPlaceholder(goVersionVal) {
			fileErrs = append(fileErrs, fmt.Sprintf("%s: status is Completed but Go Version is missing or a placeholder", planFile))
		}
		if !foundDateCompleted || isPlaceholder(dateCompletedVal) {
			fileErrs = append(fileErrs, fmt.Sprintf("%s: status is Completed but Date Completed is missing or a placeholder", planFile))
		}
		if !foundUnitTestCoverage || isPlaceholder(unitTestCoverageVal) {
			fileErrs = append(fileErrs, fmt.Sprintf("%s: status is Completed but Unit Test Coverage is missing or a placeholder", planFile))
		}
	}

	return fileErrs
}

func validateLink(workspaceRoot, planFile string, lineNum int, line string, label string, pathStr string, status *string) []string {
	var errs []string

	if isPathAbsolute(pathStr) {
		errs = append(errs, fmt.Sprintf("%s:%d: path %q must be relative, not absolute", planFile, lineNum, pathStr))
	}

	absPath, err := getAbsolutePath(workspaceRoot, planFile, pathStr)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s:%d: failed to resolve path %q: %v", planFile, lineNum, pathStr, err))
		return errs
	}

	cleanRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s:%d: failed to get absolute workspace root: %v", planFile, lineNum, err))
		return errs
	}

	isInside := absPath == cleanRoot || strings.HasPrefix(absPath, cleanRoot+string(filepath.Separator))
	if !isInside {
		errs = append(errs, fmt.Sprintf("%s:%d: path %q resolves to %q which is outside workspace root %q", planFile, lineNum, pathStr, absPath, cleanRoot))
		return errs
	}

	expectedBasename := filepath.Base(absPath)
	if strings.TrimSpace(label) != expectedBasename {
		errs = append(errs, fmt.Sprintf("%s:%d: link label %q does not match actual file basename %q", planFile, lineNum, label, expectedBasename))
	}

	if strings.Contains(line, "[MODIFY]") {
		//nolint:gosec
		if fi, err := os.Stat(absPath); err != nil {
			errs = append(errs, fmt.Sprintf("%s:%d: modified file %q does not exist on disk", planFile, lineNum, absPath))
		} else if fi.IsDir() {
			errs = append(errs, fmt.Sprintf("%s:%d: modified path %q is a directory, not a file", planFile, lineNum, absPath))
		}
	}

	if strings.Contains(line, "[NEW]") && strings.ToLower(*status) == "completed" {
		//nolint:gosec
		if fi, err := os.Stat(absPath); err != nil {
			errs = append(errs, fmt.Sprintf("%s:%d: completed new file %q does not exist on disk", planFile, lineNum, absPath))
		} else if fi.IsDir() {
			errs = append(errs, fmt.Sprintf("%s:%d: completed new path %q is a directory, not a file", planFile, lineNum, absPath))
		}
	}

	return errs
}

func getAbsolutePath(workspaceRoot, planFile, pathStr string) (string, error) {
	pathStr = strings.TrimPrefix(pathStr, "file://")

	// Compatibility normalization for historical absolute /Users/human/code/powerword/ paths
	if filepath.IsAbs(pathStr) || strings.HasPrefix(pathStr, "/") {
		for _, marker := range []string{"/code/powerword/", "/powerword/"} {
			if idx := strings.Index(pathStr, marker); idx != -1 {
				suffix := pathStr[idx+len(marker):]
				absPath := filepath.Clean(filepath.Join(workspaceRoot, suffix))
				return filepath.Abs(absPath)
			}
		}
	}

	var absPath string
	if filepath.IsAbs(pathStr) || strings.HasPrefix(pathStr, "/") {
		absPath = filepath.Clean(pathStr)
	} else {
		absPath = filepath.Clean(filepath.Join(filepath.Dir(planFile), pathStr))
	}

	return filepath.Abs(absPath)
}

func isPlaceholder(val string) bool {
	val = strings.TrimSpace(val)
	val = strings.Trim(val, "*_` ")
	if val == "" {
		return true
	}
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		return true
	}
	upper := strings.ToUpper(val)
	if upper == "TBD" || strings.Contains(upper, "TBD") {
		return true
	}
	if strings.Contains(upper, "GO VERSION") || strings.Contains(upper, "DATE COMPLETED") || strings.Contains(upper, "UNIT TEST COVERAGE") {
		return true
	}
	return false
}

func isPathAbsolute(pathStr string) bool {
	p := strings.TrimPrefix(pathStr, "file://")
	return filepath.IsAbs(p) || strings.HasPrefix(p, "/")
}

// FixAbsolutePathsInPlans recursively scans the plans/ directory and fixes absolute paths,
// mismatched labels, missing URI schemes, and auto-populates metadata on completed plans.
//
//nolint:gocognit,funlen,nestif
func FixAbsolutePathsInPlans(workspaceRoot string, cfg *config.Config) (int, error) {
	plansDir := filepath.Join(workspaceRoot, "plans")
	planFiles, err := scanPlanFiles(plansDir)
	if err != nil {
		return 0, fmt.Errorf("failed to scan plan files: %w", err)
	}

	var goVersionLineRegex = regexp.MustCompile(`(?i)(^(?:\s*[-*+]?\s*)?(?:\*\*|\*)?Go Version\b[^:]*:\s*(?:\*\*|\*|)?\s*)(.*)`)
	var dateCompletedLineRegex = regexp.MustCompile(`(?i)(^(?:\s*[-*+]?\s*)?(?:\*\*|\*)?Date Completed\b[^:]*:\s*(?:\*\*|\*|)?\s*)(.*)`)

	goVersion := getGoVersionFromMod(workspaceRoot)
	todayStr := time.Now().Format("2006-01-02")

	modifiedCount := 0

	for _, planFile := range planFiles {
		if filepath.Base(planFile) == "TEMPLATE.md" {
			continue
		}

		//nolint:gosec
		content, err := os.ReadFile(planFile)
		if err != nil {
			return modifiedCount, fmt.Errorf("failed to read plan file %s: %w", planFile, err)
		}

		lines := strings.Split(string(content), "\n")
		fileModified := false

		// 1. Pass: check status
		isCompleted := false
		for _, line := range lines {
			if statusMatch := statusRegex.FindStringSubmatch(line); len(statusMatch) > 1 {
				if strings.ToLower(statusMatch[1]) == "completed" {
					isCompleted = true
					break
				}
			}
		}

		// 2. Pass: edit lines
		for i, line := range lines {
			newLine := line

			// Handle links
			links := linkRegex.FindAllStringSubmatch(newLine, -1)
			for _, match := range links {
				label := match[1]
				linkPath := match[2]

				// Determine if it looks like a local codebase path (e.g. not a website, mailto, etc.)
				isLocal := strings.HasPrefix(linkPath, "file://") ||
					(!strings.HasPrefix(linkPath, "http://") &&
						!strings.HasPrefix(linkPath, "https://") &&
						!strings.Contains(linkPath, "://") &&
						linkPath != "")

				if !isLocal {
					continue
				}

				absPath, absPathErr := getAbsolutePath(workspaceRoot, planFile, linkPath)
				if absPathErr != nil {
					continue
				}

				cleanRoot, absRootErr := filepath.Abs(workspaceRoot)
				if absRootErr != nil {
					continue
				}

				isInside := absPath == cleanRoot || strings.HasPrefix(absPath, cleanRoot+string(filepath.Separator))
				if !isInside {
					continue
				}

				planDir, planDirErr := filepath.Abs(filepath.Dir(planFile))
				if planDirErr != nil {
					continue
				}

				relPath, relPathErr := filepath.Rel(planDir, absPath)
				if relPathErr != nil {
					continue
				}

				relPathSlash := filepath.ToSlash(relPath)
				expectedLinkPath := "file://" + relPathSlash
				expectedLabel := filepath.Base(absPath)

				// If it differs, replace it on the line
				if linkPath != expectedLinkPath || strings.TrimSpace(label) != expectedLabel {
					oldLink := fmt.Sprintf("[%s](%s)", label, linkPath)
					newLink := fmt.Sprintf("[%s](%s)", expectedLabel, expectedLinkPath)
					newLine = strings.Replace(newLine, oldLink, newLink, 1)
				}
			}

			// Handle metadata if status is completed
			if isCompleted {
				if gvMatch := goVersionLineRegex.FindStringSubmatch(newLine); len(gvMatch) > 1 {
					cleanVal := strings.Trim(gvMatch[2], "*_` ")
					if isPlaceholder(cleanVal) {
						newLine = strings.Replace(newLine, cleanVal, goVersion, 1)
					}
				}
				if dcMatch := dateCompletedLineRegex.FindStringSubmatch(newLine); len(dcMatch) > 1 {
					cleanVal := strings.Trim(dcMatch[2], "*_` ")
					if isPlaceholder(cleanVal) {
						newLine = strings.Replace(newLine, cleanVal, todayStr, 1)
					}
				}
			}

			if newLine != line {
				lines[i] = newLine
				fileModified = true
			}
		}

		if fileModified {
			//nolint:gosec
			err = os.WriteFile(planFile, []byte(strings.Join(lines, "\n")), 0600)
			if err != nil {
				return modifiedCount, fmt.Errorf("failed to write plan file %s: %w", planFile, err)
			}
			modifiedCount++
		}
	}

	return modifiedCount, nil
}

func getGoVersionFromMod(workspaceRoot string) string {
	goModPath := filepath.Join(workspaceRoot, "go.mod")
	//nolint:gosec
	content, err := os.ReadFile(goModPath)
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "go ") {
				return strings.TrimSpace(line[3:])
			}
		}
	}
	return "1.26"
}
