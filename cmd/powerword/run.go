package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"

	"powerword/internal/config"
	"powerword/internal/remote"
	"powerword/internal/review"

	"github.com/pion/webrtc/v4"
	"github.com/spf13/cobra"
)

func generateTunnelID() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate tunnel ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run the agent with a prompt or in autonomous mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Active
			if cfg == nil {
				return fmt.Errorf("configuration not loaded")
			}

			daemonFlag, _ := cmd.Flags().GetBool("daemon")
			if daemonFlag || cfg.Daemon {
				if err := runDaemonMode(cmd.Context(), cfg); err != nil {
					return err
				}
			}

			prompt := ""
			if len(args) > 0 {
				prompt = args[0]
			}

			if cfg.Autonomous {
				return review.RunAutonomousLoop(cmd.Context(), cfg)
			}

			if config.Runner == nil {
				return fmt.Errorf("no execution runner registered")
			}
			return config.Runner(cmd.Context(), cfg, prompt)
		},
	}

	cmd.Flags().Bool("daemon", false, "Start in remote daemon mode")
	return cmd
}

func runDaemonMode(ctx context.Context, cfg *config.Config) error {
	broker, err := remote.NewFirebaseBroker(ctx, cfg.FirebaseProject)
	if err != nil {
		return fmt.Errorf("failed to init firebase broker: %w", err)
	}
	tunnelID, err := generateTunnelID()
	if err != nil {
		return err
	}
	log.Printf("Starting remote daemon mode. Tunnel ID: %s", tunnelID)
	log.Printf("Waiting for dashboard connection...")

	manager, err := remote.NewWebRTCManager(cfg)
	if err != nil {
		return fmt.Errorf("failed to init webrtc manager: %w", err)
	}

	offer, err := manager.GenerateOffer()
	if err != nil {
		return fmt.Errorf("failed to generate offer: %w", err)
	}

	answer, err := broker.ExchangeSDP(ctx, tunnelID, offer)
	if err != nil {
		return fmt.Errorf("sdp exchange failed: %w", err)
	}
	log.Printf("Successfully received SDP answer from dashboard (length: %d)", len(answer))

	if err := manager.ApplyAnswer(answer); err != nil {
		return fmt.Errorf("failed to apply answer: %w", err)
	}

	log.Printf("Daemon mode WebRTC signaling complete! Waiting for channels...")

	select {
	case <-manager.TerminalReady:
		log.Printf("Terminal data channel connected!")
		cfg.OutputWriter = remote.NewDataChannelWriter(manager.TerminalChannel)
	case <-ctx.Done():
		return ctx.Err()
	}

	manager.FilesystemChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("Received filesystem command: %s", string(msg.Data))
	})
	manager.ControlChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("Received control message: %s", string(msg.Data))
	})

	return nil
}
