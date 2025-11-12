@echo off
REM ============================================================================
REM Manual Raft Cluster Bootstrap Script (Windows)
REM ============================================================================
REM This script manually bootstraps the Raft cluster by stopping all nodes,
REM clearing Raft data, and restarting with bootstrap enabled.
REM ============================================================================

echo ======================================
echo Raft Cluster Manual Bootstrap
echo ======================================
echo.

REM Check if docker is running
docker ps >nul 2>&1
if %errorlevel% neq 0 (
    echo Error: Docker is not running or not accessible
    exit /b 1
)

REM Check if coordinator-1 is running
docker ps | findstr zkp-coordinator-1 >nul
if %errorlevel% neq 0 (
    echo Error: coordinator-1 container is not running
    echo Start the cluster first with: docker-compose -f deployments/docker/docker-compose-cluster.yml up -d
    exit /b 1
)

echo Step 1: Checking current Raft state...
echo.

echo Coordinator-1:
curl -s http://localhost:8090/health | findstr "state"

echo Coordinator-2:
curl -s http://localhost:8091/health | findstr "state"

echo Coordinator-3:
curl -s http://localhost:8092/health | findstr "state"

echo.
echo ======================================
echo Step 2: Stopping all coordinators...
echo ======================================
docker stop zkp-coordinator-1 zkp-coordinator-2 zkp-coordinator-3

echo.
echo Step 3: Clearing Raft data directories...
echo.
docker exec zkp-coordinator-1 rm -rf /var/lib/zkp/raft/* 2>nul
docker exec zkp-coordinator-2 rm -rf /var/lib/zkp/raft/* 2>nul
docker exec zkp-coordinator-3 rm -rf /var/lib/zkp/raft/* 2>nul
echo   Raft data cleared

echo.
echo ======================================
echo Step 4: Starting coordinator-1...
echo ======================================
docker start zkp-coordinator-1

echo.
echo Waiting 10 seconds for coordinator-1 to bootstrap...
timeout /t 10 /nobreak >nul

echo.
echo Step 5: Checking if coordinator-1 became leader...
echo.
curl -s http://localhost:8090/health | findstr "is_leader"

echo.
echo ======================================
echo Step 6: Starting remaining coordinators...
echo ======================================
docker start zkp-coordinator-2 zkp-coordinator-3

echo.
echo Waiting 5 seconds for followers to join...
timeout /t 5 /nobreak >nul

echo.
echo ======================================
echo Final Cluster State:
echo ======================================
echo.

echo Coordinator-1 (localhost:8090):
curl -s http://localhost:8090/health | findstr "state.*is_leader"

echo.
echo Coordinator-2 (localhost:8091):
curl -s http://localhost:8091/health | findstr "state.*is_leader"

echo.
echo Coordinator-3 (localhost:8092):
curl -s http://localhost:8092/health | findstr "state.*is_leader"

echo.
echo ======================================
echo Bootstrap complete!
echo ======================================
echo.
echo To verify cluster health:
echo   curl http://localhost:8090/health
echo.
echo To view coordinator logs:
echo   docker logs -f zkp-coordinator-1

pause
