run: docker-up build
	@./securechat.exe

build:
	@go build -o securechat.exe main.go

docker-up:
	@cd docker && docker-compose up -d --remove-orphans

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

.PHONY: docker-up docker-down goose-up goose-down build run