package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/borch-ai/powerword/internal/linter"
)

func main() {
	files, err := filepath.Glob("plans/*.md")
	if err != nil {
		fmt.Printf("Error finding plans: %v\n", err)
		os.Exit(1)
	}

	failed := false
	for _, file := range files {
		// Skip TEMPLATE.md
		if filepath.Base(file) == "TEMPLATE.md" {
			continue
		}

		// #nosec G304
		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Error reading plan file %s: %v\n", file, err)
			failed = true
			continue
		}

		errors := linter.LintPlan(file, string(content))
		if len(errors) > 0 {
			failed = true
			for _, errStr := range errors {
				fmt.Println(errStr)
			}
		}
	}

	if failed {
		fmt.Println("Plan template validation failed.")
		os.Exit(1)
	}

	fmt.Println("Plan template validation passed successfully.")
}
