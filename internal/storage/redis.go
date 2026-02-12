package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/maneesh/labdropbox/internal/models"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// CacheTTL is the time-to-live for cached file metadata (5 minutes)
	CacheTTL = 5 * time.Minute
)

// RedisClient wraps Redis operations with tracing
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient initializes a new Redis client
func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test the connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// Close closes the Redis connection
func (rc *RedisClient) Close() error {
	return rc.client.Close()
}

// GetFileMetadata retrieves file metadata from cache with tracing
func (rc *RedisClient) GetFileMetadata(ctx context.Context, fileID string) (*models.File, error) {
	ctx, span := tracer.Start(ctx, "redis.get_file_metadata",
		trace.WithAttributes(
			attribute.String("file_id", fileID),
		),
	)
	defer span.End()

	key := fmt.Sprintf("file:%s", fileID)
	data, err := rc.client.Get(ctx, key).Result()

	if err == redis.Nil {
		span.SetAttributes(
			attribute.Bool("cache_hit", false),
			attribute.String("cache_status", "miss"),
		)
		return nil, nil // Cache miss, not an error
	} else if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get from cache: %w", err)
	}

	var file models.File
	if err := json.Unmarshal([]byte(data), &file); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to unmarshal cached data: %w", err)
	}

	span.SetAttributes(
		attribute.Bool("cache_hit", true),
		attribute.String("cache_status", "hit"),
	)
	return &file, nil
}

// SetFileMetadata stores file metadata in cache with tracing
func (rc *RedisClient) SetFileMetadata(ctx context.Context, fileID string, file *models.File) error {
	ctx, span := tracer.Start(ctx, "redis.set_file_metadata",
		trace.WithAttributes(
			attribute.String("file_id", fileID),
			attribute.String("file_name", file.Name),
		),
	)
	defer span.End()

	key := fmt.Sprintf("file:%s", fileID)
	data, err := json.Marshal(file)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal file: %w", err)
	}

	err = rc.client.Set(ctx, key, data, CacheTTL).Err()
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to set cache: %w", err)
	}

	span.SetAttributes(
		attribute.Bool("cache_set_success", true),
		attribute.Int64("ttl_seconds", int64(CacheTTL.Seconds())),
	)
	return nil
}

// InvalidateFileMetadata removes file metadata from cache with tracing
func (rc *RedisClient) InvalidateFileMetadata(ctx context.Context, fileID string) error {
	ctx, span := tracer.Start(ctx, "redis.invalidate_file_metadata",
		trace.WithAttributes(
			attribute.String("file_id", fileID),
		),
	)
	defer span.End()

	key := fmt.Sprintf("file:%s", fileID)
	err := rc.client.Del(ctx, key).Err()
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to invalidate cache: %w", err)
	}

	span.SetAttributes(attribute.Bool("cache_invalidate_success", true))
	return nil
}
