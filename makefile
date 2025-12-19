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

# Start monitoring stack
monitoring-up:
	@echo "Starting monitoring stack..."
	@cd monitoring && docker-compose -f docker-compose.monitoring.yml up -d
	@echo "Waiting for services to be ready..."
	@sleep 10
	@echo "✓ Monitoring stack is running"
	@echo ""
	@echo "Access your monitoring tools:"
	@echo "  Grafana:      http://localhost:3000 (admin/admin)"
	@echo "  Prometheus:   http://localhost:9090"
	@echo "  Alertmanager: http://localhost:9093"
	@echo ""

# Stop monitoring stack
monitoring-down:
	@echo "Stopping monitoring stack..."
	@cd monitoring && docker-compose -f docker-compose.monitoring.yml down
	@echo "✓ Monitoring stack stopped"

# Restart monitoring stack
monitoring-restart:
	@$(MAKE) monitoring-down
	@$(MAKE) monitoring-up

# View monitoring logs
monitoring-logs:
	@cd monitoring && docker-compose -f docker-compose.monitoring.yml logs -f

# Setup monitoring (run once)
monitoring-setup:
	@bash monitoring/setup.sh

# Open Grafana in browser (macOS)
monitoring-grafana:
	@open http://localhost:3000

# Open Prometheus in browser (macOS)
monitoring-prometheus:
	@open http://localhost:9090

# Check monitoring health
monitoring-health:
	@echo "Checking monitoring stack health..."
	@echo ""
	@echo -n "Prometheus: "
	@curl -s http://localhost:9090/-/ready && echo "✓ Ready" || echo "✗ Not ready"
	@echo -n "Grafana: "
	@curl -s http://localhost:3000/api/health | grep -q "ok" && echo "✓ Ready" || echo "✗ Not ready"
	@echo -n "Alertmanager: "
	@curl -s http://localhost:9093/-/ready && echo "✓ Ready" || echo "✗ Not ready"
	@echo ""

# Show metrics from application
monitoring-metrics:
	@echo "Fetching application metrics..."
	@curl -s http://localhost:8000/metrics | grep -E "^(http_|chat_|sse_|sessions_)" | head -20
	@echo ""
	@echo "Full metrics available at: http://localhost:8000/metrics"

# Query Prometheus
monitoring-query:
	@echo "Example Prometheus queries:"
	@echo ""
	@echo "Request rate (last 5min):"
	@curl -s 'http://localhost:9090/api/v1/query?query=sum(rate(http_requests_total[5m]))' | jq -r '.data.result[0].value[1]' && echo " requests/sec"
	@echo ""
	@echo "Error rate:"
	@curl -s 'http://localhost:9090/api/v1/query?query=sum(rate(http_requests_total{status=~"5.."}[5m]))/sum(rate(http_requests_total[5m]))' | jq -r '.data.result[0].value[1]' && echo ""
	@echo ""
	@echo "Active sessions:"
	@curl -s 'http://localhost:9090/api/v1/query?query=sessions_active' | jq -r '.data.result[0].value[1]'
	@echo ""

# Backup Grafana dashboards
monitoring-backup:
	@echo "Backing up Grafana data..."
	@docker run --rm -v grafana-data:/data -v $(PWD):/backup alpine tar czf /backup/grafana-backup-$$(date +%Y%m%d).tar.gz /data
	@echo "✓ Backup saved to grafana-backup-$$(date +%Y%m%d).tar.gz"

# Restore Grafana dashboards
monitoring-restore:
	@echo "Available backups:"
	@ls -lh grafana-backup-*.tar.gz 2>/dev/null || echo "No backups found"
	@echo ""
	@read -p "Enter backup filename to restore: " backup; \
	docker run --rm -v grafana-data:/data -v $(PWD):/backup alpine tar xzf /backup/$$backup -C /
	@echo "✓ Backup restored"

# View active alerts
monitoring-alerts:
	@echo "Active alerts:"
	@curl -s http://localhost:9093/api/v1/alerts | jq -r '.data[] | "\(.labels.alertname): \(.annotations.summary)"'

# Test alerting
monitoring-test-alert:
	@echo "Testing alerting by triggering a test alert..."
	@curl -X POST http://localhost:9093/api/v1/alerts \
		-H "Content-Type: application/json" \
		-d '[{"labels":{"alertname":"TestAlert","severity":"warning"},"annotations":{"summary":"This is a test alert"}}]'
	@echo "✓ Test alert sent"

# Clean monitoring data
monitoring-clean:
	@echo "⚠️  This will delete all monitoring data including metrics and dashboards!"
	@read -p "Are you sure? (yes/no): " confirm; \
	if [ "$$confirm" = "yes" ]; then \
		docker-compose -f monitoring/docker-compose.monitoring.yml down -v; \
		echo "✓ Monitoring data cleaned"; \
	else \
		echo "Cancelled"; \
	fi

# Help for monitoring commands
monitoring-help:
	@echo "Monitoring Commands:"
	@echo ""
	@echo "  make monitoring-up         - Start monitoring stack"
	@echo "  make monitoring-down       - Stop monitoring stack"
	@echo "  make monitoring-restart    - Restart monitoring stack"
	@echo "  make monitoring-logs       - View monitoring logs"
	@echo "  make monitoring-setup      - Initial setup (run once)"
	@echo "  make monitoring-health     - Check monitoring stack health"
	@echo "  make monitoring-metrics    - Show application metrics"
	@echo "  make monitoring-query      - Run example Prometheus queries"
	@echo "  make monitoring-alerts     - View active alerts"
	@echo "  make monitoring-backup     - Backup Grafana dashboards"
	@echo "  make monitoring-restore    - Restore Grafana dashboards"
	@echo "  make monitoring-clean      - Delete all monitoring data"
	@echo ""