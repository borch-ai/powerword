# plan: Task 6.6: TURN Server Fallback

**Status:** Complete

This plan implements a fallback TURN configuration inside Powerword. This allows End-to-End Encrypted (E2EE) WebRTC tunnels to bypass corporate firewalls or strict NAT configurations when communicating with the Lamplighter mobile dashboard.

## User Review Required

None.

## Proposed Changes

### Configuration
#### [MODIFY] `internal/config/config.go`
- Add `TurnServers []string`, `TurnUsername string`, and `TurnPassword string` to the `Config` struct.
- Add Viper bindings to load the configuration via `POWERWORD_TURN_SERVERS`, `POWERWORD_TURN_USERNAME`, and `POWERWORD_TURN_PASSWORD`.

### WebRTC Connection
#### [MODIFY] `internal/remote/webrtc.go`
- Update `NewWebRTCManager` to accept a `*config.Config` parameter instead of having zero arguments.
- Parse the `cfg.TurnServers` list and append them as `webrtc.ICEServer` objects, alongside the default Google STUN server (`stun.l.google.com:19302`).
- Attach `TurnUsername` and `TurnPassword` to the `ICEServer` configuration natively required by Pion v4.

### Daemon Runner
#### [MODIFY] `cmd/powerword/run.go`
- In `runDaemonMode`, pass the active configuration object `cfg` into `remote.NewWebRTCManager(cfg)`.
- Ensure WebRTC Manager closes on error/shutdown to avoid goroutine leaks.

---

## Verification Plan

### Automated Tests
- Run `go test ./internal/remote/...` to verify the `NewWebRTCManager` signature updates correctly initialize the WebRTC instance.
- Run `go build` to ensure the project compiles without errors.

### Manual Verification
- Define a dummy TURN server in `powerword.toml`:
  ```toml
  turn_servers = ["turn:fake.turn.server:3478"]
  turn_username = "test"
  turn_password = "test"
  ```
- Start the daemon and ensure it boots without initialization errors.

## Final Implementation Notes

- **Go Version:** Go 1.23+
- **Configuration:** Updated `internal/config/config.go` to explicitly include username and password credentials due to strict enforcement in Pion v4 for `turn:` URLs.
- **Testing:** Integration tests pass properly. The duplicate Viper bindings were resolved. Coverage maintained at 91.00%.
