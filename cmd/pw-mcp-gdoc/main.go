package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/docs/v1"

	"github.com/borch-ai/powerword/internal/plugins/gdoc"
	"github.com/borch-ai/powerword/pkg/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var authMode bool
	flag.BoolVar(&authMode, "auth", false, "Run OAuth2 interactive authentication flow")
	flag.Parse()

	workspaceRoot := os.Getenv("POWERWORD_WORKSPACE_ROOT")
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get cwd: %w", err)
		}
		workspaceRoot = cwd
	}

	cfg, err := config.LoadFromWorkspace(workspaceRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if authMode {
		return runAuthFlow(cfg)
	}

	srv, err := setupServer(cfg, nil)
	if err != nil {
		return err
	}

	transport := &mcp.StdioTransport{}
	return srv.Run(context.Background(), transport)
}

const (
	createDocSchema = `{
		"type": "object",
		"properties": {
			"title": {
				"type": "string",
				"description": "The title of the new Google Doc."
			},
			"content": {
				"type": "string",
				"description": "Optional initial text content to populate the document."
			}
		},
		"required": ["title"]
	}`

	readDocSchema = `{
		"type": "object",
		"properties": {
			"document_id": {
				"type": "string",
				"description": "The unique ID of the Google Doc to read."
			}
		},
		"required": ["document_id"]
	}`

	updateDocSchema = `{
		"type": "object",
		"properties": {
			"document_id": {
				"type": "string",
				"description": "The unique ID of the Google Doc to update."
			},
			"content": {
				"type": "string",
				"description": "The text content to write or append to the document."
			},
			"append": {
				"type": "boolean",
				"description": "If true, appends the content to the end of the document. If false or omitted, overwrites the document."
			}
		},
		"required": ["document_id", "content"]
	}`
)

func setupServer(cfg *config.Config, svc *gdoc.GDocService) (*mcp.Server, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "pw-mcp-gdoc",
		Version: "0.1.0",
	}, nil)

	gdocService := svc
	if gdocService == nil {
		gdocService = gdoc.NewGDocService(cfg, nil)
	}

	srv.AddTool(&mcp.Tool{
		Name:        "gdoc_create",
		Description: "Creates a new Google Doc with a title and optional initial content.",
		InputSchema: json.RawMessage(createDocSchema),
	}, handleCreate(gdocService))

	srv.AddTool(&mcp.Tool{
		Name:        "gdoc_read",
		Description: "Reads the raw text content of a Google Doc by its ID.",
		InputSchema: json.RawMessage(readDocSchema),
	}, handleRead(gdocService))

	srv.AddTool(&mcp.Tool{
		Name:        "gdoc_update",
		Description: "Overwrites or appends text content inside a Google Doc.",
		InputSchema: json.RawMessage(updateDocSchema),
	}, handleUpdate(gdocService))

	return srv, nil
}

func handleCreate(svc *gdoc.GDocService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.Title == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "title parameter is required"}},
			}, nil
		}

		docID, viewURL, err := svc.CreateDocument(ctx, args.Title, args.Content)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}

		res := struct {
			DocumentID string `json:"document_id"`
			URL        string `json:"url"`
		}{
			DocumentID: docID,
			URL:        viewURL,
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	}
}

func handleRead(svc *gdoc.GDocService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			DocumentID string `json:"document_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.DocumentID == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "document_id parameter is required"}},
			}, nil
		}

		text, err := svc.ReadDocumentText(ctx, args.DocumentID)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: text}},
		}, nil
	}
}

func handleUpdate(svc *gdoc.GDocService) func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			DocumentID string  `json:"document_id"`
			Content    *string `json:"content"`
			Append     bool    `json:"append"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}

		if args.DocumentID == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "document_id parameter is required"}},
			}, nil
		}

		if args.Content == nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "content parameter is required"}},
			}, nil
		}

		err := svc.UpdateDocumentText(ctx, args.DocumentID, *args.Content, args.Append)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}

		msg := "Document updated successfully"
		if args.Append {
			msg = "Document content appended successfully"
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		}, nil
	}
}

func runAuthFlow(cfg *config.Config) error {
	credPath := cfg.Plugins.GDoc.CredentialsPath
	tokenPath := cfg.Plugins.GDoc.TokenPath

	if credPath == "" || tokenPath == "" {
		return fmt.Errorf("credentials_path and token_path must be configured to run authentication flow")
	}

	expandPath := func(path string) string {
		if strings.HasPrefix(path, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				return filepath.Join(home, path[2:])
			}
		}
		return path
	}

	credPathExpanded := expandPath(credPath)
	tokenPathExpanded := expandPath(tokenPath)

	data, err := readConfiguredFile(credPathExpanded)
	if err != nil {
		return fmt.Errorf("failed to read credentials file: %w", err)
	}

	conf, err := google.ConfigFromJSON(data, docs.DocumentsScope)
	if err != nil {
		return fmt.Errorf("failed to parse client configuration: %w", err)
	}

	stateBytes := make([]byte, 16)
	if _, randErr := rand.Read(stateBytes); randErr != nil {
		stateBytes = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	stateToken := hex.EncodeToString(stateBytes)

	codeChan := make(chan string, 1)
	server, redirectURL, err := startCallbackServer(stateToken, codeChan)
	if err != nil {
		return err
	}

	conf.RedirectURL = redirectURL

	authURL := conf.AuthCodeURL(stateToken, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Printf("Go to the following link in your browser to authorize:\n\n%s\n\n", authURL)

	openBrowser(authURL)

	fmt.Printf("Waiting for authorization code from browser callback on %s...\n", redirectURL)
	var code string
	select {
	case code = <-codeChan:
	case <-time.After(5 * time.Minute):
		_ = server.Shutdown(context.Background())
		return errors.New("authentication timed out after 5 minutes")
	}
	_ = server.Shutdown(context.Background())

	token, err := conf.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	err = saveTokenConfigured(tokenPathExpanded, token)
	if err != nil {
		return fmt.Errorf("failed to save token to %s: %w", tokenPathExpanded, err)
	}

	fmt.Printf("Success! Token successfully saved to %s\n", tokenPathExpanded)
	return nil
}

func startCallbackServer(stateToken string, codeChan chan<- string) (*http.Server, string, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state != stateToken {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintln(w, "Authentication failed: state token mismatch.")
			return
		}
		code := r.URL.Query().Get("code")
		if code != "" {
			_, _ = fmt.Fprintln(w, "Authentication successful! You can now close this tab and return to the terminal.")
			select {
			case codeChan <- code:
			default:
			}
		} else {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintln(w, "Authentication failed: code parameter not found.")
		}
	})

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}

	var lc net.ListenConfig
	// Bind to port 0 to dynamically allocate an ephemeral port, preventing port collision
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("failed to start local callback server: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	go func() {
		if srvErr := server.Serve(ln); srvErr != nil && srvErr != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "warning: local callback server error: %v\n", srvErr)
		}
	}()

	return server, redirectURL, nil
}

//nolint:gosec // G204: command name is constant ("open" or "xdg-open") and URL parameter is pre-validated in openBrowser
func defaultBrowserCmd(name, url string) error {
	cmd := exec.CommandContext(context.Background(), name, url)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

//nolint:gosec // G204: command name is constant ("rundll32") and URL parameter is pre-validated in openBrowser
func defaultWindowsBrowserCmd(url string) error {
	cmd := exec.CommandContext(context.Background(), "rundll32", "url.dll,FileProtocolHandler", url)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

var runBrowserCmdHook = defaultBrowserCmd
var runWindowsBrowserCmdHook = defaultWindowsBrowserCmd

func openBrowser(rawURL string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to parse authorization URL: %v\n", err)
		return
	}
	if parsed.Scheme != "https" || parsed.Hostname() != "accounts.google.com" {
		fmt.Fprintf(os.Stderr, "warning: safety check failed; authorization URL host %q is not accounts.google.com. Please open the URL manually.\n", parsed.Hostname())
		return
	}

	var openErr error
	switch runtime.GOOS {
	case "darwin":
		openErr = runBrowserCmdHook("open", rawURL)
	case "windows":
		openErr = runWindowsBrowserCmdHook(rawURL)
	case "linux":
		openErr = runBrowserCmdHook("xdg-open", rawURL)
	default:
		fmt.Fprintf(os.Stderr, "warning: automatic browser opening is not supported on OS %q. Please open the link manually.\n", runtime.GOOS)
		return
	}

	if openErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to automatically open browser: %v. Please open the link manually.\n", openErr)
	}
}

// readConfiguredFile reads files specified by the user's configuration.
//
//nolint:gosec // G304, G703: path is pre-validated and loaded from trusted user configuration
func readConfiguredFile(path string) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// saveTokenConfigured writes token cache file.
//
//nolint:gosec // G304, G703, G117: path is pre-validated, directory is user-restricted, and token caching is necessary
func saveTokenConfigured(path string, token *oauth2.Token) error {
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
