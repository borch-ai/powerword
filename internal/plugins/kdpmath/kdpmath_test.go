package kdpmath

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTrimSize(t *testing.T) {
	tests := []struct {
		name      string
		trim      string
		wantW     float64
		wantH     float64
		expectErr bool
	}{
		{"Standard 6x9", "6x9", 6.0, 9.0, false},
		{"Standard 6X9", "6X9", 6.0, 9.0, false},
		{"Standard 5.5x8.5", "5.5x8.5", 5.5, 8.5, false},
		{"Standard 8.5x11", "8.5x11", 8.5, 11.0, false},
		{"Spaces in trim", " 6 x 9 ", 6.0, 9.0, false},
		{"Dashes in trim", "6-9", 6.0, 9.0, false},
		{"Asterisk in trim", "6*9", 6.0, 9.0, false},
		{"Invalid format", "6", 0, 0, true},
		{"Invalid format too many parts", "6x9x1", 0, 0, true},
		{"Invalid width float", "abc*9", 0, 0, true},
		{"Invalid height float", "6*abc", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, err := parseTrimSize(tt.trim)
			if (err != nil) != tt.expectErr {
				t.Fatalf("parseTrimSize(%q) error = %v, expectErr = %v", tt.trim, err, tt.expectErr)
			}
			if !tt.expectErr {
				if w != tt.wantW || h != tt.wantH {
					t.Errorf("parseTrimSize(%q) = (%v, %v), want (%v, %v)", tt.trim, w, h, tt.wantW, tt.wantH)
				}
			}
		})
	}
}

func TestGetMultiplier(t *testing.T) {
	tests := []struct {
		name      string
		paper     string
		want      float64
		expectErr bool
	}{
		{"White lower", "white", MulWhite, false},
		{"White uppercase", "WHITE", MulWhite, false},
		{"White long name", "black & white on white paper", MulWhite, false},
		{"Cream lower", "cream", MulCream, false},
		{"Cream long name", "black & white on cream paper", MulCream, false},
		{"Premium color", "premium_color", MulPremiumColor, false},
		{"Premium color long", "premium color on white paper", MulPremiumColor, false},
		{"Standard color", "standard_color", MulStandardColor, false},
		{"Standard color long", "standard color on white paper", MulStandardColor, false},
		{"Invalid paper type", "glossy", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getMultiplier(tt.paper)
			if (err != nil) != tt.expectErr {
				t.Fatalf("getMultiplier(%q) error = %v, expectErr = %v", tt.paper, err, tt.expectErr)
			}
			if !tt.expectErr && got != tt.want {
				t.Errorf("getMultiplier(%q) = %v, want %v", tt.paper, got, tt.want)
			}
		})
	}
}

func TestCalculateGeometry_InvalidInput(t *testing.T) {
	if _, err := CalculateGeometry(0, "paperback", "white", "6x9"); err == nil {
		t.Error("expected error for 0 page count")
	}
	if _, err := CalculateGeometry(100, "paperback", "glossy", "6x9"); err == nil {
		t.Error("expected error for invalid paper type")
	}
	if _, err := CalculateGeometry(100, "paperback", "white", "invalid"); err == nil {
		t.Error("expected error for invalid trim size")
	}
	if _, err := CalculateGeometry(100, "magazine", "white", "6x9"); err == nil {
		t.Error("expected error for unsupported binding type")
	}
}

func TestCalculateGeometry_Paperback(t *testing.T) {
	res, err := CalculateGeometry(100, "paperback", "white", "6x9")
	if err != nil {
		t.Fatalf("failed to calculate geometry: %v", err)
	}

	// Spine = 100 * 0.002252 + 0.06 = 0.2852
	expectedSpine := 100*MulWhite + 0.06
	if math.Abs(res.SpineWidthInches-expectedSpine) > 1e-9 {
		t.Errorf("spine width = %v, want %v", res.SpineWidthInches, expectedSpine)
	}

	// Width = 2 * 6.0 + spine + 0.25 = 12.5352
	expectedWidth := 12.0 + expectedSpine + 0.25
	if math.Abs(res.CoverWidthInches-expectedWidth) > 1e-9 {
		t.Errorf("cover width = %v, want %v", res.CoverWidthInches, expectedWidth)
	}

	// Height = 9.0 + 0.25 = 9.25
	if res.CoverHeightInches != 9.25 {
		t.Errorf("cover height = %v, want 9.25", res.CoverHeightInches)
	}

	if !res.SpineTextEligible {
		t.Error("expected spine text eligible for 100 pages")
	}

	if len(res.Guides) != 5 {
		t.Errorf("expected 5 layout guides for paperback, got %d", len(res.Guides))
	}
}

func TestCalculateGeometry_Hardcover(t *testing.T) {
	res, err := CalculateGeometry(100, "hardcover", "premium_color", "6x9")
	if err != nil {
		t.Fatalf("failed to calculate geometry: %v", err)
	}

	// Spine = 100 * 0.002347 = 0.2347 (no 0.06" allowance)
	expectedSpine := 100 * MulPremiumColor
	if math.Abs(res.SpineWidthInches-expectedSpine) > 1e-9 {
		t.Errorf("spine width = %v, want %v", res.SpineWidthInches, expectedSpine)
	}

	// Width = 2 * 6.0 + spine + 0.394 + 1.182 = 13.576 + spine = 13.8107
	expectedWidth := 12.0 + expectedSpine + 0.394 + 1.182
	if math.Abs(res.CoverWidthInches-expectedWidth) > 1e-9 {
		t.Errorf("cover width = %v, want %v", res.CoverWidthInches, expectedWidth)
	}

	// Height = 9.0 + 0.236 + 1.182 = 10.418
	expectedHeight := 9.0 + 0.236 + 1.182
	if math.Abs(res.CoverHeightInches-expectedHeight) > 1e-9 {
		t.Errorf("cover height = %v, want %v", res.CoverHeightInches, expectedHeight)
	}

	if len(res.Guides) != 7 {
		t.Errorf("expected 7 layout guides for hardcover, got %d", len(res.Guides))
	}
}

// createMinimalPDFBytes returns the exact byte array of a minimal valid PDF with 1 page of 432x648pt (6"x9").
func createMinimalPDFBytes() []byte {
	// Byte offsets must match exactly in the xref table.
	// We'll write the PDF string parts, compute offsets, and construct the final bytes.
	// Object 1: Catalog
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	// Object 2: Pages
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	// Object 3: Page with MediaBox
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

func TestValidatePDF_InvalidPath(t *testing.T) {
	_, err := ValidatePDF("nonexistent.pdf", "paperback", "white", "6x9", 100, false, false)
	if err == nil {
		t.Error("expected error for nonexistent PDF file path")
	}
}

func TestValidatePDF_Success(t *testing.T) {
	pdfBytes := createMinimalPDFBytes()
	tempDir, err := os.MkdirTemp("", "kdpmath_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	pdfPath := filepath.Join(tempDir, "test.pdf")
	err = os.WriteFile(pdfPath, pdfBytes, 0600)
	if err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	res, err := ValidatePDF(pdfPath, "paperback", "white", "6x9", 1, false, false)
	if err != nil {
		t.Fatalf("ValidatePDF returned unexpected error: %v", err)
	}
	if !res.IsValid {
		t.Errorf("expected valid PDF, got errors: %v", res.Errors)
	}
	if res.PageCount != 1 {
		t.Errorf("page count = %d, want 1", res.PageCount)
	}
	if res.WidthPoints != 432 || res.HeightPoints != 648 {
		t.Errorf("dimensions = (%v, %v), want (432, 648)", res.WidthPoints, res.HeightPoints)
	}
}

func TestValidatePDF_SizeMismatch(t *testing.T) {
	pdfBytes := createMinimalPDFBytes()
	tempDir, err := os.MkdirTemp("", "kdpmath_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	pdfPath := filepath.Join(tempDir, "test.pdf")
	err = os.WriteFile(pdfPath, pdfBytes, 0600)
	if err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	res, err := ValidatePDF(pdfPath, "paperback", "white", "6x9", 1, false, true) // hasBleed = true expects wider sizes (441x666pt)
	if err != nil {
		t.Fatalf("ValidatePDF returned unexpected error: %v", err)
	}
	if res.IsValid {
		t.Error("expected invalid PDF due to size mismatch")
	}
}

func TestValidatePDF_PageCountMismatch(t *testing.T) {
	pdfBytes := createMinimalPDFBytes()
	tempDir, err := os.MkdirTemp("", "kdpmath_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	pdfPath := filepath.Join(tempDir, "test.pdf")
	err = os.WriteFile(pdfPath, pdfBytes, 0600)
	if err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	res, err := ValidatePDF(pdfPath, "paperback", "white", "6x9", 10, false, false) // expects 10 pages, got 1
	if err != nil {
		t.Fatalf("ValidatePDF returned unexpected error: %v", err)
	}
	if res.IsValid {
		t.Error("expected invalid PDF due to page count mismatch")
	}
}

func TestGenerateManifest(t *testing.T) {
	manifest, err := GenerateManifest(100, "paperback", "white", "6x9")
	if err != nil {
		t.Fatalf("GenerateManifest returned error: %v", err)
	}

	if len(manifest) == 0 {
		t.Error("expected non-empty JSON manifest")
	}
}

type mockReaderAt struct {
	data []byte
	err  error
}

func (m *mockReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	if off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n := copy(p, m.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestSafeReaderAt_SizeTooSmall(t *testing.T) {
	r := &mockReaderAt{data: []byte("123")}
	gotR, gotSz := newSafeReaderAt(r, 3)
	if gotR != r || gotSz != 3 {
		t.Errorf("expected original reader and size, got %v, %d", gotR, gotSz)
	}
}

func TestSafeReaderAt_ReadAtError(t *testing.T) {
	r := &mockReaderAt{err: fmt.Errorf("read error")}
	gotR, gotSz := newSafeReaderAt(r, 20)
	if gotR != r || gotSz != 20 {
		t.Errorf("expected original reader on read error")
	}
}

func TestSafeReaderAt_DoublePercent(t *testing.T) {
	data := []byte("some %%EOF\n\r  ")
	r := &mockReaderAt{data: data}
	gotR, gotSz := newSafeReaderAt(r, int64(len(data)))
	if gotR != r || gotSz != int64(len(data)) {
		t.Errorf("expected original reader when %%EOF already exists")
	}
}

func TestSafeReaderAt_NoEOF(t *testing.T) {
	data := []byte("some other ending")
	r := &mockReaderAt{data: data}
	gotR, gotSz := newSafeReaderAt(r, int64(len(data)))
	if gotR != r || gotSz != int64(len(data)) {
		t.Errorf("expected original reader when %%EOF not present")
	}
}

func TestSafeReaderAt_VirtualInsertion(t *testing.T) {
	data := []byte("hello %EOF   ")
	r := &mockReaderAt{data: data}
	gotR, gotSz := newSafeReaderAt(r, int64(len(data)))
	if gotR == r {
		t.Fatal("expected wrapped reader")
	}
	expectedVirtualSize := int64(len(data) + 1)
	if gotSz != expectedVirtualSize {
		t.Errorf("expected virtual size %d, got %d", expectedVirtualSize, gotSz)
	}

	buf := make([]byte, expectedVirtualSize)
	n, err := gotR.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected read error: %v", err)
	}
	if int64(n) != expectedVirtualSize {
		t.Errorf("read %d bytes, want %d", n, expectedVirtualSize)
	}
	expectedStr := "hello %%EOF   "
	if string(buf) != expectedStr {
		t.Errorf("got content %q, want %q", string(buf), expectedStr)
	}

	_, err = gotR.ReadAt(buf, expectedVirtualSize)
	if err != io.EOF {
		t.Errorf("expected io.EOF at boundary, got %v", err)
	}

	buf2 := make([]byte, 5)
	n2, err2 := gotR.ReadAt(buf2, expectedVirtualSize-3)
	if err2 != io.EOF {
		t.Errorf("expected io.EOF, got %v, read count=%d", err2, n2)
	}
	if n2 != 3 {
		t.Errorf("expected to read 3 bytes, got %d", n2)
	}
}

func TestSafeReaderAt_LargeTrailingWhitespace(t *testing.T) {
	spaces := "                    "
	data := []byte("hello %EOF" + spaces)
	r := &mockReaderAt{data: data}
	gotR, gotSz := newSafeReaderAt(r, int64(len(data)))
	if gotR == r {
		t.Fatal("expected wrapped reader for large trailing whitespace")
	}
	expectedVirtualSize := int64(len(data) + 1)
	if gotSz != expectedVirtualSize {
		t.Errorf("expected virtual size %d, got %d", expectedVirtualSize, gotSz)
	}

	buf := make([]byte, expectedVirtualSize)
	n, err := gotR.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected read error: %v", err)
	}
	if int64(n) != expectedVirtualSize {
		t.Errorf("read %d bytes, want %d", n, expectedVirtualSize)
	}
	expectedStr := "hello %%EOF" + spaces
	if string(buf) != expectedStr {
		t.Errorf("got content %q, want %q", string(buf), expectedStr)
	}
}

func createInheritedPDFBytes() []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 432 648] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n"

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

func createMissingBoxPDFBytes() []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n"

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

func createInvalidBoxPDFBytes() []byte {
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox 432 >>\nendobj\n"

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

func TestValidatePDF_EdgeCases_Inherited(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "inherited.pdf")
	if err := os.WriteFile(p, createInheritedPDFBytes(), 0600); err != nil {
		t.Fatalf("failed to write inherited.pdf: %v", err)
	}

	res, err := ValidatePDF(p, "paperback", "white", "6x9", 1, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsValid {
		t.Errorf("expected valid PDF, got errors: %v", res.Errors)
	}
}

func TestValidatePDF_EdgeCases_MissingBox(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "missingBox.pdf")
	if err := os.WriteFile(p, createMissingBoxPDFBytes(), 0600); err != nil {
		t.Fatalf("failed to write missingBox.pdf: %v", err)
	}

	res, err := ValidatePDF(p, "paperback", "white", "6x9", 1, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsValid {
		t.Error("expected invalid PDF due to missing boxes")
	}
}

func TestValidatePDF_EdgeCases_InvalidBox(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "invalidBox.pdf")
	if err := os.WriteFile(p, createInvalidBoxPDFBytes(), 0600); err != nil {
		t.Fatalf("failed to write invalidBox.pdf: %v", err)
	}

	res, err := ValidatePDF(p, "paperback", "white", "6x9", 1, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsValid {
		t.Error("expected invalid PDF due to non-array box")
	}
}

func TestValidatePDF_EdgeCases_CoverPageCount(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "multipage.pdf")

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 432 648] >>\nendobj\n"

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

	if err := os.WriteFile(p, []byte(header+obj1+obj2+obj3+obj4+xref+trailer), 0600); err != nil {
		t.Fatalf("failed to write multipage.pdf: %v", err)
	}

	res, err := ValidatePDF(p, "paperback", "white", "6x9", 1, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsValid {
		t.Error("expected cover validation to fail for multi-page PDF")
	}
	foundPageCountErr := false
	for _, e := range res.Errors {
		if strings.Contains(e, "expected exactly 1 page") {
			foundPageCountErr = true
		}
	}
	if !foundPageCountErr {
		t.Errorf("expected error about page count, got errors: %v", res.Errors)
	}
}

func TestValidatePDF_EdgeCases_CoverGeometryError(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "inherited.pdf")
	if err := os.WriteFile(p, createInheritedPDFBytes(), 0600); err != nil {
		t.Fatalf("failed to write inherited.pdf: %v", err)
	}

	_, err := ValidatePDF(p, "paperback", "invalid_paper", "6x9", 1, true, false)
	if err == nil {
		t.Error("expected error due to invalid paper type")
	}
}

func TestValidatePDF_EdgeCases_InteriorTrimSizeError(t *testing.T) {
	tempDir := t.TempDir()
	p := filepath.Join(tempDir, "inherited.pdf")
	if err := os.WriteFile(p, createInheritedPDFBytes(), 0600); err != nil {
		t.Fatalf("failed to write inherited.pdf: %v", err)
	}

	_, err := ValidatePDF(p, "paperback", "white", "invalid_trim", 1, false, false)
	if err == nil {
		t.Error("expected error due to invalid trim size")
	}
}

func TestGenerateManifest_Error(t *testing.T) {
	_, err := GenerateManifest(100, "paperback", "invalid_paper", "6x9")
	if err == nil {
		t.Error("expected error for invalid paper type")
	}
}
