#!/bin/bash

# SecureChat Monitoring Stack Setup Script
# This script automates the setup of Prometheus, Grafana, and Alertmanager

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

# Check if Docker is installed
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        exit 1
    fi
    print_success "Docker is installed"
}

# Check if Docker Compose is installed
check_docker_compose() {
    if ! command -v docker-compose &> /dev/null; then
        print_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi
    print_success "Docker Compose is installed"
}

# Create necessary directories
create_directories() {
    print_info "Creating directory structure..."
    
    mkdir -p monitoring/prometheus
    mkdir -p monitoring/grafana/dashboards
    mkdir -p monitoring/grafana/provisioning/datasources
    mkdir -p monitoring/grafana/provisioning/dashboards
    mkdir -p monitoring/alertmanager
    
    print_success "Directories created"
}

# Check if monitoring stack is already running
check_existing_stack() {
    if docker ps | grep -q "prometheus\|grafana\|alertmanager"; then
        print_info "Monitoring stack is already running"
        read -p "Do you want to restart it? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            docker-compose -f docker/docker-compose.monitoring.yml down
            print_success "Stopped existing stack"
        else
            exit 0
        fi
    fi
}

# Start monitoring stack
start_stack() {
    print_info "Starting monitoring stack..."
    
    cd monitoring
    docker-compose -f docker-compose.monitoring.yml up -d
    cd ..
    
    print_success "Monitoring stack started"
}

# Wait for services to be ready
wait_for_services() {
    print_info "Waiting for services to be ready..."
    
    # Wait for Prometheus
    until curl -s http://localhost:9090/-/ready > /dev/null; do
        echo -n "."
        sleep 2
    done
    print_success "Prometheus is ready"
    
    # Wait for Grafana
    until curl -s http://localhost:3000/api/health > /dev/null; do
        echo -n "."
        sleep 2
    done
    print_success "Grafana is ready"
    
    # Wait for Alertmanager
    until curl -s http://localhost:9093/-/ready > /dev/null; do
        echo -n "."
        sleep 2
    done
    print_success "Alertmanager is ready"
}

# Display access information
display_info() {
    echo ""
    echo "=========================================="
    echo "  Monitoring Stack Successfully Started"
    echo "=========================================="
    echo ""
    echo "Access your monitoring tools:"
    echo ""
    echo "📊 Grafana:        http://localhost:3000"
    echo "   Username: admin"
    echo "   Password: admin"
    echo ""
    echo "📈 Prometheus:     http://localhost:9090"
    echo ""
    echo "🔔 Alertmanager:   http://localhost:9093"
    echo ""
    echo "🖥️  Node Exporter:  http://localhost:9100"
    echo ""
    echo "=========================================="
    echo ""
    print_info "Next steps:"
    echo "  1. Open Grafana and change the default password"
    echo "  2. Configure Alertmanager for notifications"
    echo "  3. Customize dashboards as needed"
    echo ""
    echo "For troubleshooting, run:"
    echo "  docker-compose -f docker/docker-compose.monitoring.yml logs"
    echo ""
}

# Check application metrics endpoint
check_app_metrics() {
    print_info "Checking if application metrics are exposed..."
    
    if curl -s http://localhost:8000/metrics > /dev/null; then
        print_success "Application metrics endpoint is accessible"
    else
        print_error "Application metrics endpoint is not accessible"
        print_info "Make sure your application is running and exposing metrics at /metrics"
    fi
}

# Main execution
main() {
    echo ""
    echo "=========================================="
    echo "  SecureChat Monitoring Stack Setup"
    echo "=========================================="
    echo ""
    
    check_docker
    check_docker_compose
    create_directories
    check_existing_stack
    start_stack
    wait_for_services
    check_app_metrics
    display_info
    
    print_success "Setup complete!"
}

# Run main function
main