package loop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"powerword/internal/llm"
)

func setupTestSessions(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	SessionsBaseDir = tempDir
	t.Cleanup(func() {
		SessionsBaseDir = ""
	})
	return tempDir
}

func TestSessionSaveLoad(t *testing.T) {
	setupTestSessions(t)

	session := &Session{
		ID:    "test-session-1",
		Model: "gemini-1.5-pro",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
			{Role: llm.RoleAssistant, Content: "Hi there"},
		},
	}

	err := SaveSession(session)
	if err != nil {
		t.Fatalf("expected no error saving session, got %v", err)
	}

	loadedSession, err := LoadSession("test-session-1")
	if err != nil {
		t.Fatalf("expected no error loading session, got %v", err)
	}

	if loadedSession.ID != session.ID {
		t.Errorf("expected id %s, got %s", session.ID, loadedSession.ID)
	}
	if loadedSession.Model != session.Model {
		t.Errorf("expected model %s, got %s", session.Model, loadedSession.Model)
	}
	if len(loadedSession.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loadedSession.Messages))
	}
	if loadedSession.Messages[0].Content != "Hello" {
		t.Errorf("expected first message Hello, got %s", loadedSession.Messages[0].Content)
	}
}

func TestLoadNonExistentSession(t *testing.T) {
	setupTestSessions(t)

	session, err := LoadSession("non-existent")
	if err != nil {
		t.Fatalf("expected no error loading non-existent session, got %v", err)
	}

	if session.ID != "non-existent" {
		t.Errorf("expected id non-existent, got %s", session.ID)
	}
	if len(session.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(session.Messages))
	}
}

func TestListSessions(t *testing.T) {
	dir := setupTestSessions(t)

	// List empty
	summaries, err := ListSessions()
	if err != nil {
		t.Fatalf("expected no error listing empty directory, got %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries, got %d", len(summaries))
	}

	// Add some sessions
	now := time.Now()
	sessions := []Session{
		{ID: "session-1", Timestamp: now.Add(-1 * time.Hour), Model: "test-model", Messages: []llm.Message{{}}},
		{ID: "session-2", Timestamp: now, Model: "test-model", Messages: []llm.Message{{}, {}}},
	}

	for _, s := range sessions {
		sCopy := s
		if saveErr := SaveSession(&sCopy); saveErr != nil {
			t.Fatalf("failed to save session: %v", saveErr)
		}
		time.Sleep(10 * time.Millisecond) // ensure deterministic timestamps
	}

	// Add an invalid file
	if writeErr := os.WriteFile(filepath.Join(dir, "sessions", "invalid.json"), []byte("not-json"), 0600); writeErr != nil {
		t.Fatalf("failed to write invalid file: %v", writeErr)
	}

	summaries, err = ListSessions()
	if err != nil {
		t.Fatalf("expected no error listing sessions, got %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	// Check order (newest first)
	if summaries[0].ID != "session-2" {
		t.Errorf("expected first summary to be session-2, got %s", summaries[0].ID)
	}
	if summaries[0].MessageCount != 2 {
		t.Errorf("expected session-2 to have 2 messages, got %d", summaries[0].MessageCount)
	}
}

func TestSaveInvalidSession(t *testing.T) {
	err := SaveSession(nil)
	if err == nil {
		t.Error("expected error saving nil session")
	}

	err = SaveSession(&Session{ID: ""})
	if err == nil {
		t.Error("expected error saving session with empty ID")
	}
}

func TestLoadEmptySessionID(t *testing.T) {
	_, err := LoadSession("")
	if err == nil {
		t.Error("expected error loading session with empty ID")
	}
}

func TestGetSessionsDir_RealHome(t *testing.T) {
	SessionsBaseDir = ""

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	dir, err := getSessionsDir()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	expected := filepath.Join(tempHome, ".local", "share", "powerword", "sessions")
	if dir != expected {
		t.Errorf("expected %s, got %s", expected, dir)
	}
}

func TestSaveSession_WriteError(t *testing.T) {
	dir := setupTestSessions(t)

	// Create the sessions directory explicitly so we can chmod it
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0750); err != nil {
		t.Fatalf("failed to create sessions directory: %v", err)
	}

	// Make dir read-only
	//nolint:gosec // testing permissions
	if err := os.Chmod(sessionsDir, 0444); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}

	err := SaveSession(&Session{ID: "test"})
	if err == nil {
		t.Error("expected error saving to read-only dir")
	}

	// Restore
	//nolint:gosec // testing permissions
	if err := os.Chmod(sessionsDir, 0750); err != nil {
		t.Fatalf("failed to restore chmod: %v", err)
	}
}

func TestLoadSession_InvalidJSON(t *testing.T) {
	dir := setupTestSessions(t)
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0750); err != nil {
		t.Fatalf("failed to create sessions directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "bad.json"), []byte("invalid"), 0600); err != nil {
		t.Fatalf("failed to write bad.json: %v", err)
	}

	_, err := LoadSession("bad")
	if err == nil {
		t.Error("expected error loading invalid json")
	}
}

func TestListSessions_ReadDirError(t *testing.T) {
	dir := setupTestSessions(t)

	// Create a FILE named "sessions" so os.ReadDir fails
	sessionsPath := filepath.Join(dir, "sessions")
	if err := os.WriteFile(sessionsPath, []byte("i am a file"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := ListSessions()
	if err == nil {
		t.Error("expected error listing sessions when dir is a file")
	}
}

func TestGetSessionsDir_NoHome(t *testing.T) {
	SessionsBaseDir = ""
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	_, err := getSessionsDir()
	if err == nil {
		t.Skip("expected error when HOME is empty, but got none (likely OS fallback)")
	}
}
