package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	planRegex  = regexp.MustCompile(`^plans/.*\.md$`)
	issueRegex = regexp.MustCompile(`(?i)Issue\s+#(\d+)`)
)

func LinkTaskIssue(ctx context.Context, prNumber, baseRef string) error {
	fmt.Printf("Analyzing changes in PR #%s compared to base ref '%s'...\n", prNumber, baseRef)

	// 1. Fetch base branch
	gitFetch(ctx, baseRef)

	// 2. Find modified plans
	plansToCheck, err := getPlansToCheck(ctx, baseRef)
	if err != nil {
		return err
	}
	if len(plansToCheck) == 0 {
		fmt.Println("No task plan files were modified in this PR. Exiting.")
		return nil
	}

	fmt.Printf("Modified plan files detected:\n%s\n", strings.Join(plansToCheck, "\n"))

	// 3. Scan modified files for plan issue IDs
	orderedIssueIDs := scanPlansForIssueIDs(plansToCheck)
	if len(orderedIssueIDs) == 0 {
		fmt.Println("No task plan files with linked Issue IDs were modified in this PR. Exiting.")
		return nil
	}

	// 4. Retrieve current PR description
	fmt.Println("Retrieving current PR description...")
	prBody, err := getPRBody(ctx, prNumber)
	if err != nil {
		return err
	}
	fmt.Printf("Current PR description:\n----------------------\n%s\n----------------------\n", prBody)

	// 5. Check which Issue IDs are missing from description
	var missingRefs []string
	for _, id := range orderedIssueIDs {
		pattern := fmt.Sprintf(`(?i)(?:closes|resolves|fixes)\s+#%s\b`, id)
		re := regexp.MustCompile(pattern)
		if !re.MatchString(prBody) {
			missingRefs = append(missingRefs, id)
		}
	}

	if len(missingRefs) == 0 {
		fmt.Println("All matching issue references are already present in the PR description. No update needed.")
		return nil
	}

	fmt.Printf("Missing issue references to link: %s\n", strings.Join(missingRefs, ", "))

	// 6. Build the new PR body and edit PR
	return updatePRBody(ctx, prNumber, prBody, missingRefs)
}

func gitFetch(ctx context.Context, baseRef string) {
	cmd := execCommand(ctx, "git", "fetch", "origin", baseRef, "--depth=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("[WARNING] Failed to fetch origin/%s with depth=1: %v. Attempting full fetch...\n", baseRef, err)
		cmdFull := execCommand(ctx, "git", "fetch", "origin", baseRef)
		cmdFull.Stdout = os.Stdout
		cmdFull.Stderr = os.Stderr
		if err := cmdFull.Run(); err != nil {
			fmt.Printf("[WARNING] Failed to fetch origin/%s: %v. Proceeding anyway...\n", baseRef, err)
		}
	}
}

func getPlansToCheck(ctx context.Context, baseRef string) ([]string, error) {
	diffCmd := execCommand(ctx, "git", "diff", "--name-only", fmt.Sprintf("origin/%s...HEAD", baseRef))
	out, err := diffCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git diff: %w", err)
	}

	modifiedFiles := strings.Split(string(out), "\n")
	var plans []string
	for _, file := range modifiedFiles {
		file = strings.TrimSpace(file)
		if file != "" && planRegex.MatchString(file) {
			plans = append(plans, file)
		}
	}
	return plans, nil
}

func scanPlansForIssueIDs(plans []string) []string {
	issueIDs := make(map[string]bool)
	var orderedIssueIDs []string
	for _, file := range plans {
		//nolint:gosec // path is read from git diff output, so it is safe
		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("[WARNING] Could not read plan file %s: %v. Skipping.\n", file, err)
			continue
		}

		matches := issueRegex.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) > 1 {
				id := match[1]
				if !issueIDs[id] {
					issueIDs[id] = true
					orderedIssueIDs = append(orderedIssueIDs, id)
					fmt.Printf("Found Issue ID #%s inside plan file: %s\n", id, file)
				}
			}
		}
	}
	return orderedIssueIDs
}

func getPRBody(ctx context.Context, prNumber string) (string, error) {
	prViewCmd := execCommand(ctx, "gh", "pr", "view", prNumber, "--json", "body")
	prOut, err := prViewCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve PR details (is gh installed and authenticated?): %w", err)
	}

	var prData struct {
		Body string `json:"body"`
	}
	if parseErr := json.Unmarshal(prOut, &prData); parseErr != nil {
		return "", fmt.Errorf("failed to parse gh output: %w", parseErr)
	}
	return strings.TrimSpace(prData.Body), nil
}

func updatePRBody(ctx context.Context, prNumber, prBody string, missingRefs []string) error {
	newBody := prBody
	if len(newBody) > 0 && !strings.HasSuffix(newBody, "\n") {
		newBody += "\n"
	}

	marker := "<!-- Auto-linked via CI plan checker -->"
	if !strings.Contains(newBody, marker) {
		newBody += "\n" + marker
	}

	for _, id := range missingRefs {
		newBody += "\nCloses #" + id
	}

	fmt.Println("Updating PR description body...")
	tempFile, err := os.CreateTemp("", "temp_pr_body_*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file for PR body: %w", err)
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	if _, writeErr := tempFile.WriteString(newBody); writeErr != nil {
		return fmt.Errorf("failed to write updated PR body to temp file: %w", writeErr)
	}
	if closeErr := tempFile.Close(); closeErr != nil {
		return fmt.Errorf("failed to close temp file: %w", closeErr)
	}

	prEditCmd := execCommand(ctx, "gh", "pr", "edit", prNumber, "--body-file", tempFile.Name())
	prEditCmd.Stdout = os.Stdout
	prEditCmd.Stderr = os.Stderr
	if editErr := prEditCmd.Run(); editErr != nil {
		return fmt.Errorf("failed to update PR body: %w", editErr)
	}

	fmt.Println("[SUCCESS] Successfully updated the PR description with closing references.")
	return nil
}
