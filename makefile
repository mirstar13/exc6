run: docker-up build
	@./securechat.exe

build:
	@go build -o securechat.exe main.go

docker-up:
	@cd docker && docker-compose up -d --remove-orphans

docker-down:
	@cd docker && docker-compose down -d --remove-orphans

goose-up:
	@cd sql/schema && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat?sslmode=disable up && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat_test?sslmode=disable up

goose-down:
	@cd sql/schema && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat?sslmode=disable down && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat_test?sslmode=disable down


# Run full load tests (takes time)
test-load:
	@cd docker && docker-compose -f docker-compose.test.yml up -d --remove-orphans
	@echo "Running load tests..."
	go test -v -timeout 30m -run "^Test.*Load|^Test.*Storm|^Test.*Performance" ./tests/load

# Run short load tests
test-load-short:
	@cd docker && docker-compose -f docker-compose.test.yml up -d --remove-orphans
	@echo "Running short load tests..."
	go test -v -timeout 5m -short -run "^Test.*Load" ./tests/load

# Run chaos engineering tests
test-chaos:
	@cd docker && docker-compose -f docker-compose.test.yml up -d --remove-orphans
	@echo "Running chaos tests..."
	@echo "Warning: This will pause/unpause Docker containers"
	go test -v -timeout 15m -run "^Test.*Failover|^Test.*Chaos" ./tests/load

# Run benchmarks
bench-load:
	@cd docker && docker-compose -f docker-compose.test.yml up -d --remove-orphans
	@echo "Running load benchmarks..."
	go test -timeout 30m -bench=. -benchmem -benchtime=10s ./tests/load

.PHONY: docker-up docker-down goose-up goose-down build run test-load test-load-short test-chaos bench-load
