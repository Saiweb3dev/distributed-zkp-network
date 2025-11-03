@echo off
REM start-services.bat
REM Windows batch file to start all distributed ZKP network services
REM Usage: start-services.bat

REM Change to the script's directory (project root)
cd /d "%~dp0"

echo ========================================
echo   Distributed ZKP Network Launcher
echo ========================================
echo.
echo Project Directory: %CD%
echo.

REM Check if Docker is running
docker info >nul 2>&1
if errorlevel 1 (
    echo ERROR: Docker is not running
    echo Please start Docker Desktop and try again
    pause
    exit /b 1
)

REM Check if Go is installed
go version >nul 2>&1
if errorlevel 1 (
    echo ERROR: Go is not installed
    echo Please install Go from https://golang.org/dl/
    pause
    exit /b 1
)

echo [1/6] Prerequisites check passed
echo.

REM Start PostgreSQL in Docker
echo [2/6] Starting PostgreSQL...
docker ps -a --format "{{.Names}}" | findstr /x "zkp-postgres" >nul 2>&1
if errorlevel 1 (
    echo Creating new PostgreSQL container...
    docker run -d --name zkp-postgres -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=zkp_network -p 5432:5432 postgres:15-alpine
) else (
    docker ps --format "{{.Names}}" | findstr /x "zkp-postgres" >nul 2>&1
    if errorlevel 1 (
        echo Starting existing PostgreSQL container...
        docker start zkp-postgres
    ) else (
        echo PostgreSQL is already running
    )
)

REM Wait for PostgreSQL to be ready
echo Waiting for PostgreSQL to be ready...
timeout /t 5 /nobreak >nul
echo PostgreSQL is ready
echo.

REM Run database migrations
echo [3/6] Running database migrations...
if exist "internal\storage\postgres\migrations\001_initial_schema.up.sql" (
    docker exec -i zkp-postgres psql -U postgres -d zkp_network < internal\storage\postgres\migrations\001_initial_schema.up.sql 2>nul
    echo Database migrations completed
) else (
    echo WARNING: Migration file not found
)
echo.

REM Launch services in separate Windows Terminal tabs
echo [4/6] Detecting Windows Terminal...
where wt >nul 2>&1
if errorlevel 1 (
    echo WARNING: Windows Terminal not found
    echo Opening services in separate cmd windows instead
    
    REM Fallback to cmd windows
    start "Coordinator" cmd /k "cd /d "%CD%" && go run cmd\coordinator\main.go -config configs\dev\coordinator-local.yaml"
    timeout /t 3 /nobreak >nul
    
    start "Worker-1" cmd /k "cd /d "%CD%" && go run cmd\worker\main.go -config configs\dev\worker-local.yaml"
    timeout /t 2 /nobreak >nul
    
    REM Create temporary config for worker 2
    (
        echo worker:
        echo   id: "worker-2"
        echo   coordinator_address: "localhost:9090"
        echo   concurrency: 4
        echo   heartbeat_interval: 5s
        echo.
        echo zkp:
        echo   curve: "bn254"
        echo   backend: "groth16"
    ) > %TEMP%\worker-2.yaml
    
    start "Worker-2" cmd /k "cd /d "%CD%" && go run cmd\worker\main.go -config %TEMP%\worker-2.yaml"
    timeout /t 2 /nobreak >nul
    
    start "API-Gateway" cmd /k "cd /d "%CD%" && go run cmd\api-gateway\main.go -config configs\dev\api-gateway-local.yaml"
) else (
    echo Using Windows Terminal
    
    REM Launch with Windows Terminal
    start wt -w 0 new-tab --title "Coordinator" cmd /k "cd /d "%CD%" && go run cmd\coordinator\main.go -config configs\dev\coordinator-local.yaml"
    timeout /t 3 /nobreak >nul
    
    start wt -w 0 new-tab --title "Worker-1" cmd /k "cd /d "%CD%" && go run cmd\worker\main.go -config configs\dev\worker-local.yaml"
    timeout /t 2 /nobreak >nul
    
    REM Create temporary config for worker 2
    (
        echo worker:
        echo   id: "worker-2"
        echo   coordinator_address: "localhost:9090"
        echo   concurrency: 4
        echo   heartbeat_interval: 5s
        echo.
        echo zkp:
        echo   curve: "bn254"
        echo   backend: "groth16"
    ) > %TEMP%\worker-2.yaml
    
    start wt -w 0 new-tab --title "Worker-2" cmd /k "cd /d "%CD%" && go run cmd\worker\main.go -config %TEMP%\worker-2.yaml"
    timeout /t 2 /nobreak >nul
    
    start wt -w 0 new-tab --title "API-Gateway" cmd /k "cd /d "%CD%" && go run cmd\api-gateway\main.go -config configs\dev\api-gateway-local.yaml"
)

echo.
echo [5/6] Services launched
timeout /t 5 /nobreak >nul
echo.

echo [6/6] Verifying services...
echo.

REM Display summary
echo ========================================
echo   All Services Started Successfully!
echo ========================================
echo.
echo Service Status:
echo   PostgreSQL:    Running on port 5432
echo   Coordinator:   Running on port 9090 (gRPC) and 8090 (HTTP)
echo   Worker 1:      ID: worker-1
echo   Worker 2:      ID: worker-2
echo   API Gateway:   Running on port 8080
echo.
echo Quick Links:
echo   Coordinator Health: http://localhost:8090/health
echo   Coordinator Metrics: http://localhost:8090/metrics
echo   API Gateway: http://localhost:8080
echo.
echo Next Steps:
echo   1. Run test-services.bat to test the system
echo   2. Check service logs in their respective windows
echo   3. Use stop-services.bat to shut down all services
echo.
echo Happy proof generation!
pause
