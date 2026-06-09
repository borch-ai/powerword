package main

import (
	"fmt"

	"powerword/internal/config"
	"powerword/internal/review"

	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	var issueID string
	var localOnly bool

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review local workspace changes against an implementation plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Active
			if cfg == nil {
				return fmt.Errorf("configuration not loaded")
			}

			if !localOnly && issueID == "" {
				return fmt.Errorf("either --issue or --local must be provided")
			}
			if localOnly && issueID != "" {
				return fmt.Errorf("cannot provide both --local and --issue flags")
			}

			var plan *review.Plan
			if localOnly {
				plan = &review.Plan{
					Goal:         "Comply with general workspace rules.",
					Changes:      "All staged and unstaged changes.",
					Verification: "make all and code review guidelines.",
				}
			} else {
				cmd.Printf("Loading issue %s...\n", issueID)
				var err error
				plan, err = review.LoadIssuePlan(cmd.Context(), issueID)
				if err != nil {
					return err
				}
			}

			return review.VerifyWorkspace(cmd.Context(), plan, cfg)
		},
	}

	cmd.Flags().StringVar(&issueID, "issue", "", "GitHub issue ID containing the active plan")
	cmd.Flags().BoolVar(&localOnly, "local", false, "Run local validation and ruleset verification only (without fetching a remote issue)")

	return cmd
}
