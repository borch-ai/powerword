package viral

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"powerword/internal/config"
)

func createMockFFmpeg(t *testing.T) string {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "mock_ffmpeg")
	scriptContent := `#!/bin/sh
for arg; do true; done
touch "$arg"
exit 0
`
	//nolint:gosec // mock script needs to be executable
	err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("failed to write mock ffmpeg script: %v", err)
	}
	return scriptPath
}

func TestCheckFFmpeg(t *testing.T) {
	// 1. Success case with standard system binary
	goPath, err := exec.LookPath("go")
	if err == nil {
		if checkErr := checkFFmpeg(goPath); checkErr != nil {
			t.Errorf("expected goPath check to succeed, got: %v", checkErr)
		}
	}

	// 2. Failure case
	err = checkFFmpeg("non-existent-binary-12345")
	if err == nil {
		t.Error("expected checkFFmpeg to fail for non-existent binary, got nil")
	} else if !strings.Contains(err.Error(), "ffmpeg binary not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGenerateTTSOpenAI(t *testing.T) {
	expectedBytes := []byte("openai-tts-audio-bytes")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST request, got: %s", r.Method)
		}
		if r.URL.Path != "/audio/speech" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer mock-openai-key" {
			t.Errorf("unexpected authorization header: %s", r.Header.Get("Authorization"))
		}

		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
			Voice string `json:"voice"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			t.Errorf("failed to decode request body: %v", decodeErr)
		}
		if req.Input != "hello book parody" {
			t.Errorf("unexpected input text: %s", req.Input)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expectedBytes)
	}))
	defer server.Close()

	t.Setenv("OPENAI_BASE_URL", server.URL)

	cfg := &config.Config{}
	svc := NewViralService(t.TempDir(), cfg)

	// Test success
	audio, err := svc.generateTTSOpenAI(context.Background(), "mock-openai-key", "tts-1", "alloy", "hello book parody")
	if err != nil {
		t.Fatalf("generateTTSOpenAI failed: %v", err)
	}
	if string(audio) != string(expectedBytes) {
		t.Errorf("expected audio bytes %s, got %s", expectedBytes, audio)
	}

	// Test error empty key
	_, err = svc.generateTTSOpenAI(context.Background(), "", "", "", "test")
	if err == nil {
		t.Error("expected error for empty API key, got nil")
	}

	// Test error non-200 status
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer errServer.Close()

	t.Setenv("OPENAI_BASE_URL", errServer.URL)
	_, err = svc.generateTTSOpenAI(context.Background(), "mock-key", "tts-1", "alloy", "test")
	if err == nil {
		t.Error("expected error for non-200 status code, got nil")
	}
}

func TestGenerateTTSElevenLabs(t *testing.T) {
	expectedBytes := []byte("elevenlabs-tts-audio-bytes")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST request, got: %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/text-to-speech/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("xi-api-key") != "mock-eleven-key" {
			t.Errorf("unexpected xi-api-key header: %s", r.Header.Get("xi-api-key"))
		}

		var req struct {
			Text    string `json:"text"`
			ModelID string `json:"model_id"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			t.Errorf("failed to decode request: %v", decodeErr)
		}
		if req.Text != "hello ASMR text" {
			t.Errorf("unexpected text: %s", req.Text)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expectedBytes)
	}))
	defer server.Close()

	t.Setenv("ELEVENLABS_BASE_URL", server.URL)

	cfg := &config.Config{}
	svc := NewViralService(t.TempDir(), cfg)

	// Test success
	audio, err := svc.generateTTSElevenLabs(context.Background(), "mock-eleven-key", "rachel-voice", "hello ASMR text")
	if err != nil {
		t.Fatalf("generateTTSElevenLabs failed: %v", err)
	}
	if string(audio) != string(expectedBytes) {
		t.Errorf("expected audio bytes %s, got %s", expectedBytes, audio)
	}

	// Test empty api key error
	_, err = svc.generateTTSElevenLabs(context.Background(), "", "", "test")
	if err == nil {
		t.Error("expected error for empty API key, got nil")
	}

	// Test non-200 response
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer errServer.Close()

	t.Setenv("ELEVENLABS_BASE_URL", errServer.URL)
	_, err = svc.generateTTSElevenLabs(context.Background(), "mock-key", "rachel-voice", "test")
	if err == nil {
		t.Error("expected error for non-200 status, got nil")
	}
}

func TestGenerateVoiceover(t *testing.T) {
	openaiExpectedBytes := []byte("openai-audio")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openaiExpectedBytes)
	}))
	defer server.Close()
	t.Setenv("OPENAI_BASE_URL", server.URL)

	tmpDir := t.TempDir()
	cfg := &config.Config{
		APIKeys: config.APIKeys{
			OpenAI: "mock-key",
		},
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				TTSProvider: "openai",
			},
		},
	}

	svc := NewViralService(tmpDir, cfg)

	// 1. Success OpenAI
	filePath, err := svc.GenerateVoiceover(context.Background(), "hello testing voice", "", "")
	if err != nil {
		t.Fatalf("GenerateVoiceover failed: %v", err)
	}
	if !strings.HasPrefix(filePath, tmpDir) {
		t.Errorf("expected path to start with %s, got %s", tmpDir, filePath)
	}
	//nolint:gosec // filePath is safely constructed in tests
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read voiceover file: %v", err)
	}
	if string(data) != string(openaiExpectedBytes) {
		t.Errorf("expected file contents %q, got %q", string(openaiExpectedBytes), string(data))
	}

	// 2. Unsupported TTS provider
	_, err = svc.GenerateVoiceover(context.Background(), "test", "", "invalid-provider")
	if err == nil {
		t.Error("expected error for invalid provider, got nil")
	}

	// 3. Script empty error
	_, err = svc.GenerateVoiceover(context.Background(), "", "", "")
	if err == nil {
		t.Error("expected error for empty script, got nil")
	}

	// 4. ElevenLabs Provider Success Mock HTTP
	elExpectedBytes := []byte("eleven-audio")
	elServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(elExpectedBytes)
	}))
	defer elServer.Close()
	t.Setenv("ELEVENLABS_BASE_URL", elServer.URL)

	t.Setenv("ELEVENLABS_API_KEY", "env-key")
	filePathEL, err := svc.GenerateVoiceover(context.Background(), "hello testing voice 2", "voice-123", "elevenlabs")
	if err != nil {
		t.Fatalf("GenerateVoiceover ElevenLabs failed: %v", err)
	}
	//nolint:gosec // filePathEL is safely constructed in tests
	dataEL, _ := os.ReadFile(filePathEL)
	if string(dataEL) != string(elExpectedBytes) {
		t.Errorf("expected file contents %q, got %q", string(elExpectedBytes), string(dataEL))
	}

	// 5. Mock Provider (Success)
	mockFFmpegPath := createMockFFmpeg(t)

	cfgMock := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: mockFFmpegPath,
			},
		},
	}
	svcMock := NewViralService(tmpDir, cfgMock)
	filePathMock, err := svcMock.GenerateVoiceover(context.Background(), "hello testing mock", "", "mock")
	if err != nil {
		t.Fatalf("GenerateVoiceover mock failed: %v", err)
	}
	if _, statErr := os.Stat(filePathMock); os.IsNotExist(statErr) {
		t.Error("expected mock voiceover file to exist, but it does not")
	}

	// 6. Mock Provider with missing/invalid FFmpeg path
	cfgNoFFmpeg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: "non-existent-binary-9999",
			},
		},
	}
	svcNoFFmpeg := NewViralService(tmpDir, cfgNoFFmpeg)
	filePathMock2, err := svcNoFFmpeg.GenerateVoiceover(context.Background(), "hello testing mock 2", "", "mock")
	if err != nil {
		t.Fatalf("GenerateVoiceover mock failed on missing ffmpeg: %v", err)
	}
	//nolint:gosec // filePathMock2 is safely constructed in tests
	dataMock2, _ := os.ReadFile(filePathMock2)
	if string(dataMock2) != "mock-mp3-audio-bytes-ffmpeg-not-installed" {
		t.Errorf("unexpected fallback content: %s", string(dataMock2))
	}
}

func TestGenerateVideo(t *testing.T) {
	tmpDir := t.TempDir()

	mockFFmpegPath := createMockFFmpeg(t)

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "mock",
				FFmpegPath:   mockFFmpegPath,
			},
		},
	}
	svc := NewViralService(tmpDir, cfg)

	// 1. Script empty error
	_, err := svc.GenerateVideo(context.Background(), "", "")
	if err == nil {
		t.Error("expected error for empty prompt, got nil")
	}

	// 2. Mock video generation
	filePath, err := svc.GenerateVideo(context.Background(), "a blue sky background", "1024x1024")
	if err != nil {
		t.Fatalf("GenerateVideo mock failed: %v", err)
	}
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		t.Error("expected mock video file to exist, but it does not")
	}

	// 3. Mock video generation where FFmpegPath fails/missing
	cfgNoFFmpeg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "mock",
				FFmpegPath:   "non-existent-binary-9999",
			},
		},
	}
	svcNoFFmpeg := NewViralService(tmpDir, cfgNoFFmpeg)
	filePath2, err := svcNoFFmpeg.GenerateVideo(context.Background(), "prompt", "")
	if err != nil {
		t.Fatalf("expected GenerateVideo to fall back to dummy write, got error: %v", err)
	}
	//nolint:gosec // filePath2 is safely constructed in tests
	data2, _ := os.ReadFile(filePath2)
	if string(data2) != "mock-mp4-video-bytes-ffmpeg-not-installed" {
		t.Errorf("unexpected fallback content: %s", string(data2))
	}

	// 4. Unsupported backend
	cfgBad := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "invalid-backend",
			},
		},
	}
	svcBad := NewViralService(tmpDir, cfgBad)
	_, err = svcBad.GenerateVideo(context.Background(), "prompt", "")
	if err == nil {
		t.Error("expected error for invalid video backend, got nil")
	}
}

func TestGenerateVideo_Veo(t *testing.T) {
	// Mock Veo API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock prediction initiate
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, ":predictLongRunning") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"operations/veo-op-viral-1"}`))
			return
		}
		// Mock operation polling
		if r.Method == "GET" && strings.Contains(r.URL.Path, "operations/veo-op-viral-1") {
			w.WriteHeader(http.StatusOK)
			//nolint:gosec // r.Host is dynamically evaluated for test requests
			_, _ = fmt.Fprintf(w, `{"done":true,"response":{"generatedVideos":[{"video":{"uri":"http://%s/video/uri-123"}}]}}`, r.Host)
			return
		}
		// Mock download
		if r.Method == "GET" && strings.Contains(r.URL.Path, "video/uri-123") {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("mock-veo-video-bytes"))
			return
		}
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)

	tmpDir := t.TempDir()
	cfg := &config.Config{
		APIKeys: config.APIKeys{
			Gemini: "mock-gemini-key",
		},
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "veo",
			},
		},
	}
	svc := NewViralService(tmpDir, cfg)

	filePath, err := svc.GenerateVideo(context.Background(), "a beautiful landscape", "1024x1792")
	if err != nil {
		t.Fatalf("GenerateVideo Veo failed: %v", err)
	}

	//nolint:gosec // filePath is safely constructed in tests
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read generated video: %v", err)
	}
	if string(data) != "mock-veo-video-bytes" {
		t.Errorf("expected mock-veo-video-bytes, got %s", string(data))
	}
}

func TestStitchTrailer_Success(t *testing.T) {
	tmpDir := t.TempDir()
	mockFFmpegPath := createMockFFmpeg(t)

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: mockFFmpegPath,
			},
		},
	}
	svc := NewViralService(tmpDir, cfg)

	videoPath := filepath.Join(tmpDir, "input_video.mp4")
	audioPath := filepath.Join(tmpDir, "input_audio.mp3")
	bgAudioPath := filepath.Join(tmpDir, "bg_audio.mp3")

	_ = os.WriteFile(videoPath, []byte("fake-video"), 0600)
	_ = os.WriteFile(audioPath, []byte("fake-audio"), 0600)
	_ = os.WriteFile(bgAudioPath, []byte("fake-bg"), 0600)

	// 1. Success without background music
	outputPath, err := svc.StitchTrailer(context.Background(), videoPath, audioPath, "", "out_trailer.mp4")
	if err != nil {
		t.Fatalf("StitchTrailer failed: %v", err)
	}
	if !strings.HasSuffix(outputPath, "out_trailer.mp4") {
		t.Errorf("unexpected output path suffix: %s", outputPath)
	}

	// 2. Success with background music
	outputPathBg, err := svc.StitchTrailer(context.Background(), videoPath, audioPath, bgAudioPath, "")
	if err != nil {
		t.Fatalf("StitchTrailer failed: %v", err)
	}
	if !strings.HasSuffix(outputPathBg, ".mp4") {
		t.Errorf("unexpected output path format: %s", outputPathBg)
	}

	// 3. Test empty outputName and missing suffix
	outputPathEmpty, err := svc.StitchTrailer(context.Background(), videoPath, audioPath, "", "")
	if err != nil {
		t.Fatalf("StitchTrailer with empty output name failed: %v", err)
	}
	if !strings.HasSuffix(outputPathEmpty, ".mp4") {
		t.Errorf("expected suffix .mp4, got %s", outputPathEmpty)
	}

	outputPathNoExt, err := svc.StitchTrailer(context.Background(), videoPath, audioPath, "", "out_no_ext")
	if err != nil {
		t.Fatalf("StitchTrailer with no ext output name failed: %v", err)
	}
	if !strings.HasSuffix(outputPathNoExt, "out_no_ext.mp4") {
		t.Errorf("expected out_no_ext.mp4, got %s", outputPathNoExt)
	}
}

func TestStitchTrailer_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	mockFFmpegPath := createMockFFmpeg(t)

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: mockFFmpegPath,
			},
		},
	}
	svc := NewViralService(tmpDir, cfg)

	videoPath := filepath.Join(tmpDir, "input_video.mp4")
	audioPath := filepath.Join(tmpDir, "input_audio.mp3")
	bgAudioPath := filepath.Join(tmpDir, "bg_audio.mp3")

	_ = os.WriteFile(videoPath, []byte("fake-video"), 0600)
	_ = os.WriteFile(audioPath, []byte("fake-audio"), 0600)
	_ = os.WriteFile(bgAudioPath, []byte("fake-bg"), 0600)

	// 1. Error: inputs do not exist
	_, err := svc.StitchTrailer(context.Background(), "non-existent-video.mp4", audioPath, "", "")
	if err == nil {
		t.Error("expected error for non-existent video, got nil")
	}

	_, err = svc.StitchTrailer(context.Background(), videoPath, "non-existent-audio.mp3", "", "")
	if err == nil {
		t.Error("expected error for non-existent audio, got nil")
	}

	_, err = svc.StitchTrailer(context.Background(), videoPath, audioPath, "non-existent-bg.mp3", "")
	if err == nil {
		t.Error("expected error for non-existent background audio, got nil")
	}

	// 2. Check ffmpeg check fails
	cfgBadFFmpeg := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: "non-existent-ffmpeg-path-999",
			},
		},
	}
	svcBadFFmpeg := NewViralService(tmpDir, cfgBadFFmpeg)
	_, err = svcBadFFmpeg.StitchTrailer(context.Background(), videoPath, audioPath, "", "")
	if err == nil {
		t.Error("expected error when ffmpeg path is invalid, got nil")
	}

	// 3. Test ffmpeg command failure
	failedFFmpeg := filepath.Join(tmpDir, "failed_ffmpeg")
	scriptContent := `#!/bin/sh
echo "mock-failed-stderr" >&2
exit 1
`
	//nolint:gosec // mock script needs to be executable
	_ = os.WriteFile(failedFFmpeg, []byte(scriptContent), 0755)

	cfgFailed := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: failedFFmpeg,
			},
		},
	}
	svcFailed := NewViralService(tmpDir, cfgFailed)
	_, err = svcFailed.StitchTrailer(context.Background(), videoPath, audioPath, "", "")
	if err == nil {
		t.Error("expected error for failed ffmpeg, got nil")
	} else if !strings.Contains(err.Error(), "mock-failed-stderr") {
		t.Errorf("expected error message to contain stderr, got: %v", err)
	}
}

func TestCheckFFmpeg_Empty(t *testing.T) {
	_ = checkFFmpeg("")
}

func TestGenerateVoiceover_FallbacksAndDefaults(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. provider and config both empty should default to openai, and fail on missing api keys
	cfgEmpty := &config.Config{}
	svcEmpty := NewViralService(tmpDir, cfgEmpty)
	_, err := svcEmpty.GenerateVoiceover(context.Background(), "hello", "", "")
	if err == nil {
		t.Error("expected error for empty provider and no keys, got nil")
	}

	// 2. config fallback for api keys and voice ID
	openaiExpectedBytes := []byte("openai-audio-fallback")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openaiExpectedBytes)
	}))
	defer server.Close()
	t.Setenv("OPENAI_BASE_URL", server.URL)

	cfgFallback := &config.Config{
		APIKeys: config.APIKeys{
			OpenAI: "fallback-global-key",
		},
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				TTSVoiceID: "fallback-voice-id",
			},
		},
	}
	svcFallback := NewViralService(tmpDir, cfgFallback)
	filePath, err := svcFallback.GenerateVoiceover(context.Background(), "hello", "", "openai")
	if err != nil {
		t.Fatalf("GenerateVoiceover with fallbacks failed: %v", err)
	}
	//nolint:gosec // filePath is safely constructed in tests
	data, _ := os.ReadFile(filePath)
	if string(data) != string(openaiExpectedBytes) {
		t.Errorf("expected %s, got %s", string(openaiExpectedBytes), string(data))
	}
}

func TestGenerateVideo_VeoErrors(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Missing Gemini key error
	cfgEmpty := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "veo",
			},
		},
	}
	svcEmpty := NewViralService(tmpDir, cfgEmpty)
	_, err := svcEmpty.GenerateVideo(context.Background(), "hello", "")
	if err == nil {
		t.Error("expected error for missing gemini key in veo backend, got nil")
	}

	// 2. Veo prediction fails (polling timeout or non-200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	t.Setenv("GOOGLE_BASE_URL", server.URL)
	cfgErr := &config.Config{
		APIKeys: config.APIKeys{
			Gemini: "mock-key",
		},
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "veo",
			},
		},
	}
	svcErr := NewViralService(tmpDir, cfgErr)
	_, err = svcErr.GenerateVideo(context.Background(), "hello", "")
	if err == nil {
		t.Error("expected error when Veo initiate fails, got nil")
	}
}

func TestUncoveredBranches(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. generateTTSOpenAI base case defaults and connection failure
	cfg := &config.Config{}
	svc := NewViralService(tmpDir, cfg)
	t.Setenv("OPENAI_BASE_URL", "http://[invalid-url]")
	_, err := svc.generateTTSOpenAI(context.Background(), "mock-key", "", "", "hello")
	if err == nil {
		t.Error("expected connection error for invalid URL, got nil")
	}

	// 2. generateTTSElevenLabs base case defaults and connection failure
	t.Setenv("ELEVENLABS_BASE_URL", "http://[invalid-url]")
	_, err = svc.generateTTSElevenLabs(context.Background(), "mock-key", "", "hello")
	if err == nil {
		t.Error("expected connection error for invalid URL, got nil")
	}

	// 3. runMockVideo write error (directory as write path)
	err = svc.runMockVideo(context.Background(), "hello", "1024x1024", tmpDir)
	if err == nil {
		t.Error("expected write error for writing to a directory, got nil")
	}

	// 4. runMockVideo command run failure (failed command script)
	failedFFmpeg := filepath.Join(tmpDir, "failed_ffmpeg_mock")
	//nolint:gosec // mock script needs to be executable
	_ = os.WriteFile(failedFFmpeg, []byte("#!/bin/sh\nexit 1\n"), 0755)
	cfgFailed := &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				FFmpegPath: failedFFmpeg,
			},
		},
	}
	svcFailed := NewViralService(tmpDir, cfgFailed)
	err = svcFailed.runMockVideo(context.Background(), "hello", "1024x1024", filepath.Join(tmpDir, "out.mp4"))
	if err == nil {
		t.Error("expected command execution failure, got nil")
	}

	// 5. MkdirAll directory creation failure in GenerateVoiceover, GenerateVideo, and StitchTrailer
	blockedDir := filepath.Join(tmpDir, "generated_media")
	_ = os.WriteFile(blockedDir, []byte("blocker"), 0600)

	svcBlocked := NewViralService(tmpDir, &config.Config{
		Plugins: config.PluginsConfig{
			Viral: config.ViralConfig{
				VideoBackend: "mock",
			},
		},
	})
	_, err = svcBlocked.GenerateVoiceover(context.Background(), "hello", "", "mock")
	if err == nil {
		t.Error("expected mkdir error in GenerateVoiceover, got nil")
	}

	_, err = svcBlocked.GenerateVideo(context.Background(), "hello", "")
	if err == nil {
		t.Error("expected mkdir error in GenerateVideo, got nil")
	}

	_, err = svcBlocked.StitchTrailer(context.Background(), filepath.Join(tmpDir, "v.mp4"), filepath.Join(tmpDir, "a.mp3"), "", "")
	if err == nil {
		t.Error("expected mkdir error in StitchTrailer, got nil")
	}

	// 6. generateTTSMock command run failure (failed command script)
	err = svcFailed.generateTTSMock(context.Background(), filepath.Join(tmpDir, "out.m4a"))
	if err == nil {
		t.Error("expected generateTTSMock ffmpeg command execution failure, got nil")
	}
}
