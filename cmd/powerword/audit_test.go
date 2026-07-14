package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/telemetry"
)

func TestAuditCmd_MissingFile(t *testing.T) {
	cmd := newAuditCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	// Use a non-existent file path
	cmd.SetArgs([]string{"--file", "non-existent-telemetry.json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No telemetry file found") {
		t.Errorf("expected 'No telemetry file found' message, got: %s", output)
	}
}

func TestAuditCmd_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "powerword-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "invalid.json")
	if wErr := os.WriteFile(filePath, []byte("{invalid-json"), 0600); wErr != nil {
		t.Fatalf("failed to write invalid json: %v", wErr)
	}

	cmd := newAuditCmd()
	cmd.SetArgs([]string{"--file", filePath})

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "failed to unmarshal telemetry JSON") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestAuditCmd_ValidMarkdownOutput(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "powerword-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Mock active config
	origActive := config.Active
	config.Active = &config.Config{
		Pricing: map[string]telemetry.ModelPricing{
			"gemini-1.5-pro": {Input: 1.0, Output: 2.0, Cached: 0.5},
		},
	}
	defer func() { config.Active = origActive }()

	// Mock telemetry file
	telemetryData := `{
		"turns": 2,
		"model_usages": {
			"gemini-1.5-pro": {
				"input_tokens": 1000000,
				"output_tokens": 500000,
				"cached_tokens": 200000
			}
		}
	}`
	filePath := filepath.Join(tmpDir, "telemetry.json")
	if wErr := os.WriteFile(filePath, []byte(telemetryData), 0600); wErr != nil {
		t.Fatalf("failed to write mock telemetry: %v", wErr)
	}

	cmd := newAuditCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	// Run with standard settings (should be within budget of 2.0 USD)
	// Billed input = 1,000,000 - 200,000 = 800,000
	// Cost = 800,000 * (1.0 / 1,000,000) + 500,000 * (2.0 / 1,000,000) + 200,000 * (0.5 / 1,000,000)
	// Cost = 0.8 + 1.0 + 0.1 = 1.9 USD
	cmd.SetArgs([]string{"--file", filePath, "--limit", "2.0", "--format", "markdown"})

	err = cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "### ⚡ Powerword Token & Budget Audit") {
		t.Errorf("expected markdown header, got: %s", output)
	}
	if !strings.Contains(output, "| `gemini-1.5-pro` | 1000000 | 500000 | 200000 | $1.90000 |") {
		t.Errorf("expected row details, got: %s", output)
	}
	if !strings.Contains(output, "- **Status:** ✅ Within Budget") {
		t.Errorf("expected status within budget, got: %s", output)
	}
}

func TestAuditCmd_StrictAndLimitExceeded(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "powerword-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	origActive := config.Active
	config.Active = &config.Config{
		Pricing: map[string]telemetry.ModelPricing{
			"gemini-1.5-pro": {Input: 1.0, Output: 2.0, Cached: 0.5},
		},
	}
	defer func() { config.Active = origActive }()

	telemetryData := `{
		"turns": 2,
		"model_usages": {
			"gemini-1.5-pro": {
				"input_tokens": 1000000,
				"output_tokens": 500000,
				"cached_tokens": 200000
			}
		}
	}`
	filePath := filepath.Join(tmpDir, "telemetry.json")
	if wErr := os.WriteFile(filePath, []byte(telemetryData), 0600); wErr != nil {
		t.Fatalf("failed to write mock telemetry: %v", wErr)
	}

	// 1. Non-strict: should print Exceeded but not return an error
	cmd := newAuditCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--file", filePath, "--limit", "1.5", "--format", "markdown"})
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error in non-strict execution: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "- **Status:** ⚠️ Budget Exceeded") {
		t.Errorf("expected status budget exceeded, got: %s", output)
	}

	// 2. Strict: should return an error
	cmdStrict := newAuditCmd()
	cmdStrict.SetArgs([]string{"--file", filePath, "--limit", "1.5", "--strict"})
	err = cmdStrict.Execute()
	if err == nil {
		t.Fatal("expected strict execution to return error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds limit") {
		t.Errorf("expected exceeds limit error, got: %v", err)
	}
}

func TestAuditCmd_FallbackPricing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "powerword-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	origActive := config.Active
	config.Active = nil // Force fallback pricing usage
	defer func() { config.Active = origActive }()

	// fallback pricing for gemini-2.5-flash is: Input: 0.075, Output: 0.30, Cached: 0.01875
	// Usage: 10,000,000 input, 1,000,000 output.
	// Billed input = 10,000,000
	// Cost = 10 * 0.075 + 1 * 0.30 = 0.75 + 0.30 = 1.05 USD
	telemetryData := `{
		"turns": 5,
		"model_usages": {
			"gemini-2.5-flash": {
				"input_tokens": 10000000,
				"output_tokens": 1000000,
				"cached_tokens": 0
			}
		}
	}`
	filePath := filepath.Join(tmpDir, "telemetry.json")
	if wErr := os.WriteFile(filePath, []byte(telemetryData), 0600); wErr != nil {
		t.Fatalf("failed to write mock telemetry: %v", wErr)
	}

	cmd := newAuditCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--file", filePath, "--limit", "2.0", "--format", "markdown"})
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "| `gemini-2.5-flash` | 10000000 | 1000000 | 0 | $1.05000 |") {
		t.Errorf("expected correct fallback pricing row calculation, got: %s", output)
	}
}

func TestAuditCmd_MissingPricing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "powerword-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	origActive := config.Active
	config.Active = &config.Config{
		Pricing: map[string]telemetry.ModelPricing{
			// Empty pricing to force missing pricing
		},
	}
	defer func() { config.Active = origActive }()

	telemetryData := `{
		"turns": 2,
		"model_usages": {
			"unknown-model-foo": {
				"input_tokens": 100,
				"output_tokens": 50,
				"cached_tokens": 0
			}
		}
	}`
	filePath := filepath.Join(tmpDir, "telemetry.json")
	if wErr := os.WriteFile(filePath, []byte(telemetryData), 0600); wErr != nil {
		t.Fatalf("failed to write mock telemetry: %v", wErr)
	}

	// 1. Markdown output: should show N/A and Incomplete status
	cmd := newAuditCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--file", filePath, "--limit", "1.5", "--format", "markdown"})
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "| `unknown-model-foo` | 100 | 50 | 0 | N/A |") {
		t.Errorf("expected N/A for missing model cost, got: %s", output)
	}
	if !strings.Contains(output, "| **Total** | **100** | **50** | **0** | **$0.00000 (Incomplete)** |") {
		t.Errorf("expected incomplete total cost, got: %s", output)
	}
	if !strings.Contains(output, "- **Status:** ⚠️ Missing Pricing (Budget Incomplete)") {
		t.Errorf("expected missing pricing status, got: %s", output)
	}

	// 2. Text output: should show N/A in Estimated Cost
	cmdText := newAuditCmd()
	var bufText bytes.Buffer
	cmdText.SetOut(&bufText)
	cmdText.SetArgs([]string{"--file", filePath, "--limit", "1.5", "--format", "text"})
	err = cmdText.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outputText := bufText.String()
	if !strings.Contains(outputText, "- Estimated Cost: N/A") {
		t.Errorf("expected N/A for estimated cost in text output, got: %s", outputText)
	}
}

func TestAuditCmd_StrictWithMissingPricing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "powerword-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	origActive := config.Active
	config.Active = &config.Config{
		Pricing: map[string]telemetry.ModelPricing{},
	}
	defer func() { config.Active = origActive }()

	telemetryData := `{
		"turns": 2,
		"model_usages": {
			"unknown-model-foo": {
				"input_tokens": 100,
				"output_tokens": 50,
				"cached_tokens": 0
			}
		}
	}`
	filePath := filepath.Join(tmpDir, "telemetry.json")
	if wErr := os.WriteFile(filePath, []byte(telemetryData), 0600); wErr != nil {
		t.Fatalf("failed to write mock telemetry: %v", wErr)
	}

	cmdStrict := newAuditCmd()
	cmdStrict.SetArgs([]string{"--file", filePath, "--limit", "1.5", "--strict"})
	err = cmdStrict.Execute()
	if err == nil {
		t.Fatal("expected strict execution to fail with missing pricing error, got nil")
	}
	if !strings.Contains(err.Error(), "budget audit failed: missing pricing for models") {
		t.Errorf("expected missing pricing error, got: %v", err)
	}
}
