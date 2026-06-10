package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"powerword/internal/config"
)

func TestRunCmd_NotLoaded(t *testing.T) {
	origConfig := config.Active
	config.Active = nil
	defer func() { config.Active = origConfig }()

	cmd := newRunCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "configuration not loaded") {
		t.Errorf("expected config not loaded error, got %v", err)
	}
}

func TestRunCmd_NoRunner(t *testing.T) {
	origConfig := config.Active
	config.Active = &config.Config{}
	defer func() { config.Active = origConfig }()

	origRunner := config.Runner
	config.Runner = nil
	defer func() { config.Runner = origRunner }()

	cmd := newRunCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "no execution runner registered") {
		t.Errorf("expected no execution runner error, got %v", err)
	}
}

func TestRunCmd_StandardRunner(t *testing.T) {
	origConfig := config.Active
	config.Active = &config.Config{}
	defer func() { config.Active = origConfig }()

	origRunner := config.Runner
	called := false
	config.Runner = func(ctx context.Context, cfg *config.Config, prompt string) error {
		called = true
		if prompt != "hello" {
			t.Errorf("expected prompt 'hello', got '%s'", prompt)
		}
		return nil
	}
	defer func() { config.Runner = origRunner }()

	cmd := newRunCmd()
	err := cmd.RunE(cmd, []string{"hello"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Errorf("expected Runner to be called")
	}
}

func TestGenerateTunnelID(t *testing.T) {
	id1, err := generateTunnelID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id2, err := generateTunnelID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id1) != 6 {
		t.Errorf("expected length 6, got %d", len(id1))
	}
	if id1 == id2 {
		t.Errorf("expected different IDs, got same: %s", id1)
	}
}

func TestRunCmd_Daemon(t *testing.T) {
	origConfig := config.Active
	config.Active = &config.Config{Daemon: true}
	defer func() { config.Active = origConfig }()

	cmd := newRunCmd()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{})
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}
