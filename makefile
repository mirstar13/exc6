run: docker-up build
	@./securechat.exe

build:
	@go build -o securechat.exe main.go

docker-up:
	@cd docker && docker-compose up -d

docker-down:
	@cd docker && docker-compose down

goose-up:
	@cd sql/schema && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat?sslmode=disable up && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat_test?sslmode=disable up

goose-down:
	@cd sql/schema && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat?sslmode=disable down && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat_test?sslmode=disable down

test-setup:
	@bash tests/test-setup.sh

test-teardown:
	@bash tests/test-teardown.sh

test-all: docker-down test-setup
	@$(MAKE) test-integration || true
	@$(MAKE) test-security || true
	@$(MAKE) test-e2e || true
	@$(MAKE) test-teardown

security-scan:
	@echo "Running security scan..."
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@gosec -fmt=json -out=security-report.json ./...
	@echo "Security report: security-report.json"

unit-test:
	@echo "Running unit tests..."
	@go test -v -race -coverprofile=coverage.txt -covermode=atomic ./tests/services/...

test-integration:
	@echo "Running integration tests..."
	@go test -v ./tests/integration/...

test-security:
	@echo "Running security tests..."
	@go test -v ./tests/security/...

test-services:
	@echo "Running service tests..."
	@go test -v ./services/...

test-performance:
	@echo "Running performance tests..."
	@go test -v -bench=. ./tests/performance/...

test-e2e:
	@echo "Running end-to-end tests..."
	@go test -v ./tests/e2e/...

test-watch:
	@echo "Watching for file changes to run tests..."
	@go install github.com/cosmtrek/air@latest
	@air -c tests/air.toml

test-coverage-check:
	@go test -cover ./... | grep -o '[0-9.]*%' | \
		awk '{if ($$1+0 < 75) {print "Coverage below 75%: " $$1; exit 1}}'

.PHONY: docker-up docker-down goose-up goose-down build run