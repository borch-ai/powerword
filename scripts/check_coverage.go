package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: <command-output> | go run check_coverage.go <threshold_percent>")
		os.Exit(1)
	}

	thresholdStr := os.Args[1]
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		fmt.Printf("Error: invalid threshold percentage '%s': %v\n", thresholdStr, err)
		os.Exit(1)
	}

	var totalLine string
	scanner := bufio.NewScanner(os.Stdin)
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
