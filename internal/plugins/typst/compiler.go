package typst

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
)

// Compiler coordinates paths validation and executes Typst commands in a sandboxed manner.
type Compiler struct {
	WorkspaceRoot string
	TypstBin      string
}

// NewCompiler instantiates a Compiler with default bin lookup.
func NewCompiler(workspaceRoot string) (*Compiler, error) {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute workspace root: %w", err)
	}

	// Resolve binary name/path
	typstBin := os.Getenv("POWERWORD_TYPST_BIN")
	if typstBin == "" {
		resolved, err := exec.LookPath("typst")
		if err == nil {
			typstBin = resolved
		} else {
			typstBin = "typst" // fallback, let execution fail with command not found
		}
	}

	return &Compiler{
		WorkspaceRoot: absRoot,
		TypstBin:      typstBin,
	}, nil
}

// CheckSandbox ensures the path is clean and strictly within the workspace root.
func (c *Compiler) CheckSandbox(target string) (string, error) {
	if filepath.IsAbs(target) {
		rel, err := filepath.Rel(c.WorkspaceRoot, target)
		if err != nil {
			return "", err
		}
		target = rel
	}

	clean := filepath.Clean(target)
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("path %s is outside of workspace root %s", target, c.WorkspaceRoot)
	}

	return securejoin.SecureJoin(c.WorkspaceRoot, target)
}

// Compile compiles a string of Typst code into a PDF at the target output path.
func (c *Compiler) Compile(ctx context.Context, typstCode string, outputPath string) error {
	// Ensure the output directory exists
	outDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outDir, 0750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create temp file strictly inside the workspace root (to respect sandbox rules)
	tmpFile, err := os.CreateTemp(c.WorkspaceRoot, ".typst_tmp_*.typ")
	if err != nil {
		return fmt.Errorf("failed to create temporary Typst file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.WriteString(typstCode); err != nil {
		return fmt.Errorf("failed to write Typst temp code: %w", err)
	}

	// Compile typst
	// We run typst compile --root / to support absolute paths safely in templates.
	//nolint:gosec // c.TypstBin is configured/validated safely and paths are sandboxed
	cmd := exec.CommandContext(ctx, c.TypstBin, "compile", "--root", "/", tmpPath, outputPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("typst compile failed: %s: %w", stderr.String(), err)
	}

	return nil
}

// ParseDimension parses a string dimension like "8.5in", "12pt" into float64 inches.
func ParseDimension(dim string) (float64, error) {
	dim = strings.TrimSpace(strings.ToLower(dim))
	if dim == "" {
		return 0, fmt.Errorf("empty dimension")
	}

	var unit string
	var valStr string
	switch {
	case strings.HasSuffix(dim, "in"):
		unit = "in"
		valStr = strings.TrimSuffix(dim, "in")
	case strings.HasSuffix(dim, "pt"):
		unit = "pt"
		valStr = strings.TrimSuffix(dim, "pt")
	case strings.HasSuffix(dim, "mm"):
		unit = "mm"
		valStr = strings.TrimSuffix(dim, "mm")
	case strings.HasSuffix(dim, "cm"):
		unit = "cm"
		valStr = strings.TrimSuffix(dim, "cm")
	default:
		unit = "in"
		valStr = dim
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(valStr), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid dimension value %q: %w", dim, err)
	}

	switch unit {
	case "in":
		return val, nil
	case "pt":
		return val / 72.0, nil
	case "mm":
		return val / 25.4, nil
	case "cm":
		return val / 2.54, nil
	default:
		return val, nil
	}
}

// ParsePageSize parses a "width,height" string like "8.5in,8.5in" into float64 inches.
func ParsePageSize(pageSize string) (float64, float64, error) {
	parts := strings.Split(pageSize, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid page size format %q: expected 'width,height'", pageSize)
	}
	w, err := ParseDimension(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid width in page_size: %w", err)
	}
	h, err := ParseDimension(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid height in page_size: %w", err)
	}
	return w, h, nil
}

// FindImageForPage locates an illustration for a specific page number inside imagesDir.
func FindImageForPage(imagesDir string, pageNum int) (string, error) {
	suffixes := []string{
		fmt.Sprintf("page_%d", pageNum),
		fmt.Sprintf("page-%d", pageNum),
		fmt.Sprintf("page%d", pageNum),
		fmt.Sprintf("%d", pageNum),
	}
	extensions := []string{".png", ".jpg", ".jpeg", ".PNG", ".JPG", ".JPEG"}

	files, err := os.ReadDir(imagesDir)
	if err != nil {
		return "", fmt.Errorf("failed to read images directory %s: %w", imagesDir, err)
	}

	for _, s := range suffixes {
		for _, ext := range extensions {
			targetName := s + ext
			for _, f := range files {
				if !f.IsDir() && strings.EqualFold(f.Name(), targetName) {
					return filepath.Join(imagesDir, f.Name()), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no matching image found for page %d in %s", pageNum, imagesDir)
}
