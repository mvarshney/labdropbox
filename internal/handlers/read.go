package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/maneesh/labdropbox/internal/chunker"
	"github.com/maneesh/labdropbox/internal/models"
	"github.com/maneesh/labdropbox/internal/storage"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ReadHandler handles file download requests
type ReadHandler struct {
	minioClient *storage.MinioClient
	tidbClient  *storage.TiDBClient
	redisClient *storage.RedisClient
}

// NewReadHandler creates a new read handler
func NewReadHandler(
	minioClient *storage.MinioClient,
	tidbClient *storage.TiDBClient,
	redisClient *storage.RedisClient,
) *ReadHandler {
	return &ReadHandler{
		minioClient: minioClient,
		tidbClient:  tidbClient,
		redisClient: redisClient,
	}
}

// ServeHTTP handles GET /read/{file_id}
func (rh *ReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "read_file",
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()

	// Get file ID from URL path
	vars := mux.Vars(r)
	fileID := vars["file_id"]
	if fileID == "" {
		http.Error(w, "missing file_id in path", http.StatusBadRequest)
		return
	}

	span.SetAttributes(attribute.String("file_id", fileID))
	log.Printf("Reading file: %s", fileID)

	// Step 1: Try to get file metadata from cache
	file, err := rh.getFileMetadata(ctx, fileID)
	if err != nil {
		span.RecordError(err)
		http.Error(w, fmt.Sprintf("failed to get file metadata: %v", err), http.StatusInternalServerError)
		return
	}

	if file == nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	span.SetAttributes(
		attribute.String("file_name", file.Name),
		attribute.Int64("file_size", file.Size),
		attribute.Int("chunk_count", file.ChunkCount),
	)

	// Step 2: Get chunk metadata from TiDB
	chunks, err := rh.getChunkMetadata(ctx, fileID)
	if err != nil {
		span.RecordError(err)
		http.Error(w, fmt.Sprintf("failed to get chunks: %v", err), http.StatusInternalServerError)
		return
	}

	// Step 3: Fetch chunks from MinIO in parallel (THE KEY FEATURE!)
	log.Printf("Fetching %d chunks in parallel...", len(chunks))
	chunkData, err := rh.fetchChunksParallel(ctx, chunks)
	if err != nil {
		span.RecordError(err)
		http.Error(w, fmt.Sprintf("failed to fetch chunks: %v", err), http.StatusInternalServerError)
		return
	}

	// Step 4: Reassemble chunks
	log.Printf("Reassembling chunks...")
	fileData := rh.reassembleFile(ctx, chunkData)

	// Step 5: Stream response
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.Name))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileData)))
	w.WriteHeader(http.StatusOK)
	w.Write(fileData)

	log.Printf("File read completed: %s (ID: %s)", file.Name, fileID)
}

func (rh *ReadHandler) getFileMetadata(ctx context.Context, fileID string) (*models.File, error) {
	// Try cache first
	ctx, cacheSpan := tracer.Start(ctx, "cache_lookup")
	file, err := rh.redisClient.GetFileMetadata(ctx, fileID)
	cacheSpan.End()

	if err != nil {
		return nil, err
	}

	if file != nil {
		log.Printf("Cache HIT for file: %s", fileID)
		return file, nil
	}

	// Cache miss - fetch from TiDB
	log.Printf("Cache MISS for file: %s", fileID)
	ctx, dbSpan := tracer.Start(ctx, "db_lookup")
	defer dbSpan.End()

	file, err = rh.tidbClient.GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}

	// Update cache for next time
	if err := rh.redisClient.SetFileMetadata(ctx, fileID, file); err != nil {
		log.Printf("Warning: failed to update cache: %v", err)
	}

	return file, nil
}

func (rh *ReadHandler) getChunkMetadata(ctx context.Context, fileID string) ([]*models.Chunk, error) {
	ctx, span := tracer.Start(ctx, "fetch_chunk_metadata")
	defer span.End()

	return rh.tidbClient.GetChunks(ctx, fileID)
}

// fetchChunksParallel fetches chunks from MinIO in parallel with proper tracing
// This is THE critical function for demonstrating parallel spans in Jaeger!
func (rh *ReadHandler) fetchChunksParallel(ctx context.Context, chunkMetadata []*models.Chunk) ([][]byte, error) {
	// Create parent span for parallel chunk fetching
	ctx, fetchSpan := tracer.Start(ctx, "fetch_chunks_parallel",
		trace.WithAttributes(
			attribute.Int("chunk_count", len(chunkMetadata)),
		),
	)
	defer fetchSpan.End()

	// Prepare slice to hold chunk data in order
	chunkData := make([][]byte, len(chunkMetadata))
	var wg sync.WaitGroup
	errChan := make(chan error, len(chunkMetadata))

	// Launch parallel goroutines to fetch each chunk
	for i, meta := range chunkMetadata {
		wg.Add(1)
		go func(idx int, chunkMeta *models.Chunk) {
			defer wg.Done()

			// CRITICAL: Create child span with propagated context
			// This ensures each goroutine's work appears as a parallel span in Jaeger
			_, chunkSpan := tracer.Start(ctx, fmt.Sprintf("download_chunk_%d", idx),
				trace.WithAttributes(
					attribute.Int("chunk_index", idx),
					attribute.String("object_key", chunkMeta.MinioObjectKey),
					attribute.Int64("chunk_size", chunkMeta.Size),
				),
			)
			defer chunkSpan.End()

			// Download chunk from MinIO
			data, err := rh.minioClient.DownloadChunk(ctx, chunkMeta.MinioObjectKey)
			if err != nil {
				chunkSpan.RecordError(err)
				errChan <- fmt.Errorf("failed to download chunk %d: %w", idx, err)
				return
			}

			// Verify hash (optional but good practice)
			if !chunker.VerifyChunkHash(data, chunkMeta.Hash) {
				err := fmt.Errorf("hash mismatch for chunk %d", idx)
				chunkSpan.RecordError(err)
				errChan <- err
				return
			}

			// Store in ordered slice
			chunkData[idx] = data
			chunkSpan.SetAttributes(attribute.Bool("download_success", true))

		}(i, meta)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	if len(errChan) > 0 {
		err := <-errChan
		fetchSpan.RecordError(err)
		return nil, err
	}

	fetchSpan.SetAttributes(attribute.Bool("all_chunks_fetched", true))
	return chunkData, nil
}

func (rh *ReadHandler) reassembleFile(ctx context.Context, chunkData [][]byte) []byte {
	ctx, span := tracer.Start(ctx, "reassemble_chunks",
		trace.WithAttributes(
			attribute.Int("chunk_count", len(chunkData)),
		),
	)
	defer span.End()

	return chunker.ReassembleChunks(chunkData)
}
