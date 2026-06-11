package main

import (
	"fmt"

	"github.com/borch-ai/powerword/internal/config"
	"github.com/borch-ai/powerword/internal/review"

	"github.com/spf13/cobra"
)

//nolint:gocognit // CLI command setup is inherently complex
func newReviewCmd() *cobra.Command {
	var issueID string
	var localOnly bool
	var listen bool
	var port int

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review local workspace changes against an implementation plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Active
			if cfg == nil {
				return fmt.Errorf("configuration not loaded")
			}

			if listen {
				listenPort := port
				if listenPort == 0 {
					listenPort = cfg.WebhookPort
					if listenPort == 0 {
						listenPort = 8080
					}
				}
				return review.StartWebhookListener(cmd.Context(), cfg, listenPort)
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
	cmd.Flags().BoolVar(&listen, "listen", false, "Start the webhook listener daemon")
	cmd.Flags().IntVar(&port, "port", 0, "Port to bind the listener to (defaults to config webhook_port or 8080)")

	return cmd
}
