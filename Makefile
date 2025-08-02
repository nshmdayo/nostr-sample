.PHONY: build build-docker save-image clean run test help

# Variables
APP_NAME := nostr-relay
VERSION := latest
OUTPUT_DIR := output
IMAGE_NAME := $(APP_NAME):$(VERSION)
TAR_FILE := $(OUTPUT_DIR)/$(APP_NAME)-$(VERSION).tar

# Default target
help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Create output directory
$(OUTPUT_DIR):
	@mkdir -p $(OUTPUT_DIR)

# Build Go binary locally
build: ## Build Go binary locally
	@echo "Building Go binary..."
	@go build -o $(OUTPUT_DIR)/nostr-server main.go
	@echo "Binary saved to $(OUTPUT_DIR)/nostr-server"

# Build Docker image
build-docker: ## Build Docker image
	@echo "Building Docker image: $(IMAGE_NAME)"
	@docker build -t $(IMAGE_NAME) .
	@echo "Docker image built successfully: $(IMAGE_NAME)"

# Save Docker image to tar file
save-image: build-docker $(OUTPUT_DIR) ## Build Docker image and save to output/ as tar file
	@echo "Saving Docker image to $(TAR_FILE)..."
	@docker save $(IMAGE_NAME) -o $(TAR_FILE)
	@echo "Docker image saved to $(TAR_FILE)"
	@ls -lh $(TAR_FILE)

# Extract binary from Docker image
extract-binary: build-docker $(OUTPUT_DIR) ## Extract binary from Docker image to output/
	@echo "Extracting binary from Docker image..."
	@docker create --name temp-container $(IMAGE_NAME)
	@docker cp temp-container:/root/nostr-server $(OUTPUT_DIR)/nostr-server-docker
	@docker rm temp-container
	@echo "Binary extracted to $(OUTPUT_DIR)/nostr-server-docker"

# Build all artifacts
all: save-image extract-binary ## Build Docker image, save as tar, and extract binary
	@echo "All artifacts built and saved to $(OUTPUT_DIR)/"

# Run Docker container
run: build-docker ## Run Docker container
	@echo "Running Docker container..."
	@docker run -p 8080:8080 --name nostr-relay-instance $(IMAGE_NAME)

# Run container in background
run-daemon: build-docker ## Run Docker container in background
	@echo "Running Docker container in background..."
	@docker run -d -p 8080:8080 --name nostr-relay-daemon $(IMAGE_NAME)
	@echo "Container started. Access at http://localhost:8080"

# Stop running containers
stop: ## Stop running containers
	@echo "Stopping containers..."
	@docker stop nostr-relay-instance nostr-relay-daemon 2>/dev/null || true
	@docker rm nostr-relay-instance nostr-relay-daemon 2>/dev/null || true

# Test the application
test: ## Run tests
	@echo "Running tests..."
	@go test ./...

# Test client
test-client: ## Run test client
	@echo "Running test client..."
	@go run client/main.go

# Clean up
clean: ## Clean up built artifacts and Docker images
	@echo "Cleaning up..."
	@rm -rf $(OUTPUT_DIR)
	@docker rmi $(IMAGE_NAME) 2>/dev/null || true
	@docker system prune -f

# Load Docker image from tar file
load-image: ## Load Docker image from tar file
	@if [ -f "$(TAR_FILE)" ]; then \
		echo "Loading Docker image from $(TAR_FILE)..."; \
		docker load -i $(TAR_FILE); \
	else \
		echo "Error: $(TAR_FILE) not found. Run 'make save-image' first."; \
		exit 1; \
	fi

# Show Docker images
images: ## Show Docker images
	@docker images | grep $(APP_NAME) || echo "No $(APP_NAME) images found"

# Show output directory contents
show-output: ## Show contents of output directory
	@if [ -d "$(OUTPUT_DIR)" ]; then \
		echo "Contents of $(OUTPUT_DIR):"; \
		ls -la $(OUTPUT_DIR)/; \
	else \
		echo "Output directory does not exist"; \
	fi
