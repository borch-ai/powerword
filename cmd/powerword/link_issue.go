package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/borch-ai/powerword/internal/review"
	"github.com/spf13/cobra"
)

//nolint:gocognit // setup is inherently complex but readable
func newLinkIssueCmd() *cobra.Command {
	var prNumber string
	var baseRef string

	cmd := &cobra.Command{
		Use:   "link-issue",
		Short: "Auto-link issues referenced in changed task plans to the current pull request description",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// Fallback to env vars
			if prNumber == "" {
				prNumber = os.Getenv("PR_NUMBER")
			}
			if baseRef == "" {
				baseRef = os.Getenv("BASE_REF")
			}

			// Only invoke gh auto-detection when PR number is still unknown.
			// baseRef can safely default to "main" without requiring gh.
			if prNumber == "" {
				cmd.Printf("PR number not provided, attempting to auto-detect via gh CLI...\n")
				detPR, detBase, err := detectPRInfo(ctx)
				if err != nil {
					return err
				}
				prNumber = detPR
				if baseRef == "" && detBase != "" {
					baseRef = detBase
				}
			}

			if prNumber == "" {
				return fmt.Errorf("could not determine PR number. Please provide --pr flag or set PR_NUMBER env var")
			}
			if baseRef == "" {
				baseRef = "main" // standard default fallback
			}

			prNumber = strings.TrimSpace(prNumber)
			baseRef = strings.TrimSpace(baseRef)

			cmd.Printf("Running link-issue with PR: #%s, Base Ref: %s...\n", prNumber, baseRef)
			return review.LinkTaskIssue(ctx, prNumber, baseRef)
		},
	}

	cmd.Flags().StringVar(&prNumber, "pr", "", "GitHub Pull Request number")
	cmd.Flags().StringVar(&baseRef, "base", "", "Base branch/ref to diff against (defaults to main)")

	return cmd
}

func detectPRInfo(ctx context.Context) (string, string, error) {
	//nolint:gosec // gh is local authenticated command
	detectCmd := exec.CommandContext(ctx, "gh", "pr", "view", "--json", "number,baseRefName")
	out, err := detectCmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to auto-detect PR info using gh CLI (is gh installed, authenticated, and are you on a PR branch?): %w", err)
	}

	var prData struct {
		Number      int    `json:"number"`
		BaseRefName string `json:"baseRefName"`
	}
	if err := json.Unmarshal(out, &prData); err != nil {
		return "", "", fmt.Errorf("failed to parse gh output: %w", err)
	}

	var prNum string
	if prData.Number != 0 {
		prNum = fmt.Sprintf("%d", prData.Number)
	}
	return prNum, prData.BaseRefName, nil
}
