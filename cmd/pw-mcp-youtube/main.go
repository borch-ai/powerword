package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/borch-ai/powerword/internal/plugins/youtube"
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
	uploadVideoSchema = `{
		"type": "object",
		"properties": {
			"video_path": {
				"type": "string",
				"description": "Absolute or relative path to the video file (.mp4, .mkv, .mov, .avi)."
			},
			"title": {
				"type": "string",
				"description": "The title of the YouTube video."
			},
			"description": {
				"type": "string",
				"description": "The description of the YouTube video."
			},
			"privacy": {
				"type": "string",
				"enum": ["public", "unlisted", "private"],
				"description": "Privacy status of the video (defaults to 'private')."
			}
		},
		"required": ["video_path"]
	}`

	updateMetadataSchema = `{
		"type": "object",
		"properties": {
			"video_id": {
				"type": "string",
				"description": "The YouTube Video ID to update."
			},
			"title": {
				"type": "string",
				"description": "Optional new title."
			},
			"description": {
				"type": "string",
				"description": "Optional new description."
			},
			"tags": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Optional list of tags."
			},
			"playlist_id": {
				"type": "string",
				"description": "Optional Playlist ID to add the video to."
			}
		},
		"required": ["video_id"]
	}`

	getMetricsSchema = `{
		"type": "object",
		"properties": {
			"video_id": {
				"type": "string",
				"description": "The YouTube Video ID to query."
			},
			"metrics": {
				"type": "array",
				"items": {
					"type": "string"
				},
				"description": "Optional list of metrics to retrieve (e.g. 'views', 'likes', 'comments', 'favorites'). If empty, returns all."
			}
		},
		"required": ["video_id"]
	}`
)

func setupServer(workspaceRoot string, cfg *config.Config, svc *youtube.YouTubeService) (*mcp.Server, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-youtube",
		Version: "0.1.0",
	}, nil)

	youtubeService := svc
	if youtubeService == nil {
		youtubeService = youtube.NewYouTubeService(cfg, nil)
	}

	srv.AddTool(&mcp.Tool{
		Name:        "youtube_upload_video",
		Description: "Uploads a video to YouTube using chunked/resumable media upload and returns the created Video ID.",
		InputSchema: json.RawMessage(uploadVideoSchema),
	}, handleUploadVideo(youtubeService, workspaceRoot))

	srv.AddTool(&mcp.Tool{
		Name:        "youtube_update_metadata",
		Description: "Updates video title, description, and tags, and optionally adds the video to a playlist.",
		InputSchema: json.RawMessage(updateMetadataSchema),
	}, handleUpdateMetadata(youtubeService))

	srv.AddTool(&mcp.Tool{
		Name:        "youtube_get_metrics",
		Description: "Retrieves basic statistics (views, likes, comments, favorites) for a video.",
		InputSchema: json.RawMessage(getMetricsSchema),
	}, handleGetMetrics(youtubeService))

	return srv, nil
}

func handleUploadVideo(svc *youtube.YouTubeService, workspaceRoot string) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			VideoPath   string `json:"video_path"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Privacy     string `json:"privacy"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.VideoPath == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "video_path parameter is required"}},
			}, nil
		}

		videoPath := args.VideoPath
		if !filepath.IsAbs(videoPath) {
			videoPath = filepath.Join(workspaceRoot, videoPath)
		}

		videoID, err := svc.UploadVideo(ctx, videoPath, args.Title, args.Description, args.Privacy)
		if err != nil {
			// Redact potential credentials if they are leaked in error string
			errMsg := redactSecrets(err.Error())
			if !strings.HasPrefix(errMsg, "failed to upload video") {
				errMsg = "failed to upload video: " + errMsg
			}
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: errMsg}},
			}, nil
		}

		res := struct {
			VideoID string `json:"video_id"`
		}{
			VideoID: videoID,
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleUpdateMetadata(svc *youtube.YouTubeService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			VideoID     string   `json:"video_id"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
			PlaylistID  string   `json:"playlist_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.VideoID == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "video_id parameter is required"}},
			}, nil
		}

		err := svc.UpdateMetadata(ctx, args.VideoID, args.Title, args.Description, args.Tags, args.PlaylistID)
		if err != nil {
			errMsg := redactSecrets(err.Error())
			if !strings.HasPrefix(errMsg, "failed to update video metadata") {
				errMsg = "failed to update video metadata: " + errMsg
			}
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: errMsg}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Video metadata updated successfully"}},
		}, nil
	}
}

func handleGetMetrics(svc *youtube.YouTubeService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			VideoID string   `json:"video_id"`
			Metrics []string `json:"metrics"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.VideoID == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "video_id parameter is required"}},
			}, nil
		}

		metrics, err := svc.GetMetrics(ctx, args.VideoID, args.Metrics)
		if err != nil {
			errMsg := redactSecrets(err.Error())
			if !strings.HasPrefix(errMsg, "failed to get metrics") {
				errMsg = "failed to get metrics: " + errMsg
			}
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: errMsg}},
			}, nil
		}

		data, err := json.MarshalIndent(metrics, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

// redactSecrets intercepts and redacts sensitive YouTube tokens/secrets from error strings.
func redactSecrets(input string) string {
	redacted := input
	for _, secret := range []string{"client_secret", "client_id", "refresh_token"} {
		redacted = replaceRawSecret(redacted, secret, "=")
		redacted = replaceRawSecret(redacted, secret, ":")
	}
	return redacted
}

func replaceRawSecret(input, secretKey, delimiter string) string {
	idx := 0
	for {
		pos := strings.Index(input[idx:], secretKey)
		if pos == -1 {
			break
		}
		start := idx + pos
		// Find delimiter after the secretKey
		delimPos := strings.Index(input[start+len(secretKey):], delimiter)
		if delimPos == -1 {
			idx = start + len(secretKey)
			continue
		}
		// Check that the delimiter is close to the key (within 5 chars to handle quotes/spaces)
		if delimPos > 5 {
			idx = start + len(secretKey)
			continue
		}

		delimIdx := start + len(secretKey) + delimPos
		end := delimIdx + len(delimiter)

		// Find value start by skipping quotes and spaces
		valStart := end
		for valStart < len(input) && (input[valStart] == ' ' || input[valStart] == '"' || input[valStart] == '\'') {
			valStart++
		}

		// Find value end (next delimiter or quote or end of string)
		valEnd := valStart
		for valEnd < len(input) && input[valEnd] != '&' && input[valEnd] != '"' && input[valEnd] != '\'' && input[valEnd] != ',' && input[valEnd] != '}' && input[valEnd] != ' ' && input[valEnd] != '\n' && input[valEnd] != '\t' {
			valEnd++
		}

		if valEnd > valStart {
			input = input[:valStart] + "[REDACTED]" + input[valEnd:]
		}
		idx = valStart + len("[REDACTED]")
		if idx >= len(input) {
			break
		}
	}
	return input
}
