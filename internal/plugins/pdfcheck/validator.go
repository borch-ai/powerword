package pdfcheck

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	EnforceGrayscale     *bool    `json:"enforce_grayscale,omitempty"`      // default: false
	MinGutterInches      *float64 `json:"min_gutter_inches,omitempty"`
	MinMarginInches      *float64 `json:"min_margin_inches,omitempty"`
	MaxInkCoverage       *int     `json:"max_ink_coverage,omitempty"`
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
var execLookPath = exec.LookPath
var jpegDecode = jpeg.Decode

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
	_, lookFontsErr := execLookPath("pdffonts")
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
func checkImages(ctx context.Context, pdfPath string, minDPI int, enforceCMYK bool, enforceGrayscale bool, res *ValidatePDFResult) {
	_, lookImagesErr := execLookPath("pdfimages")
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
	parsePDFImagesOutput(string(outputBytes), minDPI, enforceCMYK, enforceGrayscale, res)
}

// parseInputDefaults resolves and sets default values for input validation arguments.
func parseInputDefaults(input ValidatePDFInput) (float64, int, bool, bool, bool, int) {
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
	enforceGrayscale := false
	if input.EnforceGrayscale != nil {
		enforceGrayscale = *input.EnforceGrayscale
	}
	maxInkCoverage := 240
	if input.MaxInkCoverage != nil {
		maxInkCoverage = *input.MaxInkCoverage
	}
	return bleed, minDPI, enforceFonts, enforceCMYK, enforceGrayscale, maxInkCoverage
}

// ValidatePDFPreflight performs the validation of PDF geometry, fonts, images, and colors.
func ValidatePDFPreflight(ctx context.Context, input ValidatePDFInput) (*ValidatePDFResult, error) {
	bleed, minDPI, enforceFonts, enforceCMYK, enforceGrayscale, maxInkCoverage := parseInputDefaults(input)

	if input.MinGutterInches != nil && *input.MinGutterInches < 0 {
		return nil, fmt.Errorf("min_gutter_inches must be non-negative")
	}
	if input.MinMarginInches != nil && *input.MinMarginInches < 0 {
		return nil, fmt.Errorf("min_margin_inches must be non-negative")
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
	checkImages(ctx, input.PDFPath, minDPI, enforceCMYK, enforceGrayscale, res)

	// Ink density analysis
	checkInkDensity(ctx, input.PDFPath, maxInkCoverage, res)

	// Content stream vector/text operator analysis
	if enforceGrayscale {
		for i := 1; i <= numPages; i++ {
			p := r.Page(i)
			opErrs := auditPageOperators(p, i)
			res.Errors = append(res.Errors, opErrs...)
		}
	}

	// Text safe-zone margins analysis
	var gutterPts, marginPts float64
	if input.MinGutterInches != nil {
		gutterPts = *input.MinGutterInches * 72.0
	}
	if input.MinMarginInches != nil {
		marginPts = *input.MinMarginInches * 72.0
	}

	if gutterPts > 0 || marginPts > 0 {
		for i := 1; i <= numPages; i++ {
			p := r.Page(i)
			marginErrs := validateTextSafeZones(p, i, gutterPts, marginPts)
			res.Errors = append(res.Errors, marginErrs...)
		}
	}

	res.Valid = (len(res.Errors) == 0)
	return res, nil
}

// checkHorizontalMargins asserts that a text element is placed safely within margins and inside gutters horizontally.
func checkHorizontalMargins(t pdf.Text, w float64, isLeftPage bool, gutterPts, marginPts float64, pageNum int) []string {
	var errs []string
	if isLeftPage && gutterPts > 0 && t.X+t.W > w-gutterPts {
		errs = append(errs, fmt.Sprintf("Page %d text %q at X=%.2f pt is within inside gutter (min: %.2f pt)", pageNum, t.S, t.X+t.W, gutterPts))
	}
	if isLeftPage && marginPts > 0 && t.X < marginPts {
		errs = append(errs, fmt.Sprintf("Page %d text %q at X=%.2f pt is within outer margin (min: %.2f pt)", pageNum, t.S, t.X, marginPts))
	}
	if !isLeftPage && gutterPts > 0 && t.X < gutterPts {
		errs = append(errs, fmt.Sprintf("Page %d text %q at X=%.2f pt is within inside gutter (min: %.2f pt)", pageNum, t.S, t.X, gutterPts))
	}
	if !isLeftPage && marginPts > 0 && t.X+t.W > w-marginPts {
		errs = append(errs, fmt.Sprintf("Page %d text %q at X=%.2f pt is within outer margin (min: %.2f pt)", pageNum, t.S, t.X+t.W, marginPts))
	}
	return errs
}

// validateTextSafeZones extracts all text elements from the page and asserts they fall inside the safe margins and gutters.
func validateTextSafeZones(page pdf.Page, pageNum int, gutterPts, marginPts float64) []string {
	var errs []string
	w, h, pErr := getPageDimensions(page)
	if pErr != nil {
		return []string{fmt.Sprintf("Page %d: failed to get dimensions for margin checks: %v", pageNum, pErr)}
	}

	content := page.Content()
	isLeftPage := pageNum%2 == 0 // Page 1 is Right page (odd), Page 2 is Left page (even)

	for _, t := range content.Text {
		if strings.TrimSpace(t.S) == "" {
			continue
		}

		// Vertical margin checks
		if marginPts > 0 {
			if t.Y < marginPts {
				errs = append(errs, fmt.Sprintf("Page %d text %q at Y=%.2f pt is within bottom margin (min: %.2f pt)", pageNum, t.S, t.Y, marginPts))
			}
			if t.Y+t.FontSize > h-marginPts {
				errs = append(errs, fmt.Sprintf("Page %d text %q at Y=%.2f pt is within top margin (min: %.2f pt)", pageNum, t.S, t.Y+t.FontSize, marginPts))
			}
		}

		// Horizontal margin and gutter checks
		hErrs := checkHorizontalMargins(t, w, isLeftPage, gutterPts, marginPts, pageNum)
		errs = append(errs, hErrs...)
	}
	return errs
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

func parsePDFImagesOutput(output string, minDPI int, enforceCMYK bool, enforceGrayscale bool, res *ValidatePDFResult) {
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

		validateImageRow(fields, minDPI, enforceCMYK, enforceGrayscale, res)
	}
}

func validateImageRow(fields []string, minDPI int, enforceCMYK, enforceGrayscale bool, res *ValidatePDFResult) {
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

	if enforceGrayscale {
		lowerColor := strings.ToLower(colorStr)
		if lowerColor != "gray" && lowerColor != "mono" {
			res.Errors = append(res.Errors, fmt.Sprintf("Page %s image has non-grayscale color space (%s), grayscale is required", pageStr, colorStr))
		}
	}
}

// getPageContentsStreams returns all contents streams for a page.
func getPageContentsStreams(p pdf.Page) []pdf.Value {
	contents := p.V.Key("Contents")
	if contents.Kind() == pdf.Array {
		var streams []pdf.Value
		for i := 0; i < contents.Len(); i++ {
			streams = append(streams, contents.Index(i))
		}
		return streams
	}
	if contents.Kind() == pdf.Stream {
		return []pdf.Value{contents}
	}
	return nil
}

// auditPageOperators inspects content streams of a page to ensure only grayscale operators are used.
func auditPageOperators(p pdf.Page, pageNum int) []string {
	var errs []string
	streams := getPageContentsStreams(p)
	for _, strm := range streams {
		pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
			// Pop all arguments to keep the stack clean and inspect them.
			n := stk.Len()
			args := make([]pdf.Value, n)
			for i := n - 1; i >= 0; i-- {
				args[i] = stk.Pop()
			}

			switch op {
			case "rg", "RG":
				errs = append(errs, fmt.Sprintf("Page %d has RGB vector/text color setting operator (%s)", pageNum, op))
			case "k", "K":
				errs = append(errs, fmt.Sprintf("Page %d has CMYK vector/text color setting operator (%s)", pageNum, op))
			case "cs", "CS":
				if len(args) > 0 {
					name := args[0].Name()
					if isNonGrayColorspace(name, p) {
						errs = append(errs, fmt.Sprintf("Page %d sets non-grayscale color space (%s)", pageNum, name))
					}
				}
			}
		})
	}
	return errs
}

// isNonGrayColorspace returns true if the colorspace name is non-grayscale.
func isNonGrayColorspace(name string, p pdf.Page) bool {
	lower := strings.ToLower(name)
	if lower == "devicegray" || lower == "calgray" {
		return false
	}
	if lower == "devicergb" || lower == "devicecmyk" || lower == "calrgb" || lower == "lab" {
		return true
	}

	// Resolve custom color space from resources
	csRes := p.Resources().Key("ColorSpace").Key(name)
	if csRes.IsNull() {
		return false
	}

	return isNonGrayColorspaceValue(csRes, p)
}

// isNonGrayColorspaceValue checks a colorspace value for grayscale status.
func isNonGrayColorspaceValue(val pdf.Value, p pdf.Page) bool {
	if val.Kind() == pdf.Name {
		return isNonGrayColorspaceName(val, p)
	}
	if val.Kind() == pdf.Array {
		return isNonGrayColorspaceArray(val, p)
	}
	return false
}

func isNonGrayColorspaceName(val pdf.Value, p pdf.Page) bool {
	name := val.Name()
	lower := strings.ToLower(name)
	if lower == "devicegray" || lower == "calgray" {
		return false
	}
	if lower == "devicergb" || lower == "devicecmyk" || lower == "calrgb" || lower == "lab" {
		return true
	}
	// Resolve custom color space from resources
	csRes := p.Resources().Key("ColorSpace").Key(name)
	if csRes.IsNull() {
		return false
	}
	return isNonGrayColorspaceValue(csRes, p)
}

func isNonGrayColorspaceArray(val pdf.Value, p pdf.Page) bool {
	if val.Len() == 0 {
		return false
	}
	family := val.Index(0).Name()
	lowerFamily := strings.ToLower(family)
	if lowerFamily == "devicegray" || lowerFamily == "calgray" {
		return false
	}
	if lowerFamily == "devicergb" || lowerFamily == "devicecmyk" || lowerFamily == "calrgb" || lowerFamily == "lab" {
		return true
	}
	if lowerFamily == "iccbased" && val.Len() > 1 {
		// ICCBased stream is the second element, look at N components
		nComp := val.Index(1).Key("N").Int64()
		return nComp > 1
	}
	if lowerFamily == "indexed" && val.Len() > 1 {
		// Base color space is the second element
		baseCS := val.Index(1)
		return isNonGrayColorspaceValue(baseCS, p)
	}
	return true
}

// ValidateCoverInput represents the input parameters for validating a book cover PDF.
type ValidateCoverInput struct {
	PDFPath              string   `json:"pdf_path"`
	ExpectedWidthInches  float64  `json:"expected_width_inches"`
	ExpectedHeightInches float64  `json:"expected_height_inches"`
	PageCount            int      `json:"page_count"`
	PaperType            string   `json:"paper_type"`
	BleedInches          *float64 `json:"bleed_inches,omitempty"`
	ExpectedISBN         string   `json:"expected_isbn,omitempty"`
}

// ValidateCoverPDF performs validation on a compiled book cover PDF.
func ValidateCoverPDF(ctx context.Context, input ValidateCoverInput) (*ValidatePDFResult, error) {
	var thickness float64
	switch strings.ToLower(input.PaperType) {
	case "white":
		thickness = 0.002252
	case "cream":
		thickness = 0.0025
	case "color":
		thickness = 0.002347
	default:
		return nil, fmt.Errorf("invalid paper type: %q", input.PaperType)
	}

	bleed := 0.125
	if input.BleedInches != nil {
		bleed = *input.BleedInches
	}

	r, f, err := parseCoverPDF(input.PDFPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	numPages := r.NumPage()
	res := &ValidatePDFResult{
		PageCount: numPages,
		Errors:    []string{},
		Warnings:  []string{},
	}

	if numPages != 1 {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("Cover PDF must have exactly 1 page, got %d", numPages))
		return res, nil
	}

	spine := float64(input.PageCount) * thickness

	page := r.Page(1)
	if err := validateCoverDimensions(page, input, spine, bleed, res); err != nil {
		return nil, err
	}

	placements := findImagePlacements(page)

	var backCoverImages []imagePlacement
	for _, p := range placements {
		if p.ctm[4] < (input.ExpectedWidthInches+bleed)*72.0 {
			backCoverImages = append(backCoverImages, p)
		}
	}

	if len(backCoverImages) == 0 {
		res.Errors = append(res.Errors, "No barcode image found on the back cover (left half of page)")
		res.Valid = false
		return res, nil
	}

	_, lookZbarErr := execLookPath("zbarimg")
	zbarInstalled := (lookZbarErr == nil)

	_, lookImagesErr := execLookPath("pdfimages")
	pdfimagesInstalled := (lookImagesErr == nil)

	if !zbarInstalled || !pdfimagesInstalled {
		checkBarcodeFallback(backCoverImages, res, zbarInstalled, pdfimagesInstalled)
		res.Valid = (len(res.Errors) == 0)
		return res, nil
	}

	if err := verifyBarcodeWithTools(ctx, page, input, bleed, placements, res); err != nil {
		return nil, err
	}

	res.Valid = (len(res.Errors) == 0)
	return res, nil
}

func parseCoverPDF(pdfPath string) (*pdf.Reader, *os.File, error) {
	//nolint:gosec // pdfPath is validated in input handler
	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open cover PDF: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("failed to stat cover PDF: %w", err)
	}

	safeR, safeSize := newSafeReaderAt(f, fi.Size())
	r, err := pdf.NewReader(safeR, safeSize)
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("failed to parse cover PDF: %w", err)
	}

	return r, f, nil
}

func validateCoverDimensions(page pdf.Page, input ValidateCoverInput, spine, bleed float64, res *ValidatePDFResult) error {
	wPoints, hPoints, err := getPageDimensions(page)
	if err != nil {
		return fmt.Errorf("failed to get cover page dimensions: %w", err)
	}

	wInches := wPoints / 72.0
	hInches := hPoints / 72.0
	res.Dimensions = fmt.Sprintf("%.3f x %.3f in", wInches, hInches)

	expectedW := input.ExpectedWidthInches*2 + spine + bleed*2
	expectedH := input.ExpectedHeightInches + bleed*2

	if math.Abs(wInches-expectedW) > 0.05 {
		res.Errors = append(res.Errors, fmt.Sprintf("Cover width mismatch: expected %.3f in (with spine %.4f in and bleed %.3f in), got %.3f in (%.2f pt)", expectedW, spine, bleed, wInches, wPoints))
	}
	if math.Abs(hInches-expectedH) > 0.05 {
		res.Errors = append(res.Errors, fmt.Sprintf("Cover height mismatch: expected %.3f in (with bleed %.3f in), got %.3f in (%.2f pt)", expectedH, bleed, hInches, hPoints))
	}
	return nil
}

func checkBarcodeFallback(backCoverImages []imagePlacement, res *ValidatePDFResult, zbarInstalled, pdfimagesInstalled bool) {
	if !zbarInstalled {
		res.Warnings = append(res.Warnings, "zbarimg utility not found on host. Skipping barcode readability checks.")
	}
	if !pdfimagesInstalled {
		res.Warnings = append(res.Warnings, "pdfimages utility not found on host. Skipping image extraction for barcode checks.")
	}

	foundBarcodeCandidate := false
	for _, p := range backCoverImages {
		wImg := p.ctm[0]
		hImg := p.ctm[3]
		if wImg > 0 && hImg > 0 && wImg > hImg {
			foundBarcodeCandidate = true
			break
		}
	}
	if !foundBarcodeCandidate {
		res.Errors = append(res.Errors, "No barcode-like image (width > height) found on the back cover")
	}
}

func verifyBarcodeWithTools(ctx context.Context, page pdf.Page, input ValidateCoverInput, bleed float64, placements []imagePlacement, res *ValidatePDFResult) error {
	tempDir, err := os.MkdirTemp("", "cover-barcode-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	objToNum, err := getPDFImageObjects(ctx, input.PDFPath)
	if err != nil {
		return err
	}

	//nolint:gosec
	extractCmd := execCommand(ctx, "pdfimages", "-png", input.PDFPath, filepath.Join(tempDir, "img"))
	extractOut, err := extractCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to extract images from PDF: %w (output: %s)", err, string(extractOut))
	}

	var verifiedBarcode string
	var foundReadableOnBack bool
	var foundBarcodeOnFront bool
	var decodedFrontValue string

	for _, p := range placements {
		checkSinglePlacementBarcode(ctx, p, tempDir, objToNum, page, input.ExpectedWidthInches, bleed, &foundReadableOnBack, &foundBarcodeOnFront, &verifiedBarcode, &decodedFrontValue)
	}

	reportBarcodeResults(input.ExpectedISBN, foundReadableOnBack, foundBarcodeOnFront, verifiedBarcode, decodedFrontValue, res)
	return nil
}

func checkSinglePlacementBarcode(ctx context.Context, p imagePlacement, tempDir string, objToNum map[int]int, page pdf.Page, expectedW float64, bleed float64, foundReadableOnBack *bool, foundBarcodeOnFront *bool, verifiedBarcode *string, decodedFrontValue *string) {
	objID, ok := getObjectID(page, p.name)
	if !ok {
		return
	}
	num, ok := objToNum[objID]
	if !ok {
		return
	}

	imgPath := findExtractedImageFile(tempDir, num)
	if imgPath == "" {
		return
	}

	//nolint:gosec
	zbarCmd := execCommand(ctx, "zbarimg", "--raw", "-q", imgPath)
	zbarOut, zbarErr := zbarCmd.CombinedOutput()
	if zbarErr != nil {
		return
	}

	decodedVal := strings.TrimSpace(string(zbarOut))
	if decodedVal == "" {
		return
	}

	isLeftHalf := p.ctm[4] < (expectedW+bleed)*72.0
	if isLeftHalf {
		*foundReadableOnBack = true
		*verifiedBarcode = decodedVal
	} else {
		*foundBarcodeOnFront = true
		*decodedFrontValue = decodedVal
	}
}

func getPDFImageObjects(ctx context.Context, pdfPath string) (map[int]int, error) {
	//nolint:gosec
	listCmd := execCommand(ctx, "pdfimages", "-list", pdfPath)
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list images in PDF: %w (output: %s)", err, string(listOut))
	}

	objToNum := make(map[int]int)
	lines := strings.Split(string(listOut), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "page") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 11 {
			continue
		}
		num, err1 := strconv.Atoi(fields[1])
		objID, err2 := strconv.Atoi(fields[len(fields)-6])
		if err1 == nil && err2 == nil {
			objToNum[objID] = num
		}
	}
	return objToNum, nil
}

func findExtractedImageFile(tempDir string, num int) string {
	matches, _ := filepath.Glob(filepath.Join(tempDir, fmt.Sprintf("img-%03d.*", num)))
	if len(matches) > 0 {
		return matches[0]
	}
	matches2, _ := filepath.Glob(filepath.Join(tempDir, fmt.Sprintf("img-%d.*", num)))
	if len(matches2) > 0 {
		return matches2[0]
	}
	return ""
}

func reportBarcodeResults(expectedISBN string, foundReadableOnBack, foundBarcodeOnFront bool, verifiedBarcode, decodedFrontValue string, res *ValidatePDFResult) {
	if !foundReadableOnBack {
		reportMissingBarcode(foundBarcodeOnFront, decodedFrontValue, res)
		return
	}
	if expectedISBN != "" {
		verifyISBNDigits(expectedISBN, verifiedBarcode, res)
	}
}

func reportMissingBarcode(foundBarcodeOnFront bool, decodedFrontValue string, res *ValidatePDFResult) {
	if foundBarcodeOnFront {
		res.Errors = append(res.Errors, fmt.Sprintf("Barcode found on front cover (right half of page, value: %s); barcode must be placed on the back cover", decodedFrontValue))
	} else {
		res.Errors = append(res.Errors, "No readable barcode found on back cover (left half of page)")
	}
}

func verifyISBNDigits(expectedISBN, verifiedBarcode string, res *ValidatePDFResult) {
	expectedDigits := cleanDigits(expectedISBN)
	decodedDigits := cleanDigits(verifiedBarcode)
	if expectedDigits != decodedDigits {
		res.Errors = append(res.Errors, fmt.Sprintf("Barcode ISBN mismatch: expected %q, got %q", expectedDigits, decodedDigits))
	}
}

func getObjectID(page pdf.Page, name string) (int, bool) {
	xobject := page.Resources().Key("XObject").Key(name)
	if xobject.IsNull() {
		return 0, false
	}
	val := reflect.ValueOf(xobject)
	ptrField := val.FieldByName("ptr")
	if !ptrField.IsValid() {
		return 0, false
	}
	for ptrField.Kind() == reflect.Pointer || ptrField.Kind() == reflect.Interface {
		if ptrField.IsNil() {
			return 0, false
		}
		ptrField = ptrField.Elem()
	}
	if ptrField.Kind() != reflect.Struct {
		return 0, false
	}
	idField := ptrField.FieldByName("id")
	if !idField.IsValid() {
		return 0, false
	}
	idVal := idField.Uint()
	if idVal > uint64(math.MaxInt) {
		return 0, false
	}
	return int(idVal), true
}

func multiply(m1, m2 [6]float64) [6]float64 {
	return [6]float64{
		m1[0]*m2[0] + m1[1]*m2[2],
		m1[0]*m2[1] + m1[1]*m2[3],
		m1[2]*m2[0] + m1[3]*m2[2],
		m1[2]*m2[1] + m1[3]*m2[3],
		m1[4]*m2[0] + m1[5]*m2[2] + m2[4],
		m1[4]*m2[1] + m1[5]*m2[3] + m2[5],
	}
}

type imagePlacement struct {
	name string
	ctm  [6]float64
}

func findImagePlacements(p pdf.Page) []imagePlacement {
	var placements []imagePlacement
	var ctmStack [][6]float64
	currentCTM := [6]float64{1, 0, 0, 1, 0, 0}

	streams := getPageContentsStreams(p)
	for _, strm := range streams {
		pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
			n := stk.Len()
			args := make([]pdf.Value, n)
			for i := n - 1; i >= 0; i-- {
				args[i] = stk.Pop()
			}

			placements, currentCTM, ctmStack = handlePDFOp(p, op, args, placements, currentCTM, ctmStack)
		})
	}
	return placements
}

func handlePDFOp(p pdf.Page, op string, args []pdf.Value, placements []imagePlacement, currentCTM [6]float64, ctmStack [][6]float64) ([]imagePlacement, [6]float64, [][6]float64) {
	switch op {
	case "q":
		ctmStack = append(ctmStack, currentCTM)
	case "Q":
		if len(ctmStack) > 0 {
			currentCTM = ctmStack[len(ctmStack)-1]
			ctmStack = ctmStack[:len(ctmStack)-1]
		}
	case "cm":
		if len(args) == 6 {
			m := [6]float64{
				getFloat(args[0]),
				getFloat(args[1]),
				getFloat(args[2]),
				getFloat(args[3]),
				getFloat(args[4]),
				getFloat(args[5]),
			}
			currentCTM = multiply(m, currentCTM)
		}
	case "Do":
		if len(args) > 0 && args[0].Kind() == pdf.Name {
			name := args[0].Name()
			xobj := p.Resources().Key("XObject").Key(name)
			if !xobj.IsNull() && xobj.Key("Subtype").Name() == "Image" {
				placements = append(placements, imagePlacement{
					name: name,
					ctm:  currentCTM,
				})
			}
		}
	}
	return placements, currentCTM, ctmStack
}

func getFloat(v pdf.Value) float64 {
	if v.Kind() == pdf.Real || v.Kind() == pdf.Integer {
		return v.Float64()
	}
	return 0
}

func cleanDigits(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// MockExecCommand overrides the package-level execCommand and returns a cleanup function.
func MockExecCommand(fn func(ctx context.Context, name string, arg ...string) *exec.Cmd) func() {
	old := execCommand
	execCommand = fn
	return func() { execCommand = old }
}

// MockExecLookPath overrides the package-level execLookPath and returns a cleanup function.
func MockExecLookPath(fn func(file string) (string, error)) func() {
	old := execLookPath
	execLookPath = fn
	return func() { execLookPath = old }
}

// MockJpegDecode overrides the package-level jpegDecode and returns a cleanup function.
func MockJpegDecode(fn func(r io.Reader) (image.Image, error)) func() {
	old := jpegDecode
	jpegDecode = fn
	return func() { jpegDecode = old }
}

// checkInkDensity renders the pages to CMYK JPEGs using Ghostscript and parses them to check for ink coverage.
func checkInkDensity(ctx context.Context, pdfPath string, limit int, res *ValidatePDFResult) {
	_, lookGsErr := execLookPath("gs")
	if lookGsErr != nil {
		res.Warnings = append(res.Warnings, "Ghostscript (gs) utility not found on host. Skipping ink density checks.")
		return
	}

	tempDir, err := os.MkdirTemp("", "pdf-inkcov-*")
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("Failed to create temporary directory for ink density check: %v", err))
		return
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	outputPath := filepath.Join(tempDir, "page-%d.jpg")
	cmd := execCommand(ctx, "gs", "-dSAFER", "-dNOPAUSE", "-dBATCH", "-sDEVICE=jpegcmyk", "-r150", "-sOutputFile="+outputPath, pdfPath)
	outputBytes, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("Failed to run Ghostscript for ink density check: %v (output: %s)", cmdErr, string(outputBytes)))
		return
	}

	matches, err := filepath.Glob(filepath.Join(tempDir, "page-*.jpg"))
	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("Failed to list generated page images: %v", err))
		return
	}

	for _, match := range matches {
		base := filepath.Base(match)
		var pageNum int
		_, sscanfErr := fmt.Sscanf(base, "page-%d.jpg", &pageNum)
		if sscanfErr != nil {
			pageNum = 0
		}

		imgFile, err := os.Open(match)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("Page %d: failed to open rendered image: %v", pageNum, err))
			continue
		}

		img, err := jpegDecode(imgFile)
		_ = imgFile.Close()
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("Page %d: failed to decode rendered image: %v", pageNum, err))
			continue
		}

		maxDensity := 0
		cmykImg, ok := img.(*image.CMYK)
		if ok {
			bounds := cmykImg.Bounds()
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					c := cmykImg.CMYKAt(x, y)
					density := int(c.C) + int(c.M) + int(c.Y) + int(c.K)
					if density > maxDensity {
						maxDensity = density
					}
				}
			}
		} else {
			bounds := img.Bounds()
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					c := img.At(x, y)
					cmykColor := color.CMYKModel.Convert(c).(color.CMYK)
					density := int(cmykColor.C) + int(cmykColor.M) + int(cmykColor.Y) + int(cmykColor.K)
					if density > maxDensity {
						maxDensity = density
					}
				}
			}
		}

		maxPercent := float64(maxDensity) * 100.0 / 255.0
		if maxPercent > float64(limit) {
			res.Errors = append(res.Errors, fmt.Sprintf("Page %d: maximum ink density of %.1f%% exceeds the limit of %d%%", pageNum, maxPercent, limit))
		}
	}
}
