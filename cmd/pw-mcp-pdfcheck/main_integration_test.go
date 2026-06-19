//go:build integration

package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
	sdkMcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

var pluginPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "pw-mcp-pdfcheck-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	pluginPath = filepath.Join(tmpDir, "pw-mcp-pdfcheck")
	cmd := exec.Command("go", "build", "-o", pluginPath, ".")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pw-mcp-pdfcheck: %v\n", err)
		os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()

	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func createMinimalPDFBytesHelper() []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] >>\nendobj\n"

	header := "%PDF-1.4\n"
	off1 := len(header)
	off2 := off1 + len(obj1)
	off3 := off2 + len(obj2)
	offXref := off3 + len(obj3)

	xref := "xref\n0 4\n0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", off1) +
		fmt.Sprintf("%010d 00000 n \n", off2) +
		fmt.Sprintf("%010d 00000 n \n", off3)

	trailer := fmt.Sprintf("trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%EOF\n", offXref)

	return []byte(header + obj1 + obj2 + obj3 + xref + trailer)
}

func TestMCP_PdfcheckPlugin_StdoutStdin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a temp workspace directory
	workspaceDir, err := os.MkdirTemp("", "pw-pdfcheck-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	pdfBytes := createMinimalPDFBytesHelper()
	pdfPath := filepath.Join(workspaceDir, "test.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatal(err)
	}

	// Setup server config
	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir, "PATH="},
	}

	// Create and start ServerProcess
	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-pdfcheck", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() {
		_ = sp.GracefulShutdown(1 * time.Second)
	}()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	// 1. Handshake / ListTools
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	foundValidatePDF := false
	for _, tool := range tools {
		if tool.Name == "validate_pdf" {
			foundValidatePDF = true
		}
	}
	if !foundValidatePDF {
		t.Errorf("expected to find 'validate_pdf' tool, got tools: %+v", tools)
	}

	// 2. CallTool to validate the PDF
	args := map[string]interface{}{
		"pdf_path":               pdfPath,
		"expected_width_inches":  6.0,
		"expected_height_inches": 9.0,
	}
	result, err := client.CallTool(ctx, "validate_pdf", args)
	if err != nil {
		t.Fatalf("failed to call validate_pdf tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	if len(result.Content) == 0 {
		t.Fatal("expected response content, got none")
	}

	var contentStr string
	if txt, ok := result.Content[0].(*sdkMcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(result.Content[0])
	}

	type validationResponse struct {
		Valid      bool     `json:"valid"`
		PageCount  int      `json:"page_count"`
		Dimensions string   `json:"dimensions"`
		Errors     []string `json:"errors"`
		Warnings   []string `json:"warnings"`
	}

	var resp validationResponse
	if err := json.Unmarshal([]byte(contentStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal validation verdict: %v, raw content: %s", err, contentStr)
	}

	if !resp.Valid {
		t.Errorf("expected validation to be valid, got errors: %v", resp.Errors)
	}
	if resp.PageCount != 1 {
		t.Errorf("expected 1 page, got %d", resp.PageCount)
	}
	if resp.Dimensions != "6.000 x 9.000 in" {
		t.Errorf("expected dimensions '6.000 x 9.000 in', got %q", resp.Dimensions)
	}
}

func TestMCP_PdfcheckPlugin_Grayscale(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workspaceDir, err := os.MkdirTemp("", "pw-pdfcheck-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Create a PDF with RGB operator inside
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] /Contents 4 0 R >>\nendobj\n"
	contentsStr := "1 0 0 rg\n"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(contentsStr), contentsStr)

	header := "%PDF-1.4\n"
	off1 := len(header)
	off2 := off1 + len(obj1)
	off3 := off2 + len(obj2)
	off4 := off3 + len(obj3)
	offXref := off4 + len(obj4)

	xref := "xref\n0 5\n0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", off1) +
		fmt.Sprintf("%010d 00000 n \n", off2) +
		fmt.Sprintf("%010d 00000 n \n", off3) +
		fmt.Sprintf("%010d 00000 n \n", off4)

	trailer := fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%EOF\n", offXref)
	pdfBytes := []byte(header + obj1 + obj2 + obj3 + obj4 + xref + trailer)

	pdfPath := filepath.Join(workspaceDir, "color.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatal(err)
	}

	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir, "PATH="},
	}

	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-pdfcheck", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() {
		_ = sp.GracefulShutdown(1 * time.Second)
	}()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	args := map[string]interface{}{
		"pdf_path":               pdfPath,
		"expected_width_inches":  6.0,
		"expected_height_inches": 9.0,
		"enforce_grayscale":      true,
	}
	result, err := client.CallTool(ctx, "validate_pdf", args)
	if err != nil {
		t.Fatalf("failed to call validate_pdf tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	var contentStr string
	if txt, ok := result.Content[0].(*sdkMcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(result.Content[0])
	}

	type validationResponse struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}

	var resp validationResponse
	if err := json.Unmarshal([]byte(contentStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal validation verdict: %v, raw content: %s", err, contentStr)
	}

	if resp.Valid {
		t.Error("expected validation to be invalid because it contains RGB operator")
	}

	foundRGBError := false
	for _, e := range resp.Errors {
		if strings.Contains(e, "RGB vector/text color setting operator (rg)") {
			foundRGBError = true
		}
	}
	if !foundRGBError {
		t.Errorf("expected RGB vector/text color setting error, got errors: %v", resp.Errors)
	}
}

func TestMCP_PdfcheckPlugin_Margins(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workspaceDir, err := os.MkdirTemp("", "pw-pdfcheck-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	// Create a PDF with text in bottom margin Y=10
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] /Contents 4 0 R\n" +
		"  /Resources <<\n" +
		"    /Font <<\n" +
		"      /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\n" +
		"    >>\n" +
		"  >>\n" +
		">> \nendobj\n"
	contentsStr := "BT /F1 10 Tf 1 0 0 1 100 10 Tm (Violator) Tj ET\n"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(contentsStr), contentsStr)

	header := "%PDF-1.4\n"
	off1 := len(header)
	off2 := off1 + len(obj1)
	off3 := off2 + len(obj2)
	off4 := off3 + len(obj3)
	offXref := off4 + len(obj4)

	xref := "xref\n0 5\n0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", off1) +
		fmt.Sprintf("%010d 00000 n \n", off2) +
		fmt.Sprintf("%010d 00000 n \n", off3) +
		fmt.Sprintf("%010d 00000 n \n", off4)

	trailer := fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%EOF\n", offXref)
	pdfBytes := []byte(header + obj1 + obj2 + obj3 + obj4 + xref + trailer)

	pdfPath := filepath.Join(workspaceDir, "margin_violation.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatal(err)
	}

	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir, "PATH="},
	}

	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-pdfcheck", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() {
		_ = sp.GracefulShutdown(1 * time.Second)
	}()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	args := map[string]interface{}{
		"pdf_path":               pdfPath,
		"expected_width_inches":  6.0,
		"expected_height_inches": 9.0,
		"min_margin_inches":      0.25,
	}
	result, err := client.CallTool(ctx, "validate_pdf", args)
	if err != nil {
		t.Fatalf("failed to call validate_pdf tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	var contentStr string
	if txt, ok := result.Content[0].(*sdkMcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(result.Content[0])
	}

	type validationResponse struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}

	var resp validationResponse
	if err := json.Unmarshal([]byte(contentStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal validation verdict: %v, raw content: %s", err, contentStr)
	}

	if resp.Valid {
		t.Error("expected validation to be invalid because it violates the bottom margin")
	}

	foundMarginError := false
	for _, e := range resp.Errors {
		if strings.Contains(e, "within bottom margin") {
			foundMarginError = true
		}
	}
	if !foundMarginError {
		t.Errorf("expected bottom margin violation error, got errors: %v", resp.Errors)
	}
}

func TestMCP_PdfcheckPlugin_InkCoverage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	workspaceDir, err := os.MkdirTemp("", "pw-pdfcheck-workspace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspaceDir)

	pdfBytes := createMinimalPDFBytesHelper()
	pdfPath := filepath.Join(workspaceDir, "test.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatal(err)
	}

	srvCfg := config.ServerConfig{
		Command: pluginPath,
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir, "PATH=" + os.Getenv("PATH")},
	}

	sp, err := mcp.NewServerProcess(ctx, "pw-mcp-pdfcheck", srvCfg)
	if err != nil {
		t.Fatalf("failed to launch ServerProcess: %v", err)
	}
	defer func() {
		_ = sp.GracefulShutdown(1 * time.Second)
	}()

	client := sp.Client()
	if client == nil {
		t.Fatal("expected MCP client to be initialized, got nil")
	}

	// Case A: Valid MaxInkCoverage on a white/blank PDF
	args := map[string]interface{}{
		"pdf_path":               pdfPath,
		"expected_width_inches":  6.0,
		"expected_height_inches": 9.0,
		"max_ink_coverage":       240,
	}
	result, err := client.CallTool(ctx, "validate_pdf", args)
	if err != nil {
		t.Fatalf("failed to call validate_pdf tool: %v", err)
	}

	if result.IsError {
		t.Fatalf("tool execution returned error: %v", result)
	}

	var contentStr string
	if txt, ok := result.Content[0].(*sdkMcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(result.Content[0])
	}

	type validationResponse struct {
		Valid    bool     `json:"valid"`
		Errors   []string `json:"errors"`
		Warnings []string `json:"warnings"`
	}

	var resp validationResponse
	if err := json.Unmarshal([]byte(contentStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal validation verdict: %v", err)
	}

	if !resp.Valid {
		t.Errorf("expected blank PDF to be valid for ink density check, got errors: %v", resp.Errors)
	}

	// Case B: Invalid MaxInkCoverage parameter (out of bounds)
	badArgs := map[string]interface{}{
		"pdf_path":               pdfPath,
		"expected_width_inches":  6.0,
		"expected_height_inches": 9.0,
		"max_ink_coverage":       -5,
	}
	badResult, err := client.CallTool(ctx, "validate_pdf", badArgs)
	if err != nil {
		t.Fatalf("failed to call validate_pdf tool with bad args: %v", err)
	}

	if !badResult.IsError {
		t.Error("expected error response for invalid max_ink_coverage, got none")
	}

	var badContentStr string
	if txt, ok := badResult.Content[0].(*sdkMcp.TextContent); ok {
		badContentStr = txt.Text
	} else {
		badContentStr = fmt.Sprint(badResult.Content[0])
	}

	if !strings.Contains(badContentStr, "max_ink_coverage parameter must be between 0 and 400") {
		t.Errorf("expected error message to contain 'between 0 and 400', got: %s", badContentStr)
	}
}
