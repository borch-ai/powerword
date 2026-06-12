# plan: Task 3.4: SQL Database Inspector Plugin

**Status:** Open (Issue #TBD)

This task implements a native database introspection Go MCP server (`pw-mcp-db`) that connects to database nodes to let the agent query database structure, verify schemas, and diagnose connection locks.

## User Review Required

> [!WARNING]
> Database operations pose security risks. The agent must never be allowed to execute modifying queries (`INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`) in production contexts. We must enforce read-only transaction modes at the connection driver level and implement query sanitization to reject write operations, raising error notifications when unsafe requests are sent.

## Proposed Changes

### DB Plugin Component
Create a new directory `internal/plugins/db/` to contain the database client implementation.

#### [NEW] [db.go](file://../../internal/plugins/db/db.go)
- [ ] Initialize database drivers for `postgres`, `mysql`, and `sqlite3`.
- [ ] Expose the following MCP tools:
  - `db_list_tables`: return all table names in the active schema.
  - `db_describe_table`: query and return columns, types, indexes, and primary keys for a specific table.
  - `db_query_read`: execute read-only SELECT queries with strict timeouts (e.g. 5 seconds) and a limit on output row count (e.g. max 100 rows).
  - `db_show_locks`: list running queries and locking sessions (database-specific metadata queries).

#### [NEW] [db_test.go](file://../../internal/plugins/db/db_test.go)
- [ ] Run unit tests against mock database connections (e.g., using `DATA-DOG/go-sqlmock`) to verify safe execution blocks and query restrictions.

---

## Verification Plan

### Automated Tests
- [ ] Assert that execution of non-select queries is rejected with a validation error.
- [ ] Verify result serialization into markdown format for presentation to the model.
- [ ] Coverage threshold: 91% unit test coverage in `internal/plugins/db/`.

### Manual Verification
- [ ] Point the database configuration to a local SQLite file or Postgres instance.
- [ ] Run `powerword "describe the structure of the users table and fetch the first 3 entries"` and verify the database operations succeed cleanly.
