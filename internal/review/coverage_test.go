package review

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanCoverageFile(t *testing.T) {
	tmpDir := t.TempDir()
	covFile := filepath.Join(tmpDir, "coverage.out")

	// Write profile with null bytes, empty lines, and some invalid lines
	content := "mode: set\n" +
		"github.com/borch-ai/powerword/internal/review/validator.go:37.52,41.2 3 1\n" +
		"\x00\x00\n" +
		"invalid line format\n" +
		"github.com/borch-ai/powerword/internal/review/validator.go:81.1,82.2 1 0\n"

	if err := os.WriteFile(covFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test cover file: %v", err)
	}

	if err := cleanCoverageFile(covFile); err != nil {
		t.Fatalf("cleanCoverageFile failed: %v", err)
	}

	//nolint:gosec // testing context is safe
	cleaned, err := os.ReadFile(covFile)
	if err != nil {
		t.Fatalf("failed to read cleaned file: %v", err)
	}

	expected := "mode: set\n" +
		"github.com/borch-ai/powerword/internal/review/validator.go:37.52,41.2 3 1\n" +
		"github.com/borch-ai/powerword/internal/review/validator.go:81.1,82.2 1 0\n"

	if string(cleaned) != expected {
		t.Errorf("expected cleaned content:\n%q\ngot:\n%q", expected, string(cleaned))
	}
}

func TestParseCoveragePercent(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expectCov   float64
		expectError bool
	}{
		{
			name: "valid single line total",
			output: "github.com/borch-ai/powerword/internal/review/validator.go:37:\tValidatePlans\t100.0%\n" +
				"total:\t\t\t\t\t\t\t\t(statements)\t\t100.0%\n",
			expectCov:   100.0,
			expectError: false,
		},
		{
			name: "valid total with spaces",
			output: "github.com/borch-ai/powerword/internal/review/validator.go:37: ValidatePlans 50.0%\n" +
				"total: (statements) 91.1%\n",
			expectCov:   91.1,
			expectError: false,
		},
		{
			name:        "missing total line",
			output:      "github.com/borch-ai/powerword/internal/review/validator.go:37: ValidatePlans 50.0%\n",
			expectCov:   0,
			expectError: true,
		},
		{
			name:        "empty output",
			output:      "",
			expectCov:   0,
			expectError: true,
		},
		{
			name: "invalid float total",
			output: "github.com/borch-ai/powerword/internal/review/validator.go:37: ValidatePlans 50.0%\n" +
				"total: (statements) ABC%\n",
			expectCov:   0,
			expectError: true,
		},
		{
			name: "unexpected columns total",
			output: "github.com/borch-ai/powerword/internal/review/validator.go:37: ValidatePlans 50.0%\n" +
				"total: 91.1%\n",
			expectCov:   0,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cov, err := parseCoveragePercent(tc.output)
			if tc.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if cov != tc.expectCov {
				t.Errorf("expected coverage %f, got %f", tc.expectCov, cov)
			}
		})
	}
}

func TestVerifyCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	covFile := filepath.Join(tmpDir, "coverage.out")

	// Use real validator.go statements
	content := "mode: set\n" +
		"github.com/borch-ai/powerword/internal/review/validator.go:37.52,41.2 3 1\n"

	if err := os.WriteFile(covFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write cover file: %v", err)
	}

	ctx := context.Background()

	// 100% actual statement coverage in the mock profile.
	// Test passing threshold
	if err := VerifyCoverage(ctx, 90.0, covFile); err != nil {
		t.Errorf("expected VerifyCoverage to pass at 90%%, got: %v", err)
	}

	// Test failing threshold
	if err := VerifyCoverage(ctx, 101.0, covFile); err == nil {
		t.Error("expected VerifyCoverage to fail at 101%%, got nil error")
	} else if !strings.Contains(err.Error(), "below required threshold") {
		t.Errorf("expected threshold error message, got: %v", err)
	}

	// Test missing file error
	if err := VerifyCoverage(ctx, 50.0, filepath.Join(tmpDir, "non_existent.out")); err == nil {
		t.Error("expected error for non-existent coverage file, got nil")
	}
}
