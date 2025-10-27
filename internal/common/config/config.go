package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration
// We use a single struct to make it easy to pass around and test
// Each section maps to a YAML block in the config file
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Logging LoggingConfig `mapstructure:"logging"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	ZKP ZKPConfig `mapstructure:"zkp"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	CORS CORSConfig `mapstructure:"cors"`
}

// ServerConfig defines HTTP server settings
// Timeouts are CRITICAL in production - without them, slow client can exhaust resources
type ServerConfig struct {
	Host string 			`mapstructure:"host"`
	Port int     			`mapstructure:"port"`
	ReadTimeout time.Duration 	`mapstructure:"read_timeout"`
	WriteTimeout time.Duration 	`mapstructure:"write_timeout"`
	IdleTimeout time.Duration 	`mapstructure:"idle_timeout"`
}

//LoggingConfig controls logging behavior
// JSON format is essential for log aggregration (ELK, Loki, etc)
type LoggingConfig struct {
	Level string `mapstructure:"level"` // e.g., "debug", "info", "warn", "error"
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
	Curve string `mapstructure:"curve"` // e.g., "bn254", "bls12-381"
	Backend string `mapstructure:backend"` // e.g., "groth16, plonk"
	CompileCacheSize int `mapstructure:"compile_cache_size"` // Circuit compilation is expensive!
	MaxProofSizeMB int `mapstructure:"max_proof_size_mb"`// Prevent DoS via huge proofs
	MerkleTreeDepth   int    `mapstructure:"merkle_tree_depth"`   // Default Merkle tree depth
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

// GetMetricsAddress returns the metrics server address
func (c *Config) GetMetricsAddress() string {
	return fmt.Sprintf(":%d", c.Metrics.Port)
}

// IsProduction checks if we're running in production mode
// Based on log level - production should use info or higher
func (c *Config) IsProduction() bool {
	return c.Logging.Level != "debug"
}