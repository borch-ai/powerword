package telemetry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// SubmitBatch sends a slice of telemetry events to the Lighthouse collector batch endpoint.
func (a *LighthouseAdapter) SubmitBatch(ctx context.Context, events []TelemetryEvent) error {
	if len(events) == 0 {
		return nil
	}
	if a.URL == "" {
		return fmt.Errorf("lighthouse URL is required")
	}

	payload, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry payload batch: %w", err)
	}

	url := strings.TrimSuffix(a.URL, "/") + "/api/telemetry/batch"
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

// spoolOfflineEvent appends a telemetry event to the local spool file.
func spoolOfflineEvent(e TelemetryEvent) {
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		log.Printf("Warning: failed to get user home directory for telemetry spool: %v", homeErr)
		return
	}

	spoolDir := filepath.Join(home, ".local", "share", "powerword")
	if dirErr := os.MkdirAll(spoolDir, 0750); dirErr != nil {
		log.Printf("Warning: failed to create telemetry spool directory: %v", dirErr)
		return
	}

	spoolPath := filepath.Join(spoolDir, "telemetry_spool.jsonl")
	payload, marshalErr := json.Marshal(e)
	if marshalErr != nil {
		log.Printf("Warning: failed to marshal telemetry event for spooling: %v", marshalErr)
		return
	}

	//nolint:gosec // G304: path is resolved from secure UserHomeDir
	f, openErr := os.OpenFile(spoolPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if openErr != nil {
		log.Printf("Warning: failed to open telemetry spool file: %v", openErr)
		return
	}
	defer func() { _ = f.Close() }()

	if _, writeErr := f.Write(append(payload, '\n')); writeErr != nil {
		log.Printf("Warning: failed to write to telemetry spool file: %v", writeErr)
	}
}

// readSpooledEvents opens the sync file, reads and parses events line by line.
func readSpooledEvents(tempSyncPath string) ([]TelemetryEvent, error) {
	//nolint:gosec // G304: path is resolved from secure UserHomeDir
	f, err := os.Open(tempSyncPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var events []TelemetryEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev TelemetryEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			log.Printf("Warning: failed to unmarshal telemetry event from sync file: %v", err)
			continue
		}
		events = append(events, ev)
	}
	return events, scanner.Err()
}

// sendBatchEvents posts events in batches of 100 to the Lighthouse adapter.
func sendBatchEvents(ctx context.Context, adapter *LighthouseAdapter, events []TelemetryEvent) error {
	const batchSize = 100
	for i := 0; i < len(events); i += batchSize {
		end := i + batchSize
		if end > len(events) {
			end = len(events)
		}
		if err := adapter.SubmitBatch(ctx, events[i:end]); err != nil {
			return err
		}
	}
	return nil
}

// rollbackSpooledEvents writes events back to the spool file on sync failure.
func rollbackSpooledEvents(spoolPath string, events []TelemetryEvent) {
	//nolint:gosec // G304: path is resolved from secure UserHomeDir
	spoolFile, err := os.OpenFile(spoolPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		log.Printf("Warning: failed to open telemetry spool file for rollback: %v", err)
		return
	}
	defer func() { _ = spoolFile.Close() }()

	for _, ev := range events {
		payload, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		_, _ = spoolFile.Write(append(payload, '\n'))
	}
}

// SyncSpooledEvents processes any spooled telemetry events and sends them to Lighthouse.
func SyncSpooledEvents(ctx context.Context, url, apiKey string) {
	if url == "" {
		return
	}

	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return
	}

	spoolDir := filepath.Join(home, ".local", "share", "powerword")
	spoolPath := filepath.Join(spoolDir, "telemetry_spool.jsonl")

	fi, statErr := os.Stat(spoolPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return
		}
		log.Printf("Warning: failed to stat telemetry spool file: %v", statErr)
		return
	}
	if fi.Size() == 0 {
		return
	}

	// Atomic Rename strategy
	tempSyncPath := filepath.Join(spoolDir, fmt.Sprintf("telemetry_spool_sync_%d.jsonl", time.Now().UnixNano()))
	if renameErr := os.Rename(spoolPath, tempSyncPath); renameErr != nil {
		log.Printf("Warning: failed to rename telemetry spool file: %v", renameErr)
		return
	}

	events, readErr := readSpooledEvents(tempSyncPath)
	if readErr != nil {
		log.Printf("Warning: error reading temporary telemetry sync file: %v", readErr)
	}

	if len(events) == 0 {
		_ = os.Remove(tempSyncPath)
		return
	}

	adapter := &LighthouseAdapter{
		URL:    url,
		APIKey: apiKey,
	}

	if syncErr := sendBatchEvents(ctx, adapter, events); syncErr == nil {
		if removeErr := os.Remove(tempSyncPath); removeErr != nil {
			log.Printf("Warning: failed to remove temporary telemetry sync file: %v", removeErr)
		}
	} else {
		log.Printf("Warning: failed to sync spooled telemetry events: %v", syncErr)
		rollbackSpooledEvents(spoolPath, events)
		_ = os.Remove(tempSyncPath)
	}
}

var (
	wg       sync.WaitGroup
	wgMu     sync.Mutex
	syncOnce sync.Once
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

	syncOnce.Do(func() {
		wgMu.Lock()
		wg.Add(1)
		wgMu.Unlock()
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			SyncSpooledEvents(ctx, url, apiKey)
		}()
	})

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
		}

		if err := adapter.Submit(ctx, e); err != nil {
			log.Printf("Warning: lighthouse telemetry submission failed: %v", err)
			spoolOfflineEvent(e)
		}
	}()
}

// Wait blocks until all pending background telemetry submissions complete.
func Wait() {
	wgMu.Lock()
	wg.Wait()
	wgMu.Unlock()
}
