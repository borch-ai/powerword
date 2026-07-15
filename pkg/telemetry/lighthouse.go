package telemetry

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
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

// ErrEmptySpool is returned when the spool file contains no events to rename or sync.
var ErrEmptySpool = errors.New("spool file is empty")

// TelemetryEvent represents a telemetry record submitted to Lighthouse.
type TelemetryEvent struct {
	ID           string            `json:"id,omitempty"`
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

var (
	fallbackMu      sync.Mutex
	fallbackCounter int64
	randReader      io.Reader = rand.Reader
)

func generateEventID() string {
	b := make([]byte, 16)
	_, err := randReader.Read(b)
	if err != nil {
		fallbackMu.Lock()
		fallbackCounter++
		counter := fallbackCounter
		fallbackMu.Unlock()
		return fmt.Sprintf("fallback-%d-%d-%d", os.Getpid(), time.Now().UnixNano(), counter)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Submit sends the telemetry event to the Lighthouse collector.
func (a *LighthouseAdapter) Submit(ctx context.Context, e TelemetryEvent) error {
	if a.URL == "" {
		return fmt.Errorf("lighthouse URL is required")
	}

	if e.ID == "" {
		e.ID = generateEventID()
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

	for i := range events {
		if events[i].ID == "" {
			events[i].ID = generateEventID()
		}
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

var lockTimeout = 2 * time.Second

// isLockStale reads the lock file and checks if the lock has exceeded staleThreshold.
func isLockStale(lockPath string, staleThreshold time.Duration) bool {
	//nolint:gosec // G304: lockPath is resolved from secure UserHomeDir
	content, err := os.ReadFile(lockPath)
	if err != nil || len(content) == 0 {
		return false
	}
	parts := strings.Split(string(content), ",")
	if len(parts) != 2 {
		return false
	}
	var ts int64
	if _, scanErr := fmt.Sscanf(parts[1], "%d", &ts); scanErr != nil {
		return false
	}
	return time.Since(time.Unix(0, ts)) > staleThreshold
}

// withFileLock executes the given action while holding an exclusive filesystem-level lock
// on the spool file (via a lock file). It retries lock acquisition for up to lockTimeout.
// If the lock file is found to be older than staleThreshold, it is automatically cleaned up.
func withFileLock(spoolPath string, staleThreshold time.Duration, action func() error) error {
	lockPath := spoolPath + ".lock"
	acquired := false

	// Try to acquire the lock using O_EXCL (atomic creation)
	for start := time.Now(); time.Since(start) < lockTimeout; {
		//nolint:gosec // G304: lockPath is resolved from secure UserHomeDir
		lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = fmt.Fprintf(lf, "%d,%d", os.Getpid(), time.Now().UnixNano())
			_ = lf.Close()
			acquired = true
			break
		}

		if !os.IsExist(err) {
			return fmt.Errorf("failed to create lock file: %w", err)
		}

		if isLockStale(lockPath, staleThreshold) {
			_ = os.Remove(lockPath)
			continue
		}

		sleepDur := lockTimeout / 10
		if sleepDur < 1*time.Millisecond {
			sleepDur = 1 * time.Millisecond
		}
		time.Sleep(sleepDur)
	}

	if !acquired {
		return fmt.Errorf("failed to acquire telemetry lock: timeout")
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
	if dirErr := os.MkdirAll(spoolDir, 0700); dirErr != nil {
		log.Printf("Warning: failed to create telemetry spool directory: %v", dirErr)
		return
	}

	spoolPath := filepath.Join(spoolDir, "telemetry_spool.jsonl")

	if e.ID == "" {
		e.ID = generateEventID()
	}

	payload, marshalErr := json.Marshal(e)
	if marshalErr != nil {
		log.Printf("Warning: failed to marshal telemetry event for spooling: %v", marshalErr)
		return
	}

	if err := withFileLock(spoolPath, 10*time.Second, func() error {
		// Enforce max spool size limit of 5 MB to avoid unbounded disk growth
		const maxSpoolBytes = 5 * 1024 * 1024
		var currentSize int64
		if fi, err := os.Stat(spoolPath); err == nil {
			currentSize = fi.Size()
		}
		if currentSize+int64(len(payload)+1) > maxSpoolBytes {
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
	}); err != nil {
		log.Printf("Warning: failed to spool telemetry event offline: %v", err)
	}
}

// parseLine trims and unmarshals a single JSONL line into a TelemetryEvent.
// Returns the event and a boolean indicating if it was parsed successfully (or was empty).
func parseLine(line []byte) (TelemetryEvent, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return TelemetryEvent{}, true
	}
	var ev TelemetryEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		log.Printf("Warning: failed to unmarshal telemetry event from sync file: %v", err)
		return TelemetryEvent{}, false
	}
	return ev, true
}

// readSpooledEvents opens the sync file, reads and parses events line by line.
// Returns the list of parsed events, a boolean indicating if any parse error occurred, and any file I/O error.
func readSpooledEvents(tempSyncPath string) ([]TelemetryEvent, bool, error) {
	//nolint:gosec // G304: path is resolved from secure UserHomeDir
	f, err := os.Open(tempSyncPath)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()

	var events []TelemetryEvent
	hasParseError := false
	scanner := bufio.NewScanner(f)
	// Increase buffer size to handle lines up to the 5 MB max spool limit
	const maxSpoolBytes = 5 * 1024 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxSpoolBytes)

	for scanner.Scan() {
		line := scanner.Bytes()
		ev, ok := parseLine(line)
		if !ok {
			hasParseError = true
			continue
		}
		if ev.ID != "" || ev.Project != "" {
			events = append(events, ev)
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return nil, false, scanErr
	}

	return events, hasParseError, nil
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

func computeTotalPayloadSize(events []TelemetryEvent) int64 {
	var total int64
	for _, ev := range events {
		payload, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		total += int64(len(payload) + 1)
	}
	return total
}

func writeEventsToSpoolFile(f *os.File, events []TelemetryEvent) error {
	for _, ev := range events {
		payload, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		if _, writeErr := f.Write(append(payload, '\n')); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// rollbackSpooledEvents writes events back to the spool file on sync failure.
func rollbackSpooledEvents(spoolPath string, events []TelemetryEvent) error {
	return withFileLock(spoolPath, 10*time.Second, func() error {
		// Enforce max spool size limit of 5 MB
		const maxSpoolBytes = 5 * 1024 * 1024
		var currentSize int64
		if fi, err := os.Stat(spoolPath); err == nil {
			currentSize = fi.Size()
		}
		if currentSize+computeTotalPayloadSize(events) > maxSpoolBytes {
			return fmt.Errorf("telemetry spool file size limit exceeded")
		}

		//nolint:gosec // G304: path is resolved from secure UserHomeDir
		spoolFile, err := os.OpenFile(spoolPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		defer func() { _ = spoolFile.Close() }()

		return writeEventsToSpoolFile(spoolFile, events)
	})
}

// cleanEmptySyncFile deletes the sync file if it is empty, or logs a warning if it contains invalid data.
func cleanEmptySyncFile(syncPath string) {
	if fi, err := os.Stat(syncPath); err == nil && fi.Size() == 0 {
		_ = os.Remove(syncPath)
	} else if err == nil {
		log.Printf("Warning: temporary telemetry sync file is non-empty but contains no valid events; retaining for inspection: %s", syncPath)
	}
}

// handleSyncFailure rolls back unsent events and deletes the sync file on sync error.
func handleSyncFailure(spoolPath, syncPath string, events []TelemetryEvent, sentCount int, syncErr error) {
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
}

// syncFileEvents reads and synchronizes spooled events from the syncPath file.
func syncFileEvents(ctx context.Context, adapter *LighthouseAdapter, syncPath, spoolPath string) error {
	events, hasParseError, readErr := readSpooledEvents(syncPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil
		}
		log.Printf("Warning: error reading temporary telemetry sync file: %v", readErr)
		return nil
	}

	if len(events) == 0 {
		cleanEmptySyncFile(syncPath)
		return nil
	}

	sentCount, syncErr := sendBatchEvents(ctx, adapter, events)
	if syncErr != nil {
		handleSyncFailure(spoolPath, syncPath, events, sentCount, syncErr)
		return nil
	}

	if hasParseError {
		corruptPath := syncPath + ".corrupt"
		log.Printf("Warning: temporary telemetry sync file contained unparseable events; renaming to %s for inspection", corruptPath)
		if renameErr := os.Rename(syncPath, corruptPath); renameErr == nil {
			return nil
		}
	}

	if removeErr := os.Remove(syncPath); removeErr != nil {
		log.Printf("Warning: failed to remove temporary telemetry sync file: %v", removeErr)
	}
	return nil
}

// processSyncFile processes a single temporary sync file: syncs to Lighthouse, deletes on success, rolls back on failure.
func processSyncFile(ctx context.Context, adapter *LighthouseAdapter, syncPath, spoolPath string) {
	// Use lock coordination on the sync file to prevent concurrent processes from processing the same file.
	// If we fail to acquire the lock within lockTimeout, it means another process is already processing it, so we skip.
	_ = withFileLock(syncPath, 1*time.Minute, func() error {
		return syncFileEvents(ctx, adapter, syncPath, spoolPath)
	})
}

// processStrandedSyncFiles scans the spool directory and processes any leftover temporary sync files from previous runs.
func processStrandedSyncFiles(ctx context.Context, adapter *LighthouseAdapter, spoolDir, spoolPath string) {
	files, readDirErr := os.ReadDir(spoolDir)
	if readDirErr != nil {
		return
	}
	for _, entry := range files {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), "telemetry_spool_sync_") && strings.HasSuffix(entry.Name(), ".jsonl") {
			syncPath := filepath.Join(spoolDir, entry.Name())
			processSyncFile(ctx, adapter, syncPath, spoolPath)
		}
	}
}

// renameSpoolUnderLock renames the main spool file to a unique temp sync path under lock coordination.
func renameSpoolUnderLock(spoolPath, tempSyncPath string) error {
	return withFileLock(spoolPath, 10*time.Second, func() error {
		fiInner, statErrInner := os.Stat(spoolPath)
		if statErrInner != nil {
			return statErrInner
		}
		if fiInner.Size() == 0 {
			return ErrEmptySpool
		}
		return os.Rename(spoolPath, tempSyncPath)
	})
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
	processStrandedSyncFiles(ctx, adapter, spoolDir, spoolPath)

	// 2. Process the main spool file if it exists and has content (checked first without locking)
	fi, statErr := os.Stat(spoolPath)
	if statErr != nil {
		if !os.IsNotExist(statErr) {
			log.Printf("Warning: failed to stat telemetry spool file: %v", statErr)
		}
		return
	}
	if fi.Size() == 0 {
		return
	}

	// Atomic Rename strategy under lock
	tempSyncPath := filepath.Join(spoolDir, fmt.Sprintf("telemetry_spool_sync_%d.jsonl", time.Now().UnixNano()))
	if renameErr := renameSpoolUnderLock(spoolPath, tempSyncPath); renameErr != nil {
		if !os.IsNotExist(renameErr) && !errors.Is(renameErr, ErrEmptySpool) {
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

	if e.ID == "" {
		e.ID = generateEventID()
	}

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
