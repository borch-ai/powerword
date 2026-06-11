package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

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

	// Line pattern: mode: set OR package/file.go:line.col,line.col numStmt count
	validLinePattern := regexp.MustCompile(`^(mode: \w+|\S+\.go:\d+\.\d+,\d+\.\d+ \d+ \d+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if validLinePattern.MatchString(line) {
			cleanLines = append(cleanLines, line)
		}
	}
	err = scanner.Err()
	if err != nil {
		return err
	}

	// Write cleaned lines back
	outData := []byte(strings.Join(cleanLines, "\n") + "\n")
	//nolint:gosec // script runs in controlled local development/testing environment
	return os.WriteFile(path, outData, 0600)
}

func readFromFile(ctx context.Context, covFile string) (string, error) {
	// Clean the coverage file first
	if err := cleanCoverageFile(covFile); err != nil {
		return "", fmt.Errorf("error cleaning coverage file: %w", err)
	}

	// Run go tool cover
	//nolint:gosec // subcommand execution in test/build validator context is safe
	cmd := exec.CommandContext(ctx, "go", "tool", "cover", "-func="+covFile)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error running go tool cover: %w\nStderr: %s", err, stderr.String())
	}
	return out.String(), nil
}

func readFromStdin() (string, error) {
	var buf bytes.Buffer
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		buf.WriteString(scanner.Text() + "\n")
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: [command-output |] go run check_coverage.go <threshold_percent> [coverage.out]")
		os.Exit(1)
	}

	thresholdStr := os.Args[1]
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		fmt.Printf("Error: invalid threshold percentage '%s': %v\n", thresholdStr, err)
		os.Exit(1)
	}

	var coverageOutput string
	if len(os.Args) >= 3 {
		covFile := os.Args[2]
		var fileErr error
		coverageOutput, fileErr = readFromFile(context.Background(), covFile)
		if fileErr != nil {
			fmt.Println(fileErr)
			os.Exit(1)
		}
	} else {
		var stdinErr error
		coverageOutput, stdinErr = readFromStdin()
		if stdinErr != nil {
			fmt.Printf("Error reading stdin: %v\n", stdinErr)
			os.Exit(1)
		}
	}

	var totalLine string
	scanner := bufio.NewScanner(strings.NewReader(coverageOutput))
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line) // Echo the coverage lines to stdout
		if strings.HasPrefix(line, "total:") {
			totalLine = line
		}
	}

	if totalLine == "" {
		fmt.Println("Error: 'total:' line not found in coverage output")
		os.Exit(1)
	}

	// Expecting: "total: (statements) X.Y%"
	parts := strings.Fields(totalLine)
	if len(parts) < 3 {
		fmt.Printf("Error: unexpected total line format: %s\n", totalLine)
		os.Exit(1)
	}

	pctStr := strings.TrimSuffix(parts[2], "%")
	coverage, err := strconv.ParseFloat(pctStr, 64)
	if err != nil {
		fmt.Printf("Error: failed to parse coverage float '%s': %v\n", pctStr, err)
		os.Exit(1)
	}

	if coverage < threshold {
		fmt.Printf("FAIL: Total coverage (%.2f%%) is below required threshold (%.2f%%)\n", coverage, threshold)
		os.Exit(1)
	}

	fmt.Printf("PASS: Total coverage (%.2f%%) meets or exceeds threshold (%.2f%%)\n", coverage, threshold)
}
