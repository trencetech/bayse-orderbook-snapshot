.PHONY: help init format setup run serve build clean test deps tidy migrate migrate-rollback migrate-status migrate-create migrate-init

# Load environment variables from .env file
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

init: ## Install required tools (formatting, imports)
	@echo "Installing required tools..."
	@go install mvdan.cc/gofumpt@v0.9.2
	@echo "gofumpt installed"
	@go install golang.org/x/tools/cmd/goimports@v0.38.0
	@echo "goimports installed"
	@go install github.com/daixiang0/gci@v0.13.7
	@echo "gci installed"
	@echo ""
	@echo "Tools installed successfully!"

GOPATH_FIRST := $(shell go env GOPATH | awk -F':' '{print $$1}')
GOBIN := $(GOPATH_FIRST)/bin
FILES = $(shell find . -type f -name '*.go')
format: ## Format Go source files
	@$(GOBIN)/gofumpt -l -w $(FILES)
	@$(GOBIN)/goimports -local github.com/trencetech/bayse-orderbook-snapshot -l -w $(FILES)
	$(GOBIN)/gci write --section Standard --section Default --section "Prefix(github.com/trencetech/bayse-orderbook-snapshot)" $(FILES)

setup: deps ## Setup the project (download deps)
	@echo "Project setup complete!"

run: ## Run the application (development mode)
	@echo "Starting application in development mode..."
	@go run cmd/server/main.go

serve: build ## Build and run the application
	@echo "Starting server..."
	@./bin/application

build: ## Build the application binary
	@echo "Building application..."
	@mkdir -p bin
	$(eval GIT_SHA := $(shell git rev-parse --short HEAD))
	@CGO_ENABLED=0 go build -ldflags="-X 'github.com/trencetech/bayse-orderbook-snapshot/internal/version.CommitSHA=$(GIT_SHA)'" -o bin/application cmd/server/main.go
	@CGO_ENABLED=0 go build -o bin/migrate cmd/migrate/main.go
	@echo "Build complete: bin/application, bin/migrate"

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@echo "Clean complete"

test: ## Run tests
	@echo "Running tests..."
	@go test -v ./...

deps: ## Download Go dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@echo "Dependencies downloaded"

tidy: ## Tidy Go dependencies
	@echo "Tidying dependencies..."
	@go mod tidy
	@echo "Dependencies tidied"

migrate: ## Run pending database migrations
	@go run cmd/migrate/main.go up

migrate-rollback: ## Rollback the last migration
	@go run cmd/migrate/main.go down

migrate-status: ## Show migration status
	@go run cmd/migrate/main.go status

migrate-create: ## Create a new migration (usage: make migrate-create name=migration_name)
	@go run cmd/migrate/main.go create $(name)

migrate-init: ## Initialize the migration table
	@go run cmd/migrate/main.go init
