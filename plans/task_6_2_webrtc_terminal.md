# Task 6.2: Pion WebRTC Data Channel Server

**Status:** Open (Issue #29)

Integrate the pure Go Pion WebRTC library into the Powerword binary to stream terminal stdout/stderr logs and accept filesystem commands over end-to-end encrypted (E2EE) WebRTC data channels directly to the mobile dashboard.

## User Review Required

> [!WARNING]
> This feature introduces `github.com/pion/webrtc/v3`. Since WebRTC requires managing ICE candidates and connection states, this will be a relatively heavy dependency. 

## Proposed Changes

### WebRTC Layer
#### [NEW] [internal/remote/webrtc.go](file:///Users/human/code/powerword/internal/remote/webrtc.go)
- Integrate `github.com/pion/webrtc/v3` library.
- Implement a `PeerConnection` state manager.
- Declare three WebRTC data channels: `terminal`, `filesystem`, and `control` to match the dashboard expectations.

### Terminal Logging Pipe
#### [MODIFY] [internal/loop/loop.go](file:///Users/human/code/powerword/internal/loop/loop.go)
- Detect if remote daemon mode is active.
- If active, redirect command output streams to write chunked byte slices onto the WebRTC `terminal` data channel instead of standard stdout.

---

## Verification Plan

### Automated Tests
- Mock Pion connection events in `webrtc_test.go` using in-memory virtual networks to assert stdout data routing and data channel creation lifecycles.

### Manual Verification
- Boot Powerword daemon, pair it with the AntigravityMobile dashboard frontend, and verify terminal characters render on the mobile console in real time without lag.
