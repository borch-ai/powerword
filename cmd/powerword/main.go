package main

import (
	"fmt"
	"os"

	"github.com/borch-ai/powerword/internal/config"
	"github.com/borch-ai/powerword/internal/loop"
)

func main() {
	config.Runner = loop.RunLoop
	rootCmd := config.NewRootCmd()
	rootCmd.AddCommand(newModelsCmd())
	rootCmd.AddCommand(newReviewCmd())
	rootCmd.AddCommand(newRunCmd())
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
