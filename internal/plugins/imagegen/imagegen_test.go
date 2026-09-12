package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"

	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/llm"
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
	filePath, err := service.GenerateImage(context.Background(), "a retro computer", "1024x1024", "vintage", "", nil)
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
	_, err := service.GenerateImage(context.Background(), "prompt", "1024x1024", "non-existent", "", nil)
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
	_, err = serviceNoKey.GenerateImage(context.Background(), "prompt", "1024x1024", "", "", nil)
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
	_, err = serviceNoMJURL.GenerateImage(context.Background(), "prompt", "1024x1024", "", "", nil)
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
	_, err = serviceBadBackend.GenerateImage(context.Background(), "prompt", "1024x1024", "", "", nil)
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

		path, err := downloadImage(context.Background(), server.URL, tmpDir, fmt.Sprintf("prompt-%d", i), 120*time.Second)
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

	_, err := downloadImage(context.Background(), server404.URL, tmpDir, "prompt", 120*time.Second)
	if err == nil {
		t.Error("expected error downloading with 404 status, got nil")
	}

	// 3. Invalid URL error
	_, err = downloadImage(context.Background(), "http:// [invalid-url]", tmpDir, "prompt", 120*time.Second)
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

	_, err = downloadImage(context.Background(), "http://example.com", tmpDir, "prompt", 120*time.Second)
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

	filePath, err := service.GenerateImage(context.Background(), "a fancy castle", "1024x1024", "mj-style", "", nil)
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

	_, err := downloadImage(context.Background(), server.URL, tmpDir, "prompt", 120*time.Second)
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
	filePath, err := service.GenerateImage(context.Background(), "a retro computer", "1024x1024", "", "", nil)
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

	imgBytes, mimeType, err := backend.GenerateImage(context.Background(), "a beautiful painting", "1024x1792", "", nil)
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
	_, _, err := backendBad.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendErr.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendBadJson.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendMissingPred.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendBadFormat.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendMissingBytes.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendBadBase64.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	filePath, err := service.GenerateImage(context.Background(), "a green garden", "1024x1024", "", "", nil)
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

	videoBytes, mimeType, err := backend.GenerateImage(context.Background(), "a flying bird", "1792x1024", "", nil)
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
	_, _, err = backendBad.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendErr.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendMissingName.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendOpErr.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	_, _, err = backendTimeout.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
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
	filePath, err := service.GenerateImage(context.Background(), "a fancy video", "1024x1024", "", "", nil)
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
	_, _ = b1.initiateVeo(context.Background(), "prompt", "1024x1792", "", nil)
	_, _ = b1.initiateVeo(context.Background(), "prompt", "1024x1024", "", nil)

	// 2. NewRequestWithContext invalid URL (for initiate)
	b2, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	b2.apiURL = "http:// [invalid-url]"
	_, _ = b2.initiateVeo(context.Background(), "prompt", "", "", nil)

	// 3. Initiate JSON unmarshal error
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer s3.Close()
	t.Setenv("GOOGLE_BASE_URL", s3.URL)
	b3, _ := NewVeoBackend("key", "model", "1ms", "100ms")
	_, _ = b3.initiateVeo(context.Background(), "prompt", "", "", nil)

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
	_, err := serviceNoKey.GenerateImage(context.Background(), "prompt", "1024x1024", "", "", nil)
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
	_, _ = serviceCustom.GenerateImage(context.Background(), "prompt", "1024x1024", "", "", nil)
}

func TestImageGenService_MidjourneyCrefAndCw(t *testing.T) {
	imageContent := []byte("fake-image-bytes")

	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageContent)
	}))
	defer downloadServer.Close()

	var receivedPrompt string
	mjServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var payload struct {
				Prompt string `json:"prompt"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				receivedPrompt = payload.Prompt
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"mj-task-789"}`))
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

	cwVal := 50
	filePath, err := service.GenerateImage(context.Background(), "a fancy castle", "1024x1024", "mj-style", "http://example.com/cref.png", &cwVal)
	if err != nil {
		t.Fatalf("GenerateImage with Midjourney service failed: %v", err)
	}

	if !strings.HasPrefix(filePath, tmpDir) {
		t.Errorf("expected path to start with %s, got %s", tmpDir, filePath)
	}

	expectedPrompt := "a fancy castle --sref http://example.com/sref.png --cref http://example.com/cref.png --cw 50"
	if receivedPrompt != expectedPrompt {
		t.Errorf("expected Midjourney prompt %q, got %q", expectedPrompt, receivedPrompt)
	}
}

func TestImageGenService_MidjourneyCrefAndCw_Validation(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend:          "midjourney",
				MidjourneyAPIURL: "http://example.com/api/midjourney",
			},
		},
	}

	service := NewImageGenService(tmpDir, cfg)

	// Test 1: Invalid cref URL (relative URL)
	_, err := service.GenerateImage(context.Background(), "castle", "1024x1024", "", "/local/path.png", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid cref_url") {
		t.Errorf("expected error containing 'invalid cref_url', got %v", err)
	}

	// Test 2: Invalid cref URL (bad scheme)
	_, err = service.GenerateImage(context.Background(), "castle", "1024x1024", "", "ftp://example.com/cref.png", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid cref_url") {
		t.Errorf("expected error containing 'invalid cref_url', got %v", err)
	}

	// Test 3: Invalid character weight (too low)
	cwLow := -1
	_, err = service.GenerateImage(context.Background(), "castle", "1024x1024", "", "http://example.com/cref.png", &cwLow)
	if err == nil || !strings.Contains(err.Error(), "invalid character_weight") {
		t.Errorf("expected error containing 'invalid character_weight', got %v", err)
	}

	// Test 4: Invalid character weight (too high)
	cwHigh := 101
	_, err = service.GenerateImage(context.Background(), "castle", "1024x1024", "", "http://example.com/cref.png", &cwHigh)
	if err == nil || !strings.Contains(err.Error(), "invalid character_weight") {
		t.Errorf("expected error containing 'invalid character_weight', got %v", err)
	}
}

func TestImageGenService_GetRequestTimeout(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{
			name:     "empty timeout gets default",
			input:    "",
			expected: 120 * time.Second,
		},
		{
			name:     "valid positive timeout gets parsed",
			input:    "45s",
			expected: 45 * time.Second,
		},
		{
			name:     "invalid format gets default",
			input:    "invalid",
			expected: 120 * time.Second,
		},
		{
			name:     "zero duration gets default",
			input:    "0s",
			expected: 120 * time.Second,
		},
		{
			name:     "negative duration gets default",
			input:    "-5s",
			expected: 120 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				Plugins: config.PluginsConfig{
					ImageGen: config.ImageGenConfig{
						RequestTimeout: tc.input,
					},
				},
			}
			service := NewImageGenService(tmpDir, cfg)
			got := service.getRequestTimeout()
			if got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestGoogleBackend_CharacterReference(t *testing.T) {
	expectedBytes := []byte("google-image-bytes-cref")
	b64Data := base64.StdEncoding.EncodeToString(expectedBytes)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Instances []struct {
				Prompt string `json:"prompt"`
			} `json:"instances"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// cref_url is now ignored (no text prepend) — the plain prompt is sent as-is.
		expectedPrompt := "a beautiful painting"
		if len(req.Instances) == 0 || req.Instances[0].Prompt != expectedPrompt {
			t.Errorf("expected prompt %q, got %q", expectedPrompt, req.Instances[0].Prompt)
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

	imgBytes, _, err := backend.GenerateImage(context.Background(), "a beautiful painting", "1024x1792", "http://example.com/cref.png", nil)
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if string(imgBytes) != string(expectedBytes) {
		t.Errorf("expected bytes %q, got %q", string(expectedBytes), string(imgBytes))
	}
}

func TestVeoBackend_CharacterReference_GCS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req struct {
				Instances []struct {
					Prompt string `json:"prompt"`
					Image  struct {
						GCSURI string `json:"gcsUri"`
					} `json:"image"`
				} `json:"instances"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}

			if req.Instances[0].Image.GCSURI != "gs://my-bucket/char.png" {
				t.Errorf("expected GCS URI gs://my-bucket/char.png, got %s", req.Instances[0].Image.GCSURI)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"operations/veo-op-cref-gcs"}`))
			return
		}
		if r.Method == "GET" {
			respJSON := `{"name": "operations/veo-op-cref-gcs", "done": true, "response": {"generatedVideos": [{"video": {"uri": "http://example.com/file.mp4"}}]}}`
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(respJSON))
			return
		}
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	backend, err := NewVeoBackend("mock-key", "veo-2.0-generate-001", "1ms", "1s")
	if err != nil {
		t.Fatalf("failed to create VeoBackend: %v", err)
	}

	// Mock downloadVideo method to return mock bytes, while routing other calls to standard transport
	backend.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "files") || strings.Contains(req.URL.Query().Get("alt"), "media") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte("mock-video-bytes"))),
					Header:     make(http.Header),
				}, nil
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	videoBytes, _, err := backend.GenerateImage(context.Background(), "a flying bird", "1792x1024", "gs://my-bucket/char.png", nil)
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if string(videoBytes) != "mock-video-bytes" {
		t.Errorf("expected video bytes 'mock-video-bytes', got %s", string(videoBytes))
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

//nolint:gocognit // test structure adds complexity
func TestVeoBackend_CharacterReference_HTTP(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("mock-image-data"))
	}))
	defer imageServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req struct {
				Instances []struct {
					Prompt string `json:"prompt"`
					Image  struct {
						BytesBase64Encoded string `json:"bytesBase64Encoded"`
						MIMEType           string `json:"mimeType"`
					} `json:"image"`
				} `json:"instances"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}

			expectedB64 := base64.StdEncoding.EncodeToString([]byte("mock-image-data"))
			if req.Instances[0].Image.BytesBase64Encoded != expectedB64 {
				t.Errorf("expected base64 image bytes, got %s", req.Instances[0].Image.BytesBase64Encoded)
			}
			if req.Instances[0].Image.MIMEType != "image/png" {
				t.Errorf("expected mimeType image/png, got %s", req.Instances[0].Image.MIMEType)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"operations/veo-op-cref-http"}`))
			return
		}
		if r.Method == "GET" {
			respJSON := `{"name": "operations/veo-op-cref-http", "done": true, "response": {"generatedVideos": [{"video": {"uri": "http://example.com/file.mp4"}}]}}`
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(respJSON))
			return
		}
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	backend, err := NewVeoBackend("mock-key", "veo-2.0-generate-001", "1ms", "1s")
	if err != nil {
		t.Fatalf("failed to create VeoBackend: %v", err)
	}

	backend.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/operations") || strings.Contains(req.URL.Path, "predictLongRunning") {
				return http.DefaultTransport.RoundTrip(req)
			}
			if strings.Contains(req.URL.String(), "file.mp4") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte("mock-video-bytes"))),
					Header:     make(http.Header),
				}, nil
			}
			// Otherwise request image
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte("mock-image-data"))),
				Header: http.Header{
					"Content-Type": []string{"image/png"},
				},
			}, nil
		}),
	}

	videoBytes, _, err := backend.GenerateImage(context.Background(), "a flying bird", "1792x1024", imageServer.URL+"/char.png", nil)
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}
	if string(videoBytes) != "mock-video-bytes" {
		t.Errorf("expected video bytes 'mock-video-bytes', got %s", string(videoBytes))
	}
}

func TestVeoBackend_CharacterReference_InvalidScheme(t *testing.T) {
	backend, err := NewVeoBackend("mock-key", "veo-2.0-generate-001", "1ms", "1s")
	if err != nil {
		t.Fatalf("failed to create VeoBackend: %v", err)
	}

	_, _, err = backend.GenerateImage(context.Background(), "a flying bird", "1792x1024", "ftp://example.com/char.png", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid cref_url") {
		t.Errorf("expected error with invalid URL scheme, got %v", err)
	}
}

func TestVeoBackend_CharacterReference_TooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		zeros := make([]byte, 1024*1024)
		for i := 0; i < 10; i++ {
			_, _ = w.Write(zeros)
		}
		_, _ = w.Write([]byte("extra-bytes"))
	}))
	defer server.Close()

	backend, err := NewVeoBackend("mock-key", "veo-2.0-generate-001", "1ms", "1s")
	if err != nil {
		t.Fatalf("failed to create VeoBackend: %v", err)
	}

	_, _, err = backend.GenerateImage(context.Background(), "a flying bird", "1792x1024", server.URL+"/large.png", nil)
	if err == nil || !strings.Contains(err.Error(), "image exceeds maximum allowed size") {
		t.Errorf("expected error about maximum allowed size, got %v", err)
	}
}

func TestGoogleBackend_CharacterWeightWarning(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	expectedBytes := []byte("google-image-bytes-cref")
	b64Data := base64.StdEncoding.EncodeToString(expectedBytes)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	cw := 50
	_, _, err := backend.GenerateImage(context.Background(), "a beautiful painting", "1024x1792", "", &cw)
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}

	_ = w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	expectedWarning := "Warning: Google Imagen backend does not support character weight adjustment; parameter will be ignored."
	if !strings.Contains(output, expectedWarning) {
		t.Errorf("expected stderr warning %q, got %q", expectedWarning, output)
	}
}

func TestVeoBackend_CharacterWeightWarning(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"operations/veo-op-cw-test"}`))
			return
		}
		if r.Method == "GET" {
			respJSON := `{"name": "operations/veo-op-cw-test", "done": true, "response": {"generatedVideos": [{"video": {"uri": "http://example.com/file.mp4"}}]}}`
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(respJSON))
			return
		}
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	backend, err := NewVeoBackend("mock-key", "veo-2.0-generate-001", "1ms", "1s")
	if err != nil {
		t.Fatalf("failed to create VeoBackend: %v", err)
	}

	backend.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/operations") || strings.Contains(req.URL.Path, "predictLongRunning") {
				return http.DefaultTransport.RoundTrip(req)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte("mock-video-bytes"))),
				Header:     make(http.Header),
			}, nil
		}),
	}

	cw := 50
	_, _, err = backend.GenerateImage(context.Background(), "a flying bird", "1792x1024", "", &cw)
	if err != nil {
		t.Fatalf("GenerateImage failed: %v", err)
	}

	_ = w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	expectedWarning := "Warning: Google Veo backend does not support character weight adjustment; parameter will be ignored."
	if !strings.Contains(output, expectedWarning) {
		t.Errorf("expected stderr warning %q, got %q", expectedWarning, output)
	}
}

func TestGetCapabilities(t *testing.T) {
	tests := []struct {
		name           string
		backend        string
		googleModel    string
		wantCref       bool
		wantSref       bool
		wantOutputType OutputType
		expectedName   string
	}{
		{"default_empty", "", "", false, false, OutputTypeImage, "openai"},
		{"openai", "openai", "", false, false, OutputTypeImage, "openai"},
		{"midjourney", "midjourney", "", true, true, OutputTypeImage, "midjourney"},
		// Google Imagen backend never supports cref — regardless of model name.
		// To use Veo (which supports cref), set backend = "veo".
		{"google", "google", "", false, false, OutputTypeImage, "google"},
		{"google_with_imagen_model", "google", "imagen-4.0-generate-001", false, false, OutputTypeImage, "google"},
		{"imagen", "imagen", "imagen-3.0-generate-002", false, false, OutputTypeImage, "imagen"},
		// The veo and google-veo backends always support cref.
		{"veo", "veo", "", true, false, OutputTypeVideo, "veo"},
		{"google-veo", "google-veo", "", true, false, OutputTypeVideo, "google-veo"},
		{"unsupported", "unsupported", "", false, false, "", "unsupported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Plugins: config.PluginsConfig{
					ImageGen: config.ImageGenConfig{
						Backend:     tt.backend,
						GoogleModel: tt.googleModel,
					},
				},
			}
			service := NewImageGenService(t.TempDir(), cfg)
			caps := service.GetCapabilities()

			if caps.Backend != tt.expectedName {
				t.Errorf("expected backend %q, got %q", tt.expectedName, caps.Backend)
			}
			if caps.SupportsCref != tt.wantCref {
				t.Errorf("expected supports_cref %v, got %v", tt.wantCref, caps.SupportsCref)
			}
			if caps.SupportsSref != tt.wantSref {
				t.Errorf("expected supports_sref %v, got %v", tt.wantSref, caps.SupportsSref)
			}
			if caps.OutputType != tt.wantOutputType {
				t.Errorf("expected output_type %q, got %q", tt.wantOutputType, caps.OutputType)
			}
		})
	}
}

func TestGetCapabilities_ForceOverrides(t *testing.T) {
	t.Run("force_cref overrides imagen no-cref", func(t *testing.T) {
		cfg := &config.Config{
			Plugins: config.PluginsConfig{
				ImageGen: config.ImageGenConfig{
					Backend:     "google",
					GoogleModel: "imagen-4.0-generate-001",
					ForceCref:   true,
				},
			},
		}
		service := NewImageGenService(t.TempDir(), cfg)
		caps := service.GetCapabilities()
		if !caps.SupportsCref {
			t.Error("expected supports_cref=true with ForceCref override, got false")
		}
		if caps.SupportsSref {
			t.Error("expected supports_sref=false (not forced), got true")
		}
	})

	t.Run("force_sref overrides openai no-sref", func(t *testing.T) {
		cfg := &config.Config{
			Plugins: config.PluginsConfig{
				ImageGen: config.ImageGenConfig{
					Backend:   "openai",
					ForceSref: true,
				},
			},
		}
		service := NewImageGenService(t.TempDir(), cfg)
		caps := service.GetCapabilities()
		if caps.SupportsCref {
			t.Error("expected supports_cref=false (not forced), got true")
		}
		if !caps.SupportsSref {
			t.Error("expected supports_sref=true with ForceSref override, got false")
		}
	})

	t.Run("both overrides", func(t *testing.T) {
		cfg := &config.Config{
			Plugins: config.PluginsConfig{
				ImageGen: config.ImageGenConfig{
					Backend:   "openai",
					ForceCref: true,
					ForceSref: true,
				},
			},
		}
		service := NewImageGenService(t.TempDir(), cfg)
		caps := service.GetCapabilities()
		if !caps.SupportsCref {
			t.Error("expected supports_cref=true with ForceCref override, got false")
		}
		if !caps.SupportsSref {
			t.Error("expected supports_sref=true with ForceSref override, got false")
		}
	})
}

func TestBackendCapabilities(t *testing.T) {
	tests := []struct {
		name           string
		caps           Capabilities
		wantBackend    string
		wantCref       bool
		wantSref       bool
		wantOutputType OutputType
	}{
		{
			name:           "OpenAIBackend",
			caps:           (&OpenAIBackend{}).Capabilities(),
			wantBackend:    "openai",
			wantCref:       false,
			wantSref:       false,
			wantOutputType: OutputTypeImage,
		},
		{
			name:           "GoogleBackend",
			caps:           (&GoogleBackend{}).Capabilities(),
			wantBackend:    "google",
			wantCref:       false,
			wantSref:       false,
			wantOutputType: OutputTypeImage,
		},
		{
			name:           "VeoBackend",
			caps:           (&VeoBackend{}).Capabilities(),
			wantBackend:    "veo",
			wantCref:       true,
			wantSref:       false,
			wantOutputType: OutputTypeVideo,
		},
		{
			name:           "MidjourneyBackend",
			caps:           (&MidjourneyBackend{}).Capabilities(),
			wantBackend:    "midjourney",
			wantCref:       true,
			wantSref:       true,
			wantOutputType: OutputTypeImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.caps.Backend != tt.wantBackend {
				t.Errorf("expected backend %q, got %q", tt.wantBackend, tt.caps.Backend)
			}
			if tt.caps.SupportsCref != tt.wantCref {
				t.Errorf("expected supports_cref=%v, got %v", tt.wantCref, tt.caps.SupportsCref)
			}
			if tt.caps.SupportsSref != tt.wantSref {
				t.Errorf("expected supports_sref=%v, got %v", tt.wantSref, tt.caps.SupportsSref)
			}
			if tt.caps.OutputType != tt.wantOutputType {
				t.Errorf("expected output_type=%q, got %q", tt.wantOutputType, tt.caps.OutputType)
			}
		})
	}
}

func TestGenerateImage_CapabilitiesValidation(t *testing.T) {
	tmpDir := t.TempDir()

	// Test 1: OpenAI backend does not support cref_url
	cfgOpenAI := &config.Config{
		APIKeys: config.APIKeys{
			OpenAI: "mock-key",
		},
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend: "openai",
			},
		},
	}
	serviceOpenAI := NewImageGenService(tmpDir, cfgOpenAI)
	_, err := serviceOpenAI.GenerateImage(context.Background(), "prompt", "1024x1024", "", "http://example.com/cref.png", nil)
	if err == nil || !strings.Contains(err.Error(), "character reference (cref_url) is not supported by the active imagegen backend") {
		t.Errorf("expected error containing 'character reference (cref_url) is not supported by the active imagegen backend', got: %v", err)
	}

	// Test 2: Style requiring sref_url applied to OpenAI backend (which does not support style reference)
	styleStore := serviceOpenAI.StyleStore()
	err = styleStore.Register(StyleProfile{
		StyleID:    "sref-style",
		PromptSeed: "prompt seed",
		SrefURL:    "http://example.com/sref.png",
	})
	if err != nil {
		t.Fatalf("failed to register style: %v", err)
	}

	_, err = serviceOpenAI.GenerateImage(context.Background(), "prompt", "1024x1024", "sref-style", "", nil)
	if err == nil || !strings.Contains(err.Error(), "style reference (sref_url) is not supported by the active imagegen backend") {
		t.Errorf("expected error containing 'style reference (sref_url) is not supported by the active imagegen backend', got: %v", err)
	}
}

func TestGenerateImage_ImagenCrefValidation(t *testing.T) {
	tmpDir := t.TempDir()

	// Test 1: google backend with an Imagen model should reject cref_url without ForceCref.
	cfgImagen := &config.Config{
		APIKeys: config.APIKeys{
			Gemini: "mock-key",
		},
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend:     "google",
				GoogleModel: "imagen-4.0-generate-001",
			},
		},
	}
	serviceImagen := NewImageGenService(tmpDir, cfgImagen)
	_, err := serviceImagen.GenerateImage(context.Background(), "prompt", "1024x1024", "", "http://example.com/cref.png", nil)
	if err == nil || !strings.Contains(err.Error(), "character reference (cref_url) is not supported by the active imagegen backend") {
		t.Errorf("expected cref validation error for Imagen model, got: %v", err)
	}

	// Test 2: imagen backend with default (empty) GoogleModel should also reject cref_url.
	cfgImagenDefault := &config.Config{
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend: "imagen",
			},
		},
	}
	serviceImagenDefault := NewImageGenService(tmpDir, cfgImagenDefault)
	_, err = serviceImagenDefault.GenerateImage(context.Background(), "prompt", "1024x1024", "", "http://example.com/cref.png", nil)
	if err == nil || !strings.Contains(err.Error(), "character reference (cref_url) is not supported by the active imagegen backend") {
		t.Errorf("expected cref validation error for imagen backend with default model, got: %v", err)
	}

	// Test 3: ForceCref=true bypasses the capability check for the Imagen backend.
	// Use a mock httptest server so the test is hermetic and makes no real network requests.
	expectedBytesForce := []byte("google-image-bytes-force-cref")
	b64Force := base64.StdEncoding.EncodeToString(expectedBytesForce)
	forceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"predictions": []map[string]string{
				{"bytesBase64Encoded": b64Force, "mimeType": "image/jpeg"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer forceServer.Close()

	t.Setenv("GOOGLE_BASE_URL", forceServer.URL)

	cfgForce := &config.Config{
		APIKeys: config.APIKeys{
			Gemini: "mock-key",
		},
		Plugins: config.PluginsConfig{
			ImageGen: config.ImageGenConfig{
				Backend:     "google",
				GoogleModel: "imagen-4.0-generate-001",
				ForceCref:   true,
			},
		},
	}
	serviceForce := NewImageGenService(tmpDir, cfgForce)
	caps := serviceForce.GetCapabilities()
	if !caps.SupportsCref {
		t.Error("expected supports_cref=true when ForceCref is set, got false")
	}
	// The generate call should pass capability validation and succeed via the mock server.
	// Since the mock server is hermetic, require err == nil — any error here is a regression.
	_, err = serviceForce.GenerateImage(context.Background(), "prompt", "1024x1024", "", "http://example.com/cref.png", nil)
	if err != nil {
		t.Errorf("expected ForceCref generate to succeed with mock server, got: %v", err)
	}
}

func TestOpenAIBackend_RetrySuccess(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"Rate limit exceeded","code":"rate_limit_exceeded"}}`))
			return
		}
		resp := openai.ImageResponse{
			Data: []openai.ImageResponseDataInner{
				{URL: "http://example.com/success_retry.png"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("OPENAI_BASE_URL", server.URL)
	backend := NewOpenAIBackendWithTimeout("dummy", 10*time.Second)
	backend.SetRetryConfig(llm.RetryConfig{
		MaxRetries: 3,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 5 * time.Millisecond,
		Retryable:  llm.IsRetryableError,
	})

	url, err := backend.GenerateImage(context.Background(), "prompt", "1024x1024")
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if url != "http://example.com/success_retry.png" {
		t.Errorf("unexpected image URL: %s", url)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestGoogleBackend_RetrySuccess(t *testing.T) {
	var attempts int
	b64 := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("503 Service Unavailable"))
			return
		}
		resp := map[string]interface{}{
			"predictions": []map[string]string{
				{"bytesBase64Encoded": b64, "mimeType": "image/png"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	backend := NewGoogleBackend("dummy-key", "imagen-3.0-generate-002")
	backend.SetRetryConfig(llm.RetryConfig{
		MaxRetries: 3,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 5 * time.Millisecond,
		Retryable:  llm.IsRetryableError,
	})

	bytes, mime, err := backend.GenerateImage(context.Background(), "prompt", "1024x1024", "", nil)
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if string(bytes) != "fake-png-data" || mime != "image/png" {
		t.Errorf("unexpected output: bytes=%s, mime=%s", string(bytes), mime)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestVeoBackend_RetrySuccess(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("503 Service Unavailable"))
			return
		}
		// Second attempt succeeds
		resp := map[string]string{
			"name": "operations/test_op_123",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	backend, err := NewVeoBackend("dummy-key", "veo-2.0-generate-001", "100ms", "5s")
	if err != nil {
		t.Fatal(err)
	}
	backend.SetRetryConfig(llm.RetryConfig{
		MaxRetries: 3,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 5 * time.Millisecond,
		Retryable:  llm.IsRetryableError,
	})

	opName, err := backend.initiateVeo(context.Background(), "prompt", "16:9", "", nil)
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if opName != "operations/test_op_123" {
		t.Errorf("unexpected operation name: %s", opName)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestMidjourneyBackend_RetrySuccess(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("502 Bad Gateway"))
			return
		}
		resp := map[string]string{
			"task_id": "mj_task_456",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend, err := NewMidjourneyBackend(server.URL, "dummy-key", "100ms", "5s")
	if err != nil {
		t.Fatal(err)
	}
	backend.SetRetryConfig(llm.RetryConfig{
		MaxRetries: 3,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 5 * time.Millisecond,
		Retryable:  llm.IsRetryableError,
	})

	statusURL, err := backend.initiateGeneration(context.Background(), "prompt", "1:1")
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if !strings.Contains(statusURL, "mj_task_456") {
		t.Errorf("unexpected status URL: %s", statusURL)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestDownloadImage_RetrySuccess(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("503 Service Unavailable"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake-downloaded-bytes"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	path, err := downloadImage(context.Background(), server.URL, tmpDir, "retry-prompt", 10*time.Second)
	if err != nil {
		t.Fatalf("expected downloadImage to succeed after retry, got: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty file path")
	}
	//nolint:gosec // G304: path is created inside test temp directory
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != "fake-downloaded-bytes" {
		t.Errorf("unexpected file content: %s", string(data))
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestVeoBackend_PollingRetrySuccess(t *testing.T) {
	var pollAttempts int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "video.mp4") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("fake-video-bytes"))
			return
		}
		pollAttempts++
		if pollAttempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":503,"message":"service temporarily unavailable"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := fmt.Sprintf(`{"done":true,"response":{"generatedVideos":[{"video":{"uri":"%s/video.mp4"}}]}}`, server.URL)
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	b, err := NewVeoBackend("test-key", "veo-2.0", "10ms", "5s")
	if err != nil {
		t.Fatalf("failed to initialize VeoBackend: %v", err)
	}
	b.SetRetryConfig(llm.RetryConfig{
		MaxRetries: 3,
		MinBackoff: 10 * time.Millisecond,
		MaxBackoff: 50 * time.Millisecond,
		Retryable:  llm.IsRetryableError,
	})

	done, videoBytes, pollErr := b.pollOnceVeo(context.Background(), "operations/test-op")
	if pollErr != nil {
		t.Fatalf("expected pollOnceVeo to succeed after retry, got: %v", pollErr)
	}
	if !done {
		t.Error("expected done to be true")
	}
	if string(videoBytes) != "fake-video-bytes" {
		t.Errorf("unexpected videoBytes: %s", string(videoBytes))
	}
	if pollAttempts != 2 {
		t.Errorf("expected 2 poll attempts, got %d", pollAttempts)
	}
}

func TestMidjourneyBackend_PollingRetrySuccess(t *testing.T) {
	var pollAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pollAttempts++
		if pollAttempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("502 Bad Gateway"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","imageUrl":"https://example.com/mj.png"}`))
	}))
	defer server.Close()

	b, err := NewMidjourneyBackend(server.URL, "test-key", "10ms", "5s")
	if err != nil {
		t.Fatalf("failed to initialize MidjourneyBackend: %v", err)
	}
	b.SetRetryConfig(llm.RetryConfig{
		MaxRetries: 3,
		MinBackoff: 10 * time.Millisecond,
		MaxBackoff: 50 * time.Millisecond,
		Retryable:  llm.IsRetryableError,
	})

	status, imgURL, pollErr := b.pollOnce(context.Background(), server.URL+"/task-123")
	if pollErr != nil {
		t.Fatalf("expected pollOnce to succeed after retry, got: %v", pollErr)
	}
	if status != "completed" {
		t.Errorf("expected status 'completed', got: %s", status)
	}
	if imgURL != "https://example.com/mj.png" {
		t.Errorf("expected imgURL 'https://example.com/mj.png', got: %s", imgURL)
	}
	if pollAttempts != 2 {
		t.Errorf("expected 2 poll attempts, got %d", pollAttempts)
	}
}

func TestImageGenBackends_DefaultNoRetries(t *testing.T) {
	oa := NewOpenAIBackendWithTimeout("key", 10*time.Second)
	if !oa.retryConfig.Disabled {
		t.Errorf("expected OpenAIBackend to default to NoRetries (Disabled: true)")
	}

	gb := NewGoogleBackend("key", "model")
	if !gb.retryConfig.Disabled {
		t.Errorf("expected GoogleBackend to default to NoRetries (Disabled: true)")
	}

	vb, err := NewVeoBackend("key", "model", "", "")
	if err != nil {
		t.Fatalf("unexpected error creating VeoBackend: %v", err)
	}
	if !vb.retryConfig.Disabled {
		t.Errorf("expected VeoBackend to default to NoRetries (Disabled: true)")
	}

	mb, err := NewMidjourneyBackend("http://localhost", "key", "", "")
	if err != nil {
		t.Fatalf("unexpected error creating MidjourneyBackend: %v", err)
	}
	if !mb.retryConfig.Disabled {
		t.Errorf("expected MidjourneyBackend to default to NoRetries (Disabled: true)")
	}
}

func TestImageGenService_RetryConfig(t *testing.T) {
	// Default with no config should be NoRetries
	svc := NewImageGenService(t.TempDir(), &config.Config{})
	rc := svc.getRetryConfig()
	if !rc.Disabled {
		t.Errorf("expected ImageGenService to default to NoRetries, got Disabled=%v", rc.Disabled)
	}

	// Config with MaxRetries and RetryBackoff
	cfg := &config.Config{}
	cfg.Plugins.ImageGen.MaxRetries = 2
	cfg.Plugins.ImageGen.RetryBackoff = "250ms"
	svcWithCfg := NewImageGenService(t.TempDir(), cfg)
	rcCfg := svcWithCfg.getRetryConfig()
	if rcCfg.Disabled || rcCfg.MaxRetries != 2 || rcCfg.MinBackoff != 250*time.Millisecond {
		t.Errorf("unexpected retryConfig from config: %+v", rcCfg)
	}

	// Programmatic override via SetRetryConfig
	custom := llm.RetryConfig{MaxRetries: 5, MinBackoff: 1 * time.Second}
	svcWithCfg.SetRetryConfig(custom)
	rcCustom := svcWithCfg.getRetryConfig()
	if rcCustom.MaxRetries != 5 || rcCustom.MinBackoff != 1*time.Second {
		t.Errorf("expected custom retryConfig to override config, got: %+v", rcCustom)
	}
}

func TestRetryConfigFromConfig(t *testing.T) {
	// Zero MaxRetries returns NoRetries
	rcZero := RetryConfigFromConfig(config.ImageGenConfig{})
	if !rcZero.Disabled {
		t.Errorf("expected NoRetries for empty config, got Disabled=%v", rcZero.Disabled)
	}

	// Valid MaxRetries and backoff
	cfg := config.ImageGenConfig{
		MaxRetries:   3,
		RetryBackoff: "500ms",
	}
	rc := RetryConfigFromConfig(cfg)
	if rc.Disabled || rc.MaxRetries != 3 || rc.MinBackoff != 500*time.Millisecond {
		t.Errorf("unexpected retry config: %+v", rc)
	}

	// Invalid backoff falls back to default MinBackoff
	cfgInvalid := config.ImageGenConfig{
		MaxRetries:   2,
		RetryBackoff: "invalid-duration",
	}
	rcInvalid := RetryConfigFromConfig(cfgInvalid)
	if rcInvalid.Disabled || rcInvalid.MaxRetries != 2 || rcInvalid.MinBackoff != 100*time.Millisecond {
		t.Errorf("unexpected fallback retry config: %+v", rcInvalid)
	}
}

func TestVeoBackend_DownloadVideo_ExceedsSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		buf := make([]byte, 1024*1024)
		for i := 0; i < 101; i++ {
			_, _ = w.Write(buf)
		}
	}))
	defer server.Close()

	b, err := NewVeoBackend("key", "model", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, dlErr := b.downloadVideo(context.Background(), server.URL)
	if dlErr == nil || !strings.Contains(dlErr.Error(), "exceeds maximum allowed size") {
		t.Errorf("expected error mentioning maximum allowed size, got: %v", dlErr)
	}
}

func TestVeoBackend_PollOnceVeo_ExceedsSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		buf := make([]byte, 1024*1024)
		for i := 0; i < 6; i++ {
			_, _ = w.Write(buf)
		}
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	b, err := NewVeoBackend("key", "model", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, pollErr := b.pollOnceVeo(context.Background(), "test-op")
	if pollErr == nil || !strings.Contains(pollErr.Error(), "exceeds maximum allowed size") {
		t.Errorf("expected error mentioning maximum allowed size, got: %v", pollErr)
	}
}

func TestDownloadImage_CustomRetryConfig(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("503 Service Unavailable"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	// When llm.NoRetries() is passed, attempts should be exactly 1
	_, err := downloadImage(context.Background(), server.URL, tmpDir, "no-retry-prompt", 5*time.Second, llm.NoRetries())
	if err == nil {
		t.Fatal("expected downloadImage to fail on 503")
	}
	if attempts != 1 {
		t.Errorf("expected exactly 1 attempt with NoRetries(), got %d", attempts)
	}
}

func TestVeoBackend_PollVeo_PollingTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Never done
		_, _ = w.Write([]byte(`{"done":false}`))
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	b, err := NewVeoBackend("key", "model", "5ms", "25ms")
	if err != nil {
		t.Fatal(err)
	}

	_, pollErr := b.pollVeo(context.Background(), "test-op")
	if pollErr == nil || !strings.Contains(pollErr.Error(), "polling timed out after") {
		t.Errorf("expected error containing 'polling timed out after', got: %v", pollErr)
	}
}

func TestMidjourneyBackend_GenerateImage_PollingTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			_, _ = w.Write([]byte(`{"task_id":"mj_timeout_task"}`))
			return
		}
		// Polling GET: always pending
		_, _ = w.Write([]byte(`{"status":"pending"}`))
	}))
	defer server.Close()

	b, err := NewMidjourneyBackend(server.URL, "key", "5ms", "25ms")
	if err != nil {
		t.Fatal(err)
	}

	_, genErr := b.GenerateImage(context.Background(), "prompt", "1:1")
	if genErr == nil || !strings.Contains(genErr.Error(), "polling timed out after") {
		t.Errorf("expected error containing 'polling timed out after', got: %v", genErr)
	}
}

func TestMidjourneyBackend_GenerateImage_MalformedPayloadResilient(t *testing.T) {
	var pollAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task_id":"mj_resilient_task"}`))
			return
		}
		pollAttempts++
		w.Header().Set("Content-Type", "application/json")
		if pollAttempts == 1 {
			// Malformed JSON payload on first poll attempt
			_, _ = w.Write([]byte(`{invalid-json`))
			return
		}
		// Valid completed response on second attempt
		_, _ = w.Write([]byte(`{"status":"completed","imageUrl":"https://example.com/resilient.png"}`))
	}))
	defer server.Close()

	b, err := NewMidjourneyBackend(server.URL, "key", "5ms", "500ms")
	if err != nil {
		t.Fatal(err)
	}

	imgURL, err := b.GenerateImage(context.Background(), "prompt", "1:1")
	if err != nil {
		t.Fatalf("expected GenerateImage to recover from malformed payload, got err: %v", err)
	}
	if imgURL != "https://example.com/resilient.png" {
		t.Errorf("unexpected image URL: %s", imgURL)
	}
	if pollAttempts < 2 {
		t.Errorf("expected at least 2 poll attempts, got %d", pollAttempts)
	}
}

func TestMidjourneyBackend_GenerateImage_ExhaustedRetryError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task_id":"mj_exhaust_task"}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("500 Internal Server Error"))
	}))
	defer server.Close()

	b, err := NewMidjourneyBackend(server.URL, "key", "5ms", "500ms")
	if err != nil {
		t.Fatal(err)
	}
	b.SetRetryConfig(llm.RetryConfig{
		MaxRetries: 1,
		MinBackoff: 1 * time.Millisecond,
		MaxBackoff: 5 * time.Millisecond,
		Retryable:  llm.IsRetryableError,
	})

	_, err = b.GenerateImage(context.Background(), "prompt", "1:1")
	if err == nil {
		t.Fatal("expected error on exhausted polling retries, got nil")
	}
	if strings.Contains(err.Error(), "polling timed out after") {
		t.Errorf("expected request error, but got polling timed out: %v", err)
	}
}
