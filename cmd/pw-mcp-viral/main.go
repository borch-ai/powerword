package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/viral"
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

	srv, err := setupServer(workspaceRoot, cfg, nil)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

const (
	generateVoiceoverSchema = `{
		"type": "object",
		"properties": {
			"script": {
				"type": "string",
				"description": "The script text to narrate."
			},
			"voice_id": {
				"type": "string",
				"description": "Optional voice ID to override the default voice."
			},
			"provider": {
				"type": "string",
				"description": "Optional provider override ('openai', 'elevenlabs', or 'mock')."
			}
		},
		"required": ["script"]
	}`

	generateVideoSchema = `{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "The visual prompt for the background video clip."
			},
			"size": {
				"type": "string",
				"description": "Optional video dimensions (e.g., '1024x1792' or '1024x1024')."
			}
		},
		"required": ["prompt"]
	}`

	stitchTrailerSchema = `{
		"type": "object",
		"properties": {
			"video_path": {
				"type": "string",
				"description": "Absolute path to the background video clip."
			},
			"audio_path": {
				"type": "string",
				"description": "Absolute path to the voiceover narration track."
			},
			"background_audio_path": {
				"type": "string",
				"description": "Optional absolute path to background music/audio track."
			},
			"output_name": {
				"type": "string",
				"description": "Optional custom name for the output MP4 file."
			}
		},
		"required": ["video_path", "audio_path"]
	}`

	stitchSlideshowSchema = `{
		"type": "object",
		"properties": {
			"slides": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"image_path": {
							"type": "string",
							"description": "Absolute path to the slide illustration."
						},
						"audio_path": {
							"type": "string",
							"description": "Absolute path to the voiceover narration for that slide."
						}
					},
					"required": ["image_path", "audio_path"]
				},
				"description": "List of slides to stitch together."
			},
			"background_audio_path": {
				"type": "string",
				"description": "Optional absolute path to background music track."
			},
			"output_name": {
				"type": "string",
				"description": "Optional custom name for the output MP4 file."
			}
		},
		"required": ["slides"]
	}`
)

func setupServer(workspaceRoot string, cfg *config.Config, svc *viral.ViralService) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-viral",
		Version: "1.0.0",
	}, nil)

	viralService := svc
	if viralService == nil {
		viralService = viral.NewViralService(workspaceRoot, cfg)
	}

	srv.AddTool(&mcp.Tool{
		Name:        "viral_generate_voiceover",
		Description: "Synthesizes script narration using configured TTS providers.",
		InputSchema: json.RawMessage(generateVoiceoverSchema),
	}, handleGenerateVoiceover(viralService))

	srv.AddTool(&mcp.Tool{
		Name:        "viral_generate_video",
		Description: "Triggers background generative video tasks.",
		InputSchema: json.RawMessage(generateVideoSchema),
	}, handleGenerateVideo(viralService))

	srv.AddTool(&mcp.Tool{
		Name:        "viral_stitch_trailer",
		Description: "Stitches background video, main voiceover audio, and optional background music into an MP4 file using a local ffmpeg installation.",
		InputSchema: json.RawMessage(stitchTrailerSchema),
	}, handleStitchTrailer(viralService))

	srv.AddTool(&mcp.Tool{
		Name:        "viral_stitch_slideshow",
		Description: "Stitches sequences of paired images and audio narration segments into a single unified MP4 video trailer using a local ffmpeg installation.",
		InputSchema: json.RawMessage(stitchSlideshowSchema),
	}, handleStitchSlideshow(viralService))

	return srv, nil
}

func handleGenerateVoiceover(svc *viral.ViralService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Script   string `json:"script"`
			VoiceID  string `json:"voice_id"`
			Provider string `json:"provider"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Script == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "script parameter is required"}},
			}, nil
		}

		filePath, err := svc.GenerateVoiceover(ctx, args.Script, args.VoiceID, args.Provider)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to generate voiceover: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully generated voiceover and saved to: %s", filePath)}},
		}, nil
	}
}

func handleGenerateVideo(svc *viral.ViralService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Prompt string `json:"prompt"`
			Size   string `json:"size"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Prompt == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "prompt parameter is required"}},
			}, nil
		}

		filePath, err := svc.GenerateVideo(ctx, args.Prompt, args.Size)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to generate video: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully generated background video and saved to: %s", filePath)}},
		}, nil
	}
}

func handleStitchTrailer(svc *viral.ViralService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			VideoPath           string `json:"video_path"`
			AudioPath           string `json:"audio_path"`
			BackgroundAudioPath string `json:"background_audio_path"`
			OutputName          string `json:"output_name"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.VideoPath == "" || args.AudioPath == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "video_path and audio_path parameters are required"}},
			}, nil
		}

		filePath, err := svc.StitchTrailer(ctx, args.VideoPath, args.AudioPath, args.BackgroundAudioPath, args.OutputName)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to stitch trailer: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully stitched trailer and saved to: %s", filePath)}},
		}, nil
	}
}

func handleStitchSlideshow(svc *viral.ViralService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Slides              []viral.Slide `json:"slides"`
			BackgroundAudioPath string        `json:"background_audio_path"`
			OutputName          string        `json:"output_name"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if len(args.Slides) == 0 {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "slides parameter is required and cannot be empty"}},
			}, nil
		}

		filePath, err := svc.StitchSlideshow(ctx, args.Slides, args.BackgroundAudioPath, args.OutputName)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to stitch slideshow: %v", err)}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully stitched slideshow and saved to: %s", filePath)}},
		}, nil
	}
}
