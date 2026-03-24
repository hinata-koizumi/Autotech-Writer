#!/bin/bash
# Autotech Writer - Run All Tests

set -e

# ANSI color codes
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}=======================================${NC}"
echo -e "${BLUE}   Autotech Writer Test Suite Runner   ${NC}"
echo -e "${BLUE}=======================================${NC}"

# 1. Run E2E Test Flow (Transient environment)
echo -e "\n${BLUE}[1/2] Running E2E Test Workflow...${NC}"
if bash tests/e2e/test_flow.sh; then
    E2E_STATUS="${GREEN}PASSED${NC}"
else
    E2E_STATUS="${RED}FAILED${NC}"
    FAILED=1
fi

# 2. Run Health Check (Production environment - might be down)
echo -e "\n${BLUE}[2/2] Running System Health Check...${NC}"
# We don't exit on failure here because production might be intended to be down
if bash scripts/check_health.sh; then
    HEALTH_STATUS="${GREEN}OK${NC}"
else
    # Check if it's just containers not running (common during dev)
    if docker ps --format '{{.Names}}' | grep -q "autotech-writer"; then
         HEALTH_STATUS="${RED}FAILED (Some services down)${NC}"
    else
         HEALTH_STATUS="${YELLOW}SKIPPED (Containers not running)${NC}"
    fi
fi

echo -e "\n${BLUE}=======================================${NC}"
echo -e "${BLUE}            Test Summary               ${NC}"
echo -e "${BLUE}=======================================${NC}"
echo -e "E2E Test Flow:   $E2E_STATUS"
echo -e "System Health:   $HEALTH_STATUS"
echo -e "${BLUE}=======================================${NC}"

if [ "$FAILED" == "1" ]; then
    echo -e "${RED}Tests failed. Please check the logs above.${NC}"
    exit 1
else
    echo -e "${GREEN}All essential tests passed!${NC}"
fi
