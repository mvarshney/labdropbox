package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/maneesh/labdropbox/internal/chunker"
	"github.com/maneesh/labdropbox/internal/config"
	"github.com/maneesh/labdropbox/internal/handlers"
	"github.com/maneesh/labdropbox/internal/storage"
	"github.com/maneesh/labdropbox/internal/tracing"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	log.Println("Starting LabDropbox service...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Service: %s, Port: %s", cfg.ServiceName, cfg.ServicePort)

	// Initialize OpenTelemetry tracing
	shutdownTracer, err := tracing.InitTracer(cfg.ServiceName, cfg.JaegerEndpoint)
	if err != nil {
		log.Fatalf("Failed to initialize tracer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracer(ctx); err != nil {
			log.Printf("Error shutting down tracer: %v", err)
		}
	}()

	// Initialize MinIO client
	log.Println("Connecting to MinIO...")
	minioClient, err := storage.NewMinioClient(
		cfg.MinIOEndpoint,
		cfg.MinIOAccessKey,
		cfg.MinIOSecretKey,
		cfg.MinIOBucketName,
		cfg.MinIOUseSSL,
	)
	if err != nil {
		log.Fatalf("Failed to initialize MinIO client: %v", err)
	}
	log.Println("MinIO client initialized")

	// Initialize TiDB client
	log.Println("Connecting to TiDB...")
	tidbClient, err := storage.NewTiDBClient(cfg.GetDSN())
	if err != nil {
		log.Fatalf("Failed to initialize TiDB client: %v", err)
	}
	defer tidbClient.Close()
	log.Println("TiDB client initialized")

	// Initialize Redis client
	log.Println("Connecting to Redis...")
	redisClient, err := storage.NewRedisClient(cfg.GetRedisAddr(), cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatalf("Failed to initialize Redis client: %v", err)
	}
	defer redisClient.Close()
	log.Println("Redis client initialized")

	// Initialize chunker
	chunkerInstance := chunker.NewChunker(cfg.GetChunkSizeBytes())

	// Initialize handlers
	writeHandler := handlers.NewWriteHandler(minioClient, tidbClient, redisClient, chunkerInstance)
	readHandler := handlers.NewReadHandler(minioClient, tidbClient, redisClient)

	// Setup HTTP router
	router := mux.NewRouter()

	// Health check endpoint (no tracing needed)
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// File operations with tracing
	router.Handle("/write", otelhttp.NewHandler(writeHandler, "PUT /write")).Methods("PUT")
	router.Handle("/read/{file_id}", otelhttp.NewHandler(readHandler, "GET /read/{file_id}")).Methods("GET")

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.ServicePort,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server listening on port %s", cfg.ServicePort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
