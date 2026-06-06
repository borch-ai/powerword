package loop

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"powerword/internal/config"
	"powerword/internal/llm"
	"powerword/internal/mcp"
)

// newClient is a package-level variable that defaults to llm.NewClient.
// It can be overridden in unit tests to return a mock client.
var newClient = llm.NewClient

// handleListSessions processes the session listing output.
func handleListSessions() error {
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

// consumeStream processes the LLM output stream chunk by chunk.
func consumeStream(chunks <-chan llm.StreamChunk, formatter *TerminalFormatter) (string, error) {
	var fullResponse strings.Builder
	for chunk := range chunks {
		if chunk.Error != nil {
			return "", fmt.Errorf("error in stream chunk: %w", chunk.Error)
		}
		if chunk.Content != "" {
			fullResponse.WriteString(chunk.Content)
			if _, err := formatter.Write([]byte(chunk.Content)); err != nil {
				return "", fmt.Errorf("failed to write output: %w", err)
			}
		}
	}
	return fullResponse.String(), nil
}

// RunLoop runs the core execution and reasoning loop.
func RunLoop(ctx context.Context, cfg *config.Config, prompt string) (err error) {
	if cfg.ListSessions {
		return handleListSessions()
	}

	// Initialize MCP servers and registry
	manager := mcp.NewProcessManager()
	registry := mcp.NewRegistry()
	
	stopSignal := manager.StartSignalListener(5 * time.Second)
	defer stopSignal()
	defer manager.ShutdownAll(5 * time.Second)

	for name, srvCfg := range cfg.Servers {
		sp, err := mcp.NewServerProcess(ctx, name, srvCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to start MCP server %s: %v\n", name, err)
			continue
		}
		manager.Add(name, sp)
		if err := registry.AddClient(name, sp.Client()); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to register MCP server %s: %v\n", name, err)
		}
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

	formatter := NewTerminalFormatter(os.Stdout, getTerminalWidth())
	defer func() {
		flushErr := formatter.Flush()
		if err == nil && flushErr != nil {
			err = fmt.Errorf("failed to flush output: %w", flushErr)
		}
	}()

	fullResponse, streamErr := consumeStream(chunks, formatter)
	if streamErr != nil {
		return streamErr
	}

	if session != nil {
		session.Model = cfg.Model
		messages = append(messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: fullResponse,
		})
		session.Messages = messages
		if saveErr := SaveSession(session); saveErr != nil {
			return fmt.Errorf("failed to save session: %w", saveErr)
		}
	}

	return nil
}
