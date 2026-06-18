package typst

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDimension(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"8.5in", 8.5, false},
		{" 8.5 in ", 8.5, false},
		{"72pt", 1.0, false},
		{"25.4mm", 1.0, false},
		{"2.54cm", 1.0, false},
		{"6", 6.0, false}, // Defaults to inches
		{"", 0, true},
		{"abc", 0, true},
		{"12abc", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseDimension(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseDimension(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseDimension(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParsePageSize(t *testing.T) {
	tests := []struct {
		input   string
		wantW   float64
		wantH   float64
		wantErr bool
	}{
		{"8.5in,8.5in", 8.5, 8.5, false},
		{"6in, 9in", 6.0, 9.0, false},
		{"8.5,11", 8.5, 11.0, false},
		{"8.5in", 0, 0, true},
		{"8.5in,8.5in,11in", 0, 0, true},
		{"abc,8.5in", 0, 0, true},
		{"8.5in,abc", 0, 0, true},
	}

	for _, tt := range tests {
		w, h, err := ParsePageSize(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParsePageSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && (w != tt.wantW || h != tt.wantH) {
			t.Errorf("ParsePageSize(%q) = %v, %v, want %v, %v", tt.input, w, h, tt.wantW, tt.wantH)
		}
	}
}

func TestParseManuscript(t *testing.T) {
	content := `Some skipped line here
<!-- Comment -->
<!-- Multi-line
comment
-->
# Page 1
## Text
Mr. Eggerton, with worried gaze,
Sat high upon the shelf.

He fretted over market shares,
And thought about himself.

## Prompt
The scene is a grand, ornate office.

<!-- Another comment -->
# Page 2
## Text
He fretted over market shares.
`
	tmpDir, err := os.MkdirTemp("", "typst_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "manuscript.md")
	if wErr := os.WriteFile(filePath, []byte(content), 0600); wErr != nil {
		t.Fatalf("failed to write manuscript: %v", wErr)
	}

	pages, err := ParseManuscript(filePath)
	if err != nil {
		t.Fatalf("ParseManuscript failed: %v", err)
	}

	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}

	expectedPage1Text := `Mr. Eggerton, with worried gaze, \
Sat high upon the shelf.

He fretted over market shares, \
And thought about himself.`
	if pages[0].Number != 1 || pages[0].Text != expectedPage1Text || pages[0].Prompt != "The scene is a grand, ornate office." {
		t.Errorf("page 1 mismatch: %+v", pages[0])
	}

	if pages[1].Number != 2 || pages[1].Text != "He fretted over market shares." || pages[1].Prompt != "" {
		t.Errorf("page 2 mismatch: %+v", pages[1])
	}
}

func TestRenderTemplates(t *testing.T) {
	// 1. Test Interior Template with full-bleed
	params := InteriorParams{
		PhysicalWidth:  8.75,
		PhysicalHeight: 8.75,
		MarginTop:      0.625,
		MarginBottom:   0.625,
		MarginInside:   0.5,
		MarginOutside:  0.375,
		FontFamily:     "Georgia",
		Pages: []InteriorPage{
			{Text: "Hello Page 1", ImagePath: "page_1.png"},
			{Text: "Hello Page 2", ImagePath: ""},
		},
		Layout: "full-bleed",
	}

	interiorCode, err := RenderInterior(params)
	if err != nil {
		t.Fatalf("RenderInterior failed: %v", err)
	}

	if !strings.Contains(interiorCode, `width: 8.7500in`) ||
		!strings.Contains(interiorCode, `font: "Georgia"`) ||
		!strings.Contains(interiorCode, `image("page_1.png"`) ||
		!strings.Contains(interiorCode, `Hello Page 2`) {
		t.Errorf("Rendered interior lacks expected elements:\n%s", interiorCode)
	}

	// Test facing-pages layout
	params.Layout = "facing-pages"
	facingCode, err := RenderInterior(params)
	if err != nil {
		t.Fatalf("RenderInterior facing-pages failed: %v", err)
	}
	// Verify it contains text-only block (with background: none) and then image block
	if !strings.Contains(facingCode, `#page(background: none)`) ||
		!strings.Contains(facingCode, `image("page_1.png"`) ||
		!strings.Contains(facingCode, `Hello Page 1`) {
		t.Errorf("Rendered facing interior lacks expected elements:\n%s", facingCode)
	}

	// Test facing-pages-flipped layout
	params.Layout = "facing-pages-flipped"
	flippedCode, err := RenderInterior(params)
	if err != nil {
		t.Fatalf("RenderInterior facing-pages-flipped failed: %v", err)
	}
	if !strings.Contains(flippedCode, `#page(background: image("page_1.png"`) ||
		!strings.Contains(flippedCode, `Hello Page 1`) {
		t.Errorf("Rendered flipped interior lacks expected elements:\n%s", flippedCode)
	}

	// 2. Test Cover Template
	coverParams := CoverParams{
		PhysicalWidth:     17.33,
		PhysicalHeight:    8.75,
		BackCoverWidth:    8.625,
		SpineWidth:        0.08,
		SpineTextEligible: true,
		FrontCoverDx:      8.705,
		FrontCoverWidth:   8.625,
		FrontImagePath:    "cover.png",
		Title:             "Eggerton",
		Subtitle:          "Egg Drama",
		Author:            "Borch Group",
		FontFamily:        "Georgia",
	}

	coverCode, err := RenderCover(coverParams)
	if err != nil {
		t.Fatalf("RenderCover failed: %v", err)
	}

	if !strings.Contains(coverCode, `width: 17.3300in`) ||
		!strings.Contains(coverCode, `rotate(-90deg)`) ||
		!strings.Contains(coverCode, `Eggerton - Borch Group`) ||
		!strings.Contains(coverCode, `Egg Drama`) {
		t.Errorf("Rendered cover lacks expected elements:\n%s", coverCode)
	}
}

func TestCompilerSandbox(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "typst_sandbox_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	c, err := NewCompiler(tmpDir)
	if err != nil {
		t.Fatalf("NewCompiler failed: %v", err)
	}

	// Valid path
	valid := filepath.Join(tmpDir, "test.pdf")
	path, err := c.CheckSandbox(valid)
	if err != nil {
		t.Errorf("expected path inside sandbox: %v", err)
	}
	if path != valid {
		t.Errorf("sandbox path mismatch: %s != %s", path, valid)
	}

	// Invalid path (traversal out of workspace)
	invalid := filepath.Join(tmpDir, "../outside.pdf")
	_, err = c.CheckSandbox(invalid)
	if err == nil {
		t.Errorf("expected sandbox error for %s", invalid)
	}
}

func TestFindImageForPage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "typst_images_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create dummy files
	dummyFiles := []string{"page_1.png", "page-2.jpg", "3.PNG", "other.txt"}
	for _, df := range dummyFiles {
		if wErr := os.WriteFile(filepath.Join(tmpDir, df), []byte("dummy"), 0600); wErr != nil {
			t.Fatalf("failed to write dummy file: %v", wErr)
		}
	}

	// Matches
	img1, err := FindImageForPage(tmpDir, 1)
	if err != nil || !strings.HasSuffix(img1, "page_1.png") {
		t.Errorf("failed to match page 1: %s, %v", img1, err)
	}

	img2, err := FindImageForPage(tmpDir, 2)
	if err != nil || !strings.HasSuffix(img2, "page-2.jpg") {
		t.Errorf("failed to match page 2: %s, %v", img2, err)
	}

	img3, err := FindImageForPage(tmpDir, 3)
	if err != nil || !strings.HasSuffix(img3, "3.PNG") {
		t.Errorf("failed to match page 3: %s, %v", img3, err)
	}

	// Missing
	_, err = FindImageForPage(tmpDir, 4)
	if err == nil {
		t.Errorf("expected missing image error for page 4")
	}
}

func TestNewCompilerEnv(t *testing.T) {
	t.Setenv("POWERWORD_TYPST_BIN", "custom_typst")
	c, err := NewCompiler(".")
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}
	if c.TypstBin != "custom_typst" {
		t.Errorf("expected custom_typst, got %s", c.TypstBin)
	}
}

func TestNewCompilerPATHFallback(t *testing.T) {
	t.Setenv("POWERWORD_TYPST_BIN", "")
	t.Setenv("PATH", "") // Force exec.LookPath to fail
	c, err := NewCompiler(".")
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}
	if c.TypstBin != "typst" {
		t.Errorf("expected fallback to typst, got %s", c.TypstBin)
	}
}

func TestCompilerCompileTempFileError(t *testing.T) {
	c := &Compiler{
		WorkspaceRoot: "/non_existent_directory_for_typst_test_12345",
		TypstBin:      "typst",
	}
	err := c.Compile(context.Background(), "= Test", "out.pdf")
	if err == nil {
		t.Errorf("expected temp file creation error")
	}
}

func TestParseManuscriptMissingFile(t *testing.T) {
	_, err := ParseManuscript("non_existent_file_12345.md")
	if err == nil {
		t.Errorf("expected error for non-existent file")
	}
}

func TestFindImageForPageMissingDir(t *testing.T) {
	_, err := FindImageForPage("non_existent_dir_12345", 1)
	if err == nil {
		t.Errorf("expected error for non-existent images directory")
	}
}

func TestCompilerSandboxAbsoluteRelative(t *testing.T) {
	c, err := NewCompiler(".")
	if err != nil {
		t.Fatalf("failed to create compiler: %v", err)
	}

	// absolute path outside workspace
	_, err = c.CheckSandbox("/outside/path/file.pdf")
	if err == nil {
		t.Errorf("expected sandbox error for absolute path outside workspace")
	}
}

func TestCompilerCompileError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "typst_compile_err_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	c, err := NewCompiler(tmpDir)
	if err != nil {
		t.Fatalf("NewCompiler failed: %v", err)
	}

	c.TypstBin = "invalid_command_xyz_12345"

	// Invalid Typst code / compiler binary
	code := `#invalid_typst_command_here`
	outPath := filepath.Join(tmpDir, "failed.pdf")
	err = c.Compile(context.Background(), code, outPath)
	if err == nil {
		t.Errorf("expected compilation error for invalid typst code/compiler")
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
echo mock_pdf > "%out%"
`
	} else {
		content = `#!/bin/sh
out=""
for arg; do out=$arg; done
echo mock_pdf > "$out"
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

func TestCompilerCompileSuccessMock(t *testing.T) {
	mockTypst := createMockTypstBin(t)
	t.Setenv("POWERWORD_TYPST_BIN", mockTypst)

	tmpDir, err := os.MkdirTemp("", "typst_compile_mock_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	c, err := NewCompiler(tmpDir)
	if err != nil {
		t.Fatalf("NewCompiler failed: %v", err)
	}

	outPath := filepath.Join(tmpDir, "output.pdf")
	err = c.Compile(context.Background(), "= Test", outPath)
	if err != nil {
		t.Fatalf("mock compile failed: %v", err)
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Errorf("mock output file does not exist")
	}
}

func TestIntegrationTypstCompile(t *testing.T) {
	// Integration tag check
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("skipping integration tests, set RUN_INTEGRATION_TESTS=true to run")
	}

	tmpDir, err := os.MkdirTemp("", "typst_compile_integration_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	c, err := NewCompiler(tmpDir)
	if err != nil {
		t.Fatalf("NewCompiler failed: %v", err)
	}

	// Test compilation
	code := `
	#set page(width: 4in, height: 4in, margin: 0.5in)
	= Integration Test
	This verifies typst is installed and compilation works.
	`
	outPath := filepath.Join(tmpDir, "output.pdf")
	err = c.Compile(context.Background(), code, outPath)
	if err != nil {
		t.Fatalf("Compiling test document failed: %v", err)
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Errorf("compiled PDF does not exist at %s", outPath)
	}
}
