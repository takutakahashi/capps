package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/takutakahashi/capps/internal/config"
)

// upgrader is used by the mock gateway server.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// mockGatewayServer creates a test WebSocket server that simulates the openclaw
// gateway handshake and responds to RPC requests with a fixed payload.
func mockGatewayServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		handler(conn)
	}))
	return ts
}

// performMockHandshake simulates the gateway side of the challenge/connect flow.
func performMockHandshake(t *testing.T, conn *websocket.Conn) {
	t.Helper()

	// 1. Send connect.challenge
	challenge := EventFrame{
		Type:  FrameTypeEvent,
		Event: "connect.challenge",
		Payload: json.RawMessage(`{"nonce":"test-nonce","ts":1700000000000}`),
	}
	require.NoError(t, conn.WriteJSON(challenge))

	// 2. Read connect request
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err)

	var req RequestFrame
	require.NoError(t, json.Unmarshal(raw, &req))
	assert.Equal(t, "connect", req.Method)

	// 3. Send hello-ok
	resp := ResponseFrame{
		Type: FrameTypeResponse,
		ID:   req.ID,
		OK:   true,
		Payload: json.RawMessage(`{"type":"hello-ok","protocol":3,"policy":{"tickIntervalMs":15000}}`),
	}
	require.NoError(t, conn.WriteJSON(resp))
}

func TestWSClientConnect(t *testing.T) {
	ts := mockGatewayServer(t, func(conn *websocket.Conn) {
		performMockHandshake(t, conn)
		// Keep connection open until client closes.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer ts.Close()

	cfg := &config.Config{
		GatewayURL:     "ws" + strings.TrimPrefix(ts.URL, "http"),
		Token:          "test-token",
		RequestTimeout: 5 * time.Second,
	}

	client := newWSClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, client.Connect(ctx))
	assert.True(t, client.IsConnected())
	require.NoError(t, client.Close())
}

func TestWSClientCall(t *testing.T) {
	const wantPayload = `{"sessions":["s1","s2"]}`

	ts := mockGatewayServer(t, func(conn *websocket.Conn) {
		performMockHandshake(t, conn)

		// Handle the RPC call.
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req RequestFrame
		if err := json.Unmarshal(raw, &req); err != nil {
			return
		}

		resp := ResponseFrame{
			Type:    FrameTypeResponse,
			ID:      req.ID,
			OK:      true,
			Payload: json.RawMessage(wantPayload),
		}
		_ = conn.WriteJSON(resp)
	})
	defer ts.Close()

	cfg := &config.Config{
		GatewayURL:     "ws" + strings.TrimPrefix(ts.URL, "http"),
		Token:          "test-token",
		RequestTimeout: 5 * time.Second,
	}

	client := newWSClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, client.Connect(ctx))
	defer client.Close()

	payload, err := client.Call(ctx, "sessions.list", map[string]any{})
	require.NoError(t, err)
	assert.JSONEq(t, wantPayload, string(payload))
}

func TestWSClientCallGatewayError(t *testing.T) {
	ts := mockGatewayServer(t, func(conn *websocket.Conn) {
		performMockHandshake(t, conn)

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req RequestFrame
		if err := json.Unmarshal(raw, &req); err != nil {
			return
		}

		resp := ResponseFrame{
			Type: FrameTypeResponse,
			ID:   req.ID,
			OK:   false,
			Error: &GatewayError{
				Code:    "NOT_FOUND",
				Message: "session not found",
			},
		}
		_ = conn.WriteJSON(resp)
	})
	defer ts.Close()

	cfg := &config.Config{
		GatewayURL:     "ws" + strings.TrimPrefix(ts.URL, "http"),
		Token:          "test-token",
		RequestTimeout: 5 * time.Second,
	}

	client := newWSClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, client.Connect(ctx))
	defer client.Close()

	_, err := client.Call(ctx, "sessions.get", map[string]any{"id": "missing"})
	require.Error(t, err)

	var gwErr *GatewayCallError
	require.ErrorAs(t, err, &gwErr)
	assert.Equal(t, "NOT_FOUND", gwErr.Code)
}

func TestWSClientNotConnected(t *testing.T) {
	cfg := &config.Config{
		GatewayURL:     "ws://localhost:0",
		Token:          "test-token",
		RequestTimeout: 5 * time.Second,
	}
	client := newWSClient(cfg)

	_, err := client.Call(context.Background(), "health", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}
