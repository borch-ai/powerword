package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/imagegen"
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

func setupServer(workspaceRoot string, cfg *config.Config) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-imagegen",
		Version: "1.0.0",
	}, nil)

	service := imagegen.NewImageGenService(workspaceRoot, cfg)

	srv.AddTool(&mcp.Tool{
		Name:        "imagegen_generate",
		Description: "Triggers an image generation based on a text prompt, size, and style reference ID (supporting OpenAI DALL-E 3, Google Imagen 3, and custom Midjourney backends). The generated image will be saved to the workspace root directory inside a generated_images/ subfolder.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"prompt": {
					"type": "string",
					"description": "The description of the image to generate"
				},
				"size": {
					"type": "string",
					"description": "Image size (e.g. '1024x1024' or '1024x1792' or '1792x1024')"
				},
				"style_id": {
					"type": "string",
					"description": "ID of a registered style profile to apply"
				}
			},
			"required": ["prompt"]
		}`),
	}, handleGenerate(service))

	srv.AddTool(&mcp.Tool{
		Name:        "imagegen_register_style",
		Description: "Saves a visual style definition (prompt seeds or sref URLs) under a semantic ID.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"style_id": {
					"type": "string",
					"description": "Unique semantic ID for the style profile"
				},
				"prompt_seed": {
					"type": "string",
					"description": "Description of the style or style prompt seeds to append to the generation prompt"
				},
				"sref_url": {
					"type": "string",
					"description": "Optional image URL style reference (--sref) for Midjourney wrapper backends"
				}
			},
			"required": ["style_id", "prompt_seed"]
		}`),
	}, handleRegisterStyle(service))

	srv.AddTool(&mcp.Tool{
		Name:        "imagegen_list_styles",
		Description: "Lists all registered style profiles.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, handleListStyles(service))

	return srv, nil
}

func handleGenerate(service *imagegen.ImageGenService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Prompt  string `json:"prompt"`
			Size    string `json:"size"`
			StyleID string `json:"style_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		filePath, err := service.GenerateImage(ctx, args.Prompt, args.Size, args.StyleID)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to generate image: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully generated image and saved to: %s", filePath)}},
		}, nil
	}
}

func handleRegisterStyle(service *imagegen.ImageGenService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			StyleID    string `json:"style_id"`
			PromptSeed string `json:"prompt_seed"`
			SrefURL    string `json:"sref_url"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		style := imagegen.StyleProfile{
			StyleID:    args.StyleID,
			PromptSeed: args.PromptSeed,
			SrefURL:    args.SrefURL,
		}

		if err := service.StyleStore().Register(style); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to register style profile: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully registered style profile %q", args.StyleID)}},
		}, nil
	}
}

func handleListStyles(service *imagegen.ImageGenService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		styles, err := service.StyleStore().List()
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list style profiles: %v", err)}},
			}, nil
		}

		if len(styles) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "No registered style profiles found."}},
			}, nil
		}

		data, err := json.MarshalIndent(styles, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}
