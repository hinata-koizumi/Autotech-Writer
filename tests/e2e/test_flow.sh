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
MAX_ATTEMPTS=10
ATTEMPT=1
until docker exec autotech-writer-db-1 pg_isready -U autotech -d autotech_test > /dev/null 2>&1 || [ $ATTEMPT -eq $MAX_ATTEMPTS ]; do
    echo "Waiting for DB... ($ATTEMPT/$MAX_ATTEMPTS)"
    sleep 2
    ATTEMPT=$((ATTEMPT + 1))
done

if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo_error "Database failed to become ready in time."
    exit 1
fi

# Phase 2: Go Collector Integration Tests
echo_info "Running Go Integration Tests inside Docker..."
docker run --rm -v "$(pwd):/app" -w /app/go-collector --network autotech-writer_default \
  -e TEST_DB_DSN="postgres://autotech:password@db:5432/autotech_test?sslmode=disable" \
  golang:1.25.8 go test -v ./internal/repository/...
echo_success "Go Integration Tests Passed"

# Phase 3: Python LLM Integration Tests
echo_info "Preparing Python Integration Tests image..."
docker build -t autotech-python-test-img -f python-llm/Dockerfile ./python-llm > /dev/null

echo_info "Running Python Integration Tests inside Docker..."
# Note: pytest-asyncio and asyncpg are already in requirements.txt
docker run --rm -v "$(pwd):/app" -w /app/python-llm --network autotech-writer_default \
  -e TEST_DB_DSN="postgres://autotech:password@db:5432/autotech_test" \
  autotech-python-test-img pytest -v tests/test_db_integration.py
echo_success "Python Integration Tests Passed"

# Phase 4: Smoke Test / Connectivity Check
echo_info "Running Smoke Tests..."
docker run --rm -v "$(pwd):/app" -w /app --network autotech-writer_default \
  -e TEST_DB_DSN="postgres://autotech:password@db:5432/autotech_test" \
  autotech-python-test-img python tests/e2e/smoke_test.py
echo_success "Smoke Tests Passed"

echo_success "E2E Test Workflow Completed Successfully!"
