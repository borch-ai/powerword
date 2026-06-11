package viral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"powerword/internal/config"
	"powerword/internal/plugins/imagegen"
)

// ViralService coordinates voiceover generation, video generation, and ffmpeg stitching.
type ViralService struct {
	workspaceRoot string
	cfg           *config.Config
}

// NewViralService creates a new ViralService.
func NewViralService(workspaceRoot string, cfg *config.Config) *ViralService {
	return &ViralService{
		workspaceRoot: workspaceRoot,
		cfg:           cfg,
	}
}

// checkFFmpeg verifies if the ffmpeg binary is available in the path.
func checkFFmpeg(customPath string) error {
	path := customPath
	if path == "" {
		path = "ffmpeg"
	}
	_, err := exec.LookPath(path)
	if err != nil {
		return fmt.Errorf("ffmpeg binary not found. Please ensure ffmpeg is installed and added to your PATH, or configure it via [plugins.viral.ffmpeg_path]. Installation helper: run 'brew install ffmpeg' on macOS or visit https://ffmpeg.org/download.html")
	}
	return nil
}

// generateTTSOpenAI handles speech synthesis using OpenAI's TTS service.
func (s *ViralService) generateTTSOpenAI(ctx context.Context, apiKey, model, voice, input string) ([]byte, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai API key is not configured")
	}
	if model == "" {
		model = "tts-1"
	}
	if voice == "" {
		voice = "alloy"
	}

	payload := map[string]string{
		"model": model,
		"input": input,
		"voice": voice,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI TTS payload: %w", err)
	}

	apiURL := "https://api.openai.com/v1/audio/speech"
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		apiURL = baseURL + "/audio/speech"
	}

	//nolint:gosec // apiURL is validated or retrieved from trusted config/environment
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI TTS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	//nolint:gosec // request is sent to verified OpenAI API resource
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI TTS request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI TTS backend returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenAI TTS response audio content: %w", err)
	}

	return audioBytes, nil
}

// generateTTSElevenLabs handles speech synthesis using ElevenLabs text-to-speech.
func (s *ViralService) generateTTSElevenLabs(ctx context.Context, apiKey, voiceID, input string) ([]byte, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("elevenlabs API key is not configured")
	}
	if voiceID == "" {
		voiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel
	}

	payload := map[string]interface{}{
		"text":     input,
		"model_id": "eleven_monolingual_v1",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ElevenLabs TTS payload: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID)
	if baseURL := os.Getenv("ELEVENLABS_BASE_URL"); baseURL != "" {
		apiURL = fmt.Sprintf("%s/v1/text-to-speech/%s", baseURL, voiceID)
	}

	//nolint:gosec // apiURL is validated or retrieved from trusted config/environment
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create ElevenLabs TTS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	//nolint:gosec // request is sent to verified ElevenLabs API resource
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ElevenLabs TTS request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ElevenLabs TTS backend returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ElevenLabs TTS response audio content: %w", err)
	}

	return audioBytes, nil
}

// runMockVideo generates a dummy video using ffmpeg or placeholder content.
func (s *ViralService) runMockVideo(ctx context.Context, prompt, size string, outPath string) error {
	ffmpegErr := checkFFmpeg(s.cfg.Plugins.Viral.FFmpegPath)
	if ffmpegErr == nil {
		ffmpegCmd := s.cfg.Plugins.Viral.FFmpegPath
		if ffmpegCmd == "" {
			ffmpegCmd = "ffmpeg"
		}
		//nolint:gosec
		cmd := exec.CommandContext(ctx, ffmpegCmd, "-y", "-f", "lavfi", "-i", "color=c=blue:s=320x240:d=5", "-c:v", "libx264", "-pix_fmt", "yuv420p", outPath)
		if runErr := cmd.Run(); runErr != nil {
			return fmt.Errorf("failed to run mock video generator command: %w", runErr)
		}
		return nil
	}

	dummyData := []byte("mock-mp4-video-bytes-ffmpeg-not-installed")
	if writeErr := os.WriteFile(outPath, dummyData, 0600); writeErr != nil {
		return fmt.Errorf("failed to write dummy mock video file: %w", writeErr)
	}
	return nil
}

// generateTTSMock creates a mock speech track.
func (s *ViralService) generateTTSMock(ctx context.Context, filePath string) error {
	ffmpegErr := checkFFmpeg(s.cfg.Plugins.Viral.FFmpegPath)
	if ffmpegErr == nil {
		ffmpegCmd := s.cfg.Plugins.Viral.FFmpegPath
		if ffmpegCmd == "" {
			ffmpegCmd = "ffmpeg"
		}
		//nolint:gosec
		cmd := exec.CommandContext(ctx, ffmpegCmd, "-y", "-f", "lavfi", "-i", "sine=frequency=1000:duration=5", "-c:a", "aac", filePath)
		if runErr := cmd.Run(); runErr != nil {
			return fmt.Errorf("failed to generate mock audio via ffmpeg: %w", runErr)
		}
		return nil
	}

	dummyBytes := []byte("mock-mp3-audio-bytes-ffmpeg-not-installed")
	if writeErr := os.WriteFile(filePath, dummyBytes, 0600); writeErr != nil {
		return fmt.Errorf("failed to write dummy audio file: %w", writeErr)
	}
	return nil
}

// runVeo calls the Google Veo backend from imagegen.
func (s *ViralService) runVeo(ctx context.Context, prompt, size string) ([]byte, error) {
	apiKey := s.cfg.Plugins.Viral.VideoAPIKey
	if apiKey == "" {
		apiKey = s.cfg.APIKeys.Gemini
	}
	if apiKey == "" {
		return nil, fmt.Errorf("google/gemini API key is not configured for Veo (set plugins.viral.video_api_key or api_keys.gemini)")
	}

	backend, err := imagegen.NewVeoBackend(
		apiKey,
		"veo-2.0-generate-001",
		"5s",
		"5m",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Veo backend: %w", err)
	}

	videoBytes, _, err := backend.GenerateImage(ctx, prompt, size)
	if err != nil {
		return nil, err
	}
	return videoBytes, nil
}

// GenerateVoiceover synthesizes narration.
func (s *ViralService) GenerateVoiceover(ctx context.Context, script string, voiceID string, provider string) (string, error) {
	if script == "" {
		return "", fmt.Errorf("script cannot be empty")
	}

	prov := provider
	if prov == "" {
		prov = s.cfg.Plugins.Viral.TTSProvider
	}
	prov = strings.ToLower(prov)
	if prov == "" {
		prov = "openai"
	}

	dir := filepath.Join(s.workspaceRoot, "generated_media")
	if mkdirErr := os.MkdirAll(dir, 0750); mkdirErr != nil {
		return "", fmt.Errorf("failed to create generated_media directory: %w", mkdirErr)
	}

	ext := ".mp3"
	if prov == "mock" {
		ext = ".m4a"
	}
	filename := fmt.Sprintf("voiceover_%d%s", time.Now().UnixNano(), ext)
	filePath := filepath.Join(dir, filename)

	var audioBytes []byte
	var err error

	switch prov {
	case "openai":
		apiKey := s.cfg.Plugins.Viral.TTSAPIKey
		if apiKey == "" {
			apiKey = s.cfg.APIKeys.OpenAI
		}
		voice := voiceID
		if voice == "" {
			voice = s.cfg.Plugins.Viral.TTSVoiceID
		}
		audioBytes, err = s.generateTTSOpenAI(ctx, apiKey, "tts-1", voice, script)
		if err != nil {
			return "", err
		}
	case "elevenlabs":
		apiKey := s.cfg.Plugins.Viral.TTSAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("ELEVENLABS_API_KEY")
		}
		audioBytes, err = s.generateTTSElevenLabs(ctx, apiKey, voiceID, script)
		if err != nil {
			return "", err
		}
	case "mock":
		if mockErr := s.generateTTSMock(ctx, filePath); mockErr != nil {
			return "", mockErr
		}
		return filePath, nil
	default:
		return "", fmt.Errorf("unsupported TTS provider %q", prov)
	}

	if saveErr := os.WriteFile(filePath, audioBytes, 0600); saveErr != nil {
		return "", fmt.Errorf("failed to save voiceover file: %w", saveErr)
	}

	return filePath, nil
}

// GenerateVideo generates a background clip.
func (s *ViralService) GenerateVideo(ctx context.Context, prompt string, size string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}

	backend := strings.ToLower(s.cfg.Plugins.Viral.VideoBackend)
	if backend == "" {
		backend = "mock"
	}

	dir := filepath.Join(s.workspaceRoot, "generated_media")
	if mkdirErr := os.MkdirAll(dir, 0750); mkdirErr != nil {
		return "", fmt.Errorf("failed to create generated_media directory: %w", mkdirErr)
	}

	filename := fmt.Sprintf("video_%d.mp4", time.Now().UnixNano())
	filePath := filepath.Join(dir, filename)

	switch backend {
	case "mock":
		if mockErr := s.runMockVideo(ctx, prompt, size, filePath); mockErr != nil {
			return "", mockErr
		}
		return filePath, nil
	case "veo":
		videoBytes, err := s.runVeo(ctx, prompt, size)
		if err != nil {
			return "", err
		}
		if saveErr := os.WriteFile(filePath, videoBytes, 0600); saveErr != nil {
			return "", fmt.Errorf("failed to save generated video file: %w", saveErr)
		}
		return filePath, nil
	default:
		return "", fmt.Errorf("unsupported video backend %q", backend)
	}
}

// StitchTrailer multiplexes audio and video inputs together.
func (s *ViralService) StitchTrailer(ctx context.Context, videoPath, audioPath, backgroundAudioPath, outputName string) (string, error) {
	if err := checkFFmpeg(s.cfg.Plugins.Viral.FFmpegPath); err != nil {
		return "", err
	}

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file does not exist: %s", videoPath)
	}
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		return "", fmt.Errorf("audio file does not exist: %s", audioPath)
	}
	if backgroundAudioPath != "" {
		if _, err := os.Stat(backgroundAudioPath); os.IsNotExist(err) {
			return "", fmt.Errorf("background audio file does not exist: %s", backgroundAudioPath)
		}
	}

	dir := filepath.Join(s.workspaceRoot, "generated_media")
	if mkdirErr := os.MkdirAll(dir, 0750); mkdirErr != nil {
		return "", fmt.Errorf("failed to create generated_media directory: %w", mkdirErr)
	}

	outName := outputName
	if outName == "" {
		outName = fmt.Sprintf("trailer_%d.mp4", time.Now().UnixNano())
	} else {
		outName = filepath.Base(outName)
	}
	if !strings.HasSuffix(outName, ".mp4") {
		outName += ".mp4"
	}
	outputPath := filepath.Join(dir, outName)

	ffmpegCmd := s.cfg.Plugins.Viral.FFmpegPath
	if ffmpegCmd == "" {
		ffmpegCmd = "ffmpeg"
	}

	var args []string
	if backgroundAudioPath == "" {
		args = []string{
			"-y",
			"-i", videoPath,
			"-i", audioPath,
			"-map", "0:v",
			"-map", "1:a",
			"-c:v", "copy",
			"-c:a", "aac",
			"-shortest",
			outputPath,
		}
	} else {
		args = []string{
			"-y",
			"-i", videoPath,
			"-i", audioPath,
			"-i", backgroundAudioPath,
			"-filter_complex", "[1:a][2:a]amix=inputs=2:duration=first:dropout_transition=2[a]",
			"-map", "0:v",
			"-map", "[a]",
			"-c:v", "copy",
			"-c:a", "aac",
			"-shortest",
			outputPath,
		}
	}

	//nolint:gosec
	cmd := exec.CommandContext(ctx, ffmpegCmd, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg stitching failed: %w, stderr: %s", err, stderr.String())
	}

	return outputPath, nil
}
