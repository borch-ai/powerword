package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/borch-ai/powerword/internal/review"
	"github.com/spf13/cobra"
)

func newCheckCoverageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-coverage <threshold> [profile_path]",
		Short: "Check that test coverage meets a specified threshold percentage",
		Args:  cobra.RangeArgs(1, 2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			thresholdStr := args[0]
			threshold, err := strconv.ParseFloat(thresholdStr, 64)
			if err != nil {
				return fmt.Errorf("invalid threshold percentage %q: %w", thresholdStr, err)
			}

			profilePath := "coverage.out"
			if len(args) > 1 {
				profilePath = args[1]
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			return review.VerifyCoverage(ctx, threshold, profilePath)
		},
	}
	return cmd
}
