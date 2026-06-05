package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"powerword/internal/linter"
)

func main() {
	var files []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories (like .git, .gemini, .github) and built artifacts
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "bin" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".md" {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}

	failed := false
	for _, file := range files {
		// #nosec G304
		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", file, err)
			failed = true
			continue
		}

		errors := linter.LintMarkdown(file, string(content))
		if len(errors) > 0 {
			failed = true
			for _, errStr := range errors {
				fmt.Println(errStr)
			}
		}
	}

	if failed {
		fmt.Println("Markdown lint check failed.")
		os.Exit(1)
	}

	fmt.Println("Markdown lint check passed successfully.")
}
