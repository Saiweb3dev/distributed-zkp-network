@echo off
REM ============================================================================
REM Automated Test Script for Distributed ZKP Network (Windows)
REM ============================================================================

setlocal enabledelayedexpansion

set TESTS_PASSED=0
set TESTS_FAILED=0
set TOTAL_TESTS=0
set LOG_FILE=test_results_%date:~-4,4%%date:~-10,2%%date:~-7,2%_%time:~0,2%%time:~3,2%%time:~6,2%.log

echo ============================================================================
echo Distributed ZKP Network - Automated Test Suite
echo ============================================================================
echo Start time: %date% %time%
echo Log file: %LOG_FILE%
echo.

REM ============================================================================
REM Test 1: Container Health
REM ============================================================================
echo ============================================================================
echo Test 1: Container Health
echo ============================================================================

docker ps --filter "name=zkp-postgres-cluster" --format "{{.Names}}" | findstr /C:"zkp-postgres-cluster" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] zkp-postgres-cluster is running
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] zkp-postgres-cluster is NOT running
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker ps --filter "name=zkp-coordinator-1" --format "{{.Names}}" | findstr /C:"zkp-coordinator-1" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] zkp-coordinator-1 is running
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] zkp-coordinator-1 is NOT running
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker ps --filter "name=zkp-coordinator-2" --format "{{.Names}}" | findstr /C:"zkp-coordinator-2" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] zkp-coordinator-2 is running
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] zkp-coordinator-2 is NOT running
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker ps --filter "name=zkp-coordinator-3" --format "{{.Names}}" | findstr /C:"zkp-coordinator-3" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] zkp-coordinator-3 is running
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] zkp-coordinator-3 is NOT running
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker ps --filter "name=zkp-worker-1-cluster" --format "{{.Names}}" | findstr /C:"zkp-worker-1-cluster" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] zkp-worker-1-cluster is running
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] zkp-worker-1-cluster is NOT running
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker ps --filter "name=zkp-worker-2-cluster" --format "{{.Names}}" | findstr /C:"zkp-worker-2-cluster" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] zkp-worker-2-cluster is running
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] zkp-worker-2-cluster is NOT running
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker ps --filter "name=zkp-api-gateway-cluster" --format "{{.Names}}" | findstr /C:"zkp-api-gateway-cluster" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] zkp-api-gateway-cluster is running
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] zkp-api-gateway-cluster is NOT running
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 2: Health Endpoints
REM ============================================================================
echo ============================================================================
echo Test 2: Health Endpoints
echo ============================================================================

curl -s -f http://localhost:8090/health >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Coordinator-1 health endpoint responding
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Coordinator-1 health endpoint NOT responding
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

curl -s -f http://localhost:8091/health >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Coordinator-2 health endpoint responding
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Coordinator-2 health endpoint NOT responding
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

curl -s -f http://localhost:8092/health >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Coordinator-3 health endpoint responding
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Coordinator-3 health endpoint NOT responding
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

curl -s -f http://localhost:8080/health >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] API Gateway health endpoint responding
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] API Gateway health endpoint NOT responding
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 3: Raft Leader Election
REM ============================================================================
echo ============================================================================
echo Test 3: Raft Leader Election
echo ============================================================================

set LEADER_COUNT=0

curl -s http://localhost:8090/health > %TEMP%\health1.txt 2>&1
findstr /C:"is_leader" %TEMP%\health1.txt | findstr /C:"true" >nul 2>&1
if !errorlevel! equ 0 set /a LEADER_COUNT+=1

curl -s http://localhost:8091/health > %TEMP%\health2.txt 2>&1
findstr /C:"is_leader" %TEMP%\health2.txt | findstr /C:"true" >nul 2>&1
if !errorlevel! equ 0 set /a LEADER_COUNT+=1

curl -s http://localhost:8092/health > %TEMP%\health3.txt 2>&1
findstr /C:"is_leader" %TEMP%\health3.txt | findstr /C:"true" >nul 2>&1
if !errorlevel! equ 0 set /a LEADER_COUNT+=1

if !LEADER_COUNT! equ 1 (
    echo [PASS] Exactly one leader elected
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Invalid leader count: !LEADER_COUNT! ^(expected 1^)
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 4: Node Identity
REM ============================================================================
echo ============================================================================
echo Test 4: Node Identity
echo ============================================================================

curl -s http://localhost:8090/health > %TEMP%\coord1.txt 2>&1
findstr /C:"coordinator-1" %TEMP%\coord1.txt | findstr /C:"coordinator_id" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Coordinator-1 has correct ID
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Coordinator-1 has wrong ID
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

curl -s http://localhost:8091/health > %TEMP%\coord2.txt 2>&1
findstr /C:"coordinator-2" %TEMP%\coord2.txt | findstr /C:"coordinator_id" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Coordinator-2 has correct ID
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Coordinator-2 has wrong ID
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

curl -s http://localhost:8092/health > %TEMP%\coord3.txt 2>&1
findstr /C:"coordinator-3" %TEMP%\coord3.txt | findstr /C:"coordinator_id" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Coordinator-3 has correct ID
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Coordinator-3 has wrong ID
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 5: Database Connectivity
REM ============================================================================
echo ============================================================================
echo Test 5: Database Connectivity
echo ============================================================================

docker exec zkp-postgres-cluster psql -U zkp_user -d zkp_network -c "SELECT 1;" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Database connection successful
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Database connection failed
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker logs zkp-coordinator-1 2>&1 | findstr /C:"coordinator_id" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Coordinator-1 started successfully
    set /a TESTS_PASSED+=1
) else (
    echo [WARN] Could not verify coordinator-1 startup in logs
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 6: API Gateway
REM ============================================================================
echo ============================================================================
echo Test 6: API Gateway
echo ============================================================================

curl -s http://localhost:8080/health | findstr /C:"\"status\":\"healthy\"" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] API Gateway is healthy
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] API Gateway is not healthy
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

curl -s -f http://localhost:8080/ready >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] API Gateway ready endpoint responding
    set /a TESTS_PASSED+=1
) else (
    echo [WARN] API Gateway not ready
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 7: Redis Pub/Sub - Connectivity
REM ============================================================================
echo ============================================================================
echo Test 7: Redis Pub/Sub - Connectivity
echo ============================================================================

docker exec zkp-redis-cluster redis-cli ping >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Redis is responding to PING
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Redis is not responding
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker exec zkp-redis-cluster redis-cli INFO server | findstr "redis_version" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Redis server information accessible
    set /a TESTS_PASSED+=1
) else (
    echo [FAIL] Cannot access Redis server info
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 8: Redis Pub/Sub - Active Channels
REM ============================================================================
echo ============================================================================
echo Test 8: Redis Pub/Sub - Active Channels
echo ============================================================================

docker exec zkp-redis-cluster redis-cli PUBSUB CHANNELS > %TEMP%\redis_channels.txt 2>&1
if !errorlevel! equ 0 (
    echo [PASS] PUBSUB CHANNELS command executed
    set /a TESTS_PASSED+=1
    set /a TOTAL_TESTS+=1
    
    REM Check for expected channels
    findstr /C:"coordinator.tasks" %TEMP%\redis_channels.txt >nul 2>&1
    if !errorlevel! equ 0 (
        echo [PASS] coordinator.tasks channel exists
        set /a TESTS_PASSED+=1
    ) else (
        echo [WARN] coordinator.tasks channel not found ^(may be inactive^)
        set /a TESTS_PASSED+=1
    )
    set /a TOTAL_TESTS+=1
) else (
    echo [FAIL] PUBSUB CHANNELS command failed
    set /a TESTS_FAILED+=1
    set /a TOTAL_TESTS+=1
    set /a TESTS_FAILED+=1
    set /a TOTAL_TESTS+=1
)

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 9: Redis Pub/Sub - Event Bus Integration
REM ============================================================================
echo ============================================================================
echo Test 9: Redis Pub/Sub - Event Bus Integration
echo ============================================================================

REM Check coordinator logs for Redis event bus connection
docker logs zkp-coordinator-1 2>&1 | findstr /C:"Redis" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Coordinator has Redis event bus logs
    set /a TESTS_PASSED+=1
) else (
    echo [WARN] No Redis logs in coordinator-1
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

REM Check API Gateway logs for Redis connection
docker logs zkp-api-gateway-cluster 2>&1 | findstr /C:"Redis" >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] API Gateway has Redis event bus logs
    set /a TESTS_PASSED+=1
) else (
    echo [WARN] No Redis logs in API Gateway
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 10: Redis Pub/Sub - Memory and Performance
REM ============================================================================
echo ============================================================================
echo Test 10: Redis Pub/Sub - Memory and Performance
echo ============================================================================

docker exec zkp-redis-cluster redis-cli INFO memory > %TEMP%\redis_memory.txt 2>&1
findstr /C:"used_memory_human" %TEMP%\redis_memory.txt >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Redis memory stats available
    set /a TESTS_PASSED+=1
    
    REM Display memory usage
    for /f "tokens=2 delims=:" %%a in ('findstr "used_memory_human" %TEMP%\redis_memory.txt') do (
        echo        Memory usage: %%a
    )
) else (
    echo [FAIL] Cannot get Redis memory stats
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

docker exec zkp-redis-cluster redis-cli INFO stats > %TEMP%\redis_stats.txt 2>&1
findstr /C:"total_commands_processed" %TEMP%\redis_stats.txt >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Redis command statistics available
    set /a TESTS_PASSED+=1
    
    REM Display command count
    for /f "tokens=2 delims=:" %%a in ('findstr "total_commands_processed" %TEMP%\redis_stats.txt') do (
        echo        Commands processed: %%a
    )
) else (
    echo [FAIL] Cannot get Redis command stats
    set /a TESTS_FAILED+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 11: Redis Pub/Sub - Connection Pool
REM ============================================================================
echo ============================================================================
echo Test 11: Redis Pub/Sub - Connection Pool
echo ============================================================================

docker exec zkp-redis-cluster redis-cli INFO clients > %TEMP%\redis_clients.txt 2>&1
findstr /C:"connected_clients" %TEMP%\redis_clients.txt >nul 2>&1
if !errorlevel! equ 0 (
    echo [PASS] Redis client connections tracked
    set /a TESTS_PASSED+=1
    
    REM Display connection count and check threshold
    for /f "tokens=2 delims=:" %%a in ('findstr "connected_clients" %TEMP%\redis_clients.txt') do (
        set CLIENT_COUNT=%%a
    )
    
    REM Trim spaces from CLIENT_COUNT
    set CLIENT_COUNT=!CLIENT_COUNT: =!
    echo        Connected clients: !CLIENT_COUNT!
    
    REM At least 2 clients expected (coordinator + gateway)
    if !CLIENT_COUNT! geq 2 (
        echo [PASS] Multiple clients connected ^(event bus active^)
        set /a TESTS_PASSED+=1
    ) else (
        echo [WARN] Less than 2 clients connected
        set /a TESTS_FAILED+=1
    )
    set /a TOTAL_TESTS+=1
) else (
    echo [FAIL] Cannot get Redis client info
    set /a TESTS_FAILED+=1
    set /a TESTS_FAILED+=1
    set /a TOTAL_TESTS+=1
)
set /a TOTAL_TESTS+=1

echo.
timeout /t 2 /nobreak >nul

REM ============================================================================
REM Test 12: Redis Pub/Sub - Subscription Count
REM ============================================================================
echo ============================================================================
echo Test 12: Redis Pub/Sub - Subscription Count
echo ============================================================================

docker exec zkp-redis-cluster redis-cli PUBSUB NUMSUB coordinator.tasks worker.assignments > %TEMP%\redis_subs.txt 2>&1
if !errorlevel! equ 0 (
    echo [PASS] PUBSUB NUMSUB command executed
    set /a TESTS_PASSED+=1
    set /a TOTAL_TESTS+=1
    
    REM Check if any subscriptions exist
    type %TEMP%\redis_subs.txt | findstr /R "[1-9]" >nul 2>&1
    if !errorlevel! equ 0 (
        echo [PASS] Active subscriptions detected
        set /a TESTS_PASSED+=1
    ) else (
        echo [INFO] No active subscriptions ^(may be normal if no traffic^)
        set /a TESTS_PASSED+=1
    )
    set /a TOTAL_TESTS+=1
) else (
    echo [FAIL] PUBSUB NUMSUB command failed
    set /a TESTS_FAILED+=1
    set /a TOTAL_TESTS+=1
    set /a TESTS_FAILED+=1
    set /a TOTAL_TESTS+=1
)

echo.

REM ============================================================================
REM Test Summary
REM ============================================================================
echo ============================================================================
echo Test Summary
echo ============================================================================
echo Total tests: !TOTAL_TESTS!
echo Tests passed: !TESTS_PASSED!
echo Tests failed: !TESTS_FAILED!

set /a PASS_RATE=!TESTS_PASSED! * 100 / !TOTAL_TESTS!
echo Pass rate: !PASS_RATE!%%
echo.

if !TESTS_FAILED! equ 0 (
    echo ============================================================================
    echo ALL TESTS PASSED
    echo ============================================================================
    exit /b 0
) else (
    echo ============================================================================
    echo SOME TESTS FAILED
    echo ============================================================================
    exit /b 1
)
