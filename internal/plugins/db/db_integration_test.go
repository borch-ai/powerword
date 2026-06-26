//go:build integration

package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/borch-ai/powerword/pkg/config"

	// Ensure the SQLite driver is registered for the seeding helper.
	_ "modernc.org/sqlite"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// seedSQLiteFile opens a writable SQLite connection (no _query_only flag) and
// creates + populates a 'users' table for integration tests.
func seedSQLiteFile(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("seed: sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id   INTEGER PRIMARY KEY,
			name TEXT    NOT NULL,
			age  INTEGER
		)`,
		`INSERT INTO users (name, age) VALUES ('Alice', 30)`,
		`INSERT INTO users (name, age) VALUES ('Bob', 25)`,
		`INSERT INTO users (name, age) VALUES ('Carol', 35)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed stmt %q: %v", stmt, err)
		}
	}
}

// newSQLiteService creates a seeded SQLite file and returns a DBService backed
// by a real read-only SQLite connection.
func newSQLiteService(t *testing.T) *DBService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	seedSQLiteFile(t, dbPath)

	svc, err := NewDBService(&config.DBConfig{
		Backend:      "sqlite",
		DSN:          dbPath,
		MaxRows:      10,
		QueryTimeout: "5s",
	})
	if err != nil {
		t.Fatalf("NewDBService(sqlite): %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// ---------------------------------------------------------------------------
// Integration tests — SQLite (pure Go, no external service required)
// ---------------------------------------------------------------------------

func TestIntegration_SQLite_ListTables(t *testing.T) {
	svc := newSQLiteService(t)

	tables, err := svc.ListTables(context.Background())
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	found := false
	for _, tbl := range tables {
		if tbl == "users" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'users' in table list, got: %v", tables)
	}
}

func TestIntegration_SQLite_DescribeTable(t *testing.T) {
	svc := newSQLiteService(t)

	cols, err := svc.DescribeTable(context.Background(), "users")
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	if len(cols) == 0 {
		t.Fatal("expected non-empty column list")
	}
	nameFound := false
	for _, col := range cols {
		if col.Name == "name" {
			nameFound = true
		}
	}
	if !nameFound {
		t.Errorf("expected column 'name' in schema, got: %+v", cols)
	}
}

func TestIntegration_SQLite_QueryRead_OK(t *testing.T) {
	svc := newSQLiteService(t)

	res, err := svc.QueryRead(context.Background(), "SELECT id, name FROM users")
	if err != nil {
		t.Fatalf("QueryRead: %v", err)
	}
	if res.Count == 0 {
		t.Error("expected at least one row")
	}
	if len(res.Columns) == 0 {
		t.Error("expected column names to be populated")
	}
}

func TestIntegration_SQLite_QueryRead_RowLimitRespected(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "limit.db")
	seedSQLiteFile(t, dbPath)

	backend, err := newSQLBackend(context.Background(), "sqlite", dbPath)
	if err != nil {
		t.Fatalf("newSQLBackend: %v", err)
	}
	// maxRows = 1; 3 rows seeded → only 1 should be returned.
	svc := NewDBServiceWithBackend(backend, 1, 5*time.Second)
	t.Cleanup(func() { _ = svc.Close() })

	res, err := svc.QueryRead(context.Background(), "SELECT * FROM users")
	if err != nil {
		t.Fatalf("QueryRead: %v", err)
	}
	if res.Count > 1 {
		t.Errorf("expected at most 1 row, got %d", res.Count)
	}
}

func TestIntegration_SQLite_QueryRead_WriteRejected(t *testing.T) {
	svc := newSQLiteService(t)

	_, err := svc.QueryRead(context.Background(), "INSERT INTO users (name) VALUES ('Hacker')")
	if err == nil {
		t.Fatal("expected write query to be rejected, got nil")
	}
}

func TestIntegration_SQLite_ShowLocks(t *testing.T) {
	svc := newSQLiteService(t)

	// SQLite returns an empty slice — no lock visibility at the SQL level.
	locks, err := svc.ShowLocks(context.Background())
	if err != nil {
		t.Fatalf("ShowLocks: %v", err)
	}
	// Empty is correct for SQLite; just verify no error.
	_ = locks
}

func TestIntegration_SQLite_Close(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "close.db")
	seedSQLiteFile(t, dbPath)

	svc, err := NewDBService(&config.DBConfig{
		Backend:      "sqlite",
		DSN:          dbPath,
		MaxRows:      10,
		QueryTimeout: "5s",
	})
	if err != nil {
		t.Fatalf("NewDBService: %v", err)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// resolveDriver — all four SQL dialects
// ---------------------------------------------------------------------------

func TestIntegration_ResolveDriver_SQLite(t *testing.T) {
	name, dsn := resolveDriver("sqlite", "/tmp/test.db")
	if name != "sqlite" {
		t.Errorf("expected driver 'sqlite', got %q", name)
	}
	if dsn == "/tmp/test.db" {
		t.Error("expected _query_only to be appended to SQLite DSN")
	}
}

func TestIntegration_ResolveDriver_SQLiteAlreadyHasParam(t *testing.T) {
	name, dsn := resolveDriver("sqlite", "/tmp/test.db?cache=shared&_query_only=true")
	if name != "sqlite" {
		t.Errorf("expected driver 'sqlite', got %q", name)
	}
	// Should not double-append _query_only.
	count := 0
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == '_' && i+12 <= len(dsn) && dsn[i:i+12] == "_query_only=" {
			count++
		}
	}
	if count > 1 {
		t.Errorf("_query_only appears more than once in DSN: %q", dsn)
	}
}

func TestIntegration_ResolveDriver_DuckDB(t *testing.T) {
	name, dsn := resolveDriver("duckdb", "/tmp/test.duckdb")
	if name != "duckdb" {
		t.Errorf("expected driver 'duckdb', got %q", name)
	}
	if dsn == "/tmp/test.duckdb" {
		t.Error("expected access_mode to be appended to DuckDB DSN")
	}
}

func TestIntegration_ResolveDriver_Postgres(t *testing.T) {
	name, dsn := resolveDriver("postgres", "postgres://localhost/db")
	if name != "postgres" {
		t.Errorf("expected driver 'postgres', got %q", name)
	}
	if dsn != "postgres://localhost/db" {
		t.Errorf("postgres DSN should be unchanged, got %q", dsn)
	}
}

func TestIntegration_ResolveDriver_MySQL(t *testing.T) {
	name, dsn := resolveDriver("mysql", "user:pass@tcp(localhost)/db")
	if name != "mysql" {
		t.Errorf("expected driver 'mysql', got %q", name)
	}
	if dsn != "user:pass@tcp(localhost)/db" {
		t.Errorf("mysql DSN should be unchanged, got %q", dsn)
	}
}

// ---------------------------------------------------------------------------
// describeTableQuery — dialect coverage
// ---------------------------------------------------------------------------

func TestIntegration_DescribeTableQuery_AllDialects(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{"postgres", "mysql", "sqlite", "duckdb"} {
		query, args := describeTableQuery(dialect, "users")
		if query == "" {
			t.Errorf("describeTableQuery(%q) returned empty query", dialect)
		}
		// SQLite embeds the table name directly in the query (no bound params).
		if dialect != "sqlite" && len(args) == 0 {
			t.Errorf("describeTableQuery(%q) returned no args", dialect)
		}
	}
}

// ---------------------------------------------------------------------------
// applyReadOnlySession — SQLite no-op path
// ---------------------------------------------------------------------------

func TestIntegration_ApplyReadOnlySession_SQLite_IsNoop(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "session.db")
	seedSQLiteFile(t, dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// applyReadOnlySession for sqlite is a no-op; must return nil error.
	if err := applyReadOnlySession(context.Background(), db, "sqlite"); err != nil {
		t.Errorf("applyReadOnlySession(sqlite): expected nil, got %v", err)
	}
	if err := applyReadOnlySession(context.Background(), db, "duckdb"); err != nil {
		t.Errorf("applyReadOnlySession(duckdb): expected nil, got %v", err)
	}
}

func TestIntegration_ApplyReadOnlySession_PostgresCmd_OnSQLite_Fails(t *testing.T) {
	// Run the postgres session command on a SQLite connection.
	// SQLite rejects "SET SESSION CHARACTERISTICS..." covering the ExecContext error branch.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := applyReadOnlySession(context.Background(), db, "postgres"); err == nil {
		t.Error("expected error running postgres session command on SQLite, got nil")
	}
}

func TestIntegration_ApplyReadOnlySession_MySQLCmd_OnSQLite_Fails(t *testing.T) {
	// Run the mysql session command on a SQLite connection — should fail.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := applyReadOnlySession(context.Background(), db, "mysql"); err == nil {
		t.Error("expected error running mysql session command on SQLite, got nil")
	}
}

func TestIntegration_SQLite_ShowLocks_PostgresDialect_Error(t *testing.T) {
	// Construct a sqlBackend with sqlite connection but postgres dialect,
	// so ShowLocks issues the pg_stat_activity query (supported=true) which
	// SQLite will reject — covering the QueryContext error branch in ShowLocks.
	dbPath := filepath.Join(t.TempDir(), "locks.db")
	seedSQLiteFile(t, dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	b := &sqlBackend{db: db, dialect: "postgres"}
	defer func() { _ = b.Close() }()

	_, err = b.ShowLocks(context.Background())
	if err == nil {
		t.Error("expected error when running pg_stat_activity query on SQLite, got nil")
	}
}

func TestIntegration_SQLite_ListTables_PostgresDialect_Error(t *testing.T) {
	// Use postgres dialect on a SQLite connection — pg_catalog query should fail,
	// covering the error return in ListTables.
	dbPath := filepath.Join(t.TempDir(), "lt.db")
	seedSQLiteFile(t, dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	b := &sqlBackend{db: db, dialect: "postgres"}
	defer func() { _ = b.Close() }()

	_, err = b.ListTables(context.Background())
	if err == nil {
		t.Error("expected error when running postgres list-tables query on SQLite, got nil")
	}
}

func TestIntegration_SQLite_DescribeTable_PostgresDialect_Error(t *testing.T) {
	// Use postgres dialect on a SQLite connection — INFORMATION_SCHEMA query fails,
	// covering the error return in DescribeTable.
	dbPath := filepath.Join(t.TempDir(), "desc.db")
	seedSQLiteFile(t, dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	b := &sqlBackend{db: db, dialect: "postgres"}
	defer func() { _ = b.Close() }()

	_, err = b.DescribeTable(context.Background(), "users")
	if err == nil {
		t.Error("expected error when running postgres describe query on SQLite, got nil")
	}
}

func TestIntegration_SQLite_QueryRead_PostgresDialect_ErrorPath(t *testing.T) {
	// Construct a sqlBackend directly so we can inject dialect=postgres on SQLite.
	// Then call QueryRead which wraps in a subquery — should succeed with SQLite
	// since the subquery wrapping is compatible.  This exercises the QueryRead scan loop.
	dbPath := filepath.Join(t.TempDir(), "qr.db")
	seedSQLiteFile(t, dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	b := &sqlBackend{db: db, dialect: "postgres"}
	defer func() { _ = b.Close() }()

	// A valid SELECT wrapped in a subquery works on SQLite too.
	res, err := b.QueryRead(context.Background(), "SELECT * FROM users", 2)
	if err != nil {
		t.Fatalf("QueryRead: %v", err)
	}
	if res.Count == 0 {
		t.Error("expected rows")
	}
}

// ---------------------------------------------------------------------------
// BigQuery ShowLocks — advisory path (no real BQ connection needed)
// ---------------------------------------------------------------------------

func TestIntegration_BigQuery_ShowLocks_Advisory(t *testing.T) {
	b := &bigQueryBackend{}
	locks, err := b.ShowLocks(context.Background())
	if err != nil {
		t.Fatalf("BigQuery ShowLocks: %v", err)
	}
	if len(locks) == 0 {
		t.Error("expected advisory LockInfo entry")
	}
	if locks[0].State == "" {
		t.Error("expected non-empty advisory State message")
	}
}

// ---------------------------------------------------------------------------
// injectBQLimit — variant coverage
// ---------------------------------------------------------------------------

func TestIntegration_InjectBQLimit_Variants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		query    string
		limit    int
		wantSame bool // true if query should be unchanged (already has LIMIT)
	}{
		{"SELECT * FROM t", 5, false},
		{"SELECT * FROM t LIMIT 3", 5, true},
		{"SELECT * FROM t limit 3", 5, true},
	}
	for _, tc := range cases {
		out := injectBQLimit(tc.query, tc.limit)
		if tc.wantSame && out != tc.query {
			t.Errorf("injectBQLimit(%q): expected unchanged, got %q", tc.query, out)
		}
		if !tc.wantSame && out == tc.query {
			t.Errorf("injectBQLimit(%q): expected LIMIT injection, got unchanged", tc.query)
		}
	}
}

// ---------------------------------------------------------------------------
// NewDBService config validation paths
// ---------------------------------------------------------------------------

func TestIntegration_NewDBService_EmptyDSN(t *testing.T) {
	_, err := NewDBService(&config.DBConfig{
		Backend:      "sqlite",
		DSN:          "",
		MaxRows:      10,
		QueryTimeout: "5s",
	})
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func TestIntegration_NewDBService_InvalidTimeout_UsesDefault(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "timeout.db")
	seedSQLiteFile(t, dbPath)

	// Invalid timeout string should fall back to 10s default without error.
	svc, err := NewDBService(&config.DBConfig{
		Backend:      "sqlite",
		DSN:          dbPath,
		MaxRows:      10,
		QueryTimeout: "not-a-duration",
	})
	if err != nil {
		t.Fatalf("expected fallback to default timeout, got error: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if svc.queryTimeout != 10*time.Second {
		t.Errorf("expected 10s fallback timeout, got %v", svc.queryTimeout)
	}
}

func TestIntegration_NewDBService_ZeroMaxRows_UsesDefault(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "maxrows.db")
	seedSQLiteFile(t, dbPath)

	svc, err := NewDBService(&config.DBConfig{
		Backend:      "sqlite",
		DSN:          dbPath,
		MaxRows:      0, // should fallback to 200
		QueryTimeout: "5s",
	})
	if err != nil {
		t.Fatalf("NewDBService: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if svc.maxRows != 200 {
		t.Errorf("expected maxRows=200 fallback, got %d", svc.maxRows)
	}
}
func TestIntegration_NewDBService_BackendInitFails(t *testing.T) {
	// /dev/null is not a directory — SQLite cannot create a database file there.
	// This covers newSQLBackend's PingContext error branch (db.Close + return error)
	// and NewDBService's "failed to initialise backend" error return.
	_, err := NewDBService(&config.DBConfig{
		Backend:      "sqlite",
		DSN:          "/dev/null/test.db",
		QueryTimeout: "5s",
		MaxRows:      10,
	})
	if err == nil {
		t.Fatal("expected error for invalid SQLite path /dev/null/test.db")
	}
}

func TestIntegration_NewSQLBackend_PingFails(t *testing.T) {
	// /dev/null/test.db — directory component is a device node, not a dir.
	// sql.Open is lazy (succeeds), but PingContext fails — covering that branch.
	_, err := newSQLBackend(context.Background(), "sqlite", "/dev/null/test.db")
	if err == nil {
		t.Fatal("expected ping failure for invalid SQLite path")
	}
}
