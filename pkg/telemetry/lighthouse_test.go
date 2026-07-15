package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLighthouseAdapter_Submit_Success(t *testing.T) {
	var (
		mu          sync.Mutex
		receivedReq *http.Request
		receivedVal TelemetryEvent
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedReq = r

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := json.Unmarshal(body, &receivedVal); err != nil {
			t.Errorf("failed to unmarshal JSON: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := &LighthouseAdapter{
		URL:    server.URL,
		APIKey: "test-api-key",
		Client: server.Client(),
	}

	event := TelemetryEvent{
		Project:      "powerword",
		Command:      "run",
		Stage:        "react_loop",
		DurationMs:   123,
		CostUSD:      0.005,
		TokensIn:     100,
		TokensOut:    200,
		TokensCached: 50,
		ErrorMsg:     "none",
		Meta:         map[string]string{"foo": "bar"},
	}

	err := adapter.Submit(context.Background(), event)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedReq == nil {
		t.Fatal("expected server to receive a request")
	}

	if receivedReq.Method != http.MethodPost {
		t.Errorf("expected POST request, got %s", receivedReq.Method)
	}

	if receivedReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedReq.Header.Get("Content-Type"))
	}

	if receivedReq.Header.Get("Authorization") != "Bearer test-api-key" {
		t.Errorf("expected Authorization Bearer test-api-key, got %s", receivedReq.Header.Get("Authorization"))
	}

	if receivedVal.Project != event.Project ||
		receivedVal.Command != event.Command ||
		receivedVal.Stage != event.Stage ||
		receivedVal.DurationMs != event.DurationMs ||
		receivedVal.CostUSD != event.CostUSD ||
		receivedVal.TokensIn != event.TokensIn ||
		receivedVal.TokensOut != event.TokensOut ||
		receivedVal.TokensCached != event.TokensCached ||
		receivedVal.ErrorMsg != event.ErrorMsg ||
		receivedVal.Meta["foo"] != event.Meta["foo"] {
		t.Errorf("received payload does not match sent payload: %+v", receivedVal)
	}
}

func TestLighthouseAdapter_Submit_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	adapter := &LighthouseAdapter{
		URL:    server.URL,
		Client: server.Client(),
	}

	err := adapter.Submit(context.Background(), TelemetryEvent{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsString(err.Error(), "unexpected status code 401") {
		t.Errorf("expected 'unexpected status code 401' error, got %v", err)
	}
}

func TestLighthouseAdapter_Submit_ErrorWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized access token"))
	}))
	defer server.Close()

	adapter := &LighthouseAdapter{
		URL:    server.URL,
		Client: server.Client(),
	}

	err := adapter.Submit(context.Background(), TelemetryEvent{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsString(err.Error(), "unauthorized access token") {
		t.Errorf("expected 'unauthorized access token' in error, got %v", err)
	}
}

func TestLighthouseAdapter_Submit_NoURL(t *testing.T) {
	adapter := &LighthouseAdapter{}
	err := adapter.Submit(context.Background(), TelemetryEvent{})
	if err == nil {
		t.Fatal("expected error when URL is empty, got nil")
	}
}

func TestSubmitToLighthouse_Success(t *testing.T) {
	var (
		mu       sync.Mutex
		received bool
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv("LIGHTHOUSE_URL", server.URL)
	t.Setenv("LIGHTHOUSE_API_KEY", "secret")

	// We override the default Client by setting custom HTTP client or just let it use default.
	// Since SubmitToLighthouse builds its own adapter with standard http.Client{}, we don't pass
	// server.Client() directly. Standard http.Client{} can resolve httptest server URLs locally.

	SubmitToLighthouse(TelemetryEvent{Project: "test-bg"})
	Wait()

	mu.Lock()
	defer mu.Unlock()
	if !received {
		t.Error("expected mock server to receive request via background submission")
	}
}

func TestSubmitToLighthouse_NoURL(t *testing.T) {
	t.Setenv("LIGHTHOUSE_URL", "")

	// This should return immediately and not trigger any wait
	SubmitToLighthouse(TelemetryEvent{Project: "test-noop"})
	Wait()
}

func TestLighthouseAdapter_SubmitBatch_Success(t *testing.T) {
	var (
		mu          sync.Mutex
		receivedReq *http.Request
		receivedVal []TelemetryEvent
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		receivedReq = r

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := json.Unmarshal(body, &receivedVal); err != nil {
			t.Errorf("failed to unmarshal JSON: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := &LighthouseAdapter{
		URL:    server.URL,
		APIKey: "batch-key",
		Client: server.Client(),
	}

	events := []TelemetryEvent{
		{Project: "p1", Command: "run"},
		{Project: "p2", Command: "audit"},
	}

	err := adapter.SubmitBatch(context.Background(), events)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedReq == nil {
		t.Fatal("expected server to receive a request")
	}

	if receivedReq.URL.Path != "/api/telemetry/batch" {
		t.Errorf("expected path /api/telemetry/batch, got %s", receivedReq.URL.Path)
	}

	if len(receivedVal) != 2 {
		t.Errorf("expected 2 events, got %d", len(receivedVal))
	}
}

func TestLighthouseAdapter_SubmitBatch_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := &LighthouseAdapter{
		URL:    server.URL,
		Client: server.Client(),
	}

	err := adapter.SubmitBatch(context.Background(), []TelemetryEvent{{Project: "err"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTelemetrySpoolAndSync(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Spool 2 events
	event1 := TelemetryEvent{Project: "spool-1", Command: "test"}
	event2 := TelemetryEvent{Project: "spool-2", Command: "test"}

	spoolOfflineEvent(event1)
	spoolOfflineEvent(event2)

	// Verify spool file contains events
	spoolPath := filepath.Join(tempHome, ".local", "share", "powerword", "telemetry_spool.jsonl")
	if _, err := os.Stat(spoolPath); err != nil {
		t.Fatalf("expected spool file to exist, got error: %v", err)
	}

	// Setup sync mock server
	var (
		mu          sync.Mutex
		receivedVal []TelemetryEvent
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var batch []TelemetryEvent
		if err := json.Unmarshal(body, &batch); err == nil {
			receivedVal = append(receivedVal, batch...)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Sync events
	SyncSpooledEvents(context.Background(), server.URL, "")

	// Verify spool file was deleted (synced successfully)
	if _, err := os.Stat(spoolPath); !os.IsNotExist(err) {
		t.Error("expected spool file to be deleted after sync success")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(receivedVal) != 2 {
		t.Errorf("expected 2 received events, got %d", len(receivedVal))
	}
}

func TestTelemetrySync_FailureRollback(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	event := TelemetryEvent{Project: "rollback-me"}
	spoolOfflineEvent(event)

	spoolPath := filepath.Join(tempHome, ".local", "share", "powerword", "telemetry_spool.jsonl")

	// Mock server that returns error to trigger rollback
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Sync should fail and spool events back
	SyncSpooledEvents(context.Background(), server.URL, "")

	// Spool file should still exist and contain the event
	if _, err := os.Stat(spoolPath); err != nil {
		t.Fatalf("expected spool file to exist after rollback, got: %v", err)
	}

	//nolint:gosec // G304: test file path pre-validated
	content, err := os.ReadFile(spoolPath)
	if err != nil {
		t.Fatalf("failed to read spool file: %v", err)
	}

	var restored TelemetryEvent
	if err := json.Unmarshal(bytes.TrimSpace(content), &restored); err != nil {
		t.Fatalf("failed to parse restored event: %v", err)
	}

	if restored.Project != "rollback-me" {
		t.Errorf("expected Project 'rollback-me', got %s", restored.Project)
	}
}

func TestTelemetrySpool_MkdirError(t *testing.T) {
	tempHome := t.TempDir()
	blockedPath := filepath.Join(tempHome, ".local")
	if err := os.WriteFile(blockedPath, []byte("blocked"), 0600); err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}

	t.Setenv("HOME", tempHome)

	spoolOfflineEvent(TelemetryEvent{Project: "test"})
}

func TestTelemetrySpool_OpenFileError(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	spoolDir := filepath.Join(tempHome, ".local", "share", "powerword")
	if err := os.MkdirAll(spoolDir, 0750); err != nil {
		t.Fatalf("failed to create spool dir: %v", err)
	}

	spoolPath := filepath.Join(spoolDir, "telemetry_spool.jsonl")
	if err := os.Mkdir(spoolPath, 0750); err != nil {
		t.Fatalf("failed to create blocking dir: %v", err)
	}

	spoolOfflineEvent(TelemetryEvent{Project: "test"})
}

func TestTelemetrySync_InvalidJSON(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	spoolDir := filepath.Join(tempHome, ".local", "share", "powerword")
	if err := os.MkdirAll(spoolDir, 0750); err != nil {
		t.Fatalf("failed to create spool dir: %v", err)
	}

	spoolPath := filepath.Join(spoolDir, "telemetry_spool.jsonl")
	content := []byte("{\n{\"project\":\"valid\"}\n")
	if err := os.WriteFile(spoolPath, content, 0600); err != nil {
		t.Fatalf("failed to write spool file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	SyncSpooledEvents(context.Background(), server.URL, "")

	if _, err := os.Stat(spoolPath); !os.IsNotExist(err) {
		t.Error("expected spool file to be deleted")
	}
}

func TestTelemetrySync_RenameError(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	spoolDir := filepath.Join(tempHome, ".local", "share", "powerword")
	if err := os.MkdirAll(spoolDir, 0750); err != nil {
		t.Fatalf("failed to create spool dir: %v", err)
	}

	spoolPath := filepath.Join(spoolDir, "telemetry_spool.jsonl")
	if err := os.WriteFile(spoolPath, []byte("{\n"), 0600); err != nil {
		t.Fatalf("failed to write spool: %v", err)
	}

	//nolint:gosec // G302: directory permissions modified for testing read-only error handling
	if err := os.Chmod(spoolDir, 0500); err != nil {
		t.Fatalf("failed to chmod dir: %v", err)
	}
	defer func() {
		//nolint:gosec // G302: directory permissions restored
		_ = os.Chmod(spoolDir, 0750)
	}()

	SyncSpooledEvents(context.Background(), "http://localhost", "")
}

func TestTelemetrySync_RollbackError(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	event := TelemetryEvent{Project: "rollback-fail"}
	spoolOfflineEvent(event)

	spoolPath := filepath.Join(tempHome, ".local", "share", "powerword", "telemetry_spool.jsonl")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = os.Mkdir(spoolPath, 0750)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	SyncSpooledEvents(context.Background(), server.URL, "")

	_ = os.Remove(spoolPath)
}

func TestTelemetrySync_OpenError(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	spoolDir := filepath.Join(tempHome, ".local", "share", "powerword")
	if err := os.MkdirAll(spoolDir, 0750); err != nil {
		t.Fatalf("failed to create spool dir: %v", err)
	}

	spoolPath := filepath.Join(spoolDir, "telemetry_spool.jsonl")
	if err := os.WriteFile(spoolPath, []byte("{\n"), 0000); err != nil {
		t.Fatalf("failed to write spool: %v", err)
	}

	SyncSpooledEvents(context.Background(), "http://localhost", "")
}

func TestTelemetrySync_LargeBatch(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	for i := 0; i < 105; i++ {
		spoolOfflineEvent(TelemetryEvent{Project: fmt.Sprintf("event-%d", i)})
	}

	var (
		mu         sync.Mutex
		callCount  int
		totalEvent int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		callCount++

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var batch []TelemetryEvent
		if err := json.Unmarshal(body, &batch); err == nil {
			totalEvent += len(batch)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	SyncSpooledEvents(context.Background(), server.URL, "")

	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Errorf("expected 2 batch calls, got %d", callCount)
	}
	if totalEvent != 105 {
		t.Errorf("expected 105 total events, got %d", totalEvent)
	}
}

func TestLighthouseAdapter_SubmitBatch_ErrorWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error payload"))
	}))
	defer server.Close()

	adapter := &LighthouseAdapter{
		URL:    server.URL,
		Client: server.Client(),
	}

	err := adapter.SubmitBatch(context.Background(), []TelemetryEvent{{Project: "err"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsString(err.Error(), "server error payload") {
		t.Errorf("expected error payload in error message, got %v", err)
	}
}

func TestTelemetrySync_StatPermissionError(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	spoolDir := filepath.Join(tempHome, ".local", "share", "powerword")
	if err := os.MkdirAll(spoolDir, 0750); err != nil {
		t.Fatalf("failed to create spool dir: %v", err)
	}

	spoolPath := filepath.Join(spoolDir, "telemetry_spool.jsonl")
	if err := os.WriteFile(spoolPath, []byte("{\n"), 0600); err != nil {
		t.Fatalf("failed to write spool: %v", err)
	}

	//nolint:gosec // G302: directory permissions modified for testing read-only error handling
	if err := os.Chmod(spoolDir, 0000); err != nil {
		t.Fatalf("failed to chmod dir: %v", err)
	}
	defer func() {
		//nolint:gosec // G302: directory permissions restored
		_ = os.Chmod(spoolDir, 0750)
	}()

	SyncSpooledEvents(context.Background(), "http://localhost", "")
}

func TestTelemetrySync_ContextCancelled(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	event := TelemetryEvent{Project: "cancelled"}
	spoolOfflineEvent(event)

	spoolPath := filepath.Join(tempHome, ".local", "share", "powerword", "telemetry_spool.jsonl")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	SyncSpooledEvents(ctx, "http://localhost", "")

	if _, err := os.Stat(spoolPath); err != nil {
		t.Errorf("expected spool file to still exist after context cancel, got: %v", err)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s[0:len(substr)] == substr || s[len(s)-len(substr):] == substr || stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
