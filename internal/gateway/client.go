package gateway

import (
	"context"
	"encoding/json"

	"github.com/takutakahashi/capps/internal/config"
)

// GatewayClient is the abstraction for calling any openclaw gateway RPC method.
//
// The interface is intentionally kept generic so that new gateway methods can be
// called without any changes to this interface. Callers pass a method name string
// (e.g. "sessions.list", "config.get") together with arbitrary params, and receive
// the raw JSON payload of the response.
type GatewayClient interface {
	// Call executes an RPC method on the openclaw gateway and returns the raw
	// JSON payload of the successful response. Any gateway-level error (ok:false)
	// is returned as a *GatewayCallError which unwraps to the original GatewayError.
	Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)

	// Connect performs the WebSocket dial and the openclaw challenge/connect
	// handshake. It must be called before any Call invocations.
	Connect(ctx context.Context) error

	// Close terminates the underlying WebSocket connection.
	Close() error

	// IsConnected reports whether the client currently has an established,
	// authenticated WebSocket session with the gateway.
	IsConnected() bool
}

// GatewayCallError is returned when the gateway responds with ok:false.
type GatewayCallError struct {
	Code    string
	Message string
}

func (e *GatewayCallError) Error() string {
	return "gateway error " + e.Code + ": " + e.Message
}

// NewClient creates a new GatewayClient backed by a real WebSocket connection.
func NewClient(cfg *config.Config) GatewayClient {
	return newWSClient(cfg)
}
