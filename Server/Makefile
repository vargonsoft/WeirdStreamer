.PHONY: build clean run install test docker help

# Build variables
SERVER_BIN=bin/server
CLI_BIN=bin/cli
GO=go
GOFLAGS=-v
LDFLAGS=-s -w

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build server and CLI binaries
	mkdir -p bin
	@echo "Building server..."
	CGO_ENABLED=1 $(GO) -C src build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o ../$(SERVER_BIN) ./cmd/server
	@echo "Building CLI..."
	CGO_ENABLED=1 $(GO) -C src build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o ../$(CLI_BIN) ./cmd/cli
	@echo "Build complete!"

install: ## Install dependencies
	@echo "Installing dependencies..."
	$(GO) -C src mod download
	$(GO) -C src mod tidy

run: build ## Build and run the server
	@echo "Starting server..."
	./$(SERVER_BIN)

dev: ## Run server with auto-reload (requires air: go install github.com/cosmtrek/air@latest)
	air

test: ## Run tests
	$(GO) -C src test -v ./...

test-coverage: ## Run tests with coverage
	$(GO) -C src test -v -coverprofile=../coverage.out ./...
	$(GO) tool cover -html=coverage.out

clean: ## Remove build artifacts
	@echo "Cleaning..."
	rm -f $(SERVER_BIN) $(CLI_BIN)
	rm -f coverage.out
	rm -rf var/data/*.db
	rm -rf thumbnails/*
	@echo "Clean complete!"

# Your own container settings (docker/docker-compose.extra.yml, from the example) are loaded when present
COMPOSE = docker compose -f docker/docker-compose.yml -f docker/docker-compose.override.yml $(if $(wildcard docker/docker-compose.extra.yml),-f docker/docker-compose.extra.yml)

docker-config: ## Write the Docker library mounts and container config from config.json
	python3 docker/generate.py $(if $(DOCKER_USER),--user $(DOCKER_USER))

docker-build: ## Build the Docker image
	docker build -f docker/Dockerfile -t weirdstreamer:latest .

docker-compose-up: docker-config ## Generate the config, then build and start the container
	$(COMPOSE) up -d --build

docker-compose-down: ## Stop the container
	$(COMPOSE) down

docker-compose-logs: ## Follow the container's logs
	$(COMPOSE) logs -f

setup: build ## Initial setup: build, create config, and create admin user
	@if [ ! -f config.json ]; then \
		echo "Creating config.json from example..."; \
		cp config.example.json config.json; \
		echo "Please edit config.json and set your JWT secret and media paths!"; \
	fi
	@echo "Run 'make create-admin' to create an admin user"

create-admin: ## Create admin user (interactive)
	./$(CLI_BIN) user create admin --admin

scan: ## Scan media directories
	./$(CLI_BIN) scan

add-path: ## Add media path (usage: make add-path PATH=/path/to/media)
	./$(CLI_BIN) config add-path $(PATH)

list-users: ## List all users
	./$(CLI_BIN) user list

list-paths: ## List media paths
	./$(CLI_BIN) config list-paths

fmt: ## Format Go code
	$(GO) -C src fmt ./...

vet: ## Run go vet
	$(GO) -C src vet ./...

lint: ## Run golangci-lint (requires: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run

release: clean install test build ## Prepare release build
	@echo "Release build complete!"

.DEFAULT_GOAL := help
