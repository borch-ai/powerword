package main

import (
	"fmt"
	"os"

	"github.com/borch-ai/powerword/internal/loop"
	"github.com/borch-ai/powerword/pkg/config"
)

func main() {
	config.Runner = loop.RunLoop
	rootCmd := config.NewRootCmd()
	rootCmd.AddCommand(newModelsCmd())
	rootCmd.AddCommand(newReviewCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newCheckCoverageCmd())
	rootCmd.AddCommand(newLintPlansCmd())
	rootCmd.AddCommand(newLintGoCmd())
	rootCmd.AddCommand(newLinkIssueCmd())

	err := rootCmd.Execute()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
