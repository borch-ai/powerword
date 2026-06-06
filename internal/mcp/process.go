package mcp

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"powerword/internal/config"
)

// ServerProcess manages the lifecycle of an external MCP server process.
type ServerProcess struct {
	cmd       *exec.Cmd
	client    *MCPClient
	cleanupWg sync.WaitGroup
}

// NewServerProcess creates, starts, and connects to an MCP server process.
// It wraps the process IO in an MCP transport and initializes the client session.
func NewServerProcess(ctx context.Context, name string, cfg config.ServerConfig) (*ServerProcess, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("server command cannot be empty")
	}

	// We don't use CommandContext here because we manage the termination manually
	// to allow for a graceful shutdown, instead of aggressive killing on context cancellation.
	//nolint:gosec // Command execution is intentional and from config
	cmd := exec.Command(cfg.Command, cfg.Args...)

	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if startErr := cmd.Start(); startErr != nil {
		return nil, fmt.Errorf("failed to start process: %w", startErr)
	}

	sp := &ServerProcess{
		cmd: cmd,
	}

	// Read stderr in the background to avoid blocking the process and to log diagnostics
	sp.cleanupWg.Add(1)
	go func() {
		defer sp.cleanupWg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("[MCP Server %s] stderr: %s", name, scanner.Text())
		}
	}()

	transport := &mcpsdk.IOTransport{
		Reader: stdout,
		Writer: stdin,
	}

	client, err := NewClient(ctx, transport)
	if err != nil {
		sp.ForceKill()
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}
	sp.client = client

	return sp, nil
}

// Client returns the underlying MCP client for this process.
func (sp *ServerProcess) Client() *MCPClient {
	return sp.client
}

// GracefulShutdown attempts to cleanly close the MCP session and terminate the process.
// If the process does not exit within the timeout, it is forcefully killed.
func (sp *ServerProcess) GracefulShutdown(timeout time.Duration) error {
	// Close the MCP session first
	if sp.client != nil {
		_ = sp.client.Close()
	}

	if sp.cmd == nil || sp.cmd.Process == nil {
		return nil
	}

	// Send an interrupt signal to ask the process to exit cleanly
	if err := sp.cmd.Process.Signal(os.Interrupt); err != nil {
		// If signal fails, fallback to kill
		sp.ForceKill()
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- sp.cmd.Wait()
	}()

	select {
	case err := <-done:
		sp.cleanupWg.Wait() // wait for stderr reader to finish
		return err
	case <-time.After(timeout):
		// Timeout reached, force kill
		sp.ForceKill()
		<-done // Wait for the kill to complete
		sp.cleanupWg.Wait()
		return fmt.Errorf("process did not exit gracefully within timeout, forcefully killed")
	}
}

// ForceKill forcefully terminates the process.
func (sp *ServerProcess) ForceKill() {
	if sp.cmd.Process != nil {
		_ = sp.cmd.Process.Signal(syscall.SIGKILL)
		_ = sp.cmd.Wait()
	}
}
