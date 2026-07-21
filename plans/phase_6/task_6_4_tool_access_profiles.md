# plan: Task 6.4: Granular Tool Access Profiles

**Status:** Open (Issue #TBD)

This task implements fine-grained security profiles to control tool execution scopes (e.g., path white/blacklists for filesystem tools and shell commands).

## User Review Required

> [!WARNING]
> Restricting access profiles requires strict path resolution (handling symlinks and relative path escapes like `../`) to prevent directory traversal exploits by LLMs.

## Proposed Changes

### Security Controls

#### [MODIFY] [config.go](file://../../pkg/config/config.go)

- [ ] Add config structure for tool execution profiles, allowing configuration of path whitelist patterns.

#### [MODIFY] [guard.go](file://../../internal/loop/guard.go)

- [ ] Implement path resolution and security matching functions to validate directory boundaries.
- [ ] Implement shell command token validators to match whitelist regexes.

---

## Verification Plan

### Automated Tests

- [ ] Run `go test ./internal/loop/...` checking directory escape blockages and disallowed command blocks.

### Manual Verification

- [ ] Configure `powerword.toml` to restrict edits to a specific folder. Ask the agent to edit a file outside that folder, verifying it gets blocked.
