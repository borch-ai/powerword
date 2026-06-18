package coverage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestParseGoCoverage(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectPct float64
		expectErr bool
	}{
		{
			name: "100 percent coverage",
			content: `mode: set
github.com/borch-ai/powerword/pkg/linter/validator.go:37.52,41.2 3 1
github.com/borch-ai/powerword/pkg/linter/validator.go:45.10,48.2 2 4`,
			expectPct: 100.0,
			expectErr: false,
		},
		{
			name: "50 percent coverage",
			content: `mode: count
github.com/borch-ai/powerword/pkg/linter/validator.go:37.52,41.2 3 1
github.com/borch-ai/powerword/pkg/linter/validator.go:45.10,48.2 3 0`,
			expectPct: 50.0,
			expectErr: false,
		},
		{
			name:      "empty profile",
			content:   "",
			expectPct: 0,
			expectErr: true,
		},
		{
			name: "invalid line",
			content: `mode: set
github.com/borch-ai/powerword/pkg/linter/validator.go:37.52,41.2 3`,
			expectPct: 0,
			expectErr: true,
		},
		{
			name: "invalid statement count",
			content: `mode: set
github.com/borch-ai/powerword/pkg/linter/validator.go:37.52,41.2 abc 1`,
			expectPct: 0,
			expectErr: true,
		},
		{
			name: "invalid execution count",
			content: `mode: set
github.com/borch-ai/powerword/pkg/linter/validator.go:37.52,41.2 3 abc`,
			expectPct: 0,
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pct, err := parseGoCoverage([]byte(tc.content))
			if tc.expectErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pct != tc.expectPct {
				t.Errorf("expected %.2f%% coverage, got %.2f%%", tc.expectPct, pct)
			}
		})
	}
}

func TestParseLCOV(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectPct float64
		expectErr bool
	}{
		{
			name: "standard with LF and LH",
			content: `TN:
SF:src/components/Button.ts
DA:1,3
DA:2,0
LF:2
LH:1
end_of_record
SF:src/components/Input.ts
DA:5,10
LF:1
LH:1
end_of_record`,
			expectPct: 66.66666666666666,
			expectErr: false,
		},
		{
			name: "DA lines fallback only",
			content: `SF:src/components/Button.ts
DA:1,3
DA:2,0
DA:3,1
end_of_record`,
			expectPct: 66.66666666666666,
			expectErr: false,
		},
		{
			name:      "invalid LCOV",
			content:   "random string text",
			expectPct: 0,
			expectErr: true,
		},
		{
			name: "invalid LF value",
			content: `SF:src/components/Button.ts
LF:abc
LH:1
end_of_record`,
			expectPct: 0,
			expectErr: true,
		},
		{
			name: "invalid LH value",
			content: `SF:src/components/Button.ts
LF:10
LH:abc
end_of_record`,
			expectPct: 0,
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pct, err := parseLCOV([]byte(tc.content))
			if tc.expectErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if MathAbs(pct-tc.expectPct) > 0.001 {
				t.Errorf("expected %.2f%% coverage, got %.2f%%", tc.expectPct, pct)
			}
		})
	}
}

func TestParseCobertura(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectPct float64
		expectErr bool
	}{
		{
			name: "standard lines-valid lines-covered",
			content: `<?xml version="1.0" ?>
<coverage lines-valid="200" lines-covered="150" line-rate="0.75" branch-rate="0.5">
  <sources><source>/src</source></sources>
</coverage>`,
			expectPct: 75.0,
			expectErr: false,
		},
		{
			name: "fallback line-rate attribute",
			content: `<?xml version="1.0" ?>
<coverage line-rate="0.85" branch-rate="0.5">
</coverage>`,
			expectPct: 85.0,
			expectErr: false,
		},
		{
			name: "line-rate as percentage",
			content: `<?xml version="1.0" ?>
<coverage line-rate="92.5">
</coverage>`,
			expectPct: 92.5,
			expectErr: false,
		},
		{
			name:      "invalid Cobertura xml",
			content:   "<coverage><invalid",
			expectPct: 0,
			expectErr: true,
		},
		{
			name: "invalid metrics",
			content: `<?xml version="1.0" ?>
<coverage branch-rate="0.5">
</coverage>`,
			expectPct: 0,
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pct, err := parseCobertura([]byte(tc.content))
			if tc.expectErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pct != tc.expectPct {
				t.Errorf("expected %.2f%% coverage, got %.2f%%", tc.expectPct, pct)
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		content      string
		expectFormat string
	}{
		{
			name:         "lcov extension",
			path:         "coverage/lcov.info",
			content:      "irrelevant",
			expectFormat: "lcov",
		},
		{
			name:         "cobertura xml extension",
			path:         "reports/coverage.xml",
			content:      "irrelevant",
			expectFormat: "cobertura",
		},
		{
			name:         "go out extension",
			path:         "cov.out",
			content:      "irrelevant",
			expectFormat: "go",
		},
		{
			name:         "go content mode",
			path:         "undetermined_filename",
			content:      "mode: count\nfoo.go:1.2,3.4 1 1",
			expectFormat: "go",
		},
		{
			name:         "lcov content SF",
			path:         "undetermined_filename",
			content:      "TN:\nSF:main.go\nLH:0\nLF:0",
			expectFormat: "lcov",
		},
		{
			name:         "cobertura content XML",
			path:         "undetermined_filename",
			content:      "<?xml version=\"1.0\"?>\n<coverage line-rate=\"1.0\">",
			expectFormat: "cobertura",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			format := detectFormat(tc.path, []byte(tc.content))
			if format != tc.expectFormat {
				t.Errorf("expected format %s, got %s", tc.expectFormat, format)
			}
		})
	}
}

func TestCheckCoverageTool_Success(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	tmpDir := t.TempDir()
	lcovPath := filepath.Join(tmpDir, "lcov.info")
	lcovContent := "SF:Button.ts\nLF:10\nLH:9\nend_of_record\n"
	if writeErr := os.WriteFile(lcovPath, []byte(lcovContent), 0600); writeErr != nil {
		t.Fatalf("failed to write lcov file: %v", writeErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, t1)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, connErr := client.Connect(ctx, t2, nil)
	if connErr != nil {
		t.Fatalf("failed to connect client: %v", connErr)
	}
	defer func() { _ = session.Close() }()

	// Success case: 90% coverage meets 85% threshold
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_coverage",
		Arguments: map[string]interface{}{
			"threshold":    85.0,
			"profile_path": lcovPath,
			"format":       "auto",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if res.IsError {
		t.Errorf("expected success tool call, got error: %v", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected content in response")
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "PASS") {
		t.Errorf("expected output to contain 'PASS', got: %s", text)
	}
}

func TestCheckCoverageTool_Failures_Threshold(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	tmpDir := t.TempDir()
	lcovPath := filepath.Join(tmpDir, "lcov.info")
	lcovContent := "SF:Button.ts\nLF:10\nLH:9\nend_of_record\n"
	if writeErr := os.WriteFile(lcovPath, []byte(lcovContent), 0600); writeErr != nil {
		t.Fatalf("failed to write lcov file: %v", writeErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, t1)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, connErr := client.Connect(ctx, t2, nil)
	if connErr != nil {
		t.Fatalf("failed to connect client: %v", connErr)
	}
	defer func() { _ = session.Close() }()

	// Failure case: 90% coverage below 95% threshold
	resFail, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_coverage",
		Arguments: map[string]interface{}{
			"threshold":    95.0,
			"profile_path": lcovPath,
			"format":       "auto",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !resFail.IsError {
		t.Errorf("expected error tool call for unmet threshold, got success")
	}
	textFail := resFail.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(textFail, "FAIL") {
		t.Errorf("expected output to contain 'FAIL', got: %s", textFail)
	}
}

func TestCheckCoverageTool_Failures_Errors(t *testing.T) {
	srv, err := SetupServer()
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	tmpDir := t.TempDir()
	lcovPath := filepath.Join(tmpDir, "lcov.info")
	lcovContent := "SF:Button.ts\nLF:10\nLH:9\nend_of_record\n"
	if writeErr := os.WriteFile(lcovPath, []byte(lcovContent), 0600); writeErr != nil {
		t.Fatalf("failed to write lcov file: %v", writeErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, t1)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, connErr := client.Connect(ctx, t2, nil)
	if connErr != nil {
		t.Fatalf("failed to connect client: %v", connErr)
	}
	defer func() { _ = session.Close() }()

	// 1. Error case: empty profile_path
	resErrEmpty, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_coverage",
		Arguments: map[string]interface{}{
			"threshold":    85.0,
			"profile_path": "",
			"format":       "auto",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !resErrEmpty.IsError {
		t.Error("expected error for empty profile_path")
	}

	// 2. Error case: non-existent file
	resErrMissing, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_coverage",
		Arguments: map[string]interface{}{
			"threshold":    85.0,
			"profile_path": filepath.Join(tmpDir, "non_existent.info"),
			"format":       "auto",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !resErrMissing.IsError {
		t.Error("expected error for non-existent file")
	}

	// 3. Error case: unsupported format
	resErrFormat, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "check_coverage",
		Arguments: map[string]interface{}{
			"threshold":    85.0,
			"profile_path": lcovPath,
			"format":       "unsupported_fmt",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !resErrFormat.IsError {
		t.Error("expected error for unsupported format")
	}

	// 4. Error case: parsing fails (e.g. malformed LCOV data)
	badLcovPath := filepath.Join(tmpDir, "bad.info")
	if writeErr := os.WriteFile(badLcovPath, []byte("invalid data"), 0600); writeErr == nil {
		resErrParse, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "check_coverage",
			Arguments: map[string]interface{}{
				"threshold":    85.0,
				"profile_path": badLcovPath,
				"format":       "lcov",
			},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if !resErrParse.IsError {
			t.Error("expected error for malformed file parsing")
		}
	}
}

func MathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
