package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	t.Run("valid with token", func(t *testing.T) {
		cfg := &Config{
			GatewayURL: "ws://localhost:18789",
			Token:      "my-token",
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid with password", func(t *testing.T) {
		cfg := &Config{
			GatewayURL: "ws://localhost:18789",
			Password:   "my-password",
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("missing gateway url", func(t *testing.T) {
		cfg := &Config{Token: "tok"}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gateway URL is required")
	})

	t.Run("missing auth", func(t *testing.T) {
		cfg := &Config{GatewayURL: "ws://localhost:18789"}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authentication is required")
	})
}

func TestLoadWithOverrides(t *testing.T) {
	cfg, err := LoadWithOverrides(
		"ws://gateway:18789",
		"my-token",
		"",
		":9090",
		"debug",
		10*time.Second,
		3*time.Second,
		5,
	)
	require.NoError(t, err)

	assert.Equal(t, "ws://gateway:18789", cfg.GatewayURL)
	assert.Equal(t, "my-token", cfg.Token)
	assert.Equal(t, ":9090", cfg.ListenAddr)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 10*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 3*time.Second, cfg.ReconnectInterval)
	assert.Equal(t, 5, cfg.ReconnectMaxRetry)
}
