package loop

import (
	"context"
	"encoding/json"
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

func RunLoop(ctx context.Context, cfg *config.Config, prompt string) (err error) {
	if cfg.ListSessions {
		return handleListSessions()
	}

	// Initialize MCP servers and registry
	manager := mcp.NewProcessManager()
	registry := mcp.NewRegistry()

	loopCtx, cancel := context.WithCancel(ctx)
	stopSignal := manager.StartSignalListener(cancel, 5*time.Second)
	defer stopSignal()
	defer manager.ShutdownAll(5 * time.Second)

	for name, srvCfg := range cfg.Servers {
		sp, srvErr := mcp.NewServerProcess(loopCtx, name, srvCfg)
		if srvErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to start MCP server %s: %v\n", name, srvErr)
			continue
		}
		manager.Add(name, sp)
		if registryErr := registry.AddClient(name, sp.Client()); registryErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to register MCP server %s: %v\n", name, registryErr)
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

	formatter := NewTerminalFormatter(os.Stdout, getTerminalWidth())
	defer func() {
		flushErr := formatter.Flush()
		if err == nil && flushErr != nil {
			err = fmt.Errorf("failed to flush output: %w", flushErr)
		}
	}()

	updatedMessages, loopErr := runReActLoop(loopCtx, cfg, client, registry, formatter, messages)
	if loopErr != nil {
		return loopErr
	}

	if session != nil {
		session.Model = cfg.Model
		session.Messages = updatedMessages
		if saveErr := SaveSession(session); saveErr != nil {
			return fmt.Errorf("failed to save session: %w", saveErr)
		}
	}

	return nil
}

func runReActLoop(ctx context.Context, cfg *config.Config, client llm.LLMClient, registry *mcp.Registry, formatter *TerminalFormatter, messages []llm.Message) ([]llm.Message, error) {
	for i := 0; i < cfg.MaxLoopIterations; i++ {
		mcpTools, listErr := registry.ListAllTools(ctx)
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to list tools: %v\n", listErr)
		}
		tools, _ := mcp.TranslateTools(mcpTools)

		assistantMsg, genErr := client.Generate(ctx, messages, tools)
		if genErr != nil {
			return nil, fmt.Errorf("failed to generate response: %w", genErr)
		}

		if assistantMsg.Content != "" {
			if _, wErr := formatter.Write([]byte(assistantMsg.Content)); wErr != nil {
				return nil, fmt.Errorf("failed to write output: %w", wErr)
			}
			_ = formatter.Flush()
		}

		messages = append(messages, *assistantMsg)

		if len(assistantMsg.ToolCalls) == 0 {
			break
		}

		messages = executeTools(ctx, cfg, registry, assistantMsg.ToolCalls, messages)

		if i == cfg.MaxLoopIterations-1 && len(assistantMsg.ToolCalls) > 0 {
			fmt.Fprintf(os.Stderr, "\nWarning: reached maximum loop iterations (%d)\n", cfg.MaxLoopIterations)
		}
	}
	return messages, nil
}

func executeTools(ctx context.Context, cfg *config.Config, registry *mcp.Registry, toolCalls []llm.ToolCall, messages []llm.Message) []llm.Message {
	profile := Interactive
	if cfg.AutoConfirm {
		profile = Bypass
	}
	guard := NewGuard(profile, nil, nil)

	for _, tc := range toolCalls {
		args := make(map[string]interface{})
		if tc.Arguments != "" {
			if unmarshalErr := json.Unmarshal([]byte(tc.Arguments), &args); unmarshalErr != nil {
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Content:    fmt.Sprintf("Error parsing arguments: %v", unmarshalErr),
					ToolCallID: tc.ID,
				})
				continue
			}
		}

		allowed, err := guard.Authorize(tc.Name, args)
		if err != nil {
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    err.Error(),
				ToolCallID: tc.ID,
			})
			continue
		}

		if !allowed {
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    "Error: user denied tool execution",
				ToolCallID: tc.ID,
			})
			continue
		}

		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "\n=> Executing tool: %s\n", tc.Name)
		}
		result, callErr := registry.CallTool(ctx, tc.Name, args)
		if callErr != nil {
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    fmt.Sprintf("Error calling tool: %v", callErr),
				ToolCallID: tc.ID,
			})
			continue
		}

		formattedRes, _ := mcp.FormatToolResult(result)
		messages = append(messages, llm.Message{
			Role:       llm.RoleTool,
			Content:    formattedRes,
			ToolCallID: tc.ID,
		})
	}
	return messages
}
