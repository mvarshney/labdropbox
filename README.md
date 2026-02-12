# LabDropbox: Distributed File Storage with OpenTelemetry Tracing

A chunk-based file storage service built to demonstrate distributed systems concepts and observability through OpenTelemetry distributed tracing.

## Overview

LabDropbox bridges the gap between system design theory and hands-on distributed observability. The primary goal is to **visualize parallel chunk reads in Jaeger's trace waterfall view**, demonstrating how distributed tracing helps understand system behavior in real-time.

### Key Features

- **Chunked File Storage**: Files are split into 1MB chunks for distributed storage
- **Distributed Architecture**: MinIO (object storage) + TiDB (metadata) + Redis (caching)
- **OpenTelemetry Tracing**: Full distributed tracing with Jaeger visualization
- **Parallel Chunk Fetching**: Demonstrates concurrent operations in trace view
- **Read-through Caching**: Redis cache with automatic fallback to TiDB

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       ├── PUT /write?name=file.pdf
       │   ├─> Chunk file (1MB chunks)
       │   ├─> Upload chunks to MinIO
       │   ├─> Store metadata in TiDB
       │   └─> Invalidate Redis cache
       │
       └── GET /read/{file_id}
           ├─> Check Redis cache (metadata)
           ├─> Fallback to TiDB if cache miss
           ├─> Fetch chunks from MinIO IN PARALLEL ⭐
           └─> Reassemble and return file

Components:
├── Go Service (Port 8080)
├── MinIO (Object Storage, Port 9000)
├── TiDB (Distributed SQL, Port 4000)
├── Redis (Cache, Port 6379)
└── Jaeger (Tracing UI, Port 16686)
```

## Prerequisites

### Local Development (Docker Compose)
- Docker & Docker Compose
- Go 1.22+ (for local builds)
- 4GB+ RAM available

### Kubernetes Deployment
- kubectl configured
- Kubernetes cluster (minikube, kind, or cloud provider)
- 8GB+ RAM available

## Quick Start

### 1. Start with Docker Compose

```bash
# Clone the repository
git clone https://github.com/maneesh/labdropbox.git
cd labdropbox

# Start all services
make docker-up

# Initialize database schema
make migrate
```

Wait for all services to start (about 30 seconds). You should see:
- MinIO Console: http://localhost:9001 (minioadmin/minioadmin)
- Jaeger UI: http://localhost:16686
- API Server: http://localhost:8080

### 2. Upload a File

```bash
# Create a test file (10MB for 10 chunks)
dd if=/dev/urandom of=test.bin bs=1M count=10

# Upload the file
curl -X PUT \
  --data-binary @test.bin \
  "http://localhost:8080/write?name=test.bin"
```

Response:
```json
{
  "file_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "file_name": "test.bin",
  "file_size": 10485760,
  "chunk_count": 10,
  "message": "File uploaded successfully"
}
```

### 3. Download the File

```bash
# Use the file_id from the upload response
curl "http://localhost:8080/read/a1b2c3d4-e5f6-7890-abcd-ef1234567890" \
  -o downloaded.bin

# Verify integrity
sha256sum test.bin downloaded.bin
```

### 4. View Distributed Traces

1. Open Jaeger UI: http://localhost:16686
2. Select **Service**: `labdropbox-service`
3. Click **Find Traces**
4. Click on a `read_file` trace
5. **Observe the parallel spans**:
   ```
   read_file (root)
   ├─ cache_lookup
   ├─ fetch_chunk_metadata
   ├─ fetch_chunks_parallel
   │  ├─ download_chunk_0  ┐
   │  ├─ download_chunk_1  │ These run
   │  ├─ download_chunk_2  │ in parallel!
   │  ├─ download_chunk_3  │ (See the
   │  ├─ download_chunk_4  │  waterfall)
   │  ├─ download_chunk_5  │
   │  ├─ download_chunk_6  │
   │  ├─ download_chunk_7  │
   │  ├─ download_chunk_8  │
   │  └─ download_chunk_9  ┘
   └─ reassemble_chunks
   ```

### 5. Test Cache Hit

```bash
# Read the same file again
curl "http://localhost:8080/read/a1b2c3d4-e5f6-7890-abcd-ef1234567890" \
  -o downloaded2.bin
```

In Jaeger, compare the two traces:
- First read: `cache_lookup` (miss) → `db_lookup`
- Second read: `cache_lookup` (hit) → no `db_lookup` (faster!)

## Development

### Build Locally

```bash
# Build the binary
make build

# Run tests
make test

# Run locally (requires dependencies)
export MINIO_ENDPOINT=localhost:9000
export TIDB_HOST=localhost
export REDIS_HOST=localhost
export JAEGER_ENDPOINT=localhost:4318
make run
```

### Project Structure

```
labdropbox/
├── cmd/server/            # Application entry point
├── internal/
│   ├── config/           # Configuration management
│   ├── models/           # Data models (File, Chunk)
│   ├── storage/          # Storage clients (MinIO, TiDB, Redis)
│   ├── chunker/          # File chunking logic
│   ├── handlers/         # HTTP handlers (write, read)
│   └── tracing/          # OpenTelemetry setup
├── migrations/           # Database schema
├── deployments/
│   ├── docker-compose.yml
│   └── k8s/             # Kubernetes manifests
├── Dockerfile
├── Makefile
└── README.md
```

## Kubernetes Deployment

### Deploy to Kubernetes

```bash
# Build the Docker image
make docker-build

# Deploy all services
make k8s-deploy

# Check status
make k8s-status
```

### Initialize Database

```bash
# Get TiDB pod name
TIDB_POD=$(kubectl get pods -l app=tidb -o jsonpath='{.items[0].metadata.name}')

# Run migrations
kubectl exec -i $TIDB_POD -- mysql -h 127.0.0.1 -P 4000 -u root < migrations/001_init_schema.sql
```

### Access Services

```bash
# Get service URLs
kubectl get services

# Port-forward if using LoadBalancer
kubectl port-forward svc/labdropbox-app 8080:8080
kubectl port-forward svc/jaeger 16686:16686
```

### Clean Up

```bash
make k8s-delete
```

## Observability & Experiments

### Understanding the Traces

Each operation is instrumented with OpenTelemetry spans:

**Write Operation (`PUT /write`)**:
- `write_file`: Root span
  - `chunk_stream`: File chunking
  - `upload_chunks`: MinIO uploads (sequential)
  - `save_metadata`: TiDB writes
  - `invalidate_cache`: Redis invalidation

**Read Operation (`GET /read/{id}`)**:
- `read_file`: Root span
  - `cache_lookup`: Redis check
  - `db_lookup`: TiDB fallback (if cache miss)
  - `fetch_chunk_metadata`: Get chunk info
  - `fetch_chunks_parallel`: **⭐ Parallel chunk downloads**
    - `download_chunk_0`, `download_chunk_1`, ... (concurrent)
  - `reassemble_chunks`: Combine chunks

### Latency Injection Experiment

Simulate slow MinIO to see impact on read performance:

```bash
# Add latency to MinIO container
docker exec labdropbox-minio tc qdisc add dev eth0 root netem delay 100ms

# Download a file and observe in Jaeger
# You'll see all parallel chunk downloads delayed uniformly

# Remove latency
docker exec labdropbox-minio tc qdisc del dev eth0 root
```

### Bottleneck Analysis

Use Jaeger to identify bottlenecks:

1. Upload a large file (100MB = 100 chunks)
2. Download it and view the trace
3. Compare duration of:
   - Metadata lookup (cache/DB)
   - Parallel chunk downloads
   - Chunk reassembly

**Expected findings**:
- Metadata lookup: ~1-10ms (Redis) or ~10-50ms (TiDB)
- Chunk downloads: Dominated by network I/O (parallel)
- Reassembly: CPU-bound, negligible for small files

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVICE_PORT` | `8080` | HTTP server port |
| `CHUNK_SIZE_MB` | `1` | Chunk size in MB |
| `MINIO_ENDPOINT` | `localhost:9000` | MinIO address |
| `MINIO_ACCESS_KEY` | `minioadmin` | MinIO credentials |
| `TIDB_HOST` | `localhost` | TiDB host |
| `TIDB_PORT` | `4000` | TiDB port |
| `REDIS_HOST` | `localhost` | Redis host |
| `JAEGER_ENDPOINT` | `http://localhost:4318` | OTLP endpoint |

## API Reference

### Upload File

```http
PUT /write?name={filename}
Content-Type: application/octet-stream

<binary file data>
```

**Response**:
```json
{
  "file_id": "uuid",
  "file_name": "example.pdf",
  "file_size": 1048576,
  "chunk_count": 1,
  "message": "File uploaded successfully"
}
```

### Download File

```http
GET /read/{file_id}
```

**Response**:
- Content-Type: `application/octet-stream`
- Content-Disposition: `attachment; filename="example.pdf"`
- Body: Binary file data

### Health Check

```http
GET /health
```

**Response**: `OK`

## Troubleshooting

### Services not starting

```bash
# Check Docker Compose logs
make docker-logs

# Restart services
make docker-down
make docker-up
```

### Database connection errors

```bash
# Verify TiDB is running
docker exec labdropbox-tidb mysql -h 127.0.0.1 -P 4000 -u root -e "SELECT 1"

# Re-run migrations
make migrate
```

### MinIO bucket not found

```bash
# Access MinIO console: http://localhost:9001
# Login: minioadmin/minioadmin
# Create bucket "labdropbox" manually
```

### Traces not appearing in Jaeger

1. Verify Jaeger is running: http://localhost:16686
2. Check service logs for tracing errors
3. Ensure `JAEGER_ENDPOINT` is correctly set

## Success Metrics

**Primary Goal Achieved** ✅ when you can:

1. Upload a 10MB file
2. Download the file
3. Open Jaeger UI
4. See 10 parallel `download_chunk_*` spans in the waterfall view
5. Observe that they execute concurrently (overlapping time ranges)

## License

MIT License - Educational purposes

## Contributing

This is a learning project demonstrating distributed systems concepts. Feel free to fork and experiment!

## Acknowledgments

Built to demonstrate:
- System Design patterns (chunking, distributed storage)
- OpenTelemetry distributed tracing
- Go concurrency (goroutines + context propagation)
- Cloud-native infrastructure (Docker, Kubernetes)
