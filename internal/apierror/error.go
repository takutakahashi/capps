package apierror

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/takutakahashi/capps/internal/gateway"
)

// APIError is the standard error response body for all capps HTTP endpoints.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the top-level error envelope returned to REST clients.
type ErrorResponse struct {
	OK    bool     `json:"ok"`
	Error APIError `json:"error"`
}

// WriteError writes a JSON error response with the appropriate HTTP status code.
func WriteError(w http.ResponseWriter, err error) {
	status, apiErr := classify(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		OK:    false,
		Error: apiErr,
	})
}

// classify maps a Go error to an HTTP status code and an APIError.
func classify(err error) (int, APIError) {
	if err == nil {
		return http.StatusInternalServerError, APIError{
			Code:    "INTERNAL_ERROR",
			Message: "unknown error",
		}
	}

	// Gateway-level RPC error (ok: false from openclaw).
	var gwErr *gateway.GatewayCallError
	if errors.As(err, &gwErr) {
		return http.StatusBadGateway, APIError{
			Code:    gwErr.Code,
			Message: gwErr.Message,
		}
	}

	// Context deadline exceeded → 504.
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, APIError{
			Code:    "GATEWAY_TIMEOUT",
			Message: "request to gateway timed out",
		}
	}

	// Context cancelled → 503 (client disconnected or server shutting down).
	if errors.Is(err, context.Canceled) {
		return http.StatusServiceUnavailable, APIError{
			Code:    "REQUEST_CANCELLED",
			Message: "request was cancelled",
		}
	}

	// Not connected to gateway.
	if isNotConnected(err) {
		return http.StatusServiceUnavailable, APIError{
			Code:    "GATEWAY_UNAVAILABLE",
			Message: err.Error(),
		}
	}

	// Default: internal server error.
	return http.StatusInternalServerError, APIError{
		Code:    "INTERNAL_ERROR",
		Message: err.Error(),
	}
}

// isNotConnected heuristically detects "not connected to gateway" errors.
func isNotConnected(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "not connected to gateway" ||
		contains(msg, "connection refused") ||
		contains(msg, "CONNECTION_CLOSED")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
