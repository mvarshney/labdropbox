1. Project Goal
-------------------
Build a functional, chunk-based file storage service to bridge the conceptual gap between System Design theory and Distributed Observability. The primary success metric is the successful visualization of a "Parallel Chunk Read" via distributed tracing.

2. Architecture & Stack
---------------------------------
Language: Go (Golang)

Storage (Blob): MinIO (S3-compatible API)

Metadata: TiDB (MySQL-compatible distributed SQL)

Caching: Redis (Read-through cache for file metadata)

Observability: OpenTelemetry (OTEL) with Jaeger as the collector/UI.

Platform: Kubernetes (k8s)

3. Data Model
---------------------------------
Language: Go (Golang)
File: Metadata including file_id, name, and size.

Chunks: 1-to-N relationship with Files. Each chunk has a hash, order_index, and minio_object_key.

4. Scope & Simplifications
---------------------------------
Language: Go (Golang)
No Content-Addressability: Files will be chunked, but global deduplication is out of scope to focus on the request flow.

Fixed Chunk Size: 1MB (hardcoded for simplicity).

No Auth: Focus is on the data path, not security.

Basic Sync: Single-versioned files (no conflict resolution).

5. Primary Workflows (The "Traced" Paths)
---------------------------------
Language: Go (Golang)
PUT /write?name=filename

* Receive binary stream.
* Chunk the stream into 1MB buffers.
* Upload chunks to MinIO.
* Write metadata to TiDB.
* Invalidate/Update Redis.

GET /read/{file_id}

* Trace Segment 1: Hit Redis for metadata (simulate Cache Hit/Miss).
* Trace Segment 2: Fallback to TiDB if miss.
* Trace Segment 3 (The Focus): Fetch chunks from MinIO in parallel using Goroutines.
* Trace Segment 4: Reassemble and stream response.

6. Observability Requirements (The "Why")
---------------------------------
Language: Go (Golang)
Every major operation (DB call, Redis call, MinIO call) must be wrapped in an OpenTelemetry Span.

The GET request must explicitly show Parallel Spans in Jaeger when fetching chunks.

The Go service must propagate the context correctly to preserve the trace ID across goroutines.

7. Lab Experiments
---------------------------------
Language: Go (Golang)
Latency Injection: Use OpenTelemetry to measure the impact of artificial latency on MinIO.

Bottleneck Hunting: Use the Jaeger waterfall to identify if the "Metadata Lookup" or "Data Transfer" is the primary bottleneck.
