package handlers

import "time"

const (
	// Lock retry configuration
	MaxLockRetries    = 3
	InitialRetryDelay = 50 * time.Millisecond

	// Task configuration
	DefaultTaskMaxRetries = 3

	// Cache TTLs
	RequestDedupCacheTTL  = 5 * time.Minute
	CompletedTaskCacheTTL = 1 * time.Hour
	PendingTaskCacheTTL   = 5 * time.Second

	// Timeouts
	TaskCreationTimeout = 10 * time.Second
	TaskQueryTimeout    = 5 * time.Second
)
