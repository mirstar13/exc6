#!/bin/bash

echo "Cleaning up test environment..."

# Stop test services
docker-compose -f docker/docker-compose.test.yml down -v

# Clean test cache
go clean -testcache

echo "Cleanup complete"