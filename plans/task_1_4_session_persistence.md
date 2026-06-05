# Task 1.4: Session Persistence & Chat History Management

Implement local session management and persistence schemas to store chat transcript histories, model contexts, system prompts, and configuration states. Enable resuming previous CLI conversations via structured CLI flags.

## User Review Required

> [!NOTE]
> Database Strategy: We will support lightweight flat JSON file streams inside `~/.local/share/powerword/sessions/` for maximum portability and fast local IO.

## Proposed Changes

### Local Storage & Context Serializers

#### [NEW] [session.go](file:///Users/human/code/powerword/internal/loop/session.go)
- Defines structures:
  - `Session` mapping session ID, timestamp, model config, and history.
  - `HistoryStore` managing session lookup, save, and listing.
- Implements:
  - `SaveSession(id string, history []llm.Message) error`
  - `LoadSession(id string) ([]llm.Message, error)`
  - `ListSessions() ([]Session, error)`

#### [MODIFY] [root.go](file:///Users/human/code/powerword/internal/config/root.go)
- Adds CLI flags:
  - `--session <id>` (creates or resumes a conversation with the specified ID).
  - `--list-sessions` (lists recent conversations, dates, and token sizing).

---

## Verification Plan

### Automated Tests
- Test serialization and deserialization of message chains (including tool calls and results) to ensure JSON streams preserve structures.
- Verify listing empty directory states handles directories gracefully.

### Manual Verification
- Execute a prompt: `$ powerword --session="test-convo" "Hi, remember the code word 'xyz'"`
- Run follow-up prompt: `$ powerword --session="test-convo" "What was the code word?"`
- Confirm model successfully retrieves context.

---

## Final Implementation Details
- **Final Configurations:** 
  - Sessions are saved as JSON files in `~/.local/share/powerword/sessions/<id>.json`.
  - Added CLI flags `--session <id>` (to resume or create a named session) and `--list-sessions` (to view all sessions).
  - If `--session` is omitted, the session is ephemeral and not persisted to disk.
- **Testing Updates:** Unit tests added in `session_test.go` and `loop_test.go` covering edge cases, directory creation errors, read-only file scenarios, and valid workflows. Overall coverage is >= 91%.
- **Go Version:** Go 1.23+
