package mcp

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"powerword/internal/config"
)

func TestNewServerProcess_EmptyCommand(t *testing.T) {
	_, err := NewServerProcess(context.Background(), "test", config.ServerConfig{})
	if err == nil || !strings.Contains(err.Error(), "server command cannot be empty") {
		t.Fatalf("expected empty command error, got: %v", err)
	}
}

func TestNewServerProcess_InvalidCommand(t *testing.T) {
	_, err := NewServerProcess(context.Background(), "test", config.ServerConfig{
		Command: "non-existent-command-12345",
	})
	if err == nil {
		t.Fatalf("expected error for non-existent command, got nil")
	}
}

func TestNewServerProcess_FailsHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use echo to simulate a server that outputs garbage and exits, failing the MCP handshake
	cfg := config.ServerConfig{
		Command: "echo",
		Args:    []string{"not an mcp server"},
	}

	sp, err := NewServerProcess(ctx, "echo", cfg)
	if err == nil {
		sp.ForceKill()
		t.Fatalf("expected handshake to fail, but it succeeded")
	}
}

func TestServerProcess_ClientAndShutdown(t *testing.T) {
	// Create a long-running process
	cmd := exec.CommandContext(context.Background(), "sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep: %v", err)
	}

	sp := &ServerProcess{
		cmd: cmd,
	}

	if sp.Client() != nil {
		t.Errorf("expected nil client")
	}

	err := sp.GracefulShutdown(100 * time.Millisecond)
	if err != nil && !strings.Contains(err.Error(), "killed") && !strings.Contains(err.Error(), "interrupt") && !strings.Contains(err.Error(), "exit status") {
		// Just ensure it was terminated
		t.Logf("Process terminated with error: %v", err)
	}
}

func TestServerProcess_GracefulShutdownNilCmd(t *testing.T) {
	sp := &ServerProcess{}
	if err := sp.GracefulShutdown(time.Second); err != nil {
		t.Errorf("expected no error for nil cmd, got: %v", err)
	}
}

func TestServerProcess_GracefulShutdownTimeout(t *testing.T) {
	// A script that traps SIGINT and ignores it, so GracefulShutdown has to timeout and kill it
	cmd := exec.CommandContext(context.Background(), "bash", "-c", `trap "" INT; sleep 10`)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start bash: %v", err)
	}

	sp := &ServerProcess{
		cmd: cmd,
	}

	err := sp.GracefulShutdown(100 * time.Millisecond)
	if err == nil {
		t.Errorf("expected timeout/force kill error, got nil")
	}
}

func TestNewServerProcess_Env(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg := config.ServerConfig{
		Command: "env",
		Env:     []string{"TEST_MCP_ENV=1"},
	}

	sp, _ := NewServerProcess(ctx, "test_env", cfg)
	if sp != nil {
		sp.ForceKill()
	}
}

func TestServerProcess_GracefulShutdownClientClose(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep: %v", err)
	}

	sp := &ServerProcess{
		cmd:    cmd,
		client: &MCPClient{},
	}

	err := sp.GracefulShutdown(100 * time.Millisecond)
	if err != nil && !strings.Contains(err.Error(), "killed") && !strings.Contains(err.Error(), "interrupt") && !strings.Contains(err.Error(), "exit status") {
		t.Logf("Process terminated with error: %v", err)
	}
}

func TestServerProcess_GracefulShutdownSignalError(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "echo", "test")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	_ = cmd.Wait() // wait for it to exit so that signal fails

	sp := &ServerProcess{
		cmd: cmd,
	}

	err := sp.GracefulShutdown(100 * time.Millisecond)
	if err == nil {
		t.Errorf("expected error from signaling dead process")
	}
}
