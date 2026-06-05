package main

import (
	"fmt"
	"os"

	"powerword/internal/config"
	"powerword/internal/loop"
)

func main() {
	config.Runner = loop.RunLoop
	if err := config.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
