package youtube

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/borch-ai/powerword/pkg/config"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func validateBaseURL(envURL, defaultValue string) string {
	if envURL == "" {
		return defaultValue
	}
	parsed, err := url.Parse(envURL)
	if err != nil {
		return defaultValue
	}
	host := parsed.Hostname()
	scheme := parsed.Scheme
	if (scheme == "http" || scheme == "https") && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return strings.TrimSuffix(envURL, "/")
	}
	return defaultValue
}

// YouTubeService abstracts the integration with the YouTube v3 Data API.
type YouTubeService struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewYouTubeService creates a new instance of YouTubeService.
func NewYouTubeService(cfg *config.Config, httpClient *http.Client) *YouTubeService {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &YouTubeService{
		cfg:        cfg,
		httpClient: httpClient,
	}
}

// getClient builds an authenticated YouTube service client using the OAuth2 refresh token flow.
func (s *YouTubeService) getClient(ctx context.Context) (*youtube.Service, error) {
	clientID := s.cfg.Plugins.YouTube.ClientID
	clientSecret := s.cfg.Plugins.YouTube.ClientSecret
	refreshToken := s.cfg.Plugins.YouTube.RefreshToken

	if clientID == "" || clientSecret == "" || refreshToken == "" {
		return nil, errors.New("missing required YouTube credentials (client_id, client_secret, refresh_token)")
	}

	//nolint:gosec // Standard Google OAuth2 endpoints are not hardcoded credentials
	oauthTokenURL := "https://oauth2.googleapis.com/token"
	if ep := validateBaseURL(os.Getenv("POWERWORD_YOUTUBE_API_ENDPOINT"), ""); ep != "" {
		oauthTokenURL = ep + "/oauth2/token"
	}

	//nolint:gosec // Standard Google OAuth2 endpoints are not hardcoded credentials
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: oauthTokenURL,
		},
		Scopes: []string{
			youtube.YoutubeUploadScope,
			youtube.YoutubeScope,
		},
	}

	token := &oauth2.Token{
		RefreshToken: refreshToken,
	}

	// Use user-supplied httpClient (usually for testing/mocking) if provided
	client := s.httpClient
	if client == nil {
		client = conf.Client(ctx, token)
	}

	opts := []option.ClientOption{option.WithHTTPClient(client)}
	if ep := validateBaseURL(os.Getenv("POWERWORD_YOUTUBE_API_ENDPOINT"), ""); ep != "" {
		opts = append(opts, option.WithEndpoint(ep))
	}

	srv, err := youtube.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create YouTube service: %w", err)
	}
	return srv, nil
}

// UploadVideo handles a chunked/resumable video file upload to YouTube and returns the new Video ID.
func (s *YouTubeService) UploadVideo(ctx context.Context, videoPath, title, description, privacy string) (string, error) {
	if videoPath == "" {
		return "", errors.New("video_path is required")
	}

	// Validate file existence and type
	info, err := os.Stat(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to access video file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("video_path is a directory: %s", videoPath)
	}

	ext := strings.ToLower(filepath.Ext(videoPath))
	if ext != ".mp4" && ext != ".mkv" && ext != ".mov" && ext != ".avi" {
		return "", fmt.Errorf("unsupported video format: %s. Supported formats: .mp4, .mkv, .mov, .avi", ext)
	}

	srv, err := s.getClient(ctx)
	if err != nil {
		return "", err
	}

	//nolint:gosec // videoPath is validated to exist and be a file
	file, err := os.Open(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open video file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	if privacy == "" {
		privacy = "private"
	}

	video := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       title,
			Description: description,
		},
		Status: &youtube.VideoStatus{
			PrivacyStatus: privacy,
		},
	}

	call := srv.Videos.Insert([]string{"snippet", "status"}, video)

	// Build chunked media upload options
	var mediaOpts []googleapi.MediaOption
	// Set 5MB chunk size (minimum YouTube chunk size is 256KB, 5MB is standard/reasonable)
	mediaOpts = append(mediaOpts, googleapi.ChunkSize(5*1024*1024))

	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		switch ext {
		case ".mp4":
			contentType = "video/mp4"
		case ".mkv":
			contentType = "video/x-matroska"
		case ".mov":
			contentType = "video/quicktime"
		case ".avi":
			contentType = "video/x-msvideo"
		default:
			contentType = "application/octet-stream"
		}
	}
	mediaOpts = append(mediaOpts, googleapi.ContentType(contentType))

	res, err := call.Media(file, mediaOpts...).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to upload video: %w", err)
	}

	return res.Id, nil
}

// UpdateMetadata updates an existing video's snippet details (title, description, tags)
// and optionally adds the video to a playlist.
func (s *YouTubeService) UpdateMetadata(ctx context.Context, videoID, title, description string, tags []string, playlistID string) error {
	if videoID == "" {
		return errors.New("video_id is required")
	}

	srv, err := s.getClient(ctx)
	if err != nil {
		return err
	}

	// Retrieve existing video snippet to preserve other fields (e.g. category ID)
	res, err := srv.Videos.List([]string{"snippet"}).Id(videoID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to retrieve video: %w", err)
	}

	if len(res.Items) == 0 {
		return fmt.Errorf("video not found: %s", videoID)
	}

	video := res.Items[0]

	if title != "" {
		video.Snippet.Title = title
	}
	if description != "" {
		video.Snippet.Description = description
	}
	if len(tags) > 0 {
		video.Snippet.Tags = tags
	}

	_, err = srv.Videos.Update([]string{"snippet"}, video).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to update video metadata: %w", err)
	}

	if playlistID != "" {
		playlistItem := &youtube.PlaylistItem{
			Snippet: &youtube.PlaylistItemSnippet{
				PlaylistId: playlistID,
				ResourceId: &youtube.ResourceId{
					Kind:    "youtube#video",
					VideoId: videoID,
				},
			},
		}
		_, err = srv.PlaylistItems.Insert([]string{"snippet"}, playlistItem).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to add video to playlist %s: %w", playlistID, err)
		}
	}

	return nil
}

// GetMetrics queries basic public stats (views, likes, comments, favorites) for a video.
func (s *YouTubeService) GetMetrics(ctx context.Context, videoID string, metrics []string) (map[string]interface{}, error) {
	if videoID == "" {
		return nil, errors.New("video_id is required")
	}

	srv, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}

	res, err := srv.Videos.List([]string{"statistics"}).Id(videoID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve video statistics: %w", err)
	}

	if len(res.Items) == 0 {
		return nil, fmt.Errorf("video not found: %s", videoID)
	}

	video := res.Items[0]
	stats := video.Statistics
	if stats == nil {
		return nil, fmt.Errorf("no statistics available for video: %s", videoID)
	}

	result := make(map[string]interface{})

	if len(metrics) == 0 {
		metrics = []string{"views", "likes", "comments", "favorites"}
	}

	for _, m := range metrics {
		switch strings.ToLower(m) {
		case "views", "viewcount":
			result["views"] = stats.ViewCount
		case "likes", "likecount":
			result["likes"] = stats.LikeCount
		case "comments", "commentcount":
			result["comments"] = stats.CommentCount
		case "favorites", "favoritecount":
			result["favorites"] = stats.FavoriteCount
		default:
			result[m] = nil
		}
	}

	return result, nil
}
