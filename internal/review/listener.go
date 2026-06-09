package review

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"powerword/internal/config"
)

// StartWebhookListener starts an HTTP server listening for GitHub webhooks.
func StartWebhookListener(ctx context.Context, cfg *config.Config, port int) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}
	if cfg.WebhookSecret == "" {
		return fmt.Errorf("WebhookSecret must be configured")
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	mux := http.NewServeMux()

	// Initialize MCP Server and HTTP handlers
	mcpSrv := NewMCPServer()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		handleWebhook(w, r, cfg.WebhookSecret, mcpSrv)
	})

	// Add a simple healthcheck
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() { //nolint:gosec // intentional background context for shutdown
		<-ctx.Done()
		log.Println("Shutting down webhook listener...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("webhook listener shutdown error: %v", err)
		}
	}()

	log.Printf("Starting webhook listener on %s\n", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("webhook listener error: %w", err)
	}

	return nil
}

func handleWebhook(w http.ResponseWriter, r *http.Request, secret string, mcpSrv *WebhookMCPServer) {
	defer func() {
		_ = r.Body.Close()
	}()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024+1))
	if err != nil {
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}
	if len(body) > 10*1024*1024 {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if secret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, signature, secret) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	event := r.Header.Get("X-GitHub-Event")
	switch event {
	case "issues", "issue_comment", "pull_request_review_comment":
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		//nolint:gosec // event is a known header, but silencing taint analysis
		log.Printf("Received valid %q webhook event\n", event)
		mcpSrv.HandleGitHubEvent(event, payload)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Event received"))
	default:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Event ignored"))
	}
}

func verifySignature(payloadBody []byte, signatureHeader string, secret string) bool {
	if signatureHeader == "" || !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	actualMAC, err := hex.DecodeString(signatureHeader[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payloadBody)
	expectedMAC := mac.Sum(nil)
	return hmac.Equal(actualMAC, expectedMAC)
}
