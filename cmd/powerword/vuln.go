package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// allowedVulns defines audited, accepted upstream vulnerabilities that have no upstream fix
// and are mitigated within our codebase.
var allowedVulns = map[string]string{
	"GO-2026-5781": "rsc.io/pdf: uncatchable stack overflow on recursive dictionaries; mitigated by newSafeReaderAt in codebase",
}

const govulncheckVersion = "v1.3.0"

var vulnIDPattern = regexp.MustCompile(`Vulnerability #\d+:\s+(GO-\d{4}-\d+)`)

type vulnScannerFunc func(ctx context.Context) (output string, exitErr error)

var (
	defaultScanner vulnScannerFunc                   = runGovulncheck
	lookPath       func(file string) (string, error) = exec.LookPath
)

func newVulnCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vuln",
		Short: "Run govulncheck and enforce security vulnerability policy with audited exemptions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return executeVulnCommand(ctx, cmd, defaultScanner)
		},
	}
	return cmd
}

func parseVulnerabilities(output string) []string {
	matches := vulnIDPattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var ids []string
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	sort.Strings(ids)
	return ids
}

type evalResult struct {
	exempted   []string
	unexempted []string
}

func evaluateVulnerabilities(vulnIDs []string) evalResult {
	var res evalResult
	for _, id := range vulnIDs {
		if _, ok := allowedVulns[id]; ok {
			res.exempted = append(res.exempted, id)
		} else {
			res.unexempted = append(res.unexempted, id)
		}
	}
	return res
}

func isVulnExitError(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == 3
	}
	return false
}

func executeVulnCommand(ctx context.Context, cmd *cobra.Command, scanner vulnScannerFunc) error {
	output, err := scanner(ctx)
	if err != nil && output == "" {
		return fmt.Errorf("failed to run govulncheck: %w", err)
	}

	vulnIDs := parseVulnerabilities(output)
	if len(vulnIDs) == 0 {
		if err != nil {
			cmd.PrintErrln(output)
			return fmt.Errorf("govulncheck failed: %w", err)
		}
		cmd.Println("No vulnerabilities detected.")
		return nil
	}

	eval := evaluateVulnerabilities(vulnIDs)
	if len(eval.unexempted) > 0 {
		cmd.PrintErrln(output)
		return fmt.Errorf("unexempted vulnerabilities detected: %s", strings.Join(eval.unexempted, ", "))
	}

	// If vulnerabilities were reported but all were exempted, verify the scanner didn't fail with a non-vuln error.
	if err != nil && !isVulnExitError(err) {
		cmd.PrintErrln(output)
		return fmt.Errorf("govulncheck encountered non-vuln failure: %w", err)
	}

	for _, id := range eval.exempted {
		cmd.Printf("Notice: Accepted exemption for %s (%s)\n", id, allowedVulns[id])
	}
	cmd.Println("All detected vulnerabilities are covered by audited exemptions.")
	return nil
}

func getGoEnv(ctx context.Context, key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	//nolint:gosec // G204: key is trusted Go toolchain environment variable name
	out, err := exec.CommandContext(ctx, "go", "env", key).Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func ensureGovulncheck(ctx context.Context) (string, error) {
	if binPath, err := lookPath("govulncheck"); err == nil {
		return binPath, nil
	}

	gobin := getGoEnv(ctx, "GOBIN")
	if gobin == "" {
		gopath := getGoEnv(ctx, "GOPATH")
		if gopath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("cannot determine user home dir: %w", err)
			}
			gopath = filepath.Join(home, "go")
		}
		gobin = filepath.Join(gopath, "bin")
	}

	binPath := filepath.Clean(filepath.Join(gobin, "govulncheck"))
	if p, err := lookPath(binPath); err == nil {
		return p, nil
	}

	installTarget := "golang.org/x/vuln/cmd/govulncheck@" + govulncheckVersion
	cmd := exec.CommandContext(ctx, "go", "install", installTarget)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to install %s: %w", installTarget, err)
	}

	if p, err := lookPath("govulncheck"); err == nil {
		return p, nil
	}
	if p, err := lookPath(binPath); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("installed %s successfully but binary could not be found via PATH or at %s", installTarget, binPath)
}

func runGovulncheck(ctx context.Context) (string, error) {
	binPath, err := ensureGovulncheck(ctx)
	if err != nil {
		return "", err
	}

	//nolint:gosec // G204: binPath is resolved via LookPath or verified GOBIN/govulncheck installation
	cmd := exec.CommandContext(ctx, binPath, "./...")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()
	return buf.String(), runErr
}
