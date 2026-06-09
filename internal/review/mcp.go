package review

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WebhookMCPServer handles exposing GitHub events via MCP.
type WebhookMCPServer struct {
	srv      *mcp.Server
	issues   map[string]map[string]interface{}
	comments map[string]map[string]interface{}
	mu       sync.RWMutex
}

// NewMCPServer initializes the MCP server for webhook events.
func NewMCPServer() *WebhookMCPServer {
	impl := &mcp.Implementation{
		Name:    "powerword-webhook",
		Version: "1.0.0",
	}

	ws := &WebhookMCPServer{
		srv:      mcp.NewServer(impl, nil),
		issues:   make(map[string]map[string]interface{}),
		comments: make(map[string]map[string]interface{}),
	}

	ws.srv.AddResource(&mcp.Resource{
		URI:         "github://issues",
		Name:        "GitHub Issues",
		Description: "Active issues being tracked",
		MIMEType:    "application/json",
	}, ws.handleReadIssues)

	ws.srv.AddResource(&mcp.Resource{
		URI:         "github://comments",
		Name:        "GitHub Comments",
		Description: "Active comments being tracked",
		MIMEType:    "application/json",
	}, ws.handleReadComments)

	return ws
}

// Server returns the underlying MCP server so it can be connected to transports.
func (ws *WebhookMCPServer) Server() *mcp.Server {
	return ws.srv
}

func (ws *WebhookMCPServer) handleReadIssues(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	data, err := json.Marshal(ws.issues)
	if err != nil {
		return nil, fmt.Errorf("failed to encode issues: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "github://issues",
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

func (ws *WebhookMCPServer) handleReadComments(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	data, err := json.Marshal(ws.comments)
	if err != nil {
		return nil, fmt.Errorf("failed to encode comments: %w", err)
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      "github://comments",
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

// HandleGitHubEvent updates the in-memory MCP state with the event data.
func (ws *WebhookMCPServer) HandleGitHubEvent(eventType string, payload map[string]interface{}) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Extract basic information depending on event
	switch eventType {
	case "issues":
		if issue, ok := payload["issue"].(map[string]interface{}); ok {
			if idStr := extractID(issue["number"]); idStr != "" {
				ws.issues[idStr] = issue
			}
		}
	case "issue_comment":
		if comment, ok := payload["comment"].(map[string]interface{}); ok {
			if idStr := extractID(comment["id"]); idStr != "" {
				ws.comments[idStr] = comment
			}
		}
	case "pull_request_review_comment":
		if comment, ok := payload["comment"].(map[string]interface{}); ok {
			if idStr := extractID(comment["id"]); idStr != "" {
				ws.comments[idStr] = comment
			}
		}
	}

	//nolint:gosec // silencing taint analysis for known safe event types
	log.Printf("MCP Broker: Updated state with %q event", eventType)
}

func extractID(v interface{}) string {
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return ""
	case int:
		return fmt.Sprintf("%d", val)
	case string:
		return val
	}
	return ""
}
