@echo off
REM ============================================================================
REM Redis Statistics (Windows)
REM ============================================================================

echo ======================================
echo Redis Statistics
echo ======================================
echo.

echo [Connection Info]
docker exec zkp-redis-cluster redis-cli INFO clients | findstr "connected_clients"
echo.

echo [Memory Usage]
docker exec zkp-redis-cluster redis-cli INFO memory | findstr "used_memory_human used_memory_peak_human"
echo.

echo [Command Statistics]
docker exec zkp-redis-cluster redis-cli INFO stats | findstr "total_commands_processed total_connections_received keyspace"
echo.

echo [Active Pub/Sub Channels]
docker exec zkp-redis-cluster redis-cli PUBSUB CHANNELS
echo.

echo [Channel Subscriptions]
docker exec zkp-redis-cluster redis-cli PUBSUB NUMSUB zkp:events:task.created
echo.

echo ======================================
