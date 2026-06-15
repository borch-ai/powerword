package main

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/linter"
	"github.com/spf13/cobra"
)

//go:embed assets/golangci.yml
var defaultGolangciConfig string

func scanMarkdownFiles(path string) ([]string, error) {
	plansDir := filepath.Join(path, "plans")
	if strings.HasSuffix(filepath.Clean(path), "plans") {
		plansDir = path
	}

	var mdFiles []string
	err := filepath.WalkDir(plansDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			mdFiles = append(mdFiles, p)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to scan markdown files: %w", err)
	}
	return mdFiles, nil
}

//nolint:gosec
func lintMarkdownFile(cmd *cobra.Command, file string) (bool, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		cmd.PrintErrf("Error reading file %s: %v\n", file, err)
		return false, err
	}

	errors := linter.LintMarkdown(file, string(content))
	if len(errors) > 0 {
		for _, errStr := range errors {
			cmd.PrintErrln(errStr)
		}
		return false, nil
	}
	return true, nil
}

func newLintPlansCmd() *cobra.Command {
	var path string
	var templatePath string

	cmd := &cobra.Command{
		Use:   "lint-plans",
		Short: "Validate implementation plans and markdown formatting",
		Args:  cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig("")
			if err != nil {
				cfg = &config.Config{}
			}
			config.Active = cfg
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Active
			if templatePath != "" {
				cfg.PlanTemplate = templatePath
			}

			cmd.Printf("Validating plan files in %q...\n", path)
			if err := linter.ValidatePlans(path, cfg); err != nil {
				return err
			}

			mdFiles, err := scanMarkdownFiles(path)
			if err != nil {
				return err
			}

			failed := false
			for _, file := range mdFiles {
				if filepath.Base(file) == "TEMPLATE.md" {
					continue
				}

				ok, _ := lintMarkdownFile(cmd, file)
				if !ok {
					failed = true
				}
			}

			if failed {
				return fmt.Errorf("markdown lint check failed")
			}

			cmd.Println("All plan files and markdown formats conform successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "Target workspace path to validate (defaults to '.')")
	cmd.Flags().StringVar(&templatePath, "template", "", "Path to custom plan template")

	return cmd
}

//nolint:nestif
func resolveGolangciConfig(configPath string) (string, func(), error) {
	if configPath != "" {
		return configPath, func() {}, nil
	}

	localConfigs := []string{".golangci.yml", ".golangci.yaml", ".golangci.toml", ".golangci.json"}
	for _, lc := range localConfigs {
		if _, err := os.Stat(lc); err == nil {
			return lc, func() {}, nil
		}
	}

	// Fall back to embedded config with dynamic goimports local prefix
	// Create the temporary configuration file in the project root to preserve relative path resolution
	tmpFile, err := os.CreateTemp(".", ".golangci-temp-*.yml")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(tmpFile.Name())
	}

	moduleName := "github.com/borch-ai/powerword" // default fallback
	if mod, err := getModuleName("."); err == nil {
		moduleName = mod
	}

	configContent := strings.Replace(defaultGolangciConfig, "{{LOCAL_PREFIX}}", moduleName, 1)
	if _, err := tmpFile.WriteString(configContent); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to write config content: %w", err)
	}
	_ = tmpFile.Close()
	return tmpFile.Name(), cleanup, nil
}

func newLintGoCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "lint-go",
		Short: "Run golangci-lint on the Go codebase with dynamic module prefix support",
		Args:  cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// 1. Check if golangci-lint is installed
			_, err := exec.LookPath("golangci-lint")
			if err != nil {
				cmd.Println("Warning: golangci-lint not found in PATH, running basic go vet...")
				goCmd := exec.CommandContext(ctx, "go", "vet", "./...")
				goCmd.Stdout = os.Stdout
				goCmd.Stderr = os.Stderr
				return goCmd.Run()
			}

			// 2. Determine configuration to use
			activeConfigPath, cleanup, err := resolveGolangciConfig(configPath)
			if err != nil {
				return err
			}
			defer cleanup()

			// 3. Run golangci-lint
			lintCtx, lintCancel := context.WithTimeout(ctx, 5*time.Minute)
			defer lintCancel()

			//nolint:gosec
			lintCmd := exec.CommandContext(lintCtx, "golangci-lint", "run", "--config", activeConfigPath)
			lintCmd.Stdout = os.Stdout
			lintCmd.Stderr = os.Stderr
			return lintCmd.Run()
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to custom golangci-lint configuration file")

	return cmd
}

func getModuleName(dir string) (string, error) {
	goModPath := filepath.Join(dir, "go.mod")
	//nolint:gosec
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("module name not found in go.mod")
}
