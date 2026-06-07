package mcp

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ProcessManager manages all running ServerProcesses and handles shutdown signaling.
type ProcessManager struct {
	processes map[string]*ServerProcess
	mu        sync.Mutex
	stopCh    chan struct{}
}

// NewProcessManager creates a new ProcessManager.
func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*ServerProcess),
		stopCh:    make(chan struct{}),
	}
}

// Add registers a ServerProcess with the manager.
func (m *ProcessManager) Add(name string, sp *ServerProcess) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes[name] = sp
}

// ShutdownAll gracefully shuts down all registered processes.
func (m *ProcessManager) ShutdownAll(timeout time.Duration) {
	m.mu.Lock()
	processes := m.processes
	m.processes = make(map[string]*ServerProcess)
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, sp := range processes {
		wg.Add(1)
		go func(p *ServerProcess) {
			defer wg.Done()
			_ = p.GracefulShutdown(timeout)
		}(sp)
	}
	wg.Wait()
}

// StartSignalListener listens for interrupt signals in the background.
// On receiving a signal, it shuts down all processes and cancels the provided context.
// It returns a function that can be called to stop listening.
func (m *ProcessManager) StartSignalListener(cancel context.CancelFunc, timeout time.Duration) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			m.ShutdownAll(timeout)
			if cancel != nil {
				cancel()
			}
		case <-m.stopCh:
			signal.Stop(sigCh)
			return
		}
	}()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			close(m.stopCh)
		})
	}
}
