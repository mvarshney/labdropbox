package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("labdropbox-storage")

// MinioClient wraps MinIO operations with tracing
type MinioClient struct {
	client     *minio.Client
	bucketName string
}

// NewMinioClient initializes a new MinIO client
func NewMinioClient(endpoint, accessKey, secretKey, bucketName string, useSSL bool) (*MinioClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	mc := &MinioClient{
		client:     client,
		bucketName: bucketName,
	}

	// Ensure bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		log.Printf("Creating bucket: %s", bucketName)
		err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
		log.Printf("Bucket %s created successfully", bucketName)
	}

	return mc, nil
}

// UploadChunk uploads a chunk to MinIO with tracing
func (mc *MinioClient) UploadChunk(ctx context.Context, objectKey string, data []byte) error {
	ctx, span := tracer.Start(ctx, "minio.upload_chunk",
		trace.WithAttributes(
			attribute.String("object_key", objectKey),
			attribute.Int("size_bytes", len(data)),
		),
	)
	defer span.End()

	reader := bytes.NewReader(data)
	_, err := mc.client.PutObject(ctx, mc.bucketName, objectKey, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to upload chunk: %w", err)
	}

	span.SetAttributes(attribute.Bool("upload_success", true))
	return nil
}

// DownloadChunk downloads a chunk from MinIO with tracing
func (mc *MinioClient) DownloadChunk(ctx context.Context, objectKey string) ([]byte, error) {
	ctx, span := tracer.Start(ctx, "minio.download_chunk",
		trace.WithAttributes(
			attribute.String("object_key", objectKey),
		),
	)
	defer span.End()

	object, err := mc.client.GetObject(ctx, mc.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	defer object.Close()

	data, err := io.ReadAll(object)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to read object data: %w", err)
	}

	span.SetAttributes(
		attribute.Int("size_bytes", len(data)),
		attribute.Bool("download_success", true),
	)
	return data, nil
}

// DeleteChunk deletes a chunk from MinIO
func (mc *MinioClient) DeleteChunk(ctx context.Context, objectKey string) error {
	ctx, span := tracer.Start(ctx, "minio.delete_chunk",
		trace.WithAttributes(
			attribute.String("object_key", objectKey),
		),
	)
	defer span.End()

	err := mc.client.RemoveObject(ctx, mc.bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete chunk: %w", err)
	}

	return nil
}
