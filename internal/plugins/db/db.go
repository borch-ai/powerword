// Package db implements the pw-mcp-db database inspector plugin.
// It exposes read-only database introspection tools via the Model Context Protocol.
package db

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/borch-ai/powerword/pkg/config"
)

// Backend defines the read-only operations any database backend must implement.
// All implementations must enforce read-only access at the driver or session level.
type Backend interface {
	ListTables(ctx context.Context) ([]string, error)
	DescribeTable(ctx context.Context, table string) ([]ColumnInfo, error)
	QueryRead(ctx context.Context, query string, limit int) (*QueryResult, error)
	ShowLocks(ctx context.Context) ([]LockInfo, error)
	Close() error
}

// ColumnInfo describes a single column in a table.
type ColumnInfo struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Nullable string `json:"nullable"`
	Key      string `json:"key,omitempty"` // PRIMARY, UNIQUE, etc.
	Default  string `json:"default,omitempty"`
}

// QueryResult holds the output of a read-only SELECT query.
type QueryResult struct {
	Columns []string `json:"columns"`
	Rows    []any    `json:"rows"`
	Count   int      `json:"count"`
}

// LockInfo represents a running query or blocking session.
type LockInfo struct {
	PID     int64  `json:"pid"`
	Query   string `json:"query"`
	State   string `json:"state"`
	Wait    string `json:"wait_event,omitempty"`
	Seconds int64  `json:"duration_seconds,omitempty"`
}

// DBService coordinates a backend and applies cross-cutting policies
// such as read-only query validation and configurable result limits.
type DBService struct {
	backend      Backend
	maxRows      int
	queryTimeout time.Duration
}

// NewDBService constructs a DBService by selecting and initialising the correct backend
// based on the provided DBConfig.
func NewDBService(cfg *config.DBConfig) (*DBService, error) {
	if cfg == nil || cfg.Backend == "" {
		return nil, fmt.Errorf("db backend is not configured (set POWERWORD_DB_BACKEND)")
	}

	timeout, err := time.ParseDuration(cfg.QueryTimeout)
	if err != nil || timeout <= 0 {
		timeout = 10 * time.Second
	}
	maxRows := cfg.MaxRows
	if maxRows <= 0 {
		maxRows = 200
	}

	var b Backend
	// Use a bounded context for init so PingContext never hangs indefinitely.
	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer initCancel()
	switch strings.ToLower(cfg.Backend) {
	case "postgres", "postgresql":
		b, err = newSQLBackend(initCtx, "postgres", cfg.DSN)
	case "mysql":
		b, err = newSQLBackend(initCtx, "mysql", cfg.DSN)
	case "sqlite", "sqlite3":
		b, err = newSQLBackend(initCtx, "sqlite", cfg.DSN)
	case "duckdb":
		b, err = newSQLBackend(initCtx, "duckdb", cfg.DSN)
	case "bigquery":
		b, err = newBigQueryBackend(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported db backend %q: must be one of postgres, mysql, sqlite, duckdb, bigquery", cfg.Backend)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialise %s backend: %w", cfg.Backend, err)
	}

	return &DBService{
		backend:      b,
		maxRows:      maxRows,
		queryTimeout: timeout,
	}, nil
}

// NewDBServiceWithBackend constructs a DBService with an injected backend (used in tests).
func NewDBServiceWithBackend(b Backend, maxRows int, queryTimeout time.Duration) *DBService {
	if maxRows <= 0 {
		maxRows = 200
	}
	if queryTimeout <= 0 {
		queryTimeout = 10 * time.Second
	}
	return &DBService{backend: b, maxRows: maxRows, queryTimeout: queryTimeout}
}

// Close releases the underlying backend connection.
func (s *DBService) Close() error {
	return s.backend.Close()
}

// ListTables returns all table names visible in the configured database/schema.
func (s *DBService) ListTables(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	return s.backend.ListTables(ctx)
}

// DescribeTable returns column metadata for the named table.
func (s *DBService) DescribeTable(ctx context.Context, table string) ([]ColumnInfo, error) {
	if table == "" {
		return nil, fmt.Errorf("table name is required")
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	return s.backend.DescribeTable(ctx, table)
}

// QueryRead validates and executes a read-only SELECT query, capping results at maxRows.
func (s *DBService) QueryRead(ctx context.Context, query string) (*QueryResult, error) {
	if err := validateReadOnly(query); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	return s.backend.QueryRead(ctx, query, s.maxRows)
}

// ShowLocks returns information about running queries or lock contention.
func (s *DBService) ShowLocks(ctx context.Context) ([]LockInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	return s.backend.ShowLocks(ctx)
}

// allowedReadKeywords is the set of SQL statement-opener keywords that are safe
// to execute. WITH and EXPLAIN are included but require secondary validation
// (see validateReadOnly) to reject patterns like "WITH ... DELETE" or
// "EXPLAIN UPDATE ..." that could carry write intent.
var allowedReadKeywords = map[string]bool{
	"SELECT":   true,
	"SHOW":     true,
	"DESCRIBE": true,
	"DESC":     true,
	"EXPLAIN":  true,
	"WITH":     true,
	"PRAGMA":   true, // SQLite / DuckDB diagnostic commands
}

// writeKeywords is the set of SQL keywords that indicate a mutating statement.
var writeKeywords = map[string]bool{
	"INSERT": true, "UPDATE": true, "DELETE": true, "MERGE": true,
	"DROP": true, "CREATE": true, "ALTER": true, "TRUNCATE": true,
	"GRANT": true, "REVOKE": true, "REPLACE": true, "CALL": true,
}

// validateReadOnly inspects each semicolon-delimited statement in the query string
// and returns an error if any statement begins with a non-read keyword.
// For statements starting with WITH or EXPLAIN, it additionally scans all tokens
// for write keywords to reject patterns such as "WITH ... DELETE ..." or
// "EXPLAIN UPDATE ...", which are valid on BigQuery and other engines that lack
// a session-level read-only mode.
func validateReadOnly(query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query must not be empty")
	}
	for _, stmt := range strings.Split(query, ";") {
		stmt = strings.TrimFunc(stmt, unicode.IsSpace)
		if stmt == "" {
			continue
		}
		// Strip leading block comments /* … */
		stmt = stripLeadingComment(stmt)
		// Extract the first token
		fields := strings.FieldsFunc(stmt, func(r rune) bool {
			return unicode.IsSpace(r) || r == '('
		})
		if len(fields) == 0 {
			continue
		}
		keyword := strings.ToUpper(fields[0])
		if !allowedReadKeywords[keyword] {
			return fmt.Errorf("query rejected: statement starting with %q is not allowed (only read-only queries are permitted)", keyword)
		}
		// WITH and EXPLAIN can precede DML on BigQuery and other engines that
		// do not enforce session read-only mode at the driver level. Scan all
		// subsequent tokens for write keywords to catch these patterns.
		if keyword == "WITH" || keyword == "EXPLAIN" {
			for _, tok := range fields[1:] {
				if writeKeywords[strings.ToUpper(tok)] {
					return fmt.Errorf("query rejected: %q statement contains write keyword %q", keyword, strings.ToUpper(tok))
				}
			}
		}
	}
	return nil
}

// stripLeadingComment removes a leading /* … */ block comment from a SQL statement.
func stripLeadingComment(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "/*") {
		return s
	}
	end := strings.Index(s, "*/")
	if end == -1 {
		return s
	}
	return strings.TrimSpace(s[end+2:])
}
