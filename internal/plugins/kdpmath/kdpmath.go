package kdpmath

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"rsc.io/pdf"
)

// Constants for KDP paper thickness factors in inches per page.
const (
	MulWhite         = 0.002252
	MulCream         = 0.0025
	MulPremiumColor  = 0.002347
	MulStandardColor = 0.002252
)

// LayoutGuide represents a logical bounding box in the cover coordinate system.
type LayoutGuide struct {
	Label        string  `json:"label"`
	XInches      float64 `json:"x_inches"`
	YInches      float64 `json:"y_inches"`
	WidthInches  float64 `json:"width_inches"`
	HeightInches float64 `json:"height_inches"`
}

// GeometryResult contains calculated coordinates and safe margins for cover layout.
type GeometryResult struct {
	BindingType          string        `json:"binding_type"`
	PaperType            string        `json:"paper_type"`
	PageCount            int           `json:"page_count"`
	TrimWidthInches      float64       `json:"trim_width_inches"`
	TrimHeightInches     float64       `json:"trim_height_inches"`
	SpineWidthInches     float64       `json:"spine_width_inches"`
	SpineWidthPoints     float64       `json:"spine_width_points"`
	CoverWidthInches     float64       `json:"cover_width_inches"`
	CoverWidthPoints     float64       `json:"cover_width_points"`
	CoverHeightInches    float64       `json:"cover_height_inches"`
	CoverHeightPoints    float64       `json:"cover_height_points"`
	HingeWidthInches     float64       `json:"hinge_width_inches,omitempty"`
	HingeWidthPoints     float64       `json:"hinge_width_points,omitempty"`
	WrapWidthInches      float64       `json:"wrap_width_inches,omitempty"`
	WrapWidthPoints      float64       `json:"wrap_width_points,omitempty"`
	OverhangHeightInches float64       `json:"overhang_height_inches,omitempty"`
	OverhangHeightPoints float64       `json:"overhang_height_points,omitempty"`
	SpineTextEligible    bool          `json:"spine_text_eligible"`
	Guides               []LayoutGuide `json:"guides,omitempty"`
}

// ValidationResult reports structural and dimension validation findings on a PDF file.
type ValidationResult struct {
	IsValid              bool     `json:"is_valid"`
	PageCount            int      `json:"page_count"`
	ExpectedPageCount    int      `json:"expected_page_count,omitempty"`
	WidthPoints          float64  `json:"width_points"`
	HeightPoints         float64  `json:"height_points"`
	ExpectedWidthPoints  float64  `json:"expected_width_points"`
	ExpectedHeightPoints float64  `json:"expected_height_points"`
	Errors               []string `json:"errors,omitempty"`
	IsCover              bool     `json:"is_cover"`
}

// parseTrimSize extracts width and height from trim size configuration (e.g. "6x9").
func parseTrimSize(trimSize string) (float64, float64, error) {
	parts := strings.FieldsFunc(trimSize, func(r rune) bool {
		return r == 'x' || r == 'X' || r == '*' || r == '-'
	})
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid trim size format %q: expected format like '6x9'", trimSize)
	}
	wStr := strings.TrimSpace(parts[0])
	hStr := strings.TrimSpace(parts[1])
	w, err := strconv.ParseFloat(wStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse trim width %q: %w", wStr, err)
	}
	h, err := strconv.ParseFloat(hStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse trim height %q: %w", hStr, err)
	}
	return w, h, nil
}

// getMultiplier maps KDP paper options to thickness multipliers.
func getMultiplier(paperType string) (float64, error) {
	switch strings.ToLower(strings.TrimSpace(paperType)) {
	case "white", "bw_white", "black_white_white", "black & white on white paper":
		return MulWhite, nil
	case "cream", "bw_cream", "black_white_cream", "black & white on cream paper":
		return MulCream, nil
	case "premium_color", "color", "premium color", "premium color on white paper":
		return MulPremiumColor, nil
	case "standard_color", "standard color", "standard color on white paper":
		return MulStandardColor, nil
	default:
		return 0, fmt.Errorf("unsupported paper type %q: must be white, cream, premium_color, or standard_color", paperType)
	}
}

// CalculateGeometry computes KDP cover layout dimensions, safe zones, and panel guides.
func CalculateGeometry(pageCount int, bindingType string, paperType string, trimSize string) (*GeometryResult, error) {
	if pageCount <= 0 {
		return nil, fmt.Errorf("page count must be positive, got %d", pageCount)
	}
	mult, err := getMultiplier(paperType)
	if err != nil {
		return nil, err
	}
	tw, th, err := parseTrimSize(trimSize)
	if err != nil {
		return nil, err
	}

	bType := strings.ToLower(strings.TrimSpace(bindingType))
	pType := strings.ToLower(strings.TrimSpace(paperType))

	res := &GeometryResult{
		BindingType:       bType,
		PaperType:         pType,
		PageCount:         pageCount,
		TrimWidthInches:   tw,
		TrimHeightInches:  th,
		SpineTextEligible: pageCount >= 79,
	}

	switch bType {
	case "paperback":
		// Spine Width = Page Count * Multiplier + 0.06" cover allowance
		res.SpineWidthInches = float64(pageCount)*mult + 0.06
		// Cover Width = 0.125" (bleed) + tw + spine + tw + 0.125" (bleed)
		res.CoverWidthInches = 2*tw + res.SpineWidthInches + 0.25
		// Cover Height = trim height + 2 * 0.125" (bleed)
		res.CoverHeightInches = th + 0.25

		// Set paperback layout panel guides
		res.Guides = []LayoutGuide{
			{Label: "Left Bleed", XInches: 0, YInches: 0, WidthInches: 0.125, HeightInches: res.CoverHeightInches},
			{Label: "Back Cover", XInches: 0.125, YInches: 0.125, WidthInches: tw, HeightInches: th},
			{Label: "Spine", XInches: 0.125 + tw, YInches: 0.125, WidthInches: res.SpineWidthInches, HeightInches: th},
			{Label: "Front Cover", XInches: 0.125 + tw + res.SpineWidthInches, YInches: 0.125, WidthInches: tw, HeightInches: th},
			{Label: "Right Bleed", XInches: 0.125 + 2*tw + res.SpineWidthInches, YInches: 0, WidthInches: 0.125, HeightInches: res.CoverHeightInches},
		}

	case "hardcover":
		// Spine Width = Page Count * Multiplier
		res.SpineWidthInches = float64(pageCount) * mult
		// Cover Width = 2*tw + spine + 0.394" (hinge total) + 2*0.591" (wrap total)
		res.CoverWidthInches = 2*tw + res.SpineWidthInches + 0.394 + 1.182
		// Cover Height = trim height + 0.236" (overhang total) + 2*0.591" (wrap total)
		res.CoverHeightInches = th + 0.236 + 1.182

		res.HingeWidthInches = 0.394
		res.HingeWidthPoints = 0.394 * 72.0
		res.WrapWidthInches = 0.591
		res.WrapWidthPoints = 0.591 * 72.0
		res.OverhangHeightInches = 0.236
		res.OverhangHeightPoints = 0.236 * 72.0

		// Set hardcover case wrap panel guides
		// Hinge width is 0.197" per side (0.394" total / 2)
		hSide := 0.197
		res.Guides = []LayoutGuide{
			{Label: "Left Wrap", XInches: 0, YInches: 0, WidthInches: 0.591, HeightInches: res.CoverHeightInches},
			{Label: "Back Cover Board", XInches: 0.591, YInches: 0.591 + 0.118, WidthInches: tw, HeightInches: th},
			{Label: "Left Hinge", XInches: 0.591 + tw, YInches: 0.591 + 0.118, WidthInches: hSide, HeightInches: th},
			{Label: "Spine Board", XInches: 0.591 + tw + hSide, YInches: 0.591 + 0.118, WidthInches: res.SpineWidthInches, HeightInches: th},
			{Label: "Right Hinge", XInches: 0.591 + tw + hSide + res.SpineWidthInches, YInches: 0.591 + 0.118, WidthInches: hSide, HeightInches: th},
			{Label: "Front Cover Board", XInches: 0.591 + tw + 2*hSide + res.SpineWidthInches, YInches: 0.591 + 0.118, WidthInches: tw, HeightInches: th},
			{Label: "Right Wrap", XInches: 0.591 + 2*tw + 2*hSide + res.SpineWidthInches, YInches: 0, WidthInches: 0.591, HeightInches: res.CoverHeightInches},
		}

	default:
		return nil, fmt.Errorf("unsupported binding type %q: must be paperback or hardcover", bindingType)
	}

	res.SpineWidthPoints = res.SpineWidthInches * 72.0
	res.CoverWidthPoints = res.CoverWidthInches * 72.0
	res.CoverHeightPoints = res.CoverHeightInches * 72.0

	return res, nil
}

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

// ValidatePDF checks PDF integrity, total page numbers, and page box dimensions (trim size and bleed compliance) in points.
func ValidatePDF(pdfPath string, bindingType string, paperType string, trimSize string, expectedPageCount int, isCover bool, hasBleed bool) (*ValidationResult, error) {
	//nolint:gosec // pdfPath is expected to be a dynamic path checked/provided by the user
	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF file %s: %w", pdfPath, err)
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat PDF file %s: %w", pdfPath, err)
	}

	safeR, safeSize := newSafeReaderAt(f, fi.Size())
	r, err := pdf.NewReader(safeR, safeSize)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF file %s: %w", pdfPath, err)
	}

	numPages := r.NumPage()
	res := &ValidationResult{
		PageCount: numPages,
		IsCover:   isCover,
	}

	if numPages == 0 {
		res.IsValid = false
		res.Errors = append(res.Errors, "PDF has 0 pages")
		return res, nil
	}

	if isCover {
		if err := validateCoverPDF(r, bindingType, paperType, trimSize, expectedPageCount, res); err != nil {
			return nil, err
		}
	} else {
		if err := validateInteriorPDF(r, trimSize, expectedPageCount, hasBleed, res); err != nil {
			return nil, err
		}
	}

	res.IsValid = len(res.Errors) == 0
	return res, nil
}

func validateCoverPDF(r *pdf.Reader, bindingType string, paperType string, trimSize string, expectedPageCount int, res *ValidationResult) error {
	numPages := r.NumPage()
	if numPages != 1 {
		res.Errors = append(res.Errors, fmt.Sprintf("expected exactly 1 page for a cover PDF, got %d pages", numPages))
	}

	geom, gErr := CalculateGeometry(expectedPageCount, bindingType, paperType, trimSize)
	if gErr != nil {
		return fmt.Errorf("failed to calculate expected cover geometry: %w", gErr)
	}

	res.ExpectedWidthPoints = geom.CoverWidthPoints
	res.ExpectedHeightPoints = geom.CoverHeightPoints

	p := r.Page(1)
	w, h, pErr := getPageDimensions(p)
	if pErr != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("failed to parse page 1 dimensions: %v", pErr))
	} else {
		res.WidthPoints = w
		res.HeightPoints = h

		// Validate with a tolerance of 0.05" (3.6 points)
		if math.Abs(w-geom.CoverWidthPoints) > 3.6 {
			res.Errors = append(res.Errors, fmt.Sprintf("cover width mismatch: expected %.2f pt (%.2f in), got %.2f pt (%.2f in)",
				geom.CoverWidthPoints, geom.CoverWidthInches, w, w/72.0))
		}
		if math.Abs(h-geom.CoverHeightPoints) > 3.6 {
			res.Errors = append(res.Errors, fmt.Sprintf("cover height mismatch: expected %.2f pt (%.2f in), got %.2f pt (%.2f in)",
				geom.CoverHeightPoints, geom.CoverHeightInches, h, h/72.0))
		}
	}
	return nil
}

func validateInteriorPDF(r *pdf.Reader, trimSize string, expectedPageCount int, hasBleed bool, res *ValidationResult) error {
	numPages := r.NumPage()
	if expectedPageCount > 0 && numPages != expectedPageCount {
		res.ExpectedPageCount = expectedPageCount
		res.Errors = append(res.Errors, fmt.Sprintf("page count mismatch: expected %d pages, got %d pages", expectedPageCount, numPages))
	}

	tw, th, parseErr := parseTrimSize(trimSize)
	if parseErr != nil {
		return fmt.Errorf("failed to parse trim size: %w", parseErr)
	}

	var expectedWInches, expectedHInches float64
	if hasBleed {
		expectedWInches = tw + 0.125
		expectedHInches = th + 0.25
	} else {
		expectedWInches = tw
		expectedHInches = th
	}

	expectedWPoints := expectedWInches * 72.0
	expectedHPoints := expectedHInches * 72.0

	res.ExpectedWidthPoints = expectedWPoints
	res.ExpectedHeightPoints = expectedHPoints

	validateInteriorPageDimensions(r, numPages, expectedWPoints, expectedHPoints, expectedWInches, expectedHInches, res)
	return nil
}

func validateInteriorPageDimensions(r *pdf.Reader, numPages int, expectedWPoints, expectedHPoints, expectedWInches, expectedHInches float64, res *ValidationResult) {
	var firstW, firstH float64
	for i := 1; i <= numPages; i++ {
		p := r.Page(i)
		w, h, pErr := getPageDimensions(p)
		if pErr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("failed to parse page %d dimensions: %v", i, pErr))
			continue
		}

		if i == 1 {
			firstW = w
			firstH = h
			res.WidthPoints = w
			res.HeightPoints = h
			validateFirstPageDimensions(w, h, expectedWPoints, expectedHPoints, expectedWInches, expectedHInches, res)
		} else {
			if math.Abs(w-firstW) > 3.6 {
				res.Errors = append(res.Errors, fmt.Sprintf("page %d width (%.2f pt) does not match page 1 width (%.2f pt)", i, w, firstW))
			}
			if math.Abs(h-firstH) > 3.6 {
				res.Errors = append(res.Errors, fmt.Sprintf("page %d height (%.2f pt) does not match page 1 height (%.2f pt)", i, h, firstH))
			}
		}
	}
}

func validateFirstPageDimensions(w, h, expectedWPoints, expectedHPoints, expectedWInches, expectedHInches float64, res *ValidationResult) {
	if math.Abs(w-expectedWPoints) > 3.6 {
		res.Errors = append(res.Errors, fmt.Sprintf("page 1 width mismatch: expected %.2f pt (%.2f in), got %.2f pt (%.2f in)",
			expectedWPoints, expectedWInches, w, w/72.0))
	}
	if math.Abs(h-expectedHPoints) > 3.6 {
		res.Errors = append(res.Errors, fmt.Sprintf("page 1 height mismatch: expected %.2f pt (%.2f in), got %.2f pt (%.2f in)",
			expectedHPoints, expectedHInches, h, h/72.0))
	}
}

// GenerateManifest produces standard JSON layout configs matching Inkfluence/BookBolt.
func GenerateManifest(pageCount int, bindingType string, paperType string, trimSize string) (string, error) {
	geom, err := CalculateGeometry(pageCount, bindingType, paperType, trimSize)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(geom, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal geometry: %w", err)
	}

	return string(data), nil
}
