package review

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	validLinePattern = regexp.MustCompile(`^(mode: \w+|\S+\.go:\d+\.\d+,\d+\.\d+ \d+ \d+)$`)
)

// VerifyCoverage cleans the target coverage profile, runs go tool cover -func,
// parses the output to get the total coverage percentage, and asserts it meets the threshold.
func VerifyCoverage(ctx context.Context, threshold float64, profilePath string) error {
	if err := cleanCoverageFile(profilePath); err != nil {
		return fmt.Errorf("failed to clean coverage file %s: %w", profilePath, err)
	}

	// Run go tool cover
	//nolint:gosec // subcommand execution in test/build validator context is safe
	cmd := execCommand(ctx, "go", "tool", "cover", "-func="+profilePath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running go tool cover: %w\nStderr: %s", err, stderr.String())
	}

	coverage, err := parseCoveragePercent(stdout.String())
	if err != nil {
		return err
	}

	if coverage < threshold {
		return fmt.Errorf("FAIL: Total coverage (%.2f%%) is below required threshold (%.2f%%)", coverage, threshold)
	}

	fmt.Printf("PASS: Total coverage (%.2f%%) meets or exceeds threshold (%.2f%%)\n", coverage, threshold)
	return nil
}

func cleanCoverageFile(path string) error {
	//nolint:gosec // script runs in controlled local development/testing environment
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Remove null bytes
	data = bytes.ReplaceAll(data, []byte{0}, []byte{})

	// Filter valid coverage lines
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var cleanLines []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if validLinePattern.MatchString(line) {
			cleanLines = append(cleanLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Write cleaned lines back
	outData := []byte(strings.Join(cleanLines, "\n") + "\n")
	//nolint:gosec // script runs in controlled local development/testing environment
	return os.WriteFile(path, outData, 0600)
}

func parseCoveragePercent(output string) (float64, error) {
	var totalLine string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line) // Echo coverage lines to stdout
		if strings.HasPrefix(line, "total:") {
			totalLine = line
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	if totalLine == "" {
		return 0, fmt.Errorf("error: 'total:' line not found in coverage output")
	}

	// Expecting: "total: (statements) X.Y%"
	parts := strings.Fields(totalLine)
	if len(parts) < 3 {
		return 0, fmt.Errorf("error: unexpected total line format: %s", totalLine)
	}

	pctStr := strings.TrimSuffix(parts[len(parts)-1], "%")
	coverage, err := strconv.ParseFloat(pctStr, 64)
	if err != nil {
		return 0, fmt.Errorf("error: failed to parse coverage float %q: %w", pctStr, err)
	}

	return coverage, nil
}
