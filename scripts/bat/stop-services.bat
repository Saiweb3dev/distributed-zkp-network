@echo off
REM stop-services.bat
REM Windows batch file to stop all distributed ZKP network services
REM Usage: stop-services.bat

echo ========================================
echo   Stopping ZKP Network Services
echo ========================================
echo.

echo [1/5] Stopping Go processes...

REM Kill all Go processes running our services
for /f "tokens=2" %%a in ('tasklist /FI "IMAGENAME eq go.exe" /NH 2^>nul') do (
    echo Stopping process %%a
    taskkill /PID %%a /F >nul 2>&1
)

echo Go processes stopped
echo.

echo [2/5] Stopping PostgreSQL...
docker ps --format "{{.Names}}" | findstr /x "zkp-postgres" >nul 2>&1
if not errorlevel 1 (
    echo Stopping PostgreSQL container...
    docker stop zkp-postgres
    echo PostgreSQL stopped
) else (
    echo PostgreSQL container is not running
)
echo.

echo [3/5] Clean up temporary files...
if exist "%TEMP%\worker-2.yaml" (
    del /F /Q "%TEMP%\worker-2.yaml"
    echo Temporary config files removed
)
echo.

echo [4/5] Remove PostgreSQL container?
set /p REMOVE="Remove PostgreSQL container and data? (y/N): "
if /i "%REMOVE%"=="y" (
    docker rm zkp-postgres 2>nul
    echo PostgreSQL container removed
)
echo.

echo [5/5] Cleanup complete
echo.

echo ========================================
echo   All Services Stopped
echo ========================================
echo.
echo To restart services, run: start-services.bat
pause
