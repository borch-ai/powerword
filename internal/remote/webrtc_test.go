package remote

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"powerword/internal/config"

	"github.com/pion/webrtc/v4"
)

func setupDaemon(t *testing.T) (*WebRTCManager, string) {
	manager, err := NewWebRTCManager(nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	offerJSON, err := manager.GenerateOffer()
	if err != nil {
		t.Fatalf("failed to generate offer: %v", err)
	}
	return manager, offerJSON
}

func setupDashboard(t *testing.T, daemonOfferJSON string) (*webrtc.PeerConnection, chan *webrtc.DataChannel, string) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	}
	dashboardPC, err := webrtc.NewPeerConnection(config)
	if err != nil {
		t.Fatalf("failed to create dashboard pc: %v", err)
	}

	dashboardTerminalChan := make(chan *webrtc.DataChannel, 1)

	dashboardPC.OnDataChannel(func(d *webrtc.DataChannel) {
		if d.Label() == "terminal" {
			d.OnOpen(func() {
				dashboardTerminalChan <- d
			})
		}
	})

	var daemonOffer webrtc.SessionDescription
	if errUn := json.Unmarshal([]byte(daemonOfferJSON), &daemonOffer); errUn != nil {
		t.Fatalf("failed to unmarshal offer: %v", errUn)
	}

	if errSet := dashboardPC.SetRemoteDescription(daemonOffer); errSet != nil {
		t.Fatalf("failed to set remote desc: %v", errSet)
	}

	dashboardAnswer, errAns := dashboardPC.CreateAnswer(nil)
	if errAns != nil {
		t.Fatalf("failed to create answer: %v", errAns)
	}

	gatherComplete := webrtc.GatheringCompletePromise(dashboardPC)
	if errSetL := dashboardPC.SetLocalDescription(dashboardAnswer); errSetL != nil {
		t.Fatalf("failed to set local desc: %v", errSetL)
	}
	<-gatherComplete

	dashboardAnswerJSON, errMarsh := json.Marshal(dashboardPC.LocalDescription())
	if errMarsh != nil {
		t.Fatalf("failed to marshal answer: %v", errMarsh)
	}

	return dashboardPC, dashboardTerminalChan, string(dashboardAnswerJSON)
}

func TestWebRTCManager(t *testing.T) {
	manager, offerJSON := setupDaemon(t)
	defer func() { _ = manager.Close() }()

	dashboardPC, dashboardTerminalChan, dashboardAnswerJSON := setupDashboard(t, offerJSON)
	defer func() { _ = dashboardPC.Close() }()

	if err := manager.ApplyAnswer(dashboardAnswerJSON); err != nil {
		t.Fatalf("failed to apply answer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	select {
	case <-manager.TerminalReady:
	case <-ctx.Done():
		t.Fatalf("timeout waiting for daemon terminal channel")
	}

	var dashboardTerminal *webrtc.DataChannel
	select {
	case dashboardTerminal = <-dashboardTerminalChan:
	case <-ctx.Done():
		t.Fatalf("timeout waiting for dashboard terminal channel")
	}

	writer := NewDataChannelWriter(manager.TerminalChannel)
	msg := []byte("hello from daemon")

	receivedMsg := make(chan string, 1)
	dashboardTerminal.OnMessage(func(m webrtc.DataChannelMessage) {
		receivedMsg <- string(m.Data)
	})

	n, err := writer.Write(msg)
	if err != nil {
		t.Fatalf("failed to write to terminal channel: %v", err)
	}
	if n != len(msg) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(msg), n)
	}

	select {
	case r := <-receivedMsg:
		if r != string(msg) {
			t.Fatalf("expected message %q, got %q", string(msg), r)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for message on dashboard")
	}
}

func TestWebRTCManager_Errors(t *testing.T) {
	manager, err := NewWebRTCManager(nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	// Test ApplyAnswer with invalid JSON
	if err := manager.ApplyAnswer("invalid json"); err == nil {
		t.Errorf("expected error on invalid json, got nil")
	}

	// Test ApplyAnswer with valid JSON but invalid SDP
	if err := manager.ApplyAnswer(`{"type":"answer", "sdp":"invalid"}`); err == nil {
		t.Errorf("expected error on invalid sdp content, got nil")
	}

	// Close the manager's peer connection to trigger a GenerateOffer error
	_ = manager.Close()
	if _, err := manager.GenerateOffer(); err == nil {
		t.Errorf("expected error when generating offer on closed connection")
	}
}

func TestDataChannelWriter_Error(t *testing.T) {
	manager, err := NewWebRTCManager(nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	writer := NewDataChannelWriter(manager.TerminalChannel)

	// Close the manager to force the data channel write to fail
	_ = manager.Close()

	_, err = writer.Write([]byte("this should fail"))
	if err == nil {
		t.Errorf("expected error writing to closed data channel")
	}
}

func TestNewWebRTCManager_Config(t *testing.T) {
	cfg := &config.Config{
		TurnServers:  []string{"turn:fake.turn.server:3478"},
		TurnUsername: "user",
		TurnPassword: "password",
	}

	manager, err := NewWebRTCManager(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	pcConfig := manager.pc.GetConfiguration()

	// We expect 2 ICE servers: the default stun and the custom turn
	if len(pcConfig.ICEServers) != 2 {
		t.Errorf("expected 2 ICE servers, got %d", len(pcConfig.ICEServers))
	}

	foundTurn := false
	for _, s := range pcConfig.ICEServers {
		for _, url := range s.URLs {
			if url == "turn:fake.turn.server:3478" {
				foundTurn = true
			}
		}
	}

	if !foundTurn {
		t.Errorf("expected to find custom turn server in ICE servers")
	}
}
