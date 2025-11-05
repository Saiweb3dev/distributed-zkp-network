package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type EventType string

const (
	EventTaskCreated   EventType = "task.created"
	EventTaskCompleted EventType = "task.completed"
	EventTaskFailed    EventType = "task.failed"
	EventWorkerJoined  EventType = "worker.joined"
	EventWorkerLeft    EventType = "worker.left"
)

// Event represents a system event that can be published/subscribed
type Event struct {
	Type      EventType              `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// EventBus handles pub/sub messaging for event-driven architecture
// It uses Redis as the message broker, enabling communication across
// distributed services (API Gateway, Coordinators, Workers)
type EventBus struct {
	redis   *redis.Client
	pubsub  *redis.PubSub
	logger  *zap.Logger
	enabled bool

	// For graceful shutdown
	mu       sync.RWMutex
	channels map[string]chan Event
	ctx      context.Context
	cancel   context.CancelFunc
}

// EventBusConfig holds configuration for the event bus
type EventBusConfig struct {
	Host         string
	Port         int
	Password     string
	DB           int
	MaxRetries   int
	PoolSize     int
	MinIdleConns int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Enabled      bool
}

// NewEventBus creates a new event bus instance
// If Redis connection fails, it returns an error but the application can
// still function using polling mode as fallback
func NewEventBus(config EventBusConfig, logger *zap.Logger) (*EventBus, error) {
	if !config.Enabled {
		logger.Info("Event bus disabled, using polling mode only")
		return &EventBus{
			enabled:  false,
			logger:   logger,
			channels: make(map[string]chan Event),
		}, nil
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	// Set default timeouts if not specified
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
		logger.Warn("Redis connection failed, falling back to polling mode",
			zap.String("addr", addr),
			zap.Error(err),
		)
		return &EventBus{
			enabled:  false,
			logger:   logger,
			channels: make(map[string]chan Event),
		}, nil
	}

	logger.Info("Event bus connected to Redis",
		zap.String("addr", addr),
		zap.Int("db", config.DB),
	)

	ctx, cancel = context.WithCancel(context.Background())

	return &EventBus{
		redis:    client,
		logger:   logger,
		enabled:  true,
		channels: make(map[string]chan Event),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// NewDisabledEventBus creates an event bus that is explicitly disabled
// Used for graceful degradation when Redis is not available or disabled in config
func NewDisabledEventBus(logger *zap.Logger) *EventBus {
	logger.Info("Creating disabled event bus, system will use polling mode only")
	return &EventBus{
		enabled:  false,
		logger:   logger,
		channels: make(map[string]chan Event),
	}
}

// Publish sends an event to all subscribers
// If Redis is unavailable, it logs a warning but doesn't fail
// This allows graceful degradation to polling mode
func (eb *EventBus) Publish(ctx context.Context, event Event) error {
	if !eb.enabled {
		// Event bus disabled, skip publishing
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	channel := fmt.Sprintf("zkp:events:%s", event.Type)

	if err := eb.redis.Publish(ctx, channel, data).Err(); err != nil {
		eb.logger.Warn("Failed to publish event to Redis",
			zap.String("event_type", string(event.Type)),
			zap.Error(err),
		)
		// Don't return error - allow application to continue
		return nil
	}

	eb.logger.Debug("Event published",
		zap.String("type", string(event.Type)),
		zap.String("channel", channel),
	)

	return nil
}

// Subscribe listens for events of specific types
// Returns a channel that receives events. The channel is buffered
// to prevent slow consumers from blocking publishers
func (eb *EventBus) Subscribe(ctx context.Context, eventTypes ...EventType) (<-chan Event, error) {
	if !eb.enabled {
		// Return a channel that never receives anything
		// This allows code to work with event bus disabled
		ch := make(chan Event)
		go func() {
			<-ctx.Done()
			close(ch)
		}()
		return ch, nil
	}

	// Build channel names
	channels := make([]string, len(eventTypes))
	for i, et := range eventTypes {
		channels[i] = fmt.Sprintf("zkp:events:%s", et)
	}

	// Subscribe to Redis channels
	pubsub := eb.redis.Subscribe(ctx, channels...)

	// Wait for confirmation
	if _, err := pubsub.Receive(ctx); err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	eb.logger.Info("Subscribed to events",
		zap.Strings("channels", channels),
	)

	// Create event channel
	eventChan := make(chan Event, 100) // Buffered to handle bursts

	// Store reference for cleanup
	eb.mu.Lock()
	channelKey := fmt.Sprintf("sub_%d", time.Now().UnixNano())
	eb.channels[channelKey] = eventChan
	eb.mu.Unlock()

	// Start receiving messages
	go eb.receiveLoop(ctx, pubsub, eventChan, channelKey)

	return eventChan, nil
}

// receiveLoop handles incoming messages from Redis
func (eb *EventBus) receiveLoop(ctx context.Context, pubsub *redis.PubSub, eventChan chan Event, channelKey string) {
	defer func() {
		close(eventChan)
		pubsub.Close()

		eb.mu.Lock()
		delete(eb.channels, channelKey)
		eb.mu.Unlock()

		eb.logger.Debug("Subscription closed", zap.String("key", channelKey))
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-eb.ctx.Done():
			return

		default:
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					// Context cancelled, normal shutdown
					return
				}

				eb.logger.Error("Redis receive error", zap.Error(err))

				// Implement exponential backoff on errors
				time.Sleep(time.Second)
				continue
			}

			var event Event
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				eb.logger.Error("Failed to unmarshal event",
					zap.Error(err),
					zap.String("payload", msg.Payload),
				)
				continue
			}

			// Send to channel (non-blocking)
			select {
			case eventChan <- event:
				eb.logger.Debug("Event delivered",
					zap.String("type", string(event.Type)),
				)
			case <-ctx.Done():
				return
			default:
				// Channel full, log warning but don't block
				eb.logger.Warn("Event channel full, dropping event",
					zap.String("type", string(event.Type)),
				)
			}
		}
	}
}

// Close gracefully shuts down the event bus
func (eb *EventBus) Close() error {
	if !eb.enabled {
		return nil
	}

	eb.logger.Info("Closing event bus")

	eb.cancel()

	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Close all subscriber channels
	for key, ch := range eb.channels {
		close(ch)
		delete(eb.channels, key)
	}

	if eb.redis != nil {
		return eb.redis.Close()
	}

	return nil
}

// IsEnabled returns whether the event bus is operational
func (eb *EventBus) IsEnabled() bool {
	return eb.enabled
}

// HealthCheck verifies Redis connection is alive
func (eb *EventBus) HealthCheck(ctx context.Context) error {
	if !eb.enabled {
		return fmt.Errorf("event bus disabled")
	}

	return eb.redis.Ping(ctx).Err()
}
