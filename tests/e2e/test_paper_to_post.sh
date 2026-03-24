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

# Teardown function
teardown() {
    echo_info "Tearing down E2E test environment..."
    docker stop python-llm-e2e-test || true
    docker rm python-llm-e2e-test || true
    docker compose -f docker-compose.test.yml down -v
    echo_success "Teardown complete."
}

trap teardown EXIT

echo_info "Starting E2E Paper-to-Post Test Workflow..."

# 1. Setup Database
echo_info "Starting test database container..."
docker compose -f docker-compose.test.yml up -d db

echo_info "Waiting for PostgreSQL to become ready..."
MAX_ATTEMPTS=15
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

# 2. Build Images
echo_info "Building component images..."
docker build -t autotech-python-e2e-img -f python-llm/Dockerfile ./python-llm
docker build -t autotech-go-collector-e2e-img -f go-collector/Dockerfile ./go-collector

# 3. Start Python LLM Service as a Daemon
echo_info "Starting Python LLM Service..."
docker run -d --name python-llm-e2e-test \
  --network autotech-writer_default \
  -e DATABASE_URL="postgres://autotech:password@db:5432/autotech_test" \
  -e DRY_RUN="true" \
  -e STAY_AS_DRAFT="true" \
  -e MIN_ARTICLE_LENGTH="100" \
  -p 8000:8000 \
  autotech-python-e2e-img

echo_info "Waiting for Python service to be healthy..."
for i in {1..10}; do
    if curl -s http://localhost:8000/health | grep -q "ok"; then
        echo_success "Python service is up!"
        break
    fi
    echo "Waiting for Python service... ($i/10)"
    sleep 2
    if [ $i -eq 10 ]; then
        echo_error "Python service failed to start."
        docker logs python-llm-e2e-test
        exit 1
    fi
done

# 4. Run Go Collector in ONESHOT mode
echo_info "Running Go Collector in ONESHOT mode..."
# We use ArXiv categories that likely have recent papers to ensure one-shot finds something
# Or we can point to a specific RSS feed if we want to be more deterministic
docker run --rm \
  --network autotech-writer_default \
  -e DATABASE_URL="postgres://autotech:password@db:5432/autotech_test?sslmode=disable" \
  -e WEBHOOK_URL="http://python-llm-e2e-test:8000/trigger" \
  -e ONESHOT="true" \
  -e ARXIV_CATEGORIES="cs.AI" \
  autotech-go-collector-e2e-img

echo_info "Waiting for Python pipeline to finish processing..."
# The pipeline runs in the background. We'll wait and then check the DB.
sleep 20

# 5. Verification
echo_info "Verifying results in the database..."

# Check articles table count
ARTICLE_COUNT=$(docker exec autotech-writer-db-1 psql -U autotech -d autotech_test -t -c "SELECT count(*) FROM articles;" | xargs)
echo_info "Total articles in DB: $ARTICLE_COUNT"

if [ "$ARTICLE_COUNT" -eq 0 ]; then
    echo_error "No articles were collected by Go Collector."
    docker logs python-llm-e2e-test
    exit 1
fi

echo_info "Checking for generated content in articles table..."
DRAFT_COUNT=$(docker exec autotech-writer-db-1 psql -U autotech -d autotech_test -t -c "SELECT count(*) FROM articles WHERE status = 'draft';" | xargs || echo "0")
echo_info "Articles with status 'draft': $DRAFT_COUNT"

if [ "$DRAFT_COUNT" -eq 0 ]; then
    echo_error "No articles were processed into drafts by the LLM pipeline."
    echo_info "Current statuses in DB:"
    docker exec autotech-writer-db-1 psql -U autotech -d autotech_test -c "SELECT id, title, status FROM articles;"
    docker logs python-llm-e2e-test
    exit 1
fi

echo_success "E2E Test Success! Flow from Paper Fetching to Post Generation (Draft) is working."
