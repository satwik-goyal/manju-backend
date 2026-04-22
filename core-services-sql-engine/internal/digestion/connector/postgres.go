package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresConnector struct {
	cfg  SourceConfig
	pool *pgxpool.Pool
}

func newPostgres(cfg SourceConfig) *postgresConnector {
	return &postgresConnector{cfg: cfg}
}

func (p *postgresConnector) Connect(ctx context.Context) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		p.cfg.Host, p.cfg.Port, p.cfg.Database,
		p.cfg.Username, p.cfg.Password, p.cfg.SSLMode,
	)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to source: %w", err)
	}
	p.pool = pool
	return nil
}

func (p *postgresConnector) Close() error {
	if p.pool != nil {
		p.pool.Close()
	}
	return nil
}

func (p *postgresConnector) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *postgresConnector) DiscoverTables(ctx context.Context) ([]TableMeta, error) {
	const q = `
		SELECT
			n.nspname AS schema_name,
			c.relname  AS table_name,
			GREATEST(c.reltuples::BIGINT, 0) AS estimated_rows
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r'
		  AND n.nspname NOT IN ('pg_catalog','information_schema','pg_toast')
		ORDER BY n.nspname, c.relname`

	rows, err := p.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("discover tables: %w", err)
	}
	defer rows.Close()

	var tables []TableMeta
	for rows.Next() {
		var t TableMeta
		if err := rows.Scan(&t.Schema, &t.Name, &t.EstimatedRows); err != nil {
			return nil, err
		}
		// Discover PKs for each table inline.
		pks, err := p.discoverPrimaryKeys(ctx, t.Schema, t.Name)
		if err != nil {
			return nil, err
		}
		t.PrimaryKeys = pks
		t.TimestampColumn = guessTimestampColumn(ctx, p.pool, t.Schema, t.Name)
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (p *postgresConnector) discoverPrimaryKeys(ctx context.Context, schema, table string) ([]string, error) {
	const q = `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON kcu.constraint_name = tc.constraint_name
			AND kcu.table_schema   = tc.table_schema
			AND kcu.table_name     = tc.table_name
		WHERE tc.constraint_type = 'PRIMARY KEY'
		  AND tc.table_schema    = $1
		  AND tc.table_name      = $2
		ORDER BY kcu.ordinal_position`

	rows, err := p.pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

// guessTimestampColumn returns the name of the most likely "updated_at" style
// column, or "" if none found. Errors are silently ignored.
func guessTimestampColumn(ctx context.Context, pool *pgxpool.Pool, schema, table string) string {
	const q = `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		  AND data_type IN ('timestamp without time zone','timestamp with time zone')
		ORDER BY
			CASE column_name
				WHEN 'updated_at' THEN 0
				WHEN 'modified_at' THEN 1
				WHEN 'last_modified' THEN 2
				WHEN 'created_at' THEN 3
				ELSE 10
			END
		LIMIT 1`

	var col string
	_ = pool.QueryRow(ctx, q, schema, table).Scan(&col)
	return col
}

func (p *postgresConnector) DiscoverColumns(ctx context.Context, schema, table string) ([]ColumnMeta, error) {
	const q = `
		SELECT
			c.column_name,
			c.ordinal_position,
			c.data_type,
			COALESCE(c.character_maximum_length, 0),
			COALESCE(c.numeric_precision, 0),
			COALESCE(c.numeric_scale, 0),
			c.is_nullable = 'YES',
			c.column_default IS NOT NULL,
			COALESCE(c.column_default, ''),
			CASE WHEN kcu.column_name IS NOT NULL THEN true ELSE false END AS is_pk
		FROM information_schema.columns c
		LEFT JOIN (
			SELECT kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
				ON kcu.constraint_name = tc.constraint_name
				AND kcu.table_schema   = tc.table_schema
				AND kcu.table_name     = tc.table_name
			WHERE tc.constraint_type = 'PRIMARY KEY'
			  AND tc.table_schema    = $1
			  AND tc.table_name      = $2
		) kcu ON kcu.column_name = c.column_name
		WHERE c.table_schema = $1 AND c.table_name = $2
		ORDER BY c.ordinal_position`

	rows, err := p.pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, fmt.Errorf("discover columns: %w", err)
	}
	defer rows.Close()

	var cols []ColumnMeta
	for rows.Next() {
		var col ColumnMeta
		if err := rows.Scan(
			&col.Name, &col.OrdinalPosition, &col.DataType,
			&col.MaxLength, &col.NumericPrecision, &col.NumericScale,
			&col.IsNullable, &col.HasDefault, &col.DefaultValue,
			&col.IsPrimaryKey,
		); err != nil {
			return nil, err
		}
		col.MappedType = mapType(col.DataType)
		col.SemanticType = detectSemanticType(col.Name, col.DataType, col.IsPrimaryKey)
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

func (p *postgresConnector) DiscoverForeignKeys(ctx context.Context) ([]ForeignKeyMeta, error) {
	const q = `
		SELECT
			tc.constraint_name,
			tc.table_schema  AS from_schema,
			tc.table_name    AS from_table,
			kcu.column_name  AS from_column,
			ccu.table_schema AS to_schema,
			ccu.table_name   AS to_table,
			ccu.column_name  AS to_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema   = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema   = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		ORDER BY from_table, from_column`

	rows, err := p.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("discover foreign keys: %w", err)
	}
	defer rows.Close()

	var fks []ForeignKeyMeta
	for rows.Next() {
		var fk ForeignKeyMeta
		if err := rows.Scan(
			&fk.ConstraintName, &fk.FromSchema, &fk.FromTable, &fk.FromColumn,
			&fk.ToSchema, &fk.ToTable, &fk.ToColumn,
		); err != nil {
			return nil, err
		}
		fks = append(fks, fk)
	}
	return fks, rows.Err()
}

func (p *postgresConnector) StreamRows(ctx context.Context, schema, table string, since *time.Time) (<-chan Row, <-chan error) {
	rowCh := make(chan Row, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(rowCh)
		defer close(errCh)

		var q string
		var args []any
		if since != nil {
			// Timestamp-based incremental — best-effort, caller ensures column exists.
			q = fmt.Sprintf(
				`SELECT * FROM %s.%s WHERE updated_at > $1 ORDER BY updated_at`,
				pgx.Identifier{schema}.Sanitize(),
				pgx.Identifier{table}.Sanitize(),
			)
			args = append(args, *since)
		} else {
			q = fmt.Sprintf(`SELECT * FROM %s.%s`,
				pgx.Identifier{schema}.Sanitize(),
				pgx.Identifier{table}.Sanitize(),
			)
		}

		rows, err := p.pool.Query(ctx, q, args...)
		if err != nil {
			errCh <- fmt.Errorf("stream rows from %s.%s: %w", schema, table, err)
			return
		}
		defer rows.Close()

		fields := rows.FieldDescriptions()
		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				errCh <- err
				return
			}
			row := make(Row, len(fields))
			for i, fd := range fields {
				row[string(fd.Name)] = normalizeValue(values[i])
			}
			select {
			case rowCh <- row:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}
		if err := rows.Err(); err != nil {
			errCh <- err
		}
	}()

	return rowCh, errCh
}

func (p *postgresConnector) CountRows(ctx context.Context, schema, table string) (int64, error) {
	q := fmt.Sprintf(`SELECT COUNT(*) FROM %s.%s`,
		pgx.Identifier{schema}.Sanitize(),
		pgx.Identifier{table}.Sanitize(),
	)
	var n int64
	err := p.pool.QueryRow(ctx, q).Scan(&n)
	return n, err
}

// normalizeValue converts pgx-native types into JSON-friendly Go values.
func normalizeValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case [16]byte: // UUID
		return fmt.Sprintf("%x-%x-%x-%x-%x", val[0:4], val[4:6], val[6:8], val[8:10], val[10:])
	case []byte:
		return string(val) // JSONB / bytea
	case time.Time:
		return val.UTC().Format(time.RFC3339)
	default:
		return val
	}
}

// mapType converts PostgreSQL data types to a simplified mapped type.
func mapType(pgType string) string {
	switch {
	case strings.Contains(pgType, "int"):
		return "integer"
	case strings.Contains(pgType, "numeric"), strings.Contains(pgType, "decimal"),
		pgType == "real", strings.Contains(pgType, "double"):
		return "float"
	case strings.Contains(pgType, "char"), pgType == "text", pgType == "name":
		return "text"
	case pgType == "boolean":
		return "boolean"
	case strings.Contains(pgType, "timestamp"), pgType == "date", pgType == "time":
		return "datetime"
	case pgType == "json", pgType == "jsonb":
		return "json"
	case pgType == "uuid":
		return "uuid"
	case strings.Contains(pgType, "geometry"), strings.Contains(pgType, "geography"):
		return "geometry"
	default:
		return "text"
	}
}

// detectSemanticType infers business meaning from the column name and type.
func detectSemanticType(name, dataType string, isPK bool) string {
	lower := strings.ToLower(name)
	if isPK {
		return "primary_key"
	}
	switch {
	case strings.HasSuffix(lower, "_id") || strings.HasSuffix(lower, "id"):
		return "foreign_key"
	case lower == "latitude" || lower == "lat":
		return "latitude"
	case lower == "longitude" || lower == "lng" || lower == "lon":
		return "longitude"
	case lower == "status":
		return "status_enum"
	case strings.Contains(lower, "email"):
		return "email"
	case strings.Contains(lower, "phone"):
		return "phone"
	case lower == "name" || strings.HasSuffix(lower, "_name"):
		return "name"
	case strings.Contains(lower, "updated_at") || strings.Contains(lower, "modified_at"):
		return "updated_at"
	case strings.Contains(lower, "created_at"):
		return "created_at"
	default:
		return ""
	}
}
