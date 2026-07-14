package gdoc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/borch-ai/powerword/pkg/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// GDocService provides operations on Google Docs.
type GDocService struct {
	cfg        *config.Config
	mu         sync.Mutex
	httpClient *http.Client
}

// NewGDocService creates a new instance of GDocService.
func NewGDocService(cfg *config.Config, httpClient *http.Client) *GDocService {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &GDocService{
		cfg:        cfg,
		httpClient: httpClient,
	}
}

// getClient instantiates and returns Docs API client.
func (s *GDocService) getClient(ctx context.Context) (*docs.Service, error) {
	s.mu.Lock()
	if s.httpClient == nil {
		c, err := s.authorize(ctx)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		s.httpClient = c
	}
	client := s.httpClient
	s.mu.Unlock()

	opts := []option.ClientOption{option.WithHTTPClient(client)}
	if ep := validateEndpoint(os.Getenv("POWERWORD_GDOC_API_ENDPOINT")); ep != "" {
		opts = append(opts, option.WithEndpoint(ep))
	}

	docsSvc, err := docs.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docs service: %w", err)
	}

	return docsSvc, nil
}

// authorize resolves and configures the HTTP client based on the available credentials.
func (s *GDocService) authorize(ctx context.Context) (*http.Client, error) {
	// 1. Check Service Account credentials path
	saPath := expandHomeDir(s.cfg.Plugins.GDoc.ServiceAccountPath)
	if saPath != "" {
		client, err := s.authorizeServiceAccount(ctx, saPath)
		if err != nil {
			return nil, fmt.Errorf("service account auth configured but failed: %w", err)
		}
		return client, nil
	}

	// 2. Check Credentials path + Token path (OAuth2 User authentication)
	credPath := expandHomeDir(s.cfg.Plugins.GDoc.CredentialsPath)
	tokenPath := expandHomeDir(s.cfg.Plugins.GDoc.TokenPath)

	if credPath != "" || tokenPath != "" {
		if credPath == "" || tokenPath == "" {
			return nil, errors.New("both credentials_path and token_path must be configured for OAuth user flow")
		}
		client, err := s.authorizeUserOAuth(ctx, credPath, tokenPath)
		if err != nil {
			return nil, fmt.Errorf("OAuth user flow configured but failed: %w", err)
		}
		return client, nil
	}

	// 3. Fallback to Application Default Credentials (ADC)
	creds, err := google.FindDefaultCredentials(ctx, docs.DocumentsScope)
	if err == nil {
		return oauth2.NewClient(ctx, creds.TokenSource), nil
	}

	return nil, errors.New("no valid authentication found. Configure service_account_path, credentials_path + token_path, or setup Application Default Credentials")
}

func (s *GDocService) authorizeServiceAccount(ctx context.Context, saPath string) (*http.Client, error) {
	data, err := readCredFile(saPath)
	if err != nil {
		return nil, err
	}
	creds, err := google.CredentialsFromJSONWithType(ctx, data, google.ServiceAccount, docs.DocumentsScope)
	if err != nil {
		return nil, fmt.Errorf("failed to parse service account JSON: %w", err)
	}
	// Eagerly fetch a token to validate that credentials work
	if _, err := creds.TokenSource.Token(); err != nil {
		return nil, fmt.Errorf("service account credentials failed to retrieve token: %w", err)
	}
	return oauth2.NewClient(ctx, creds.TokenSource), nil
}

func (s *GDocService) authorizeUserOAuth(ctx context.Context, credPath, tokenPath string) (*http.Client, error) {
	data, err := readCredFile(credPath)
	if err != nil {
		return nil, err
	}
	conf, err := google.ConfigFromJSON(data, docs.DocumentsScope)
	if err != nil {
		return nil, fmt.Errorf("failed to parse client configuration: %w", err)
	}

	token, err := readTokenFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load token: %w", err)
	}

	tokSource := conf.TokenSource(ctx, token)
	updatedTok, err := tokSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve valid token: %w", err)
	}

	if updatedTok.RefreshToken == "" {
		updatedTok.RefreshToken = token.RefreshToken
	}

	tokenChanged := updatedTok.AccessToken != token.AccessToken ||
		updatedTok.RefreshToken != token.RefreshToken ||
		!updatedTok.Expiry.Equal(token.Expiry)

	if tokenChanged {
		if err := writeTokenFile(tokenPath, updatedTok); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to cache updated OAuth token: %v\n", err)
		}
	}

	return oauth2.NewClient(ctx, tokSource), nil
}

func validateEndpoint(envURL string) string {
	if envURL == "" {
		return ""
	}
	parsed, err := url.Parse(envURL)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	scheme := parsed.Scheme
	if (scheme == "http" || scheme == "https") && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
		return envURL
	}
	return ""
}

// CreateDocument creates a new Google Doc and returns the document ID and edit URL.
func (s *GDocService) CreateDocument(ctx context.Context, title string, content string) (string, string, error) {
	if title == "" {
		return "", "", errors.New("document title is required")
	}

	docsSvc, err := s.getClient(ctx)
	if err != nil {
		return "", "", err
	}

	doc, err := docsSvc.Documents.Create(&docs.Document{Title: title}).Do()
	if err != nil {
		return "", "", fmt.Errorf("failed to create document resource: %w", err)
	}

	if content != "" {
		reqs := []*docs.Request{
			{
				InsertText: &docs.InsertTextRequest{
					Location: &docs.Location{Index: 1},
					Text:     content,
				},
			},
		}
		_, err = docsSvc.Documents.BatchUpdate(doc.DocumentId, &docs.BatchUpdateDocumentRequest{
			Requests: reqs,
		}).Do()
		if err != nil {
			return "", "", fmt.Errorf("failed to insert initial document content: %w", err)
		}
	}

	editURL := fmt.Sprintf("https://docs.google.com/document/d/%s/edit", doc.DocumentId)
	return doc.DocumentId, editURL, nil
}

// ReadDocumentText reads and returns all text from a Google Doc.
func (s *GDocService) ReadDocumentText(ctx context.Context, docID string) (string, error) {
	if docID == "" {
		return "", errors.New("document_id is required")
	}

	docsSvc, err := s.getClient(ctx)
	if err != nil {
		return "", err
	}

	doc, err := docsSvc.Documents.Get(docID).Do()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve document content: %w", err)
	}

	if doc.Body == nil {
		return "", errors.New("document body is nil")
	}

	var sb strings.Builder
	for _, el := range doc.Body.Content {
		if el.Paragraph != nil {
			for _, run := range el.Paragraph.Elements {
				if run.TextRun != nil {
					sb.WriteString(run.TextRun.Content)
				}
			}
		}
	}

	return sb.String(), nil
}

// UpdateDocumentText overwrites or appends text content inside a Google Doc.
func (s *GDocService) UpdateDocumentText(ctx context.Context, docID string, content string, appendMode bool) error {
	if docID == "" {
		return errors.New("document_id is required")
	}

	docsSvc, err := s.getClient(ctx)
	if err != nil {
		return err
	}

	doc, err := docsSvc.Documents.Get(docID).Do()
	if err != nil {
		return fmt.Errorf("failed to get document prior to update: %w", err)
	}

	if doc.Body == nil {
		return errors.New("document body is nil")
	}

	reqs := []*docs.Request{}
	var endIndex int64 = 1
	if len(doc.Body.Content) > 0 {
		endIndex = doc.Body.Content[len(doc.Body.Content)-1].EndIndex
	}

	if appendMode {
		insertIndex := endIndex - 1
		if insertIndex < 1 {
			insertIndex = 1
		}
		reqs = append(reqs, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertIndex},
				Text:     content,
			},
		})
	} else {
		if endIndex > 2 {
			reqs = append(reqs, &docs.Request{
				DeleteContentRange: &docs.DeleteContentRangeRequest{
					Range: &docs.Range{
						StartIndex: 1,
						EndIndex:   endIndex - 1,
					},
				},
			})
		}
		reqs = append(reqs, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: 1},
				Text:     content,
			},
		})
	}

	_, err = docsSvc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: reqs,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to execute document batch update: %w", err)
	}

	return nil
}

// readCredFile encapsulates reading credentials with validated configuration paths.
//
//nolint:gosec // G304: path is validated to exist and loaded from configuration
func readCredFile(path string) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// readTokenFile encapsulates reading token cache files from validated paths.
//
//nolint:gosec // G304: path is validated and loaded from configuration
func readTokenFile(path string) (*oauth2.Token, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	tok := &oauth2.Token{}
	err = json.NewDecoder(file).Decode(tok)
	return tok, err
}

// writeTokenFile encapsulates writing updated token files.
//
//nolint:gosec // G304, G117: path is validated, directory is restricted, and token marshalling is required
func writeTokenFile(path string, token *oauth2.Token) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	return json.NewEncoder(file).Encode(token)
}

func expandHomeDir(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
