package pdfcheck

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	// Temporarily clear PATH so that LookPath fails to find pdffonts and pdfimages
	t.Setenv("PATH", "")

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
	parsePDFImagesOutput(out, 300, false, &res)

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
	parsePDFImagesOutput(pdfimagesOutputMissingHeaders, 300, true, &res2)
	if len(res2.Errors) < 1 {
		t.Error("expected at least 1 image error for low DPI fallback")
	}

	// 4. pdfimages line shorter than xDpiStart
	var res3 ValidatePDFResult
	pdfimagesShortLine := "page   num  type   width height color comp bpc  enc interp  object ID x-dpi y-dpi   size ratio\n---------------------------\nshort line"
	parsePDFImagesOutput(pdfimagesShortLine, 300, false, &res3)
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
