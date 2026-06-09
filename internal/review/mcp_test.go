package review

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWebhookMCPServer_HandleGitHubEvent(t *testing.T) {
	srv := NewMCPServer()

	// Test issues event
	issuePayload := map[string]interface{}{
		"issue": map[string]interface{}{
			"number": 42.0, // JSON decoding maps numbers to float64
			"title":  "Test Issue",
		},
	}
	srv.HandleGitHubEvent("issues", issuePayload)

	srv.mu.RLock()
	issue, ok := srv.issues["42"]
	srv.mu.RUnlock()

	if !ok {
		t.Fatalf("expected issue 42 to be added")
	}
	if issue["title"] != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got %v", issue["title"])
	}

	// Test issue_comment event
	commentPayload := map[string]interface{}{
		"comment": map[string]interface{}{
			"id":   123.0,
			"body": "Test comment",
		},
	}
	srv.HandleGitHubEvent("issue_comment", commentPayload)

	srv.mu.RLock()
	comment, ok := srv.comments["123"]
	srv.mu.RUnlock()

	if !ok {
		t.Fatalf("expected comment 123 to be added")
	}
	if comment["body"] != "Test comment" {
		t.Errorf("expected body 'Test comment', got %v", comment["body"])
	}

	// Test handleReadIssues
	req := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "github://issues",
		},
	}
	res, err := srv.handleReadIssues(context.Background(), req)
	if err != nil {
		t.Fatalf("handleReadIssues failed: %v", err)
	}

	if len(res.Contents) == 0 {
		t.Fatalf("expected resource contents")
	}

	txtContent := res.Contents[0]
	if txtContent.Text == "" {
		t.Fatalf("expected text content")
	}

	var issuesData map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(txtContent.Text), &issuesData); err != nil {
		t.Fatalf("failed to parse issues json: %v", err)
	}

	if _, ok := issuesData["42"]; !ok {
		t.Errorf("expected issue 42 in resource output")
	}
}

func TestExtractID(t *testing.T) {
	if got := extractID(42.0); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
	if got := extractID(123); got != "123" {
		t.Errorf("expected '123', got %q", got)
	}
	if got := extractID("abc"); got != "abc" {
		t.Errorf("expected 'abc', got %q", got)
	}
	if got := extractID(nil); got != "" {
		t.Errorf("expected '', got %q", got)
	}
}
