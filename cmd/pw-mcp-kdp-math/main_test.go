package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/pkg/config"
)

func assertResponse(t *testing.T, res *mcp.CallToolResult, wantError bool, wantSubstr string) {
	t.Helper()
	if res.IsError != wantError {
		t.Errorf("expected error: %v, got: %v (content: %v)", wantError, res.IsError, res.Content)
	}
	if len(res.Content) == 0 {
		t.Errorf("no content in response")
		return
	}
	var contentStr string
	if txt, ok := res.Content[0].(*mcp.TextContent); ok {
		contentStr = txt.Text
	} else {
		contentStr = fmt.Sprint(res.Content[0])
	}
	if !strings.Contains(contentStr, wantSubstr) {
		t.Errorf("expected response to contain %q, got %q", wantSubstr, contentStr)
	}
}

// createMinimalPDFBytes returns the byte array of a minimal PDF with 1 page of 432x648pt (6"x9") but ending in a single-percent EOF (%EOF) to test the virtual safeReaderAt correction logic.
func createMinimalPDFBytes() []byte {
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

func startTestServer(t *testing.T, tempDir string) (*mcp.ClientSession, context.Context, func()) {
	t.Helper()
	cfg := &config.Config{}
	srv, err := setupServer(tempDir, cfg)
	if err != nil {
		t.Fatalf("failed to setup server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t1, t2 := mcp.NewInMemoryTransports()
	go func() {
		if runErr := srv.Run(ctx, t1); runErr != nil && runErr != context.Canceled {
			panic(fmt.Errorf("server run err: %v", runErr))
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		cancel()
		t.Fatalf("failed to connect client: %v", err)
	}

	cleanup := func() {
		_ = session.Close()
		cancel()
	}
	return session, ctx, cleanup
}

func TestKDPMath_MCP_CalculateGeometry(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	// Test 1: Calculate Geometry Success
	calcRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kdp_calculate_geometry",
		Arguments: json.RawMessage(`{
			"page_count": 100,
			"binding_type": "paperback",
			"paper_type": "white",
			"trim_size": "6x9"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool kdp_calculate_geometry failed: %v", err)
	}
	assertResponse(t, calcRes, false, "cover_width_inches")
	assertResponse(t, calcRes, false, "spine_width_inches")

	// Test 2: Calculate Geometry Error (invalid paper type)
	calcErrRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kdp_calculate_geometry",
		Arguments: json.RawMessage(`{
			"page_count": 100,
			"binding_type": "paperback",
			"paper_type": "glossy",
			"trim_size": "6x9"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool kdp_calculate_geometry error call failed: %v", err)
	}
	assertResponse(t, calcErrRes, true, "failed to calculate geometry")
}

func TestKDPMath_MCP_ValidatePDF(t *testing.T) {
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	pdfBytes := createMinimalPDFBytes()
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write mock PDF: %v", err)
	}

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	// Test 1: Validate PDF Success
	valRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kdp_validate_pdf",
		Arguments: json.RawMessage(fmt.Sprintf(`{
			"pdf_path": %q,
			"binding_type": "paperback",
			"paper_type": "white",
			"trim_size": "6x9",
			"expected_page_count": 1,
			"is_cover": false,
			"has_bleed": false
		}`, pdfPath)),
	})
	if err != nil {
		t.Fatalf("CallTool kdp_validate_pdf failed: %v", err)
	}
	assertResponse(t, valRes, false, `"is_valid": true`)

	// Test 2: Validate PDF Error (invalid trim size)
	valErrRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kdp_validate_pdf",
		Arguments: json.RawMessage(fmt.Sprintf(`{
			"pdf_path": %q,
			"binding_type": "paperback",
			"paper_type": "white",
			"trim_size": "invalid_trim",
			"expected_page_count": 1,
			"is_cover": false,
			"has_bleed": false
		}`, pdfPath)),
	})
	if err != nil {
		t.Fatalf("CallTool kdp_validate_pdf error call failed: %v", err)
	}
	assertResponse(t, valErrRes, true, "failed to validate PDF")
}

func TestKDPMath_MCP_GenerateManifest(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	// Test 1: Generate Manifest Success
	manRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kdp_generate_manifest",
		Arguments: json.RawMessage(`{
			"page_count": 100,
			"binding_type": "paperback",
			"paper_type": "white",
			"trim_size": "6x9"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool kdp_generate_manifest failed: %v", err)
	}
	assertResponse(t, manRes, false, "trim_width_inches")

	// Test 2: Generate Manifest Error (invalid paper type)
	manErrRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kdp_generate_manifest",
		Arguments: json.RawMessage(`{
			"page_count": 100,
			"binding_type": "paperback",
			"paper_type": "glossy",
			"trim_size": "6x9"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool kdp_generate_manifest error call failed: %v", err)
	}
	assertResponse(t, manErrRes, true, "failed to generate manifest")
}

func TestKDPMath_MCP_UnmarshalErrors(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kdp_calculate_geometry",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kdp_validate_pdf",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kdp_generate_manifest",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}

func TestKDPMath_MCP_SandboxEscape(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	valRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kdp_validate_pdf",
		Arguments: json.RawMessage(`{
			"pdf_path": "../escaped_secret.pdf",
			"binding_type": "paperback",
			"paper_type": "white",
			"trim_size": "6x9",
			"expected_page_count": 1,
			"is_cover": false,
			"has_bleed": false
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool kdp_validate_pdf failed: %v", err)
	}
	assertResponse(t, valRes, true, "is outside of workspace")
}
