package telemetry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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

// HTTPError represents an HTTP status error returned by the collector.
type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("unexpected status code %d (%s): %s", e.StatusCode, e.Status, e.Body)
	}
	return fmt.Sprintf("unexpected status code %d (%s)", e.StatusCode, e.Status)
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
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       bodyStr,
		}
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
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       bodyStr,
		}
	}

	return nil
}

var lockTimeout = 200 * time.Millisecond

// withFileLock executes the given action while holding an exclusive filesystem-level lock
// on the spool file (via a lock file). It retries lock acquisition for up to lockTimeout.
func withFileLock(spoolPath string, action func() error) error {
	lockPath := spoolPath + ".lock"
	acquired := false

	// Try to acquire the lock using O_EXCL (atomic creation)
	for start := time.Now(); time.Since(start) < lockTimeout; time.Sleep(10 * time.Millisecond) {
		//nolint:gosec // G304: lockPath is resolved from secure UserHomeDir
		lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_ = lf.Close()
			acquired = true
			break
		}
	}

	if !acquired {
		return fmt.Errorf("failed to acquire telemetry lock")
	}

	defer func() {
		_ = os.Remove(lockPath)
	}()

	return action()
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

	_ = withFileLock(spoolPath, func() error {
		// Enforce max spool size limit of 5 MB to avoid unbounded disk growth
		const maxSpoolBytes = 5 * 1024 * 1024
		if fi, err := os.Stat(spoolPath); err == nil && fi.Size() >= maxSpoolBytes {
			log.Printf("Warning: telemetry spool file size limit exceeded; dropping event")
			return nil
		}

		//nolint:gosec // G304: path is resolved from secure UserHomeDir
		f, openErr := os.OpenFile(spoolPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if openErr != nil {
			return openErr
		}
		defer func() { _ = f.Close() }()

		if _, writeErr := f.Write(append(payload, '\n')); writeErr != nil {
			return writeErr
		}
		return nil
	})
}

// parseLine trims and unmarshals a single JSONL line into a TelemetryEvent.
func parseLine(line []byte) (TelemetryEvent, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return TelemetryEvent{}, false
	}
	var ev TelemetryEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		log.Printf("Warning: failed to unmarshal telemetry event from sync file: %v", err)
		return TelemetryEvent{}, false
	}
	return ev, true
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
	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			if readErr == io.EOF {
				if ev, ok := parseLine(line); ok {
					events = append(events, ev)
				}
				break
			}
			return nil, readErr
		}
		if ev, ok := parseLine(line); ok {
			events = append(events, ev)
		}
	}
	return events, nil
}

// sendBatchEvents posts events in batches of 100 to the Lighthouse adapter.
// Returns the count of successfully sent events and any error encountered.
func sendBatchEvents(ctx context.Context, adapter *LighthouseAdapter, events []TelemetryEvent) (int, error) {
	const batchSize = 100
	for i := 0; i < len(events); i += batchSize {
		end := i + batchSize
		if end > len(events) {
			end = len(events)
		}
		if err := adapter.SubmitBatch(ctx, events[i:end]); err != nil {
			return i, err
		}
	}
	return len(events), nil
}

// rollbackSpooledEvents writes events back to the spool file on sync failure.
func rollbackSpooledEvents(spoolPath string, events []TelemetryEvent) error {
	return withFileLock(spoolPath, func() error {
		// Enforce max spool size limit of 5 MB
		const maxSpoolBytes = 5 * 1024 * 1024
		if fi, err := os.Stat(spoolPath); err == nil && fi.Size() >= maxSpoolBytes {
			return fmt.Errorf("telemetry spool file size limit exceeded")
		}

		//nolint:gosec // G304: path is resolved from secure UserHomeDir
		spoolFile, err := os.OpenFile(spoolPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		defer func() { _ = spoolFile.Close() }()

		for _, ev := range events {
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, writeErr := spoolFile.Write(append(payload, '\n')); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
}

// processSyncFile processes a single temporary sync file: syncs to Lighthouse, deletes on success, rolls back on failure.
func processSyncFile(ctx context.Context, adapter *LighthouseAdapter, syncPath, spoolPath string) {
	events, readErr := readSpooledEvents(syncPath)
	if readErr != nil {
		log.Printf("Warning: error reading temporary telemetry sync file: %v", readErr)
		return // Keep the sync file so it is retried next time
	}

	if len(events) == 0 {
		if fi, err := os.Stat(syncPath); err == nil && fi.Size() == 0 {
			_ = os.Remove(syncPath)
		} else {
			log.Printf("Warning: temporary telemetry sync file is non-empty but contains no valid events; retaining for inspection: %s", syncPath)
		}
		return
	}

	sentCount, syncErr := sendBatchEvents(ctx, adapter, events)
	if syncErr != nil {
		log.Printf("Warning: failed to sync spooled telemetry events: %v", syncErr)
		unsentEvents := events[sentCount:]
		if len(unsentEvents) > 0 {
			rollbackErr := rollbackSpooledEvents(spoolPath, unsentEvents)
			if rollbackErr != nil {
				log.Printf("Warning: failed to rollback spooled telemetry events: %v", rollbackErr)
				return // Keep the sync file so we don't lose the telemetry events
			}
		}
		_ = os.Remove(syncPath)
		return
	}

	if removeErr := os.Remove(syncPath); removeErr != nil {
		log.Printf("Warning: failed to remove temporary telemetry sync file: %v", removeErr)
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

	adapter := &LighthouseAdapter{
		URL:    url,
		APIKey: apiKey,
	}

	// 1. Process any leftover stranded temporary sync files from previous runs
	files, readDirErr := os.ReadDir(spoolDir)
	if readDirErr == nil {
		for _, entry := range files {
			if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), "telemetry_spool_sync_") && strings.HasSuffix(entry.Name(), ".jsonl") {
				syncPath := filepath.Join(spoolDir, entry.Name())
				processSyncFile(ctx, adapter, syncPath, spoolPath)
			}
		}
	}

	// 2. Process the main spool file if it exists and has content
	tempSyncPath := filepath.Join(spoolDir, fmt.Sprintf("telemetry_spool_sync_%d.jsonl", time.Now().UnixNano()))

	renameErr := withFileLock(spoolPath, func() error {
		fi, statErr := os.Stat(spoolPath)
		if statErr != nil {
			return statErr
		}
		if fi.Size() == 0 {
			return fmt.Errorf("spool file is empty")
		}
		if err := os.Rename(spoolPath, tempSyncPath); err != nil {
			return err
		}
		return nil
	})

	if renameErr != nil {
		if !os.IsNotExist(renameErr) && renameErr.Error() != "spool file is empty" {
			log.Printf("Warning: failed to rename telemetry spool file: %v", renameErr)
		}
		return
	}

	processSyncFile(ctx, adapter, tempSyncPath, spoolPath)
}

var (
	wg       sync.WaitGroup
	wgMu     sync.Mutex
	syncOnce sync.Once
)

// isRetryableError determines if the error represents a retryable condition
// (such as network issues, timeouts, or 429/5xx HTTP responses).
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 1. Check context cancellation/timeout
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// 2. Check net.Error (timeouts, DNS failures, connection refused, etc.)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// 3. Check structured HTTP status error (429 or 5xx)
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == 429 || httpErr.StatusCode >= 500
	}

	// 4. Fallback string-based matching for backwards compatibility and generic error wrappers
	errStr := err.Error()
	if strings.Contains(errStr, "unexpected status code") {
		var code int
		// Scan format: "unexpected status code %d"
		if _, scanErr := fmt.Sscanf(errStr, "unexpected status code %d", &code); scanErr == nil {
			return code == 429 || code >= 500
		}
		// Fallback matches:
		return strings.Contains(errStr, "429") || strings.Contains(errStr, "500") || strings.Contains(errStr, "502") || strings.Contains(errStr, "503") || strings.Contains(errStr, "504")
	}
	return strings.Contains(errStr, "http request failed") ||
		strings.Contains(errStr, "context canceled") ||
		strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset")
}

// SubmitToLighthouse reads configuration from the environment and submits
// a telemetry event to Lighthouse asynchronously in a background goroutine.
// It is a no-op if LIGHTHOUSE_URL is unset.
func SubmitToLighthouse(e TelemetryEvent) {
	url := os.Getenv("LIGHTHOUSE_URL")
	if url == "" {
		return
	}
	apiKey := os.Getenv("LIGHTHOUSE_API_KEY")

	// Read timeout from environment, defaulting to 500ms for CLI flush budget
	timeout := 500 * time.Millisecond
	if tStr := os.Getenv("POWERWORD_TELEMETRY_TIMEOUT"); tStr != "" {
		if d, pErr := time.ParseDuration(tStr); pErr == nil {
			timeout = d
		}
	}

	syncOnce.Do(func() {
		wgMu.Lock()
		wg.Add(1)
		wgMu.Unlock()
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			SyncSpooledEvents(ctx, url, apiKey)
		}()
	})

	wgMu.Lock()
	wg.Add(1)
	wgMu.Unlock()
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		adapter := &LighthouseAdapter{
			URL:    url,
			APIKey: apiKey,
		}

		if err := adapter.Submit(ctx, e); err != nil {
			log.Printf("Warning: lighthouse telemetry submission failed: %v", err)
			if isRetryableError(err) {
				spoolOfflineEvent(e)
			}
		}
	}()
}

// Wait blocks until all pending background telemetry submissions complete.
func Wait() {
	wgMu.Lock()
	wg.Wait()
	wgMu.Unlock()
}
