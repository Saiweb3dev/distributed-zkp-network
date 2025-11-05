@echo off
REM ============================================================================
REM Clean Docker Cache and Images (Windows)
REM ============================================================================

echo ======================================
echo Docker Cleanup
echo ======================================

echo.
echo [1/5] Stopping all containers...
docker-compose -f deployments\docker\docker-compose-cluster.yml down -v 2>nul
echo [OK] Containers stopped

echo.
echo [2/5] Removing ZKP images...
docker rmi zkp-api-gateway-cluster 2>nul
docker rmi zkp-coordinator-cluster 2>nul
docker rmi zkp-worker-cluster 2>nul
docker rmi distributed-zkp-network-api-gateway 2>nul
docker rmi distributed-zkp-network-coordinator 2>nul
docker rmi distributed-zkp-network-worker 2>nul
echo [OK] ZKP images removed

echo.
echo [3/5] Pruning build cache...
docker builder prune -f
echo [OK] Build cache pruned

echo.
echo [4/5] Pruning system (dangling images)...
docker system prune -f
echo [OK] System pruned

echo.
echo [5/5] Cleaning volumes (optional)...
set /p CLEAN_VOLUMES="Clean volumes (all data will be lost)? [y/N]: "
if /i "%CLEAN_VOLUMES%"=="y" (
    docker volume rm distributed-zkp-network_redis_data 2>nul
    docker volume rm distributed-zkp-network_postgres_cluster_data 2>nul
    docker volume rm distributed-zkp-network_raft_data_1 2>nul
    docker volume rm distributed-zkp-network_raft_data_2 2>nul
    docker volume rm distributed-zkp-network_raft_data_3 2>nul
    echo [OK] Volumes cleaned
) else (
    echo [SKIP] Volumes preserved
)

echo.
echo ======================================
echo Cleanup Complete!
echo ======================================
echo.
echo You can now run: start-cluster.bat
echo.

pause
