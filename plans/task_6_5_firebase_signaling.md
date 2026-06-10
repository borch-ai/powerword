# feat: Firebase RTDB Signaling Broker for WebRTC

This plan fleshes out the implementation strategy for [Issue #61](https://github.com/borch-ai/powerword/issues/61) to add a Firebase Realtime Database signaling broker, enabling the Lamplighter mobile dashboard to dynamically pair with the daemon over the internet.

## Description
We are building the remote dashboard (Lamplighter) and need the Powerword daemon to support establishing a WebRTC PeerConnection. To do this without requiring a local network connection, we need a signaling broker.

The Firebase Realtime Database signaling broker in the Powerword CLI will:
1. Initialize the Firebase Admin SDK.
2. Expose a command (`powerword run --daemon`) which hooks into the daemon startup.
3. Generate a unique short Tunnel ID.
4. Write the daemon's SDP offer to `/tunnels/{tunnelId}/offer`.
5. Listen for the dashboard's SDP answer at `/tunnels/{tunnelId}/answer`.

## User Review Required
None

## Proposed Changes

The codebase currently contains a functional draft of the WebRTC Daemon Mode (`internal/remote/firebase.go` and hooks in `cmd/powerword/run.go`).

### Tests & Coverage (internal/remote)
**`internal/remote/firebase_test.go`**
Comprehensive unit tests have been added for `NewFirebaseBroker` and `ExchangeSDP`. 
To facilitate this, the `db.Client` and `db.Ref` dependencies were wrapped in internal interfaces (`rtdbClient` and `rtdbRef`), allowing us to inject a mock in-memory RTDB client for testing.

### Daemon Run Hooks (cmd/powerword)
**`cmd/powerword/run.go`**
The `--daemon` flag initializes the `FirebaseBroker` and initiates an SDP exchange.

**`cmd/powerword/run_test.go`**
Tests were added to cover the `--daemon` code path and the internal `generateTunnelID` function to ensure high coverage.

## Verification Plan
- Run `make check-coverage` to ensure `internal/remote` and `cmd/powerword` exceed 91%.
- Run `make lint` and `make vuln` to ensure code quality and security.
