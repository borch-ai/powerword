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

func createMockTypstBin(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	mockPath := filepath.Join(tmpDir, "mock_typst")

	var content string
	if filepath.Separator == '\\' {
		content = `@echo off
set "out="
for %%a in (%*) do set "out=%%a"
(
echo %%PDF-1.4
echo 1 0 obj ^<^< /Type /Catalog /Pages 2 0 R ^>^> endobj
echo 2 0 obj ^<^< /Type /Pages /Kids [3 0 R] /Count 1 ^>^> endobj
echo 3 0 obj ^<^< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] ^>^> endobj
echo xref
echo 0 4
echo 0000000000 65535 f
echo 0000000009 00000 n
echo 0000000058 00000 n
echo 0000000114 00000 n
echo trailer ^<^< /Size 4 /Root 1 0 R ^>^>
echo startxref
echo 178
echo %%%%EOF
) > "%out%"
`
	} else {
		content = `#!/bin/sh
out=""
for arg; do out=$arg; done
cat << 'EOF' > "$out"
%PDF-1.4
1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj
2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj
3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >> endobj
xref
0 4
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000114 00000 n
trailer << /Size 4 /Root 1 0 R >>
startxref
178
%%EOF
EOF
`
	}

	if err := os.WriteFile(mockPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write mock typst script: %v", err)
	}
	//nolint:gosec // mock test script needs execute permission
	if err := os.Chmod(mockPath, 0700); err != nil {
		t.Fatalf("failed to chmod mock typst script: %v", err)
	}

	return mockPath
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

func TestTypst_MCP_CompileInteriorError(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	calcRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "compile_interior",
		Arguments: json.RawMessage(`{
			"manuscript_path": "../escaped_manuscript.md",
			"images_dir": "images",
			"output_path": "output.pdf"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool compile_interior failed: %v", err)
	}
	assertResponse(t, calcRes, true, "is outside of workspace")
}

func TestTypst_MCP_CompileCoverError(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	calcRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "compile_cover",
		Arguments: json.RawMessage(`{
			"front_image_path": "../escaped_image.png",
			"title": "Title",
			"subtitle": "Subtitle",
			"author": "Author",
			"page_count": 32,
			"output_path": "cover.pdf"
		}`),
	})
	if err != nil {
		t.Fatalf("CallTool compile_cover failed: %v", err)
	}
	assertResponse(t, calcRes, true, "is outside of workspace")
}

func TestTypst_MCP_UnmarshalErrors(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_interior",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_cover",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}

func TestTypst_MCP_CompileInteriorSuccess(t *testing.T) {
	mockTypst := createMockTypstBin(t)
	t.Setenv("POWERWORD_TYPST_BIN", mockTypst)

	tempDir := t.TempDir()

	manuscriptPath := filepath.Join(tempDir, "manuscript.md")
	manuscriptContent := `
# Page 1
## Text
Stanza 1

## Prompt
Prompt 1
`
	if err := os.WriteFile(manuscriptPath, []byte(manuscriptContent), 0600); err != nil {
		t.Fatalf("failed to write mock manuscript: %v", err)
	}

	imagesDir := filepath.Join(tempDir, "images")
	if err := os.Mkdir(imagesDir, 0700); err != nil {
		t.Fatalf("failed to create images dir: %v", err)
	}
	realPNG, err := os.ReadFile("/Users/human/code/pithos/books/red-riding-hood/images/page_1.png")
	if err != nil {
		t.Fatalf("failed to read source png: %v", err)
	}
	//nolint:gosec // paths are generated safely in tempDir
	err = os.WriteFile(filepath.Join(imagesDir, "page_1.png"), realPNG, 0600)
	if err != nil {
		t.Fatalf("failed to write mock image: %v", err)
	}

	outputPath := filepath.Join(tempDir, "output.pdf")

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	args := fmt.Sprintf(`{
		"manuscript_path": %q,
		"images_dir": %q,
		"output_path": %q,
		"page_size": "4in,4in",
		"margin_inside": "0.5in",
		"margin_outside": "0.5in",
		"bleed": "0in"
	}`, manuscriptPath, imagesDir, outputPath)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_interior",
		Arguments: json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("CallTool compile_interior failed: %v", err)
	}

	assertResponse(t, res, false, "output_pdf")
	assertResponse(t, res, false, "page_count")
}

func TestTypst_MCP_CompileCoverSuccess(t *testing.T) {
	mockTypst := createMockTypstBin(t)
	t.Setenv("POWERWORD_TYPST_BIN", mockTypst)

	tempDir := t.TempDir()
	frontImgPath := filepath.Join(tempDir, "front.png")
	realPNG, err := os.ReadFile("/Users/human/code/pithos/books/red-riding-hood/images/page_1.png")
	if err != nil {
		t.Fatalf("failed to read source png: %v", err)
	}
	//nolint:gosec // paths are generated safely in tempDir
	err = os.WriteFile(frontImgPath, realPNG, 0600)
	if err != nil {
		t.Fatalf("failed to write mock image: %v", err)
	}

	outputPath := filepath.Join(tempDir, "cover.pdf")

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	args := fmt.Sprintf(`{
		"front_image_path": %q,
		"title": "Title",
		"subtitle": "Subtitle",
		"author": "Author",
		"page_count": 32,
		"output_path": %q,
		"trim_size": "8.5x8.5",
		"paper_type": "white",
		"binding_type": "paperback"
	}`, frontImgPath, outputPath)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_cover",
		Arguments: json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("CallTool compile_cover failed: %v", err)
	}

	assertResponse(t, res, false, "output_pdf")
}

func TestTypst_MCP_CompileInteriorTypstSuccess(t *testing.T) {
	mockTypst := createMockTypstBin(t)
	t.Setenv("POWERWORD_TYPST_BIN", mockTypst)

	tempDir := t.TempDir()
	manuscriptPath := filepath.Join(tempDir, "manuscript.typ")
	content := "= My Typst Title\nThis is a pure Typst document."
	if err := os.WriteFile(manuscriptPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write mock manuscript: %v", err)
	}

	outputPath := filepath.Join(tempDir, "output.pdf")

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	args := fmt.Sprintf(`{
		"manuscript_path": %q,
		"images_dir": %q,
		"output_path": %q,
		"page_size": "8.5in,8.5in"
	}`, manuscriptPath, tempDir, outputPath)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_interior",
		Arguments: json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("CallTool compile_interior failed: %v", err)
	}

	assertResponse(t, res, false, "output_pdf")
	assertResponse(t, res, false, "page_count")
}

func TestIntegrationTypst_MCP_CompileInterior(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("skipping integration tests, set RUN_INTEGRATION_TESTS=true to run")
	}

	tempDir := t.TempDir()

	manuscriptPath := filepath.Join(tempDir, "manuscript.md")
	manuscriptContent := `
# Page 1
## Text
Stanza 1 with some text to compile.

## Prompt
Prompt 1
`
	if err := os.WriteFile(manuscriptPath, []byte(manuscriptContent), 0600); err != nil {
		t.Fatalf("failed to write manuscript: %v", err)
	}

	imagesDir := filepath.Join(tempDir, "images")
	if err := os.Mkdir(imagesDir, 0700); err != nil {
		t.Fatalf("failed to create images dir: %v", err)
	}
	realPNG, err := os.ReadFile("/Users/human/code/pithos/books/red-riding-hood/images/page_1.png")
	if err != nil {
		t.Fatalf("failed to read source png: %v", err)
	}
	//nolint:gosec // paths are generated safely in tempDir
	err = os.WriteFile(filepath.Join(imagesDir, "page_1.png"), realPNG, 0600)
	if err != nil {
		t.Fatalf("failed to write image: %v", err)
	}

	outputPath := filepath.Join(tempDir, "output.pdf")

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	args := fmt.Sprintf(`{
		"manuscript_path": %q,
		"images_dir": %q,
		"output_path": %q,
		"page_size": "4in,4in",
		"margin_inside": "0.5in",
		"margin_outside": "0.5in",
		"bleed": "0in"
	}`, manuscriptPath, imagesDir, outputPath)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_interior",
		Arguments: json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("CallTool compile_interior failed: %v", err)
	}

	assertResponse(t, res, false, "output_pdf")
	assertResponse(t, res, false, "page_count")

	// Verify the PDF actually exists and has content
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("compiled output PDF does not exist: %v", err)
	}
	if info.Size() < 100 {
		t.Errorf("compiled PDF size too small: %d bytes", info.Size())
	}
}

func TestIntegrationTypst_MCP_CompileCover(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("skipping integration tests, set RUN_INTEGRATION_TESTS=true to run")
	}

	tempDir := t.TempDir()
	frontImgPath := filepath.Join(tempDir, "front.png")
	realPNG, err := os.ReadFile("/Users/human/code/pithos/books/red-riding-hood/images/page_1.png")
	if err != nil {
		t.Fatalf("failed to read source png: %v", err)
	}
	//nolint:gosec // paths are generated safely in tempDir
	err = os.WriteFile(frontImgPath, realPNG, 0600)
	if err != nil {
		t.Fatalf("failed to write image: %v", err)
	}

	outputPath := filepath.Join(tempDir, "cover.pdf")

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	args := fmt.Sprintf(`{
		"front_image_path": %q,
		"title": "Title",
		"subtitle": "Subtitle",
		"author": "Author",
		"page_count": 32,
		"output_path": %q,
		"trim_size": "8.5x8.5",
		"paper_type": "white",
		"binding_type": "paperback"
	}`, frontImgPath, outputPath)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "compile_cover",
		Arguments: json.RawMessage(args),
	})
	if err != nil {
		t.Fatalf("CallTool compile_cover failed: %v", err)
	}

	assertResponse(t, res, false, "output_pdf")

	// Verify the PDF actually exists and has content
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("compiled output PDF does not exist: %v", err)
	}
	if info.Size() < 100 {
		t.Errorf("compiled PDF size too small: %d bytes", info.Size())
	}
}
