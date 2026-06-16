//go:build integration

package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		Env:     []string{"POWERWORD_WORKSPACE_ROOT=" + workspaceDir},
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
