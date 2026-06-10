# plan: Task 6.6: TURN Server Fallback

**Status:** Open

This plan implements a fallback TURN configuration inside Powerword. This allows End-to-End Encrypted (E2EE) WebRTC tunnels to bypass corporate firewalls or strict NAT configurations when communicating with the Lamplighter mobile dashboard.

## User Review Required

None.

## Proposed Changes

### Configuration
#### [MODIFY] `internal/config/config.go`
- Add `TurnServers []string` to the `Config` struct.
- Add Viper bindings to load the configuration via `POWERWORD_TURN_SERVERS` or the `powerword.toml` file under the `turn_servers` key.

### WebRTC Connection
#### [MODIFY] `internal/remote/webrtc.go`
- Update `NewWebRTCManager` to accept a `*config.Config` parameter instead of having zero arguments.
- Parse the `cfg.TurnServers` list and append them as `webrtc.ICEServer` objects, alongside the default Google STUN server (`stun.l.google.com:19302`).

### Daemon Runner
#### [MODIFY] `cmd/powerword/run.go`
- In `runDaemonMode`, pass the active configuration object `cfg` into `remote.NewWebRTCManager(cfg)`.

---

## Verification Plan

### Automated Tests
- Run `go test ./internal/remote/...` to verify the `NewWebRTCManager` signature updates correctly initialize the WebRTC instance.
- Run `go build` to ensure the project compiles without errors.

### Manual Verification
- Define a dummy TURN server in `powerword.toml`:
  ```toml
  turn_servers = ["turn:fake.turn.server:3478"]
  ```
- Start the daemon and ensure it boots without initialization errors.
