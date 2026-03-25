#!/bin/bash
set -eo pipefail

# ANSI color codes for logs
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

echo_info() { echo -e "${BLUE}[INFO] $1${NC}"; }
echo_success() { echo -e "${GREEN}[SUCCESS] $1${NC}"; }
echo_error() { echo -e "${RED}[ERROR] $1${NC}"; }

# Go to project root directory
cd "$(dirname "$0")/../.."

# Teardown function to ensure database container is removed on exit or error
teardown() {
    echo_info "Tearing down testing environment..."
    docker compose -f docker-compose.test.yml down -v
    echo_success "Teardown complete."
}

# Trap EXIT to always run teardown
trap teardown EXIT

echo_info "Starting E2E Test Workflow..."

# Phase 1: Infrastructure Setup
echo_info "Starting test database container..."
docker compose -f docker-compose.test.yml up -d db

echo_info "Waiting for PostgreSQL to become ready..."
# Wait for Docker healthcheck to pass
MAX_ATTEMPTS=20
ATTEMPT=1
until [ "$(docker inspect -f '{{.State.Health.Status}}' autotech-writer-db-1 2>/dev/null)" == "healthy" ] || [ $ATTEMPT -eq $MAX_ATTEMPTS ]; do
    echo "Waiting for DB healthcheck... ($ATTEMPT/$MAX_ATTEMPTS)"
    sleep 2
    ATTEMPT=$((ATTEMPT + 1))
done

if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo_error "Database failed to become healthy in time."
    exit 1
fi

# Phase 2: Go Collector Integration Tests
echo_info "Running Go Tests inside Docker..."
# Use host network to connect to DB at localhost:5432
docker run --rm -v "$(pwd):/app" -w /app/go-collector --network host \
  golang:1.23-alpine go test -v ./...
echo_success "Go Tests Passed"

# Phase 3: Python LLM Integration Tests
echo_info "Preparing Python Integration Tests image..."
docker build -t autotech-python-test-img -f python-llm/Dockerfile ./python-llm > /dev/null

echo_info "Running Python Tests inside Docker..."
# Run full test suite including new features (retry, line webhook, prompt switching, pg listener)
docker run --rm -v "$(pwd):/app" -w /app/python-llm --network host \
  -e TEST_DB_DSN="postgres://postgres:postgres@localhost:5432/autotech" \
  autotech-python-test-img pytest -v tests/
echo_success "Python Tests Passed"

# Phase 4: Smoke Test / Connectivity Check
echo_info "Running Smoke Tests..."
docker run --rm -v "$(pwd):/app" -w /app --network host \
  -e TEST_DB_DSN="postgres://postgres:postgres@localhost:5432/autotech" \
  autotech-python-test-img python tests/e2e/smoke_test.py
echo_success "Smoke Tests Passed"

echo_success "E2E Test Workflow Completed Successfully!"
