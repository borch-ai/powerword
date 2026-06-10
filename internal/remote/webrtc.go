package remote

import (
	"encoding/json"
	"fmt"
	"io"

	"powerword/internal/config"

	"github.com/pion/webrtc/v4"
)

// WebRTCManager manages the Pion WebRTC connection state
type WebRTCManager struct {
	pc *webrtc.PeerConnection

	TerminalChannel   *webrtc.DataChannel
	FilesystemChannel *webrtc.DataChannel
	ControlChannel    *webrtc.DataChannel

	// Channels to signal when DataChannels are opened
	TerminalReady   chan struct{}
	FilesystemReady chan struct{}
	ControlReady    chan struct{}
}

// NewWebRTCManager initializes a new WebRTC PeerConnection
func NewWebRTCManager(cfg *config.Config) (*WebRTCManager, error) {
	iceServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}

	if cfg != nil && len(cfg.TurnServers) > 0 {
		iceServer := webrtc.ICEServer{
			URLs: cfg.TurnServers,
		}
		if cfg.TurnUsername != "" {
			iceServer.Username = cfg.TurnUsername
		}
		if cfg.TurnPassword != "" {
			iceServer.Credential = cfg.TurnPassword
		}
		iceServers = append(iceServers, iceServer)
	}

	pcConfig := webrtc.Configuration{
		ICEServers: iceServers,
	}

	pc, err := webrtc.NewPeerConnection(pcConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	m := &WebRTCManager{
		pc:              pc,
		TerminalReady:   make(chan struct{}),
		FilesystemReady: make(chan struct{}),
		ControlReady:    make(chan struct{}),
	}

	// Create data channels
	ordered := true
	options := &webrtc.DataChannelInit{
		Ordered: &ordered,
	}

	termDC, err := pc.CreateDataChannel("terminal", options)
	if err != nil {
		return nil, fmt.Errorf("failed to create terminal channel: %w", err)
	}
	m.TerminalChannel = termDC

	fsDC, err := pc.CreateDataChannel("filesystem", options)
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem channel: %w", err)
	}
	m.FilesystemChannel = fsDC

	ctrlDC, err := pc.CreateDataChannel("control", options)
	if err != nil {
		return nil, fmt.Errorf("failed to create control channel: %w", err)
	}
	m.ControlChannel = ctrlDC

	// Handle open events
	termDC.OnOpen(func() {
		close(m.TerminalReady)
	})
	fsDC.OnOpen(func() {
		close(m.FilesystemReady)
	})
	ctrlDC.OnOpen(func() {
		close(m.ControlReady)
	})

	return m, nil
}

// GenerateOffer creates an SDP offer and returns it as a JSON string
func (m *WebRTCManager) GenerateOffer() (string, error) {
	offer, err := m.pc.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create offer: %w", err)
	}

	if err = m.pc.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	// Gather ICE candidates
	gatherComplete := webrtc.GatheringCompletePromise(m.pc)
	<-gatherComplete

	b, err := json.Marshal(m.pc.LocalDescription())
	if err != nil {
		return "", fmt.Errorf("failed to marshal local description: %w", err)
	}

	return string(b), nil
}

// ApplyAnswer accepts a remote SDP answer as a JSON string and applies it
func (m *WebRTCManager) ApplyAnswer(answer string) error {
	var sd webrtc.SessionDescription
	if err := json.Unmarshal([]byte(answer), &sd); err != nil {
		return fmt.Errorf("failed to unmarshal answer: %w", err)
	}

	if err := m.pc.SetRemoteDescription(sd); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	return nil
}

// Close closes the peer connection
func (m *WebRTCManager) Close() error {
	return m.pc.Close()
}

// DataChannelWriter wraps a webrtc.DataChannel to implement io.Writer
type DataChannelWriter struct {
	dc *webrtc.DataChannel
}

// NewDataChannelWriter creates a new DataChannelWriter
func NewDataChannelWriter(dc *webrtc.DataChannel) io.Writer {
	return &DataChannelWriter{dc: dc}
}

// Write implements io.Writer
func (w *DataChannelWriter) Write(p []byte) (n int, err error) {
	// Send in chunks to avoid WebRTC data channel limits (e.g. 16KB max msg size)
	chunkSize := 16384
	for i := 0; i < len(p); i += chunkSize {
		end := i + chunkSize
		if end > len(p) {
			end = len(p)
		}
		if err := w.dc.Send(p[i:end]); err != nil {
			return i, err
		}
	}
	return len(p), nil
}
