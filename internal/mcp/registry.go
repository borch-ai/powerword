package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Registry orchestrates multiple MCPClient connections.
// It aggregates tools across all clients and handles routing tool calls.
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*MCPClient
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		clients: make(map[string]*MCPClient),
	}
}

// AddClient registers a new MCP client under the given name.
func (r *Registry) AddClient(name string, client *MCPClient) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.clients[name]; exists {
		return fmt.Errorf("client %q already exists", name)
	}
	r.clients[name] = client
	return nil
}

// RemoveClient removes a client and closes its session.
func (r *Registry) RemoveClient(name string) error {
	r.mu.Lock()
	client, exists := r.clients[name]
	if exists {
		delete(r.clients, name)
	}
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("client %q not found", name)
	}
	return client.Close()
}

// ListAllTools queries all registered clients for their tools.
// To prevent name collisions, it prefixes tool names with the client name: "<client>__<tool>".
// It skips clients that fail to return tools, handling them gracefully.
func (r *Registry) ListAllTools(ctx context.Context) ([]mcpsdk.Tool, error) {
	r.mu.RLock()
	clientsCopy := make(map[string]*MCPClient, len(r.clients))
	for k, v := range r.clients {
		clientsCopy[k] = v
	}
	r.mu.RUnlock()

	var allTools []mcpsdk.Tool
	seenNames := make(map[string]bool)

	for clientName, client := range clientsCopy {
		tools, err := client.ListTools(ctx)
		if err != nil {
			// Gracefully handle client errors by skipping this client
			continue
		}

		for _, t := range tools {
			// Prefix tool name to prevent collisions
			aggregatedName := fmt.Sprintf("%s__%s", clientName, t.Name)

			if !seenNames[aggregatedName] {
				t.Name = aggregatedName
				allTools = append(allTools, t)
				seenNames[aggregatedName] = true
			}
		}
	}

	return allTools, nil
}

// CallTool parses the aggregated name and routes the call to the appropriate client.
func (r *Registry) CallTool(ctx context.Context, aggregatedName string, args map[string]interface{}) (*mcpsdk.CallToolResult, error) {
	parts := strings.SplitN(aggregatedName, "__", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid aggregated tool name %q: expected format <client>__<tool>", aggregatedName)
	}

	clientName := parts[0]
	originalToolName := parts[1]

	r.mu.RLock()
	client, exists := r.clients[clientName]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("client %q not found for tool %q", clientName, aggregatedName)
	}

	return client.CallTool(ctx, originalToolName, args)
}
