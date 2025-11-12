@echo off
REM ============================================================================
REM Monitor Redis in Real-Time (Windows)
REM ============================================================================

echo ======================================
echo Redis Real-Time Monitor
echo ======================================
echo Press Ctrl+C to stop
echo.

docker exec -it zkp-redis-cluster redis-cli MONITOR
