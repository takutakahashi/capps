package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/takutakahashi/capps/internal/config"
)

type connectionState int

const (
	stateDisconnected connectionState = iota
	stateConnecting
	stateConnected
)

// wsClient implements GatewayClient using a gorilla/websocket connection.
//
// The pending map pattern is used to correlate asynchronous RPC responses with
// their outstanding requests: each Call registers a channel keyed by request ID;
// the receive loop dispatches incoming ResponseFrames to the correct channel.
type wsClient struct {
	cfg    *config.Config
	logger *zap.Logger

	mu    sync.RWMutex
	state connectionState
	conn  *websocket.Conn

	pendingMu sync.Mutex
	pending   map[string]chan ResponseFrame

	// closed is closed when the client is permanently shut down.
	closed chan struct{}
	once   sync.Once
}

func newWSClient(cfg *config.Config) *wsClient {
	logger, _ := zap.NewProduction()
	return &wsClient{
		cfg:     cfg,
		logger:  logger,
		pending: make(map[string]chan ResponseFrame),
		closed:  make(chan struct{}),
	}
}

// Connect dials the gateway WebSocket and performs the openclaw handshake.
func (c *wsClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	c.state = stateConnecting
	c.mu.Unlock()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, c.cfg.GatewayURL, nil)
	if err != nil {
		c.mu.Lock()
		c.state = stateDisconnected
		c.mu.Unlock()
		return fmt.Errorf("websocket dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Perform the openclaw challenge/connect handshake.
	if err := performHandshake(ctx, conn, c.cfg); err != nil {
		_ = conn.Close()
		c.mu.Lock()
		c.state = stateDisconnected
		c.conn = nil
		c.mu.Unlock()
		return fmt.Errorf("handshake: %w", err)
	}

	c.mu.Lock()
	c.state = stateConnected
	c.mu.Unlock()

	// Start the receive loop in the background.
	go c.receiveLoop()

	c.logger.Info("connected to gateway", zap.String("url", c.cfg.GatewayURL))
	return nil
}

// Call sends an RPC request and waits for the matching response.
func (c *wsClient) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to gateway")
	}

	id := uuid.NewString()
	ch := make(chan ResponseFrame, 1)
	c.registerPending(id, ch)
	defer c.unregisterPending(id)

	frame := RequestFrame{
		Type:   FrameTypeRequest,
		ID:     id,
		Method: method,
		Params: params,
	}

	if err := c.sendJSON(frame); err != nil {
		return nil, fmt.Errorf("send frame: %w", err)
	}

	// Apply per-call timeout from config if context has no deadline.
	callCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, c.cfg.RequestTimeout)
		defer cancel()
	}

	select {
	case res := <-ch:
		if !res.OK {
			if res.Error != nil {
				return nil, &GatewayCallError{Code: res.Error.Code, Message: res.Error.Message}
			}
			return nil, &GatewayCallError{Code: "UNKNOWN", Message: "gateway returned ok:false with no error details"}
		}
		return res.Payload, nil
	case <-callCtx.Done():
		return nil, fmt.Errorf("call %q timed out: %w", method, callCtx.Err())
	case <-c.closed:
		return nil, fmt.Errorf("client closed while waiting for response to %q", method)
	}
}

// Close shuts down the WebSocket connection and the receive loop.
func (c *wsClient) Close() error {
	var err error
	c.once.Do(func() {
		close(c.closed)
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.conn != nil {
			err = c.conn.Close()
			c.conn = nil
		}
		c.state = stateDisconnected
	})
	return err
}

// IsConnected reports whether the client is in the connected state.
func (c *wsClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == stateConnected
}

// sendJSON marshals v and writes it as a text message to the WebSocket.
func (c *wsClient) sendJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("connection is nil")
	}
	return c.conn.WriteJSON(v)
}

// receiveLoop reads frames from the WebSocket and dispatches them.
// It runs in its own goroutine and exits when the connection is closed.
func (c *wsClient) receiveLoop() {
	defer func() {
		c.mu.Lock()
		c.state = stateDisconnected
		c.mu.Unlock()

		// Drain all pending calls with an error.
		c.pendingMu.Lock()
		for id, ch := range c.pending {
			ch <- ResponseFrame{ID: id, OK: false, Error: &GatewayError{
				Code:    "CONNECTION_CLOSED",
				Message: "WebSocket connection closed",
			}}
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()

		c.logger.Info("receive loop exited")
	}()

	for {
		select {
		case <-c.closed:
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			return
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.logger.Info("websocket closed normally")
			} else {
				c.logger.Error("websocket read error", zap.Error(err))
			}
			return
		}

		c.dispatch(raw)
	}
}

// dispatch parses a raw WebSocket message and routes it appropriately.
func (c *wsClient) dispatch(raw []byte) {
	var incoming IncomingFrame
	if err := json.Unmarshal(raw, &incoming); err != nil {
		c.logger.Warn("failed to unmarshal incoming frame", zap.Error(err))
		return
	}

	switch incoming.Type {
	case FrameTypeResponse:
		var res ResponseFrame
		if err := json.Unmarshal(raw, &res); err != nil {
			c.logger.Warn("failed to unmarshal response frame", zap.Error(err))
			return
		}
		c.pendingMu.Lock()
		ch, ok := c.pending[res.ID]
		c.pendingMu.Unlock()
		if ok {
			ch <- res
		} else {
			c.logger.Warn("received response for unknown request ID", zap.String("id", res.ID))
		}

	case FrameTypeEvent:
		var evt EventFrame
		if err := json.Unmarshal(raw, &evt); err != nil {
			// Log at debug level — openclaw may send events with schema variations
			// (e.g. stateVersion as object instead of int) that are safe to ignore.
			c.logger.Debug("failed to unmarshal event frame (ignored)", zap.Error(err))
			return
		}
		c.logger.Debug("received event", zap.String("event", evt.Event))

	default:
		c.logger.Warn("received frame with unknown type", zap.String("type", string(incoming.Type)))
	}
}

// registerPending stores a response channel for the given request ID.
func (c *wsClient) registerPending(id string, ch chan ResponseFrame) {
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
}

// unregisterPending removes the response channel for the given request ID.
func (c *wsClient) unregisterPending(id string) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}
