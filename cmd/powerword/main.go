package main

import (
	"fmt"
	"os"
	"time"

	"github.com/borch-ai/powerword/internal/loop"
	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/telemetry"
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

	// Wait up to configurable timeout (default 500ms) for background telemetry to flush
	timeout := 500 * time.Millisecond
	if tStr := os.Getenv("POWERWORD_TELEMETRY_TIMEOUT"); tStr != "" {
		if d, pErr := time.ParseDuration(tStr); pErr == nil {
			timeout = d
		}
	}

	done := make(chan struct{})
	go func() {
		telemetry.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
