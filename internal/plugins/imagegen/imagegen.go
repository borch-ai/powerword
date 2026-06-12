package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sashabaranov/go-openai"

	"github.com/borch-ai/powerword/pkg/config"
)

// StyleProfile stores style definitions.
type StyleProfile struct {
	StyleID    string `json:"style_id"`
	PromptSeed string `json:"prompt_seed"`
	SrefURL    string `json:"sref_url,omitempty"`
}

// StyleStore manages registered styles saved in the workspace.
type StyleStore struct {
	mu       sync.RWMutex
	filePath string
}

// NewStyleStore initializes a style store at <workspaceRoot>/.powerword/imagegen_styles.json
func NewStyleStore(workspaceRoot string) *StyleStore {
	return &StyleStore{
		filePath: filepath.Join(workspaceRoot, ".powerword", "imagegen_styles.json"),
	}
}

// Load reads and parses style profiles from storage.
func (s *StyleStore) Load() (map[string]StyleProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return make(map[string]StyleProfile), nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read styles file: %w", err)
	}

	var styles map[string]StyleProfile
	if err := json.Unmarshal(data, &styles); err != nil {
		return nil, fmt.Errorf("failed to unmarshal styles: %w", err)
	}
	return styles, nil
}

// Save writes styles back to storage.
func (s *StyleStore) Save(styles map[string]StyleProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(styles, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal styles: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write styles file: %w", err)
	}
	return nil
}

// Register adds or updates a style profile.
func (s *StyleStore) Register(style StyleProfile) error {
	styles, err := s.Load()
	if err != nil {
		return err
	}
	styles[style.StyleID] = style
	return s.Save(styles)
}

// Get retrieves a style profile.
func (s *StyleStore) Get(styleID string) (StyleProfile, bool, error) {
	styles, err := s.Load()
	if err != nil {
		return StyleProfile{}, false, err
	}
	style, exists := styles[styleID]
	return style, exists, nil
}

// List returns a slice of all registered style profiles.
func (s *StyleStore) List() ([]StyleProfile, error) {
	styles, err := s.Load()
	if err != nil {
		return nil, err
	}
	list := make([]StyleProfile, 0, len(styles))
	for _, style := range styles {
		list = append(list, style)
	}
	return list, nil
}

// OpenAIBackend implements DALL-E 3 image generation.
type OpenAIBackend struct {
	client *openai.Client
}

// NewOpenAIBackend creates a new OpenAI image generator wrapper.
func NewOpenAIBackend(apiKey string) *OpenAIBackend {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &OpenAIBackend{
		client: openai.NewClientWithConfig(cfg),
	}
}

// GenerateImage requests image URL from DALL-E 3.
func (b *OpenAIBackend) GenerateImage(ctx context.Context, prompt string, size string) (string, error) {
	if size == "" {
		size = "1024x1024"
	}
	req := openai.ImageRequest{
		Prompt: prompt,
		Size:   size,
		N:      1,
		Model:  openai.CreateImageModelDallE3,
	}

	resp, err := b.client.CreateImage(ctx, req)
	if err != nil {
		return "", fmt.Errorf("openai error: %w", err)
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("openai returned no image URL data")
	}
	return resp.Data[0].URL, nil
}

// GoogleBackend implements Imagen 3 image generation using Google AI Studio predict REST API.
type GoogleBackend struct {
	apiURL string
	apiKey string
	model  string
	client *http.Client
}

// NewGoogleBackend creates a new Google prediction API client wrapper.
func NewGoogleBackend(apiKey, model string) *GoogleBackend {
	if model == "" {
		model = "imagen-3.0-generate-002"
	}
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:predict", model)
	if baseURL := os.Getenv("GOOGLE_BASE_URL"); baseURL != "" {
		apiURL = fmt.Sprintf("%s/v1beta/models/%s:predict", baseURL, model)
	}
	return &GoogleBackend{
		apiURL: apiURL,
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// GenerateImage generates the image via direct predict REST call and returns decoded raw bytes and mimeType.
func (b *GoogleBackend) GenerateImage(ctx context.Context, prompt string, size string) ([]byte, string, error) {
	aspectRatio := "1:1"
	switch size {
	case "1024x1792":
		aspectRatio = "9:16"
	case "1792x1024":
		aspectRatio = "16:9"
	}

	payload := map[string]interface{}{
		"instances": []map[string]string{
			{
				"prompt": prompt,
			},
		},
		"parameters": map[string]interface{}{
			"sampleCount": 1,
			"aspectRatio": aspectRatio,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal Google payload: %w", err)
	}

	reqURL := fmt.Sprintf("%s?key=%s", b.apiURL, b.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Google request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("google predict request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("google backend predict returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}

	var responseMap map[string]interface{}
	err = json.Unmarshal(bodyBytes, &responseMap)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode Google response: %w", err)
	}

	predictions, ok := responseMap["predictions"].([]interface{})
	if !ok || len(predictions) == 0 {
		return nil, "", fmt.Errorf("no predictions returned by Google: %s", string(bodyBytes))
	}

	prediction, ok := predictions[0].(map[string]interface{})
	if !ok {
		return nil, "", fmt.Errorf("invalid prediction structure: %s", string(bodyBytes))
	}

	base64Data, ok := prediction["bytesBase64Encoded"].(string)
	if !ok || base64Data == "" {
		return nil, "", fmt.Errorf("missing bytesBase64Encoded: %s", string(bodyBytes))
	}

	mimeType, _ := prediction["mimeType"].(string)
	if mimeType == "" {
		mimeType = "image/png"
	}

	imgBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode base64 image bytes: %w", err)
	}

	return imgBytes, mimeType, nil
}

// VeoBackend implements Google Veo video generation.
type VeoBackend struct {
	apiURL          string
	apiKey          string
	model           string
	pollingInterval time.Duration
	pollingTimeout  time.Duration
	client          *http.Client
}

// NewVeoBackend creates a new Veo API client wrapper.
func NewVeoBackend(apiKey, model, intervalStr, timeoutStr string) (*VeoBackend, error) {
	if model == "" {
		model = "veo-2.0-generate-001"
	}
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:predictLongRunning", model)
	if baseURL := os.Getenv("GOOGLE_BASE_URL"); baseURL != "" {
		apiURL = fmt.Sprintf("%s/v1beta/models/%s:predictLongRunning", baseURL, model)
	}

	interval := 10 * time.Second
	if intervalStr != "" {
		if d, err := time.ParseDuration(intervalStr); err == nil {
			interval = d
		}
	}
	timeout := 5 * time.Minute
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	return &VeoBackend{
		apiURL:          apiURL,
		apiKey:          apiKey,
		model:           model,
		pollingInterval: interval,
		pollingTimeout:  timeout,
		client:          &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (b *VeoBackend) downloadVideo(ctx context.Context, videoURI string) ([]byte, error) {
	var downloadURL string
	if strings.Contains(videoURI, "?") {
		downloadURL = fmt.Sprintf("%s&key=%s&alt=media", videoURI, b.apiKey)
	} else {
		downloadURL = fmt.Sprintf("%s?key=%s&alt=media", videoURI, b.apiKey)
	}

	//nolint:gosec // downloadURL is verified and retrieved from Google API response
	dlReq, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	//nolint:gosec // request is sent to trusted Google resource
	dlResp, err := b.client.Do(dlReq)
	if err != nil {
		return nil, fmt.Errorf("failed to download video content: %w", err)
	}
	defer func() { _ = dlResp.Body.Close() }()

	if dlResp.StatusCode != http.StatusOK {
		dlBytes, _ := io.ReadAll(dlResp.Body)
		return nil, fmt.Errorf("failed to download video content, status %d: %s", dlResp.StatusCode, string(dlBytes))
	}

	videoBytes, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read downloaded video content: %w", err)
	}

	return videoBytes, nil
}

// GenerateImage generates the video via predictLongRunning API and polls until complete.
// It returns the video bytes and "video/mp4" mimeType.
func (b *VeoBackend) GenerateImage(ctx context.Context, prompt string, size string) ([]byte, string, error) {
	opName, err := b.initiateVeo(ctx, prompt, size)
	if err != nil {
		return nil, "", err
	}
	videoBytes, err := b.pollVeo(ctx, opName)
	if err != nil {
		return nil, "", err
	}
	return videoBytes, "video/mp4", nil
}

func (b *VeoBackend) initiateVeo(ctx context.Context, prompt, size string) (string, error) {
	aspectRatio := "1:1"
	switch size {
	case "1024x1792":
		aspectRatio = "9:16"
	case "1792x1024":
		aspectRatio = "16:9"
	}

	payload := map[string]interface{}{
		"instances": []map[string]string{
			{
				"prompt": prompt,
			},
		},
		"parameters": map[string]interface{}{
			"sampleCount": 1,
			"aspectRatio": aspectRatio,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Veo payload: %w", err)
	}

	reqURL := fmt.Sprintf("%s?key=%s", b.apiURL, b.apiKey)
	//nolint:gosec // reqURL is internally validated and constructed from model identifier
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create Veo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	//nolint:gosec // request goes to trusted Google model API
	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("veo initiate request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("veo backend returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var initResp struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(bodyBytes, &initResp); err != nil {
		return "", fmt.Errorf("failed to decode Veo response: %w", err)
	}

	if initResp.Name == "" {
		return "", fmt.Errorf("missing operation name in response: %s", string(bodyBytes))
	}

	return initResp.Name, nil
}

func (b *VeoBackend) pollOnceVeo(ctx context.Context, opName string) (bool, []byte, error) {
	opURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/%s?key=%s", strings.TrimPrefix(opName, "/"), b.apiKey)
	if baseURL := os.Getenv("GOOGLE_BASE_URL"); baseURL != "" {
		opURL = fmt.Sprintf("%s/v1beta/%s?key=%s", baseURL, strings.TrimPrefix(opName, "/"), b.apiKey)
	}

	//nolint:gosec // opURL is internally verified and constructed from trusted operation name
	pollReq, err := http.NewRequestWithContext(ctx, "GET", opURL, nil)
	if err != nil {
		return false, nil, err
	}

	//nolint:gosec // request is sent to trusted Google resource
	pollResp, err := b.client.Do(pollReq)
	if err != nil {
		return false, nil, nil // return no error to retry
	}

	pBytes, err := io.ReadAll(pollResp.Body)
	_ = pollResp.Body.Close()
	if err != nil {
		return false, nil, nil // retry
	}

	if pollResp.StatusCode != http.StatusOK {
		return false, nil, nil // retry
	}

	var opStatus struct {
		Done  bool `json:"done"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
		Response *struct {
			GeneratedVideos []struct {
				Video struct {
					URI string `json:"uri"`
				} `json:"video"`
			} `json:"generatedVideos"`
		} `json:"response,omitempty"`
	}

	if err := json.Unmarshal(pBytes, &opStatus); err != nil {
		return false, nil, nil // retry
	}

	if opStatus.Error != nil {
		return false, nil, fmt.Errorf("veo generation failed: code %d, message %s", opStatus.Error.Code, opStatus.Error.Message)
	}

	if opStatus.Done {
		if opStatus.Response == nil || len(opStatus.Response.GeneratedVideos) == 0 {
			return false, nil, fmt.Errorf("veo completed but returned no video metadata: %s", string(pBytes))
		}
		videoURI := opStatus.Response.GeneratedVideos[0].Video.URI
		if videoURI == "" {
			return false, nil, fmt.Errorf("veo completed but video URI is empty: %s", string(pBytes))
		}

		videoBytes, err := b.downloadVideo(ctx, videoURI)
		if err != nil {
			return false, nil, err
		}
		return true, videoBytes, nil
	}

	return false, nil, nil
}

func (b *VeoBackend) pollVeo(ctx context.Context, opName string) ([]byte, error) {
	ticker := time.NewTicker(b.pollingInterval)
	defer ticker.Stop()

	timeoutChan := time.After(b.pollingTimeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutChan:
			return nil, fmt.Errorf("polling timed out after %v", b.pollingTimeout)
		case <-ticker.C:
			done, videoBytes, err := b.pollOnceVeo(ctx, opName)
			if err != nil {
				return nil, err
			}
			if done {
				return videoBytes, nil
			}
		}
	}
}

// MidjourneyBackend implements custom/wrapper Midjourney webhook and polling API.
type MidjourneyBackend struct {
	apiURL          string
	apiKey          string
	pollingInterval time.Duration
	pollingTimeout  time.Duration
	httpClient      *http.Client
}

// NewMidjourneyBackend creates a custom API polling client wrapper.
func NewMidjourneyBackend(apiURL, apiKey, intervalStr, timeoutStr string) (*MidjourneyBackend, error) {
	interval := 5 * time.Second
	if intervalStr != "" {
		if d, err := time.ParseDuration(intervalStr); err == nil {
			interval = d
		}
	}
	timeout := 5 * time.Minute
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}
	return &MidjourneyBackend{
		apiURL:          apiURL,
		apiKey:          apiKey,
		pollingInterval: interval,
		pollingTimeout:  timeout,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func extractTaskID(m map[string]interface{}) string {
	for _, key := range []string{"id", "generationId", "generation_id", "task_id", "taskId", "result_id"} {
		if val, ok := m[key]; ok {
			if s, ok := val.(string); ok {
				return s
			}
		}
	}
	return ""
}

func extractStatus(m map[string]interface{}) string {
	for _, key := range []string{"status", "state", "status_code"} {
		if val, ok := m[key]; ok {
			if s, ok := val.(string); ok {
				return strings.ToLower(s)
			}
		}
	}
	return ""
}

func extractImageURL(m map[string]interface{}) string {
	for _, key := range []string{"imageUrl", "image_url", "url", "image"} {
		if val, ok := m[key]; ok {
			if s, ok := val.(string); ok {
				return s
			}
		}
	}
	return ""
}

func extractStatusURL(m map[string]interface{}, apiURL, taskID string) string {
	statusURL := apiURL + "/" + taskID
	if val, ok := m["statusUrl"]; ok {
		if s, ok := val.(string); ok && s != "" {
			statusURL = s
		}
	} else if val, ok := m["status_url"]; ok {
		if s, ok := val.(string); ok && s != "" {
			statusURL = s
		}
	}

	if strings.HasPrefix(statusURL, "/") {
		if u, err := url.Parse(apiURL); err == nil {
			statusURL = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, statusURL)
		}
	}
	return statusURL
}

func (b *MidjourneyBackend) pollOnce(ctx context.Context, statusURL string) (string, string, error) {
	pollReq, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
	if err != nil {
		return "", "", err
	}
	if b.apiKey != "" {
		pollReq.Header.Set("Authorization", "Bearer "+b.apiKey)
	}

	pollResp, err := b.httpClient.Do(pollReq)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = pollResp.Body.Close() }()

	if pollResp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("bad status code: %d", pollResp.StatusCode)
	}

	pollBody, err := io.ReadAll(pollResp.Body)
	if err != nil {
		return "", "", err
	}

	var pollMap map[string]interface{}
	if err := json.Unmarshal(pollBody, &pollMap); err != nil {
		return "", "", err
	}

	status := extractStatus(pollMap)
	if status == "completed" || status == "success" || status == "finished" || status == "done" {
		imgURL := extractImageURL(pollMap)
		if imgURL == "" {
			return status, "", fmt.Errorf("task completed but no image URL found: %s", string(pollBody))
		}
		return status, imgURL, nil
	}

	if status == "failed" || status == "error" {
		errMsg, _ := pollMap["error"].(string)
		if errMsg == "" {
			errMsg = "unknown generation error"
		}
		return status, "", fmt.Errorf("generation failed on backend: %s", errMsg)
	}

	return status, "", nil
}

func (b *MidjourneyBackend) initiateGeneration(ctx context.Context, prompt, size string) (string, error) {
	payload := map[string]string{
		"prompt": prompt,
	}
	if size != "" {
		payload["size"] = size
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Midjourney payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", b.apiURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create Midjourney request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if b.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.apiKey)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http POST request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("midjourney backend returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &responseMap); err != nil {
		return "", fmt.Errorf("failed to decode json response: %w", err)
	}

	taskID := extractTaskID(responseMap)
	if taskID == "" {
		return "", fmt.Errorf("failed to extract task ID from response: %s", string(bodyBytes))
	}

	statusURL := extractStatusURL(responseMap, b.apiURL, taskID)
	return statusURL, nil
}

// GenerateImage POSTs a generation task, then polls for completion.
func (b *MidjourneyBackend) GenerateImage(ctx context.Context, prompt string, size string) (string, error) {
	statusURL, err := b.initiateGeneration(ctx, prompt, size)
	if err != nil {
		return "", err
	}

	ticker := time.NewTicker(b.pollingInterval)
	defer ticker.Stop()

	timeoutChan := time.After(b.pollingTimeout)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeoutChan:
			return "", fmt.Errorf("polling timed out after %v", b.pollingTimeout)
		case <-ticker.C:
			status, imgURL, err := b.pollOnce(ctx, statusURL)
			if err != nil {
				if status == "failed" || status == "error" {
					return "", err
				}
				continue
			}

			if imgURL != "" {
				return imgURL, nil
			}
		}
	}
}

// ImageGenService coordinates image generation, styles management, and image downloads.
type ImageGenService struct {
	workspaceRoot string
	cfg           *config.Config
	styleStore    *StyleStore
}

// NewImageGenService creates a new ImageGenService.
func NewImageGenService(workspaceRoot string, cfg *config.Config) *ImageGenService {
	return &ImageGenService{
		workspaceRoot: workspaceRoot,
		cfg:           cfg,
		styleStore:    NewStyleStore(workspaceRoot),
	}
}

// StyleStore returns the underlying style profile registry.
func (s *ImageGenService) StyleStore() *StyleStore {
	return s.styleStore
}

func (s *ImageGenService) resolvePrompt(prompt, styleID string) (string, string, error) {
	finalPrompt := prompt
	var srefURL string
	if styleID != "" {
		style, exists, err := s.styleStore.Get(styleID)
		if err != nil {
			return "", "", fmt.Errorf("failed to load style: %w", err)
		}
		if !exists {
			return "", "", fmt.Errorf("style profile %q not found", styleID)
		}
		if style.PromptSeed != "" {
			finalPrompt = fmt.Sprintf("%s, in the style of %s", prompt, style.PromptSeed)
		}
		srefURL = style.SrefURL
	}
	return finalPrompt, srefURL, nil
}

func (s *ImageGenService) runOpenAI(ctx context.Context, finalPrompt, size string) (string, error) {
	apiKey := s.cfg.Plugins.ImageGen.OpenAIAPIKey
	if apiKey == "" {
		apiKey = s.cfg.APIKeys.OpenAI
	}
	if apiKey == "" {
		return "", fmt.Errorf("openai API key is not configured (set plugins.imagegen.openai_api_key or api_keys.openai)")
	}
	client := NewOpenAIBackend(apiKey)
	return client.GenerateImage(ctx, finalPrompt, size)
}

func (s *ImageGenService) runMidjourney(ctx context.Context, finalPrompt, size, srefURL string) (string, error) {
	apiURL := s.cfg.Plugins.ImageGen.MidjourneyAPIURL
	if apiURL == "" {
		return "", fmt.Errorf("midjourney API URL is not configured (set plugins.imagegen.midjourney_api_url)")
	}
	if srefURL != "" {
		finalPrompt = fmt.Sprintf("%s --sref %s", finalPrompt, srefURL)
	}
	client, newErr := NewMidjourneyBackend(
		apiURL,
		s.cfg.Plugins.ImageGen.MidjourneyAPIKey,
		s.cfg.Plugins.ImageGen.MidjourneyPollingInterval,
		s.cfg.Plugins.ImageGen.MidjourneyPollingTimeout,
	)
	if newErr != nil {
		return "", fmt.Errorf("failed to initialize Midjourney backend: %w", newErr)
	}
	return client.GenerateImage(ctx, finalPrompt, size)
}

func (s *ImageGenService) runGoogle(ctx context.Context, finalPrompt, size string) ([]byte, string, error) {
	apiKey := s.cfg.Plugins.ImageGen.GoogleAPIKey
	if apiKey == "" {
		apiKey = s.cfg.APIKeys.Gemini
	}
	if apiKey == "" {
		return nil, "", fmt.Errorf("google/gemini API key is not configured (set plugins.imagegen.google_api_key or api_keys.gemini)")
	}
	client := NewGoogleBackend(apiKey, s.cfg.Plugins.ImageGen.GoogleModel)
	return client.GenerateImage(ctx, finalPrompt, size)
}

func (s *ImageGenService) runVeo(ctx context.Context, finalPrompt, size string) ([]byte, string, error) {
	apiKey := s.cfg.Plugins.ImageGen.GoogleAPIKey
	if apiKey == "" {
		apiKey = s.cfg.APIKeys.Gemini
	}
	if apiKey == "" {
		return nil, "", fmt.Errorf("google/gemini API key is not configured for Veo (set plugins.imagegen.google_api_key or api_keys.gemini)")
	}
	model := s.cfg.Plugins.ImageGen.GoogleModel
	if model == "" || !strings.Contains(model, "veo") {
		model = "veo-2.0-generate-001"
	}
	client, err := NewVeoBackend(
		apiKey,
		model,
		s.cfg.Plugins.ImageGen.MidjourneyPollingInterval,
		s.cfg.Plugins.ImageGen.MidjourneyPollingTimeout,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to initialize Veo backend: %w", err)
	}
	return client.GenerateImage(ctx, finalPrompt, size)
}

// GenerateImage generates and downloads the image to workspaceRoot/generated_images/
func (s *ImageGenService) GenerateImage(ctx context.Context, prompt string, size string, styleID string) (string, error) {
	finalPrompt, srefURL, err := s.resolvePrompt(prompt, styleID)
	if err != nil {
		return "", err
	}

	backend := strings.ToLower(s.cfg.Plugins.ImageGen.Backend)
	if backend == "" {
		backend = "openai"
	}

	var imageURL string
	var imageBytes []byte
	var mimeType string

	switch backend {
	case "openai":
		imageURL, err = s.runOpenAI(ctx, finalPrompt, size)
		if err != nil {
			return "", err
		}
	case "midjourney":
		imageURL, err = s.runMidjourney(ctx, finalPrompt, size, srefURL)
		if err != nil {
			return "", err
		}
	case "google", "imagen":
		imageBytes, mimeType, err = s.runGoogle(ctx, finalPrompt, size)
		if err != nil {
			return "", err
		}
	case "veo", "google-veo":
		imageBytes, mimeType, err = s.runVeo(ctx, finalPrompt, size)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported imagegen backend %q", backend)
	}

	var localPath string
	if imageBytes != nil {
		localPath, err = saveImageBytes(imageBytes, mimeType, s.workspaceRoot, prompt)
	} else {
		localPath, err = downloadImage(ctx, imageURL, s.workspaceRoot, prompt)
	}
	if err != nil {
		return "", fmt.Errorf("failed to save generated image: %w", err)
	}

	return localPath, nil
}

func saveImageBytes(data []byte, mimeType string, workspaceRoot string, prompt string) (string, error) {
	ext := ".png"
	switch strings.ToLower(mimeType) {
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	case "video/mp4":
		ext = ".mp4"
	}

	dir := filepath.Join(workspaceRoot, "generated_images")
	if mkdirErr := os.MkdirAll(dir, 0750); mkdirErr != nil {
		return "", fmt.Errorf("failed to create generated_images folder: %w", mkdirErr)
	}

	slug := slugify(prompt)
	filename := fmt.Sprintf("image_%d_%s%s", time.Now().Unix(), slug, ext)
	filePath := filepath.Join(dir, filename)

	//nolint:gosec // path is safely localized inside workspace root directory
	out, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open local destination file: %w", err)
	}
	defer func() { _ = out.Close() }()

	_, err = out.Write(data)
	if err != nil {
		return "", fmt.Errorf("failed to write image bytes: %w", err)
	}

	return filePath, nil
}

func downloadImage(ctx context.Context, urlStr string, workspaceRoot string, prompt string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request image URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image, status %d", resp.StatusCode)
	}

	ext := ".png"
	contentType := resp.Header.Get("Content-Type")
	switch strings.ToLower(contentType) {
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	dir := filepath.Join(workspaceRoot, "generated_images")
	if mkdirErr := os.MkdirAll(dir, 0750); mkdirErr != nil {
		return "", fmt.Errorf("failed to create generated_images folder: %w", mkdirErr)
	}

	slug := slugify(prompt)
	filename := fmt.Sprintf("image_%d_%s%s", time.Now().Unix(), slug, ext)
	filePath := filepath.Join(dir, filename)

	//nolint:gosec // path is safely localized inside workspace root directory
	out, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open local destination file: %w", err)
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to copy image bytes to file: %w", err)
	}

	return filePath, nil
}

func slugify(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	res := sb.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	res = strings.Trim(res, "-")
	if len(res) > 30 {
		res = res[:30]
		res = strings.TrimRight(res, "-")
	}
	if res == "" {
		res = "image"
	}
	return res
}
