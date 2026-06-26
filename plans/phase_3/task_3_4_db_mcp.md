# plan: Task 3.4: SQL Database Inspector Plugin

**Status:** Complete — PR [#127](https://github.com/borch-ai/powerword/pull/127) awaiting merge
**Date Completed:** 2026-06-26
**Go Version:** 1.26.4
**Unit Test Coverage:** 91.1%

This task implements `pw-mcp-db`, a vendor-agnostic read-only database MCP server supporting
Postgres, MySQL, SQLite, DuckDB (deliberate CGO exception), and BigQuery.

## User Review Required

> [!WARNING]
> Database operations pose security risks. The implementation enforces read-only safety via:
> 1. Session-level driver settings (`_query_only` for SQLite, `SET default_transaction_read_only` for Postgres, `SET SESSION TRANSACTION READ ONLY` for MySQL).
> 2. `validateReadOnly()` keyword allowlist that also deep-scans `WITH`/`EXPLAIN` for downstream DML.
> 3. `validateIdentifier()` that restricts table names to `[a-zA-Z0-9_.]` before PRAGMA interpolation.

## Proposed Changes

### Config (`pkg/config/config.go`) [MODIFIED]

Added `DBConfig` struct with `Backend`, `DSN`, `MaxRows` (default 200), and `QueryTimeout`
(default 10s). Overridable via `POWERWORD_DB_MAX_ROWS` / `POWERWORD_DB_QUERY_TIMEOUT`.

---

### Core Plugin (`internal/plugins/db/`) [NEW]

#### [NEW] [db.go](../../internal/plugins/db/db.go)
- `Backend` interface: `ListTables`, `DescribeTable`, `QueryRead`, `ShowLocks`
- `DBService` orchestrates calls; enforces row limits and query timeouts
- `validateReadOnly()` — keyword allowlist + secondary DML scan for `WITH`/`EXPLAIN`
- `validateIdentifier()` — `[a-zA-Z0-9_.]` allowlist for PRAGMA table name interpolation
- `writeKeywords` map — `INSERT`, `UPDATE`, `DELETE`, `MERGE`, `DROP`, `CREATE`, etc.
- `NewDBService()` — bounded 30s init context for `PingContext`/`applyReadOnlySession`

#### [NEW] [sql_backend.go](../../internal/plugins/db/sql_backend.go)
- `sqlBackend` wraps `database/sql` for Postgres, MySQL, SQLite, DuckDB
- `resolveDriver()` maps dialect → driver + session-level read-only DSN modifications
- `applyReadOnlySession()` sets dialect-specific read-only pragmas/modes on connection open
- DuckDB is the deliberate CGO exception; CGO_ENABLED=1 is always required for this package

#### [NEW] [bigquery_backend.go](../../internal/plugins/db/bigquery_backend.go)
- `bigQueryBackend` wraps BQ client behind interfaces for full testability
- `realBQRowIterator` adapter exposes schema from BQ's concrete `RowIterator`

#### [NEW] [db_test.go](../../internal/plugins/db/db_test.go)
- Mock backend unit tests; `validateReadOnly` table-driven tests (allowed + rejected)
- `TestValidateReadOnly_WithDML_Rejected` — 7 cases for WITH/EXPLAIN + DML bypass
- `TestValidateIdentifier` — safe and unsafe table name cases

#### [NEW] [db_integration_test.go](../../internal/plugins/db/db_integration_test.go)
- SQLite-backed integration tests for all 4 tools
- Dialect-mismatch error paths (Postgres/MySQL PRAGMA rejected on SQLite)

---

### MCP Server (`cmd/pw-mcp-db/main.go`) [NEW]
Registers `db_list_tables`, `db_describe_table`, `db_query_read`, and `db_show_locks` MCP tools.

---

### Build (`Makefile`) [MODIFIED]
- `test` target: unmodified fast unit test cycle (db package and its tests are gated by `cgo` build constraint)
- `build` target: compiles `pw-mcp-db` plugin under CGO when requested

---

## Verification Plan

### Automated Tests
- `make lint` — 0 issues
- `make check-coverage` — 91.0% ✅ (threshold maintained after all additions)
- Pre-push hook (`make all`) passes cleanly

### Manual Verification
- Smoke test with `sqlite://file::memory:?cache=shared` — `db_list_tables`, `db_describe_table`, `db_query_read`, `db_show_locks` all succeed
- Write queries (`INSERT`, `WITH ... DELETE`) rejected with clear error message

### PR Review
- PR [#127](https://github.com/borch-ai/powerword/pull/127) opened on `feat/task-3.4-db-inspector-plugin`
- All Copilot findings addressed across iterative review rounds:
  - PRAGMA injection, `WITH`/`EXPLAIN` DML bypass, comments, and bounded init context handled.
  - SQLite and DuckDB DSN path rewriting issue resolved by parsing and rebuilding query parameters cleanly instead of applying global regex replacements.
  - Handled override checks for `_query_only` and `access_mode` parameters when conflicting values are specified.
  - Simplified the `contains` helper assertions in `internal/mcp/translator_test.go` by replacing them with the standard library `strings.Contains`.
  - Resolved semicolon splitting issues inside SQL string literals and comments by introducing `stripCommentsAndStrings` (with linter-compliant decomposed helpers to satisfy `gocognit` and `nestif` complexity rules).
  - Secured PostgreSQL and MySQL connection pools against connection recreation by appending read-only transaction session configurations directly to their connection DSNs.
  - Masked DSN credentials in ping error logs to prevent credential leakage.
  - Excluded CGO compilation in the default Makefile `test` target by forcing `CGO_ENABLED=0` to keep local unit tests fast and toolchain-agnostic.
- Review loop completed with all feedback addressed and verified.
