package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/pdfcheck"
	"github.com/borch-ai/powerword/pkg/config"
)

// createMinimalPDFBytesHelper returns the byte array of a minimal PDF with 1 page of 432x648pt (6"x9")
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

func startTestServer(t *testing.T, workspaceRoot string) (*mcp.ClientSession, context.Context, func()) {
	t.Helper()
	t.Setenv("PATH", "")
	cfg := &config.Config{}
	srv, err := setupServer(workspaceRoot, cfg)
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

func TestPdfcheck_MCP_ValidatePDF(t *testing.T) {
	tempDir := t.TempDir()

	pdfBytes := createMinimalPDFBytesHelper()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	_ = os.WriteFile(pdfPath, pdfBytes, 0600)

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	// 1. Success Call
	argsJSON := fmt.Sprintf(`{
		"pdf_path": %q,
		"expected_width_inches": 6.0,
		"expected_height_inches": 9.0
	}`, pdfPath)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validate_pdf",
		Arguments: json.RawMessage(argsJSON),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, false, `"valid": true`)

	// 2. Mismatch Width
	mismatchJSON := fmt.Sprintf(`{
		"pdf_path": %q,
		"expected_width_inches": 8.5,
		"expected_height_inches": 11.0
	}`, pdfPath)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validate_pdf",
		Arguments: json.RawMessage(mismatchJSON),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, false, `"valid": false`)

	// 3. Missing pdf_path
	badArgsJSON := `{
		"expected_width_inches": 6.0,
		"expected_height_inches": 9.0
	}`

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validate_pdf",
		Arguments: json.RawMessage(badArgsJSON),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, true, "pdf_path parameter is required")

	// 4. Invalid expected dimensions
	invalidDimJSON := fmt.Sprintf(`{
		"pdf_path": %q,
		"expected_width_inches": 0,
		"expected_height_inches": -5
	}`, pdfPath)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validate_pdf",
		Arguments: json.RawMessage(invalidDimJSON),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, true, "parameters must be positive")

	// 5. Preflight execution failure (e.g. nonexistent file path)
	nonexistentJSON := fmt.Sprintf(`{
		"pdf_path": %q,
		"expected_width_inches": 6.0,
		"expected_height_inches": 9.0
	}`, "nonexistent_file_path.pdf")

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validate_pdf",
		Arguments: json.RawMessage(nonexistentJSON),
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	assertResponse(t, res, true, "preflight validation failed")
}

func TestPdfcheck_MCP_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validate_pdf",
		Arguments: json.RawMessage(`{invalid_json}`),
	})
	if err == nil {
		t.Error("expected JSON unmarshal error")
	}
}

func TestRun_ConfigParsing(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	srv, err := setupServer(tempDir, nil)
	if err != nil {
		t.Fatalf("setupServer failed: %v", err)
	}
	if srv == nil {
		t.Error("expected setupServer to return a server")
	}

	// Write bad config to force run() error
	badConfigPath := tempDir + "/powerword.toml"
	_ = os.WriteFile(badConfigPath, []byte("bad config format"), 0600)
	err = run()
	if err == nil {
		t.Error("expected run to fail with invalid config file, got nil")
	}
}

func TestRun_Success(t *testing.T) {
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r
	_ = w.Close() // EOF immediately

	_, wOut, _ := os.Pipe()
	os.Stdout = wOut
	defer func() { _ = wOut.Close() }()

	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	// Write a valid configuration file to test that branch of run()
	validConfigPath := tempDir + "/powerword.toml"
	_ = os.WriteFile(validConfigPath, []byte("[plugins]\ntypst = {}\n"), 0600)

	// should run and exit immediately on EOF
	err := run()
	if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "EOF") {
		t.Errorf("expected clean exit or EOF error from run(), got: %v", err)
	}
}

func TestRun_FallbackAndWorkspaceLookup(t *testing.T) {
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r
	_ = w.Close() // EOF immediately

	_, wOut, _ := os.Pipe()
	os.Stdout = wOut
	defer func() { _ = wOut.Close() }()

	// Unset environment variable to cover fallback branch of run()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", "")

	// should run and exit immediately on EOF
	err := run()
	if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "EOF") {
		t.Errorf("expected clean exit or EOF error from run(), got: %v", err)
	}
}

func TestMainFunction_Success(t *testing.T) {
	oldExit := osExit
	defer func() { osExit = oldExit }()

	osExit = func(code int) {
		// Stdio exit error is acceptable on direct EOF, so we just log it
		t.Logf("osExit called with %d", code)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r
	_ = w.Close() // EOF immediately

	_, wOut, _ := os.Pipe()
	os.Stdout = wOut
	defer func() { _ = wOut.Close() }()

	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	main()
}

func TestMainFunction_Error(t *testing.T) {
	oldExit := osExit
	defer func() { osExit = oldExit }()

	exitCalled := false
	exitCode := -1
	osExit = func(code int) {
		exitCalled = true
		exitCode = code
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r
	_ = w.Close() // EOF immediately

	_, wOut, _ := os.Pipe()
	os.Stdout = wOut
	defer func() { _ = wOut.Close() }()

	tempDir := t.TempDir()
	t.Setenv("POWERWORD_WORKSPACE_ROOT", tempDir)

	// Write bad config to force run() error
	badConfigPath := tempDir + "/powerword.toml"
	_ = os.WriteFile(badConfigPath, []byte("bad config format"), 0600)

	main()

	if !exitCalled {
		t.Error("expected osExit to be called")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
}

func createCoverPDFBytesHelper(widthPt, heightPt float64, contentsStr string) []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.1f %.1f] /Contents 4 0 R /Resources << /XObject << /Im1 5 0 R >> >> >>\nendobj\n", widthPt, heightPt)
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(contentsStr), contentsStr)
	obj5 := "5 0 obj\n<< /Type /XObject /Subtype /Image /Width 100 /Height 100 >>\nstream\n\nendstream\nendobj\n"

	header := "%PDF-1.4\n"
	off1 := len(header)
	off2 := off1 + len(obj1)
	off3 := off2 + len(obj2)
	off4 := off3 + len(obj3)
	off5 := off4 + len(obj4)
	offXref := off5 + len(obj5)

	xref := "xref\n0 6\n0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", off1) +
		fmt.Sprintf("%010d 00000 n \n", off2) +
		fmt.Sprintf("%010d 00000 n \n", off3) +
		fmt.Sprintf("%010d 00000 n \n", off4) +
		fmt.Sprintf("%010d 00000 n \n", off5)

	trailer := fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", offXref)
	return []byte(header + obj1 + obj2 + obj3 + obj4 + obj5 + xref + trailer)
}

func setupValidateCoverMocks() func() {
	unmockLookPath := pdfcheck.MockExecLookPath(func(file string) (string, error) {
		return "/mocked/path/to/" + file, nil
	})

	unmockCommand := pdfcheck.MockExecCommand(func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if command == "pdfimages" {
			if len(args) > 0 && args[0] == "-list" {
				out := "page   num  type   width height color comp bpc  enc interp  object ID x-dpi y-dpi   size ratio\n" +
					"--------------------------------------------------------------------------------------------\n" +
					"   1     0 image     100   100 gray     1   8  png    no         5  0   300   300   10K   10%\n"
				return exec.CommandContext(ctx, "/bin/echo", out)
			}
			if len(args) > 0 && args[0] == "-png" {
				prefix := args[len(args)-1]
				targetFile := prefix + "-000.png"
				//nolint:gosec
				return exec.CommandContext(ctx, "/bin/sh", "-c", "echo dummy > "+targetFile)
			}
		}
		if command == "zbarimg" {
			return exec.CommandContext(ctx, "/bin/echo", "9781234567890")
		}
		return exec.CommandContext(ctx, "/bin/echo", "")
	})

	return func() {
		unmockCommand()
		unmockLookPath()
	}
}

func TestPdfcheck_MCP_ValidateCoverPDF(t *testing.T) {
	defer setupValidateCoverMocks()()

	tempDir := t.TempDir()

	// 200 page B&W book: spine = 0.4504 in. Width = 12.7004 in (914.43 pt). Height = 9.25 in (666 pt).
	// Image drawn at X=50 pt (back cover)
	contents := "q\n100 0 0 100 50 100 cm\n/Im1 Do\nQ\n"
	pdfBytes := createCoverPDFBytesHelper(914.43, 666.00, contents)
	pdfPath := filepath.Join(tempDir, "cover.pdf")
	_ = os.WriteFile(pdfPath, pdfBytes, 0600)

	session, ctx, cleanup := startTestServer(t, tempDir)
	defer cleanup()

	tests := []struct {
		name       string
		args       string
		wantError  bool
		wantSubstr string
	}{
		{
			name: "Success Call",
			args: fmt.Sprintf(`{
				"pdf_path": %q,
				"expected_width_inches": 6.0,
				"expected_height_inches": 9.0,
				"page_count": 200,
				"paper_type": "white",
				"expected_isbn": "9781234567890"
			}`, pdfPath),
			wantError:  false,
			wantSubstr: `"valid": true`,
		},
		{
			name: "Missing pdf_path",
			args: `{
				"expected_width_inches": 6.0,
				"expected_height_inches": 9.0,
				"page_count": 200,
				"paper_type": "white"
			}`,
			wantError:  true,
			wantSubstr: "pdf_path parameter is required",
		},
		{
			name: "Invalid expected dimensions",
			args: fmt.Sprintf(`{
				"pdf_path": %q,
				"expected_width_inches": 0,
				"expected_height_inches": 9.0,
				"page_count": 200,
				"paper_type": "white"
			}`, pdfPath),
			wantError:  true,
			wantSubstr: "parameters must be positive",
		},
		{
			name: "Invalid page count",
			args: fmt.Sprintf(`{
				"pdf_path": %q,
				"expected_width_inches": 6.0,
				"expected_height_inches": 9.0,
				"page_count": 0,
				"paper_type": "white"
			}`, pdfPath),
			wantError:  true,
			wantSubstr: "page_count parameter must be positive",
		},
		{
			name: "Invalid paper type",
			args: fmt.Sprintf(`{
				"pdf_path": %q,
				"expected_width_inches": 6.0,
				"expected_height_inches": 9.0,
				"page_count": 200,
				"paper_type": "invalid"
			}`, pdfPath),
			wantError:  true,
			wantSubstr: "paper_type parameter must be one of",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "validate_cover_pdf",
				Arguments: json.RawMessage(tc.args),
			})
			if err != nil {
				t.Fatalf("CallTool failed: %v", err)
			}
			assertResponse(t, res, tc.wantError, tc.wantSubstr)
		})
	}
}
