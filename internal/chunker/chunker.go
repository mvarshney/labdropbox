package chunker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/maneesh/labdropbox/internal/models"
)

// Chunker handles file chunking and reassembly
type Chunker struct {
	chunkSize int64
}

// NewChunker creates a new chunker with the specified chunk size
func NewChunker(chunkSize int64) *Chunker {
	return &Chunker{
		chunkSize: chunkSize,
	}
}

// ChunkStream reads from a reader and yields chunks of specified size
func (c *Chunker) ChunkStream(reader io.Reader) ([]*models.ChunkData, int64, error) {
	var chunks []*models.ChunkData
	var totalSize int64
	orderIndex := 0

	for {
		buffer := make([]byte, c.chunkSize)
		n, err := io.ReadFull(reader, buffer)

		if n > 0 {
			// Trim buffer to actual size read
			chunkData := buffer[:n]
			hash := ComputeHash(chunkData)

			chunk := &models.ChunkData{
				Data:       chunkData,
				OrderIndex: orderIndex,
				Hash:       hash,
				Size:       int64(n),
			}

			chunks = append(chunks, chunk)
			totalSize += int64(n)
			orderIndex++
		}

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		} else if err != nil {
			return nil, 0, fmt.Errorf("error reading chunk: %w", err)
		}
	}

	return chunks, totalSize, nil
}

// ComputeHash computes SHA256 hash of data
func ComputeHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ReassembleChunks combines chunks in order
func ReassembleChunks(chunks [][]byte) []byte {
	// Calculate total size
	totalSize := 0
	for _, chunk := range chunks {
		totalSize += len(chunk)
	}

	// Allocate buffer
	result := make([]byte, 0, totalSize)

	// Append all chunks
	for _, chunk := range chunks {
		result = append(result, chunk...)
	}

	return result
}

// VerifyChunkHash verifies that chunk data matches the expected hash
func VerifyChunkHash(data []byte, expectedHash string) bool {
	actualHash := ComputeHash(data)
	return actualHash == expectedHash
}
