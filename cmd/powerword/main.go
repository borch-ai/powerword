package main

import (
	"fmt"
	"os"

	"powerword/internal/config"
	"powerword/internal/loop"
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
