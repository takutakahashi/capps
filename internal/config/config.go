package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the capps server.
type Config struct {
	// Gateway connection settings
	GatewayURL string `mapstructure:"gateway_url"`
	Token      string `mapstructure:"token"`
	Password   string `mapstructure:"password"`

	// HTTP server settings
	ListenAddr string `mapstructure:"listen_addr"`

	// Logging
	LogLevel string `mapstructure:"log_level"`

	// Timeouts and retries
	RequestTimeout    time.Duration `mapstructure:"request_timeout"`
	ReconnectInterval time.Duration `mapstructure:"reconnect_interval"`
	ReconnectMaxRetry int           `mapstructure:"reconnect_max_retry"`

	// Client identification
	ClientID      string `mapstructure:"client_id"`
	ClientVersion string `mapstructure:"client_version"`
}

// Load reads configuration from environment variables and viper bindings.
func Load() (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("listen_addr", ":8080")
	v.SetDefault("log_level", "info")
	v.SetDefault("request_timeout", "30s")
	v.SetDefault("reconnect_interval", "5s")
	v.SetDefault("reconnect_max_retry", 0)
	v.SetDefault("client_id", "capps")
	v.SetDefault("client_version", "0.1.0")

	// Environment variable bindings
	v.SetEnvPrefix("")
	v.AutomaticEnv()

	// Map env vars to config keys
	_ = v.BindEnv("gateway_url", "OPENCLAW_GATEWAY_URL")
	_ = v.BindEnv("token", "OPENCLAW_GATEWAY_TOKEN")
	_ = v.BindEnv("password", "OPENCLAW_GATEWAY_PASSWORD")
	_ = v.BindEnv("listen_addr", "CAPPS_LISTEN_ADDR")
	_ = v.BindEnv("log_level", "CAPPS_LOG_LEVEL")
	_ = v.BindEnv("request_timeout", "CAPPS_REQUEST_TIMEOUT")
	_ = v.BindEnv("reconnect_interval", "CAPPS_RECONNECT_INTERVAL")
	_ = v.BindEnv("reconnect_max_retry", "CAPPS_RECONNECT_MAX_RETRY")
	_ = v.BindEnv("client_id", "CAPPS_CLIENT_ID")
	_ = v.BindEnv("client_version", "CAPPS_CLIENT_VERSION")

	cfg := &Config{}

	// Parse durations manually since viper doesn't auto-convert string to time.Duration
	cfg.GatewayURL = v.GetString("gateway_url")
	cfg.Token = v.GetString("token")
	cfg.Password = v.GetString("password")
	cfg.ListenAddr = v.GetString("listen_addr")
	cfg.LogLevel = v.GetString("log_level")
	cfg.ReconnectMaxRetry = v.GetInt("reconnect_max_retry")
	cfg.ClientID = v.GetString("client_id")
	cfg.ClientVersion = v.GetString("client_version")

	var err error
	cfg.RequestTimeout, err = time.ParseDuration(v.GetString("request_timeout"))
	if err != nil {
		return nil, fmt.Errorf("invalid request_timeout: %w", err)
	}

	cfg.ReconnectInterval, err = time.ParseDuration(v.GetString("reconnect_interval"))
	if err != nil {
		return nil, fmt.Errorf("invalid reconnect_interval: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadWithOverrides creates a Config with explicit overrides (for CLI flags).
func LoadWithOverrides(gatewayURL, token, password, listenAddr, logLevel string, requestTimeout, reconnectInterval time.Duration, reconnectMaxRetry int) (*Config, error) {
	cfg, err := Load()
	if err != nil {
		// If base load fails due to missing gateway_url, we may override it below
		cfg = &Config{
			ListenAddr:        ":8080",
			LogLevel:          "info",
			RequestTimeout:    30 * time.Second,
			ReconnectInterval: 5 * time.Second,
			ClientID:          "capps",
			ClientVersion:     "0.1.0",
		}
	}

	if gatewayURL != "" {
		cfg.GatewayURL = gatewayURL
	}
	if token != "" {
		cfg.Token = token
	}
	if password != "" {
		cfg.Password = password
	}
	if listenAddr != "" {
		cfg.ListenAddr = listenAddr
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if requestTimeout > 0 {
		cfg.RequestTimeout = requestTimeout
	}
	if reconnectInterval > 0 {
		cfg.ReconnectInterval = reconnectInterval
	}
	if reconnectMaxRetry > 0 {
		cfg.ReconnectMaxRetry = reconnectMaxRetry
	}

	return cfg, cfg.Validate()
}

// Validate checks that required configuration is present.
func (c *Config) Validate() error {
	if c.GatewayURL == "" {
		return fmt.Errorf("gateway URL is required (set OPENCLAW_GATEWAY_URL or --gateway-url)")
	}
	if c.Token == "" && c.Password == "" {
		return fmt.Errorf("authentication is required: set OPENCLAW_GATEWAY_TOKEN or OPENCLAW_GATEWAY_PASSWORD")
	}
	return nil
}
