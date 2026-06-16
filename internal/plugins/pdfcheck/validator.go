package pdfcheck

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"rsc.io/pdf"
)

// ValidatePDFInput represents the input schema for the validate_pdf tool.
type ValidatePDFInput struct {
	PDFPath              string   `json:"pdf_path"`
	ExpectedWidthInches  float64  `json:"expected_width_inches"`
	ExpectedHeightInches float64  `json:"expected_height_inches"`
	BleedInches          *float64 `json:"bleed_inches,omitempty"`           // default: 0.125
	MinDPI               *int     `json:"min_dpi,omitempty"`                // default: 300
	EnforceEmbeddedFonts *bool    `json:"enforce_embedded_fonts,omitempty"` // default: true
	EnforceCMYK          *bool    `json:"enforce_cmyk,omitempty"`           // default: false
}

// ValidatePDFResult represents the JSON response output for the validate_pdf tool.
type ValidatePDFResult struct {
	Valid      bool     `json:"valid"`
	PageCount  int      `json:"page_count"`
	Dimensions string   `json:"dimensions"`
	Errors     []string `json:"errors"`
	Warnings   []string `json:"warnings"`
}

// Package-level function variable to allow unit-test mocking of external subprocess runs.
var execCommand = exec.CommandContext

// findInherited walks the Page dictionary tree ancestors via 'Parent' nodes to resolve inherited values.
func findInherited(v pdf.Value, key string) pdf.Value {
	for !v.IsNull() {
		if val := v.Key(key); !val.IsNull() {
			return val
		}
		v = v.Key("Parent")
	}
	return pdf.Value{}
}

// getPageDimensions extracts structural bounding dimensions in points.
func getPageDimensions(page pdf.Page) (float64, float64, error) {
	boxVal := findInherited(page.V, "MediaBox")
	if boxVal.IsNull() {
		boxVal = findInherited(page.V, "CropBox")
	}
	if boxVal.IsNull() || boxVal.Kind() != pdf.Array || boxVal.Len() != 4 {
		return 0, 0, fmt.Errorf("unable to find valid MediaBox or CropBox array")
	}

	x1 := boxVal.Index(0).Float64()
	y1 := boxVal.Index(1).Float64()
	x2 := boxVal.Index(2).Float64()
	y2 := boxVal.Index(3).Float64()

	width := math.Abs(x2 - x1)
	height := math.Abs(y2 - y1)

	return width, height, nil
}

// safeReaderAt is a wrapper that virtually inserts '%' before the first '%' of '%EOF' (to produce '%%EOF')
// to bypass a literal typo bug in rsc.io/pdf which strictly expects '%%EOF' at the end of the file.
type safeReaderAt struct {
	r         io.ReaderAt
	size      int64
	insertIdx int64
}

func newSafeReaderAt(r io.ReaderAt, size int64) (io.ReaderAt, int64) {
	bufSize := int64(1024)
	if size < bufSize {
		bufSize = size
	}
	if bufSize < 10 {
		return r, size
	}
	buf := make([]byte, bufSize)
	offset := size - bufSize
	n, err := r.ReadAt(buf, offset)
	if err != nil && n == 0 {
		return r, size
	}
	buf = buf[:n]
	trimmed := bytes.TrimRight(buf, "\r\n\t ")
	if bytes.HasSuffix(trimmed, []byte("%EOF")) && !bytes.HasSuffix(trimmed, []byte("%%EOF")) {
		trailingSpacesLen := len(buf) - len(trimmed)
		endOfEOFOffset := size - int64(trailingSpacesLen)
		insertIdx := endOfEOFOffset - 4
		return &safeReaderAt{r: r, size: size, insertIdx: insertIdx}, size + 1
	}
	return r, size
}

func (s *safeReaderAt) ReadAt(p []byte, off int64) (int, error) {
	virtualSize := s.size + 1
	if off >= virtualSize {
		return 0, io.EOF
	}

	var eof error
	if off+int64(len(p)) > virtualSize {
		p = p[:virtualSize-off]
		eof = io.EOF
	}

	totalRead := 0
	for len(p) > 0 {
		currOff := off + int64(totalRead)
		switch {
		case currOff < s.insertIdx:
			toRead := s.insertIdx - currOff
			if toRead > int64(len(p)) {
				toRead = int64(len(p))
			}
			n, err := s.r.ReadAt(p[:toRead], currOff)
			totalRead += n
			p = p[n:]
			if err != nil {
				return totalRead, err
			}
		case currOff == s.insertIdx:
			p[0] = '%'
			totalRead++
			p = p[1:]
		default:
			n, err := s.r.ReadAt(p, currOff-1)
			totalRead += n
			p = p[n:]
			if err != nil {
				return totalRead, err
			}
		}
	}

	return totalRead, eof
}

// validateFirstPage checks the trim size (and optional bleed specs) of the first page.
func validateFirstPage(w, h float64, expectedW, expectedH, bleed float64) []string {
	var errors []string
	wInches := w / 72.0
	hInches := h / 72.0

	wMatch := math.Abs(wInches-expectedW) <= 0.05 ||
		math.Abs(wInches-(expectedW+bleed)) <= 0.05 ||
		math.Abs(wInches-(expectedW+2*bleed)) <= 0.05

	hMatch := math.Abs(hInches-expectedH) <= 0.05 ||
		math.Abs(hInches-(expectedH+2*bleed)) <= 0.05 ||
		math.Abs(hInches-(expectedH+bleed)) <= 0.05

	if !wMatch {
		errors = append(errors, fmt.Sprintf("Page 1 width mismatch: expected %.3f in (with optional bleed), got %.3f in (%.2f pt)", expectedW, wInches, w))
	}
	if !hMatch {
		errors = append(errors, fmt.Sprintf("Page 1 height mismatch: expected %.3f in (with optional bleed), got %.3f in (%.2f pt)", expectedH, hInches, h))
	}
	return errors
}

// validateDimensions validates that all pages have matching dimensions and the first page matches KDP requirements.
func validateDimensions(r *pdf.Reader, input ValidatePDFInput, bleed float64) (string, []string) {
	numPages := r.NumPage()
	errors := []string{}
	var firstW, firstH float64
	var dimensions string

	for i := 1; i <= numPages; i++ {
		p := r.Page(i)
		w, h, pErr := getPageDimensions(p)
		if pErr != nil {
			errors = append(errors, fmt.Sprintf("Page %d: failed to parse dimensions: %v", i, pErr))
			continue
		}

		if i == 1 {
			firstW = w
			firstH = h
			dimensions = fmt.Sprintf("%.3f x %.3f in", w/72.0, h/72.0)
			firstErrs := validateFirstPage(w, h, input.ExpectedWidthInches, input.ExpectedHeightInches, bleed)
			errors = append(errors, firstErrs...)
		} else {
			if math.Abs(w-firstW) > 3.6 {
				errors = append(errors, fmt.Sprintf("Page %d width (%.2f pt) does not match page 1 width (%.2f pt)", i, w, firstW))
			}
			if math.Abs(h-firstH) > 3.6 {
				errors = append(errors, fmt.Sprintf("Page %d height (%.2f pt) does not match page 1 height (%.2f pt)", i, h, firstH))
			}
		}
	}
	return dimensions, errors
}

// checkFonts executes pdffonts and updates verification results.
func checkFonts(ctx context.Context, pdfPath string, enforceFonts bool, res *ValidatePDFResult) {
	_, lookFontsErr := exec.LookPath("pdffonts")
	if lookFontsErr != nil {
		res.Warnings = append(res.Warnings, "pdffonts utility not found on host. Skipping font embedding checks.")
		return
	}
	cmd := execCommand(ctx, "pdffonts", pdfPath)
	outputBytes, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("Failed to run pdffonts: %v (output: %s)", cmdErr, string(outputBytes)))
		return
	}
	parsePDFFontsOutput(string(outputBytes), enforceFonts, res)
}

// checkImages executes pdfimages and updates verification results.
func checkImages(ctx context.Context, pdfPath string, minDPI int, enforceCMYK bool, res *ValidatePDFResult) {
	_, lookImagesErr := exec.LookPath("pdfimages")
	if lookImagesErr != nil {
		res.Warnings = append(res.Warnings, "pdfimages utility not found on host. Skipping image resolution and color space checks.")
		return
	}
	cmd := execCommand(ctx, "pdfimages", "-list", pdfPath)
	outputBytes, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("Failed to run pdfimages: %v (output: %s)", cmdErr, string(outputBytes)))
		return
	}
	parsePDFImagesOutput(string(outputBytes), minDPI, enforceCMYK, res)
}

// ValidatePDFPreflight performs the validation of PDF geometry, fonts, images, and colors.
func ValidatePDFPreflight(ctx context.Context, input ValidatePDFInput) (*ValidatePDFResult, error) {
	bleed := 0.125
	if input.BleedInches != nil {
		bleed = *input.BleedInches
	}

	minDPI := 300
	if input.MinDPI != nil {
		minDPI = *input.MinDPI
	}

	enforceFonts := true
	if input.EnforceEmbeddedFonts != nil {
		enforceFonts = *input.EnforceEmbeddedFonts
	}

	enforceCMYK := false
	if input.EnforceCMYK != nil {
		enforceCMYK = *input.EnforceCMYK
	}

	f, err := os.Open(input.PDFPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF file %s: %w", input.PDFPath, err)
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat PDF file %s: %w", input.PDFPath, err)
	}

	safeR, safeSize := newSafeReaderAt(f, fi.Size())
	r, err := pdf.NewReader(safeR, safeSize)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF file %s: %w", input.PDFPath, err)
	}

	numPages := r.NumPage()
	res := &ValidatePDFResult{
		PageCount: numPages,
		Errors:    []string{},
		Warnings:  []string{},
	}

	if numPages == 0 {
		res.Valid = false
		res.Errors = append(res.Errors, "PDF has 0 pages")
		return res, nil
	}

	dims, dimErrs := validateDimensions(r, input, bleed)
	res.Dimensions = dims
	res.Errors = append(res.Errors, dimErrs...)

	// Font embedding analysis
	checkFonts(ctx, input.PDFPath, enforceFonts, res)

	// Image resolution and color space analysis
	checkImages(ctx, input.PDFPath, minDPI, enforceCMYK, res)

	res.Valid = (len(res.Errors) == 0)
	return res, nil
}

func parsePDFFontsOutput(output string, enforceFonts bool, res *ValidatePDFResult) {
	if !enforceFonts {
		return
	}

	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return
	}

	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 5 {
			continue
		}

		fontName := fields[0]
		embStr := fields[len(fields)-5]

		if strings.ToLower(embStr) != "yes" {
			res.Errors = append(res.Errors, fmt.Sprintf("Font %s is not embedded", fontName))
		}
	}
}

func parsePDFImagesOutput(output string, minDPI int, enforceCMYK bool, res *ValidatePDFResult) {
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return
	}

	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 11 {
			continue
		}

		pageStr := fields[0]
		colorStr := fields[len(fields)-11]
		xDpiStr := fields[len(fields)-4]
		yDpiStr := fields[len(fields)-3]

		xDpi, xErr := strconv.Atoi(xDpiStr)
		yDpi, yErr := strconv.Atoi(yDpiStr)

		if xErr == nil && yErr == nil {
			if xDpi < minDPI || yDpi < minDPI {
				res.Errors = append(res.Errors, fmt.Sprintf("Page %s image resolution %d x %d DPI is below minimum of %d DPI", pageStr, xDpi, yDpi, minDPI))
			}
		} else {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Page %s image has unparseable DPI resolution: %s x %s", pageStr, xDpiStr, yDpiStr))
		}

		if enforceCMYK {
			lowerColor := strings.ToLower(colorStr)
			if strings.Contains(lowerColor, "rgb") {
				res.Errors = append(res.Errors, fmt.Sprintf("Page %s image has RGB color space (%s), CMYK is required", pageStr, colorStr))
			}
		}
	}
}
