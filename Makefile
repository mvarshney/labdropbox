.PHONY: help build test clean docker-build docker-up docker-down docker-logs migrate k8s-deploy k8s-delete k8s-status run

# Default target
help:
	@echo "LabDropbox - Distributed File Storage Service"
	@echo ""
	@echo "Available targets:"
	@echo "  build          - Build the Go binary"
	@echo "  run            - Run the service locally"
	@echo "  test           - Run unit tests"
	@echo "  clean          - Clean build artifacts"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-up      - Start all services with Docker Compose"
	@echo "  docker-down    - Stop all services"
	@echo "  docker-logs    - Show logs from all services"
	@echo "  migrate        - Run database migrations on TiDB"
	@echo "  k8s-deploy     - Deploy to Kubernetes"
	@echo "  k8s-delete     - Delete Kubernetes resources"
	@echo "  k8s-status     - Show Kubernetes status"

# Build the Go binary
build:
	@echo "Building LabDropbox..."
	@go build -o bin/labdropbox ./cmd/server
	@echo "Build complete: bin/labdropbox"

# Run the service locally (requires dependencies to be running)
run: build
	@echo "Starting LabDropbox service..."
	@./bin/labdropbox

# Run tests
test:
	@echo "Running tests..."
	@go test -v -race ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@go clean

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build -t labdropbox:latest .
	@echo "Docker image built: labdropbox:latest"

# Start all services with Docker Compose
docker-up:
	@echo "Starting services with Docker Compose..."
	@cd deployments && docker-compose up -d
	@echo "Services started. Waiting for initialization..."
	@sleep 10
	@echo ""
	@echo "Services are running:"
	@echo "  MinIO Console: http://localhost:9001 (minioadmin/minioadmin)"
	@echo "  Jaeger UI:     http://localhost:16686"
	@echo "  API Server:    http://localhost:8080"
	@echo ""
	@echo "Run 'make migrate' to initialize the database schema"

# Stop all services
docker-down:
	@echo "Stopping services..."
	@cd deployments && docker-compose down
	@echo "Services stopped"

# Show logs from all services
docker-logs:
	@cd deployments && docker-compose logs -f

# Run database migrations
migrate:
	@echo "Running database migrations..."
	@docker exec -i labdropbox-tidb mysql -h 127.0.0.1 -P 4000 -u root < migrations/001_init_schema.sql
	@echo "Migrations complete"

# Deploy to Kubernetes
k8s-deploy:
	@echo "Deploying to Kubernetes..."
	@kubectl apply -f deployments/k8s/minio.yaml
	@kubectl apply -f deployments/k8s/tidb.yaml
	@kubectl apply -f deployments/k8s/redis.yaml
	@kubectl apply -f deployments/k8s/jaeger.yaml
	@kubectl apply -f deployments/k8s/app-configmap.yaml
	@kubectl apply -f deployments/k8s/app-deployment.yaml
	@echo "Deployment complete. Check status with 'make k8s-status'"

# Delete Kubernetes resources
k8s-delete:
	@echo "Deleting Kubernetes resources..."
	@kubectl delete -f deployments/k8s/app-deployment.yaml --ignore-not-found
	@kubectl delete -f deployments/k8s/app-configmap.yaml --ignore-not-found
	@kubectl delete -f deployments/k8s/jaeger.yaml --ignore-not-found
	@kubectl delete -f deployments/k8s/redis.yaml --ignore-not-found
	@kubectl delete -f deployments/k8s/tidb.yaml --ignore-not-found
	@kubectl delete -f deployments/k8s/minio.yaml --ignore-not-found
	@echo "Resources deleted"

# Show Kubernetes status
k8s-status:
	@echo "Kubernetes Status:"
	@echo ""
	@echo "Pods:"
	@kubectl get pods
	@echo ""
	@echo "Services:"
	@kubectl get services
	@echo ""
	@echo "PVCs:"
	@kubectl get pvc
