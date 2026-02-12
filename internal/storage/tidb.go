package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/maneesh/labdropbox/internal/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TiDBClient wraps TiDB operations with tracing
type TiDBClient struct {
	db *sql.DB
}

// NewTiDBClient initializes a new TiDB client
func NewTiDBClient(dsn string) (*TiDBClient, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	return &TiDBClient{db: db}, nil
}

// Close closes the database connection
func (tc *TiDBClient) Close() error {
	return tc.db.Close()
}

// CreateFile inserts file metadata with tracing
func (tc *TiDBClient) CreateFile(ctx context.Context, file *models.File) error {
	ctx, span := tracer.Start(ctx, "tidb.create_file",
		trace.WithAttributes(
			attribute.String("file_id", file.ID),
			attribute.String("file_name", file.Name),
			attribute.Int64("file_size", file.Size),
		),
	)
	defer span.End()

	query := `INSERT INTO files (id, name, size, chunk_count, created_at)
			  VALUES (?, ?, ?, ?, ?)`

	_, err := tc.db.ExecContext(ctx, query, file.ID, file.Name, file.Size, file.ChunkCount, file.CreatedAt)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to insert file: %w", err)
	}

	span.SetAttributes(attribute.Bool("insert_success", true))
	return nil
}

// CreateChunk inserts chunk metadata with tracing
func (tc *TiDBClient) CreateChunk(ctx context.Context, chunk *models.Chunk) error {
	ctx, span := tracer.Start(ctx, "tidb.create_chunk",
		trace.WithAttributes(
			attribute.String("chunk_id", chunk.ID),
			attribute.String("file_id", chunk.FileID),
			attribute.Int("order_index", chunk.OrderIndex),
		),
	)
	defer span.End()

	query := `INSERT INTO chunks (id, file_id, order_index, hash, minio_object_key, size)
			  VALUES (?, ?, ?, ?, ?, ?)`

	_, err := tc.db.ExecContext(ctx, query, chunk.ID, chunk.FileID, chunk.OrderIndex, chunk.Hash, chunk.MinioObjectKey, chunk.Size)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to insert chunk: %w", err)
	}

	span.SetAttributes(attribute.Bool("insert_success", true))
	return nil
}

// GetFile retrieves file metadata by ID with tracing
func (tc *TiDBClient) GetFile(ctx context.Context, fileID string) (*models.File, error) {
	ctx, span := tracer.Start(ctx, "tidb.get_file",
		trace.WithAttributes(
			attribute.String("file_id", fileID),
		),
	)
	defer span.End()

	query := `SELECT id, name, size, chunk_count, created_at FROM files WHERE id = ?`

	var file models.File
	err := tc.db.QueryRowContext(ctx, query, fileID).Scan(
		&file.ID,
		&file.Name,
		&file.Size,
		&file.ChunkCount,
		&file.CreatedAt,
	)

	if err == sql.ErrNoRows {
		span.SetAttributes(attribute.Bool("found", false))
		return nil, fmt.Errorf("file not found: %s", fileID)
	} else if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query file: %w", err)
	}

	span.SetAttributes(attribute.Bool("found", true))
	return &file, nil
}

// GetChunks retrieves all chunks for a file ordered by order_index with tracing
func (tc *TiDBClient) GetChunks(ctx context.Context, fileID string) ([]*models.Chunk, error) {
	ctx, span := tracer.Start(ctx, "tidb.get_chunks",
		trace.WithAttributes(
			attribute.String("file_id", fileID),
		),
	)
	defer span.End()

	query := `SELECT id, file_id, order_index, hash, minio_object_key, size
			  FROM chunks
			  WHERE file_id = ?
			  ORDER BY order_index ASC`

	rows, err := tc.db.QueryContext(ctx, query, fileID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []*models.Chunk
	for rows.Next() {
		var chunk models.Chunk
		err := rows.Scan(
			&chunk.ID,
			&chunk.FileID,
			&chunk.OrderIndex,
			&chunk.Hash,
			&chunk.MinioObjectKey,
			&chunk.Size,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		chunks = append(chunks, &chunk)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating chunks: %w", err)
	}

	span.SetAttributes(
		attribute.Int("chunk_count", len(chunks)),
		attribute.Bool("query_success", true),
	)
	return chunks, nil
}

// BeginTx starts a new transaction
func (tc *TiDBClient) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return tc.db.BeginTx(ctx, nil)
}
