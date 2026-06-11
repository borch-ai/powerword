package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"

	"powerword/internal/config"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"a   b", "a-b"},
		{"A!@#$B%^&*C", "a-b-c"},
		{"-leading-and-trailing-", "leading-and-trailing"},
		{"this-is-an-extremely-long-prompt-that-should-be-cut-off-gracefully", "this-is-an-extremely-long-prom"},
		{"", "image"},
		{"!!!", "image"},
	}

	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.expected {
			t.Errorf("slugify(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestStyleStore(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStyleStore(tmpDir)

	// List empty
	list, err := store.List()
	if err != nil {
		t.Fatalf("List empty returned error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 styles, got %d", len(list))
	}

	// Register style
	style := StyleProfile{
		StyleID:    "test-style",
		PromptSeed: "test prompt seed",
		SrefURL:    "http://example.com/sref.png",
	}

	err = store.Register(style)
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	// Get style
	got, exists, err := store.Get("test-style")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !exists {
		t.Fatalf("expected style to exist")
	}
	if got.StyleID != style.StyleID || got.PromptSeed != style.PromptSeed || got.SrefURL != style.SrefURL {
		t.Errorf("Get returned %+v; want %+v", got, style)
	}

	// Get non-existent style
	_, exists, err = store.Get("non-existent")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if exists {
		t.Errorf("expected non-existent style to not exist")
	}

	// List styles
	list, err = store.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 style, got %d", len(list))
	}
	if list[0].StyleID != "test-style" {
		t.Errorf("expected style 'test-style', got '%s'", list[0].StyleID)
	}
}

func TestOpenAIBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req struct {
			Prompt string `json:"prompt"`
			Size   string `json:"size"`
			Model  string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		if req.Prompt != "a beautiful landscape" {
			t.Errorf("unexpected prompt: %s", req.Prompt)
		}
		if req.Size != "1024x1024" {
			t.Errorf("unexpected size: %s", req.Size)
		}
		if req.Model != "dall-e-3" {
			t.Errorf("unexpected model: %s", req.Model)
		}

		resp := openai.ImageResponse{
			Created: time.Now().Unix(),
			Data: []openai.ImageResponseDataInner{
				{URL: "http://example.com/generated.png"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("OPENAI_BASE_URL", server.URL)
	backend := NewOpenAIBackend("mock-api-key")

	url, err := backend.GenerateImage(context.Background(), "a beautiful landscape", "1024x1024")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if url != "http://example.com/generated.png" {
		t.Errorf("expected url 'http://example.com/generated.png', got '%s'", url)
	}
}

func TestMidjourneyBackend(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if r.Method == "POST" {
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["prompt"] != "a cute cat" {
				t.Errorf("unexpected prompt: %s", req["prompt"])
			}

			resp := map[string]string{
				"id":         "mj-task-123",
				"status":     "pending",
				"status_url": "/api/mj-status/mj-task-123",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method == "GET" {
			var resp map[string]interface{}
			if requestCount < 3 {
				resp = map[string]interface{}{
					"status": "processing",
				}
			} else {
				resp = map[string]interface{}{
					"status":    "completed",
					"image_url": "http://example.com/cat.png",
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	}))
	defer server.Close()

	// Base URL needs to be corrected because status_url mock returned is relative
	backend, err := NewMidjourneyBackend(server.URL, "secret-key", "1ms", "2s")
	if err != nil {
		t.Fatalf("failed to initialize MidjourneyBackend: %v", err)
	}
	// override the status check url to prepend mock server url
	backend.apiURL = server.URL

	url, err := backend.GenerateImage(context.Background(), "a cute cat", "1024x1024")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if url != "http://example.com/cat.png" {
		t.Errorf("expected url 'http://example.com/cat.png', got '%s'", url)
	}
}

func TestImageGenService_EndToEnd(t *testing.T) {
	imageContent := []byte("fake-image-bytes")

	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageContent)
	}))
	defer downloadServer.Close()

	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ImageResponse{
			Data: []openai.ImageResponseDataInner{
				{URL: downloadServer.URL + "/image.png"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer openaiServer.Close()

	t.Setenv("OPENAI_BASE_URL", openaiServer.URL)

	tmpDir := t.TempDir()

	cfg := &config.Config{
		APIKeys: config.APIKeys{
			OpenAI: "global-openai-key",
		},
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend: "openai",
			},
		},
	}

	service := NewImageGenService(tmpDir, cfg)

	// Register style first
	style := StyleProfile{
		StyleID:    "vintage",
		PromptSeed: "vintage 1970s print style",
	}
	if err := service.StyleStore().Register(style); err != nil {
		t.Fatalf("failed to register style: %v", err)
	}

	// Generate image
	filePath, err := service.GenerateImage(context.Background(), "a retro computer", "1024x1024", "vintage")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}

	// Validate saved file path and contents
	if !strings.HasPrefix(filePath, tmpDir) {
		t.Errorf("expected file path to be in %s, got %s", tmpDir, filePath)
	}

	//nolint:gosec // filePath is constructed safely in TestImageGenService_EndToEnd
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read generated image file: %v", err)
	}
	if string(data) != "fake-image-bytes" {
		t.Errorf("expected file content 'fake-image-bytes', got '%s'", string(data))
	}
}

func TestStyleStore_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStyleStore(tmpDir)

	// Write invalid JSON to file to trigger load error
	if err := os.MkdirAll(filepath.Dir(store.filePath), 0750); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	err := os.WriteFile(store.filePath, []byte("invalid-json{"), 0600)
	if err != nil {
		t.Fatalf("failed to write invalid JSON: %v", err)
	}

	_, err = store.Load()
	if err == nil {
		t.Error("expected error loading invalid JSON, got nil")
	}

	// Register should fail when Load fails
	err = store.Register(StyleProfile{StyleID: "test"})
	if err == nil {
		t.Error("expected Register to fail when load fails, got nil")
	}

	// Get should fail when Load fails
	_, _, err = store.Get("test")
	if err == nil {
		t.Error("expected Get to fail when load fails, got nil")
	}

	// List should fail when Load fails
	_, err = store.List()
	if err == nil {
		t.Error("expected List to fail when load fails, got nil")
	}
}

func TestOpenAIBackend_Errors(t *testing.T) {
	// Error response from DALL-E
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid prompt"}}`))
	}))
	defer errServer.Close()

	t.Setenv("OPENAI_BASE_URL", errServer.URL)
	backend := NewOpenAIBackend("mock-key")
	_, err := backend.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error from API failure, got nil")
	}

	// Empty response (no image data returned)
	emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer emptyServer.Close()

	t.Setenv("OPENAI_BASE_URL", emptyServer.URL)
	backendEmpty := NewOpenAIBackend("mock-key")
	_, err = backendEmpty.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error from empty response, got nil")
	}
}

//nolint:funlen // test function length is acceptable
func TestMidjourneyBackend_Errors(t *testing.T) {
	// Test POST invalid URL
	backendBad, err := NewMidjourneyBackend("http:// [invalid-url]:80", "key", "1ms", "10ms")
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	_, err = backendBad.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with invalid URL, got nil")
	}

	// Test non-OK status on POST
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer errServer.Close()

	backendErr, _ := NewMidjourneyBackend(errServer.URL, "key", "1ms", "100ms")
	_, err = backendErr.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with non-OK post status, got nil")
	}

	// Test invalid JSON on POST response
	badJsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer badJsonServer.Close()

	backendBadJson, _ := NewMidjourneyBackend(badJsonServer.URL, "key", "1ms", "100ms")
	_, err = backendBadJson.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with invalid json, got nil")
	}

	// Test missing task ID on POST response
	missingIDServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"pending"}`))
	}))
	defer missingIDServer.Close()

	backendMissingID, _ := NewMidjourneyBackend(missingIDServer.URL, "key", "1ms", "100ms")
	_, err = backendMissingID.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with missing task ID, got nil")
	}

	// Test GET status failed
	statusFailedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task-1"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","error":"midjourney timed out"}`))
	}))
	defer statusFailedServer.Close()

	backendStatusFailed, _ := NewMidjourneyBackend(statusFailedServer.URL, "key", "1ms", "100ms")
	backendStatusFailed.apiURL = statusFailedServer.URL
	_, err = backendStatusFailed.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error when status is failed, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "midjourney timed out") {
		t.Errorf("unexpected error: %v", err)
	}

	// Test GET status completed but no image URL
	noURLServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task-1"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed"}`))
	}))
	defer noURLServer.Close()

	backendNoURL, _ := NewMidjourneyBackend(noURLServer.URL, "key", "1ms", "100ms")
	backendNoURL.apiURL = noURLServer.URL
	_, err = backendNoURL.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error when completed but missing URL, got nil")
	}

	// Test polling timeout
	processingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"task-1"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"processing"}`))
	}))
	defer processingServer.Close()

	backendTimeout, _ := NewMidjourneyBackend(processingServer.URL, "key", "1ms", "5ms")
	backendTimeout.apiURL = processingServer.URL
	_, err = backendTimeout.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected polling timeout error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "polling timed out") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestImageGenService_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	// Missing style ID
	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{Backend: "openai"},
		},
	}
	service := NewImageGenService(tmpDir, cfg)
	_, err := service.GenerateImage(context.Background(), "prompt", "1024x1024", "non-existent")
	if err == nil {
		t.Error("expected error for non-existent style, got nil")
	}

	// OpenAI API Key missing
	cfgNoKey := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{Backend: "openai"},
		},
	}
	serviceNoKey := NewImageGenService(tmpDir, cfgNoKey)
	_, err = serviceNoKey.GenerateImage(context.Background(), "prompt", "1024x1024", "")
	if err == nil {
		t.Error("expected error for missing openai API key, got nil")
	}

	// Midjourney URL missing
	cfgNoMJURL := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{Backend: "midjourney"},
		},
	}
	serviceNoMJURL := NewImageGenService(tmpDir, cfgNoMJURL)
	_, err = serviceNoMJURL.GenerateImage(context.Background(), "prompt", "1024x1024", "")
	if err == nil {
		t.Error("expected error for missing midjourney url, got nil")
	}

	// Unsupported backend
	cfgBadBackend := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{Backend: "unknown"},
		},
	}
	serviceBadBackend := NewImageGenService(tmpDir, cfgBadBackend)
	_, err = serviceBadBackend.GenerateImage(context.Background(), "prompt", "1024x1024", "")
	if err == nil {
		t.Error("expected error for unknown backend, got nil")
	}

	// Midjourney duration fallback check
	mjBackend, err := NewMidjourneyBackend("http://example.com", "", "invalid", "invalid")
	if err != nil {
		t.Fatalf("NewMidjourneyBackend failed: %v", err)
	}
	if mjBackend.pollingInterval != 5*time.Second {
		t.Errorf("expected default interval 5s, got %v", mjBackend.pollingInterval)
	}
	if mjBackend.pollingTimeout != 5*time.Minute {
		t.Errorf("expected default timeout 5m, got %v", mjBackend.pollingTimeout)
	}
}

func TestDownloadImage_ContentTypesAndErrors(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Content types: jpeg, gif, webp
	contentTypes := []string{"image/jpeg", "image/gif", "image/webp"}
	expectedExts := []string{".jpg", ".gif", ".webp"}

	for i, ct := range contentTypes {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ct)
			_, _ = w.Write([]byte("fake-bytes"))
		}))

		path, err := downloadImage(context.Background(), server.URL, tmpDir, fmt.Sprintf("prompt-%d", i))
		server.Close()

		if err != nil {
			t.Fatalf("downloadImage failed for content type %s: %v", ct, err)
		}
		if !strings.HasSuffix(path, expectedExts[i]) {
			t.Errorf("expected path to end in %s, got %s", expectedExts[i], path)
		}
	}

	// 2. Download error (404 status)
	server404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server404.Close()

	_, err := downloadImage(context.Background(), server404.URL, tmpDir, "prompt")
	if err == nil {
		t.Error("expected error downloading with 404 status, got nil")
	}

	// 3. Invalid URL error
	_, err = downloadImage(context.Background(), "http:// [invalid-url]", tmpDir, "prompt")
	if err == nil {
		t.Error("expected error with invalid URL, got nil")
	}
}

func TestStyleStore_SaveError(t *testing.T) {
	tmpDir := t.TempDir()
	store := &StyleStore{
		filePath: tmpDir,
	}
	err := store.Save(map[string]StyleProfile{"test": {StyleID: "test"}})
	if err == nil {
		t.Error("expected Save error when writing to a directory path, got nil")
	}
}

func TestDownloadImage_MkdirAllError(t *testing.T) {
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "generated_images"), []byte("not-a-dir"), 0600)
	if err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	_, err = downloadImage(context.Background(), "http://example.com", tmpDir, "prompt")
	if err == nil {
		t.Error("expected error when generated_images is a file blocking directory creation, got nil")
	}
}

func TestImageGenService_Midjourney(t *testing.T) {
	imageContent := []byte("fake-image-bytes")

	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageContent)
	}))
	defer downloadServer.Close()

	mjServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"mj-task-456"}`))
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"status":"completed","image_url":"%s/mj.png"}`, downloadServer.URL)
			return
		}
	}))
	defer mjServer.Close()

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend:                   "midjourney",
				MidjourneyAPIURL:          mjServer.URL,
				MidjourneyPollingInterval: "1ms",
				MidjourneyPollingTimeout:  "1s",
			},
		},
	}

	service := NewImageGenService(tmpDir, cfg)

	// Register a style with sref URL only
	style := StyleProfile{
		StyleID: "mj-style",
		SrefURL: "http://example.com/sref.png",
	}
	if err := service.StyleStore().Register(style); err != nil {
		t.Fatalf("failed to register style: %v", err)
	}

	filePath, err := service.GenerateImage(context.Background(), "a fancy castle", "1024x1024", "mj-style")
	if err != nil {
		t.Fatalf("GenerateImage with Midjourney service failed: %v", err)
	}

	if !strings.HasPrefix(filePath, tmpDir) {
		t.Errorf("expected path to start with %s, got %s", tmpDir, filePath)
	}
}

func TestStyleStore_MkdirAllError(t *testing.T) {
	tmpDir := t.TempDir()
	blockedFile := filepath.Join(tmpDir, "blocked-dir")
	err := os.WriteFile(blockedFile, []byte("block"), 0600)
	if err != nil {
		t.Fatalf("failed to write blocked file: %v", err)
	}

	store := &StyleStore{
		filePath: filepath.Join(blockedFile, "styles.json"),
	}
	err = store.Save(map[string]StyleProfile{"test": {StyleID: "test"}})
	if err == nil {
		t.Error("expected Save error when mkdir fails, got nil")
	}
}

func TestDownloadImage_OpenFileError(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "generated_images")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Create read-only folder
	if err := os.Chmod(dir, 0400); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}
	defer func() {
		// Restore permissions so cleanup can delete it
		//nolint:gosec // permissions are restored after testing
		_ = os.Chmod(dir, 0750)
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("data"))
	}))
	defer server.Close()

	_, err := downloadImage(context.Background(), server.URL, tmpDir, "prompt")
	if err == nil {
		t.Error("expected error when directory is read-only, got nil")
	}
}

func TestImageGenService_OpenAIKeyFromPlugin(t *testing.T) {
	imageContent := []byte("fake-image-bytes")

	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageContent)
	}))
	defer downloadServer.Close()

	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openai.ImageResponse{
			Data: []openai.ImageResponseDataInner{
				{URL: downloadServer.URL + "/image.png"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer openaiServer.Close()

	t.Setenv("OPENAI_BASE_URL", openaiServer.URL)

	tmpDir := t.TempDir()

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend:      "openai",
				OpenAIAPIKey: "plugin-specific-openai-key",
			},
		},
	}

	service := NewImageGenService(tmpDir, cfg)
	filePath, err := service.GenerateImage(context.Background(), "a retro computer", "1024x1024", "")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if !strings.HasPrefix(filePath, tmpDir) {
		t.Errorf("expected path to start with %s, got %s", tmpDir, filePath)
	}
}

func TestGoogleBackend_Success(t *testing.T) {
	expectedBytes := []byte("google-image-bytes")
	b64Data := base64.StdEncoding.EncodeToString(expectedBytes)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/imagen-3.0-generate-002:predict" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "mock-key" {
			t.Errorf("unexpected API key query parameter: %s", r.URL.Query().Get("key"))
		}

		var req struct {
			Instances []struct {
				Prompt string `json:"prompt"`
			} `json:"instances"`
			Parameters struct {
				SampleCount int    `json:"sampleCount"`
				AspectRatio string `json:"aspectRatio"`
			} `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		if len(req.Instances) == 0 || req.Instances[0].Prompt != "a beautiful painting" {
			t.Errorf("unexpected prompt: %v", req.Instances)
		}
		if req.Parameters.AspectRatio != "9:16" {
			t.Errorf("unexpected aspect ratio: %s", req.Parameters.AspectRatio)
		}

		resp := map[string]interface{}{
			"predictions": []map[string]string{
				{
					"bytesBase64Encoded": b64Data,
					"mimeType":           "image/jpeg",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	backend := NewGoogleBackend("mock-key", "imagen-3.0-generate-002")

	imgBytes, mimeType, err := backend.GenerateImage(context.Background(), "a beautiful painting", "1024x1792")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if string(imgBytes) != string(expectedBytes) {
		t.Errorf("expected bytes %q, got %q", string(expectedBytes), string(imgBytes))
	}
	if mimeType != "image/jpeg" {
		t.Errorf("expected mimeType 'image/jpeg', got '%s'", mimeType)
	}
}

func TestGoogleBackend_Errors(t *testing.T) {
	// 1. Invalid URL / POST fails
	backendBad := NewGoogleBackend("mock-key", "model")
	backendBad.apiURL = "http:// [invalid-url]"
	_, _, err := backendBad.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with invalid URL, got nil")
	}

	// 2. Non-200 status
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("google error"))
	}))
	defer errServer.Close()
	t.Setenv("GOOGLE_BASE_URL", errServer.URL)
	backendErr := NewGoogleBackend("mock-key", "model")
	_, _, err = backendErr.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with 500 status, got nil")
	}

	// 3. Invalid JSON response
	badJsonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer badJsonServer.Close()
	t.Setenv("GOOGLE_BASE_URL", badJsonServer.URL)
	backendBadJson := NewGoogleBackend("mock-key", "model")
	_, _, err = backendBadJson.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with bad json response, got nil")
	}

	// 4. Missing predictions
	missingPredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"predictions":[]}`))
	}))
	defer missingPredServer.Close()
	t.Setenv("GOOGLE_BASE_URL", missingPredServer.URL)
	backendMissingPred := NewGoogleBackend("mock-key", "model")
	_, _, err = backendMissingPred.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with empty predictions, got nil")
	}

	// 5. Invalid predictions format (not a map)
	badFormatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"predictions":["not-a-map"]}`))
	}))
	defer badFormatServer.Close()
	t.Setenv("GOOGLE_BASE_URL", badFormatServer.URL)
	backendBadFormat := NewGoogleBackend("mock-key", "model")
	_, _, err = backendBadFormat.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with invalid prediction format, got nil")
	}

	// 6. Missing bytesBase64Encoded
	missingBytesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"predictions":[{"mimeType":"image/png"}]}`))
	}))
	defer missingBytesServer.Close()
	t.Setenv("GOOGLE_BASE_URL", missingBytesServer.URL)
	backendMissingBytes := NewGoogleBackend("mock-key", "model")
	_, _, err = backendMissingBytes.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with missing base64 bytes, got nil")
	}

	// 7. Invalid base64 bytes
	badBase64Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"predictions":[{"bytesBase64Encoded":"invalid-b64-!"}]}`))
	}))
	defer badBase64Server.Close()
	t.Setenv("GOOGLE_BASE_URL", badBase64Server.URL)
	backendBadBase64 := NewGoogleBackend("mock-key", "model")
	_, _, err = backendBadBase64.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with invalid base64 encoding, got nil")
	}
}

func TestImageGenService_Google(t *testing.T) {
	expectedBytes := []byte("google-service-bytes")
	b64Data := base64.StdEncoding.EncodeToString(expectedBytes)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"predictions": []map[string]string{
				{
					"bytesBase64Encoded": b64Data,
					"mimeType":           "image/png",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)

	tmpDir := t.TempDir()

	cfg := &config.Config{
		APIKeys: config.APIKeys{
			Gemini: "global-gemini-key",
		},
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend: "google",
			},
		},
	}

	service := NewImageGenService(tmpDir, cfg)
	filePath, err := service.GenerateImage(context.Background(), "a green garden", "1024x1024", "")
	if err != nil {
		t.Fatalf("GenerateImage with Google service failed: %v", err)
	}

	if !strings.HasPrefix(filePath, tmpDir) {
		t.Errorf("expected path to start with %s, got %s", tmpDir, filePath)
	}

	//nolint:gosec // filePath is constructed safely in TestImageGenService_Google
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read generated image: %v", err)
	}
	if string(data) != string(expectedBytes) {
		t.Errorf("expected file contents %q, got %q", string(expectedBytes), string(data))
	}
}

//nolint:gocognit // Test server mock callbacks add complexity
func TestVeoBackend_Success(t *testing.T) {
	expectedVideoBytes := []byte("veo-video-bytes")

	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/files/abc" {
			t.Errorf("unexpected download path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "media" {
			t.Errorf("expected alt=media query param, got %s", r.URL.Query().Get("alt"))
		}
		if r.URL.Query().Get("key") != "mock-key" {
			t.Errorf("expected key=mock-key, got %s", r.URL.Query().Get("key"))
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(expectedVideoBytes)
	}))
	defer downloadServer.Close()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method == "POST" {
			if r.URL.Path != "/v1beta/models/veo-2.0-generate-001:predictLongRunning" {
				t.Errorf("unexpected post path: %s", r.URL.Path)
			}
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			instances := req["instances"].([]interface{})
			inst := instances[0].(map[string]interface{})
			if inst["prompt"] != "a flying bird" {
				t.Errorf("expected prompt 'a flying bird', got %v", inst["prompt"])
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"operations/veo-op-123"}`))
			return
		}

		if r.Method == "GET" {
			if r.URL.Path != "/v1beta/operations/veo-op-123" {
				t.Errorf("unexpected status check path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			if requestCount < 3 {
				_, _ = w.Write([]byte(`{"name":"operations/veo-op-123","done":false}`))
			} else {
				respJSON := fmt.Sprintf(`{
					"name": "operations/veo-op-123",
					"done": true,
					"response": {
						"generatedVideos": [
							{
								"video": {
									"uri": "%s/v1beta/files/abc"
								}
							}
						]
					}
				}`, downloadServer.URL)
				_, _ = w.Write([]byte(respJSON))
			}
			return
		}
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	backend, err := NewVeoBackend("mock-key", "veo-2.0-generate-001", "1ms", "1s")
	if err != nil {
		t.Fatalf("failed to create VeoBackend: %v", err)
	}

	videoBytes, mimeType, err := backend.GenerateImage(context.Background(), "a flying bird", "1792x1024")
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if string(videoBytes) != string(expectedVideoBytes) {
		t.Errorf("expected video bytes %q, got %q", string(expectedVideoBytes), string(videoBytes))
	}
	if mimeType != "video/mp4" {
		t.Errorf("expected mimeType 'video/mp4', got %s", mimeType)
	}
}

func TestVeoBackend_Errors(t *testing.T) {
	// 1. Invalid URL / POST fails
	backendBad, err := NewVeoBackend("mock-key", "model", "1ms", "10ms")
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	backendBad.apiURL = "http:// [invalid-url]"
	_, _, err = backendBad.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with invalid URL, got nil")
	}

	// 2. Non-200 status on initiate
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("google error"))
	}))
	defer errServer.Close()
	t.Setenv("GOOGLE_BASE_URL", errServer.URL)
	backendErr, _ := NewVeoBackend("mock-key", "model", "1ms", "10ms")
	_, _, err = backendErr.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with 500 status on initiate, got nil")
	}

	// 3. Missing name in initiate
	missingNameServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer missingNameServer.Close()
	t.Setenv("GOOGLE_BASE_URL", missingNameServer.URL)
	backendMissingName, _ := NewVeoBackend("mock-key", "model", "1ms", "10ms")
	_, _, err = backendMissingName.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error with missing name, got nil")
	}

	// 4. Operation error returned during polling
	opErrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_, _ = w.Write([]byte(`{"name":"op1"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"done":true,"error":{"code":3,"message":"some error"}}`))
	}))
	defer opErrServer.Close()
	t.Setenv("GOOGLE_BASE_URL", opErrServer.URL)
	backendOpErr, _ := NewVeoBackend("mock-key", "model", "1ms", "100ms")
	_, _, err = backendOpErr.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected error when operation fails, got nil")
	}

	// 5. Polling timeout
	pollingTimeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_, _ = w.Write([]byte(`{"name":"op1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"done":false}`))
	}))
	defer pollingTimeoutServer.Close()
	t.Setenv("GOOGLE_BASE_URL", pollingTimeoutServer.URL)
	backendTimeout, _ := NewVeoBackend("mock-key", "model", "1ms", "5ms")
	_, _, err = backendTimeout.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err == nil {
		t.Error("expected polling timeout error, got nil")
	}
}

func TestImageGenService_Veo(t *testing.T) {
	expectedBytes := []byte("veo-service-bytes")

	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(expectedBytes)
	}))
	defer downloadServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_, _ = w.Write([]byte(`{"name":"operations/veo-op-456"}`))
			return
		}
		if r.Method == "GET" {
			resp := fmt.Sprintf(`{"done":true,"response":{"generatedVideos":[{"video":{"uri":"%s/v1beta/files/abc"}}]}}`, downloadServer.URL)
			_, _ = w.Write([]byte(resp))
			return
		}
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)

	tmpDir := t.TempDir()

	cfg := &config.Config{
		APIKeys: config.APIKeys{
			Gemini: "global-gemini-key",
		},
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend: "veo",
			},
		},
	}

	service := NewImageGenService(tmpDir, cfg)
	filePath, err := service.GenerateImage(context.Background(), "a fancy video", "1024x1024", "")
	if err != nil {
		t.Fatalf("GenerateImage with Veo service failed: %v", err)
	}

	if !strings.HasPrefix(filePath, tmpDir) {
		t.Errorf("expected path to start with %s, got %s", tmpDir, filePath)
	}
	if !strings.HasSuffix(filePath, ".mp4") {
		t.Errorf("expected path to end with .mp4, got %s", filePath)
	}

	//nolint:gosec
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read generated video: %v", err)
	}
	if string(data) != string(expectedBytes) {
		t.Errorf("expected file contents %q, got %q", string(expectedBytes), string(data))
	}
}

//nolint:gocognit
func TestVeoBackend_DetailedErrors(t *testing.T) {
	// 1. initiateVeo with size "1024x1792"
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"op1"}`))
	}))
	defer s1.Close()
	t.Setenv("GOOGLE_BASE_URL", s1.URL)
	b1, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _ = b1.initiateVeo(context.Background(), "prompt", "1024x1792")
	_, _ = b1.initiateVeo(context.Background(), "prompt", "1024x1024")

	// 2. NewRequestWithContext invalid URL (for initiate)
	b2, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	b2.apiURL = "http:// [invalid-url]"
	_, _ = b2.initiateVeo(context.Background(), "prompt", "")

	// 3. Initiate JSON unmarshal error
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer s3.Close()
	t.Setenv("GOOGLE_BASE_URL", s3.URL)
	b3, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _ = b3.initiateVeo(context.Background(), "prompt", "")

	// 4. pollOnceVeo - HTTP request creation error
	b4, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _, _ = b4.pollOnceVeo(context.Background(), " [invalid-name]")

	// 5. pollOnceVeo - JSON unmarshal error
	s5 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer s5.Close()
	t.Setenv("GOOGLE_BASE_URL", s5.URL)
	b5, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _, _ = b5.pollOnceVeo(context.Background(), "op")

	// 6. pollOnceVeo - Done but empty response
	s6 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"done":true}`))
	}))
	defer s6.Close()
	t.Setenv("GOOGLE_BASE_URL", s6.URL)
	b6, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _, _ = b6.pollOnceVeo(context.Background(), "op")

	// 7. pollOnceVeo - Done but empty video URI
	s7 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"done":true,"response":{"generatedVideos":[{"video":{"uri":""}}]}}`))
	}))
	defer s7.Close()
	t.Setenv("GOOGLE_BASE_URL", s7.URL)
	b7, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _, _ = b7.pollOnceVeo(context.Background(), "op")

	// 8. pollOnceVeo - Done but downloadVideo fails
	s8 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"done":true,"response":{"generatedVideos":[{"video":{"uri":"http://non-existent-server/video.mp4"}}]}}`))
	}))
	defer s8.Close()
	t.Setenv("GOOGLE_BASE_URL", s8.URL)
	b8, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _, _ = b8.pollOnceVeo(context.Background(), "op")

	// 9. downloadVideo - URI contains "?" and bad request status code
	s9 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer s9.Close()
	b9, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _ = b9.downloadVideo(context.Background(), s9.URL+"/video.mp4?param=1")

	// 10. downloadVideo - bad URL creation
	b10, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _ = b10.downloadVideo(context.Background(), "http:// [invalid-url]")
}

func TestNewVeoBackend_TimeoutParsing(t *testing.T) {
	backend, err := NewVeoBackend("key", "model", "invalid", "invalid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.pollingInterval != 10*time.Second {
		t.Errorf("expected 10s, got %v", backend.pollingInterval)
	}
	if backend.pollingTimeout != 5*time.Minute {
		t.Errorf("expected 5m, got %v", backend.pollingTimeout)
	}
}

func TestImageGenService_VeoErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("GOOGLE_BASE_URL", server.URL)

	tmpDir := t.TempDir()

	cfgNoKey := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend: "veo",
			},
		},
	}
	serviceNoKey := NewImageGenService(tmpDir, cfgNoKey)
	_, err := serviceNoKey.GenerateImage(context.Background(), "prompt", "1024x1024", "")
	if err == nil {
		t.Error("expected error for missing google key in runVeo, got nil")
	}

	cfgCustom := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend:      "veo",
				GoogleAPIKey: "custom-google-key",
				GoogleModel:  "veo-custom-model",
			},
		},
	}
	serviceCustom := NewImageGenService(tmpDir, cfgCustom)
	_, _ = serviceCustom.GenerateImage(context.Background(), "prompt", "1024x1024", "")
}
