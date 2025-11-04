@echo off
REM ============================================================================
REM Start 3-Node Raft Coordinator Cluster (Windows)
REM ============================================================================

echo ======================================
echo ZKP Network Cluster Startup
echo ======================================

REM Step 1: Check dependencies
echo.
echo [1/5] Checking dependencies...
where docker >nul 2>&1
if %errorlevel% neq 0 (
    echo Error: docker is not installed
    exit /b 1
)

where docker-compose >nul 2>&1
if %errorlevel% neq 0 (
    echo Error: docker-compose is not installed
    exit /b 1
)

echo [OK] Docker and docker-compose are installed

REM Step 2: Install Go dependencies
echo.
echo [2/5] Installing Go dependencies...
cd /d "%~dp0..\.."
go get github.com/hashicorp/raft@v1.5.0
go get github.com/hashicorp/raft-boltdb@v2.3.0
go mod tidy
echo [OK] Dependencies installed

REM Step 3: Stop existing containers
echo.
echo [3/5] Stopping existing containers...
docker-compose -f deployments/docker/docker-compose-cluster.yml down -v 2>nul
echo [OK] Existing containers stopped

REM Step 4: Build images
echo.
echo [4/5] Building Docker images...
docker-compose -f deployments/docker/docker-compose-cluster.yml build
if %errorlevel% neq 0 (
    echo Error: Failed to build images
    exit /b 1
)
echo [OK] Images built successfully

REM Step 5: Start cluster
echo.
echo [5/5] Starting cluster...
docker-compose -f deployments/docker/docker-compose-cluster.yml up -d
if %errorlevel% neq 0 (
    echo Error: Failed to start cluster
    exit /b 1
)
echo [OK] Cluster started

REM Wait for services to be ready
echo.
echo Waiting for services to start...
timeout /t 10 /nobreak >nul

REM Check health
echo.
echo ======================================
echo Cluster Status:
echo ======================================
echo.

echo Coordinator-1 (localhost:8090):
curl -s http://localhost:8090/health
echo.

echo Coordinator-2 (localhost:8091):
curl -s http://localhost:8091/health
echo.

echo Coordinator-3 (localhost:8092):
curl -s http://localhost:8092/health
echo.

echo ======================================
echo Cluster Started Successfully!
echo ======================================
echo.

echo Services:
echo   - API Gateway: http://localhost:8080
echo   - Coordinator-1: http://localhost:8090 (gRPC: 9090, Raft: 7000)
echo   - Coordinator-2: http://localhost:8091 (gRPC: 9091, Raft: 7001)
echo   - Coordinator-3: http://localhost:8092 (gRPC: 9092, Raft: 7002)
echo   - PostgreSQL: localhost:5432
echo.
echo Commands:
echo   - View logs: docker-compose -f deployments/docker/docker-compose-cluster.yml logs -f
echo   - Stop cluster: docker-compose -f deployments/docker/docker-compose-cluster.yml down
echo   - Check leader: curl http://localhost:8090/health
echo.
echo Next: See docs/RAFT_QUICKSTART.md for testing instructions

pause
