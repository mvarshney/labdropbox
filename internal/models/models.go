package models

import "time"

// File represents file metadata stored in TiDB
type File struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ChunkCount int       `json:"chunk_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// Chunk represents a chunk of a file
type Chunk struct {
	ID             string `json:"id"`
	FileID         string `json:"file_id"`
	OrderIndex     int    `json:"order_index"`
	Hash           string `json:"hash"`
	MinioObjectKey string `json:"minio_object_key"`
	Size           int64  `json:"size"`
}

// ChunkData holds chunk information during upload/download
type ChunkData struct {
	Data       []byte
	OrderIndex int
	Hash       string
	Size       int64
}
