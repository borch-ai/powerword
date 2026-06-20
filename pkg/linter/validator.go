package linter

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

// safeReadFile reads a file from a path within the trusted boundary.
// G304: path is restricted to workspace-scoped directory trees or explicitly user-configured template overrides.
//
//nolint:gosec // G304: paths are restricted to the trusted workspace boundaries or explicit user configuration overrides
func safeReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// safeStat stats a file from a path within the trusted boundary.
// G304: path is restricted to workspace-scoped directory trees or explicitly user-configured template overrides.
//
//nolint:gosec // G304: paths are restricted to the trusted workspace boundaries or explicit user configuration overrides
func safeStat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func resolveTemplate(workspaceRoot string, cfg *config.Config) (string, error) {
	// 1. Workspace-specific
	workspacePath := filepath.Join(workspaceRoot, "plans", "TEMPLATE.md")
	if _, err := safeStat(workspacePath); err == nil {
		content, err := safeReadFile(workspacePath)
		if err != nil {
			return "", fmt.Errorf("failed to read workspace plan template %s: %w", workspacePath, err)
		}
		return string(content), nil
	}

	// 2. Global Config
	if cfg != nil && cfg.PlanTemplate != "" {
		if _, err := safeStat(cfg.PlanTemplate); err == nil {
			content, err := safeReadFile(cfg.PlanTemplate)
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

// planParseResult holds everything extracted from one parse pass over a plan file.
type planParseResult struct {
	hasTitle              bool
	seenHeadings          map[string]bool
	status                string
	foundGoVersion        bool
	foundDateCompleted    bool
	foundUnitTestCoverage bool
	goVersionVal          string
	dateCompletedVal      string
	unitTestCoverageVal   string
	links                 []planLink
	scanErr               error
}

type planLink struct {
	lineNum  int
	line     string
	label    string
	linkPath string
}

// parsePlanContent scans plan file content and extracts headings, status, metadata, and links.
func parsePlanContent(content string, titleRegex *regexp.Regexp) planParseResult {
	result := planParseResult{seenHeadings: make(map[string]bool)}

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	inCodeBlock := false

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

		parseLineContent(line, lineNum, titleRegex, &result)
	}
	result.scanErr = scanner.Err()
	return result
}

func parseLineContent(line string, lineNum int, titleRegex *regexp.Regexp, result *planParseResult) {
	if strings.HasPrefix(line, "# ") && titleRegex.MatchString(line) {
		result.hasTitle = true
	}
	if strings.HasPrefix(line, "## ") {
		result.seenHeadings[strings.TrimSpace(line)] = true
	}
	if m := statusRegex.FindStringSubmatch(line); len(m) > 1 {
		result.status = m[1]
	}
	if m := goVersionRegex.FindStringSubmatch(line); len(m) > 1 {
		result.foundGoVersion = true
		result.goVersionVal = m[1]
	}
	if m := dateCompletedRegex.FindStringSubmatch(line); len(m) > 1 {
		result.foundDateCompleted = true
		result.dateCompletedVal = m[1]
	}
	if m := unitTestCoverageRegex.FindStringSubmatch(line); len(m) > 1 {
		result.foundUnitTestCoverage = true
		result.unitTestCoverageVal = m[1]
	}

	for _, match := range linkRegex.FindAllStringSubmatch(line, -1) {
		label, linkPath := match[1], match[2]
		isAction := strings.Contains(line, "[NEW]") || strings.Contains(line, "[MODIFY]") || strings.Contains(line, "[DELETE]")
		if isAction || strings.HasPrefix(linkPath, "file://") {
			result.links = append(result.links, planLink{lineNum: lineNum, line: line, label: label, linkPath: linkPath})
		}
	}
}

// checkCompletedMetadata returns errors for any missing/placeholder metadata fields on a completed plan.
func checkCompletedMetadata(planFile string, r planParseResult) []string {
	var errs []string
	if !r.foundGoVersion || isPlaceholder(r.goVersionVal) {
		errs = append(errs, fmt.Sprintf("%s: status is Completed but Go Version is missing or a placeholder", planFile))
	}
	if !r.foundDateCompleted || isPlaceholder(r.dateCompletedVal) {
		errs = append(errs, fmt.Sprintf("%s: status is Completed but Date Completed is missing or a placeholder", planFile))
	}
	if !r.foundUnitTestCoverage || isPlaceholder(r.unitTestCoverageVal) {
		errs = append(errs, fmt.Sprintf("%s: status is Completed but Unit Test Coverage is missing or a placeholder", planFile))
	}
	return errs
}

func validateSinglePlan(workspaceRoot string, planFile string, titleRegex *regexp.Regexp, templateHeadings []string) []string {
	var fileErrs []string

	content, err := safeReadFile(planFile)
	if err != nil {
		return []string{fmt.Sprintf("%s: failed to read file: %v", planFile, err)}
	}

	r := parsePlanContent(string(content), titleRegex)

	if r.scanErr != nil {
		return append(fileErrs, fmt.Sprintf("%s: scanning error: %v", planFile, r.scanErr))
	}

	if !r.hasTitle {
		fileErrs = append(fileErrs, fmt.Sprintf("%s: missing top-level plan header matching title pattern", planFile))
	}

	for _, reqHeading := range templateHeadings {
		if !r.seenHeadings[reqHeading] {
			fileErrs = append(fileErrs, fmt.Sprintf("%s: missing heading %q", planFile, reqHeading))
		}
	}

	if len(templateHeadings) == 0 {
		for _, ch := range []string{"## Proposed Changes", "## Verification Plan"} {
			if !r.seenHeadings[ch] {
				fileErrs = append(fileErrs, fmt.Sprintf("%s: missing heading %q", planFile, ch))
			}
		}
	}

	if strings.ToLower(r.status) == "completed" {
		fileErrs = append(fileErrs, checkCompletedMetadata(planFile, r)...)
	}

	for _, lnk := range r.links {
		fileErrs = append(fileErrs, validateLink(workspaceRoot, planFile, lnk.lineNum, lnk.line, lnk.label, lnk.linkPath, &r.status)...)
	}

	return fileErrs
}

func isGitIgnoredOrOptional(path string) bool {
	base := filepath.Base(path)
	ignoredBases := map[string]bool{
		"powerword.toml":       true,
		"config.yaml":          true,
		".env":                 true,
		".powerword-critic.md": true,
		"review_out.txt":       true,
	}
	if ignoredBases[base] {
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		return true
	}
	return false
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

	if strings.Contains(line, "[MODIFY]") && !isGitIgnoredOrOptional(absPath) {
		if fi, err := safeStat(absPath); err != nil {
			errs = append(errs, fmt.Sprintf("%s:%d: modified file %q does not exist on disk", planFile, lineNum, absPath))
		} else if fi.IsDir() {
			errs = append(errs, fmt.Sprintf("%s:%d: modified path %q is a directory, not a file", planFile, lineNum, absPath))
		}
	}

	if strings.Contains(line, "[NEW]") && strings.ToLower(*status) == "completed" && !isGitIgnoredOrOptional(absPath) {
		if fi, err := safeStat(absPath); err != nil {
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
	if isPathAbsolute(pathStr) {
		for _, marker := range []string{"/code/powerword/", "/powerword/"} {
			if idx := strings.Index(pathStr, marker); idx != -1 {
				suffix := pathStr[idx+len(marker):]
				absPath := filepath.Clean(filepath.Join(workspaceRoot, suffix))
				return filepath.Abs(absPath)
			}
		}
	}

	var absPath string
	if isPathAbsolute(pathStr) {
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
	// Standard Unix absolute or runtime-environment absolute
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return true
	}
	// Windows drive letter absolute (e.g. C:/ or C:\)
	if len(p) >= 3 && p[1] == ':' && (p[2] == '/' || p[2] == '\\') && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
		return true
	}
	// Windows UNC paths (e.g. \\server\share or //server/share)
	if strings.HasPrefix(p, "\\\\") || strings.HasPrefix(p, "//") {
		return true
	}
	return false
}

// FixAbsolutePathsInPlans recursively scans the plans/ directory and fixes absolute paths,
// mismatched labels, missing URI schemes, and auto-populates metadata on completed plans.
func FixAbsolutePathsInPlans(workspaceRoot string, cfg *config.Config) (int, error) {
	plansDir := filepath.Join(workspaceRoot, "plans")
	planFiles, err := scanPlanFiles(plansDir)
	if err != nil {
		return 0, fmt.Errorf("failed to scan plan files: %w", err)
	}

	goVersion := getGoVersionFromMod(workspaceRoot)
	todayStr := time.Now().Format("2006-01-02")
	modifiedCount := 0

	for _, planFile := range planFiles {
		if filepath.Base(planFile) == "TEMPLATE.md" {
			continue
		}

		content, err := safeReadFile(planFile)
		if err != nil {
			return modifiedCount, fmt.Errorf("failed to read plan file %s: %w", planFile, err)
		}

		lines := strings.Split(string(content), "\n")
		newLines, fileModified, fixErr := fixPlanFileLines(workspaceRoot, planFile, lines, goVersion, todayStr)
		if fixErr != nil {
			return modifiedCount, fixErr
		}

		if fileModified {
			if writeErr := os.WriteFile(planFile, []byte(strings.Join(newLines, "\n")), 0600); writeErr != nil { //nolint:gosec // G306: plan files are user-owned workspace files with 0600 perms
				return modifiedCount, fmt.Errorf("failed to write plan file %s: %w", planFile, writeErr)
			}
			modifiedCount++
		}
	}

	return modifiedCount, nil
}

// fixPlanFileLines applies link normalization and metadata patching to a slice of plan file lines.
func fixPlanFileLines(workspaceRoot, planFile string, lines []string, goVersion, todayStr string) ([]string, bool, error) {
	var goVersionLineRegex = regexp.MustCompile(`(?i)(^(?:\s*[-*+]?\s*)?(?:\*\*|\*)?Go Version\b[^:]*:\s*(?:\*\*|\*|)?\s*)(.*)`)
	var dateCompletedLineRegex = regexp.MustCompile(`(?i)(^(?:\s*[-*+]?\s*)?(?:\*\*|\*)?Date Completed\b[^:]*:\s*(?:\*\*|\*|)?\s*)(.*)`)

	// Determine status in a first pass
	isCompleted := false
	for _, line := range lines {
		if m := statusRegex.FindStringSubmatch(line); len(m) > 1 && strings.ToLower(m[1]) == "completed" {
			isCompleted = true
			break
		}
	}

	fileModified := false
	for i, line := range lines {
		newLine := fixLineLinks(workspaceRoot, planFile, line)
		if isCompleted {
			newLine = patchCompletedMetadata(newLine, goVersionLineRegex, dateCompletedLineRegex, goVersion, todayStr)
		}
		if newLine != line {
			lines[i] = newLine
			fileModified = true
		}
	}
	return lines, fileModified, nil
}

// fixLineLinks rewrites any local file links on a single line to use relative paths and correct labels.
func fixLineLinks(workspaceRoot, planFile, line string) string {
	for _, match := range linkRegex.FindAllStringSubmatch(line, -1) {
		label, linkPath := match[1], match[2]

		isLocal := strings.HasPrefix(linkPath, "file://") ||
			(!strings.HasPrefix(linkPath, "http://") &&
				!strings.HasPrefix(linkPath, "https://") &&
				!strings.Contains(linkPath, "://") &&
				linkPath != "")
		if !isLocal {
			continue
		}

		absPath, err := getAbsolutePath(workspaceRoot, planFile, linkPath)
		if err != nil {
			continue
		}

		cleanRoot, err := filepath.Abs(workspaceRoot)
		if err != nil {
			continue
		}

		isInside := absPath == cleanRoot || strings.HasPrefix(absPath, cleanRoot+string(filepath.Separator))
		if !isInside {
			continue
		}

		planDir, err := filepath.Abs(filepath.Dir(planFile))
		if err != nil {
			continue
		}

		relPath, err := filepath.Rel(planDir, absPath)
		if err != nil {
			continue
		}

		expectedLinkPath := "file://" + filepath.ToSlash(relPath)
		expectedLabel := filepath.Base(absPath)

		if linkPath != expectedLinkPath || strings.TrimSpace(label) != expectedLabel {
			line = strings.Replace(line, fmt.Sprintf("[%s](%s)", label, linkPath), fmt.Sprintf("[%s](%s)", expectedLabel, expectedLinkPath), 1)
		}
	}
	return line
}

// patchCompletedMetadata fills in placeholder Go Version and Date Completed fields on completed plan lines.
func patchCompletedMetadata(line string, goVersionRe, dateCompletedRe *regexp.Regexp, goVersion, todayStr string) string {
	if m := goVersionRe.FindStringSubmatch(line); len(m) > 1 {
		if isPlaceholder(strings.Trim(m[2], "*_` ")) {
			return m[1] + goVersion
		}
	}
	if m := dateCompletedRe.FindStringSubmatch(line); len(m) > 1 {
		if isPlaceholder(strings.Trim(m[2], "*_` ")) {
			return m[1] + todayStr
		}
	}
	return line
}

func getGoVersionFromMod(workspaceRoot string) string {
	goModPath := filepath.Join(workspaceRoot, "go.mod")
	content, err := safeReadFile(goModPath)
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
