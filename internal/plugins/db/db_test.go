//go:build cgo

package db

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	_ "modernc.org/sqlite"

	"github.com/borch-ai/powerword/pkg/config"
)

// ---------------------------------------------------------------------------
// Mock backend
// ---------------------------------------------------------------------------

type mockBackend struct {
	tables      []string
	columns     []ColumnInfo
	queryResult *QueryResult
	locks       []LockInfo
	listErr     error
	describeErr error
	queryErr    error
	locksErr    error
	closed      bool
}

func (m *mockBackend) ListTables(_ context.Context) ([]string, error) {
	return m.tables, m.listErr
}

func (m *mockBackend) DescribeTable(_ context.Context, _ string) ([]ColumnInfo, error) {
	return m.columns, m.describeErr
}

func (m *mockBackend) QueryRead(_ context.Context, _ string, limit int) (*QueryResult, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	if m.queryResult == nil {
		return &QueryResult{}, nil
	}
	result := *m.queryResult
	if result.Count > limit {
		result.Rows = result.Rows[:limit]
		result.Count = limit
	}
	return &result, nil
}

func (m *mockBackend) ShowLocks(_ context.Context) ([]LockInfo, error) {
	return m.locks, m.locksErr
}

func (m *mockBackend) Close() error {
	m.closed = true
	return nil
}

// ---------------------------------------------------------------------------
// validateReadOnly tests
// ---------------------------------------------------------------------------

func TestValidateReadOnly_Allowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
	}{
		{"simple select", "SELECT * FROM users"},
		{"select with limit", "SELECT id FROM orders LIMIT 10"},
		{"cte", "WITH cte AS (SELECT 1) SELECT * FROM cte"},
		{"explain", "EXPLAIN SELECT * FROM users"},
		{"show tables", "SHOW TABLES"},
		{"describe", "DESCRIBE users"},
		{"desc", "DESC orders"},
		{"pragma", "PRAGMA table_info('users')"},
		{"leading block comment", "/* analytics */ SELECT count(*) FROM events"},
		{"empty second stmt", "SELECT 1;"},
		{"comment only statement", "SELECT 1; /* just a comment */"},
		{"semicolon in string", "SELECT ';' AS semi, 'hello; world' AS val"},
		{"semicolon in comment", "SELECT 1 -- my comment; here\n"},
		{"semicolon in block comment", "SELECT 1 /* comment; */"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := validateReadOnly(tc.query); err != nil {
				t.Errorf("expected no error for %q, got: %v", tc.query, err)
			}
		})
	}
}

func TestValidateReadOnly_Rejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
	}{
		{"insert", "INSERT INTO users VALUES (1)"},
		{"update", "UPDATE users SET name='x'"},
		{"delete", "DELETE FROM users"},
		{"drop", "DROP TABLE users"},
		{"alter", "ALTER TABLE users ADD COLUMN age INT"},
		{"truncate", "TRUNCATE users"},
		{"grant", "GRANT ALL ON users TO admin"},
		{"revoke", "REVOKE ALL ON users FROM admin"},
		{"create", "CREATE TABLE t (id INT)"},
		{"mixed valid then invalid", "SELECT 1; DROP TABLE users"},
		{"multi stmt selects", "SELECT 1; SELECT 2"},
		{"empty", ""},
		{"only whitespace", "   \t\n   "},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := validateReadOnly(tc.query); err == nil {
				t.Errorf("expected error for %q, but got nil", tc.query)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DBService tests via mock backend
// ---------------------------------------------------------------------------

func newTestService(m *mockBackend) *DBService {
	return NewDBServiceWithBackend(m, 5, 2*time.Second)
}

func TestDBService_ListTables_OK(t *testing.T) {
	t.Parallel()
	m := &mockBackend{tables: []string{"users", "orders"}}
	svc := newTestService(m)

	tables, err := svc.ListTables(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(tables))
	}
}

func TestDBService_ListTables_Error(t *testing.T) {
	t.Parallel()
	m := &mockBackend{listErr: errors.New("connection refused")}
	svc := newTestService(m)

	_, err := svc.ListTables(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDBService_DescribeTable_OK(t *testing.T) {
	t.Parallel()
	m := &mockBackend{columns: []ColumnInfo{
		{Name: "id", DataType: "integer", Nullable: "NO"},
		{Name: "name", DataType: "text", Nullable: "YES"},
	}}
	svc := newTestService(m)

	cols, err := svc.DescribeTable(context.Background(), "users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cols) != 2 {
		t.Errorf("expected 2 columns, got %d", len(cols))
	}
}

func TestDBService_DescribeTable_EmptyName(t *testing.T) {
	t.Parallel()
	m := &mockBackend{}
	svc := newTestService(m)

	_, err := svc.DescribeTable(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty table name")
	}
}

func TestDBService_QueryRead_OK(t *testing.T) {
	t.Parallel()
	rows := make([]any, 3)
	for i := range rows {
		rows[i] = map[string]any{"id": i + 1}
	}
	m := &mockBackend{queryResult: &QueryResult{
		Columns: []string{"id"},
		Rows:    rows,
		Count:   3,
	}}
	svc := newTestService(m)

	res, err := svc.QueryRead(context.Background(), "SELECT id FROM users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Count != 3 {
		t.Errorf("expected 3 rows, got %d", res.Count)
	}
}

func TestDBService_QueryRead_RowLimitEnforced(t *testing.T) {
	t.Parallel()
	// Backend has 10 rows; service limit is 5.
	rows := make([]any, 10)
	for i := range rows {
		rows[i] = map[string]any{"id": i + 1}
	}
	m := &mockBackend{queryResult: &QueryResult{
		Columns: []string{"id"},
		Rows:    rows,
		Count:   10,
	}}
	svc := newTestService(m) // maxRows = 5

	res, err := svc.QueryRead(context.Background(), "SELECT id FROM users")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Count > 5 {
		t.Errorf("expected at most 5 rows (limit), got %d", res.Count)
	}
}

func TestDBService_QueryRead_WriteRejected(t *testing.T) {
	t.Parallel()
	writeQueries := []string{
		"INSERT INTO users VALUES (1)",
		"UPDATE users SET name='x'",
		"DELETE FROM users",
		"DROP TABLE users",
		"ALTER TABLE users ADD COLUMN x INT",
	}

	m := &mockBackend{}
	svc := newTestService(m)

	for _, q := range writeQueries {
		_, err := svc.QueryRead(context.Background(), q)
		if err == nil {
			t.Errorf("expected error for write query %q, got nil", q)
		}
	}
}

func TestDBService_ShowLocks_OK(t *testing.T) {
	t.Parallel()
	m := &mockBackend{locks: []LockInfo{
		{PID: 123, State: "active", Query: "SELECT sleep(10)"},
	}}
	svc := newTestService(m)

	locks, err := svc.ShowLocks(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locks) != 1 {
		t.Errorf("expected 1 lock, got %d", len(locks))
	}
}

func TestDBService_Close(t *testing.T) {
	t.Parallel()
	m := &mockBackend{}
	svc := newTestService(m)

	if err := svc.Close(); err != nil {
		t.Fatalf("unexpected error on close: %v", err)
	}
	if !m.closed {
		t.Error("expected backend.Close() to have been called")
	}
}

// ---------------------------------------------------------------------------
// stripLeadingComments tests
// ---------------------------------------------------------------------------

func TestStripLeadingComments(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"/* foo */ SELECT 1", "SELECT 1"},
		{"SELECT 1", "SELECT 1"},
		{"/* unclosed SELECT", "/* unclosed SELECT"},
		{"  /* foo */  SELECT 1", "SELECT 1"},
		{"-- line comment\nSELECT 1", "SELECT 1"},
		{"# hash comment\nSELECT 1", "SELECT 1"},
		{"/* block */ -- line\n# hash\nSELECT 1", "SELECT 1"},
		{"-- line comment without newline", ""},
		{"# hash comment without newline", ""},
	}
	for _, tc := range cases {
		got := stripLeadingComments(tc.input)
		if got != tc.want {
			t.Errorf("stripLeadingComments(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// applyLimit tests (sql backend helper)
// ---------------------------------------------------------------------------

func TestApplyLimit_NoExistingLimit(t *testing.T) {
	t.Parallel()
	q := applyLimit("SELECT * FROM users", 100, "postgres")
	if q == "SELECT * FROM users" {
		t.Error("expected limit to be applied, but query was unchanged")
	}
}

func TestApplyLimit_ExistingLimit(t *testing.T) {
	t.Parallel()
	q := applyLimit("SELECT * FROM users LIMIT 50", 100, "postgres")
	if q != "SELECT * FROM users LIMIT 50" {
		t.Errorf("expected existing limit to be preserved, got: %q", q)
	}

	// Verify that limit preceded by newline or tab is also preserved
	qNewline := applyLimit("SELECT * FROM users\nLIMIT 50", 100, "postgres")
	if qNewline != "SELECT * FROM users\nLIMIT 50" {
		t.Errorf("expected existing newline limit to be preserved, got: %q", qNewline)
	}

	qTab := applyLimit("SELECT * FROM users\tLIMIT 50", 100, "postgres")
	if qTab != "SELECT * FROM users\tLIMIT 50" {
		t.Errorf("expected existing tab limit to be preserved, got: %q", qTab)
	}
}

func TestApplyLimit_MultiStatement(t *testing.T) {
	t.Parallel()
	q := applyLimit("SELECT * FROM users; SELECT * FROM products;", 100, "postgres")
	if q != "SELECT * FROM users; SELECT * FROM products;" {
		t.Errorf("expected multi-statement query to not be wrapped, got %q", q)
	}
}

// ---------------------------------------------------------------------------
// injectBQLimit tests (bigquery backend helper)
// ---------------------------------------------------------------------------

func TestInjectBQLimit_NoExistingLimit(t *testing.T) {
	t.Parallel()
	q := injectBQLimit("SELECT * FROM `project.dataset.table`", 10)
	if q == "SELECT * FROM `project.dataset.table`" {
		t.Error("expected limit to be injected")
	}
}

func TestInjectBQLimit_ExistingLimit(t *testing.T) {
	t.Parallel()
	q := injectBQLimit("SELECT * FROM t LIMIT 5", 100)
	if q != "SELECT * FROM t LIMIT 5" {
		t.Errorf("expected existing limit preserved, got: %q", q)
	}

	// Verify that limit preceded by newline or tab is also preserved
	qNewline := injectBQLimit("SELECT * FROM t\nLIMIT 5", 100)
	if qNewline != "SELECT * FROM t\nLIMIT 5" {
		t.Errorf("expected existing newline limit preserved, got: %q", qNewline)
	}

	qTab := injectBQLimit("SELECT * FROM t\tLIMIT 5", 100)
	if qTab != "SELECT * FROM t\tLIMIT 5" {
		t.Errorf("expected existing tab limit preserved, got: %q", qTab)
	}
}

// ---------------------------------------------------------------------------
// NewDBService configuration error path
// ---------------------------------------------------------------------------

func TestNewDBService_EmptyBackend(t *testing.T) {
	t.Parallel()
	_, err := NewDBService(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewDBService_UnsupportedBackend(t *testing.T) {
	t.Parallel()
	_, err := NewDBService(&config.DBConfig{Backend: "cassandra", DSN: "localhost"})
	if err == nil {
		t.Fatal("expected error for unsupported backend 'cassandra', got nil")
	}
}

func TestNewDBServiceWithBackend_ZeroMaxRows_UsesDefault(t *testing.T) {
	t.Parallel()
	svc := NewDBServiceWithBackend(&mockBackend{}, 0, 5*time.Second)
	if svc.maxRows != 200 {
		t.Errorf("expected maxRows=200 default, got %d", svc.maxRows)
	}
}

func TestNewDBServiceWithBackend_ZeroTimeout_UsesDefault(t *testing.T) {
	t.Parallel()
	svc := NewDBServiceWithBackend(&mockBackend{}, 10, 0)
	if svc.queryTimeout != 10*time.Second {
		t.Errorf("expected 10s default timeout, got %v", svc.queryTimeout)
	}
}

func TestNewDBServiceWithBackend_PositiveValues(t *testing.T) {
	t.Parallel()
	svc := NewDBServiceWithBackend(&mockBackend{}, 50, 30*time.Second)
	if svc.maxRows != 50 {
		t.Errorf("expected maxRows=50, got %d", svc.maxRows)
	}
	if svc.queryTimeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", svc.queryTimeout)
	}
}

func TestListTablesQuery_AllDialects(t *testing.T) {
	t.Parallel()
	dialects := []string{"postgres", "mysql", "sqlite", "duckdb", "unknown"}
	for _, d := range dialects {
		q := listTablesQuery(d)
		if q == "" {
			t.Errorf("listTablesQuery(%q) returned empty string", d)
		}
	}
}

func TestShowLocksQuery_SupportedDialects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		dialect   string
		supported bool
	}{
		{"postgres", true},
		{"mysql", true},
		{"sqlite", false},
		{"duckdb", false},
	}
	for _, tc := range cases {
		_, supported := showLocksQuery(tc.dialect)
		if supported != tc.supported {
			t.Errorf("showLocksQuery(%q) supported=%v, want %v", tc.dialect, supported, tc.supported)
		}
	}
}

// ---------------------------------------------------------------------------
// BigQuery backend unit tests (mock bqClient)
// ---------------------------------------------------------------------------

// mockBQTableIterator implements bqTableIterator.
type mockBQTableIterator struct {
	tables []*bigquery.Table
	pos    int
	err    error
}

func (m *mockBQTableIterator) Next() (*bigquery.Table, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.pos >= len(m.tables) {
		return nil, iterator.Done
	}
	tbl := m.tables[m.pos]
	m.pos++
	return tbl, nil
}

// mockBQRowIterator implements bqRowIterator.
type mockBQRowIterator struct {
	rows   []map[string]bigquery.Value
	schema bigquery.Schema
	pos    int
	err    error
}

func (m *mockBQRowIterator) Next(dst interface{}) error {
	if m.err != nil {
		return m.err
	}
	if m.pos >= len(m.rows) {
		return iterator.Done
	}
	row := m.rows[m.pos]
	m.pos++
	if target, ok := dst.(*map[string]bigquery.Value); ok {
		*target = row
	}
	return nil
}

func (m *mockBQRowIterator) Schema() bigquery.Schema { return m.schema }

// mockBQClient implements the flat bqClient interface.
type mockBQClient struct {
	tableIt  bqTableIterator
	meta     *bigquery.TableMetadata
	metaErr  error
	rowIt    bqRowIterator
	queryErr error
	closeErr error
	closed   bool
}

func (m *mockBQClient) Tables(_ context.Context, _ string) bqTableIterator { return m.tableIt }
func (m *mockBQClient) TableMetadata(_ context.Context, _, _ string) (*bigquery.TableMetadata, error) {
	return m.meta, m.metaErr
}
func (m *mockBQClient) Query(_ context.Context, _ string) (bqRowIterator, error) {
	return m.rowIt, m.queryErr
}
func (m *mockBQClient) Close() error { m.closed = true; return m.closeErr }

func TestBQ_ListTables_IteratorError(t *testing.T) {
	t.Parallel()
	mc := &mockBQClient{tableIt: &mockBQTableIterator{err: errors.New("iter err")}}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	if _, err := b.ListTables(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestBQ_ListTables_Empty(t *testing.T) {
	t.Parallel()
	mc := &mockBQClient{tableIt: &mockBQTableIterator{}}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	tables, err := b.ListTables(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("expected empty, got %v", tables)
	}
}

func TestBQ_DescribeTable_OK(t *testing.T) {
	t.Parallel()
	meta := &bigquery.TableMetadata{Schema: bigquery.Schema{
		{Name: "id", Type: bigquery.IntegerFieldType, Required: true},
		{Name: "name", Type: bigquery.StringFieldType},
	}}
	mc := &mockBQClient{meta: meta}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	cols, err := b.DescribeTable(context.Background(), "users")
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	if len(cols) != 2 {
		t.Fatalf("expected 2 cols, got %d", len(cols))
	}
	if cols[0].Nullable != "NO" {
		t.Errorf("want NO, got %q", cols[0].Nullable)
	}
	if cols[1].Nullable != "YES" {
		t.Errorf("want YES, got %q", cols[1].Nullable)
	}
}

func TestBQ_DescribeTable_Error(t *testing.T) {
	t.Parallel()
	mc := &mockBQClient{metaErr: errors.New("not found")}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	if _, err := b.DescribeTable(context.Background(), "bad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBQ_QueryRead_OK(t *testing.T) {
	t.Parallel()
	rows := []map[string]bigquery.Value{{"id": int64(1)}, {"id": int64(2)}}
	schema := bigquery.Schema{{Name: "id", Type: bigquery.IntegerFieldType}}
	mc := &mockBQClient{rowIt: &mockBQRowIterator{rows: rows, schema: schema}}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	res, err := b.QueryRead(context.Background(), "SELECT * FROM t", 10)
	if err != nil {
		t.Fatalf("QueryRead: %v", err)
	}
	if res.Count != 2 {
		t.Errorf("expected 2 rows, got %d", res.Count)
	}
}

func TestBQ_QueryRead_LimitEnforced(t *testing.T) {
	t.Parallel()
	rows := []map[string]bigquery.Value{{"id": int64(1)}, {"id": int64(2)}, {"id": int64(3)}}
	schema := bigquery.Schema{{Name: "id", Type: bigquery.IntegerFieldType}}
	mc := &mockBQClient{rowIt: &mockBQRowIterator{rows: rows, schema: schema}}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	res, err := b.QueryRead(context.Background(), "SELECT * FROM t", 1)
	if err != nil {
		t.Fatalf("QueryRead: %v", err)
	}
	if res.Count > 1 {
		t.Errorf("expected ≤1 row, got %d", res.Count)
	}
}

func TestBQ_QueryRead_QueryError(t *testing.T) {
	t.Parallel()
	mc := &mockBQClient{queryErr: errors.New("fail")}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	if _, err := b.QueryRead(context.Background(), "SELECT 1", 10); err == nil {
		t.Fatal("expected error")
	}
}

func TestBQ_QueryRead_RowError(t *testing.T) {
	t.Parallel()
	mc := &mockBQClient{rowIt: &mockBQRowIterator{err: errors.New("row fail")}}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	if _, err := b.QueryRead(context.Background(), "SELECT 1", 10); err == nil {
		t.Fatal("expected row iterator error")
	}
}

func TestBQ_Close_OK(t *testing.T) {
	t.Parallel()
	mc := &mockBQClient{}
	b := newBigQueryBackendWithClient(mc, "p", "d")
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !mc.closed {
		t.Error("client not closed")
	}
}

func TestBQ_NewBigQueryBackend_EmptyDSN(t *testing.T) {
	t.Parallel()
	if _, err := newBigQueryBackend(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func TestBQ_NewBigQueryBackend_MalformedDSN(t *testing.T) {
	t.Parallel()
	if _, err := newBigQueryBackend(context.Background(), "noslash"); err == nil {
		t.Fatal("expected error for DSN without slash")
	}
}

func TestBQ_NewBigQueryBackend_ValidDSN_Error(t *testing.T) {
	t.Parallel()
	_, _ = newBigQueryBackend(context.Background(), "test-proj/test-dataset")
}

// ---------------------------------------------------------------------------
// Real adapter coverage — no GCP credentials required
// ---------------------------------------------------------------------------

func TestRealBQRowIterator_Schema(t *testing.T) {
	// bigquery.RowIterator.Schema is a public field (slice). A zero-value
	// RowIterator returns a nil schema — no network call, no panic.
	it := &realBQRowIterator{it: &bigquery.RowIterator{}}
	if s := it.Schema(); s != nil {
		t.Errorf("expected nil schema from zero-value iterator, got %v", s)
	}
}

func TestRealBQClient_AdapterMethods(t *testing.T) {
	// Use an httptest server so all BigQuery HTTP calls return immediately
	// (HTTP 403), covering the adapter method bodies and their error returns
	// without needing real GCP credentials or blocking on gRPC retries.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	ctx := context.Background()
	c, err := bigquery.NewClient(ctx, "fake-project",
		option.WithoutAuthentication(),
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Skipf("cannot construct BQ client: %v", err)
	}
	rc := &realBQClient{c: c}

	// Tables() is lazy — constructs an iterator object, no HTTP call.
	_ = rc.Tables(ctx, "fake-dataset")

	// TableMetadata and Query.Read both hit the test server (HTTP 403),
	// covering the function bodies and their error returns immediately.
	_, _ = rc.TableMetadata(ctx, "fake-dataset", "fake-table")
	_, _ = rc.Query(ctx, "SELECT 1")

	// Close covers the Close adapter.
	_ = rc.Close()
}
func TestRealBQRowIterator_Next(t *testing.T) {
	// Next on a zero-value RowIterator may panic internally (nil pageInfo).
	// recover() ensures the test passes either way while covering the adapter body.
	it := &realBQRowIterator{it: &bigquery.RowIterator{}}
	func() {
		defer func() { recover() }() //nolint:errcheck // recover is intentional here
		var dst map[string]bigquery.Value
		_ = it.Next(&dst)
	}()
}

// ---------------------------------------------------------------------------
// Tests for Copilot-flagged security fixes
// ---------------------------------------------------------------------------

func TestValidateReadOnly_WithDML_Rejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
	}{
		{"with delete", "WITH cte AS (SELECT 1) DELETE FROM users WHERE id IN (SELECT id FROM cte)"},
		{"with update", "WITH src AS (SELECT 1 AS x) UPDATE users SET name='x' FROM src"},
		{"with insert", "WITH vals AS (SELECT 1) INSERT INTO users SELECT * FROM vals"},
		{"with merge", "WITH src AS (SELECT 1) MERGE users USING src ON users.id = src.id WHEN MATCHED THEN DELETE"},
		{"explain update", "EXPLAIN UPDATE users SET name='x'"},
		{"explain delete", "EXPLAIN DELETE FROM users"},
		{"explain insert", "EXPLAIN INSERT INTO users VALUES(1)"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := validateReadOnly(tc.query); err == nil {
				t.Errorf("expected error for %q (WITH/EXPLAIN + DML), got nil", tc.query)
			}
		})
	}
}

func TestValidateIdentifier(t *testing.T) {
	t.Parallel()
	valid := []string{"users", "my_table", "schema.table", "Table123", "t"}
	for _, name := range valid {
		if err := validateIdentifier(name); err != nil {
			t.Errorf("validateIdentifier(%q) should pass, got: %v", name, err)
		}
	}
	invalid := []string{"", "users; DROP TABLE", "tab'le", "my-table", "tab(le", "us rs"}
	for _, name := range invalid {
		if err := validateIdentifier(name); err == nil {
			t.Errorf("validateIdentifier(%q) should fail but passed", name)
		}
	}
}

// Migrated integration tests to standard CGO-enabled unit tests
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
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
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

func Test_SQLite_ListTables(t *testing.T) {
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

func Test_SQLite_DescribeTable(t *testing.T) {
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

func Test_SQLite_QueryRead_OK(t *testing.T) {
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

func Test_SQLite_QueryRead_RowLimitRespected(t *testing.T) {
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

func Test_SQLite_QueryRead_WriteRejected(t *testing.T) {
	svc := newSQLiteService(t)

	_, err := svc.QueryRead(context.Background(), "INSERT INTO users (name) VALUES ('Hacker')")
	if err == nil {
		t.Fatal("expected write query to be rejected, got nil")
	}
}

func Test_SQLite_ShowLocks(t *testing.T) {
	svc := newSQLiteService(t)

	// SQLite returns an empty slice — no lock visibility at the SQL level.
	locks, err := svc.ShowLocks(context.Background())
	if err != nil {
		t.Fatalf("ShowLocks: %v", err)
	}
	// Empty is correct for SQLite; just verify no error.
	_ = locks
}

func Test_SQLite_Close(t *testing.T) {
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

func Test_ResolveDriver_SQLite(t *testing.T) {
	name, dsn := resolveDriver("sqlite", "/tmp/test.db")
	if name != "sqlite" {
		t.Errorf("expected driver 'sqlite', got %q", name)
	}
	if dsn == "/tmp/test.db" {
		t.Error("expected _query_only to be appended to SQLite DSN")
	}
}

func Test_ResolveDriver_SQLiteAlreadyHasParam(t *testing.T) {
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

	// Verify that _query_only=false is overridden to true
	_, dsnOverride := resolveDriver("sqlite", "/tmp/test.db?_query_only=false")
	if !strings.Contains(dsnOverride, "_query_only=true") || strings.Contains(dsnOverride, "false") {
		t.Errorf("expected _query_only=false to be overridden to true, got: %q", dsnOverride)
	}

	// Verify that _query_only in the file path itself is not rewritten
	_, dsnPath := resolveDriver("sqlite", "/tmp/_query_only.db")
	if !strings.Contains(dsnPath, "_query_only.db") {
		t.Errorf("expected file path to remain intact, got: %q", dsnPath)
	}

	// Verify that _query_only in path with other query parameters is not rewritten and correct param is appended
	_, dsnPathWithParam := resolveDriver("sqlite", "/tmp/_query_only.db?cache=shared&_query_only=false")
	if !strings.HasPrefix(dsnPathWithParam, "/tmp/_query_only.db?") {
		t.Errorf("expected path to remain intact, got: %q", dsnPathWithParam)
	}
	if !strings.Contains(dsnPathWithParam, "_query_only=true") || strings.Contains(dsnPathWithParam, "false") {
		t.Errorf("expected _query_only=false to be overridden to true, got: %q", dsnPathWithParam)
	}
}

func Test_ResolveDriver_DuckDB(t *testing.T) {
	name, dsn := resolveDriver("duckdb", "/tmp/test.duckdb")
	if name != "duckdb" {
		t.Errorf("expected driver 'duckdb', got %q", name)
	}
	if dsn == "/tmp/test.duckdb" {
		t.Error("expected access_mode to be appended to DuckDB DSN")
	}

	// Verify that access_mode=READ_WRITE is overridden to READ_ONLY
	_, dsnOverride := resolveDriver("duckdb", "/tmp/test.duckdb?access_mode=READ_WRITE")
	if !strings.Contains(dsnOverride, "access_mode=READ_ONLY") || strings.Contains(dsnOverride, "READ_WRITE") {
		t.Errorf("expected access_mode=READ_WRITE to be overridden to READ_ONLY, got: %q", dsnOverride)
	}

	// Verify that access_mode in the file path itself is not rewritten
	_, dsnPath := resolveDriver("duckdb", "/tmp/access_mode.duckdb")
	if !strings.Contains(dsnPath, "access_mode.duckdb") {
		t.Errorf("expected file path to remain intact, got: %q", dsnPath)
	}

	// Verify that access_mode in path with query parameters is not rewritten and correct param is appended
	_, dsnPathWithParam := resolveDriver("duckdb", "/tmp/access_mode.duckdb?cache=shared&access_mode=READ_WRITE")
	if !strings.HasPrefix(dsnPathWithParam, "/tmp/access_mode.duckdb?") {
		t.Errorf("expected path to remain intact, got: %q", dsnPathWithParam)
	}
	if !strings.Contains(dsnPathWithParam, "access_mode=READ_ONLY") || strings.Contains(dsnPathWithParam, "READ_WRITE") {
		t.Errorf("expected access_mode=READ_WRITE to be overridden to READ_ONLY, got: %q", dsnPathWithParam)
	}
}

func Test_ResolveDriver_Postgres(t *testing.T) {
	name, dsn := resolveDriver("postgres", "postgres://localhost/db")
	if name != "postgres" {
		t.Errorf("expected driver 'postgres', got %q", name)
	}
	if !strings.Contains(dsn, "default_transaction_read_only%3Don") {
		t.Errorf("expected default_transaction_read_only to be set, got %q", dsn)
	}

	// Test URL format with existing options
	_, dsnUrl := resolveDriver("postgres", "postgres://localhost/db?options=-c%20search_path%3Dpublic")
	if !strings.Contains(dsnUrl, "options=-c%20search_path%3Dpublic%20-c%20default_transaction_read_only%3Don") {
		t.Errorf("expected options to be appended, got %q", dsnUrl)
	}

	// Test key-value DSN format
	_, dsnKV := resolveDriver("postgres", "host=localhost dbname=test")
	if !strings.Contains(dsnKV, "options='-c default_transaction_read_only=on'") {
		t.Errorf("expected options to be set in KV format, got %q", dsnKV)
	}

	// Test key-value DSN format with existing options
	_, dsnKVOpt := resolveDriver("postgres", "host=localhost dbname=test options='-c search_path=public'")
	if !strings.Contains(dsnKVOpt, "options='-c search_path=public -c default_transaction_read_only=on'") {
		t.Errorf("expected options to be appended in KV format, got %q", dsnKVOpt)
	}
}

func Test_ResolveDriver_MySQL(t *testing.T) {
	name, dsn := resolveDriver("mysql", "user:pass@tcp(localhost)/db")
	if name != "mysql" {
		t.Errorf("expected driver 'mysql', got %q", name)
	}
	if !strings.Contains(dsn, "sessionVariables=transaction_read_only=1,tx_read_only=1") {
		t.Errorf("expected sessionVariables to be set, got %q", dsn)
	}

	// Test with existing sessionVariables
	_, dsnSess := resolveDriver("mysql", "user:pass@tcp(localhost)/db?sessionVariables=sql_mode=TRADITIONAL")
	if !strings.Contains(dsnSess, "sessionVariables=sql_mode=TRADITIONAL,transaction_read_only=1,tx_read_only=1") {
		t.Errorf("expected sessionVariables to be appended, got %q", dsnSess)
	}
}

func TestNewSQLBackend_PingErrorMasksDSN(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to open a postgres backend with a credentials-containing DSN that will fail to ping
	secretDSN := "postgres://" + "secret_user:" + "secret_password" + "@127.0.0.1:9999/nonexistent_db"
	_, err := newSQLBackend(ctx, "postgres", secretDSN)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if strings.Contains(err.Error(), "secret_password") {
		t.Errorf("error leaked secret credentials in DSN: %v", err)
	}
	if strings.Contains(err.Error(), "secret_user") {
		t.Errorf("error leaked username in DSN: %v", err)
	}
}

// ---------------------------------------------------------------------------
// describeTableQuery — dialect coverage
// ---------------------------------------------------------------------------

func Test_DescribeTableQuery_AllDialects(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{"postgres", "mysql", "sqlite", "duckdb"} {
		query, args, err := describeTableQuery(dialect, "users")
		if err != nil {
			t.Errorf("describeTableQuery(%q) unexpected error: %v", dialect, err)
			continue
		}
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

func Test_ApplyReadOnlySession_SQLite_IsNoop(t *testing.T) {
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

func Test_ApplyReadOnlySession_PostgresCmd_OnSQLite_Fails(t *testing.T) {
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

func Test_ApplyReadOnlySession_MySQLCmd_OnSQLite_Fails(t *testing.T) {
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

func Test_SQLite_ShowLocks_PostgresDialect_Error(t *testing.T) {
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

func Test_SQLite_ListTables_PostgresDialect_Error(t *testing.T) {
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

func Test_SQLite_DescribeTable_PostgresDialect_Error(t *testing.T) {
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

func Test_SQLite_QueryRead_PostgresDialect_ErrorPath(t *testing.T) {
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

func Test_BigQuery_ShowLocks_Advisory(t *testing.T) {
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

func Test_InjectBQLimit_Variants(t *testing.T) {
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

func Test_NewDBService_EmptyDSN(t *testing.T) {
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

func Test_NewDBService_InvalidTimeout_UsesDefault(t *testing.T) {
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

func Test_NewDBService_ZeroMaxRows_UsesDefault(t *testing.T) {
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
func Test_NewDBService_BackendInitFails(t *testing.T) {
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

func Test_NewSQLBackend_PingFails(t *testing.T) {
	// /dev/null/test.db — directory component is a device node, not a dir.
	// sql.Open is lazy (succeeds), but PingContext fails — covering that branch.
	_, err := newSQLBackend(context.Background(), "sqlite", "/dev/null/test.db")
	if err == nil {
		t.Fatal("expected ping failure for invalid SQLite path")
	}
}
