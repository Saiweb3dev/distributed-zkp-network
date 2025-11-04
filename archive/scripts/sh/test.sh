#!/bin/bash
# test-complete.sh - Full test suite for your running API

API_URL="http://localhost:8080"

echo "========================================"
echo "ZKP Network API - Complete Test Suite"
echo "========================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test counter
PASSED=0
FAILED=0

# ============================================================================
# Test 1: Health Check
# ============================================================================
echo -e "${BLUE}Test 1: Health Check${NC}"
response=$(curl -s -w "\n%{http_code}" $API_URL/health)
status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$status_code" = "200" ]; then
    echo -e "${GREEN}✓ PASSED${NC} - Health endpoint returned 200"
    echo "Response: $body"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Expected 200, got $status_code"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 2: Root Endpoint
# ============================================================================
echo -e "${BLUE}Test 2: Root Endpoint${NC}"
response=$(curl -s -w "\n%{http_code}" $API_URL/)
status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$status_code" = "200" ] && echo "$body" | grep -q "zkp-network"; then
    echo -e "${GREEN}✓ PASSED${NC} - Root endpoint working"
    echo "Response: $body"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Root endpoint check failed"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 3: Merkle Proof - Small Tree (2 leaves)
# ============================================================================
echo -e "${BLUE}Test 3: Merkle Proof - Small Tree (2 leaves)${NC}"
echo -e "${YELLOW}⚠️  First proof will take ~30s (compiling circuit)${NC}"
start=$(date +%s)

response=$(curl -s -w "\n%{http_code}" -X POST $API_URL/api/v1/proofs/merkle \
    -H "Content-Type: application/json" \
    -d '{
        "leaves": ["0x1111111111111111", "0x2222222222222222"],
        "leaf_index": 0
    }')

end=$(date +%s)
duration=$((end - start))
status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

echo "Time taken: ${duration}s"

if [ "$status_code" = "200" ] && echo "$body" | grep -q '"success":true'; then
    echo -e "${GREEN}✓ PASSED${NC} - Small Merkle tree proof generated"
    
    # Pretty print the response
    proof=$(echo "$body" | grep -o '"proof":"[^"]*"' | cut -d'"' -f4 | head -c 40)
    root=$(echo "$body" | grep -o '"merkle_root":"[^"]*"' | cut -d'"' -f4 | head -c 40)
    gen_time=$(echo "$body" | grep -o '"generation_time":"[^"]*"' | cut -d'"' -f4)
    
    echo "  Proof (truncated): ${proof}..."
    echo "  Merkle Root: ${root}..."
    echo "  Generation Time: $gen_time"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Merkle proof generation failed"
    echo "Response: $body"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 4: Merkle Proof - Cached (should be faster)
# ============================================================================
echo -e "${BLUE}Test 4: Merkle Proof - Cached Circuit${NC}"
echo -e "${YELLOW}⚠️  This should be MUCH faster (~2s) due to caching${NC}"
start=$(date +%s)

response=$(curl -s -w "\n%{http_code}" -X POST $API_URL/api/v1/proofs/merkle \
    -H "Content-Type: application/json" \
    -d '{
        "leaves": ["0xaaaaaaaaaaaaaaaa", "0xbbbbbbbbbbbbbbbb"],
        "leaf_index": 1
    }')

end=$(date +%s)
duration=$((end - start))
status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

echo "Time taken: ${duration}s"

if [ "$status_code" = "200" ] && echo "$body" | grep -q '"success":true'; then
    if [ "$duration" -lt 5 ]; then
        echo -e "${GREEN}✓ PASSED${NC} - Cached circuit is fast! (${duration}s < 5s)"
        ((PASSED++))
    else
        echo -e "${YELLOW}⚠ WARNING${NC} - Proof generated but slower than expected (${duration}s)"
        ((PASSED++))
    fi
else
    echo -e "${RED}✗ FAILED${NC} - Cached proof generation failed"
    echo "Response: $body"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 5: Medium Tree (4 leaves)
# ============================================================================
echo -e "${BLUE}Test 5: Merkle Proof - Medium Tree (4 leaves)${NC}"
start=$(date +%s)

response=$(curl -s -w "\n%{http_code}" -X POST $API_URL/api/v1/proofs/merkle \
    -H "Content-Type: application/json" \
    -d '{
        "leaves": [
            "0x1111111111111111",
            "0x2222222222222222",
            "0x3333333333333333",
            "0x4444444444444444"
        ],
        "leaf_index": 2
    }')

end=$(date +%s)
duration=$((end - start))
status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$status_code" = "200" ] && echo "$body" | grep -q '"success":true'; then
    echo -e "${GREEN}✓ PASSED${NC} - Medium tree proof generated (${duration}s)"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Medium tree proof failed"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 6: Large Tree (8 leaves)
# ============================================================================
echo -e "${BLUE}Test 6: Merkle Proof - Large Tree (8 leaves)${NC}"
start=$(date +%s)

response=$(curl -s -w "\n%{http_code}" -X POST $API_URL/api/v1/proofs/merkle \
    -H "Content-Type: application/json" \
    -d '{
        "leaves": [
            "0x1111111111111111", "0x2222222222222222",
            "0x3333333333333333", "0x4444444444444444",
            "0x5555555555555555", "0x6666666666666666",
            "0x7777777777777777", "0x8888888888888888"
        ],
        "leaf_index": 5
    }')

end=$(date +%s)
duration=$((end - start))
status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$status_code" = "200" ] && echo "$body" | grep -q '"success":true'; then
    echo -e "${GREEN}✓ PASSED${NC} - Large tree proof generated (${duration}s)"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Large tree proof failed"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 7: Error Handling - Invalid Leaf Index
# ============================================================================
echo -e "${BLUE}Test 7: Error Handling - Invalid Leaf Index${NC}"
response=$(curl -s -w "\n%{http_code}" -X POST $API_URL/api/v1/proofs/merkle \
    -H "Content-Type: application/json" \
    -d '{
        "leaves": ["0x11", "0x22"],
        "leaf_index": 999
    }')

status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$status_code" = "400" ] && echo "$body" | grep -q '"success":false'; then
    echo -e "${GREEN}✓ PASSED${NC} - Correctly rejected invalid index (400)"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Should return 400 for invalid index"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 8: Error Handling - Empty Leaves
# ============================================================================
echo -e "${BLUE}Test 8: Error Handling - Empty Leaves${NC}"
response=$(curl -s -w "\n%{http_code}" -X POST $API_URL/api/v1/proofs/merkle \
    -H "Content-Type: application/json" \
    -d '{
        "leaves": [],
        "leaf_index": 0
    }')

status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$status_code" = "400" ] && echo "$body" | grep -q '"success":false'; then
    echo -e "${GREEN}✓ PASSED${NC} - Correctly rejected empty leaves (400)"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Should return 400 for empty leaves"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 9: Error Handling - Invalid Hex
# ============================================================================
echo -e "${BLUE}Test 9: Error Handling - Invalid Hex${NC}"
response=$(curl -s -w "\n%{http_code}" -X POST $API_URL/api/v1/proofs/merkle \
    -H "Content-Type: application/json" \
    -d '{
        "leaves": ["not-a-hex-string"],
        "leaf_index": 0
    }')

status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | head -n-1)

if [ "$status_code" = "400" ] && echo "$body" | grep -q '"success":false'; then
    echo -e "${GREEN}✓ PASSED${NC} - Correctly rejected invalid hex (400)"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Should return 400 for invalid hex"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Test 10: Invalid HTTP Method
# ============================================================================
echo -e "${BLUE}Test 10: Invalid HTTP Method${NC}"
response=$(curl -s -w "\n%{http_code}" -X GET $API_URL/api/v1/proofs/merkle)
status_code=$(echo "$response" | tail -n1)

if [ "$status_code" = "405" ]; then
    echo -e "${GREEN}✓ PASSED${NC} - Correctly rejected GET method (405)"
    ((PASSED++))
else
    echo -e "${RED}✗ FAILED${NC} - Should return 405 for wrong method"
    ((FAILED++))
fi
echo ""

# ============================================================================
# Summary
# ============================================================================
echo "========================================"
echo -e "${BLUE}Test Summary${NC}"
echo "========================================"
echo -e "${GREEN}Passed: $PASSED${NC}"
echo -e "${RED}Failed: $FAILED${NC}"
echo ""

if [ "$FAILED" -eq 0 ]; then
    echo -e "${GREEN}🎉 All tests passed! Your API is working perfectly!${NC}"
    exit 0
else
    echo -e "${RED}❌ Some tests failed. Check the output above.${NC}"
    exit 1
fi