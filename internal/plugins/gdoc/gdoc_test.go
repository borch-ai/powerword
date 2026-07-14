package gdoc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/borch-ai/powerword/pkg/config"
	"golang.org/x/oauth2"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestGDocService_GetClient_MissingCredentials(t *testing.T) {
	// Isolate from host's Application Default Credentials (ADC)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", "/nonexistent-home-directory-for-testing")
	t.Setenv("USERPROFILE", "/nonexistent-home-directory-for-testing")

	cfg := &config.Config{} // no credentials
	svc := NewGDocService(cfg, nil)

	_, _, err := svc.getClient(context.Background())
	if err == nil {
		t.Fatal("expected error due to missing credentials, got nil")
	}
	if !strings.Contains(err.Error(), "no valid authentication found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGDocService_CreateDocument(t *testing.T) {
	cfg := &config.Config{}

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/v1/documents") && !strings.Contains(req.URL.Path, ":batchUpdate") {
				// Documents.Create request
				respBody := `{"documentId": "test-doc-123", "title": "My Title"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/v1/documents/test-doc-123:batchUpdate") {
				// BatchUpdate request
				respBody := `{}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error": "bad request"}`)),
			}, nil
		}),
	}

	svc := NewGDocService(cfg, mockClient)
	docID, viewURL, err := svc.CreateDocument(context.Background(), "My Title", "Initial content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if docID != "test-doc-123" {
		t.Errorf("expected docID test-doc-123, got %s", docID)
	}
	if !strings.Contains(viewURL, "test-doc-123") {
		t.Errorf("expected viewURL to contain test-doc-123, got %s", viewURL)
	}

	// Error case: empty title
	_, _, err = svc.CreateDocument(context.Background(), "", "")
	if err == nil {
		t.Error("expected error for empty title, got nil")
	}
}

func TestGDocService_ReadDocumentText(t *testing.T) {
	cfg := &config.Config{}

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/v1/documents/test-doc-123") {
				// Documents.Get request
				respBody := `{
					"documentId": "test-doc-123",
					"body": {
						"content": [
							{
								"paragraph": {
									"elements": [
										{"textRun": {"content": "Hello World\n"}}
									]
								}
							}
						]
					}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error": "bad request"}`)),
			}, nil
		}),
	}

	svc := NewGDocService(cfg, mockClient)
	text, err := svc.ReadDocumentText(context.Background(), "test-doc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Hello World\n" {
		t.Errorf("expected text 'Hello World\\n', got %q", text)
	}

	// Error case: empty doc ID
	_, err = svc.ReadDocumentText(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty doc ID, got nil")
	}
}

func TestGDocService_UpdateDocumentText(t *testing.T) {
	cfg := &config.Config{}

	var batchUpdateCalled bool
	var batchUpdateBody []byte

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/v1/documents/test-doc-123") {
				// Documents.Get request
				respBody := `{
					"documentId": "test-doc-123",
					"body": {
						"content": [
							{
								"endIndex": 12
							}
						]
					}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/v1/documents/test-doc-123:batchUpdate") {
				batchUpdateCalled = true
				var err error
				batchUpdateBody, err = io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				respBody := `{}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error": "bad request"}`)),
			}, nil
		}),
	}

	svc := NewGDocService(cfg, mockClient)

	// Test Overwrite Mode
	err := svc.UpdateDocumentText(context.Background(), "test-doc-123", "New text", false)
	if err != nil {
		t.Fatalf("unexpected error on overwrite: %v", err)
	}
	if !batchUpdateCalled {
		t.Error("expected batch update to be called")
	}
	if !strings.Contains(string(batchUpdateBody), "deleteContentRange") {
		t.Errorf("expected batch update to contain deleteContentRange, got %s", string(batchUpdateBody))
	}
	if !strings.Contains(string(batchUpdateBody), "New text") {
		t.Errorf("expected batch update to contain 'New text', got %s", string(batchUpdateBody))
	}

	// Test Append Mode
	batchUpdateCalled = false
	err = svc.UpdateDocumentText(context.Background(), "test-doc-123", "Appended text", true)
	if err != nil {
		t.Fatalf("unexpected error on append: %v", err)
	}
	if !batchUpdateCalled {
		t.Error("expected batch update to be called")
	}
	if strings.Contains(string(batchUpdateBody), "deleteContentRange") {
		t.Error("expected batch update NOT to contain deleteContentRange in append mode")
	}
	if !strings.Contains(string(batchUpdateBody), "Appended text") {
		t.Errorf("expected batch update to contain 'Appended text', got %s", string(batchUpdateBody))
	}

	// Error case: empty doc ID
	err = svc.UpdateDocumentText(context.Background(), "", "", false)
	if err == nil {
		t.Error("expected error for empty doc ID, got nil")
	}
}

func TestGDocService_Authorize_ServiceAccount(t *testing.T) {
	tmpDir := t.TempDir()
	saFile := filepath.Join(tmpDir, "sa.json")
	//nolint:gosec // G101: dummy credentials for testing
	saMap := map[string]interface{}{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key_id": "key123",
		"private_key":    "-----BEGIN " + "PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC3\n-----END " + "PRIVATE KEY-----\n",
		"client_email":   "test@test.iam.gserviceaccount.com",
		"client_id":      "client123",
		"auth_uri":       "https://accounts.google.com/o/oauth2/auth",
		"token_uri":      "https://oauth2.googleapis.com/token",
	}
	saBytes, err := json.Marshal(saMap)
	if err != nil {
		t.Fatalf("failed to marshal dummy service account key: %v", err)
	}
	err = os.WriteFile(saFile, saBytes, 0600)
	if err != nil {
		t.Fatalf("failed to write dummy service account key: %v", err)
	}

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				ServiceAccountPath: saFile,
			},
		},
	}

	svc := NewGDocService(cfg, nil)
	docsSvc, driveSvc, err := svc.getClient(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if docsSvc == nil || driveSvc == nil {
		t.Error("expected service clients to be non-nil")
	}
}

func TestGDocService_Authorize_OAuthUserFlow(t *testing.T) {
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "credentials.json")
	tokenFile := filepath.Join(tmpDir, "token.json")

	//nolint:gosec // G101: dummy credentials for testing
	credMap := map[string]interface{}{
		"installed": map[string]interface{}{
			"client_id":     "client123",
			"client_secret": "secret123",
			"auth_uri":      "https://accounts.google.com/o/oauth2/auth",
			"token_uri":     "https://oauth2.googleapis.com/token",
			"redirect_uris": []string{"http://localhost:8080/callback"},
		},
	}
	credBytes, err := json.Marshal(credMap)
	if err != nil {
		t.Fatalf("failed to marshal dummy client credentials: %v", err)
	}
	err = os.WriteFile(credFile, credBytes, 0600)
	if err != nil {
		t.Fatalf("failed to write dummy client credentials: %v", err)
	}

	//nolint:gosec // G101: dummy credentials for testing
	tokenContent := `{"access_token":"access123","token_type":"Bearer","refresh_token":"refresh123","expiry":"2000-01-01T00:00:00Z"}`
	err = os.WriteFile(tokenFile, []byte(tokenContent), 0600)
	if err != nil {
		t.Fatalf("failed to write dummy token: %v", err)
	}

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				CredentialsPath: credFile,
				TokenPath:       tokenFile,
			},
		},
	}

	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost && (strings.Contains(req.URL.Path, "/oauth2/token") || strings.Contains(req.URL.Path, "/token")) {
				respBody := `{"access_token":"new_access123","token_type":"Bearer","expires_in":3600}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error": "bad request"}`)),
			}, nil
		}),
	}

	svc := NewGDocService(cfg, nil)
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, mockClient)
	docsSvc, driveSvc, err := svc.getClient(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if docsSvc == nil || driveSvc == nil {
		t.Error("expected service clients to be non-nil")
	}

	updatedTok, err := readTokenFile(tokenFile)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}
	if updatedTok.AccessToken != "new_access123" {
		t.Errorf("expected access token to be updated to new_access123, got %s", updatedTok.AccessToken)
	}
}

func TestGDocService_ErrorPaths(t *testing.T) {
	cfg := &config.Config{}

	// 1. CreateDocument failing
	mockClientFail := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{"error":"internal server error"}`)),
			}, nil
		}),
	}
	svcFail := NewGDocService(cfg, mockClientFail)
	_, _, err := svcFail.CreateDocument(context.Background(), "Title", "")
	if err == nil {
		t.Error("expected error from CreateDocument on HTTP failure, got nil")
	}

	// 2. CreateDocument populating content failing
	var createCalled bool
	mockClientPopulateFail := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/v1/documents") && !strings.Contains(req.URL.Path, ":batchUpdate") {
				createCalled = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"documentId": "doc123"}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{"error":"batch update failed"}`)),
			}, nil
		}),
	}
	svcPopulateFail := NewGDocService(cfg, mockClientPopulateFail)
	_, _, err = svcPopulateFail.CreateDocument(context.Background(), "Title", "Some initial content")
	if err == nil {
		t.Error("expected error from CreateDocument on BatchUpdate failure, got nil")
	}
	if !createCalled {
		t.Error("expected CreateDocument to be called before BatchUpdate")
	}

	// 3. ReadDocumentText failing
	_, err = svcFail.ReadDocumentText(context.Background(), "doc123")
	if err == nil {
		t.Error("expected error from ReadDocumentText on HTTP failure, got nil")
	}

	// 4. UpdateDocumentText failing (Get document fails)
	err = svcFail.UpdateDocumentText(context.Background(), "doc123", "content", false)
	if err == nil {
		t.Error("expected error from UpdateDocumentText on Get failure, got nil")
	}

	// 5. UpdateDocumentText failing (BatchUpdate fails)
	mockClientBatchFail := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/v1/documents/doc123") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"documentId": "doc123", "body": {"content": []}}`)),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{"error":"batch update failed"}`)),
			}, nil
		}),
	}
	svcBatchFail := NewGDocService(cfg, mockClientBatchFail)
	err = svcBatchFail.UpdateDocumentText(context.Background(), "doc123", "content", false)
	if err == nil {
		t.Error("expected error from UpdateDocumentText on BatchUpdate failure, got nil")
	}
}

func TestGDocService_Authorize_Errors(t *testing.T) {
	// Isolate from host's Application Default Credentials (ADC)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", "/nonexistent")
	t.Setenv("USERPROFILE", "/nonexistent")

	// Service account path configured but file does not exist
	cfg1 := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				ServiceAccountPath: "/nonexistent/sa.json",
			},
		},
	}
	svc1 := NewGDocService(cfg1, nil)
	_, err := svc1.authorize(context.Background())
	if err == nil {
		t.Error("expected authorize to fail on nonexistent sa file, got nil")
	}

	// Credentials path configured but file does not exist
	cfg2 := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				CredentialsPath: "/nonexistent/creds.json",
				TokenPath:       "/nonexistent/token.json",
			},
		},
	}
	svc2 := NewGDocService(cfg2, nil)
	_, err = svc2.authorize(context.Background())
	if err == nil {
		t.Error("expected authorize to fail on nonexistent credentials file, got nil")
	}

	// Service account path configured but file is invalid JSON
	tmpDir := t.TempDir()
	saFile := filepath.Join(tmpDir, "sa.json")
	err = os.WriteFile(saFile, []byte("invalid json"), 0600)
	if err != nil {
		t.Fatalf("failed to write invalid JSON: %v", err)
	}
	cfg3 := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				ServiceAccountPath: saFile,
			},
		},
	}
	svc3 := NewGDocService(cfg3, nil)
	_, err = svc3.authorize(context.Background())
	if err == nil {
		t.Error("expected authorize to fail on invalid service account JSON, got nil")
	}

	// Credentials path configured but file is invalid JSON
	credFile := filepath.Join(tmpDir, "creds.json")
	err = os.WriteFile(credFile, []byte("invalid json"), 0600)
	if err != nil {
		t.Fatalf("failed to write invalid JSON: %v", err)
	}
	cfg4 := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				CredentialsPath: credFile,
				TokenPath:       filepath.Join(tmpDir, "token.json"),
			},
		},
	}
	svc4 := NewGDocService(cfg4, nil)
	_, err = svc4.authorize(context.Background())
	if err == nil {
		t.Error("expected authorize to fail on invalid credentials JSON, got nil")
	}
}

func TestGDocService_ValidateEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"valid localhost", "http://localhost:8080", "http://localhost:8080"},
		{"valid 127.0.0.1", "https://127.0.0.1:9090", "https://127.0.0.1:9090"},
		{"invalid host", "https://google.com/api", ""},
		{"invalid scheme", "ftp://localhost:8080", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateEndpoint(tc.input)
			if got != tc.expected {
				t.Errorf("validateEndpoint(%q) = %q, expected %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestGDocService_NilBody(t *testing.T) {
	// Isolate from host's Application Default Credentials (ADC)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", "/nonexistent")
	t.Setenv("USERPROFILE", "/nonexistent")

	cfg := &config.Config{}
	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"documentId": "doc123", "body": null}`)),
			}, nil
		}),
	}

	svc := NewGDocService(cfg, mockClient)

	// 1. ReadDocumentText fails on nil body
	_, err := svc.ReadDocumentText(context.Background(), "doc123")
	if err == nil || !strings.Contains(err.Error(), "document body is nil") {
		t.Errorf("expected nil body error from ReadDocumentText, got: %v", err)
	}

	// 2. UpdateDocumentText fails on nil body
	err = svc.UpdateDocumentText(context.Background(), "doc123", "new text", false)
	if err == nil || !strings.Contains(err.Error(), "document body is nil") {
		t.Errorf("expected nil body error from UpdateDocumentText, got: %v", err)
	}
}

func TestGDocService_ExpandHomeDir(t *testing.T) {
	// 1. Path starts with ~/
	origHome := os.Getenv("HOME")
	defer t.Setenv("HOME", origHome)
	t.Setenv("HOME", "/custom-home")

	got := expandHomeDir("~/some/path.json")
	expected := filepath.Join("/custom-home", "some/path.json")
	if got != expected {
		t.Errorf("expandHomeDir('~/some/path.json') = %q, expected %q", got, expected)
	}

	// 2. Path does not start with ~/
	got2 := expandHomeDir("/absolute/path.json")
	if got2 != "/absolute/path.json" {
		t.Errorf("expandHomeDir('/absolute/path.json') = %q, expected %q", got2, "/absolute/path.json")
	}
}

func TestNewGDocService_NilConfig(t *testing.T) {
	svc := NewGDocService(nil, nil)
	if svc.cfg == nil {
		t.Error("expected non-nil config in GDocService when passing nil")
	}
}

func TestGDocService_ExtraEdgeCases(t *testing.T) {
	// 1. validateEndpoint with unparseable URL
	// A control character like ASCII 127 in URL will cause url.Parse to return error in older go, or we can use:
	// "http://a b" (spaces are not allowed in URLs parsed by url.Parse)
	got := validateEndpoint("http://a b")
	if got != "" {
		t.Errorf("expected validateEndpoint to fail on invalid URL, got %q", got)
	}

	// 2. readTokenFile with invalid JSON content
	tmpDir := t.TempDir()
	invalidJSONFile := filepath.Join(tmpDir, "invalid_token.json")
	err := os.WriteFile(invalidJSONFile, []byte("{invalid"), 0600)
	if err != nil {
		t.Fatalf("failed to write invalid JSON: %v", err)
	}
	_, err = readTokenFile(invalidJSONFile)
	if err == nil {
		t.Error("expected readTokenFile to fail on invalid JSON, got nil")
	}

	// 3. writeTokenFile directory creation failure
	// We can make directory creation fail by writing a file first, and then trying to write a token file inside it.
	conflictFilePath := filepath.Join(tmpDir, "conflict_file")
	err = os.WriteFile(conflictFilePath, []byte("some content"), 0600)
	if err != nil {
		t.Fatalf("failed to write conflict file: %v", err)
	}
	// Try to write token file as if conflict_file was a directory
	badTokenPath := filepath.Join(conflictFilePath, "token.json")
	err = writeTokenFile(badTokenPath, &oauth2.Token{})
	if err == nil {
		t.Error("expected writeTokenFile to fail when directory creation fails, got nil")
	}

	// Try to open a path we cannot write to (e.g. read-only folder or empty path)
	err = writeTokenFile("", &oauth2.Token{})
	if err == nil {
		t.Error("expected writeTokenFile to fail on empty path, got nil")
	}
}

func TestGDocService_Authorize_OAuthUserFlow_TokenNotChanged(t *testing.T) {
	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, "credentials.json")
	tokenFile := filepath.Join(tmpDir, "token.json")

	//nolint:gosec // G101: dummy credentials for testing
	credMap := map[string]interface{}{
		"installed": map[string]interface{}{
			"client_id":     "client123",
			"client_secret": "secret123",
			"auth_uri":      "https://accounts.google.com/o/oauth2/auth",
			"token_uri":     "https://oauth2.googleapis.com/token",
			"redirect_uris": []string{"http://localhost:8080/callback"},
		},
	}
	credBytes, err := json.Marshal(credMap)
	if err != nil {
		t.Fatalf("failed to marshal dummy client credentials: %v", err)
	}
	err = os.WriteFile(credFile, credBytes, 0600)
	if err != nil {
		t.Fatalf("failed to write dummy client credentials: %v", err)
	}

	// Token expiry in the future so that it does NOT need refresh
	//nolint:gosec // G101: dummy credentials for testing
	tokenContent := `{"access_token":"access123","token_type":"Bearer","refresh_token":"refresh123","expiry":"2100-01-01T00:00:00Z"}`
	err = os.WriteFile(tokenFile, []byte(tokenContent), 0600)
	if err != nil {
		t.Fatalf("failed to write dummy token: %v", err)
	}

	cfg := &config.Config{
		Plugins: config.PluginsConfig{
			GDoc: config.GDocConfig{
				CredentialsPath: credFile,
				TokenPath:       tokenFile,
			},
		},
	}

	// No HTTP requests should be made because the token is not expired
	mockClient := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			t.Error("unexpected HTTP request when token is not expired")
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error": "bad request"}`)),
			}, nil
		}),
	}

	svc := NewGDocService(cfg, nil)
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, mockClient)
	docsSvc, driveSvc, err := svc.getClient(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if docsSvc == nil || driveSvc == nil {
		t.Error("expected service clients to be non-nil")
	}

	// Check that the token file was NOT overwritten (token unchanged)
	//nolint:gosec // G304: test file read is safe
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("failed to read token file: %v", err)
	}
	if string(data) != tokenContent {
		t.Errorf("expected token file content to remain unchanged, got %s", string(data))
	}
}
