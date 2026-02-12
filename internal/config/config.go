package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all application configuration
type Config struct {
	// Service configuration
	ServicePort  string
	ChunkSizeMB  int
	ServiceName  string

	// MinIO configuration
	MinIOEndpoint   string
	MinIOAccessKey  string
	MinIOSecretKey  string
	MinIOBucketName string
	MinIOUseSSL     bool

	// TiDB configuration
	TiDBHost     string
	TiDBPort     string
	TiDBUser     string
	TiDBPassword string
	TiDBDatabase string

	// Redis configuration
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	// Jaeger configuration
	JaegerEndpoint string
}

// LoadConfig loads configuration from environment variables with sensible defaults
func LoadConfig() (*Config, error) {
	config := &Config{
		// Service defaults
		ServicePort:  getEnv("SERVICE_PORT", "8080"),
		ChunkSizeMB:  getEnvAsInt("CHUNK_SIZE_MB", 1),
		ServiceName:  getEnv("SERVICE_NAME", "labdropbox-service"),

		// MinIO defaults
		MinIOEndpoint:   getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:  getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:  getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOBucketName: getEnv("MINIO_BUCKET_NAME", "labdropbox"),
		MinIOUseSSL:     getEnvAsBool("MINIO_USE_SSL", false),

		// TiDB defaults
		TiDBHost:     getEnv("TIDB_HOST", "localhost"),
		TiDBPort:     getEnv("TIDB_PORT", "4000"),
		TiDBUser:     getEnv("TIDB_USER", "root"),
		TiDBPassword: getEnv("TIDB_PASSWORD", ""),
		TiDBDatabase: getEnv("TIDB_DATABASE", "labdropbox"),

		// Redis defaults
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		// Jaeger defaults
		JaegerEndpoint: getEnv("JAEGER_ENDPOINT", "http://localhost:4318"),
	}

	return config, nil
}

// GetDSN returns the TiDB connection string
func (c *Config) GetDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.TiDBUser,
		c.TiDBPassword,
		c.TiDBHost,
		c.TiDBPort,
		c.TiDBDatabase,
	)
}

// GetRedisAddr returns the Redis address
func (c *Config) GetRedisAddr() string {
	return fmt.Sprintf("%s:%s", c.RedisHost, c.RedisPort)
}

// GetChunkSizeBytes returns chunk size in bytes
func (c *Config) GetChunkSizeBytes() int64 {
	return int64(c.ChunkSizeMB) * 1024 * 1024
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := getEnv(key, "")
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}
	return defaultValue
}
