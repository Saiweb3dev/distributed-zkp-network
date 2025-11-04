package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration (API Gateway)
// We use a single struct to make it easy to pass around and test
// Each section maps to a YAML block in the config file
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	ZKP       ZKPConfig       `mapstructure:"zkp"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	CORS      CORSConfig      `mapstructure:"cors"`
}

// WorkerConfig holds worker node configuration
type WorkerConfig struct {
	Worker  WorkerNodeConfig `mapstructure:"worker"`
	Logging LoggingConfig    `mapstructure:"logging"`
	Metrics MetricsConfig    `mapstructure:"metrics"`
	ZKP     ZKPConfig        `mapstructure:"zkp"`
}

// CoordinatorConfig holds coordinator node configuration
type CoordinatorConfig struct {
	Coordinator CoordinatorNodeConfig `mapstructure:"coordinator"`
	Database    DatabaseConfig        `mapstructure:"database"`
	Logging     LoggingConfig         `mapstructure:"logging"`
	Metrics     MetricsConfig         `mapstructure:"metrics"`
}

// ServerConfig defines HTTP server settings
// Timeouts are CRITICAL in production - without them, slow client can exhaust resources
type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// DatabaseConfig defines PostgreSQL connection settings
// Connection pooling is critical for distributed systems
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Database        string        `mapstructure:"database"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	SSLMode         string        `mapstructure:"ssl_mode"`          // disable, require, verify-ca, verify-full
	MaxOpenConns    int           `mapstructure:"max_open_conns"`    // Max connections in pool
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`    // Idle connections to keep
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"` // Max connection lifetime
}

// WorkerNodeConfig defines worker node settings
type WorkerNodeConfig struct {
	ID                 string        `mapstructure:"id"`                  // Unique worker ID
	CoordinatorAddress string        `mapstructure:"coordinator_address"` // Coordinator gRPC address
	Concurrency        int           `mapstructure:"concurrency"`         // Max concurrent tasks
	HeartbeatInterval  time.Duration `mapstructure:"heartbeat_interval"`  // How often to send heartbeat
	RegisterRetries    int           `mapstructure:"register_retries"`    // Retries for registration
	TaskTimeout        time.Duration `mapstructure:"task_timeout"`        // Max time for a single task
}

// CoordinatorNodeConfig defines coordinator node settings
type CoordinatorNodeConfig struct {
	ID               string        `mapstructure:"id"`                 // Unique coordinator ID
	GRPCPort         int           `mapstructure:"grpc_port"`          // gRPC server port for workers
	HTTPPort         int           `mapstructure:"http_port"`          // HTTP API port (health, metrics)
	PollInterval     time.Duration `mapstructure:"poll_interval"`      // Task polling frequency
	HeartbeatTimeout time.Duration `mapstructure:"heartbeat_timeout"`  // Worker heartbeat timeout
	StaleTaskTimeout time.Duration `mapstructure:"stale_task_timeout"` // When to reassign stale tasks
	CleanupInterval  time.Duration `mapstructure:"cleanup_interval"`   // Dead worker cleanup frequency
	Raft             RaftConfig    `mapstructure:"raft"`               // Raft consensus configuration
}

// RaftConfig defines Raft consensus settings
type RaftConfig struct {
	NodeID            string        `mapstructure:"node_id"`            // Raft node ID
	RaftPort          int           `mapstructure:"raft_port"`          // Raft consensus port
	RaftDir           string        `mapstructure:"raft_dir"`           // Raft data directory
	Bootstrap         bool          `mapstructure:"bootstrap"`          // Bootstrap new cluster
	Peers             []RaftPeer    `mapstructure:"peers"`              // Cluster peers
	HeartbeatTimeout  time.Duration `mapstructure:"heartbeat_timeout"`  // Raft heartbeat timeout
	ElectionTimeout   time.Duration `mapstructure:"election_timeout"`   // Election timeout
	CommitTimeout     time.Duration `mapstructure:"commit_timeout"`     // Commit timeout
	MaxAppendEntries  int           `mapstructure:"max_append_entries"` // Max entries per append
	SnapshotInterval  time.Duration `mapstructure:"snapshot_interval"`  // Snapshot interval
	SnapshotThreshold uint64        `mapstructure:"snapshot_threshold"` // Log entries before snapshot
}

// RaftPeer represents a Raft cluster peer
type RaftPeer struct {
	ID      string `mapstructure:"id"`      // Peer node ID
	Address string `mapstructure:"address"` // Peer Raft address (host:port)
}

// LoggingConfig controls logging behavior
// JSON format is essential for log aggregration (ELK, Loki, etc)
type LoggingConfig struct {
	Level  string `mapstructure:"level"`  // e.g., "debug", "info", "warn", "error"
	Format string `mapstructure:"format"` // e.g., "json" or "console"
	Output string
}

// MetricsConfig for Prometheus integration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// ZKPConfig defines zero-knowledge proof settings
// These directly map to gnark lib options
type ZKPConfig struct {
	Curve            string `mapstructure:"curve"`              // e.g., "bn254", "bls12-381"
	Backend          string `mapstructure:"backend"`            // e.g., "groth16, plonk"
	CompileCacheSize int    `mapstructure:"compile_cache_size"` // Circuit compilation is expensive!
	MaxProofSizeMB   int    `mapstructure:"max_proof_size_mb"`  // Prevent DoS via huge proofs
	MerkleTreeDepth  int    `mapstructure:"merkle_tree_depth"`  // Default Merkle tree depth
}

// RateLimitConfig prevents API abuse
// Token bucket algorithm: allows bursts but limits sustained rate
type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
}

// CORSConfig for browser-based clients
type CORSConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
}

// Load reads configuration from file and environment variables
// Priority: ENV VARS > config file > defaults
// This pattern is standard in cloud-native apps (12-factor)
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults - these are fallbacks if nothing else is specified
	setDefaults(v)

	// Configure Viper to read from config file
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Read env variables
	// API_GATEWAY_SERVER_PORT will override server.port in YAML
	v.SetEnvPrefix("API_GATEWAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Attempt to read config file (not fatal if missing - we have defaults)
	if err := v.ReadInConfig(); err != nil {
		// Check if it's a "file not found" error vs parse error
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// File not found is OK - we'll use defaults + env vars
	}

	// Unmarshal into our Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// setDefaults establishes sensible defaults
// These should work for local development out-of-the-box
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "120s")

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.database", "zkp_network")
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "postgres")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "5m")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9090)
	v.SetDefault("metrics.path", "/metrics")

	// ZKP defaults
	v.SetDefault("zkp.curve", "bn254")
	v.SetDefault("zkp.backend", "groth16")
	v.SetDefault("zkp.compile_cache_size", 100)
	v.SetDefault("zkp.max_proof_size_mb", 10)

	// Rate limiting defaults
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_second", 100)
	v.SetDefault("rate_limit.burst", 200)

	// CORS defaults
	v.SetDefault("cors.enabled", true)
	v.SetDefault("cors.allowed_origins", []string{"http://localhost:3000"})
	v.SetDefault("cors.allowed_methods", []string{"GET", "POST", "PUT", "DELETE"})
	v.SetDefault("cors.allowed_headers", []string{"Content-Type", "Authorization"})
}

// Validate checks if configuration is valid
// Fail fast principle: catch config errors at startup, not during runtime
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Server.ReadTimeout < time.Second {
		return fmt.Errorf("read_timeout must be at least 1 second")
	}

	// Database validation
	if c.Database.Host == "" {
		return fmt.Errorf("database host cannot be empty")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", c.Database.Port)
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database name cannot be empty")
	}
	if c.Database.User == "" {
		return fmt.Errorf("database user cannot be empty")
	}
	validSSLModes := map[string]bool{"disable": true, "require": true, "verify-ca": true, "verify-full": true}
	if !validSSLModes[c.Database.SSLMode] {
		return fmt.Errorf("invalid ssl_mode: %s", c.Database.SSLMode)
	}
	if c.Database.MaxOpenConns < 1 {
		return fmt.Errorf("max_open_conns must be at least 1")
	}
	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("max_idle_conns cannot be negative")
	}

	// Logging validation
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	// ZKP validation
	validCurves := map[string]bool{"bn254": true, "bls12-381": true, "bls12-377": true, "bw6-761": true}
	if !validCurves[c.ZKP.Curve] {
		return fmt.Errorf("unsupported curve: %s", c.ZKP.Curve)
	}

	validBackends := map[string]bool{"groth16": true, "plonk": true}
	if !validBackends[c.ZKP.Backend] {
		return fmt.Errorf("unsupported backend: %s", c.ZKP.Backend)
	}

	if c.ZKP.MaxProofSizeMB < 1 || c.ZKP.MaxProofSizeMB > 100 {
		return fmt.Errorf("max_proof_size_mb must be between 1 and 100")
	}

	// Rate limit validation
	if c.RateLimit.Enabled {
		if c.RateLimit.RequestsPerSecond <= 0 {
			return fmt.Errorf("requests_per_second must be positive")
		}
		if c.RateLimit.Burst < 1 {
			return fmt.Errorf("burst must be at least 1")
		}
	}

	return nil
}

// GetServerAddress returns the full server address
// Helper method to avoid string concatenation everywhere
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// GetDatabaseConnectionString returns PostgreSQL connection string
// Format: postgres://user:password@host:port/database?sslmode=disable
func (c *Config) GetDatabaseConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Database.Host,
		c.Database.Port,
		c.Database.User,
		c.Database.Password,
		c.Database.Database,
		c.Database.SSLMode,
	)
}

// GetMetricsAddress returns the metrics server address
func (c *Config) GetMetricsAddress() string {
	return fmt.Sprintf(":%d", c.Metrics.Port)
}

// IsProduction checks if we're running in production mode
// Based on log level - production should use info or higher
func (c *Config) IsProduction() bool {
	return c.Logging.Level != "debug"
}

// ============================================================================
// Worker Configuration Loading
// ============================================================================

// LoadWorkerConfig reads worker configuration from file and environment variables
// Priority: ENV VARS > config file > defaults
func LoadWorkerConfig(configPath string) (*WorkerConfig, error) {
	v := viper.New()

	// Set defaults for worker
	setWorkerDefaults(v)

	// Configure Viper to read from config file
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Read env variables
	// WORKER_WORKER_ID will override worker.id in YAML
	v.SetEnvPrefix("WORKER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Attempt to read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read worker config file: %w", err)
		}
	}

	// Unmarshal into WorkerConfig struct
	var cfg WorkerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal worker config: %w", err)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid worker configuration: %w", err)
	}

	return &cfg, nil
}

// setWorkerDefaults establishes sensible defaults for worker nodes
func setWorkerDefaults(v *viper.Viper) {
	// Worker defaults
	v.SetDefault("worker.id", "worker-1")
	v.SetDefault("worker.coordinator_address", "localhost:9090")
	v.SetDefault("worker.concurrency", 4)
	v.SetDefault("worker.heartbeat_interval", "5s")
	v.SetDefault("worker.register_retries", 3)
	v.SetDefault("worker.task_timeout", "5m")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9091)
	v.SetDefault("metrics.path", "/metrics")

	// ZKP defaults
	v.SetDefault("zkp.curve", "bn254")
	v.SetDefault("zkp.backend", "groth16")
	v.SetDefault("zkp.compile_cache_size", 50)
	v.SetDefault("zkp.max_proof_size_mb", 10)
	v.SetDefault("zkp.merkle_tree_depth", 8)
}

// Validate checks if worker configuration is valid
func (c *WorkerConfig) Validate() error {
	// Worker validation
	if c.Worker.ID == "" {
		return fmt.Errorf("worker ID cannot be empty")
	}
	if c.Worker.CoordinatorAddress == "" {
		return fmt.Errorf("coordinator address cannot be empty")
	}
	if c.Worker.Concurrency < 1 || c.Worker.Concurrency > 32 {
		return fmt.Errorf("concurrency must be between 1 and 32")
	}
	if c.Worker.HeartbeatInterval < time.Second {
		return fmt.Errorf("heartbeat_interval must be at least 1 second")
	}
	if c.Worker.TaskTimeout < time.Second*10 {
		return fmt.Errorf("task_timeout must be at least 10 seconds")
	}

	// Logging validation
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	// ZKP validation
	validCurves := map[string]bool{"bn254": true, "bls12-381": true, "bls12-377": true, "bw6-761": true}
	if !validCurves[c.ZKP.Curve] {
		return fmt.Errorf("unsupported curve: %s", c.ZKP.Curve)
	}

	validBackends := map[string]bool{"groth16": true, "plonk": true}
	if !validBackends[c.ZKP.Backend] {
		return fmt.Errorf("unsupported backend: %s", c.ZKP.Backend)
	}

	return nil
}

// GetMetricsAddress returns the metrics server address for worker
func (c *WorkerConfig) GetMetricsAddress() string {
	return fmt.Sprintf(":%d", c.Metrics.Port)
}

// ============================================================================
// Coordinator Configuration Loading
// ============================================================================

// LoadCoordinatorConfig reads coordinator configuration from file and environment variables
func LoadCoordinatorConfig(configPath string) (*CoordinatorConfig, error) {
	v := viper.New()

	// Set defaults for coordinator
	setCoordinatorDefaults(v)

	// Configure Viper to read from config file
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// Read env variables with COORDINATOR_ prefix
	v.SetEnvPrefix("COORDINATOR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Attempt to read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read coordinator config file: %w", err)
		}
	}

	// Unmarshal into CoordinatorConfig struct
	var cfg CoordinatorConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal coordinator config: %w", err)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid coordinator configuration: %w", err)
	}

	return &cfg, nil
}

// setCoordinatorDefaults establishes sensible defaults for coordinator
func setCoordinatorDefaults(v *viper.Viper) {
	// Coordinator defaults
	v.SetDefault("coordinator.id", "coordinator-1")
	v.SetDefault("coordinator.grpc_port", 9090)
	v.SetDefault("coordinator.http_port", 8090)
	v.SetDefault("coordinator.poll_interval", "2s")
	v.SetDefault("coordinator.heartbeat_timeout", "30s")
	v.SetDefault("coordinator.stale_task_timeout", "5m")
	v.SetDefault("coordinator.cleanup_interval", "1m")

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.database", "zkp_network")
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.password", "postgres")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "5m")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9091)
	v.SetDefault("metrics.path", "/metrics")
}

// Validate checks if coordinator configuration is valid
func (c *CoordinatorConfig) Validate() error {
	// Coordinator validation
	if c.Coordinator.ID == "" {
		return fmt.Errorf("coordinator ID cannot be empty")
	}
	if c.Coordinator.GRPCPort < 1 || c.Coordinator.GRPCPort > 65535 {
		return fmt.Errorf("invalid gRPC port: %d", c.Coordinator.GRPCPort)
	}
	if c.Coordinator.HTTPPort < 1 || c.Coordinator.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP port: %d", c.Coordinator.HTTPPort)
	}
	if c.Coordinator.PollInterval < time.Second {
		return fmt.Errorf("poll_interval must be at least 1 second")
	}
	if c.Coordinator.HeartbeatTimeout < time.Second*5 {
		return fmt.Errorf("heartbeat_timeout must be at least 5 seconds")
	}

	// Database validation
	if c.Database.Host == "" {
		return fmt.Errorf("database host cannot be empty")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", c.Database.Port)
	}
	if c.Database.Database == "" {
		return fmt.Errorf("database name cannot be empty")
	}

	// Logging validation
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	return nil
}

// GetDatabaseConnectionString returns PostgreSQL connection string for coordinator
func (c *CoordinatorConfig) GetDatabaseConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Database.Host,
		c.Database.Port,
		c.Database.User,
		c.Database.Password,
		c.Database.Database,
		c.Database.SSLMode,
	)
}

// GetGRPCAddress returns the gRPC server address
func (c *CoordinatorConfig) GetGRPCAddress() string {
	return fmt.Sprintf(":%d", c.Coordinator.GRPCPort)
}

// GetHTTPAddress returns the HTTP API address
func (c *CoordinatorConfig) GetHTTPAddress() string {
	return fmt.Sprintf(":%d", c.Coordinator.HTTPPort)
}
