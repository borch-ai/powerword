package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"powerword/internal/llm"
)

// Session represents a stored chat session.
type Session struct {
	ID        string        `json:"id"`
	Timestamp time.Time     `json:"timestamp"`
	Model     string        `json:"model"`
	Messages  []llm.Message `json:"messages"`
}

// SessionSummary provides metadata about a session for listing.
type SessionSummary struct {
	ID           string
	Timestamp    time.Time
	Model        string
	MessageCount int
}

var sessionsBaseDir string

func getSessionsDir() (string, error) {
	if sessionsBaseDir != "" {
		dir := filepath.Join(sessionsBaseDir, "sessions")
		if err := os.MkdirAll(dir, 0750); err != nil {
			return "", fmt.Errorf("failed to create sessions directory: %w", err)
		}
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	dir := filepath.Join(home, ".local", "share", "powerword", "sessions")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("failed to create sessions directory: %w", err)
	}
	return dir, nil
}

// SaveSession saves the given session to a JSON file.
func SaveSession(session *Session) error {
	if session == nil {
		return errors.New("invalid session")
	}
	if session.ID == "" || session.ID == "." || session.ID == ".." || strings.ContainsAny(session.ID, "/\\") {
		return fmt.Errorf("invalid session ID: %q", session.ID)
	}

	dir, err := getSessionsDir()
	if err != nil {
		return err
	}

	// Update timestamp on save
	session.Timestamp = time.Now()

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	filePath := filepath.Join(dir, session.ID+".json")
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// LoadSession loads a session by ID. If the file doesn't exist, it returns a new empty session.
func LoadSession(id string) (*Session, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") {
		return nil, fmt.Errorf("invalid session ID: %q", id)
	}

	dir, err := getSessionsDir()
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(dir, id+".json")
	//nolint:gosec // filePath is constructed from known dir
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Return an empty session to be populated later
			return &Session{
				ID:        id,
				Timestamp: time.Now(),
				Messages:  make([]llm.Message, 0),
			}, nil
		}
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// ListSessions returns a summary of all saved sessions.
func ListSessions() ([]SessionSummary, error) {
	dir, err := getSessionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []SessionSummary{}, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var summaries []SessionSummary
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		//nolint:gosec // filePath is constructed from known dir
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Skip files that can't be read
			continue
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			// Skip invalid JSON
			continue
		}

		summaries = append(summaries, SessionSummary{
			ID:           session.ID,
			Timestamp:    session.Timestamp,
			Model:        session.Model,
			MessageCount: len(session.Messages),
		})
	}

	// Sort newest first
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Timestamp.After(summaries[j].Timestamp)
	})

	return summaries, nil
}
