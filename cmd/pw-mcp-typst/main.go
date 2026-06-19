package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"rsc.io/pdf"

	"github.com/borch-ai/powerword/internal/plugins/kdpmath"
	"github.com/borch-ai/powerword/internal/plugins/typst"
	"github.com/borch-ai/powerword/pkg/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	workspaceRoot := os.Getenv("POWERWORD_WORKSPACE_ROOT")
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get cwd: %w", err)
		}
		workspaceRoot = cwd
	}

	cfgPath := filepath.Join(workspaceRoot, "powerword.toml")
	var cfg *config.Config
	var err error

	//nolint:gosec // cfgPath is constructed from validated workspaceRoot
	if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
		cfg, err = config.LoadConfig("")
		if err != nil {
			cfg = &config.Config{}
		}
	} else {
		cfg, err = config.LoadConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to load configuration from %s: %w", cfgPath, err)
		}
	}

	srv, err := setupServer(workspaceRoot, cfg)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

const (
	compileInteriorSchema = `{
		"type": "object",
		"properties": {
			"manuscript_path": {
				"type": "string",
				"description": "Absolute or workspace-relative path to the .typ or .md source file"
			},
			"images_dir": {
				"type": "string",
				"description": "Directory containing page illustration PNGs/JPGs"
			},
			"output_path": {
				"type": "string",
				"description": "Desired output PDF file path"
			},
			"page_size": {
				"type": "string",
				"description": "Trim page size, default: '8.5in,8.5in'"
			},
			"margin_inside": {
				"type": "string",
				"description": "Inside page margin, default: '0.375in'"
			},
			"margin_outside": {
				"type": "string",
				"description": "Outside page margin, default: '0.25in'"
			},
			"bleed": {
				"type": "string",
				"description": "Page bleed margin, default: '0.125in'"
			},
			"font_family": {
				"type": "string",
				"description": "Font family name, default: 'Georgia'"
			},
			"layout": {
				"type": "string",
				"description": "Book page layout mode: 'full-bleed', 'facing-pages', or 'facing-pages-flipped', default: 'full-bleed'"
			}
		},
		"required": ["manuscript_path", "output_path"]
	}`

	compileCoverSchema = `{
		"type": "object",
		"properties": {
			"front_image_path": {
				"type": "string",
				"description": "Absolute or workspace-relative path to the front cover image file"
			},
			"title": {
				"type": "string",
				"description": "Book title"
			},
			"subtitle": {
				"type": "string",
				"description": "Book subtitle"
			},
			"author": {
				"type": "string",
				"description": "Book author"
			},
			"page_count": {
				"type": "integer",
				"description": "Total interior page count of the book for spine calculation"
			},
			"output_path": {
				"type": "string",
				"description": "Desired output cover PDF file path"
			},
			"trim_size": {
				"type": "string",
				"description": "Trim size of the book cover, default: '8.5x8.5'"
			},
			"paper_type": {
				"type": "string",
				"description": "Paper type: 'white', 'cream', 'standard_color', or 'premium_color', default: 'white'"
			},
			"binding_type": {
				"type": "string",
				"description": "Binding type: 'paperback' or 'hardcover', default: 'paperback'"
			},
			"font_family": {
				"type": "string",
				"description": "Font family name, default: 'Georgia'"
			}
		},
		"required": ["front_image_path", "title", "subtitle", "author", "page_count", "output_path"]
	}`
)

func setupServer(workspaceRoot string, _ *config.Config) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-typst",
		Version: "0.1.0",
	}, nil)

	compiler, err := typst.NewCompiler(workspaceRoot)
	if err != nil {
		return nil, err
	}

	srv.AddTool(&mcp.Tool{
		Name:        "compile_interior",
		Description: "Compiles a multi-page book interior layout from a Markdown or Typst manuscript and illustration directory.",
		InputSchema: json.RawMessage(compileInteriorSchema),
	}, handleCompileInterior(compiler))

	srv.AddTool(&mcp.Tool{
		Name:        "compile_cover",
		Description: "Compiles a print-ready paperback/hardcover PDF cover spread (Back Cover, Spine, Front Cover) with design layout.",
		InputSchema: json.RawMessage(compileCoverSchema),
	}, handleCompileCover(compiler))

	return srv, nil
}

type interiorArgs struct {
	ManuscriptPath string `json:"manuscript_path"`
	ImagesDir      string `json:"images_dir"`
	OutputPath     string `json:"output_path"`
	PageSize       string `json:"page_size"`
	MarginInside   string `json:"margin_inside"`
	MarginOutside  string `json:"margin_outside"`
	Bleed          string `json:"bleed"`
	FontFamily     string `json:"font_family"`
	Layout         string `json:"layout"`
}

type interiorConfig struct {
	absManuscript string
	absImages     string
	absOutput     string
	trimW         float64
	trimH         float64
	inside        float64
	outside       float64
	bleed         float64
}

func parseAndSanitizeInterior(compiler *typst.Compiler, args *interiorArgs) (*interiorConfig, error) {
	absManuscript, err := compiler.CheckSandbox(args.ManuscriptPath)
	if err != nil {
		return nil, fmt.Errorf("manuscript path error: %w", err)
	}
	var absImages string
	if !strings.HasSuffix(strings.ToLower(absManuscript), ".typ") {
		if args.ImagesDir == "" {
			return nil, fmt.Errorf("images directory is required for Markdown manuscripts")
		}
		absImages, err = compiler.CheckSandbox(args.ImagesDir)
		if err != nil {
			return nil, fmt.Errorf("images directory error: %w", err)
		}
	}
	absOutput, err := compiler.CheckSandbox(args.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("output path error: %w", err)
	}

	trimW, trimH, err := typst.ParsePageSize(args.PageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to parse page_size: %w", err)
	}
	inside, err := typst.ParseDimension(args.MarginInside)
	if err != nil {
		return nil, fmt.Errorf("failed to parse margin_inside: %w", err)
	}
	outside, err := typst.ParseDimension(args.MarginOutside)
	if err != nil {
		return nil, fmt.Errorf("failed to parse margin_outside: %w", err)
	}
	b, err := typst.ParseDimension(args.Bleed)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bleed: %w", err)
	}

	if args.Layout != "full-bleed" && args.Layout != "facing-pages" && args.Layout != "facing-pages-flipped" {
		return nil, fmt.Errorf("invalid layout %q: must be 'full-bleed', 'facing-pages', or 'facing-pages-flipped'", args.Layout)
	}

	if strings.HasSuffix(strings.ToLower(absManuscript), ".typ") {
		if args.Layout != "full-bleed" {
			return nil, fmt.Errorf("layout option %q is not supported for pure Typst (.typ) manuscripts", args.Layout)
		}
	}

	return &interiorConfig{
		absManuscript: absManuscript,
		absImages:     absImages,
		absOutput:     absOutput,
		trimW:         trimW,
		trimH:         trimH,
		inside:        inside,
		outside:       outside,
		bleed:         b,
	}, nil
}

func handleCompileInterior(compiler *typst.Compiler) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args interiorArgs
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		applyInteriorDefaults(&args)

		ic, pErr := parseAndSanitizeInterior(compiler, &args)
		if pErr != nil {
			return errorResult(pErr.Error()), nil
		}

		var compErr error
		if strings.HasSuffix(strings.ToLower(ic.absManuscript), ".typ") {
			compErr = compileFromTypstFile(ctx, compiler, ic.absManuscript, ic.trimW, ic.trimH, ic.inside, ic.outside, ic.bleed, args.FontFamily, ic.absOutput)
		} else {
			compErr = compileFromMarkdownFile(ctx, compiler, ic.absManuscript, ic.absImages, ic.trimW, ic.trimH, ic.inside, ic.outside, ic.bleed, args.FontFamily, args.Layout, ic.absOutput)
		}

		if compErr != nil {
			return errorResult(fmt.Sprintf("failed to compile Typst interior: %v", compErr)), nil
		}

		pageCount := resolvePageCount(ic.absOutput)
		res := map[string]interface{}{
			"output_pdf": ic.absOutput,
			"page_count": pageCount,
		}

		data, jErr := json.MarshalIndent(res, "", "  ")
		if jErr != nil {
			return nil, jErr
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func applyInteriorDefaults(args *interiorArgs) {
	if args.PageSize == "" {
		args.PageSize = "8.5in,8.5in"
	}
	if args.MarginInside == "" {
		args.MarginInside = "0.375in"
	}
	if args.MarginOutside == "" {
		args.MarginOutside = "0.25in"
	}
	if args.Bleed == "" {
		args.Bleed = "0.125in"
	}
	if args.FontFamily == "" {
		args.FontFamily = "Georgia"
	}
	if args.Layout == "" {
		args.Layout = "full-bleed"
	}
}

func compileFromTypstFile(ctx context.Context, compiler *typst.Compiler, absManuscript string, trimW, trimH, inside, outside, b float64, fontFamily, absOutput string) error {
	//nolint:gosec // absManuscript is validated inside c.CheckSandbox
	contentBytes, err := os.ReadFile(absManuscript)
	if err != nil {
		return err
	}

	pageSetBlock := fmt.Sprintf("#set page(\n  width: %.4fin,\n  height: %.4fin,\n  margin: (\n    top: %.4fin,\n    bottom: %.4fin,\n    inside: %.4fin,\n    outside: %.4fin,\n  )\n)\n#set text(font: %q)\n",
		trimW+(b*2),
		trimH+(b*2),
		0.5+b,
		0.5+b,
		inside+b,
		outside+b,
		fontFamily,
	)

	return compiler.Compile(ctx, pageSetBlock+"\n"+string(contentBytes), absOutput)
}

func compileFromMarkdownFile(ctx context.Context, compiler *typst.Compiler, absManuscript, absImages string, trimW, trimH, inside, outside, b float64, fontFamily, layout, absOutput string) error {
	pages, err := typst.ParseManuscript(absManuscript)
	if err != nil {
		return err
	}

	var interiorPages []typst.InteriorPage
	for _, p := range pages {
		ip := typst.InteriorPage{
			Text:   p.Text,
			Layout: p.Layout,
		}

		imgFile, imgErr := typst.FindImageForPage(absImages, p.Number)
		if imgErr == nil {
			ip.ImagePath = imgFile
		}

		interiorPages = append(interiorPages, ip)
	}

	typstCode, err := typst.RenderInterior(typst.InteriorParams{
		PhysicalWidth:  trimW + (b * 2),
		PhysicalHeight: trimH + (b * 2),
		MarginTop:      0.5 + b,
		MarginBottom:   0.5 + b,
		MarginInside:   inside + b,
		MarginOutside:  outside + b,
		FontFamily:     fontFamily,
		Pages:          interiorPages,
		Layout:         layout,
	})
	if err != nil {
		return err
	}

	return compiler.Compile(ctx, typstCode, absOutput)
}

func resolvePageCount(absOutput string) int {
	pageCount := 0
	//nolint:gosec // absOutput is validated inside c.CheckSandbox
	f, err := os.Open(absOutput)
	if err == nil {
		defer func() { _ = f.Close() }()
		fi, statErr := f.Stat()
		if statErr == nil {
			r, rErr := pdf.NewReader(f, fi.Size())
			if rErr == nil {
				pageCount = r.NumPage()
			}
		}
	}
	return pageCount
}

type coverArgs struct {
	FrontImagePath string `json:"front_image_path"`
	Title          string `json:"title"`
	Subtitle       string `json:"subtitle"`
	Author         string `json:"author"`
	PageCount      int    `json:"page_count"`
	OutputPath     string `json:"output_path"`
	TrimSize       string `json:"trim_size"`
	PaperType      string `json:"paper_type"`
	BindingType    string `json:"binding_type"`
	FontFamily     string `json:"font_family"`
}

func handleCompileCover(compiler *typst.Compiler) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args coverArgs
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		applyCoverDefaults(&args)

		absFrontImg, pErr := compiler.CheckSandbox(args.FrontImagePath)
		if pErr != nil {
			return errorResult(fmt.Sprintf("front image path error: %v", pErr)), nil
		}
		absOutput, pErr := compiler.CheckSandbox(args.OutputPath)
		if pErr != nil {
			return errorResult(fmt.Sprintf("output path error: %v", pErr)), nil
		}

		// Calculate exact KDP dimensions using our shared library kdpmath
		geom, pErr := kdpmath.CalculateGeometry(args.PageCount, args.BindingType, args.PaperType, args.TrimSize)
		if pErr != nil {
			return errorResult(fmt.Sprintf("geometry calculation failed: %v", pErr)), nil
		}

		// Set paperback parameters
		backCoverWidth := geom.TrimWidthInches + 0.125
		spineWidth := geom.SpineWidthInches
		frontCoverDx := backCoverWidth + spineWidth
		frontCoverWidth := geom.TrimWidthInches + 0.125

		typstCode, pErr := typst.RenderCover(typst.CoverParams{
			PhysicalWidth:     geom.CoverWidthInches,
			PhysicalHeight:    geom.CoverHeightInches,
			BackCoverWidth:    backCoverWidth,
			SpineWidth:        spineWidth,
			SpineTextEligible: geom.SpineTextEligible,
			FrontCoverDx:      frontCoverDx,
			FrontCoverWidth:   frontCoverWidth,
			FrontImagePath:    absFrontImg,
			Title:             args.Title,
			Subtitle:          args.Subtitle,
			Author:            args.Author,
			FontFamily:        args.FontFamily,
		})
		if pErr != nil {
			return errorResult(fmt.Sprintf("failed to render cover template: %v", pErr)), nil
		}

		if pErr = compiler.Compile(ctx, typstCode, absOutput); pErr != nil {
			return errorResult(fmt.Sprintf("failed to compile cover Typst: %v", pErr)), nil
		}

		res := map[string]interface{}{
			"output_pdf": absOutput,
		}

		data, jErr := json.MarshalIndent(res, "", "  ")
		if jErr != nil {
			return nil, jErr
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func applyCoverDefaults(args *coverArgs) {
	if args.TrimSize == "" {
		args.TrimSize = "8.5x8.5"
	}
	if args.PaperType == "" {
		args.PaperType = "white"
	}
	if args.BindingType == "" {
		args.BindingType = "paperback"
	}
	if args.FontFamily == "" {
		args.FontFamily = "Georgia"
	}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}
}
