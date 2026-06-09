package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// JSONPayload represents the structured output for headless mode.
type JSONPayload struct {
	Response        string         `json:"response"`
	ToolsExecuted   []llm.ToolCall `json:"tools_executed,omitempty"`
	ExecutionStatus string         `json:"execution_status"`
}

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

	if prompt == "" {
		p, _ := readStdinPrompt()
		prompt = p
	}
	if prompt == "" && !cfg.ListSessions {
		return fmt.Errorf("no prompt provided and stdin is empty")
	}

	// Initialize MCP servers and registry
	manager := mcp.NewProcessManager()
	registry := mcp.NewRegistry()

	loopCtx, cancel := context.WithCancel(ctx)
	stopSignal := manager.StartSignalListener(cancel, 5*time.Second)
	defer stopSignal()
	defer manager.ShutdownAll(5 * time.Second)

	startServers(loopCtx, cfg, manager, registry)

	var session *Session
	var messages []llm.Message

	if cfg.Session != "" {
		session, err = LoadSession(cfg.Session)
		if err != nil {
			return fmt.Errorf("failed to load session %s: %w", cfg.Session, err)
		}
		messages = session.Messages
	}

	activeClient, targetModel, prompt, err := resolveClientAndRoute(ctx, cfg, prompt)
	if err != nil {
		return err
	}

	if cfg.Verbose && targetModel != cfg.Model {
		fmt.Fprintf(os.Stderr, "Routed to model: %s\n", targetModel)
	}

	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: prompt,
	})

	var outWriter io.Writer = os.Stdout
	if cfg.JSONOutput {
		outWriter = io.Discard
	}
	formatter := NewTerminalFormatter(outWriter, getTerminalWidth())
	defer func() {
		flushErr := formatter.Flush()
		if err == nil && flushErr != nil {
			err = fmt.Errorf("failed to flush output: %w", flushErr)
		}
	}()

	initialLen := len(messages)
	updatedMessages, loopErr := runReActLoop(loopCtx, cfg, activeClient, registry, formatter, messages)

	if session != nil && loopErr == nil {
		session.Model = targetModel
		session.Messages = updatedMessages
		if saveErr := SaveSession(session); saveErr != nil {
			loopErr = fmt.Errorf("failed to save session: %w", saveErr)
		}
	}

	if cfg.JSONOutput {
		printJSONPayload(loopErr, updatedMessages, initialLen)
	}

	return loopErr
}

func startServers(ctx context.Context, cfg *config.Config, manager *mcp.ProcessManager, registry *mcp.Registry) {
	for name, srvCfg := range cfg.Servers {
		sp, srvErr := mcp.NewServerProcess(ctx, name, srvCfg)
		if srvErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to start MCP server %s: %v\n", name, srvErr)
			continue
		}
		manager.Add(name, sp)
		if registryErr := registry.AddClient(name, sp.Client()); registryErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to register MCP server %s: %v\n", name, registryErr)
		}
	}
}

func readStdinPrompt() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		b, readErr := io.ReadAll(os.Stdin)
		if readErr == nil {
			return strings.TrimSpace(string(b)), nil
		}
		return "", readErr
	}
	return "", nil
}

func printJSONPayload(loopErr error, updatedMessages []llm.Message, initialLen int) {
	payload := JSONPayload{
		ExecutionStatus: "success",
	}
	if loopErr != nil {
		payload.ExecutionStatus = "error: " + loopErr.Error()
	}

	var responseBuilder strings.Builder
	var executedTools []llm.ToolCall
	for i := initialLen; i < len(updatedMessages); i++ {
		msg := updatedMessages[i]
		if msg.Role == llm.RoleAssistant && msg.Content != "" {
			responseBuilder.WriteString(msg.Content)
		}
		if msg.Role == llm.RoleAssistant && len(msg.ToolCalls) > 0 {
			executedTools = append(executedTools, msg.ToolCalls...)
		}
	}
	payload.Response = responseBuilder.String()
	payload.ToolsExecuted = executedTools

	b, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Println(string(b))
}

func resolveClientAndRoute(ctx context.Context, cfg *config.Config, prompt string) (llm.LLMClient, string, string, error) {
	clientCache := make(map[string]llm.LLMClient)

	defaultClient, err := newClient(cfg)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to create LLM client: %w", err)
	}
	clientCache[cfg.Model] = defaultClient

	var classifierClient llm.LLMClient
	if cfg.ClassifierModel != "" {
		classifierCfg := *cfg
		classifierCfg.Model = cfg.ClassifierModel
		cc, cErr := newClient(&classifierCfg)
		if cErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create classifier client %s: %v\n", cfg.ClassifierModel, cErr)
		} else {
			classifierClient = cc
			clientCache[cfg.ClassifierModel] = cc
		}
	}

	router := llm.NewRouter(cfg, classifierClient)
	targetModel, modifiedPrompt, routeErr := router.Route(ctx, prompt)
	if routeErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: routing failed: %v\n", routeErr)
		targetModel = cfg.Model
		modifiedPrompt = prompt
	}
	prompt = modifiedPrompt

	activeClient, exists := clientCache[targetModel]
	if !exists {
		targetCfg := *cfg
		targetCfg.Model = targetModel
		ac, acErr := newClient(&targetCfg)
		if acErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create client for target model %s: %v, falling back to default\n", targetModel, acErr)
			activeClient = defaultClient
			targetModel = cfg.Model
		} else {
			activeClient = ac
		}
	}
	return activeClient, targetModel, prompt, nil
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
	} else if cfg.Headless {
		profile = ReadOnly
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
