package epub

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateUUID(t *testing.T) {
	u1, err1 := GenerateUUID()
	if err1 != nil {
		t.Fatalf("unexpected error: %v", err1)
	}
	u2, err2 := GenerateUUID()
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}

	if len(u1) != 36 {
		t.Errorf("expected UUID length to be 36, got %d", len(u1))
	}
	if u1 == u2 {
		t.Errorf("expected generated UUIDs to be unique, got duplicate: %s", u1)
	}

	// UUID v4 format check
	parts := strings.Split(u1, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 hyphen-separated parts in UUID, got %v", parts)
	}
}

func TestGenerateUUID_Error(t *testing.T) {
	oldFn := generateUUIDFn
	generateUUIDFn = func() (string, error) {
		return "", fmt.Errorf("mocked uuid error")
	}
	defer func() { generateUUIDFn = oldFn }()

	// Verify CompileEPUB propagates GenerateUUID error
	tempDir := t.TempDir()
	opts := CompileOpts{
		ManuscriptPath: filepath.Join(tempDir, "manuscript.md"),
		OutputPath:     filepath.Join(tempDir, "output.epub"),
		Title:          "Title",
		Author:         "Author",
	}
	_ = os.WriteFile(opts.ManuscriptPath, []byte("# Chapter 1\nContent"), 0600)
	err := CompileEPUB(context.Background(), opts)
	if err == nil {
		t.Error("expected error from CompileEPUB when GenerateUUID fails, got nil")
	}
}

func TestParseManuscript(t *testing.T) {
	manuscript := `
Some pre-header text.
Second line of pre-header.

# Chapter 1 Title

This is a paragraph in chapter 1.
It continues on the next line.

## Chapter 2 Title

Another paragraph.

- Unordered item 1
- Unordered item 2

1. Ordered item 1
2. Ordered item 2

### Minor Section Header

Text after minor header with **bold** and *italic* and _italic2_ markup.
An image: ![An Image](illust1.png)
`
	chaps := ParseManuscript(manuscript)

	if len(chaps) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chaps))
	}

	// Chapter 1 (pre-header text default)
	if chaps[0].Title != "Introduction" {
		t.Errorf("expected first chapter title to be Introduction, got %s", chaps[0].Title)
	}
	if !strings.Contains(chaps[0].Body, "<p>Some pre-header text.") {
		t.Errorf("missing pre-header text in body, got: %s", chaps[0].Body)
	}

	// Chapter 2 (# Chapter 1 Title)
	if chaps[1].Title != "Chapter 1 Title" {
		t.Errorf("expected second chapter title, got %s", chaps[1].Title)
	}
	if !strings.Contains(chaps[1].Body, "<p>This is a paragraph in chapter 1.") {
		t.Errorf("missing paragraph text in body, got: %s", chaps[1].Body)
	}

	// Chapter 3 (## Chapter 2 Title)
	if chaps[2].Title != "Chapter 2 Title" {
		t.Errorf("expected third chapter title, got %s", chaps[2].Title)
	}
	if !strings.Contains(chaps[2].Body, "<ul>") || !strings.Contains(chaps[2].Body, "<li>Unordered item 1</li>") {
		t.Errorf("missing unordered list in body, got: %s", chaps[2].Body)
	}
	if !strings.Contains(chaps[2].Body, "<ol>") || !strings.Contains(chaps[2].Body, "<li>Ordered item 1</li>") {
		t.Errorf("missing ordered list in body, got: %s", chaps[2].Body)
	}
	if !strings.Contains(chaps[2].Body, "<h3>Minor Section Header</h3>") {
		t.Errorf("missing minor section header in body, got: %s", chaps[2].Body)
	}
	if !strings.Contains(chaps[2].Body, "<strong>bold</strong>") {
		t.Errorf("missing bold element in body, got: %s", chaps[2].Body)
	}
	if !strings.Contains(chaps[2].Body, "<em>italic</em>") || !strings.Contains(chaps[2].Body, "<em>italic2</em>") {
		t.Errorf("missing italic element in body, got: %s", chaps[2].Body)
	}
	if !strings.Contains(chaps[2].Body, `<img src="images/illust1.png" alt="An Image" />`) {
		t.Errorf("missing image tag in body, got: %s", chaps[2].Body)
	}
}

func TestFindImages(t *testing.T) {
	tempDir := t.TempDir()

	// Write mock images
	_ = os.WriteFile(filepath.Join(tempDir, "image1.png"), []byte("png"), 0600)
	_ = os.WriteFile(filepath.Join(tempDir, "image2.jpg"), []byte("jpg"), 0600)
	_ = os.WriteFile(filepath.Join(tempDir, "image3.jpeg"), []byte("jpeg"), 0600)
	_ = os.WriteFile(filepath.Join(tempDir, "image4.gif"), []byte("gif"), 0600)
	_ = os.WriteFile(filepath.Join(tempDir, "image5.svg"), []byte("svg"), 0600)
	_ = os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("txt"), 0600) // ignored file

	assets, err := findImages(tempDir)
	if err != nil {
		t.Fatalf("findImages failed: %v", err)
	}

	if len(assets) != 5 {
		t.Errorf("expected 5 image assets, got %d", len(assets))
	}

	// Verify IDs are generated correctly (replaces hyphens/spaces with underscore, prepends img_)
	for _, asset := range assets {
		if !strings.HasPrefix(asset.ID, "img_") {
			t.Errorf("expected image ID to start with img_, got %s", asset.ID)
		}
		if asset.Filename == "image1.png" && asset.MediaType != "image/png" {
			t.Errorf("expected image/png for image1.png, got %s", asset.MediaType)
		}
	}

	// Verify empty directory returns nil/no-error
	emptyDir := t.TempDir()
	emptyAssets, err := findImages(emptyDir)
	if err != nil {
		t.Errorf("expected no error on empty directory, got %v", err)
	}
	if len(emptyAssets) != 0 {
		t.Errorf("expected 0 assets for empty directory, got %d", len(emptyAssets))
	}

	// Verify non-existent directory returns nil/no-error
	missingAssets, err := findImages(filepath.Join(tempDir, "does-not-exist"))
	if err != nil {
		t.Errorf("expected no error on non-existent directory, got %v", err)
	}
	if len(missingAssets) != 0 {
		t.Errorf("expected 0 assets for non-existent directory, got %d", len(missingAssets))
	}
}

func TestCompileEPUB(t *testing.T) {
	tempDir := t.TempDir()

	manuscriptPath := filepath.Join(tempDir, "manuscript.md")
	manuscriptContent := `# Chapter 1 & Special <tag>
Hello world & raw text <foo>.
![Image & alt](test_img.png)
### Minor header & tag <bar>
`
	_ = os.WriteFile(manuscriptPath, []byte(manuscriptContent), 0600)

	stylesheetPath := filepath.Join(tempDir, "style.css")
	_ = os.WriteFile(stylesheetPath, []byte("body { color: black; }"), 0600)

	imagesDir := filepath.Join(tempDir, "images")
	_ = os.Mkdir(imagesDir, 0750)
	_ = os.WriteFile(filepath.Join(imagesDir, "test_img.png"), []byte("pngdata"), 0600)

	outputPath := filepath.Join(tempDir, "output.epub")

	opts := CompileOpts{
		ManuscriptPath: manuscriptPath,
		ImagesDir:      imagesDir,
		OutputPath:     outputPath,
		Title:          "Test Book & Title <with tags>",
		Author:         "Test Author & Name <with tags>",
		Language:       "en",
		StylesheetPath: stylesheetPath,
	}

	ctx := context.Background()
	err := CompileEPUB(ctx, opts)
	if err != nil {
		t.Fatalf("CompileEPUB failed: %v", err)
	}

	// Open compiled EPUB
	rc, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("failed to open compiled EPUB zip: %v", err)
	}
	defer func() { _ = rc.Close() }()

	if len(rc.File) == 0 {
		t.Fatal("compiled EPUB is empty")
	}

	// Check that first file is "mimetype" and is STORED (Method = 0)
	firstFile := rc.File[0]
	if firstFile.Name != "mimetype" {
		t.Errorf("expected first file in EPUB to be mimetype, got %s", firstFile.Name)
	}
	if firstFile.Method != zip.Store {
		t.Errorf("expected mimetype to be stored with no compression (Store), got method %d", firstFile.Method)
	}

	// Verify mimetype content
	fr, err := firstFile.Open()
	if err != nil {
		t.Fatalf("failed to open mimetype file: %v", err)
	}
	mimetypeBytes, err := io.ReadAll(fr)
	_ = fr.Close()
	if err != nil {
		t.Fatalf("failed to read mimetype content: %v", err)
	}
	if string(mimetypeBytes) != "application/epub+zip" {
		t.Errorf("expected mimetype content to be 'application/epub+zip', got %q", string(mimetypeBytes))
	}

	// Verify other expected files are present
	expectedFiles := map[string]bool{
		"META-INF/container.xml":   false,
		"EPUB/css/stylesheet.css":  false,
		"EPUB/images/test_img.png": false,
		"EPUB/content.opf":         false,
		"EPUB/toc.xhtml":           false,
		"EPUB/chap_1.xhtml":        false,
	}

	for _, file := range rc.File {
		if _, ok := expectedFiles[file.Name]; ok {
			expectedFiles[file.Name] = true
		}
	}

	for filename, found := range expectedFiles {
		if !found {
			t.Errorf("missing expected file in EPUB: %s", filename)
		}
	}

	verifyOPFEscaping(t, rc)
	verifyTOCEscaping(t, rc)
	verifyChapterEscaping(t, rc)
}

func verifyOPFEscaping(t *testing.T, rc *zip.ReadCloser) {
	t.Helper()
	opfFile, err := rc.Open("EPUB/content.opf")
	if err != nil {
		t.Fatalf("failed to open content.opf: %v", err)
	}
	defer func() { _ = opfFile.Close() }()
	opfContent, err := io.ReadAll(opfFile)
	if err != nil {
		t.Fatalf("failed to read content.opf: %v", err)
	}
	if !strings.Contains(string(opfContent), "<dc:title>Test Book &amp; Title &lt;with tags&gt;</dc:title>") {
		t.Errorf("expected dc:title to be escaped in content.opf, got %s", string(opfContent))
	}
	if !strings.Contains(string(opfContent), "<dc:creator id=\"creator\">Test Author &amp; Name &lt;with tags&gt;</dc:creator>") {
		t.Errorf("expected dc:creator to be escaped in content.opf, got %s", string(opfContent))
	}
}

func verifyTOCEscaping(t *testing.T, rc *zip.ReadCloser) {
	t.Helper()
	tocFile, err := rc.Open("EPUB/toc.xhtml")
	if err != nil {
		t.Fatalf("failed to open toc.xhtml: %v", err)
	}
	defer func() { _ = tocFile.Close() }()
	tocContent, err := io.ReadAll(tocFile)
	if err != nil {
		t.Fatalf("failed to read toc.xhtml: %v", err)
	}
	if !strings.Contains(string(tocContent), "Chapter 1 &amp; Special &lt;tag&gt;") {
		t.Errorf("expected chapter title to be escaped in toc.xhtml, got %s", string(tocContent))
	}
}

func verifyChapterEscaping(t *testing.T, rc *zip.ReadCloser) {
	t.Helper()
	chapFile, err := rc.Open("EPUB/chap_1.xhtml")
	if err != nil {
		t.Fatalf("failed to open chap_1.xhtml: %v", err)
	}
	defer func() { _ = chapFile.Close() }()
	chapContent, err := io.ReadAll(chapFile)
	if err != nil {
		t.Fatalf("failed to read chap_1.xhtml: %v", err)
	}
	if !strings.Contains(string(chapContent), "<h1>Chapter 1 &amp; Special &lt;tag&gt;</h1>") {
		t.Errorf("expected header H1 to be escaped in chap_1.xhtml, got %s", string(chapContent))
	}
	if !strings.Contains(string(chapContent), "Hello world &amp; raw text &lt;foo&gt;.") {
		t.Errorf("expected inline body text to be escaped in chap_1.xhtml, got %s", string(chapContent))
	}
	if !strings.Contains(string(chapContent), "alt=\"Image &amp; alt\"") {
		t.Errorf("expected image alt text to be escaped in chap_1.xhtml, got %s", string(chapContent))
	}
	if !strings.Contains(string(chapContent), "<h3>Minor header &amp; tag &lt;bar&gt;</h3>") {
		t.Errorf("expected minor header H3 to be escaped in chap_1.xhtml, got %s", string(chapContent))
	}
}

func TestCompileEPUB_MissingManuscript(t *testing.T) {
	opts := CompileOpts{
		ManuscriptPath: "does-not-exist.md",
		OutputPath:     "output.epub",
		Title:          "Title",
		Author:         "Author",
	}
	err := CompileEPUB(context.Background(), opts)
	if err == nil {
		t.Error("expected error for non-existent manuscript, got nil")
	}
}

func TestCompileEPUB_MissingImagesDir(t *testing.T) {
	tempDir := t.TempDir()
	manuscriptPath := filepath.Join(tempDir, "manuscript.md")
	_ = os.WriteFile(manuscriptPath, []byte("# Intro\nText"), 0600)
	outputPath := filepath.Join(tempDir, "output.epub")

	opts := CompileOpts{
		ManuscriptPath: manuscriptPath,
		ImagesDir:      "/path/does/not/exist",
		OutputPath:     outputPath,
		Title:          "Title",
		Author:         "Author",
	}
	err := CompileEPUB(context.Background(), opts)
	if err != nil {
		t.Errorf("expected no error from missing images directory (it returns empty assets), got: %v", err)
	}
}

func TestCompileEPUB_BadStylesheet(t *testing.T) {
	tempDir := t.TempDir()
	manuscriptPath := filepath.Join(tempDir, "manuscript.md")
	_ = os.WriteFile(manuscriptPath, []byte("# Intro\nText"), 0600)
	outputPath := filepath.Join(tempDir, "output.epub")

	opts := CompileOpts{
		ManuscriptPath: manuscriptPath,
		OutputPath:     outputPath,
		Title:          "Title",
		Author:         "Author",
		StylesheetPath: "/path/does/not/exist.css",
	}
	err := CompileEPUB(context.Background(), opts)
	if err == nil {
		t.Error("expected error for non-existent stylesheet, got nil")
	}
}

func TestCompileEPUB_ContextCancelled(t *testing.T) {
	tempDir := t.TempDir()
	opts := CompileOpts{
		ManuscriptPath: filepath.Join(tempDir, "manuscript.md"),
		OutputPath:     filepath.Join(tempDir, "output.epub"),
		Title:          "Title",
		Author:         "Author",
	}
	_ = os.WriteFile(opts.ManuscriptPath, []byte("# Chapter 1\nContent"), 0600)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := CompileEPUB(ctx, opts)
	if err == nil {
		t.Error("expected context canceled error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected error to be context.Canceled, got: %v", err)
	}
}

func TestCompileEPUB_CreateFileError(t *testing.T) {
	tempDir := t.TempDir()
	manuscriptPath := filepath.Join(tempDir, "manuscript.md")
	_ = os.WriteFile(manuscriptPath, []byte("# Intro\nText"), 0600)

	opts := CompileOpts{
		ManuscriptPath: manuscriptPath,
		OutputPath:     tempDir, // directory path will cause os.Create to fail
		Title:          "Title",
		Author:         "Author",
	}
	err := CompileEPUB(context.Background(), opts)
	if err == nil {
		t.Error("expected error when OutputPath is a directory, got nil")
	}
}

type errorWriter struct{}

func (errorWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("forced write error")
}

func TestInternalHelpers_Errors(t *testing.T) {
	tempDir := t.TempDir()
	dummyFile := filepath.Join(tempDir, "dummy.txt")
	_ = os.WriteFile(dummyFile, []byte("test"), 0600)

	zw := zip.NewWriter(errorWriter{})
	// Write a large file with zip.Store to force a flush and trigger the write error
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "trigger-error",
		Method: zip.Store,
	})
	if err == nil {
		_, _ = w.Write(make([]byte, 5000))
	}

	if err := writeMimetype(zw); err == nil {
		t.Error("expected writeMimetype to fail on closed zip writer")
	}
	if err := writeContainerXML(zw); err == nil {
		t.Error("expected writeContainerXML to fail on closed zip writer")
	}
	// G304 testing: ReadFile should pass but zip Create fails
	if err := writeStylesheet(zw, dummyFile); err == nil {
		t.Error("expected writeStylesheet to fail on closed zip writer")
	}
	// ReadFile error in writeStylesheet
	if err := writeStylesheet(zw, "/path/does/not/exist"); err == nil {
		t.Error("expected writeStylesheet to fail on missing file")
	}
	// ReadFile error in writeImages
	if err := writeImages(zw, tempDir, []ImageAsset{{Filename: "nonexistent.png"}}); err == nil {
		t.Error("expected writeImages to fail on missing image file")
	}
	// G304 testing: ReadFile passes but zip Create fails
	if err := writeImages(zw, tempDir, []ImageAsset{{Filename: "dummy.txt"}}); err == nil {
		t.Error("expected writeImages to fail on closed zip writer")
	}
	if err := writeOPF(zw, compileContext{}); err == nil {
		t.Error("expected writeOPF to fail on closed zip writer")
	}
	if err := writeTOC(zw, compileContext{}); err == nil {
		t.Error("expected writeTOC to fail on closed zip writer")
	}
	if err := writeChapters(zw, compileContext{Chapters: []Chapter{{Filename: "c.xhtml"}}, Language: "en"}, false); err == nil {
		t.Error("expected writeChapters to fail on closed zip writer")
	}
}
