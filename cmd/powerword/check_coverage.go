package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	internalmcp "github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/internal/review"
	"github.com/borch-ai/powerword/pkg/config"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func newCheckCoverageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-coverage <threshold> [profile_path]",
		Short: "Check that test coverage meets a specified threshold percentage",
		Args:  cobra.RangeArgs(1, 2),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			thresholdStr := args[0]
			threshold, err := strconv.ParseFloat(thresholdStr, 64)
			if err != nil {
				return fmt.Errorf("invalid threshold percentage %q: %w", thresholdStr, err)
			}

			profilePath := "coverage.out"
			if len(args) > 1 {
				profilePath = args[1]
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			return executeCheckCoverage(ctx, threshold, profilePath)
		},
	}
	return cmd
}

func executeCheckCoverage(ctx context.Context, threshold float64, profilePath string) error {
	// Auto-detect format
	fmtName := detectFormat(profilePath)
	if strings.ToLower(fmtName) == "go" {
		return review.VerifyCoverage(ctx, threshold, profilePath)
	}

	srvCfg := config.ServerConfig{
		Command: "pw-mcp-coverage",
		Args:    []string{},
	}

	// Check user-defined overrides in configuration
	cfg := config.Active
	hasOverride := false
	if cfg != nil {
		if srvVal, ok := cfg.Servers["coverage"]; ok {
			srvCfg = srvVal
			hasOverride = true
		}
	}

	if !hasOverride {
		// Only check bin/ or PATH if no override is provided
		if _, statErr := os.Stat("bin/pw-mcp-coverage"); statErr == nil {
			srvCfg.Command = "./bin/pw-mcp-coverage"
		} else if _, lookErr := exec.LookPath("pw-mcp-coverage"); lookErr != nil {
			return fmt.Errorf("pw-mcp-coverage not found in bin/ or PATH, run 'make build' first")
		}
	}

	srv, err := internalmcp.NewServerProcess(ctx, "coverage", srvCfg)
	if err != nil {
		return fmt.Errorf("failed to start MCP coverage server (%s): %w", srvCfg.Command, err)
	}
	defer func() {
		_ = srv.GracefulShutdown(time.Second * 5)
	}()

	res, err := srv.Client().CallTool(ctx, "check_coverage", map[string]interface{}{
		"threshold":    threshold,
		"profile_path": profilePath,
		"format":       fmtName,
	})
	if err != nil {
		return fmt.Errorf("coverage tool call failed: %w", err)
	}

	var msg string
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcpsdk.TextContent); ok {
			msg = tc.Text
		}
	}

	if res.IsError {
		if msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("coverage threshold check failed")
	}

	if msg != "" {
		fmt.Println(msg)
	}

	return nil
}

func detectFormat(path string) string {
	base := strings.ToLower(filepath.Base(path))
	if base == "lcov.info" || strings.HasSuffix(base, ".info") {
		return "lcov"
	}
	if strings.HasSuffix(base, ".xml") {
		return "cobertura"
	}
	if strings.HasSuffix(base, ".out") {
		return "go"
	}

	// Read first 4096 bytes of the file to check content safely
	//nolint:gosec // path is user-specified input from CLI/tool
	f, err := os.Open(path)
	if err != nil {
		return "go" // default fallback
	}
	defer func() {
		_ = f.Close()
	}()

	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	content := string(buf[:n])

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "mode:") {
			return "go"
		}
		if strings.HasPrefix(line, "SF:") || strings.HasPrefix(line, "TN:") {
			return "lcov"
		}
	}

	if strings.Contains(content, "<?xml") || strings.Contains(content, "<coverage") {
		return "cobertura"
	}

	return "go" // default fallback
}
