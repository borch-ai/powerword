package mcp

import (
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
	defer m.mu.Unlock()

	var wg sync.WaitGroup
	for _, sp := range m.processes {
		wg.Add(1)
		go func(p *ServerProcess) {
			defer wg.Done()
			_ = p.GracefulShutdown(timeout)
		}(sp)
	}
	wg.Wait()

	// Clear the processes map after shutting down
	m.processes = make(map[string]*ServerProcess)
}

// StartSignalListener listens for interrupt signals in the background.
// On receiving a signal, it shuts down all processes and exits the program.
// It returns a function that can be called to stop listening.
func (m *ProcessManager) StartSignalListener(timeout time.Duration) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			m.ShutdownAll(timeout)
			os.Exit(0)
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
