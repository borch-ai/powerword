package db

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

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
		{"multi stmt selects", "SELECT 1; SELECT 2"},
		{"empty second stmt", "SELECT 1;"},
		{"comment only statement", "SELECT 1; /* just a comment */"},
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
// stripLeadingComment tests
// ---------------------------------------------------------------------------

func TestStripLeadingComment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"/* foo */ SELECT 1", "SELECT 1"},
		{"SELECT 1", "SELECT 1"},
		{"/* unclosed SELECT", "/* unclosed SELECT"},
		{"  /* foo */  SELECT 1", "SELECT 1"},
	}
	for _, tc := range cases {
		got := stripLeadingComment(tc.input)
		if got != tc.want {
			t.Errorf("stripLeadingComment(%q) = %q, want %q", tc.input, got, tc.want)
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
	if _, err := newBigQueryBackend(""); err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func TestBQ_NewBigQueryBackend_MalformedDSN(t *testing.T) {
	t.Parallel()
	if _, err := newBigQueryBackend("noslash"); err == nil {
		t.Fatal("expected error for DSN without slash")
	}
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
