package main

import (
	"fmt"

	"powerword/internal/config"
	"powerword/internal/review"

	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run the agent with a prompt or in autonomous mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Active
			if cfg == nil {
				return fmt.Errorf("configuration not loaded")
			}

			prompt := ""
			if len(args) > 0 {
				prompt = args[0]
			}

			if cfg.Autonomous {
				return review.RunAutonomousLoop(cmd.Context(), cfg)
			}

			if config.Runner == nil {
				return fmt.Errorf("no execution runner registered")
			}
			return config.Runner(cmd.Context(), cfg, prompt)
		},
	}

	return cmd
}
