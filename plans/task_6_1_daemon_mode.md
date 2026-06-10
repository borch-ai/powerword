# plan: Task 6.1: Daemon Mode & Firebase Realtime Database Signaling

**Status:** Superseded (Issue #28)

> [!NOTE]
> This task has been superseded and fulfilled by **Task 6.5 (Issue #61)**, which implemented both the `--daemon` CLI flag and the Firebase Realtime Database Signaling Broker simultaneously.

Implement a `--daemon` CLI flag (or `powerword daemon` subcommand) in the Go runtime. This subcommand will act as a persistent connection broker that negotiates WebRTC connection handshakes with the AntigravityMobile dashboard over Firebase Realtime Database, replacing the Node.js wrapper daemon.

## User Review Required

> [!IMPORTANT]
> This feature introduces the `github.com/firebase/firebase-admin-go` dependency. We will need to distribute standard GCP service account credentials (`serviceAccountKey.json`) during execution or use application default credentials.

## Proposed Changes

### Configuration
#### [MODIFY] [powerword.toml](file:///Users/human/code/powerword/powerword.toml)
- Add configuration settings under a `[remote]` block to parse the Firebase Realtime Database URL and the path to local credentials.

### CLI Command Routing
#### [MODIFY] [cmd/powerword/main.go](file:///Users/human/code/powerword/cmd/powerword/main.go)
- Add command-line flag parser support for `--daemon` or `daemon`.

#### [NEW] [internal/remote/firebase.go](file:///Users/human/code/powerword/internal/remote/firebase.go)
- Integrate the Firebase Go Admin SDK (`github.com/firebase/firebase-admin-go`).
- Read configurations and establish dynamic listeners on database path `/tunnels/{tunnel_id}`.
- Generate a unique pairing code (e.g. 6-digit PIN) or QR code string, write the initial SDP offer to the `/tunnels/{id}/offer` node, and subscribe to answer updates.

---

## Verification Plan

### Automated Tests
- Mock the Firebase Realtime Database client in `internal/remote/firebase_test.go` to assert correct listener triggers and JSON data parsing.

### Manual Verification
- Compile powerword: `make build`.
- Run `./bin/powerword --daemon`.
- Verify the CLI prints a connection status and successfully registers a new tunnel node in the Firebase console.
