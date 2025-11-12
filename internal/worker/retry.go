// Retry utilities for worker operations
// Consolidates exponential backoff retry logic used across the worker service

package worker

import (
	"context"
	"time"

	"github.com/saiweb3dev/distributed-zkp-network/internal/worker/constants"
	"go.uber.org/zap"
)

// RetryConfig defines retry behavior parameters
type RetryConfig struct {
	MaxRetries  int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration // Optional: cap exponential growth
}

// DefaultRetryConfig returns standard retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  constants.DefaultRetryMaxAttempts,
		BaseBackoff: constants.DefaultRetryBaseBackoff,
		MaxBackoff:  constants.DefaultRetryMaxBackoff,
	}
}

// AggressiveRetryConfig returns configuration for critical operations
func AggressiveRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  constants.AggressiveRetryMaxAttempts,
		BaseBackoff: constants.AggressiveRetryBaseBackoff,
		MaxBackoff:  constants.AggressiveRetryMaxBackoff,
	}
}

// RetryWithBackoff executes a function with exponential backoff retry logic
// Returns the last error if all retries fail, or nil on success
func RetryWithBackoff(
	ctx context.Context,
	cfg RetryConfig,
	logger *zap.Logger,
	operation string,
	fn func(context.Context) error,
) error {
	var lastErr error

	for attempt := 0; attempt < cfg.MaxRetries; attempt++ {
		// Execute the operation
		if err := fn(ctx); err != nil {
			lastErr = err

			// Calculate exponential backoff: 1s, 2s, 4s, 8s, 16s...
			backoff := cfg.BaseBackoff * time.Duration(1<<attempt)
			if cfg.MaxBackoff > 0 && backoff > cfg.MaxBackoff {
				backoff = cfg.MaxBackoff
			}

			// Log retry attempt
			logger.Warn("Operation failed, retrying",
				zap.String("operation", operation),
				zap.Int("attempt", attempt+1),
				zap.Int("max_retries", cfg.MaxRetries),
				zap.Duration("backoff", backoff),
				zap.Error(err),
			)

			// Wait before retrying (unless it's the last attempt)
			if attempt < cfg.MaxRetries-1 {
				select {
				case <-time.After(backoff):
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		} else {
			// Success
			if attempt > 0 {
				logger.Info("Operation succeeded after retry",
					zap.String("operation", operation),
					zap.Int("attempts", attempt+1),
				)
			}
			return nil
		}
	}

	// All retries exhausted
	logger.Error("Operation failed after all retries",
		zap.String("operation", operation),
		zap.Int("max_retries", cfg.MaxRetries),
		zap.Error(lastErr),
	)
	return lastErr
}

// RetryWithContext is a convenience wrapper that creates a timeout context
func RetryWithContext(
	parentCtx context.Context,
	timeout time.Duration,
	cfg RetryConfig,
	logger *zap.Logger,
	operation string,
	fn func(context.Context) error,
) error {
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	return RetryWithBackoff(ctx, cfg, logger, operation, fn)
}
