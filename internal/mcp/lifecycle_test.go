package mcp

import (
	"testing"
	"time"
)

func TestProcessManager_AddAndShutdown(t *testing.T) {
	manager := NewProcessManager()

	manager.Add("test", &ServerProcess{})
	if len(manager.processes) != 1 {
		t.Errorf("expected 1 process in manager, got %d", len(manager.processes))
	}

	// Test ShutdownAll handles nil cmd correctly (will just clean map)
	manager.ShutdownAll(1 * time.Second)

	if len(manager.processes) != 0 {
		t.Errorf("expected 0 processes after shutdown, got %d", len(manager.processes))
	}
}

func TestProcessManager_SignalListener(t *testing.T) {
	manager := NewProcessManager()
	
	// Ensure that stop function successfully closes the channel
	stopFunc := manager.StartSignalListener(1 * time.Second)
	stopFunc() // Test that it doesn't block
	
	// Test idempotent stop (sync.Once)
	stopFunc()
}
