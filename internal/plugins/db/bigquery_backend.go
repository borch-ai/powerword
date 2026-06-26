//go:build cgo

package db

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// ---------------------------------------------------------------------------
// Interface definitions — defined at the consumer to enable test injection
// ---------------------------------------------------------------------------

// bqClient is the subset of the BigQuery client API used by bigQueryBackend.
type bqClient interface {
	// Tables returns an iterator over tables in the given dataset.
	Tables(ctx context.Context, datasetID string) bqTableIterator
	// TableMetadata returns schema metadata for a table.
	TableMetadata(ctx context.Context, datasetID, tableID string) (*bigquery.TableMetadata, error)
	// Query runs a SQL query and returns a row iterator.
	Query(ctx context.Context, q string) (bqRowIterator, error)
	// Close closes the underlying client.
	Close() error
}

// bqTableIterator is satisfied by *bigquery.TableIterator.
type bqTableIterator interface {
	Next() (*bigquery.Table, error)
}

// bqRowIterator wraps a *bigquery.RowIterator.
type bqRowIterator interface {
	Next(dst interface{}) error
	Schema() bigquery.Schema
}

// ---------------------------------------------------------------------------
// Real implementation wrapping the BigQuery SDK
// ---------------------------------------------------------------------------

type realBQClient struct {
	c *bigquery.Client
}

func (r *realBQClient) Tables(ctx context.Context, datasetID string) bqTableIterator {
	return r.c.Dataset(datasetID).Tables(ctx)
}

func (r *realBQClient) TableMetadata(ctx context.Context, datasetID, tableID string) (*bigquery.TableMetadata, error) {
	return r.c.Dataset(datasetID).Table(tableID).Metadata(ctx)
}

func (r *realBQClient) Query(ctx context.Context, q string) (bqRowIterator, error) {
	it, err := r.c.Query(q).Read(ctx)
	if err != nil {
		return nil, err
	}
	return &realBQRowIterator{it}, nil
}

func (r *realBQClient) Close() error { return r.c.Close() }

// realBQRowIterator wraps *bigquery.RowIterator to satisfy bqRowIterator.
// Schema is a public field on the real type; we expose it as a method here.
type realBQRowIterator struct{ it *bigquery.RowIterator }

func (r *realBQRowIterator) Next(dst interface{}) error { return r.it.Next(dst) }
func (r *realBQRowIterator) Schema() bigquery.Schema    { return r.it.Schema }

// ---------------------------------------------------------------------------
// bigQueryBackend
// ---------------------------------------------------------------------------

// bigQueryBackend implements Backend using the bqClient interface.
// Authentication relies on Application Default Credentials (ADC).
type bigQueryBackend struct {
	client    bqClient
	projectID string
	datasetID string
}

// newBigQueryBackend constructs a BigQuery backend from a "projectID/datasetID" DSN.
func newBigQueryBackend(ctx context.Context, dsn string) (Backend, error) {
	if dsn == "" {
		return nil, fmt.Errorf("BigQuery DSN is required in the format \"projectID/datasetID\"")
	}
	parts := strings.SplitN(dsn, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid BigQuery DSN %q: expected format \"projectID/datasetID\"", dsn)
	}
	projectID := parts[0]
	datasetID := parts[1]

	c, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create BigQuery client for project %q: %w", projectID, err)
	}

	return &bigQueryBackend{
		client:    &realBQClient{c},
		projectID: projectID,
		datasetID: datasetID,
	}, nil
}

// newBigQueryBackendWithClient constructs a bigQueryBackend with an injected
// client — used in unit tests.
func newBigQueryBackendWithClient(client bqClient, projectID, datasetID string) Backend {
	return &bigQueryBackend{client: client, projectID: projectID, datasetID: datasetID}
}

// ListTables returns all table names in the configured BigQuery dataset.
func (b *bigQueryBackend) ListTables(ctx context.Context) ([]string, error) {
	it := b.client.Tables(ctx, b.datasetID)

	var tables []string
	for {
		tbl, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list BigQuery tables in dataset %q: %w", b.datasetID, err)
		}
		tables = append(tables, tbl.TableID)
	}
	return tables, nil
}

// DescribeTable returns schema metadata for the named BigQuery table.
func (b *bigQueryBackend) DescribeTable(ctx context.Context, table string) ([]ColumnInfo, error) {
	meta, err := b.client.TableMetadata(ctx, b.datasetID, table)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for table %q: %w", table, err)
	}

	cols := make([]ColumnInfo, 0, len(meta.Schema))
	for _, field := range meta.Schema {
		nullable := "YES"
		if field.Required {
			nullable = "NO"
		}
		cols = append(cols, ColumnInfo{
			Name:     field.Name,
			DataType: string(field.Type),
			Nullable: nullable,
		})
	}
	return cols, nil
}

// QueryRead executes a read-only SELECT query and returns up to limit rows.
func (b *bigQueryBackend) QueryRead(ctx context.Context, query string, limit int) (*QueryResult, error) {
	bounded := injectBQLimit(query, limit)

	it, err := b.client.Query(ctx, bounded)
	if err != nil {
		return nil, fmt.Errorf("BigQuery query failed: %w", err)
	}

	var allRows []any
	var colNames []string
	colsExtracted := false

	for {
		var row map[string]bigquery.Value
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("BigQuery row iteration failed: %w", err)
		}
		if !colsExtracted {
			for _, field := range it.Schema() {
				colNames = append(colNames, field.Name)
			}
			colsExtracted = true
		}
		plain := make(map[string]any, len(row))
		for k, v := range row {
			plain[k] = v
		}
		allRows = append(allRows, plain)
		if len(allRows) >= limit {
			break
		}
	}

	return &QueryResult{
		Columns: colNames,
		Rows:    allRows,
		Count:   len(allRows),
	}, nil
}

// injectBQLimit appends a LIMIT clause if not already present.
func injectBQLimit(query string, limit int) string {
	upper := strings.ToUpper(strings.TrimSpace(query))
	if strings.Contains(upper, " LIMIT ") || strings.HasSuffix(upper, "LIMIT") {
		return query
	}
	return fmt.Sprintf("%s LIMIT %d", strings.TrimRight(query, "; \t\n"), limit)
}

// ShowLocks returns an advisory message; BigQuery does not expose lock info.
func (b *bigQueryBackend) ShowLocks(_ context.Context) ([]LockInfo, error) {
	return []LockInfo{
		{
			PID:   0,
			Query: "N/A",
			State: "BigQuery is a serverless engine and does not expose lock or session information.",
		},
	}, nil
}

// Close closes the BigQuery client.
func (b *bigQueryBackend) Close() error {
	return b.client.Close()
}
