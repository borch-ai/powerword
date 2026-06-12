package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/borch-ai/powerword/internal/mcp"
	"github.com/borch-ai/powerword/pkg/config"
	"github.com/borch-ai/powerword/pkg/llm"
	"github.com/borch-ai/powerword/pkg/telemetry"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// newClient is a package-level variable that defaults to llm.NewClient.
// It can be overridden in unit tests to return a mock client.
var newClient = llm.NewClient

// JSONPayload represents the structured output for headless mode.
type JSONPayload struct {
	Response        string                  `json:"response"`
	ToolsExecuted   []llm.ToolCall          `json:"tools_executed,omitempty"`
	ExecutionStatus string                  `json:"execution_status"`
	Usage           *telemetry.UsageTracker `json:"usage,omitempty"`
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

func setupWorkspaceRollback(ctx context.Context, cfg *config.Config, loopFailed *bool) (*WorkspaceSnapshot, func(), error) {
	if !cfg.GitRollback {
		return nil, func() {}, nil
	}
	snapshot, snapErr := NewWorkspaceSnapshot(ctx, "")
	if snapErr != nil {
		return nil, nil, fmt.Errorf("failed to initialize workspace rollback snapshot: %w", snapErr)
	}
	cleanupFunc := func() {
		if *loopFailed {
			fmt.Fprintln(os.Stderr, "Error in agent loop. Rolling back workspace...")
			restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer restoreCancel()
			if restoreErr := snapshot.Restore(restoreCtx); restoreErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to restore workspace rollback snapshot: %v\n", restoreErr)
			}
		} else {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cleanupCancel()
			if cleanErr := snapshot.CleanUp(cleanupCtx); cleanErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to clean up workspace snapshot stash: %v\n", cleanErr)
			}
		}
	}
	return snapshot, cleanupFunc, nil
}

func initializeSessionAndClient(ctx context.Context, cfg *config.Config, prompt string) (*Session, llm.LLMClient, string, string, []llm.Message, error) {
	var session *Session
	var messages []llm.Message
	var err error

	sessionID := cfg.Session
	isResume := false
	if cfg.Resume != "" {
		sessionID = cfg.Resume
		isResume = true
	}

	if sessionID != "" {
		session, err = LoadSession(sessionID)
		if err != nil {
			return nil, nil, "", "", nil, fmt.Errorf("failed to load session %s: %w", sessionID, err)
		}
		messages = session.Messages
	}

	if isResume && (session == nil || len(session.Messages) == 0) {
		return nil, nil, "", "", nil, fmt.Errorf("cannot resume empty or non-existent session %q", sessionID)
	}

	activeClient, targetModel, prompt, err := resolveClientAndRoute(ctx, cfg, prompt)
	if err != nil {
		return nil, nil, "", "", nil, err
	}

	if cfg.Verbose && targetModel != cfg.Model {
		fmt.Fprintf(os.Stderr, "Routed to model: %s\n", targetModel)
	}

	if !isResume || prompt != "" {
		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: prompt,
		})
	}

	return session, activeClient, targetModel, prompt, messages, nil
}

func validateAndResolvePrompt(cfg *config.Config, prompt string) (string, error) {
	if cfg.Resume != "" && prompt != "" {
		return "", fmt.Errorf("cannot provide a prompt when resuming a session; use --session instead to continue with a new prompt")
	}
	if prompt == "" && cfg.Resume == "" {
		p, _ := readStdinPrompt()
		prompt = p
	}
	if prompt == "" && !cfg.ListSessions && cfg.Resume == "" {
		return "", fmt.Errorf("no prompt provided and stdin is empty")
	}
	return prompt, nil
}

func handleSaveSession(session *Session, targetModel string, updatedMessages []llm.Message, isPaused bool, loopFailed *bool) error {
	session.Model = targetModel
	session.Messages = updatedMessages
	if saveErr := SaveSession(session); saveErr != nil {
		if !isPaused {
			*loopFailed = true
		}
		return fmt.Errorf("failed to save session: %w", saveErr)
	}
	if isPaused {
		fmt.Fprintf(os.Stderr, "\nSession paused. To resume, run:\n  powerword --resume %s\n", session.ID)
	}
	return nil
}

func RunLoop(ctx context.Context, cfg *config.Config, prompt string) (err error) {
	if cfg.ListSessions {
		return handleListSessions()
	}

	var promptErr error
	prompt, promptErr = validateAndResolvePrompt(cfg, prompt)
	if promptErr != nil {
		return promptErr
	}

	var loopFailed bool
	_, rollbackCleanup, err := setupWorkspaceRollback(ctx, cfg, &loopFailed)
	if err != nil {
		return err
	}
	defer rollbackCleanup()

	// Initialize MCP servers and registry
	manager := mcp.NewProcessManager()
	registry := mcp.NewRegistry()

	loopCtx, cancel := context.WithCancel(ctx)
	stopSignal := manager.StartSignalListener(cancel, 5*time.Second)
	defer stopSignal()
	defer manager.ShutdownAll(5 * time.Second)

	startServers(loopCtx, cfg, manager, registry)

	session, activeClient, targetModel, _, messages, initErr := initializeSessionAndClient(ctx, cfg, prompt)
	if initErr != nil {
		loopFailed = true
		return initErr
	}

	outWriter := getOutputWriter(cfg)
	formatter := NewTerminalFormatter(outWriter, getTerminalWidth())
	defer func() {
		flushErr := formatter.Flush()
		if err == nil && flushErr != nil {
			err = fmt.Errorf("failed to flush output: %w", flushErr)
		}
	}()

	tracker := telemetry.NewUsageTracker()

	initialLen := len(messages)
	updatedMessages, loopErr := runReActLoop(loopCtx, cfg, activeClient, registry, formatter, messages, tracker, targetModel)
	isPaused := errors.Is(loopErr, ErrSessionPaused)

	if loopErr != nil && !isPaused {
		loopFailed = true
	}

	if isPaused && session == nil {
		pausedID := fmt.Sprintf("paused-%d", time.Now().Unix())
		session = &Session{
			ID:        pausedID,
			Timestamp: time.Now(),
			Messages:  make([]llm.Message, 0),
		}
		fmt.Fprintf(os.Stderr, "Generating session ID for pause: %s\n", pausedID)
	}

	if session != nil && (loopErr == nil || isPaused) {
		if saveErr := handleSaveSession(session, targetModel, updatedMessages, isPaused, &loopFailed); saveErr != nil {
			loopErr = saveErr
		} else if isPaused {
			loopErr = nil
		}
	}

	if cfg.JSONOutput {
		printJSONPayload(loopErr, updatedMessages, initialLen, tracker)
	} else if len(tracker.ModelUsages) > 0 {
		fmt.Fprintln(os.Stderr, "\n"+tracker.FormatSummary(cfg.Pricing))
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

func printJSONPayload(loopErr error, updatedMessages []llm.Message, initialLen int, tracker *telemetry.UsageTracker) {
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
	if tracker != nil && len(tracker.ModelUsages) > 0 {
		payload.Usage = tracker
	}

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

func runReActLoop(ctx context.Context, cfg *config.Config, client llm.LLMClient, registry *mcp.Registry, formatter *TerminalFormatter, messages []llm.Message, tracker *telemetry.UsageTracker, modelName string) ([]llm.Message, error) {
	pending, _ := getPendingToolCalls(messages)
	if len(pending) > 0 {
		var execErr error
		messages, execErr = executeTools(ctx, cfg, registry, pending, messages)
		if execErr != nil {
			return messages, execErr
		}
	}

	for i := 0; i < cfg.MaxLoopIterations; i++ {
		var done bool
		var err error
		messages, done, err = executeLoopIteration(ctx, cfg, client, registry, formatter, messages, tracker, modelName, i)
		if err != nil {
			return messages, err
		}
		if done {
			break
		}
	}
	return messages, nil
}

func executeLoopIteration(ctx context.Context, cfg *config.Config, client llm.LLMClient, registry *mcp.Registry, formatter *TerminalFormatter, messages []llm.Message, tracker *telemetry.UsageTracker, modelName string, i int) ([]llm.Message, bool, error) {
	if err := checkBudget(cfg, tracker); err != nil {
		return messages, false, err
	}

	var tools []llm.ToolDefinition
	if registry != nil {
		mcpTools, listErr := registry.ListAllTools(ctx)
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to list tools: %v\n", listErr)
		}
		tools, _ = mcp.TranslateTools(mcpTools)
	}

	assistantMsg, genErr := client.Generate(ctx, messages, tools)
	if genErr != nil {
		return messages, false, fmt.Errorf("failed to generate response: %w", genErr)
	}

	var usage llm.TokenUsage
	if assistantMsg.Usage != nil {
		usage = *assistantMsg.Usage
	}
	tracker.RecordUsage(modelName, usage)

	if assistantMsg.Content != "" {
		if _, wErr := formatter.Write([]byte(assistantMsg.Content)); wErr != nil {
			return messages, false, fmt.Errorf("failed to write output: %w", wErr)
		}
		_ = formatter.Flush()
	}

	messages = append(messages, *assistantMsg)

	if err := checkBudget(cfg, tracker); err != nil {
		return messages, false, err
	}

	if len(assistantMsg.ToolCalls) == 0 {
		return messages, true, nil
	}

	var executeErr error
	messages, executeErr = executeTools(ctx, cfg, registry, assistantMsg.ToolCalls, messages)
	if executeErr != nil {
		return messages, false, executeErr
	}

	if i == cfg.MaxLoopIterations-1 && len(assistantMsg.ToolCalls) > 0 {
		fmt.Fprintf(os.Stderr, "\nWarning: reached maximum loop iterations (%d)\n", cfg.MaxLoopIterations)
	}

	return messages, false, nil
}

func executeSingleTool(ctx context.Context, cfg *config.Config, registry *mcp.Registry, guard *Guard, tc llm.ToolCall) (llm.Message, error) {
	args := make(map[string]interface{})
	if tc.Arguments != "" {
		if unmarshalErr := json.Unmarshal([]byte(tc.Arguments), &args); unmarshalErr != nil {
			return llm.Message{
				Role:       llm.RoleTool,
				Content:    fmt.Sprintf("Error parsing arguments: %v", unmarshalErr),
				ToolCallID: tc.ID,
			}, nil
		}
	}

	allowed, err := guard.Authorize(tc.Name, args)
	if err != nil {
		if errors.Is(err, ErrSessionPaused) {
			return llm.Message{}, err
		}
		return llm.Message{
			Role:       llm.RoleTool,
			Content:    err.Error(),
			ToolCallID: tc.ID,
		}, nil
	}

	if !allowed {
		return llm.Message{
			Role:       llm.RoleTool,
			Content:    "Error: user denied tool execution",
			ToolCallID: tc.ID,
		}, nil
	}

	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "\n=> Executing tool: %s\n", tc.Name)
	}
	var result *mcpsdk.CallToolResult
	var callErr error
	if registry != nil {
		result, callErr = registry.CallTool(ctx, tc.Name, args)
	} else {
		callErr = errors.New("MCP registry not initialized")
	}
	if callErr != nil {
		return llm.Message{
			Role:       llm.RoleTool,
			Content:    fmt.Sprintf("Error calling tool: %v", callErr),
			ToolCallID: tc.ID,
		}, nil
	}

	formattedRes, _ := mcp.FormatToolResult(result)
	return llm.Message{
		Role:       llm.RoleTool,
		Content:    formattedRes,
		ToolCallID: tc.ID,
	}, nil
}

func executeTools(ctx context.Context, cfg *config.Config, registry *mcp.Registry, toolCalls []llm.ToolCall, messages []llm.Message) ([]llm.Message, error) {
	profile := Interactive
	if cfg.AutoConfirm {
		profile = Bypass
	} else if cfg.Headless {
		profile = ReadOnly
	}
	guard := NewGuard(profile, nil, nil)

	for _, tc := range toolCalls {
		msg, err := executeSingleTool(ctx, cfg, registry, guard, tc)
		if err != nil {
			if errors.Is(err, ErrSessionPaused) {
				return messages, err
			}
		}
		if msg.Role != "" {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

func getOutputWriter(cfg *config.Config) io.Writer {
	if cfg.OutputWriter != nil {
		return cfg.OutputWriter
	}
	if cfg.JSONOutput {
		return io.Discard
	}
	return os.Stdout
}

// BudgetExceededError indicates that a configured token or cost budget has been exceeded.
type BudgetExceededError struct {
	Reason string
}

func (e *BudgetExceededError) Error() string {
	return "budget exceeded: " + e.Reason
}

// checkBudget checks if the accumulated usage has exceeded any configured budgets.
func checkBudget(cfg *config.Config, tracker *telemetry.UsageTracker) error {
	if cfg.MaxCost > 0 {
		currentCost := tracker.EstimatedCost(cfg.Pricing)
		if currentCost >= cfg.MaxCost {
			return &BudgetExceededError{
				Reason: fmt.Sprintf("estimated cost $%.5f exceeded maximum budget of $%.5f", currentCost, cfg.MaxCost),
			}
		}
	}

	if cfg.MaxTokens > 0 {
		totalTokens := tracker.TotalTokens()
		if totalTokens >= cfg.MaxTokens {
			return &BudgetExceededError{
				Reason: fmt.Sprintf("total tokens %d exceeded maximum budget of %d", totalTokens, cfg.MaxTokens),
			}
		}
	}

	if cfg.MaxInputTokens > 0 {
		inputTokens := tracker.TotalInputTokens()
		if inputTokens >= cfg.MaxInputTokens {
			return &BudgetExceededError{
				Reason: fmt.Sprintf("input tokens %d exceeded maximum budget of %d", inputTokens, cfg.MaxInputTokens),
			}
		}
	}

	if cfg.MaxOutputTokens > 0 {
		outputTokens := tracker.TotalOutputTokens()
		if outputTokens >= cfg.MaxOutputTokens {
			return &BudgetExceededError{
				Reason: fmt.Sprintf("output tokens %d exceeded maximum budget of %d", outputTokens, cfg.MaxOutputTokens),
			}
		}
	}

	if cfg.MaxCachedTokens > 0 {
		cachedTokens := tracker.TotalCachedTokens()
		if cachedTokens >= cfg.MaxCachedTokens {
			return &BudgetExceededError{
				Reason: fmt.Sprintf("cached tokens %d exceeded maximum budget of %d", cachedTokens, cfg.MaxCachedTokens),
			}
		}
	}

	return nil
}

// getPendingToolCalls checks if there are tool calls in the last assistant message
// that do not have corresponding tool responses in the subsequent message history.
func getPendingToolCalls(messages []llm.Message) ([]llm.ToolCall, int) {
	// Find index of the last assistant message
	lastAssistantIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleAssistant {
			lastAssistantIdx = i
			break
		}
	}
	if lastAssistantIdx == -1 {
		return nil, -1
	}

	assistantMsg := messages[lastAssistantIdx]
	if len(assistantMsg.ToolCalls) == 0 {
		return nil, -1
	}

	// Build a map of tool call IDs that have been executed (have a subsequent RoleTool response)
	executed := make(map[string]bool)
	for i := lastAssistantIdx + 1; i < len(messages); i++ {
		if messages[i].Role == llm.RoleTool && messages[i].ToolCallID != "" {
			executed[messages[i].ToolCallID] = true
		}
	}

	// Filter down to only those tool calls that are not marked executed
	var pending []llm.ToolCall
	for _, tc := range assistantMsg.ToolCalls {
		if !executed[tc.ID] {
			pending = append(pending, tc)
		}
	}

	return pending, lastAssistantIdx
}
