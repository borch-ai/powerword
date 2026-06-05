package main

import (
	"fmt"
	"os"

	"powerword/internal/config"
)

func main() {
	if err := config.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
