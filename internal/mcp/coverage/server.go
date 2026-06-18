package coverage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const checkCoverageSchema = `{
	"type": "object",
	"properties": {
		"threshold": {
			"type": "number",
			"description": "The minimum coverage percentage required (e.g., 85.0)."
		},
		"profile_path": {
			"type": "string",
			"description": "The path to the coverage profile file."
		},
		"format": {
			"type": "string",
			"enum": ["lcov", "cobertura", "go", "auto"],
			"description": "Optional coverage format (lcov, cobertura, go, auto). Defaults to auto."
		}
	},
	"required": ["threshold", "profile_path"]
}`

// SetupServer creates and configures the coverage MCP server.
func SetupServer() (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-coverage",
		Version: "1.0.0",
	}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "check_coverage",
		Description: "Parses coverage reports (Go cover, LCOV, Cobertura XML) and checks against a threshold.",
		InputSchema: json.RawMessage(checkCoverageSchema),
	}, handleCheckCoverage())

	return srv, nil
}

func handleCheckCoverage() func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Threshold   float64 `json:"threshold"`
			ProfilePath string  `json:"profile_path"`
			Format      string  `json:"format"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.ProfilePath == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "profile_path is required"}},
			}, nil
		}

		// Read the file
		//nolint:gosec // profile path is provided as parameter for this coverage tool
		data, err := os.ReadFile(args.ProfilePath)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to read profile path: %v", err)}},
			}, nil
		}

		// Detect format
		fmtName := args.Format
		if fmtName == "" || fmtName == "auto" {
			fmtName = detectFormat(args.ProfilePath, data)
		}

		var pct float64
		var parseErr error

		switch strings.ToLower(fmtName) {
		case "go":
			pct, parseErr = parseGoCoverage(data)
		case "lcov":
			pct, parseErr = parseLCOV(data)
		case "cobertura":
			pct, parseErr = parseCobertura(data)
		default:
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("unsupported format: %s", fmtName)}},
			}, nil
		}

		if parseErr != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to parse coverage file (format: %s): %v", fmtName, parseErr)}},
			}, nil
		}

		if pct < args.Threshold {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{
					Text: fmt.Sprintf("FAIL: Total coverage (%.2f%%) is below required threshold (%.2f%%)", pct, args.Threshold),
				}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf("PASS: Total coverage (%.2f%%) meets or exceeds threshold (%.2f%%)", pct, args.Threshold),
			}},
		}, nil
	}
}

func detectFormat(path string, data []byte) string {
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

	// inspect up to first 4096 bytes
	limit := len(data)
	if limit > 4096 {
		limit = 4096
	}
	content := string(data[:limit])

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

	// Legacy/default fallback
	return "go"
}

func parseGoCoverage(data []byte) (float64, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var totalStatements int64
	var coveredStatements int64
	hasLines := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		hasLines = true
		parts := strings.Fields(line)
		if len(parts) < 3 {
			return 0, fmt.Errorf("invalid go coverage line: %s", line)
		}
		numStmt, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid statement count in line %q: %w", line, err)
		}
		count, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid execution count in line %q: %w", line, err)
		}
		totalStatements += numStmt
		if count > 0 {
			coveredStatements += numStmt
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if !hasLines || totalStatements == 0 {
		return 0, fmt.Errorf("no coverage statements found")
	}
	return float64(coveredStatements) / float64(totalStatements) * 100.0, nil
}

func parseLCOV(data []byte) (float64, error) {
	pct, hasSummary, err := parseLCOVSummary(data)
	if err != nil {
		return 0, err
	}
	if hasSummary {
		return pct, nil
	}
	return parseLCOVLines(data)
}

func parseLCOVSummary(data []byte) (float64, bool, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var totalLines int64
	var coveredLines int64
	hasLF := false
	hasLH := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "LF:") {
			lfVal, err := strconv.ParseInt(strings.TrimPrefix(line, "LF:"), 10, 64)
			if err != nil {
				return 0, false, fmt.Errorf("invalid LF value %q: %w", line, err)
			}
			totalLines += lfVal
			hasLF = true
		} else if strings.HasPrefix(line, "LH:") {
			lhVal, err := strconv.ParseInt(strings.TrimPrefix(line, "LH:"), 10, 64)
			if err != nil {
				return 0, false, fmt.Errorf("invalid LH value %q: %w", line, err)
			}
			coveredLines += lhVal
			hasLH = true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, false, err
	}

	if hasLF && hasLH && totalLines > 0 {
		return float64(coveredLines) / float64(totalLines) * 100.0, true, nil
	}
	return 0, false, nil
}

func parseLCOVLines(data []byte) (float64, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var totalLines int64
	var coveredLines int64
	hasDA := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "DA:") {
			parts := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			if len(parts) < 2 {
				continue
			}
			count, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				continue
			}
			totalLines++
			if count > 0 {
				coveredLines++
			}
			hasDA = true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if !hasDA || totalLines == 0 {
		return 0, fmt.Errorf("no LCOV coverage data found in profile")
	}
	return float64(coveredLines) / float64(totalLines) * 100.0, nil
}

func parseCobertura(data []byte) (float64, error) {
	var cov struct {
		XMLName      xml.Name `xml:"coverage"`
		LineRate     string   `xml:"line-rate,attr"`
		LinesValid   string   `xml:"lines-valid,attr"`
		LinesCovered string   `xml:"lines-covered,attr"`
	}
	if err := xml.Unmarshal(data, &cov); err != nil {
		return 0, fmt.Errorf("failed to parse Cobertura XML: %w", err)
	}

	if cov.LinesValid != "" && cov.LinesCovered != "" {
		valid, err1 := strconv.ParseInt(cov.LinesValid, 10, 64)
		covered, err2 := strconv.ParseInt(cov.LinesCovered, 10, 64)
		if err1 == nil && err2 == nil && valid > 0 {
			return float64(covered) / float64(valid) * 100.0, nil
		}
	}

	if cov.LineRate != "" {
		rate, err := strconv.ParseFloat(cov.LineRate, 64)
		if err == nil {
			if rate > 1.0 {
				return rate, nil
			}
			return rate * 100.0, nil
		}
	}

	return 0, fmt.Errorf("could not find valid coverage metrics in Cobertura XML")
}
