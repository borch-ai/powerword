package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// TelemetryEvent represents a telemetry record submitted to Lighthouse.
type TelemetryEvent struct {
	Project      string            `json:"project"`
	Command      string            `json:"command"`
	Stage        string            `json:"stage"`
	DurationMs   int64             `json:"duration_ms"`
	CostUSD      float64           `json:"cost_usd"`
	TokensIn     int64             `json:"tokens_in"`
	TokensOut    int64             `json:"tokens_out"`
	TokensCached int64             `json:"tokens_cached"`
	ErrorMsg     string            `json:"error_msg,omitempty"`
	Meta         map[string]string `json:"meta,omitempty"`
}

// LighthouseAdapter submits a TelemetryEvent to a Lighthouse collector.
type LighthouseAdapter struct {
	URL    string
	APIKey string
	Client *http.Client
}

// Submit sends the telemetry event to the Lighthouse collector.
func (a *LighthouseAdapter) Submit(ctx context.Context, e TelemetryEvent) error {
	if a.URL == "" {
		return fmt.Errorf("lighthouse URL is required")
	}

	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry payload: %w", err)
	}

	url := strings.TrimSuffix(a.URL, "/") + "/api/telemetry"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if a.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.APIKey)
	}

	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr != "" {
			return fmt.Errorf("unexpected status code %d (%s): %s", resp.StatusCode, resp.Status, bodyStr)
		}
		return fmt.Errorf("unexpected status code %d (%s)", resp.StatusCode, resp.Status)
	}

	return nil
}

var (
	wg   sync.WaitGroup
	wgMu sync.Mutex
)

// SubmitToLighthouse reads configuration from the environment and submits
// a telemetry event to Lighthouse asynchronously in a background goroutine.
// It is a no-op if LIGHTHOUSE_URL is unset.
func SubmitToLighthouse(e TelemetryEvent) {
	url := os.Getenv("LIGHTHOUSE_URL")
	if url == "" {
		return
	}
	apiKey := os.Getenv("LIGHTHOUSE_API_KEY")

	wgMu.Lock()
	wg.Add(1)
	wgMu.Unlock()
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		adapter := &LighthouseAdapter{
			URL:    url,
			APIKey: apiKey,
			Client: &http.Client{Timeout: 5 * time.Second},
		}

		if err := adapter.Submit(ctx, e); err != nil {
			log.Printf("Warning: lighthouse telemetry submission failed: %v", err)
		}
	}()
}

// Wait blocks until all pending background telemetry submissions complete.
func Wait() {
	wgMu.Lock()
	wg.Wait()
	wgMu.Unlock()
}
