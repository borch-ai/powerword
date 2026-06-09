package review

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"powerword/internal/config"
)

func TestHandleWebhook_ValidSignature(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"issue": {"number": 123}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+expectedMAC)
	req.Header.Set("X-GitHub-Event", "issues")

	rr := httptest.NewRecorder()
	mcpSrv := NewMCPServer()

	handleWebhook(rr, req, secret, mcpSrv)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	mcpSrv.mu.RLock()
	if _, ok := mcpSrv.issues["123"]; !ok {
		t.Errorf("issue not added to mcp server")
	}
	mcpSrv.mu.RUnlock()
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"issue": {"number": 123}}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	req.Header.Set("X-GitHub-Event", "issues")

	rr := httptest.NewRecorder()
	mcpSrv := NewMCPServer()

	handleWebhook(rr, req, secret, mcpSrv)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
	}
}

func TestHandleWebhook_IgnoreEvent(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"push": {}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+expectedMAC)
	req.Header.Set("X-GitHub-Event", "push")

	rr := httptest.NewRecorder()
	mcpSrv := NewMCPServer()

	handleWebhook(rr, req, secret, mcpSrv)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	if rr.Body.String() != "Event ignored" {
		t.Errorf("expected Event ignored, got %v", rr.Body.String())
	}
}

func TestStartWebhookListener_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := &config.Config{WebhookSecret: "test", WebhookPort: 0}
	
	errCh := make(chan error, 1)
	go func() {
		errCh <- StartWebhookListener(ctx, cfg, 0)
	}()
	
	// Cancel immediately to trigger shutdown
	cancel()
	err := <-errCh
	if err != nil {
		t.Errorf("expected nil error on graceful shutdown, got %v", err)
	}
}

func TestHandleWebhook_IssueComment(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"comment": {"id": 123}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+expectedMAC)
	req.Header.Set("X-GitHub-Event", "issue_comment")

	rr := httptest.NewRecorder()
	mcpSrv := NewMCPServer()

	handleWebhook(rr, req, secret, mcpSrv)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestHandleWebhook_PullRequestReviewComment(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"comment": {"id": 123}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+expectedMAC)
	req.Header.Set("X-GitHub-Event", "pull_request_review_comment")

	rr := httptest.NewRecorder()
	mcpSrv := NewMCPServer()

	handleWebhook(rr, req, secret, mcpSrv)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestHandleWebhook_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/webhook", nil)
	rr := httptest.NewRecorder()
	mcpSrv := NewMCPServer()

	handleWebhook(rr, req, "", mcpSrv)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusMethodNotAllowed)
	}
}

func TestHandleWebhook_InvalidJSON(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/webhook", bytes.NewReader([]byte(`{invalid}`)))
	req.Header.Set("X-GitHub-Event", "issues")
	rr := httptest.NewRecorder()
	mcpSrv := NewMCPServer()

	handleWebhook(rr, req, "", mcpSrv)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}
