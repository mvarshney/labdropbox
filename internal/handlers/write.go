package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/maneesh/labdropbox/internal/chunker"
	"github.com/maneesh/labdropbox/internal/models"
	"github.com/maneesh/labdropbox/internal/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("labdropbox-handlers")

// WriteHandler handles file upload requests
type WriteHandler struct {
	minioClient *storage.MinioClient
	tidbClient  *storage.TiDBClient
	redisClient *storage.RedisClient
	chunker     *chunker.Chunker
}

// NewWriteHandler creates a new write handler
func NewWriteHandler(
	minioClient *storage.MinioClient,
	tidbClient *storage.TiDBClient,
	redisClient *storage.RedisClient,
	chunker *chunker.Chunker,
) *WriteHandler {
	return &WriteHandler{
		minioClient: minioClient,
		tidbClient:  tidbClient,
		redisClient: redisClient,
		chunker:     chunker,
	}
}

// WriteResponse represents the response for a write operation
type WriteResponse struct {
	FileID     string `json:"file_id"`
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	ChunkCount int    `json:"chunk_count"`
	Message    string `json:"message"`
}

// ServeHTTP handles PUT /write?name=filename
func (wh *WriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "write_file",
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()

	// Get filename from query parameter
	filename := r.URL.Query().Get("name")
	if filename == "" {
		http.Error(w, "missing 'name' query parameter", http.StatusBadRequest)
		return
	}

	span.SetAttributes(attribute.String("file_name", filename))

	// Generate file ID
	fileID := uuid.New().String()
	span.SetAttributes(attribute.String("file_id", fileID))

	// Step 1: Chunk the stream
	log.Printf("Chunking file: %s (ID: %s)", filename, fileID)
	chunks, totalSize, err := wh.chunkStream(ctx, r.Body)
	if err != nil {
		span.RecordError(err)
		http.Error(w, fmt.Sprintf("failed to chunk file: %v", err), http.StatusInternalServerError)
		return
	}

	span.SetAttributes(
		attribute.Int64("file_size", totalSize),
		attribute.Int("chunk_count", len(chunks)),
	)

	log.Printf("File chunked: %d chunks, total size: %d bytes", len(chunks), totalSize)

	// Step 2: Upload chunks to MinIO
	log.Printf("Uploading chunks to MinIO...")
	chunkModels, err := wh.uploadChunks(ctx, fileID, chunks)
	if err != nil {
		span.RecordError(err)
		http.Error(w, fmt.Sprintf("failed to upload chunks: %v", err), http.StatusInternalServerError)
		return
	}

	// Step 3: Save metadata to TiDB
	log.Printf("Saving metadata to TiDB...")
	file := &models.File{
		ID:         fileID,
		Name:       filename,
		Size:       totalSize,
		ChunkCount: len(chunks),
		CreatedAt:  time.Now(),
	}

	if err := wh.saveMetadata(ctx, file, chunkModels); err != nil {
		span.RecordError(err)
		http.Error(w, fmt.Sprintf("failed to save metadata: %v", err), http.StatusInternalServerError)
		return
	}

	// Step 4: Invalidate cache (if file was previously cached)
	log.Printf("Invalidating cache...")
	if err := wh.invalidateCache(ctx, fileID); err != nil {
		// Log error but don't fail the request
		log.Printf("Warning: failed to invalidate cache: %v", err)
	}

	// Return success response
	response := WriteResponse{
		FileID:     fileID,
		FileName:   filename,
		FileSize:   totalSize,
		ChunkCount: len(chunks),
		Message:    "File uploaded successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)

	log.Printf("File upload completed: %s (ID: %s)", filename, fileID)
}

func (wh *WriteHandler) chunkStream(ctx context.Context, body io.ReadCloser) ([]*models.ChunkData, int64, error) {
	ctx, span := tracer.Start(ctx, "chunk_stream")
	defer span.End()
	defer body.Close()

	return wh.chunker.ChunkStream(body)
}

func (wh *WriteHandler) uploadChunks(ctx context.Context, fileID string, chunks []*models.ChunkData) ([]*models.Chunk, error) {
	ctx, span := tracer.Start(ctx, "upload_chunks",
		trace.WithAttributes(
			attribute.Int("chunk_count", len(chunks)),
		),
	)
	defer span.End()

	var chunkModels []*models.Chunk

	for _, chunkData := range chunks {
		// Generate chunk ID and MinIO object key
		chunkID := uuid.New().String()
		objectKey := fmt.Sprintf("chunks/%s/%d", fileID, chunkData.OrderIndex)

		// Upload to MinIO
		if err := wh.minioClient.UploadChunk(ctx, objectKey, chunkData.Data); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to upload chunk %d: %w", chunkData.OrderIndex, err)
		}

		// Create chunk model
		chunk := &models.Chunk{
			ID:             chunkID,
			FileID:         fileID,
			OrderIndex:     chunkData.OrderIndex,
			Hash:           chunkData.Hash,
			MinioObjectKey: objectKey,
			Size:           chunkData.Size,
		}

		chunkModels = append(chunkModels, chunk)
	}

	span.SetAttributes(attribute.Int("chunks_uploaded", len(chunkModels)))
	return chunkModels, nil
}

func (wh *WriteHandler) saveMetadata(ctx context.Context, file *models.File, chunks []*models.Chunk) error {
	ctx, span := tracer.Start(ctx, "save_metadata")
	defer span.End()

	// Create file record
	if err := wh.tidbClient.CreateFile(ctx, file); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to create file record: %w", err)
	}

	// Create chunk records
	for _, chunk := range chunks {
		if err := wh.tidbClient.CreateChunk(ctx, chunk); err != nil {
			span.RecordError(err)
			return fmt.Errorf("failed to create chunk record: %w", err)
		}
	}

	span.SetAttributes(attribute.Bool("metadata_saved", true))
	return nil
}

func (wh *WriteHandler) invalidateCache(ctx context.Context, fileID string) error {
	ctx, span := tracer.Start(ctx, "invalidate_cache")
	defer span.End()

	return wh.redisClient.InvalidateFileMetadata(ctx, fileID)
}
