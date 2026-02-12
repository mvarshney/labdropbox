-- Create database if not exists
CREATE DATABASE IF NOT EXISTS labdropbox DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE labdropbox;

-- Files table: stores file metadata
CREATE TABLE IF NOT EXISTS files (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(512) NOT NULL,
    size BIGINT NOT NULL,
    chunk_count INT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_name (name),
    INDEX idx_created_at (created_at)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Chunks table: stores chunk metadata
CREATE TABLE IF NOT EXISTS chunks (
    id VARCHAR(36) PRIMARY KEY,
    file_id VARCHAR(36) NOT NULL,
    order_index INT NOT NULL,
    hash VARCHAR(64) NOT NULL,
    minio_object_key VARCHAR(512) NOT NULL,
    size BIGINT NOT NULL,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
    UNIQUE KEY idx_file_order (file_id, order_index),
    INDEX idx_file_id (file_id),
    INDEX idx_hash (hash)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
