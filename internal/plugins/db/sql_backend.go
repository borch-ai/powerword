//go:build cgo && integration

package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	// Pure-Go SQLite driver (no CGO required).
	_ "modernc.org/sqlite"

	// Postgres driver (no CGO required).
	_ "github.com/lib/pq"

	// MySQL driver (no CGO required).
	_ "github.com/go-sql-driver/mysql"

	// DuckDB driver (CGO required — deliberate exception for pw-mcp-db).
	// This package always requires CGO_ENABLED=1; there is no build tag
	// gating this import. Only cmd/pw-mcp-db should import this package.
	_ "github.com/duckdb/duckdb-go/v2"
)

// sqlBackend wraps a *sql.DB and adapts it to the Backend interface.
// It works for postgres, mysql, sqlite, and duckdb via their respective
// database/sql drivers.
type sqlBackend struct {
	db      *sql.DB
	dialect string // "postgres" | "mysql" | "sqlite" | "duckdb"
}

// newSQLBackend opens a database/sql connection for the given dialect and DSN,
// then applies session-level read-only settings where supported.
func newSQLBackend(ctx context.Context, dialect, dsn string) (Backend, error) {
	if dsn == "" {
		return nil, fmt.Errorf("DSN is required for %s backend", dialect)
	}

	driverName, sessionDSN := resolveDriver(dialect, dsn)

	db, err := sql.Open(driverName, sessionDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s connection: %w", dialect, err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping %s database: %w", dialect, err)
	}

	// Apply session-level read-only guardrail where the driver supports it.
	if err := applyReadOnlySession(ctx, db, dialect); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &sqlBackend{db: db, dialect: dialect}, nil
}

// resolveDriver returns the registered driver name and the final DSN to use.
// For SQLite we append and override the read-only query parameter to ensure it is true.
// For DuckDB we append and override the access_mode parameter to ensure it is READ_ONLY.
func resolveDriver(dialect, dsn string) (string, string) {
	switch dialect {
	case "sqlite":
		// modernc.org/sqlite registers as "sqlite".
		// Enforce _query_only=true at the driver level, overriding any existing value.
		parts := strings.SplitN(dsn, "?", 2)
		if len(parts) == 1 {
			dsn = parts[0] + "?_query_only=true"
		} else {
			params := strings.Split(parts[1], "&")
			var newParams []string
			for _, p := range params {
				if !strings.HasPrefix(p, "_query_only=") && p != "_query_only" {
					newParams = append(newParams, p)
				}
			}
			newParams = append(newParams, "_query_only=true")
			dsn = parts[0] + "?" + strings.Join(newParams, "&")
		}
		return "sqlite", dsn
	case "duckdb":
		// DuckDB driver registers as "duckdb".
		// Enforce access_mode=READ_ONLY at the driver level, overriding any existing value.
		parts := strings.SplitN(dsn, "?", 2)
		if len(parts) == 1 {
			dsn = parts[0] + "?access_mode=READ_ONLY"
		} else {
			params := strings.Split(parts[1], "&")
			var newParams []string
			for _, p := range params {
				if !strings.HasPrefix(p, "access_mode=") && p != "access_mode" {
					newParams = append(newParams, p)
				}
			}
			newParams = append(newParams, "access_mode=READ_ONLY")
			dsn = parts[0] + "?" + strings.Join(newParams, "&")
		}
		return "duckdb", dsn
	case "mysql":
		return "mysql", appendMySQLReadOnly(dsn)
	default: // postgres / postgresql
		return "postgres", appendPostgresReadOnly(dsn)
	}
}

// appendPostgresReadOnly appends Postgres connection parameters to enforce read-only transactions
// at the connection level. It works on both URL DSNs and key-value DSNs.
func appendPostgresReadOnly(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return appendPostgresURLReadOnly(dsn)
	}
	return appendPostgresKVReadOnly(dsn)
}

func appendPostgresURLReadOnly(dsn string) string {
	parts := strings.SplitN(dsn, "?", 2)
	if len(parts) == 1 {
		return parts[0] + "?options=-c%20default_transaction_read_only%3Don"
	}
	params := strings.Split(parts[1], "&")
	var newParams []string
	found := false
	for _, p := range params {
		if strings.HasPrefix(p, "options=") {
			found = true
			val := strings.TrimPrefix(p, "options=")
			newParams = append(newParams, "options="+val+"%20-c%20default_transaction_read_only%3Don")
		} else {
			newParams = append(newParams, p)
		}
	}
	if !found {
		newParams = append(newParams, "options=-c%20default_transaction_read_only%3Don")
	}
	return parts[0] + "?" + strings.Join(newParams, "&")
}

func appendPostgresKVReadOnly(dsn string) string {
	reQuote := regexp.MustCompile(`\boptions\s*=\s*'([^']*)'`)
	if reQuote.MatchString(dsn) {
		return reQuote.ReplaceAllString(dsn, "options='${1} -c default_transaction_read_only=on'")
	}
	reDQuote := regexp.MustCompile(`\boptions\s*=\s*"([^"]*)"`)
	if reDQuote.MatchString(dsn) {
		return reDQuote.ReplaceAllString(dsn, `options="${1} -c default_transaction_read_only=on"`)
	}
	reNoQuote := regexp.MustCompile(`\boptions\s*=\s*([^\s]+)`)
	if reNoQuote.MatchString(dsn) {
		return reNoQuote.ReplaceAllString(dsn, "options='${1} -c default_transaction_read_only=on'")
	}
	return dsn + " options='-c default_transaction_read_only=on'"
}

// appendMySQLReadOnly appends MySQL session variables parameter to enforce read-only transactions.
func appendMySQLReadOnly(dsn string) string {
	parts := strings.SplitN(dsn, "?", 2)
	if len(parts) == 1 {
		return parts[0] + "?sessionVariables=transaction_read_only=1,tx_read_only=1"
	}
	params := strings.Split(parts[1], "&")
	var newParams []string
	found := false
	for _, p := range params {
		if strings.HasPrefix(p, "sessionVariables=") {
			found = true
			val := strings.TrimPrefix(p, "sessionVariables=")
			newParams = append(newParams, "sessionVariables="+val+",transaction_read_only=1,tx_read_only=1")
		} else {
			newParams = append(newParams, p)
		}
	}
	if !found {
		newParams = append(newParams, "sessionVariables=transaction_read_only=1,tx_read_only=1")
	}
	return parts[0] + "?" + strings.Join(newParams, "&")
}

// applyReadOnlySession executes session-level read-only commands for backends
// that need them in addition to driver-level settings.
func applyReadOnlySession(ctx context.Context, db *sql.DB, dialect string) error {
	var stmt string
	switch dialect {
	case "postgres":
		stmt = "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY"
	case "mysql":
		stmt = "SET SESSION TRANSACTION READ ONLY"
	default:
		// SQLite and DuckDB use DSN parameters; no session command needed.
		return nil
	}

	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("failed to set read-only session for %s: %w", dialect, err)
	}
	return nil
}

// ListTables returns all user table and view names in the current schema/database.
func (b *sqlBackend) ListTables(ctx context.Context) ([]string, error) {
	query := listTablesQuery(b.dialect)
	rows, err := b.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list tables failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// listTablesQuery returns the dialect-specific SQL to enumerate tables/views.
func listTablesQuery(dialect string) string {
	switch dialect {
	case "mysql":
		return `SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME`
	case "sqlite":
		return `SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`
	case "duckdb":
		return `SELECT table_name FROM information_schema.tables WHERE table_schema = 'main' ORDER BY table_name`
	default: // postgres
		return `SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema') UNION ALL SELECT viewname FROM pg_catalog.pg_views WHERE schemaname NOT IN ('pg_catalog','information_schema') ORDER BY 1`
	}
}

// DescribeTable returns column metadata for the given table name.
func (b *sqlBackend) DescribeTable(ctx context.Context, table string) ([]ColumnInfo, error) {
	query, args, err := describeTableQuery(b.dialect, table)
	if err != nil {
		return nil, err
	}
	rows, err := b.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("describe table %q failed: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var cols []ColumnInfo
	for rows.Next() {
		var c ColumnInfo
		var nullable, key, def sql.NullString
		if err := rows.Scan(&c.Name, &c.DataType, &nullable, &key, &def); err != nil {
			return nil, fmt.Errorf("scan column info: %w", err)
		}
		c.Nullable = nullable.String
		c.Key = key.String
		c.Default = def.String
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// validateIdentifier checks that a SQL identifier (table or column name) contains
// only safe characters. This prevents SQL injection in PRAGMA statements and other
// contexts where the driver does not support bound parameters.
func validateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("identifier must not be empty")
	}
	for _, r := range name {
		if ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("unsafe character %q in table name %q", r, name)
	}
	return nil
}

// describeTableQuery returns the dialect-specific INFORMATION_SCHEMA query and args.
// For SQLite, args is nil — the table name is embedded directly in the PRAGMA statement
// because modernc.org/sqlite does not support pragma_table_info() as a virtual table
// with parameterised input. The table name is validated by validateIdentifier before
// interpolation to prevent injection.
func describeTableQuery(dialect, table string) (string, []any, error) {
	switch dialect {
	case "mysql":
		return `SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_DEFAULT
			FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
			ORDER BY ORDINAL_POSITION`, []any{table}, nil
	case "sqlite":
		if err := validateIdentifier(table); err != nil {
			return "", nil, fmt.Errorf("describe table: %w", err)
		}
		// The table name is embedded as a literal because pragma_table_info()
		// does not accept bound parameters; validateIdentifier above ensures
		// the name contains only safe characters.
		//nolint:gosec // G201: table name is allow-listed to [a-zA-Z0-9_.] by validateIdentifier
		q := fmt.Sprintf(`SELECT name, type,
			CASE WHEN "notnull" = 1 THEN 'NO' ELSE 'YES' END,
			CASE WHEN pk > 0 THEN 'PRIMARY' ELSE '' END,
			dflt_value
			FROM pragma_table_info('%s')`, table)
		return q, nil, nil
	case "duckdb":
		return `SELECT column_name, data_type, is_nullable, '' AS key, column_default
			FROM information_schema.columns
			WHERE table_schema = 'main' AND table_name = ?
			ORDER BY ordinal_position`, []any{table}, nil
	default: // postgres
		return `SELECT column_name, udt_name, is_nullable, '' AS key, column_default
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1
			ORDER BY ordinal_position`, []any{table}, nil
	}
}

// QueryRead executes a validated read-only SELECT and returns up to limit rows.
func (b *sqlBackend) QueryRead(ctx context.Context, query string, limit int) (*QueryResult, error) {
	// Enforce row limit by wrapping in a subquery if no LIMIT clause is present.
	bounded := applyLimit(query, limit, b.dialect)

	rows, err := b.db.QueryContext(ctx, bounded)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("fetch columns: %w", err)
	}

	var allRows []any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		rowMap := make(map[string]any, len(cols))
		for i, col := range cols {
			rowMap[col] = vals[i]
		}
		allRows = append(allRows, rowMap)
		if len(allRows) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration: %w", err)
	}

	return &QueryResult{
		Columns: cols,
		Rows:    allRows,
		Count:   len(allRows),
	}, nil
}

// applyLimit wraps the query in a subquery with a LIMIT clause if one is not already
// present. This is a best-effort safeguard; the primary limit is enforced by the
// row-count break in QueryRead.
func applyLimit(query string, limit int, dialect string) string {
	query = strings.TrimSpace(query)
	// Skip subquery-wrapping if query contains multiple non-empty statements.
	var nonCount int
	for _, stmt := range strings.Split(query, ";") {
		if strings.TrimSpace(stmt) != "" {
			nonCount++
		}
	}
	if nonCount > 1 {
		return query
	}

	// Single statement: trim trailing semicolon.
	trimmed := strings.TrimSuffix(query, ";")
	trimmed = strings.TrimSpace(trimmed)

	if limitRegex.MatchString(trimmed) {
		return query
	}
	return fmt.Sprintf("SELECT * FROM (%s) _pw_q LIMIT %d", trimmed, limit)
}

// ShowLocks returns active long-running queries and locks. Returns an empty slice
// for backends that do not support lock introspection.
func (b *sqlBackend) ShowLocks(ctx context.Context) ([]LockInfo, error) {
	query, supported := showLocksQuery(b.dialect)
	if !supported {
		// SQLite and DuckDB do not expose lock metadata via SQL.
		return []LockInfo{}, nil
	}

	rows, err := b.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("show locks failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var locks []LockInfo
	for rows.Next() {
		var l LockInfo
		var wait sql.NullString
		var secs sql.NullInt64
		if err := rows.Scan(&l.PID, &l.State, &l.Query, &wait, &secs); err != nil {
			return nil, fmt.Errorf("scan lock info: %w", err)
		}
		l.Wait = wait.String
		l.Seconds = secs.Int64
		locks = append(locks, l)
	}
	return locks, rows.Err()
}

// showLocksQuery returns the dialect-specific query for lock/session info.
func showLocksQuery(dialect string) (string, bool) {
	switch dialect {
	case "postgres":
		return `SELECT pid, state, query,
			wait_event,
			EXTRACT(EPOCH FROM (now() - query_start))::bigint AS duration_seconds
			FROM pg_stat_activity
			WHERE state IS NOT NULL AND query NOT LIKE '%pg_stat_activity%'
			ORDER BY duration_seconds DESC NULLS LAST
			LIMIT 50`, true
	case "mysql":
		return `SELECT id, state, info,
			NULL AS wait_event,
			time AS duration_seconds
			FROM information_schema.PROCESSLIST
			WHERE command != 'Sleep'
			ORDER BY time DESC
			LIMIT 50`, true
	default:
		return "", false
	}
}

// Close closes the underlying database connection.
func (b *sqlBackend) Close() error {
	return b.db.Close()
}
