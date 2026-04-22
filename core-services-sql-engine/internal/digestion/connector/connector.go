package connector

import (
	"context"
	"fmt"
	"time"
)

// SourceConfig holds the connection parameters for a source database.
type SourceConfig struct {
	Type     string // "postgres", "mysql"
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

// TableMeta is the schema-level metadata for a discovered table.
type TableMeta struct {
	Schema          string
	Name            string
	EstimatedRows   int64
	PrimaryKeys     []string
	TimestampColumn string // best-guess timestamp column for incremental sync
}

// ColumnMeta is the schema-level metadata for a discovered column.
type ColumnMeta struct {
	Name            string
	OrdinalPosition int
	DataType        string
	MappedType      string // normalized type: text, integer, float, boolean, datetime, json, uuid
	MaxLength       int
	NumericPrecision int
	NumericScale    int
	IsNullable      bool
	IsPrimaryKey    bool
	IsUnique        bool
	HasDefault      bool
	DefaultValue    string
	SemanticType    string // latitude, longitude, status_enum, email, phone, name, ...
}

// ForeignKeyMeta describes a foreign key relationship in the source.
type ForeignKeyMeta struct {
	ConstraintName string
	FromSchema     string
	FromTable      string
	FromColumn     string
	ToSchema       string
	ToTable        string
	ToColumn       string
}

// Row is a single row of data from the source, keyed by column name.
type Row map[string]any

// Connector is the interface every database driver must implement.
type Connector interface {
	Connect(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error

	DiscoverTables(ctx context.Context) ([]TableMeta, error)
	DiscoverColumns(ctx context.Context, schema, table string) ([]ColumnMeta, error)
	DiscoverForeignKeys(ctx context.Context) ([]ForeignKeyMeta, error)

	// StreamRows returns a channel of rows and an error channel.
	// since is nil for a full snapshot, or a timestamp for incremental sync.
	// The caller must drain both channels until they are closed.
	StreamRows(ctx context.Context, schema, table string, since *time.Time) (<-chan Row, <-chan error)

	CountRows(ctx context.Context, schema, table string) (int64, error)
}

// New creates a Connector for the given source configuration.
func New(cfg SourceConfig) (Connector, error) {
	switch cfg.Type {
	case "postgres":
		return newPostgres(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported connector type %q", cfg.Type)
	}
}
