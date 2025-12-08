up:
	@cd sql/schema && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat?sslmode=disable up

down:
	@cd sql/schema && \
	goose postgres postgres://postgres:postgres@localhost:5432/securechat?sslmode=disable down