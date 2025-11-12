// Package health provides health check utilities for service dependencies
package health

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/common/cache"
	"github.com/saiweb3dev/distributed-zkp-network/internal/common/events"
	"go.uber.org/zap"
)

// Checker performs health checks on system dependencies
type Checker struct {
	logger *zap.Logger
}

// NewChecker creates a new health checker
func NewChecker(logger *zap.Logger) *Checker {
	return &Checker{logger: logger}
}

// CheckResult represents the result of a health check
type CheckResult struct {
	Component string        `json:"component"`
	Healthy   bool          `json:"healthy"`
	Message   string        `json:"message,omitempty"`
	Duration  time.Duration `json:"duration_ms"`
}

// SystemHealth represents overall system health
type SystemHealth struct {
	Healthy bool          `json:"healthy"`
	Checks  []CheckResult `json:"checks"`
}

// CheckAll performs health checks on all critical dependencies
func (h *Checker) CheckAll(ctx context.Context, db *sql.DB, eventBus *events.EventBus, cacheLayer *cache.CacheLayer) *SystemHealth {
	results := make([]CheckResult, 0, 3)
	allHealthy := true

	// Check database
	dbResult := h.CheckDatabase(ctx, db)
	results = append(results, dbResult)
	if !dbResult.Healthy {
		allHealthy = false
	}

	// Check event bus (if enabled)
	if eventBus != nil && eventBus.IsEnabled() {
		eventResult := h.CheckEventBus(ctx, eventBus)
		results = append(results, eventResult)
		if !eventResult.Healthy {
			h.logger.Warn("Event bus unhealthy, but continuing (non-critical)",
				zap.String("message", eventResult.Message),
			)
			// Event bus failure is not critical - we can fall back to polling
		}
	}

	// Check cache layer (if enabled)
	if cacheLayer != nil && cacheLayer.IsEnabled() {
		cacheResult := h.CheckCache(ctx, cacheLayer)
		results = append(results, cacheResult)
		if !cacheResult.Healthy {
			h.logger.Warn("Cache unhealthy, but continuing (non-critical)",
				zap.String("message", cacheResult.Message),
			)
			// Cache failure is not critical - we can operate without it
		}
	}

	return &SystemHealth{
		Healthy: allHealthy,
		Checks:  results,
	}
}

// CheckDatabase verifies PostgreSQL connectivity and basic operations
func (h *Checker) CheckDatabase(ctx context.Context, db *sql.DB) CheckResult {
	start := time.Now()
	result := CheckResult{
		Component: "database",
		Healthy:   false,
	}

	// Create context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Test basic connectivity
	if err := db.PingContext(checkCtx); err != nil {
		result.Message = fmt.Sprintf("ping failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// Test query execution
	var version string
	err := db.QueryRowContext(checkCtx, "SELECT version()").Scan(&version)
	if err != nil {
		result.Message = fmt.Sprintf("query failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// Check connection pool stats
	stats := db.Stats()
	if stats.OpenConnections >= stats.MaxOpenConnections {
		h.logger.Warn("Database connection pool exhausted",
			zap.Int("open", stats.OpenConnections),
			zap.Int("max", stats.MaxOpenConnections),
		)
	}

	result.Healthy = true
	result.Message = "ok"
	result.Duration = time.Since(start)
	return result
}

// CheckEventBus verifies Redis event bus connectivity
func (h *Checker) CheckEventBus(ctx context.Context, eventBus *events.EventBus) CheckResult {
	start := time.Now()
	result := CheckResult{
		Component: "event_bus",
		Healthy:   false,
	}

	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := eventBus.HealthCheck(checkCtx); err != nil {
		result.Message = fmt.Sprintf("health check failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	result.Healthy = true
	result.Message = "ok"
	result.Duration = time.Since(start)
	return result
}

// CheckCache verifies Redis cache layer connectivity
func (h *Checker) CheckCache(ctx context.Context, cacheLayer *cache.CacheLayer) CheckResult {
	start := time.Now()
	result := CheckResult{
		Component: "cache",
		Healthy:   false,
	}

	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := cacheLayer.HealthCheck(checkCtx); err != nil {
		result.Message = fmt.Sprintf("health check failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	result.Healthy = true
	result.Message = "ok"
	result.Duration = time.Since(start)
	return result
}

// WaitForHealthy blocks until all critical dependencies are healthy or timeout
func (h *Checker) WaitForHealthy(ctx context.Context, db *sql.DB, eventBus *events.EventBus, cacheLayer *cache.CacheLayer, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		h.logger.Info("Checking system health", zap.Int("attempt", attempt))

		health := h.CheckAll(ctx, db, eventBus, cacheLayer)

		if health.Healthy {
			h.logger.Info("All critical dependencies healthy")
			return nil
		}

		// Log unhealthy components
		for _, check := range health.Checks {
			if !check.Healthy {
				h.logger.Warn("Dependency unhealthy",
					zap.String("component", check.Component),
					zap.String("message", check.Message),
				)
			}
		}

		// Wait before retry (exponential backoff, max 5s)
		waitTime := time.Duration(attempt) * time.Second
		if waitTime > 5*time.Second {
			waitTime = 5 * time.Second
		}

		h.logger.Info("Retrying health check", zap.Duration("wait", waitTime))
		time.Sleep(waitTime)
	}

	return fmt.Errorf("health check timeout after %v", maxWait)
}
