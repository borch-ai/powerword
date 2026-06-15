package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

// Chapter represents a section or chapter of the book.
type Chapter struct {
	ID       string
	Title    string
	Filename string
	Body     string
}

// ImageAsset represents an illustration or graphic embedded in the book.
type ImageAsset struct {
	ID        string
	Filename  string
	MediaType string
}

// CompileOpts defines the inputs for the EPUB builder.
type CompileOpts struct {
	ManuscriptPath string
	ImagesDir      string
	OutputPath     string
	Title          string
	Author         string
	Language       string // defaults to "en"
	StylesheetPath string // optional custom CSS
}

// compileContext holds variables needed to generate template XMLs.
type compileContext struct {
	Title         string
	Author        string
	UUID          string
	Language      string
	ModifiedTime  string
	Chapters      []Chapter
	Images        []ImageAsset
	HasStylesheet bool
}

const (
	containerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="EPUB/content.opf" media-type="application/oebps-package+xml"/>
    </rootfiles>
</container>`

	contentOPFTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookID" version="3.0">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
        <dc:title>{{.Title}}</dc:title>
        <dc:creator id="creator">{{.Author}}</dc:creator>
        <dc:identifier id="BookID">urn:uuid:{{.UUID}}</dc:identifier>
        <dc:language>{{.Language}}</dc:language>
        <meta property="dcterms:modified">{{.ModifiedTime}}</meta>
    </metadata>
    <manifest>
        <item id="toc" href="toc.xhtml" media-type="application/xhtml+xml" properties="nav"/>
        {{range .Chapters}}
        <item id="{{.ID}}" href="{{.Filename}}" media-type="application/xhtml+xml"/>
        {{end}}
        {{if .HasStylesheet}}
        <item id="stylesheet" href="css/stylesheet.css" media-type="text/css"/>
        {{end}}
        {{range .Images}}
        <item id="{{.ID}}" href="images/{{.Filename}}" media-type="{{.MediaType}}"/>
        {{end}}
    </manifest>
    <spine>
        <itemref idref="toc"/>
        {{range .Chapters}}
        <itemref idref="{{.ID}}"/>
        {{end}}
    </spine>
</package>`

	tocTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2001/epub" xml:lang="{{.Language}}" lang="{{.Language}}">
<head>
    <title>Table of Contents</title>
    <meta charset="utf-8" />
    {{if .HasStylesheet}}
    <link rel="stylesheet" type="text/css" href="css/stylesheet.css" />
    {{end}}
</head>
<body>
    <nav epub:type="toc" id="toc">
        <h1>Table of Contents</h1>
        <ol>
            {{range .Chapters}}
            <li><a href="{{.Filename}}">{{.Title}}</a></li>
            {{end}}
        </ol>
    </nav>
</body>
</html>`

	chapterTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="{{.Language}}" lang="{{.Language}}">
<head>
    <title>{{.Title}}</title>
    <meta charset="utf-8" />
    {{if .HasStylesheet}}
    <link rel="stylesheet" type="text/css" href="css/stylesheet.css" />
    {{end}}
</head>
<body>
    <section>
        <h1>{{.Title}}</h1>
        {{.Body}}
    </section>
</body>
</html>`
)

// GenerateUUID generates a random RFC 4122 Version 4 UUID.
func GenerateUUID() string {
	uuid := make([]byte, 16)
	_, _ = rand.Read(uuid)
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

// CompileEPUB compiles a book manuscript and illustration assets into an EPUB file.
func CompileEPUB(ctx context.Context, opts CompileOpts) error {
	_ = ctx
	if opts.Language == "" {
		opts.Language = "en"
	}

	manuscriptBytes, err := os.ReadFile(opts.ManuscriptPath)
	if err != nil {
		return fmt.Errorf("failed to read manuscript file %s: %w", opts.ManuscriptPath, err)
	}

	chapters := ParseManuscript(string(manuscriptBytes))

	images, err := findImages(opts.ImagesDir)
	if err != nil {
		return fmt.Errorf("failed to scan images directory: %w", err)
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	uuidVal := GenerateUUID()
	compCtx := compileContext{
		Title:         opts.Title,
		Author:        opts.Author,
		UUID:          uuidVal,
		Language:      opts.Language,
		ModifiedTime:  now,
		Chapters:      chapters,
		Images:        images,
		HasStylesheet: opts.StylesheetPath != "",
	}

	outDir := filepath.Dir(opts.OutputPath)
	if outDir != "" {
		// Enforce directory permissions to be 0750 or less for security
		if err = os.MkdirAll(outDir, 0750); err != nil {
			return fmt.Errorf("failed to create output directory %s: %w", outDir, err)
		}
	}

	outFile, err := os.Create(opts.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output EPUB file %s: %w", opts.OutputPath, err)
	}
	defer func() { _ = outFile.Close() }()

	zipWriter := zip.NewWriter(outFile)
	defer func() { _ = zipWriter.Close() }()

	if err = writeMimetype(zipWriter); err != nil {
		return err
	}

	if err = writeContainerXML(zipWriter); err != nil {
		return err
	}

	if opts.StylesheetPath != "" {
		if err = writeStylesheet(zipWriter, opts.StylesheetPath); err != nil {
			return err
		}
	}

	if err = writeImages(zipWriter, opts.ImagesDir, images); err != nil {
		return err
	}

	if err = writeOPF(zipWriter, compCtx); err != nil {
		return err
	}

	if err = writeTOC(zipWriter, compCtx); err != nil {
		return err
	}

	return writeChapters(zipWriter, compCtx, opts.StylesheetPath != "", opts.Language)
}

func writeMimetype(zw *zip.Writer) error {
	mimetypeHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mw, err := zw.CreateHeader(mimetypeHeader)
	if err != nil {
		return fmt.Errorf("failed to create mimetype zip header: %w", err)
	}
	if _, err = mw.Write([]byte("application/epub+zip")); err != nil {
		return fmt.Errorf("failed to write mimetype to zip: %w", err)
	}
	return nil
}

func writeContainerXML(zw *zip.Writer) error {
	cw, err := zw.Create("META-INF/container.xml")
	if err != nil {
		return fmt.Errorf("failed to create container.xml zip entry: %w", err)
	}
	if _, err = cw.Write([]byte(containerXML)); err != nil {
		return fmt.Errorf("failed to write container.xml to zip: %w", err)
	}
	return nil
}

func writeStylesheet(zw *zip.Writer, path string) error {
	//nolint:gosec // path is validated as a configuration parameter
	styleBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read stylesheet file %s: %w", path, err)
	}
	sw, err := zw.Create("EPUB/css/stylesheet.css")
	if err != nil {
		return fmt.Errorf("failed to create stylesheet.css zip entry: %w", err)
	}
	if _, err = sw.Write(styleBytes); err != nil {
		return fmt.Errorf("failed to write stylesheet.css to zip: %w", err)
	}
	return nil
}

func writeImages(zw *zip.Writer, dir string, images []ImageAsset) error {
	for _, img := range images {
		imgSrcPath := filepath.Join(dir, img.Filename)
		//nolint:gosec // imgSrcPath is constructed from validated parameters
		imgBytes, err := os.ReadFile(imgSrcPath)
		if err != nil {
			return fmt.Errorf("failed to read image file %s: %w", imgSrcPath, err)
		}
		iw, err := zw.Create("EPUB/images/" + img.Filename)
		if err != nil {
			return fmt.Errorf("failed to create image zip entry for %s: %w", img.Filename, err)
		}
		if _, err = iw.Write(imgBytes); err != nil {
			return fmt.Errorf("failed to write image %s to zip: %w", img.Filename, err)
		}
	}
	return nil
}

func writeOPF(zw *zip.Writer, compCtx compileContext) error {
	opfTmpl, err := template.New("opf").Parse(contentOPFTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse content.opf template: %w", err)
	}
	var opfBuf bytes.Buffer
	if err = opfTmpl.Execute(&opfBuf, compCtx); err != nil {
		return fmt.Errorf("failed to execute content.opf template: %w", err)
	}
	opfW, err := zw.Create("EPUB/content.opf")
	if err != nil {
		return fmt.Errorf("failed to create content.opf zip entry: %w", err)
	}
	if _, err = opfW.Write(opfBuf.Bytes()); err != nil {
		return fmt.Errorf("failed to write content.opf to zip: %w", err)
	}
	return nil
}

func writeTOC(zw *zip.Writer, compCtx compileContext) error {
	tocTmpl, err := template.New("toc").Parse(tocTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse toc.xhtml template: %w", err)
	}
	var tocBuf bytes.Buffer
	if err = tocTmpl.Execute(&tocBuf, compCtx); err != nil {
		return fmt.Errorf("failed to execute toc.xhtml template: %w", err)
	}
	tocW, err := zw.Create("EPUB/toc.xhtml")
	if err != nil {
		return fmt.Errorf("failed to create toc.xhtml zip entry: %w", err)
	}
	if _, err = tocW.Write(tocBuf.Bytes()); err != nil {
		return fmt.Errorf("failed to write toc.xhtml to zip: %w", err)
	}
	return nil
}

func writeChapters(zw *zip.Writer, compCtx compileContext, hasStylesheet bool, lang string) error {
	chapTmpl, err := template.New("chapter").Parse(chapterTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse chapter template: %w", err)
	}
	for _, chap := range compCtx.Chapters {
		type chapContext struct {
			Title         string
			Language      string
			Body          string
			HasStylesheet bool
		}
		cCtx := chapContext{
			Title:         chap.Title,
			Language:      lang,
			Body:          chap.Body,
			HasStylesheet: hasStylesheet,
		}
		var chapBuf bytes.Buffer
		if err = chapTmpl.Execute(&chapBuf, cCtx); err != nil {
			return fmt.Errorf("failed to execute chapter template for %s: %w", chap.ID, err)
		}
		chapW, err := zw.Create("EPUB/" + chap.Filename)
		if err != nil {
			return fmt.Errorf("failed to create chapter zip entry for %s: %w", chap.Filename, err)
		}
		if _, err = chapW.Write(chapBuf.Bytes()); err != nil {
			return fmt.Errorf("failed to write chapter %s to zip: %w", chap.Filename, err)
		}
	}
	return nil
}

type manuscriptParser struct {
	chapters     []Chapter
	currentChap  *Chapter
	bodyBuilder  strings.Builder
	inParagraph  bool
	inList       bool
	listType     string
	imgRegex     *regexp.Regexp
	boldRegex    *regexp.Regexp
	italicRegex1 *regexp.Regexp
	italicRegex2 *regexp.Regexp
	orderedRegex *regexp.Regexp
}

func (p *manuscriptParser) closeList() {
	if p.inList {
		_, _ = fmt.Fprintf(&p.bodyBuilder, "</%s>\n", p.listType)
		p.inList = false
		p.listType = ""
	}
}

func (p *manuscriptParser) closeParagraph() {
	if p.inParagraph {
		p.bodyBuilder.WriteString("</p>\n")
		p.inParagraph = false
	}
}

func (p *manuscriptParser) closeAll() {
	p.closeParagraph()
	p.closeList()
}

func (p *manuscriptParser) commitChapter() {
	if p.currentChap != nil {
		p.closeAll()
		p.currentChap.Body = p.bodyBuilder.String()
		p.chapters = append(p.chapters, *p.currentChap)
		p.bodyBuilder.Reset()
	}
}

// ParseManuscript parses Markdown text and splits it into Chapters on # or ## headers.
func ParseManuscript(content string) []Chapter {
	lines := strings.Split(content, "\n")

	parser := &manuscriptParser{
		imgRegex:     regexp.MustCompile(`!\[(.*?)\]\((.*?)\)`),
		boldRegex:    regexp.MustCompile(`\*\*(.*?)\*\*`),
		italicRegex1: regexp.MustCompile(`\*(.*?)\*`),
		italicRegex2: regexp.MustCompile(`_(.*?)_`),
		orderedRegex: regexp.MustCompile(`^\d+\.\s+`),
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for level 1 or 2 headers (starts new chapter)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			parser.commitChapter()

			headerText := ""
			if strings.HasPrefix(trimmed, "# ") {
				headerText = strings.TrimPrefix(trimmed, "# ")
			} else {
				headerText = strings.TrimPrefix(trimmed, "## ")
			}
			headerText = strings.TrimSpace(headerText)

			chapID := fmt.Sprintf("chap_%d", len(parser.chapters)+1)
			parser.currentChap = &Chapter{
				ID:       chapID,
				Title:    headerText,
				Filename: fmt.Sprintf("%s.xhtml", chapID),
			}
			continue
		}

		// If no header has been encountered, create a default first chapter
		if parser.currentChap == nil && trimmed != "" {
			parser.currentChap = &Chapter{
				ID:       "chap_1",
				Title:    "Introduction",
				Filename: "chap_1.xhtml",
			}
		}

		if parser.currentChap == nil {
			continue
		}

		parser.parseLine(trimmed)
	}

	parser.commitChapter()

	return parser.chapters
}

func (p *manuscriptParser) parseLine(trimmed string) {
	// Check for minor header (###)
	if strings.HasPrefix(trimmed, "### ") {
		p.closeAll()
		headerText := strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))
		_, _ = fmt.Fprintf(&p.bodyBuilder, "<h3>%s</h3>\n", headerText)
		return
	}

	// Empty line closes open lists/paragraphs
	if trimmed == "" {
		p.closeAll()
		return
	}

	// Unordered list item
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		p.closeParagraph()
		itemText := trimmed[2:]
		if !p.inList || p.listType != "ul" {
			p.closeList()
			p.bodyBuilder.WriteString("<ul>\n")
			p.inList = true
			p.listType = "ul"
		}
		_, _ = fmt.Fprintf(&p.bodyBuilder, "  <li>%s</li>\n", p.parseInlineMarkup(itemText))
		return
	}

	// Ordered list item
	if p.orderedRegex.MatchString(trimmed) {
		p.closeParagraph()
		itemText := p.orderedRegex.ReplaceAllString(trimmed, "")
		if !p.inList || p.listType != "ol" {
			p.closeList()
			p.bodyBuilder.WriteString("<ol>\n")
			p.inList = true
			p.listType = "ol"
		}
		_, _ = fmt.Fprintf(&p.bodyBuilder, "  <li>%s</li>\n", p.parseInlineMarkup(itemText))
		return
	}

	// Default regular paragraph text
	p.closeList()
	if !p.inParagraph {
		p.bodyBuilder.WriteString("<p>")
		p.inParagraph = true
	} else {
		p.bodyBuilder.WriteString("\n")
	}
	p.bodyBuilder.WriteString(p.parseInlineMarkup(trimmed))
}

func (p *manuscriptParser) parseInlineMarkup(text string) string {
	res := text
	res = p.imgRegex.ReplaceAllString(res, `<img src="images/$2" alt="$1" />`)
	res = p.boldRegex.ReplaceAllString(res, "<strong>$1</strong>")
	res = p.italicRegex1.ReplaceAllString(res, "<em>$1</em>")
	res = p.italicRegex2.ReplaceAllString(res, "<em>$1</em>")
	return res
}

func findImages(dir string) ([]ImageAsset, error) {
	if dir == "" {
		return nil, nil
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var assets []ImageAsset
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(file.Name()))
		mediaType := ""
		switch ext {
		case ".png":
			mediaType = "image/png"
		case ".jpg", ".jpeg":
			mediaType = "image/jpeg"
		case ".gif":
			mediaType = "image/gif"
		case ".svg":
			mediaType = "image/svg+xml"
		default:
			continue
		}

		filename := file.Name()
		id := strings.TrimSuffix(filename, filepath.Ext(filename))
		id = strings.ReplaceAll(id, "-", "_")
		id = strings.ReplaceAll(id, " ", "_")
		id = "img_" + id

		assets = append(assets, ImageAsset{
			ID:        id,
			Filename:  filename,
			MediaType: mediaType,
		})
	}

	return assets, nil
}
