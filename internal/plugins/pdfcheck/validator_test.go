package pdfcheck

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rsc.io/pdf"
)

// createMinimalPDFBytes returns the byte array of a minimal PDF with 1 page of 432x648pt (6"x9")
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

func createMultipageMismatchPDFBytes() []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 500 700] >>\nendobj\n"

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

	return []byte(header + obj1 + obj2 + obj3 + obj4 + xref + trailer)
}

func TestValidatePDFPreflight_Success(t *testing.T) {
	defer mockLookPathSuccess()()
	pdfBytes := createMinimalPDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if strings.Contains(command, "pdffonts") {
			out := "name                                 type              encoding         emb sub uni object ID\n------------------------------------ ----------------- ---------------- --- --- --- ---------\nHelvetica                            Type 1            Standard         yes no  no       8  0\n"
			return exec.CommandContext(ctx, "echo", out)
		}
		if strings.Contains(command, "pdfimages") {
			out := "page   num  type   width height color comp bpc  enc interp  object ID x-dpi y-dpi   size ratio\n--------------------------------------------------------------------------------------------\n   1     0 image    1200   1800 cmyk     4   8  jpeg   no        12  0   300   300  84.5K  1.3%\n"
			return exec.CommandContext(ctx, "echo", out)
		}
		return exec.CommandContext(ctx, "echo", "")
	}

	bleed := 0.125
	minDPI := 300
	enforceFonts := true
	enforceCMYK := true

	input := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		BleedInches:          &bleed,
		MinDPI:               &minDPI,
		EnforceEmbeddedFonts: &enforceFonts,
		EnforceCMYK:          &enforceCMYK,
	}

	res, err := ValidatePDFPreflight(context.Background(), input)
	if err != nil {
		t.Fatalf("ValidatePDFPreflight returned unexpected error: %v", err)
	}

	if !res.Valid {
		t.Errorf("expected valid PDF, got errors: %v", res.Errors)
	}
	if len(res.Errors) != 0 {
		t.Errorf("expected 0 errors, got: %v", res.Errors)
	}
	if res.PageCount != 1 {
		t.Errorf("page count = %d, want 1", res.PageCount)
	}
	if res.Dimensions != "6.000 x 9.000 in" {
		t.Errorf("expected dimensions '6.000 x 9.000 in', got %q", res.Dimensions)
	}
}

func TestValidatePDFPreflight_Errors(t *testing.T) {
	defer mockLookPathSuccess()()
	pdfBytes := createMinimalPDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if strings.Contains(command, "pdffonts") {
			// Arial is NOT embedded ("no" in the 4th column)
			out := "name                                 type              encoding         emb sub uni object ID\n------------------------------------ ----------------- ---------------- --- --- --- ---------\nArial                                Type 1            Standard         no  no  no       8  0\n"
			return exec.CommandContext(ctx, "echo", out)
		}
		if strings.Contains(command, "pdfimages") {
			// RGB color space and 150 DPI (below min_dpi of 300)
			out := "page   num  type   width height color comp bpc  enc interp  object ID x-dpi y-dpi   size ratio\n--------------------------------------------------------------------------------------------\n   1     0 image    1200   1800 rgb      3   8  jpeg   no        12  0   150   150  84.5K  1.3%\n"
			return exec.CommandContext(ctx, "echo", out)
		}
		return exec.CommandContext(ctx, "echo", "")
	}

	bleed := 0.125
	minDPI := 300
	enforceFonts := true
	enforceCMYK := true

	input := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  8.5,  // mismatch width (minimal PDF is 6x9)
		ExpectedHeightInches: 11.0, // mismatch height
		BleedInches:          &bleed,
		MinDPI:               &minDPI,
		EnforceEmbeddedFonts: &enforceFonts,
		EnforceCMYK:          &enforceCMYK,
	}

	res, err := ValidatePDFPreflight(context.Background(), input)
	if err != nil {
		t.Fatalf("ValidatePDFPreflight returned unexpected error: %v", err)
	}

	if res.Valid {
		t.Error("expected invalid PDF validation result")
	}

	// Verify all errors are correctly caught
	assertHasError(t, res.Errors, "width mismatch")
	assertHasError(t, res.Errors, "height mismatch")
	assertHasError(t, res.Errors, "Font Arial is not embedded")
	assertHasError(t, res.Errors, "resolution 150 x 150 DPI is below minimum")
	assertHasError(t, res.Errors, "RGB color space")
}

func TestValidatePDFPreflight_MissingTools(t *testing.T) {
	oldLookPath := execLookPath
	defer func() { execLookPath = oldLookPath }()
	execLookPath = func(file string) (string, error) {
		return "", fmt.Errorf("tool not found")
	}

	pdfBytes := createMinimalPDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	bleed := 0.125
	minDPI := 300
	enforceFonts := true
	enforceCMYK := true

	input := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		BleedInches:          &bleed,
		MinDPI:               &minDPI,
		EnforceEmbeddedFonts: &enforceFonts,
		EnforceCMYK:          &enforceCMYK,
	}

	res, err := ValidatePDFPreflight(context.Background(), input)
	if err != nil {
		t.Fatalf("ValidatePDFPreflight returned unexpected error: %v", err)
	}

	if !res.Valid {
		t.Errorf("expected PDF to be valid since only warnings are returned on missing tools, got errors: %v", res.Errors)
	}

	hasFontsWarn := false
	hasImagesWarn := false

	for _, w := range res.Warnings {
		if strings.Contains(w, "pdffonts utility not found") {
			hasFontsWarn = true
		}
		if strings.Contains(w, "pdfimages utility not found") {
			hasImagesWarn = true
		}
	}

	if !hasFontsWarn {
		t.Error("expected warning for missing pdffonts")
	}
	if !hasImagesWarn {
		t.Error("expected warning for missing pdfimages")
	}
}

func TestValidatePDFPreflight_SizeMismatchBetweenPages(t *testing.T) {
	defer mockLookPathSuccess()()
	pdfBytes := createMultipageMismatchPDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test_mismatch.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "")
	}

	bleed := 0.125
	minDPI := 300

	input := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		BleedInches:          &bleed,
		MinDPI:               &minDPI,
	}

	res, err := ValidatePDFPreflight(context.Background(), input)
	if err != nil {
		t.Fatalf("ValidatePDFPreflight returned unexpected error: %v", err)
	}

	if res.Valid {
		t.Error("expected invalid PDF due to mismatch of sizes between pages 1 and 2")
	}

	foundWidthMismatch := false
	foundHeightMismatch := false
	for _, e := range res.Errors {
		if strings.Contains(e, "width (500.00 pt) does not match page 1 width (432.00 pt)") {
			foundWidthMismatch = true
		}
		if strings.Contains(e, "height (700.00 pt) does not match page 1 height (648.00 pt)") {
			foundHeightMismatch = true
		}
	}

	if !foundWidthMismatch {
		t.Error("expected page 2 width mismatch error")
	}
	if !foundHeightMismatch {
		t.Error("expected page 2 height mismatch error")
	}
}

func TestValidatePDFPreflight_InvalidPath(t *testing.T) {
	bleed := 0.125
	minDPI := 300

	input := ValidatePDFInput{
		PDFPath:              "nonexistent_file.pdf",
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		BleedInches:          &bleed,
		MinDPI:               &minDPI,
	}

	_, err := ValidatePDFPreflight(context.Background(), input)
	if err == nil {
		t.Error("expected error for nonexistent PDF file path")
	}
}

func TestValidatePDFPreflight_ToolErrors(t *testing.T) {
	defer mockLookPathSuccess()()
	pdfBytes := createMinimalPDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		// Mock tool execution exit failures
		return exec.CommandContext(ctx, "false")
	}

	bleed := 0.125
	minDPI := 300

	input := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		BleedInches:          &bleed,
		MinDPI:               &minDPI,
	}

	res, err := ValidatePDFPreflight(context.Background(), input)
	if err != nil {
		t.Fatalf("ValidatePDFPreflight returned unexpected error: %v", err)
	}

	if res.Valid {
		t.Error("expected invalid PDF due to tool execution failures")
	}

	hasFontsErr := false
	hasImagesErr := false

	for _, e := range res.Errors {
		if strings.Contains(e, "Failed to run pdffonts") {
			hasFontsErr = true
		}
		if strings.Contains(e, "Failed to run pdfimages") {
			hasImagesErr = true
		}
	}

	if !hasFontsErr {
		t.Error("expected pdffonts execution failure error")
	}
	if !hasImagesErr {
		t.Error("expected pdfimages execution failure error")
	}
}

func TestParsePDFImagesOutput_UnparseableDPI(t *testing.T) {
	var res ValidatePDFResult

	// Simulate output with non-integer values for DPI columns
	out := "page   num  type   width height color comp bpc  enc interp  object ID x-dpi y-dpi   size ratio\n--------------------------------------------------------------------------------------------\n   1     0 image    1200   1800 cmyk     4   8  jpeg   no        12  0   abc   xyz  84.5K  1.3%\n"
	parsePDFImagesOutput(out, 300, false, false, &res)

	if len(res.Warnings) != 1 {
		t.Fatalf("expected 1 warning for unparseable DPI, got: %v", res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "has unparseable DPI resolution: abc x xyz") {
		t.Errorf("unexpected warning: %s", res.Warnings[0])
	}
}

func createCropBoxPDFBytes() []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /CropBox [0 0 432 648] >>\nendobj\n"

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

func TestValidatePDFPreflight_CropBox(t *testing.T) {
	defer mockLookPathSuccess()()
	pdfBytes := createCropBoxPDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "cropbox.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "")
	}

	input := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
	}

	res, err := ValidatePDFPreflight(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Errorf("expected cropbox validation to succeed, got errors: %v", res.Errors)
	}
}

func TestValidatePDFPreflight_NoBleed(t *testing.T) {
	defer mockLookPathSuccess()()
	pdfBytes := createMinimalPDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "")
	}

	bleed := 0.0
	enforceFonts := false
	enforceCMYK := false

	input := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		BleedInches:          &bleed,
		EnforceEmbeddedFonts: &enforceFonts,
		EnforceCMYK:          &enforceCMYK,
	}

	res, err := ValidatePDFPreflight(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Errorf("expected no-bleed validation to succeed, got errors: %v", res.Errors)
	}
}

func TestValidator_EdgeCases(t *testing.T) {
	defer mockLookPathSuccess()()
	// 1. Very small PDF size (<10 bytes) in ValidatePDFPreflight
	tempDir := t.TempDir()
	tinyPath := filepath.Join(tempDir, "tiny.pdf")
	_ = os.WriteFile(tinyPath, []byte("PDF"), 0600)

	bleed := 0.125
	_, err := ValidatePDFPreflight(context.Background(), ValidatePDFInput{
		PDFPath:              tinyPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		BleedInches:          &bleed,
	})
	if err == nil {
		t.Error("expected error for tiny PDF")
	}

	// 2. pdffonts output parse with missing/custom headers to trigger fallbacks
	var res1 ValidatePDFResult
	pdffontsOutputMissingHeaders := "some random headers here\n-------------------------\nHelvetica                            Type 1            Standard         no  no  no       8  0"
	parsePDFFontsOutput(pdffontsOutputMissingHeaders, true, &res1)
	if len(res1.Errors) != 1 {
		t.Errorf("expected 1 font error, got %d", len(res1.Errors))
	}

	// 3. pdfimages output parse with missing/custom headers to trigger fallbacks
	var res2 ValidatePDFResult
	// We pad the line with spaces so the fallback indices line up:
	// xDpiStart = 68 (width 6), yDpiStart = 74 (width 6), sizeStart = 80
	// 68 spaces + "150   150   84.5K"
	pdfimagesOutputMissingHeaders := "some random headers for images\n-------------------------\n" +
		"1 0 image 100 100 rgb 3 8 jpeg no 12 0 150 150 100B 1%"
	parsePDFImagesOutput(pdfimagesOutputMissingHeaders, 300, true, false, &res2)
	if len(res2.Errors) < 1 {
		t.Error("expected at least 1 image error for low DPI fallback")
	}

	// 4. pdfimages line shorter than xDpiStart
	var res3 ValidatePDFResult
	pdfimagesShortLine := "page   num  type   width height color comp bpc  enc interp  object ID x-dpi y-dpi   size ratio\n---------------------------\nshort line"
	parsePDFImagesOutput(pdfimagesShortLine, 300, false, false, &res3)
	if len(res3.Errors) != 0 || len(res3.Warnings) != 0 {
		t.Error("expected short line to be skipped with no errors/warnings")
	}
}

type staticErrorReaderAt struct {
	err error
}

func (s *staticErrorReaderAt) ReadAt(p []byte, off int64) (int, error) {
	return 0, s.err
}

func TestSafeReaderAt_ErrorPaths(t *testing.T) {
	// 1. Error before insert index
	r1 := &staticErrorReaderAt{err: fmt.Errorf("before error")}
	s1 := &safeReaderAt{r: r1, size: 14, insertIdx: 10}
	buf := make([]byte, 8)
	_, err := s1.ReadAt(buf, 0)
	if err == nil || !strings.Contains(err.Error(), "before error") {
		t.Errorf("expected before error, got: %v", err)
	}

	// 2. Error after insert index
	r2 := &staticErrorReaderAt{err: fmt.Errorf("after error")}
	s2 := &safeReaderAt{r: r2, size: 14, insertIdx: 10}
	buf2 := make([]byte, 5)
	_, err = s2.ReadAt(buf2, 11)
	if err == nil || !strings.Contains(err.Error(), "after error") {
		t.Errorf("expected after error, got: %v", err)
	}
}

func TestPDFFonts_ShortLine(t *testing.T) {
	var res ValidatePDFResult
	pdffontsShortLine := "name                                 type              encoding         emb sub uni object ID\n-------------------------\nshort"
	parsePDFFontsOutput(pdffontsShortLine, true, &res)
	if len(res.Errors) != 0 {
		t.Errorf("expected short line to be skipped, got errors: %v", res.Errors)
	}
}

func assertHasError(t *testing.T, errors []string, substr string) {
	t.Helper()
	for _, e := range errors {
		if strings.Contains(e, substr) {
			return
		}
	}
	t.Errorf("expected error containing %q, but got none. Errors: %v", substr, errors)
}

func mockLookPathSuccess() func() {
	oldLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		return "/mocked/path/to/" + file, nil
	}
	return func() {
		execLookPath = oldLookPath
	}
}

func createPDFWithContentsBytes(contentsStr string) []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] /Contents 4 0 R >>\nendobj\n"
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

	return []byte(header + obj1 + obj2 + obj3 + obj4 + xref + trailer)
}

func TestValidatePDFPreflight_EnforceGrayscale(t *testing.T) {
	defer mockLookPathSuccess()()
	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if strings.Contains(command, "pdffonts") {
			return exec.CommandContext(ctx, "echo", "")
		}
		if strings.Contains(command, "pdfimages") {
			// Mocking images: one is gray, one is rgb
			out := "page   num  type   width height color comp bpc  enc interp  object ID x-dpi y-dpi   size ratio\n" +
				"--------------------------------------------------------------------------------------------\n" +
				"   1     0 image    100    100 gray     1   8  jpeg   no        12  0   300   300  10K   10%\n" +
				"   1     1 image    100    100 rgb      3   8  jpeg   no        13  0   300   300  10K   10%\n"
			return exec.CommandContext(ctx, "echo", out)
		}
		return exec.CommandContext(ctx, "echo", "")
	}

	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	if err := os.WriteFile(pdfPath, createMinimalPDFBytes(), 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	enforceGrayscale := true
	input := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		EnforceGrayscale:     &enforceGrayscale,
	}

	res, err := ValidatePDFPreflight(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Valid {
		t.Error("expected PDF to be invalid because it contains an RGB image")
	}
	assertHasError(t, res.Errors, "has non-grayscale color space (rgb)")
}

func TestValidatePDFPreflight_EnforceGrayscale_Operators(t *testing.T) {
	defer mockLookPathSuccess()()
	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	// Mock pdfimages returning only gray images so image check passes
	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		if strings.Contains(command, "pdfimages") {
			out := "page   num  type   width height color comp bpc  enc interp  object ID x-dpi y-dpi   size ratio\n" +
				"--------------------------------------------------------------------------------------------\n" +
				"   1     0 image    100    100 gray     1   8  jpeg   no        12  0   300   300  10K   10%\n"
			return exec.CommandContext(ctx, "echo", out)
		}
		return exec.CommandContext(ctx, "echo", "")
	}

	tests := []struct {
		name        string
		contents    string
		expectError string
	}{
		{
			name:        "valid gray operator",
			contents:    "0.5 g\n0.2 G\n",
			expectError: "",
		},
		{
			name:        "invalid rg operator",
			contents:    "1 0 0 rg\n",
			expectError: "RGB vector/text color setting operator (rg)",
		},
		{
			name:        "invalid RG operator",
			contents:    "0.5 0.5 0.5 RG\n",
			expectError: "RGB vector/text color setting operator (RG)",
		},
		{
			name:        "invalid k operator",
			contents:    "0 0 0 1 k\n",
			expectError: "CMYK vector/text color setting operator (k)",
		},
		{
			name:        "invalid cs colorspace",
			contents:    "/DeviceRGB cs\n",
			expectError: "sets non-grayscale color space (DeviceRGB)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			pdfPath := filepath.Join(tempDir, "test.pdf")
			pdfBytes := createPDFWithContentsBytes(tt.contents)
			if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
				t.Fatalf("failed to write temp PDF: %v", err)
			}

			enforceGrayscale := true
			input := ValidatePDFInput{
				PDFPath:              pdfPath,
				ExpectedWidthInches:  6.0,
				ExpectedHeightInches: 9.0,
				EnforceGrayscale:     &enforceGrayscale,
			}

			res, err := ValidatePDFPreflight(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectError != "" {
				if res.Valid {
					t.Errorf("expected PDF to be invalid, contents: %q", tt.contents)
				}
				assertHasError(t, res.Errors, tt.expectError)
			} else if !res.Valid {
				t.Errorf("expected PDF to be valid, got errors: %v", res.Errors)
			}
		})
	}
}

func createComplexColorspacePDFBytes() []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"

	// Page with resources /ColorSpace
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] /Contents 4 0 R\n" +
		"  /Resources <<\n" +
		"    /ColorSpace <<\n" +
		"      /CS1 /DeviceRGB\n" +
		"      /CS2 [ /ICCBased 5 0 R ]\n" +
		"      /CS3 [ /Indexed /DeviceRGB 15 7 0 R ]\n" +
		"      /CS4 [ /Indexed [ /ICCBased 5 0 R ] 15 7 0 R ]\n" +
		"      /CS5 /DeviceGray\n" +
		"      /CS6 [ /DeviceGray ]\n" +
		"      /CS7 [ /CalGray ]\n" +
		"      /CS8 [ /DeviceCMYK ]\n" +
		"      /CS9 [ /CalRGB ]\n" +
		"      /CS10 [ /Lab ]\n" +
		"      /CS11 [ /ICCBased 6 0 R ]\n" +
		"    >>\n" +
		"  >>\n" +
		">> \nendobj\n"

	// Contents stream
	obj4 := "4 0 obj\n<< /Length 1 >>\nstream\n \nendstream\nendobj\n"

	// ICCBased color space stream with N = 3 (RGB)
	obj5 := "5 0 obj\n<< /Length 0 /N 3 >>\nstream\n\nendstream\nendobj\n"

	// ICCBased color space stream with N = 1 (Gray)
	obj6 := "6 0 obj\n<< /Length 0 /N 1 >>\nstream\n\nendstream\nendobj\n"

	// Indexed lookup table string
	obj7 := "7 0 obj\n(some_lookup_data)\nendobj\n"

	header := "%PDF-1.4\n"
	off1 := len(header)
	off2 := off1 + len(obj1)
	off3 := off2 + len(obj2)
	off4 := off3 + len(obj3)
	off5 := off4 + len(obj4)
	off6 := off5 + len(obj5)
	off7 := off6 + len(obj6)
	offXref := off7 + len(obj7)

	xref := "xref\n0 8\n0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", off1) +
		fmt.Sprintf("%010d 00000 n \n", off2) +
		fmt.Sprintf("%010d 00000 n \n", off3) +
		fmt.Sprintf("%010d 00000 n \n", off4) +
		fmt.Sprintf("%010d 00000 n \n", off5) +
		fmt.Sprintf("%010d 00000 n \n", off6) +
		fmt.Sprintf("%010d 00000 n \n", off7)

	trailer := fmt.Sprintf("trailer\n<< /Size 8 /Root 1 0 R >>\nstartxref\n%d\n%%EOF\n", offXref)

	return []byte(header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6 + obj7 + xref + trailer)
}

func createPDFWithMultiContentsBytes(contentsStr1, contentsStr2 string) []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] /Contents [4 0 R 5 0 R] >>\nendobj\n"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(contentsStr1), contentsStr1)
	obj5 := fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(contentsStr2), contentsStr2)

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

	trailer := fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%EOF\n", offXref)

	return []byte(header + obj1 + obj2 + obj3 + obj4 + obj5 + xref + trailer)
}

func TestIsNonGrayColorspace_Detailed(t *testing.T) {
	pdfBytes := createComplexColorspacePDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "complex.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	//nolint:gosec // pdfPath is constructed using t.TempDir() in tests
	f, err := os.Open(pdfPath)
	if err != nil {
		t.Fatalf("failed to open PDF: %v", err)
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat PDF: %v", err)
	}

	safeR, safeSize := newSafeReaderAt(f, fi.Size())
	r, err := pdf.NewReader(safeR, safeSize)
	if err != nil {
		t.Fatalf("failed to parse PDF: %v", err)
	}

	p := r.Page(1)

	// Test direct names
	if !isNonGrayColorspace("DeviceRGB", p) {
		t.Error("expected DeviceRGB to be non-gray")
	}
	if !isNonGrayColorspace("DeviceCMYK", p) {
		t.Error("expected DeviceCMYK to be non-gray")
	}
	if isNonGrayColorspace("DeviceGray", p) {
		t.Error("expected DeviceGray to be gray")
	}
	if isNonGrayColorspace("CalGray", p) {
		t.Error("expected CalGray to be gray")
	}

	// Test resolved resource color spaces
	if !isNonGrayColorspace("CS1", p) {
		t.Error("expected CS1 (DeviceRGB) to be non-gray")
	}
	if !isNonGrayColorspace("CS2", p) {
		t.Error("expected CS2 (ICCBased N=3) to be non-gray")
	}
	if !isNonGrayColorspace("CS3", p) {
		t.Error("expected CS3 (Indexed DeviceRGB) to be non-gray")
	}
	if !isNonGrayColorspace("CS4", p) {
		t.Error("expected CS4 (Indexed ICCBased N=3) to be non-gray")
	}
	if isNonGrayColorspace("CS5", p) {
		t.Error("expected CS5 (DeviceGray name) to be gray")
	}
	if isNonGrayColorspace("CS6", p) {
		t.Error("expected CS6 (DeviceGray array) to be gray")
	}
	if isNonGrayColorspace("CS7", p) {
		t.Error("expected CS7 (CalGray array) to be gray")
	}
	if !isNonGrayColorspace("CS8", p) {
		t.Error("expected CS8 (DeviceCMYK array) to be non-gray")
	}
	if !isNonGrayColorspace("CS9", p) {
		t.Error("expected CS9 (CalRGB array) to be non-gray")
	}
	if !isNonGrayColorspace("CS10", p) {
		t.Error("expected CS10 (Lab array) to be non-gray")
	}
	if isNonGrayColorspace("CS11", p) {
		t.Error("expected CS11 (ICCBased N=1) to be gray")
	}

	// Nonexistent CS
	if isNonGrayColorspace("CS_NONEXISTENT", p) {
		t.Error("expected CS_NONEXISTENT to be gray/fallback false")
	}
}

func TestGetPageContentsStreams_Array(t *testing.T) {
	pdfBytes := createPDFWithMultiContentsBytes("1 0 0 rg\n", "0 g\n")
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "multi_contents.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	//nolint:gosec // pdfPath is constructed using t.TempDir() in tests
	f, err := os.Open(pdfPath)
	if err != nil {
		t.Fatalf("failed to open PDF: %v", err)
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat PDF: %v", err)
	}

	safeR, safeSize := newSafeReaderAt(f, fi.Size())
	r, err := pdf.NewReader(safeR, safeSize)
	if err != nil {
		t.Fatalf("failed to parse PDF: %v", err)
	}

	p := r.Page(1)
	streams := getPageContentsStreams(p)
	if len(streams) != 2 {
		t.Errorf("expected 2 streams, got %d", len(streams))
	}
}

func createMultipagePDFWithTextBytes(page1Contents, page2Contents string) []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>\nendobj\n"
	// Page 1 (Right page, odd)
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] /Contents 5 0 R\n" +
		"  /Resources <<\n" +
		"    /Font <<\n" +
		"      /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\n" +
		"    >>\n" +
		"  >>\n" +
		">> \nendobj\n"
	// Page 2 (Left page, even)
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] /Contents 6 0 R\n" +
		"  /Resources <<\n" +
		"    /Font <<\n" +
		"      /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\n" +
		"    >>\n" +
		"  >>\n" +
		">> \nendobj\n"
	// Contents Page 1
	obj5 := fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(page1Contents), page1Contents)
	// Contents Page 2
	obj6 := fmt.Sprintf("6 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(page2Contents), page2Contents)

	header := "%PDF-1.4\n"
	off1 := len(header)
	off2 := off1 + len(obj1)
	off3 := off2 + len(obj2)
	off4 := off3 + len(obj3)
	off5 := off4 + len(obj4)
	off6 := off5 + len(obj5)
	offXref := off6 + len(obj6)

	xref := "xref\n0 7\n0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", off1) +
		fmt.Sprintf("%010d 00000 n \n", off2) +
		fmt.Sprintf("%010d 00000 n \n", off3) +
		fmt.Sprintf("%010d 00000 n \n", off4) +
		fmt.Sprintf("%010d 00000 n \n", off5) +
		fmt.Sprintf("%010d 00000 n \n", off6)

	trailer := fmt.Sprintf("trailer\n<< /Size 7 /Root 1 0 R >>\nstartxref\n%d\n%%EOF\n", offXref)

	return []byte(header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6 + xref + trailer)
}

func TestValidatePDFPreflight_Margins(t *testing.T) {
	defer mockLookPathSuccess()()
	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "echo", "")
	}

	tests := []struct {
		name          string
		page1Contents string
		page2Contents string
		minGutter     *float64
		minMargin     *float64
		expectErrors  []string
	}{
		{
			name:          "perfectly safe margins",
			page1Contents: "BT /F1 10 Tf 1 0 0 1 100 100 Tm (Safe Text) Tj ET\n",
			page2Contents: "BT /F1 10 Tf 1 0 0 1 100 100 Tm (Safe Text) Tj ET\n",
			minGutter:     floatPtr(0.5),  // 36pt
			minMargin:     floatPtr(0.25), // 18pt
			expectErrors:  nil,
		},
		{
			name:          "bottom margin violation page 1",
			page1Contents: "BT /F1 10 Tf 1 0 0 1 100 10 Tm (Bottom Violator) Tj ET\n",
			page2Contents: "BT /F1 10 Tf 1 0 0 1 100 100 Tm (Safe Text) Tj ET\n",
			minGutter:     floatPtr(0.5),
			minMargin:     floatPtr(0.25),
			expectErrors:  []string{"Page 1 text \"B\" at Y=10.00 pt is within bottom margin"},
		},
		{
			name:          "top margin violation page 2",
			page1Contents: "BT /F1 10 Tf 1 0 0 1 100 100 Tm (Safe Text) Tj ET\n",
			page2Contents: "BT /F1 10 Tf 1 0 0 1 100 640 Tm (Top Violator) Tj ET\n",
			minGutter:     floatPtr(0.5),
			minMargin:     floatPtr(0.25),
			expectErrors:  []string{"Page 2 text \"T\" at Y=650.00 pt is within top margin"}, // Y=640 + 10pt FontSize
		},
		{
			name:          "right page inside gutter violation (left edge)",
			page1Contents: "BT /F1 10 Tf 1 0 0 1 10 100 Tm (Gutter Violator) Tj ET\n",
			page2Contents: "BT /F1 10 Tf 1 0 0 1 100 100 Tm (Safe Text) Tj ET\n",
			minGutter:     floatPtr(0.5),
			minMargin:     floatPtr(0.25),
			expectErrors:  []string{"Page 1 text \"G\" at X=10.00 pt is within inside gutter"},
		},
		{
			name:          "right page outer margin violation (right edge)",
			page1Contents: "BT /F1 10 Tf 1 0 0 1 420 100 Tm (Outer Violator) Tj ET\n",
			page2Contents: "BT /F1 10 Tf 1 0 0 1 100 100 Tm (Safe Text) Tj ET\n",
			minGutter:     floatPtr(0.5),
			minMargin:     floatPtr(0.25),
			expectErrors:  []string{"Page 1 text \"O\" at X=420.00 pt is within outer margin"},
		},
		{
			name:          "left page inside gutter violation (right edge)",
			page1Contents: "BT /F1 10 Tf 1 0 0 1 100 100 Tm (Safe Text) Tj ET\n",
			page2Contents: "BT /F1 10 Tf 1 0 0 1 410 100 Tm (Gutter Violator) Tj ET\n",
			minGutter:     floatPtr(0.5),
			minMargin:     floatPtr(0.25),
			expectErrors:  []string{"Page 2 text \"G\" at X=410.00 pt is within inside gutter"},
		},
		{
			name:          "left page outer margin violation (left edge)",
			page1Contents: "BT /F1 10 Tf 1 0 0 1 100 100 Tm (Safe Text) Tj ET\n",
			page2Contents: "BT /F1 10 Tf 1 0 0 1 10 100 Tm (Outer Violator) Tj ET\n",
			minGutter:     floatPtr(0.5),
			minMargin:     floatPtr(0.25),
			expectErrors:  []string{"Page 2 text \"O\" at X=10.00 pt is within outer margin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			pdfPath := filepath.Join(tempDir, "test.pdf")
			pdfBytes := createMultipagePDFWithTextBytes(tt.page1Contents, tt.page2Contents)
			if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
				t.Fatalf("failed to write temp PDF: %v", err)
			}

			input := ValidatePDFInput{
				PDFPath:              pdfPath,
				ExpectedWidthInches:  6.0,
				ExpectedHeightInches: 9.0,
				MinGutterInches:      tt.minGutter,
				MinMarginInches:      tt.minMargin,
			}

			res, err := ValidatePDFPreflight(context.Background(), input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(tt.expectErrors) > 0 {
				assertMarginErrors(t, res, tt.expectErrors)
			} else if !res.Valid {
				t.Errorf("expected PDF to be valid, got errors: %v", res.Errors)
			}
		})
	}
}

func assertMarginErrors(t *testing.T, res *ValidatePDFResult, expectedErrors []string) {
	t.Helper()
	if res.Valid {
		t.Error("expected PDF to be invalid, but got valid")
	}
	for _, expectedSubstr := range expectedErrors {
		assertHasError(t, res.Errors, expectedSubstr)
	}
}

func floatPtr(f float64) *float64 {
	return &f
}

func TestValidatePDFPreflight_NegativeMargins(t *testing.T) {
	pdfBytes := createMinimalPDFBytes()
	tempDir := t.TempDir()
	pdfPath := filepath.Join(tempDir, "test.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	negVal := -0.5

	inputGutter := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		MinGutterInches:      &negVal,
	}
	_, err := ValidatePDFPreflight(context.Background(), inputGutter)
	if err == nil || !strings.Contains(err.Error(), "min_gutter_inches must be non-negative") {
		t.Errorf("expected error for negative min_gutter_inches, got: %v", err)
	}

	inputMargin := ValidatePDFInput{
		PDFPath:              pdfPath,
		ExpectedWidthInches:  6.0,
		ExpectedHeightInches: 9.0,
		MinMarginInches:      &negVal,
	}
	_, err = ValidatePDFPreflight(context.Background(), inputMargin)
	if err == nil || !strings.Contains(err.Error(), "min_margin_inches must be non-negative") {
		t.Errorf("expected error for negative min_margin_inches, got: %v", err)
	}
}
