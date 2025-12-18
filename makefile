docker-up:
	@cd docker && docker-compose up -d

docker-down:
	@cd docker && docker-compose down

goose-up:
	@cd sql/schema && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat?sslmode=disable up

goose-down:
	@cd sql/schema && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat?sslmode=disable down

.PHONY: docker-up docker-down goose-up goose-down