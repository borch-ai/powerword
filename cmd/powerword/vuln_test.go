package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseVulnerabilities(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []string
	}{
		{
			name:     "empty output",
			output:   "",
			expected: nil,
		},
		{
			name:     "no vulnerabilities",
			output:   "No vulnerabilities found in your code.",
			expected: nil,
		},
		{
			name: "single vulnerability",
			output: `=== Symbol Results ===

Vulnerability #1: GO-2026-5781
    Uncatchable stack-overflow denial of service in rsc.io/pdf`,
			expected: []string{"GO-2026-5781"},
		},
		{
			name: "multiple vulnerabilities with duplicates",
			output: `=== Symbol Results ===

Vulnerability #1: GO-2026-5781
    Uncatchable stack-overflow denial of service in rsc.io/pdf

Vulnerability #2: GO-2026-6218
    Avoid quadratic complexity in resolvePath in net/url

Vulnerability #3: GO-2026-5781
    Repeated trace for rsc.io/pdf`,
			expected: []string{"GO-2026-5781", "GO-2026-6218"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVulnerabilities(tc.output)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestEvaluateVulnerabilities(t *testing.T) {
	tests := []struct {
		name           string
		input          []string
		wantExempted   []string
		wantUnexempted []string
	}{
		{
			name:           "empty slice",
			input:          nil,
			wantExempted:   nil,
			wantUnexempted: nil,
		},
		{
			name:           "only exempted",
			input:          []string{"GO-2026-5781"},
			wantExempted:   []string{"GO-2026-5781"},
			wantUnexempted: nil,
		},
		{
			name:           "only unexempted",
			input:          []string{"GO-2026-9999"},
			wantExempted:   nil,
			wantUnexempted: []string{"GO-2026-9999"},
		},
		{
			name:           "mixed",
			input:          []string{"GO-2026-5781", "GO-2026-9999"},
			wantExempted:   []string{"GO-2026-5781"},
			wantUnexempted: []string{"GO-2026-9999"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := evaluateVulnerabilities(tc.input)
			if !reflect.DeepEqual(res.exempted, tc.wantExempted) {
				t.Errorf("exempted mismatch: expected %v, got %v", tc.wantExempted, res.exempted)
			}
			if !reflect.DeepEqual(res.unexempted, tc.wantUnexempted) {
				t.Errorf("unexempted mismatch: expected %v, got %v", tc.wantUnexempted, res.unexempted)
			}
		})
	}
}

func TestExecuteVulnCommand_Success(t *testing.T) {
	ctx := context.Background()

	t.Run("no vulnerabilities found with exit 0", func(t *testing.T) {
		cmd := newVulnCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		scanner := func(ctx context.Context) (string, error) {
			return "No vulnerabilities found.", nil
		}
		err := executeVulnCommand(ctx, cmd, scanner)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !strings.Contains(outBuf.String(), "No vulnerabilities detected.") {
			t.Fatalf("expected 'No vulnerabilities detected.', got %q", outBuf.String())
		}
	})

	t.Run("only exempted vulnerabilities detected with clean exit", func(t *testing.T) {
		cmd := newVulnCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		scanner := func(ctx context.Context) (string, error) {
			return "Vulnerability #1: GO-2026-5781\nDetails...", nil
		}
		err := executeVulnCommand(ctx, cmd, scanner)
		if err != nil {
			t.Fatalf("expected nil error for exempted vuln, got %v", err)
		}
		output := outBuf.String()
		if !strings.Contains(output, "Accepted exemption for GO-2026-5781") {
			t.Fatalf("expected exemption notice, got %q", output)
		}
		if !strings.Contains(output, "All detected vulnerabilities are covered by audited exemptions.") {
			t.Fatalf("expected all covered message, got %q", output)
		}
	})

	t.Run("only exempted vulnerabilities detected with exit code 3", func(t *testing.T) {
		cmd := newVulnCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		exit3Err := exec.CommandContext(ctx, "sh", "-c", "exit 3").Run()
		scanner := func(ctx context.Context) (string, error) {
			return "Vulnerability #1: GO-2026-5781\nDetails...", exit3Err
		}
		err := executeVulnCommand(ctx, cmd, scanner)
		if err != nil {
			t.Fatalf("expected nil error for exempted vuln with exit code 3, got %v", err)
		}
		output := outBuf.String()
		if !strings.Contains(output, "Accepted exemption for GO-2026-5781") {
			t.Fatalf("expected exemption notice, got %q", output)
		}
		if !strings.Contains(output, "All detected vulnerabilities are covered by audited exemptions.") {
			t.Fatalf("expected all covered message, got %q", output)
		}
	})
}

func TestIsVulnExitError(t *testing.T) {
	ctx := context.Background()

	if isVulnExitError(nil) {
		t.Error("expected false for nil error")
	}

	if isVulnExitError(errors.New("generic error")) {
		t.Error("expected false for non-exit error")
	}

	exit1Err := exec.CommandContext(ctx, "sh", "-c", "exit 1").Run()
	if isVulnExitError(exit1Err) {
		t.Error("expected false for exit code 1")
	}

	exit3Err := exec.CommandContext(ctx, "sh", "-c", "exit 3").Run()
	if !isVulnExitError(exit3Err) {
		t.Error("expected true for exit code 3")
	}
}

func TestExecuteVulnCommand_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("scanner execution failure with no output", func(t *testing.T) {
		cmd := newVulnCmd()
		scanner := func(ctx context.Context) (string, error) {
			return "", errors.New("command not found")
		}
		err := executeVulnCommand(ctx, cmd, scanner)
		if err == nil || !strings.Contains(err.Error(), "failed to run govulncheck") {
			t.Fatalf("expected failed to run govulncheck error, got %v", err)
		}
	})

	t.Run("no vulnerabilities parsed but scanner returned error", func(t *testing.T) {
		cmd := newVulnCmd()
		scanner := func(ctx context.Context) (string, error) {
			return "syntax error in package", errors.New("exit status 1")
		}
		err := executeVulnCommand(ctx, cmd, scanner)
		if err == nil || !strings.Contains(err.Error(), "govulncheck failed") {
			t.Fatalf("expected govulncheck failed error, got %v", err)
		}
	})

	t.Run("exempted vulnerabilities detected but scanner returned non-vuln failure", func(t *testing.T) {
		cmd := newVulnCmd()
		var errBuf bytes.Buffer
		cmd.SetErr(&errBuf)
		scanner := func(ctx context.Context) (string, error) {
			return "Vulnerability #1: GO-2026-5781\nCrash occurred...", errors.New("exit status 1")
		}
		err := executeVulnCommand(ctx, cmd, scanner)
		if err == nil {
			t.Fatal("expected error when scanner fails with non-vuln exit, got nil")
		}
		if !strings.Contains(err.Error(), "govulncheck encountered non-vuln failure") {
			t.Fatalf("expected non-vuln failure error, got %v", err)
		}
	})

	t.Run("unexempted vulnerabilities detected", func(t *testing.T) {
		cmd := newVulnCmd()
		var errBuf bytes.Buffer
		cmd.SetErr(&errBuf)
		scanner := func(ctx context.Context) (string, error) {
			return "Vulnerability #1: GO-2026-9999\nUnfixable something", errors.New("exit status 3")
		}
		err := executeVulnCommand(ctx, cmd, scanner)
		if err == nil {
			t.Fatal("expected error for unexempted vuln, got nil")
		}
		if !strings.Contains(err.Error(), "unexempted vulnerabilities detected: GO-2026-9999") {
			t.Fatalf("expected unexempted vulnerabilities error, got %v", err)
		}
	})
}

func TestNewVulnCmdIntegration(t *testing.T) {
	origScanner := defaultScanner
	defer func() { defaultScanner = origScanner }()

	defaultScanner = func(ctx context.Context) (string, error) {
		return "No vulnerabilities found.", nil
	}

	cmd := newVulnCmd()
	cmd.SetArgs([]string{})
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing vuln command: %v", err)
	}
	if !strings.Contains(outBuf.String(), "No vulnerabilities detected.") {
		t.Fatalf("expected success message, got %q", outBuf.String())
	}
}

func TestEnsureGovulncheckPath_FoundAtBinPath(t *testing.T) {
	ctx := context.Background()

	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	t.Setenv("GOBIN", "/fake/bin")
	lookPath = func(file string) (string, error) {
		return "/fake/bin/govulncheck", nil
	}
	found, err := ensureGovulncheck(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != "/fake/bin/govulncheck" {
		t.Fatalf("expected /fake/bin/govulncheck, got %s", found)
	}
}

func TestEnsureGovulncheckPath_FoundOnPATHWhenGOBINNotFound(t *testing.T) {
	ctx := context.Background()

	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	t.Setenv("GOBIN", "/empty/gobin")
	lookPath = func(file string) (string, error) {
		if file == "/empty/gobin/govulncheck" {
			return "", exec.ErrNotFound
		}
		if file == "govulncheck" {
			return "/system/bin/govulncheck", nil
		}
		return "", exec.ErrNotFound
	}

	found, err := ensureGovulncheck(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != "/system/bin/govulncheck" {
		t.Fatalf("expected /system/bin/govulncheck, got %s", found)
	}
}

func TestEnsureGovulncheckPath_PrefersGOBIN(t *testing.T) {
	ctx := context.Background()

	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	tmpDir := t.TempDir()
	t.Setenv("GOBIN", tmpDir)
	fakeBin := filepath.Join(tmpDir, "govulncheck")
	//nolint:gosec // G302: test helper script must be executable for lookPath validation
	if wErr := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0"), 0700); wErr != nil {
		t.Fatalf("failed to create fake bin: %v", wErr)
	}

	lookPath = func(file string) (string, error) {
		if file == fakeBin {
			return fakeBin, nil
		}
		if file == "govulncheck" {
			return "/system/bin/govulncheck", nil
		}
		return exec.LookPath(file)
	}

	found, err := ensureGovulncheck(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != fakeBin {
		t.Fatalf("expected %s from GOBIN to take precedence, got %s", fakeBin, found)
	}
}

func TestGetGovulncheckPath(t *testing.T) {
	ctx := context.Background()

	t.Run("with explicit GOBIN", func(t *testing.T) {
		t.Setenv("GOBIN", "/custom/gobin")
		path, err := getGovulncheckPath(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Clean("/custom/gobin/govulncheck")
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	})

	t.Run("with empty GOBIN uses GOPATH", func(t *testing.T) {
		t.Setenv("GOBIN", "")
		t.Setenv("GOPATH", "/custom/gopath")
		path, err := getGovulncheckPath(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Clean("/custom/gopath/bin/govulncheck")
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	})

	t.Run("with multi-entry GOPATH uses first entry", func(t *testing.T) {
		t.Setenv("GOBIN", "")
		gopath := strings.Join([]string{"/first/gopath", "/second/gopath"}, string(filepath.ListSeparator))
		t.Setenv("GOPATH", gopath)
		path, err := getGovulncheckPath(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := filepath.Clean("/first/gopath/bin/govulncheck")
		if path != expected {
			t.Fatalf("expected %s, got %s", expected, path)
		}
	})
}
