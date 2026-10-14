.PHONY: help up down logs ps verify token-provider-a token-provider-b token-internal test test-race vet fmt tidy

# Default target
.DEFAULT_GOAL := help

help: ## Show available Makefile targets
	@echo "======================================================================"
	@echo "                   Jungle Gaming - Makefile Commands                 "
	@echo "======================================================================"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

up: ## Start PostgreSQL, LocalStack (SQS), and Keycloak in background
	docker compose up -d

down: ## Stop all services and remove volumes
	docker compose down -v

logs: ## Follow logs from all docker compose services
	docker compose logs -f

ps: ## Display status of all docker compose containers
	docker compose ps

verify: ## Verify infrastructure services (Postgres, SQS FIFO, Keycloak)
	@bash scripts/verify-infra.sh

deploy-stack: ## Re-deploy CloudFormation template into LocalStack
	docker exec jungle-localstack awslocal cloudformation deploy --stack-name jungle-messaging-stack --template-file /etc/localstack/init/ready.d/sqs-template.yaml

token-provider-a: ## Fetch JWT access token for provider-a from Keycloak
	@bash scripts/get-token.sh provider-a

token-provider-b: ## Fetch JWT access token for provider-b from Keycloak
	@bash scripts/get-token.sh provider-b

token-internal: ## Fetch JWT access token for internal-service from Keycloak
	@bash scripts/get-token.sh internal-service

test: ## Run standard Go tests
	go test ./...

test-race: ## Run Go tests with race detector enabled
	go test -race ./...

vet: ## Run go vet static analyzer
	go vet ./...

fmt: ## Format Go source code
	gofmt -s -w .

migrate-up: ## Run all pending database migrations
	go run cmd/migrate/main.go up

migrate-down: ## Rollback the last applied database migration
	go run cmd/migrate/main.go down

migrate-version: ## Check current database migration version
	go run cmd/migrate/main.go version

tidy: ## Download and tidy Go module dependencies
	go mod tidy


