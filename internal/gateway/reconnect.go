package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/takutakahashi/capps/internal/config"
)

// ReconnectingClient wraps a GatewayClient and automatically reconnects on
// failure. It satisfies the GatewayClient interface so callers are unaware of
// the reconnection logic.
type ReconnectingClient struct {
	cfg    *config.Config
	logger *zap.Logger
	inner  GatewayClient
}

// NewReconnectingClient creates a GatewayClient that automatically reconnects.
func NewReconnectingClient(cfg *config.Config) GatewayClient {
	logger, _ := zap.NewProduction()
	rc := &ReconnectingClient{
		cfg:    cfg,
		logger: logger,
		inner:  NewClient(cfg),
	}
	return rc
}

// Connect initiates the first connection with retry support.
func (r *ReconnectingClient) Connect(ctx context.Context) error {
	return r.connectWithRetry(ctx)
}

// Call forwards the RPC call to the inner client, reconnecting if necessary.
func (r *ReconnectingClient) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if !r.inner.IsConnected() {
		if err := r.connectWithRetry(ctx); err != nil {
			return nil, fmt.Errorf("reconnect before call %q: %w", method, err)
		}
	}
	return r.inner.Call(ctx, method, params)
}

// Close delegates to the inner client.
func (r *ReconnectingClient) Close() error {
	return r.inner.Close()
}

// IsConnected delegates to the inner client.
func (r *ReconnectingClient) IsConnected() bool {
	return r.inner.IsConnected()
}

// connectWithRetry attempts to connect, retrying on failure according to cfg.
func (r *ReconnectingClient) connectWithRetry(ctx context.Context) error {
	maxRetry := r.cfg.ReconnectMaxRetry // 0 means unlimited
	interval := r.cfg.ReconnectInterval

	for attempt := 0; ; attempt++ {
		if maxRetry > 0 && attempt >= maxRetry {
			return fmt.Errorf("exceeded maximum reconnect attempts (%d)", maxRetry)
		}

		if attempt > 0 {
			r.logger.Info("reconnecting to gateway",
				zap.Int("attempt", attempt),
				zap.Duration("interval", interval),
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}

		// Replace the inner client with a fresh one before each attempt so
		// the previous (failed) connection state is cleanly discarded.
		if attempt > 0 {
			_ = r.inner.Close()
			r.inner = NewClient(r.cfg)
		}

		connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := r.inner.Connect(connectCtx)
		cancel()

		if err == nil {
			r.logger.Info("gateway connection established", zap.Int("attempt", attempt+1))
			return nil
		}

		r.logger.Warn("failed to connect to gateway", zap.Error(err), zap.Int("attempt", attempt+1))
	}
}
