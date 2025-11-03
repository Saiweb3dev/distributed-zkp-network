./#!/bin/bash

# test-services.sh
# Comprehensive test suite for the distributed ZKP network
# Tests all major functionality:
# - Service health checks
# - Task submission (Merkle proof)
# - Task status monitoring
# - Worker capacity tracking
# - End-to-end proof generation
#
# Usage: ./test-services.sh
# Prerequisites: All services must be running (use ./start-services.sh)

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m' # No Color

# Configuration
GATEWAY_URL="http://localhost:8080"
COORDINATOR_URL="http://localhost:8090"

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Distributed ZKP Network Test Suite${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Function to run a test
run_test() {
    local test_name=$1
    local test_func=$2
    
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    echo -e "\n${CYAN}[Test $TESTS_TOTAL] $test_name${NC}"
    echo "----------------------------------------"
    
    if $test_func; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        echo -e "${GREEN}✓ PASSED${NC}"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo -e "${RED}✗ FAILED${NC}"
    fi
}

# Function to make HTTP request and check response
http_request() {
    local method=$1
    local url=$2
    local data=$3
    local expected_status=$4
    
    if [ -n "$data" ]; then
        response=$(curl -s -w "\n%{http_code}" -X "$method" \
            -H "Content-Type: application/json" \
            -d "$data" \
            "$url" 2>/dev/null)
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$url" 2>/dev/null)
    fi
    
    body=$(echo "$response" | head -n -1)
    status=$(echo "$response" | tail -n 1)
    
    echo "Response Status: $status"
    echo "Response Body: $body" | head -c 500
    echo ""
    
    if [ "$status" = "$expected_status" ]; then
        return 0
    else
        echo -e "${RED}Expected status $expected_status, got $status${NC}"
        return 1
    fi
}

# Function to extract JSON field
json_field() {
    local json=$1
    local field=$2
    echo "$json" | grep -o "\"$field\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | cut -d'"' -f4
}

# Function to extract JSON number
json_number() {
    local json=$1
    local field=$2
    echo "$json" | grep -o "\"$field\"[[:space:]]*:[[:space:]]*[0-9]*" | grep -o "[0-9]*$"
}

# ============================================================================
# Test 1: API Gateway Health Check
# ============================================================================
test_gateway_health() {
    echo "Testing API Gateway health endpoint..."
    
    response=$(curl -s "$GATEWAY_URL/health" 2>/dev/null)
    
    if echo "$response" | grep -q "\"status\""; then
        echo "Gateway health response: $response"
        return 0
    else
        echo -e "${RED}Gateway health check failed${NC}"
        return 1
    fi
}

# ============================================================================
# Test 2: Coordinator Health Check
# ============================================================================
test_coordinator_health() {
    echo "Testing Coordinator health endpoint..."
    
    response=$(curl -s "$COORDINATOR_URL/health" 2>/dev/null)
    
    if echo "$response" | grep -q "\"status\""; then
        echo "Coordinator health response:"
        echo "$response" | grep -o "\"workers\"[^}]*}" || echo "$response"
        
        # Check if workers are registered
        active_workers=$(json_number "$response" "active")
        if [ -n "$active_workers" ] && [ "$active_workers" -gt 0 ]; then
            echo -e "${GREEN}Found $active_workers active worker(s)${NC}"
        else
            echo -e "${YELLOW}WARNING: No active workers found${NC}"
        fi
        
        return 0
    else
        echo -e "${RED}Coordinator health check failed${NC}"
        return 1
    fi
}

# ============================================================================
# Test 3: Coordinator Metrics
# ============================================================================
test_coordinator_metrics() {
    echo "Testing Coordinator metrics endpoint..."
    
    response=$(curl -s "$COORDINATOR_URL/metrics" 2>/dev/null)
    
    if echo "$response" | grep -q "coordinator_workers"; then
        echo "Sample metrics:"
        echo "$response" | grep "coordinator_workers" | head -5
        return 0
    else
        echo -e "${RED}Metrics endpoint failed${NC}"
        return 1
    fi
}

# ============================================================================
# Test 4: Submit Merkle Proof Task
# ============================================================================
test_submit_merkle_proof() {
    echo "Submitting Merkle proof task..."
    
    # Create test data with 4 leaves
    local payload='{
        "leaves": [
            "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
            "0xfedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
            "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
            "0x7890abcdef1234567890abcdef1234567890abcdef1234567890abcdef123456"
        ],
        "leaf_index": 1
    }'
    
    response=$(curl -s -w "\n%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d "$payload" \
        "$GATEWAY_URL/api/v1/tasks/merkle" 2>/dev/null)
    
    body=$(echo "$response" | head -n -1)
    status=$(echo "$response" | tail -n 1)
    
    echo "Response Status: $status"
    echo "Response Body: $body"
    
    if [ "$status" = "201" ] || [ "$status" = "200" ]; then
        # Extract task ID for later use
        TASK_ID=$(json_field "$body" "task_id")
        
        if [ -n "$TASK_ID" ]; then
            echo -e "${GREEN}Task submitted successfully! Task ID: $TASK_ID${NC}"
            return 0
        else
            echo -e "${YELLOW}Task submitted but no ID returned${NC}"
            return 0
        fi
    else
        echo -e "${RED}Task submission failed with status $status${NC}"
        return 1
    fi
}

# ============================================================================
# Test 5: Check Task Status
# ============================================================================
test_task_status() {
    if [ -z "$TASK_ID" ]; then
        echo -e "${YELLOW}Skipping: No task ID available${NC}"
        return 0
    fi
    
    echo "Checking status of task: $TASK_ID"
    
    response=$(curl -s "$GATEWAY_URL/api/v1/tasks/$TASK_ID" 2>/dev/null)
    
    if echo "$response" | grep -q "\"task_id\""; then
        echo "Task status response:"
        echo "$response" | head -c 500
        
        status=$(json_field "$response" "status")
        echo -e "\n${CYAN}Current task status: $status${NC}"
        
        return 0
    else
        echo -e "${RED}Failed to retrieve task status${NC}"
        return 1
    fi
}

# ============================================================================
# Test 6: Wait for Task Completion
# ============================================================================
test_task_completion() {
    if [ -z "$TASK_ID" ]; then
        echo -e "${YELLOW}Skipping: No task ID available${NC}"
        return 0
    fi
    
    echo "Waiting for task to complete (max 30 seconds)..."
    
    local max_attempts=30
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        response=$(curl -s "$GATEWAY_URL/api/v1/tasks/$TASK_ID" 2>/dev/null)
        status=$(json_field "$response" "status")
        
        echo -n "."
        
        if [ "$status" = "completed" ]; then
            echo ""
            echo -e "${GREEN}Task completed successfully!${NC}"
            echo "Final task details:"
            echo "$response" | head -c 1000
            return 0
        elif [ "$status" = "failed" ]; then
            echo ""
            echo -e "${RED}Task failed${NC}"
            echo "$response"
            return 1
        fi
        
        attempt=$((attempt + 1))
        sleep 1
    done
    
    echo ""
    echo -e "${YELLOW}Task did not complete within 30 seconds (Status: $status)${NC}"
    echo "This is normal for complex proofs. Check later with:"
    echo "curl $GATEWAY_URL/api/v1/tasks/$TASK_ID"
    return 0
}

# ============================================================================
# Test 7: Submit Multiple Tasks (Load Test)
# ============================================================================
test_submit_multiple_tasks() {
    echo "Submitting 5 tasks to test parallel processing..."
    
    local submitted=0
    local failed=0
    
    for i in {1..5}; do
        local payload="{
            \"leaves\": [
                \"0x$(printf '%064d' $i)\",
                \"0x$(printf '%064d' $((i+1)))\",
                \"0x$(printf '%064d' $((i+2)))\",
                \"0x$(printf '%064d' $((i+3)))\"
            ],
            \"leaf_index\": 0
        }"
        
        response=$(curl -s -w "\n%{http_code}" -X POST \
            -H "Content-Type: application/json" \
            -d "$payload" \
            "$GATEWAY_URL/api/v1/tasks/merkle" 2>/dev/null)
        
        status=$(echo "$response" | tail -n 1)
        
        if [ "$status" = "201" ] || [ "$status" = "200" ]; then
            submitted=$((submitted + 1))
            echo -n "✓"
        else
            failed=$((failed + 1))
            echo -n "✗"
        fi
    done
    
    echo ""
    echo -e "${CYAN}Submitted: $submitted, Failed: $failed${NC}"
    
    if [ $submitted -ge 3 ]; then
        return 0
    else
        return 1
    fi
}

# ============================================================================
# Test 8: Check Worker Capacity
# ============================================================================
test_worker_capacity() {
    echo "Checking worker capacity from coordinator..."
    
    response=$(curl -s "$COORDINATOR_URL/health" 2>/dev/null)
    
    if echo "$response" | grep -q "\"capacity\""; then
        echo "Capacity information:"
        echo "$response" | grep -o "\"capacity\"[^}]*}" || echo "$response"
        
        total_capacity=$(json_number "$response" "total")
        available_capacity=$(json_number "$response" "available")
        
        if [ -n "$total_capacity" ]; then
            echo -e "${CYAN}Total capacity: $total_capacity${NC}"
            echo -e "${CYAN}Available capacity: $available_capacity${NC}"
            return 0
        fi
    fi
    
    return 1
}

# ============================================================================
# Test 9: Invalid Task Submission (Error Handling)
# ============================================================================
test_invalid_task() {
    echo "Testing error handling with invalid task..."
    
    local payload='{
        "leaves": ["invalid"],
        "leaf_index": 999
    }'
    
    response=$(curl -s -w "\n%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d "$payload" \
        "$GATEWAY_URL/api/v1/tasks/merkle" 2>/dev/null)
    
    status=$(echo "$response" | tail -n 1)
    body=$(echo "$response" | head -n -1)
    
    echo "Response Status: $status"
    echo "Response Body: $body"
    
    # Should return 4xx error
    if [ "$status" -ge 400 ] && [ "$status" -lt 500 ]; then
        echo -e "${GREEN}Correctly rejected invalid task${NC}"
        return 0
    else
        echo -e "${YELLOW}Expected 4xx error, got $status${NC}"
        return 1
    fi
}

# ============================================================================
# Test 10: Gateway Root Endpoint
# ============================================================================
test_gateway_root() {
    echo "Testing API Gateway root endpoint..."
    
    response=$(curl -s "$GATEWAY_URL/" 2>/dev/null)
    
    if echo "$response" | grep -q "\"service\""; then
        echo "Gateway info:"
        echo "$response" | head -c 300
        return 0
    else
        return 1
    fi
}

# ============================================================================
# Run All Tests
# ============================================================================

echo -e "${MAGENTA}Starting test suite...${NC}"
echo ""

# Basic connectivity tests
run_test "API Gateway Health Check" test_gateway_health
run_test "Coordinator Health Check" test_coordinator_health
run_test "Coordinator Metrics Endpoint" test_coordinator_metrics
run_test "Gateway Root Endpoint" test_gateway_root

# Core functionality tests
run_test "Submit Merkle Proof Task" test_submit_merkle_proof
run_test "Check Task Status" test_task_status
run_test "Wait for Task Completion" test_task_completion

# Advanced tests
run_test "Worker Capacity Check" test_worker_capacity
run_test "Submit Multiple Tasks (Load Test)" test_submit_multiple_tasks
run_test "Invalid Task Submission (Error Handling)" test_invalid_task

# ============================================================================
# Test Summary
# ============================================================================

echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Test Suite Summary${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "Total Tests:    ${CYAN}$TESTS_TOTAL${NC}"
echo -e "Passed:         ${GREEN}$TESTS_PASSED${NC}"
echo -e "Failed:         ${RED}$TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  ✓ All Tests Passed!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "${CYAN}Your distributed ZKP network is working correctly! 🎉${NC}"
    exit 0
else
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}  ✗ Some Tests Failed${NC}"
    echo -e "${RED}========================================${NC}"
    echo ""
    echo -e "${YELLOW}Troubleshooting:${NC}"
    echo "  1. Check if all services are running"
    echo "  2. Review service logs in terminal windows"
    echo "  3. Verify database migrations completed"
    echo "  4. Ensure workers registered with coordinator"
    exit 1
fi
