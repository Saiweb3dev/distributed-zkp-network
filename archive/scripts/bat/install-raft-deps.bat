@echo off
REM Phase 03: Raft Integration - Dependency Installation Script (Windows)
REM This script installs required Go dependencies for Raft consensus

setlocal enabledelayedexpansion

echo ========================================
echo   Raft Integration - Installing Deps
echo ========================================
echo.

REM Check if Go is installed
where go >nul 2>nul
if !ERRORLEVEL! NEQ 0 (
    echo Error: Go is not installed
    echo Please install Go 1.21+ from https://golang.org/dl/
    exit /b 1
)

for /f "tokens=3" %%G in ('go version') do set GO_VERSION=%%G
echo Go version: !GO_VERSION!
echo.

REM Navigate to project root
cd /d "%~dp0.."
echo Project directory: %CD%
echo.

REM Install Raft dependencies
echo Installing Raft dependencies...
echo.

echo   - Installing hashicorp/raft@v1.5.0
go get github.com/hashicorp/raft@v1.5.0

echo   - Installing hashicorp/raft-boltdb@v2.3.0
go get github.com/hashicorp/raft-boltdb@v2.3.0

echo   - Installing boltdb/bolt (dependency)
go get github.com/boltdb/bolt@v1.3.1

echo.
echo Tidying up go.mod...
go mod tidy

echo.
echo Dependencies installed successfully!
echo.

REM Verify installation
echo Verifying installation...
findstr /C:"github.com/hashicorp/raft" go.mod >nul
if !ERRORLEVEL! EQU 0 (
    echo   hashicorp/raft found in go.mod
) else (
    echo   hashicorp/raft NOT found in go.mod
    exit /b 1
)

findstr /C:"github.com/hashicorp/raft-boltdb" go.mod >nul
if !ERRORLEVEL! EQU 0 (
    echo   hashicorp/raft-boltdb found in go.mod
) else (
    echo   hashicorp/raft-boltdb NOT found in go.mod
    exit /b 1
)

echo.
echo ========================================
echo   Installation Complete!
echo ========================================
echo.
echo Next steps:
echo   1. Build the coordinator:
echo      go build -o bin\coordinator.exe cmd\coordinator\main.go
echo.
echo   2. Test single node:
echo      go run cmd\coordinator\main.go --config configs\coordinator-local.yaml
echo.
echo   3. Test cluster:
echo      docker-compose -f deployments\docker\docker-compose-cluster.yml up
echo.
echo   4. Read documentation:
echo      docs\RAFT_INTEGRATION_GUIDE.md
echo      docs\RAFT_QUICKSTART.md
echo.

pause
