package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/takutakahashi/capps/internal/apierror"
	"github.com/takutakahashi/capps/internal/gateway"
)

// CallResponse is the envelope returned to REST clients on a successful RPC call.
type CallResponse struct {
	OK      bool            `json:"ok"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// StatusResponse is returned by GET /status.
type StatusResponse struct {
	Connected bool   `json:"connected"`
	GatewayURL string `json:"gateway_url"`
}

// handler bundles the dependencies shared by all HTTP handlers.
type handler struct {
	client     gateway.GatewayClient
	gatewayURL string
}

// handleCall is the generic, catch-all endpoint:
//
//	POST /call/{method...}
//
// The URL wildcard is joined with "." to form the openclaw method name:
//
//	/call/sessions/list  →  "sessions.list"
//	/call/config/get     →  "config.get"
//	/call/health         →  "health"
//
// The request body must be a JSON object (or empty {}). It is forwarded
// verbatim as the params of the gateway RPC call.
func (h *handler) handleCall(w http.ResponseWriter, r *http.Request) {
	// Derive the openclaw method name from the URL wildcard segment.
	methodPath := chi.URLParam(r, "*")
	if methodPath == "" {
		http.Error(w, "method is required", http.StatusBadRequest)
		return
	}
	method := strings.ReplaceAll(methodPath, "/", ".")

	// Parse the request body as the RPC params.
	params := make(map[string]any)
	if r.ContentLength != 0 {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&params); err != nil {
			// Tolerate unknown field errors — just pass params as-is.
			// Re-parse without restriction.
			params = make(map[string]any)
			_ = json.NewDecoder(r.Body).Decode(&params)
		}
	}

	payload, err := h.client.Call(r.Context(), method, params)
	if err != nil {
		apierror.WriteError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(CallResponse{
		OK:      true,
		Payload: payload,
	})
}

// handleHealthz returns a simple 200 OK to confirm the capps process is alive.
func (h *handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}` + "\n"))
}

// handleStatus returns the current WebSocket connection state.
func (h *handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := StatusResponse{
		Connected:  h.client.IsConnected(),
		GatewayURL: h.gatewayURL,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
