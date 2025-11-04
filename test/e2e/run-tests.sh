#!/bin/bash

# ============================================================================
# Automated Test Script for Distributed ZKP Network
# ============================================================================
# This script runs automated tests for the Raft cluster and ZKP network
# ============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results
TESTS_PASSED=0
TESTS_FAILED=0
TOTAL_TESTS=0

# Logging
LOG_FILE="test_results_$(date +%Y%m%d_%H%M%S).log"

# ============================================================================
# Helper Functions
# ============================================================================

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

log_success() {
    echo -e "${GREEN}✓ PASS:${NC} $1" | tee -a "$LOG_FILE"
    ((TESTS_PASSED++))
    ((TOTAL_TESTS++))
}

log_fail() {
    echo -e "${RED}✗ FAIL:${NC} $1" | tee -a "$LOG_FILE"
    ((TESTS_FAILED++))
    ((TOTAL_TESTS++))
}

log_warning() {
    echo -e "${YELLOW}⚠ WARNING:${NC} $1" | tee -a "$LOG_FILE"
}

section() {
    echo "" | tee -a "$LOG_FILE"
    echo "============================================================================" | tee -a "$LOG_FILE"
    echo "$1" | tee -a "$LOG_FILE"
    echo "============================================================================" | tee -a "$LOG_FILE"
}

# ============================================================================
# Test Functions
# ============================================================================

test_containers_running() {
    section "Test 1: Container Health"
    
    local containers=("zkp-postgres-cluster" "zkp-coordinator-1" "zkp-coordinator-2" 
                     "zkp-coordinator-3" "zkp-worker-1-cluster" "zkp-worker-2-cluster" 
                     "zkp-api-gateway-cluster")
    
    for container in "${containers[@]}"; do
        if docker ps | grep -q "$container"; then
            log_success "$container is running"
        else
            log_fail "$container is NOT running"
        fi
    done
}

test_health_endpoints() {
    section "Test 2: Health Endpoints"
    
    # Test coordinator-1
    if curl -s -f http://localhost:8090/health > /dev/null; then
        log_success "Coordinator-1 health endpoint responding"
    else
        log_fail "Coordinator-1 health endpoint NOT responding"
    fi
    
    # Test coordinator-2
    if curl -s -f http://localhost:8091/health > /dev/null; then
        log_success "Coordinator-2 health endpoint responding"
    else
        log_fail "Coordinator-2 health endpoint NOT responding"
    fi
    
    # Test coordinator-3
    if curl -s -f http://localhost:8092/health > /dev/null; then
        log_success "Coordinator-3 health endpoint responding"
    else
        log_fail "Coordinator-3 health endpoint NOT responding"
    fi
    
    # Test API Gateway
    if curl -s -f http://localhost:8080/health > /dev/null; then
        log_success "API Gateway health endpoint responding"
    else
        log_fail "API Gateway health endpoint NOT responding"
    fi
}

test_leader_election() {
    section "Test 3: Raft Leader Election"
    
    # Count leaders
    local leader_count=0
    
    if curl -s http://localhost:8090/health | grep -q '"is_leader":true'; then
        ((leader_count++))
    fi
    
    if curl -s http://localhost:8091/health | grep -q '"is_leader":true'; then
        ((leader_count++))
    fi
    
    if curl -s http://localhost:8092/health | grep -q '"is_leader":true'; then
        ((leader_count++))
    fi
    
    if [ "$leader_count" -eq 1 ]; then
        log_success "Exactly one leader elected (count: $leader_count)"
    else
        log_fail "Invalid leader count: $leader_count (expected 1)"
    fi
    
    # Check leader address consistency
    local leader1=$(curl -s http://localhost:8090/health | grep -o '"leader_address":"[^"]*"' | cut -d'"' -f4)
    local leader2=$(curl -s http://localhost:8091/health | grep -o '"leader_address":"[^"]*"' | cut -d'"' -f4)
    local leader3=$(curl -s http://localhost:8092/health | grep -o '"leader_address":"[^"]*"' | cut -d'"' -f4)
    
    if [ "$leader1" = "$leader2" ] && [ "$leader2" = "$leader3" ]; then
        log_success "All coordinators agree on leader: $leader1"
    else
        log_fail "Leader address mismatch: $leader1, $leader2, $leader3"
    fi
}

test_node_identity() {
    section "Test 4: Node Identity"
    
    # Check coordinator-1 ID
    local id1=$(curl -s http://localhost:8090/health | grep -o '"coordinator_id":"[^"]*"' | cut -d'"' -f4)
    if [ "$id1" = "coordinator-1" ]; then
        log_success "Coordinator-1 has correct ID: $id1"
    else
        log_fail "Coordinator-1 has wrong ID: $id1 (expected coordinator-1)"
    fi
    
    # Check coordinator-2 ID
    local id2=$(curl -s http://localhost:8091/health | grep -o '"coordinator_id":"[^"]*"' | cut -d'"' -f4)
    if [ "$id2" = "coordinator-2" ]; then
        log_success "Coordinator-2 has correct ID: $id2"
    else
        log_fail "Coordinator-2 has wrong ID: $id2 (expected coordinator-2)"
    fi
    
    # Check coordinator-3 ID
    local id3=$(curl -s http://localhost:8092/health | grep -o '"coordinator_id":"[^"]*"' | cut -d'"' -f4)
    if [ "$id3" = "coordinator-3" ]; then
        log_success "Coordinator-3 has correct ID: $id3"
    else
        log_fail "Coordinator-3 has wrong ID: $id3 (expected coordinator-3)"
    fi
    
    # Check for duplicates
    if [ "$id1" != "$id2" ] && [ "$id2" != "$id3" ] && [ "$id1" != "$id3" ]; then
        log_success "All coordinator IDs are unique"
    else
        log_fail "Duplicate coordinator IDs detected"
    fi
}

test_worker_registration() {
    section "Test 5: Worker Registration"
    
    # Get worker count from leader
    local leader_port=8090
    if curl -s http://localhost:8091/health | grep -q '"is_leader":true'; then
        leader_port=8091
    elif curl -s http://localhost:8092/health | grep -q '"is_leader":true'; then
        leader_port=8092
    fi
    
    local total_workers=$(curl -s http://localhost:$leader_port/health | grep -o '"total":[0-9]*' | head -1 | cut -d':' -f2)
    local active_workers=$(curl -s http://localhost:$leader_port/health | grep -o '"active":[0-9]*' | head -1 | cut -d':' -f2)
    
    if [ "$total_workers" -ge 1 ]; then
        log_success "Workers registered: $total_workers total, $active_workers active"
    else
        log_fail "No workers registered (expected at least 1)"
    fi
    
    # Check worker capacity
    local total_capacity=$(curl -s http://localhost:$leader_port/health | grep -o '"total":[0-9]*' | tail -1 | cut -d':' -f2)
    if [ "$total_capacity" -gt 0 ]; then
        log_success "Worker capacity available: $total_capacity slots"
    else
        log_warning "No worker capacity available"
    fi
}

test_term_stability() {
    section "Test 6: Raft Term Stability"
    
    # Get initial term
    local term1=$(curl -s http://localhost:8090/health | grep -o '"term":[0-9]*' | cut -d':' -f2)
    log "Initial term: $term1"
    
    # Wait 10 seconds
    log "Waiting 10 seconds..."
    sleep 10
    
    # Get term again
    local term2=$(curl -s http://localhost:8090/health | grep -o '"term":[0-9]*' | cut -d':' -f2)
    log "Term after 10 seconds: $term2"
    
    if [ "$term1" = "$term2" ]; then
        log_success "Term is stable (no elections): $term1"
    else
        log_warning "Term changed from $term1 to $term2 (possible re-election)"
    fi
}

test_database_connectivity() {
    section "Test 7: Database Connectivity"
    
    # Check if postgres is running
    if docker ps | grep -q "zkp-postgres-cluster"; then
        log_success "PostgreSQL container running"
    else
        log_fail "PostgreSQL container NOT running"
        return
    fi
    
    # Test database connection
    if docker exec zkp-postgres-cluster psql -U zkp_user -d zkp_network -c "SELECT 1;" > /dev/null 2>&1; then
        log_success "Database connection successful"
    else
        log_fail "Database connection failed"
    fi
    
    # Check coordinator logs for DB connection
    if docker logs zkp-coordinator-1 2>&1 | grep -q "Database connected"; then
        log_success "Coordinator-1 connected to database"
    else
        log_warning "Could not verify coordinator-1 database connection in logs"
    fi
}

test_api_gateway() {
    section "Test 8: API Gateway"
    
    # Test health endpoint
    local health_status=$(curl -s http://localhost:8080/health | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
    if [ "$health_status" = "healthy" ]; then
        log_success "API Gateway is healthy"
    else
        log_fail "API Gateway status: $health_status (expected healthy)"
    fi
    
    # Test ready endpoint
    if curl -s -f http://localhost:8080/ready > /dev/null; then
        log_success "API Gateway ready endpoint responding"
    else
        log_warning "API Gateway not ready"
    fi
}

test_log_replication() {
    section "Test 9: Log Replication"
    
    # Check leader logs for replication
    if docker logs zkp-coordinator-1 2>&1 | grep -q "pipelining replication"; then
        log_success "Log replication active (pipelining detected)"
    else
        log_warning "Could not verify log replication in logs"
    fi
    
    # Check for replication errors
    if docker logs zkp-coordinator-1 --tail=100 2>&1 | grep -q "replication.*error"; then
        log_fail "Replication errors detected in logs"
    else
        log_success "No replication errors in recent logs"
    fi
}

test_leader_failover() {
    section "Test 10: Leader Failover (Optional - Destructive)"
    
    log_warning "This test stops the leader and verifies failover"
    log_warning "Skipping by default (uncomment to enable)"
    
    # Uncomment to enable:
    # local leader_port=8090
    # if curl -s http://localhost:8091/health | grep -q '"is_leader":true'; then
    #     leader_port=8091
    # elif curl -s http://localhost:8092/health | grep -q '"is_leader":true'; then
    #     leader_port=8092
    # fi
    # 
    # local leader_id="coordinator-$((leader_port - 8089))"
    # log "Current leader: $leader_id (port $leader_port)"
    # 
    # docker stop zkp-$leader_id
    # log "Stopped $leader_id, waiting for election..."
    # sleep 5
    # 
    # # Check if new leader elected
    # local new_leader_count=0
    # for port in 8090 8091 8092; do
    #     if [ $port -ne $leader_port ] && curl -s http://localhost:$port/health | grep -q '"is_leader":true'; then
    #         ((new_leader_count++))
    #     fi
    # done
    # 
    # if [ $new_leader_count -eq 1 ]; then
    #     log_success "New leader elected after failover"
    # else
    #     log_fail "Leader election failed after failover"
    # fi
    # 
    # docker start zkp-$leader_id
    # log "Restarted $leader_id"
}

test_response_time() {
    section "Test 11: Response Time"
    
    log "Measuring health endpoint response time (10 samples)..."
    
    local total_time=0
    local samples=10
    
    for i in $(seq 1 $samples); do
        local response_time=$(curl -s -o /dev/null -w "%{time_total}" http://localhost:8090/health)
        total_time=$(echo "$total_time + $response_time" | bc)
    done
    
    local avg_time=$(echo "scale=4; $total_time / $samples" | bc)
    log "Average response time: ${avg_time}s"
    
    if (( $(echo "$avg_time < 0.1" | bc -l) )); then
        log_success "Response time is good: ${avg_time}s (< 100ms)"
    elif (( $(echo "$avg_time < 0.5" | bc -l) )); then
        log_warning "Response time is acceptable: ${avg_time}s (< 500ms)"
    else
        log_fail "Response time is slow: ${avg_time}s (> 500ms)"
    fi
}

test_memory_usage() {
    section "Test 12: Memory Usage"
    
    log "Checking container memory usage..."
    
    # Get memory stats
    docker stats --no-stream --format "{{.Name}}: {{.MemUsage}}" | while read line; do
        log "$line"
    done
    
    # Check if any container using > 1GB
    local high_mem=$(docker stats --no-stream --format "{{.Name}},{{.MemPerc}}" | awk -F, '$2 > 50.0 {print $1}')
    
    if [ -z "$high_mem" ]; then
        log_success "All containers using < 50% memory"
    else
        log_warning "High memory usage in: $high_mem"
    fi
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    section "Distributed ZKP Network - Automated Test Suite"
    log "Start time: $(date)"
    log "Log file: $LOG_FILE"
    
    # Check prerequisites
    if ! command -v docker &> /dev/null; then
        log_fail "Docker is not installed"
        exit 1
    fi
    
    if ! command -v curl &> /dev/null; then
        log_fail "curl is not installed"
        exit 1
    fi
    
    if ! command -v bc &> /dev/null; then
        log_warning "bc is not installed (some tests will be skipped)"
    fi
    
    # Run tests
    test_containers_running
    sleep 2
    test_health_endpoints
    sleep 2
    test_leader_election
    sleep 2
    test_node_identity
    sleep 2
    test_worker_registration
    sleep 2
    test_term_stability
    sleep 2
    test_database_connectivity
    sleep 2
    test_api_gateway
    sleep 2
    test_log_replication
    sleep 2
    # test_leader_failover  # Uncomment to enable
    test_response_time
    sleep 2
    test_memory_usage
    
    # Summary
    section "Test Summary"
    log "Total tests: $TOTAL_TESTS"
    log_success "Passed: $TESTS_PASSED"
    log_fail "Failed: $TESTS_FAILED"
    
    local pass_rate=$(echo "scale=2; $TESTS_PASSED * 100 / $TOTAL_TESTS" | bc 2>/dev/null || echo "N/A")
    log "Pass rate: ${pass_rate}%"
    
    if [ $TESTS_FAILED -eq 0 ]; then
        section "✓ ALL TESTS PASSED"
        exit 0
    else
        section "✗ SOME TESTS FAILED"
        exit 1
    fi
}

# Run main function
main "$@"
