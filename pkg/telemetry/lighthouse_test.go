package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
	if !containsString(err.Error(), "unexpected status code: 401") {
		t.Errorf("expected 'unexpected status code: 401' error, got %v", err)
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

	originalURL := os.Getenv("LIGHTHOUSE_URL")
	originalKey := os.Getenv("LIGHTHOUSE_API_KEY")
	defer func() {
		setEnvHelper(t, "LIGHTHOUSE_URL", originalURL)
		setEnvHelper(t, "LIGHTHOUSE_API_KEY", originalKey)
	}()

	setEnvHelper(t, "LIGHTHOUSE_URL", server.URL)
	setEnvHelper(t, "LIGHTHOUSE_API_KEY", "secret")

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
	originalURL := os.Getenv("LIGHTHOUSE_URL")
	defer func() {
		setEnvHelper(t, "LIGHTHOUSE_URL", originalURL)
	}()

	setEnvHelper(t, "LIGHTHOUSE_URL", "")

	// This should return immediately and not trigger any wait
	SubmitToLighthouse(TelemetryEvent{Project: "test-noop"})
	Wait()
}

func setEnvHelper(t *testing.T, key, value string) {
	t.Helper()
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set env var %s: %v", key, err)
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
