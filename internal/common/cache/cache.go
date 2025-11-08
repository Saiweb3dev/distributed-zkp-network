// Package cache provides a Redis-based distributed cache layer
// This enables multiple API Gateway instances to share cached results
// Supports task result caching to reduce database load and improve response times
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheLayer provides distributed caching for task results
type CacheLayer struct {
	redis   *redis.Client
	logger  *zap.Logger
	enabled bool
	ttl     time.Duration
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	Host         string
	Port         int
	Password     string
	DB           int
	TTL          time.Duration // Default TTL for cache entries
	MaxRetries   int
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Enabled      bool
}

// CacheEntry represents a cached value with metadata
type CacheEntry struct {
	Value     interface{} `json:"value"`
	CachedAt  int64       `json:"cached_at"`
	ExpiresAt int64       `json:"expires_at"`
}

// NewCacheLayer creates a new cache layer instance
func NewCacheLayer(config CacheConfig, logger *zap.Logger) (*CacheLayer, error) {
	if !config.Enabled {
		logger.Info("Cache layer disabled")
		return &CacheLayer{
			enabled: false,
			logger:  logger,
		}, nil
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	// Set default timeouts
	dialTimeout := config.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}
	readTimeout := config.ReadTimeout
	if readTimeout == 0 {
		readTimeout = 3 * time.Second
	}
	writeTimeout := config.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = 3 * time.Second
	}
	minIdleConns := config.MinIdleConns
	if minIdleConns == 0 {
		minIdleConns = 5
	}
	ttl := config.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute // Default 5 minutes
	}

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     config.Password,
		DB:           config.DB,
		MaxRetries:   config.MaxRetries,
		PoolSize:     config.PoolSize,
		MinIdleConns: minIdleConns,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		logger.Warn("Cache Redis connection failed, caching disabled",
			zap.String("addr", addr),
			zap.Error(err),
		)
		return &CacheLayer{
			enabled: false,
			logger:  logger,
		}, nil
	}

	logger.Info("Cache layer connected to Redis",
		zap.String("addr", addr),
		zap.Int("db", config.DB),
		zap.Duration("default_ttl", ttl),
	)

	return &CacheLayer{
		redis:   client,
		logger:  logger,
		enabled: true,
		ttl:     ttl,
	}, nil
}

// NewDisabledCacheLayer creates a cache layer that is explicitly disabled
func NewDisabledCacheLayer(logger *zap.Logger) *CacheLayer {
	logger.Info("Creating disabled cache layer")
	return &CacheLayer{
		enabled: false,
		logger:  logger,
	}
}

// ============================================================================
// Task Result Caching
// ============================================================================

// GetTaskResult retrieves a cached task result by task ID
func (c *CacheLayer) GetTaskResult(ctx context.Context, taskID string) (interface{}, bool, error) {
	if !c.enabled {
		return nil, false, nil
	}

	key := c.taskResultKey(taskID)

	data, err := c.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		// Cache miss
		c.logger.Debug("Cache miss", zap.String("task_id", taskID))
		return nil, false, nil
	}
	if err != nil {
		c.logger.Warn("Cache read error",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return nil, false, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		c.logger.Error("Failed to unmarshal cache entry",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return nil, false, err
	}

	c.logger.Debug("Cache hit",
		zap.String("task_id", taskID),
		zap.Int64("cached_at", entry.CachedAt),
	)

	return entry.Value, true, nil
}

// SetTaskResult stores a task result in cache
func (c *CacheLayer) SetTaskResult(ctx context.Context, taskID string, value interface{}, ttl time.Duration) error {
	if !c.enabled {
		return nil
	}

	key := c.taskResultKey(taskID)

	// Use default TTL if not specified
	if ttl == 0 {
		ttl = c.ttl
	}

	now := time.Now().Unix()
	entry := CacheEntry{
		Value:     value,
		CachedAt:  now,
		ExpiresAt: now + int64(ttl.Seconds()),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	if err := c.redis.Set(ctx, key, data, ttl).Err(); err != nil {
		c.logger.Warn("Failed to cache task result",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return err
	}

	c.logger.Debug("Task result cached",
		zap.String("task_id", taskID),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// InvalidateTaskResult removes a task result from cache
func (c *CacheLayer) InvalidateTaskResult(ctx context.Context, taskID string) error {
	if !c.enabled {
		return nil
	}

	key := c.taskResultKey(taskID)

	if err := c.redis.Del(ctx, key).Err(); err != nil {
		c.logger.Warn("Failed to invalidate cache",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		return err
	}

	c.logger.Debug("Task result invalidated", zap.String("task_id", taskID))
	return nil
}

// ============================================================================
// Request-Based Caching (for idempotent operations)
// ============================================================================

// GenerateRequestKey creates a deterministic cache key from request data
// Uses SHA-256 hash of normalized JSON to ensure same requests get same key
func (c *CacheLayer) GenerateRequestKey(prefix string, data interface{}) (string, error) {
	// Serialize data to JSON (ensures consistent ordering)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request data: %w", err)
	}

	// Hash the JSON to create deterministic key
	hash := sha256.Sum256(jsonData)
	hashStr := hex.EncodeToString(hash[:])

	// Format: zkp:cache:{prefix}:{hash}
	key := fmt.Sprintf("zkp:cache:%s:%s", prefix, hashStr)

	return key, nil
}

// GetByRequestHash retrieves cached value by request hash
func (c *CacheLayer) GetByRequestHash(ctx context.Context, prefix string, requestData interface{}) (interface{}, bool, error) {
	if !c.enabled {
		return nil, false, nil
	}

	key, err := c.GenerateRequestKey(prefix, requestData)
	if err != nil {
		return nil, false, err
	}

	data, err := c.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		c.logger.Debug("Cache miss (request hash)", zap.String("key", key))
		return nil, false, nil
	}
	if err != nil {
		c.logger.Warn("Cache read error", zap.Error(err))
		return nil, false, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false, err
	}

	c.logger.Debug("Cache hit (request hash)",
		zap.String("key", key),
		zap.Int64("cached_at", entry.CachedAt),
	)

	return entry.Value, true, nil
}

// SetByRequestHash stores value by request hash
func (c *CacheLayer) SetByRequestHash(ctx context.Context, prefix string, requestData interface{}, value interface{}, ttl time.Duration) error {
	if !c.enabled {
		return nil
	}

	key, err := c.GenerateRequestKey(prefix, requestData)
	if err != nil {
		return err
	}

	if ttl == 0 {
		ttl = c.ttl
	}

	now := time.Now().Unix()
	entry := CacheEntry{
		Value:     value,
		CachedAt:  now,
		ExpiresAt: now + int64(ttl.Seconds()),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	if err := c.redis.Set(ctx, key, data, ttl).Err(); err != nil {
		c.logger.Warn("Failed to cache by request hash", zap.Error(err))
		return err
	}

	c.logger.Debug("Value cached by request hash",
		zap.String("key", key),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// ============================================================================
// Distributed Lock (for preventing duplicate task submissions)
// ============================================================================

// AcquireLock tries to acquire a distributed lock
// Returns true if lock acquired, false if already held
func (c *CacheLayer) AcquireLock(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	if !c.enabled {
		return true, nil // Always succeed if cache disabled
	}

	key := fmt.Sprintf("zkp:lock:%s", lockKey)

	// Try to set key only if it doesn't exist (NX flag)
	success, err := c.redis.SetNX(ctx, key, time.Now().Unix(), ttl).Result()
	if err != nil {
		c.logger.Warn("Failed to acquire lock",
			zap.String("lock_key", lockKey),
			zap.Error(err),
		)
		return false, err
	}

	if success {
		c.logger.Debug("Lock acquired",
			zap.String("lock_key", lockKey),
			zap.Duration("ttl", ttl),
		)
	} else {
		c.logger.Debug("Lock already held", zap.String("lock_key", lockKey))
	}

	return success, nil
}

// ReleaseLock releases a distributed lock
func (c *CacheLayer) ReleaseLock(ctx context.Context, lockKey string) error {
	if !c.enabled {
		return nil
	}

	key := fmt.Sprintf("zkp:lock:%s", lockKey)

	if err := c.redis.Del(ctx, key).Err(); err != nil {
		c.logger.Warn("Failed to release lock",
			zap.String("lock_key", lockKey),
			zap.Error(err),
		)
		return err
	}

	c.logger.Debug("Lock released", zap.String("lock_key", lockKey))
	return nil
}

// ============================================================================
// Utility Methods
// ============================================================================

// taskResultKey generates a Redis key for task results
func (c *CacheLayer) taskResultKey(taskID string) string {
	return fmt.Sprintf("zkp:task:result:%s", taskID)
}

// IsEnabled returns whether caching is operational
func (c *CacheLayer) IsEnabled() bool {
	return c.enabled
}

// HealthCheck verifies Redis connection is alive
func (c *CacheLayer) HealthCheck(ctx context.Context) error {
	if !c.enabled {
		return fmt.Errorf("cache disabled")
	}

	return c.redis.Ping(ctx).Err()
}

// GetStats returns cache statistics
func (c *CacheLayer) GetStats(ctx context.Context) (map[string]interface{}, error) {
	if !c.enabled {
		return map[string]interface{}{
			"enabled": false,
		}, nil
	}

	info, err := c.redis.Info(ctx, "stats").Result()
	if err != nil {
		return nil, err
	}

	keyspace, err := c.redis.Info(ctx, "keyspace").Result()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"enabled":  true,
		"stats":    info,
		"keyspace": keyspace,
	}, nil
}

// Close gracefully shuts down the cache layer
func (c *CacheLayer) Close() error {
	if !c.enabled || c.redis == nil {
		return nil
	}

	c.logger.Info("Closing cache layer")
	return c.redis.Close()
}

// ============================================================================
// Batch Operations (for efficiency)
// ============================================================================

// GetMany retrieves multiple cache entries in a single operation
func (c *CacheLayer) GetMany(ctx context.Context, keys []string) (map[string]interface{}, error) {
	if !c.enabled || len(keys) == 0 {
		return make(map[string]interface{}), nil
	}

	// Use pipeline for efficient batch retrieval
	pipe := c.redis.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))

	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	results := make(map[string]interface{})
	for i, cmd := range cmds {
		if data, err := cmd.Bytes(); err == nil {
			var entry CacheEntry
			if err := json.Unmarshal(data, &entry); err == nil {
				results[keys[i]] = entry.Value
			}
		}
	}

	return results, nil
}

// SetMany stores multiple cache entries in a single operation
func (c *CacheLayer) SetMany(ctx context.Context, entries map[string]interface{}, ttl time.Duration) error {
	if !c.enabled || len(entries) == 0 {
		return nil
	}

	if ttl == 0 {
		ttl = c.ttl
	}

	now := time.Now().Unix()
	pipe := c.redis.Pipeline()

	for key, value := range entries {
		entry := CacheEntry{
			Value:     value,
			CachedAt:  now,
			ExpiresAt: now + int64(ttl.Seconds()),
		}

		data, err := json.Marshal(entry)
		if err != nil {
			continue
		}

		pipe.Set(ctx, key, data, ttl)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// InvalidatePattern removes all keys matching a pattern
// Use with caution in production - can be slow with many keys
func (c *CacheLayer) InvalidatePattern(ctx context.Context, pattern string) error {
	if !c.enabled {
		return nil
	}

	iter := c.redis.Scan(ctx, 0, pattern, 100).Iterator()
	pipe := c.redis.Pipeline()
	count := 0

	for iter.Next(ctx) {
		pipe.Del(ctx, iter.Val())
		count++

		// Execute in batches of 100
		if count%100 == 0 {
			_, err := pipe.Exec(ctx)
			if err != nil {
				return err
			}
			pipe = c.redis.Pipeline()
		}
	}

	if err := iter.Err(); err != nil {
		return err
	}

	// Execute remaining
	if count%100 != 0 {
		_, err := pipe.Exec(ctx)
		if err != nil {
			return err
		}
	}

	c.logger.Info("Invalidated cache keys by pattern",
		zap.String("pattern", pattern),
		zap.Int("count", count),
	)

	return nil
}
