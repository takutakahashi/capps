package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/takutakahashi/capps/internal/gateway"
)

// mockGatewayClient is a test double for gateway.GatewayClient.
type mockGatewayClient struct {
	connected   bool
	callResult  json.RawMessage
	callError   error
	calledWith  struct{ method string; params map[string]any }
}

func (m *mockGatewayClient) Call(_ context.Context, method string, params map[string]any) (json.RawMessage, error) {
	m.calledWith.method = method
	m.calledWith.params = params
	return m.callResult, m.callError
}

func (m *mockGatewayClient) Connect(_ context.Context) error { return nil }
func (m *mockGatewayClient) Close() error                    { return nil }
func (m *mockGatewayClient) IsConnected() bool               { return m.connected }

// newTestHandler creates a handler with a mock client and registers it in a chi router.
func newTestHandler(client gateway.GatewayClient) http.Handler {
	h := &handler{client: client, gatewayURL: "ws://test-gateway"}
	r := chi.NewRouter()
	r.Post("/call/*", h.handleCall)
	r.Get("/healthz", h.handleHealthz)
	r.Get("/status", h.handleStatus)
	return r
}

func TestHandleCallSuccess(t *testing.T) {
	want := json.RawMessage(`{"sessions":["a","b"]}`)
	mock := &mockGatewayClient{connected: true, callResult: want}

	req := httptest.NewRequest(http.MethodPost, "/call/sessions/list", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newTestHandler(mock).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp CallResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.OK)
	assert.JSONEq(t, string(want), string(resp.Payload))

	// Verify method translation: /call/sessions/list → sessions.list
	assert.Equal(t, "sessions.list", mock.calledWith.method)
}

func TestHandleCallMethodTranslation(t *testing.T) {
	cases := []struct {
		path       string
		wantMethod string
	}{
		{"/call/health", "health"},
		{"/call/config/get", "config.get"},
		{"/call/sessions/list", "sessions.list"},
		{"/call/logs/tail", "logs.tail"},
		{"/call/node/list", "node.list"},
	}

	for _, tc := range cases {
		mock := &mockGatewayClient{connected: true, callResult: json.RawMessage(`{}`)}
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		newTestHandler(mock).ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "path: %s", tc.path)
		assert.Equal(t, tc.wantMethod, mock.calledWith.method, "path: %s", tc.path)
	}
}

func TestHandleCallGatewayError(t *testing.T) {
	gwErr := &gateway.GatewayCallError{Code: "NOT_FOUND", Message: "session not found"}
	mock := &mockGatewayClient{connected: true, callError: gwErr}

	req := httptest.NewRequest(http.MethodPost, "/call/sessions/get", bytes.NewBufferString(`{"id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newTestHandler(mock).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestHandleCallNotConnected(t *testing.T) {
	mock := &mockGatewayClient{connected: true, callError: errors.New("not connected to gateway")}

	req := httptest.NewRequest(http.MethodPost, "/call/health", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newTestHandler(mock).ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleHealthz(t *testing.T) {
	mock := &mockGatewayClient{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	newTestHandler(mock).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ok":true`)
}

func TestHandleStatus(t *testing.T) {
	mock := &mockGatewayClient{connected: true}
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()

	newTestHandler(mock).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp StatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Connected)
	assert.Equal(t, "ws://test-gateway", resp.GatewayURL)
}

func TestHandleCallWithParams(t *testing.T) {
	mock := &mockGatewayClient{connected: true, callResult: json.RawMessage(`{}`)}

	body := `{"sinceMs":60000,"limit":100}`
	req := httptest.NewRequest(http.MethodPost, "/call/logs/tail", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	newTestHandler(mock).ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "logs.tail", mock.calledWith.method)
	assert.Equal(t, float64(60000), mock.calledWith.params["sinceMs"])
}
