package loop

import (
	"context"
	"fmt"
	"os"
	"strings"

	"powerword/internal/config"
	"powerword/internal/llm"
)

// newClient is a package-level variable that defaults to llm.NewClient.
// It can be overridden in unit tests to return a mock client.
var newClient = llm.NewClient

// RunLoop runs the core execution and reasoning loop.
func RunLoop(ctx context.Context, cfg *config.Config, prompt string) (err error) {
	if cfg.ListSessions {
		summaries, listErr := ListSessions()
		if listErr != nil {
			return fmt.Errorf("failed to list sessions: %w", listErr)
		}
		if len(summaries) == 0 {
			fmt.Println("No sessions found.")
			return nil
		}
		fmt.Printf("%-36s | %-19s | %-15s | %s\n", "SESSION ID", "LAST UPDATED", "MODEL", "MESSAGES")
		fmt.Println(strings.Repeat("-", 85))
		for _, s := range summaries {
			fmt.Printf("%-36s | %-19s | %-15s | %d\n", s.ID, s.Timestamp.Format("2006-01-02 15:04:05"), s.Model, s.MessageCount)
		}
		return nil
	}

	var session *Session
	var messages []llm.Message

	if cfg.Session != "" {
		session, err = LoadSession(cfg.Session)
		if err != nil {
			return fmt.Errorf("failed to load session %s: %w", cfg.Session, err)
		}
		messages = session.Messages
	}

	client, err := newClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: prompt,
	})

	chunks, err := client.Stream(ctx, messages, nil)
	if err != nil {
		return fmt.Errorf("failed to start model stream: %w", err)
	}

	width := getTerminalWidth()
	formatter := NewTerminalFormatter(os.Stdout, width)
	defer func() {
		flushErr := formatter.Flush()
		if err == nil && flushErr != nil {
			err = fmt.Errorf("failed to flush output: %w", flushErr)
		}
	}()

	var fullResponse strings.Builder
	for chunk := range chunks {
		if chunk.Error != nil {
			return fmt.Errorf("error in stream chunk: %w", chunk.Error)
		}
		if chunk.Content != "" {
			fullResponse.WriteString(chunk.Content)
			if _, err := formatter.Write([]byte(chunk.Content)); err != nil {
				return fmt.Errorf("failed to write output: %w", err)
			}
		}
	}

	if session != nil {
		session.Model = cfg.Model
		messages = append(messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: fullResponse.String(),
		})
		session.Messages = messages
		if saveErr := SaveSession(session); saveErr != nil {
			return fmt.Errorf("failed to save session: %w", saveErr)
		}
	}

	return nil
}
