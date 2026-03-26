package apierror

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"encoding/json"

	"github.com/takutakahashi/capps/internal/gateway"
)

func TestWriteErrorGatewayCallError(t *testing.T) {
	gwErr := &gateway.GatewayCallError{Code: "NOT_FOUND", Message: "session not found"}
	w := httptest.NewRecorder()
	WriteError(w, gwErr)

	assert.Equal(t, http.StatusBadGateway, w.Code)

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.OK)
	assert.Equal(t, "NOT_FOUND", resp.Error.Code)
}

func TestWriteErrorDeadlineExceeded(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, context.DeadlineExceeded)

	assert.Equal(t, http.StatusGatewayTimeout, w.Code)
	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "GATEWAY_TIMEOUT", resp.Error.Code)
}

func TestWriteErrorContextCancelled(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, context.Canceled)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestWriteErrorNotConnected(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, errors.New("not connected to gateway"))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestWriteErrorInternal(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, errors.New("something unexpected"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
