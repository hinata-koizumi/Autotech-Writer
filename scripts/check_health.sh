#!/bin/bash
# Autotech Writer System Health Check Script

set -e

# ANSI color codes
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

WAIT_MODE=0
MAX_RETRIES=12
RETRY_INTERVAL=5

# Parse arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        -w|--wait) WAIT_MODE=1 ;;
        *) echo "Unknown parameter passed: $1"; exit 1 ;;
    esac
    shift
done

echo -e "${BLUE}=== Autotech Writer System Health Check ===${NC}"

# 1. Check for .env file
if [ -f ".env" ]; then
    echo -e "${GREEN}[OK] .env file exists.${NC}"
else
    echo -e "${RED}[ERROR] .env file not found. Please create it from .env.example.${NC}"
    exit 1
fi

# 2. Check Docker overall status
if docker info > /dev/null 2>&1; then
    echo -e "${GREEN}[OK] Docker daemon is running.${NC}"
    DOCKER_AVAILABLE=1
else
    echo -e "${YELLOW}[WARN] Docker daemon is not running or current user has no permission.${NC}"
    echo -e "${YELLOW}       Some checks requiring Docker will be skipped.${NC}"
    DOCKER_AVAILABLE=0
fi

check_containers() {
    local all_running=1
    CONTAINERS=("autotech-writer-db-1" "autotech-writer-python-llm-1" "autotech-writer-go-collector-1")
    for container in "${CONTAINERS[@]}"; do
        if docker ps --format '{{.Names}}' | grep -q "^$container$"; then
            echo -e "${GREEN}[OK] Container '$container' is running.${NC}"
        else
            echo -e "${YELLOW}[WARN] Container '$container' is NOT running.${NC}"
            all_running=0
        fi
    done
    return $all_running
}

# 3. Check Docker Containers (if Docker available)
if [ "$DOCKER_AVAILABLE" -eq 1 ]; then
    echo -e "${BLUE}--- Checking Docker Containers ---${NC}"
    if [ "$WAIT_MODE" -eq 1 ]; then
        for ((i=1; i<=MAX_RETRIES; i++)); do
            if check_containers; then break; fi
            echo -e "${YELLOW}Waiting for containers... (Attempt $i/$MAX_RETRIES)${NC}"
            sleep $RETRY_INTERVAL
        done
    else
        check_containers || true
    fi
fi

# 4. Check Database Connectivity
echo -e "${BLUE}--- Checking Database Connection ---${NC}"
DB_URL=$(grep "^DATABASE_URL=" .env | cut -d '=' -f2- | sed 's/\r//')
if [ -n "$DB_URL" ]; then
    if [ "$DOCKER_AVAILABLE" -eq 1 ]; then
        if docker ps --format '{{.Names}}' | grep -q "autotech-writer-db-1"; then
            if [ "$WAIT_MODE" -eq 1 ]; then
                echo -e "Waiting for database to be ready..."
                docker exec autotech-writer-db-1 pg_isready -U postgres -t 60 || echo -e "${RED}[ERROR] Database timeout.${NC}"
            fi
            if docker exec autotech-writer-db-1 pg_isready -U postgres > /dev/null 2>&1; then
                echo -e "${GREEN}[OK] Database is ready.${NC}"
            else
                echo -e "${RED}[ERROR] Database is NOT ready inside container.${NC}"
                echo -e "${YELLOW}Tip: Check 'docker logs autotech-writer-db-1' for startup errors.${NC}"
            fi
        fi
    fi
else
    echo -e "${RED}[ERROR] DATABASE_URL not found in .env.${NC}"
fi

# 5. Check Python-LLM Health Endpoint
echo -e "${BLUE}--- Checking Python-LLM API ---${NC}"
WEBHOOK_URL=$(grep "^WEBHOOK_URL=" .env | cut -d '=' -f2- | sed 's/\r//')
if [ -n "$WEBHOOK_URL" ]; then
    HEALTH_URL=$(echo "$WEBHOOK_URL" | sed 's/\/trigger/\/health/')
    
    check_llm_health() {
        if curl -s --max-time 5 "$HEALTH_URL" | grep -q '"status":"ok"'; then
            echo -e "${GREEN}[OK] Python-LLM health check successful ($HEALTH_URL).${NC}"
            return 0
        else
            return 1
        fi
    }

    if [ "$WAIT_MODE" -eq 1 ]; then
        for ((i=1; i<=MAX_RETRIES; i++)); do
            if check_llm_health; then break; fi
            echo -e "${YELLOW}Waiting for Python-LLM... (Attempt $i/$MAX_RETRIES)${NC}"
            sleep $RETRY_INTERVAL
            if [ $i -eq $MAX_RETRIES ]; then
                echo -e "${RED}[ERROR] Python-LLM health check failed after timeout.${NC}"
            fi
        done
    else
        check_llm_health || echo -e "${RED}[ERROR] Python-LLM health check failed ($HEALTH_URL).${NC}"
    fi
else
    echo -e "${YELLOW}[WARN] WEBHOOK_URL not found in .env, skipping python-llm health check.${NC}"
fi

# 6. Check Go-Collector Logs (if container running)
if [ "$DOCKER_AVAILABLE" -eq 1 ]; then
    echo -e "${BLUE}--- Checking Go-Collector Logs ---${NC}"
    if docker ps --format '{{.Names}}' | grep -q "autotech-writer-go-collector-1"; then
        if docker logs --tail 20 autotech-writer-go-collector-1 2>&1 | grep -i "error"; then
            echo -e "${YELLOW}[WARN] Recent errors found in go-collector logs.${NC}"
            echo -e "${YELLOW}       Run 'docker logs autotech-writer-go-collector-1' to see details.${NC}"
        else
            echo -e "${GREEN}[OK] No recent errors in go-collector logs.${NC}"
        fi
    fi
fi

echo -e "${BLUE}=== Health Check Completed ===${NC}"
