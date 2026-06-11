package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/borch-ai/powerword/internal/linter"
)

func main() {
	files, err := walkMarkdownFiles(".")
	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}

	failed := lintFiles(files)

	if failed {
		fmt.Println("Markdown lint check failed.")
		os.Exit(1)
	}

	fmt.Println("Markdown lint check passed successfully.")
}

func walkMarkdownFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories (like .git, .gemini, .github) and built artifacts
			name := info.Name()
			if (strings.HasPrefix(name, ".") && path != ".") || name == "bin" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".md" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func lintFiles(files []string) bool {
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
	return failed
}
