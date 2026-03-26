package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestFrameMarshal(t *testing.T) {
	frame := RequestFrame{
		Type:   FrameTypeRequest,
		ID:     "test-id",
		Method: "sessions.list",
		Params: map[string]any{"limit": 10},
	}

	raw, err := json.Marshal(frame)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	assert.Equal(t, "req", m["type"])
	assert.Equal(t, "test-id", m["id"])
	assert.Equal(t, "sessions.list", m["method"])
}

func TestResponseFrameUnmarshal(t *testing.T) {
	raw := []byte(`{"type":"res","id":"abc","ok":true,"payload":{"sessions":[]}}`)
	var frame ResponseFrame
	require.NoError(t, json.Unmarshal(raw, &frame))

	assert.Equal(t, FrameTypeResponse, frame.Type)
	assert.Equal(t, "abc", frame.ID)
	assert.True(t, frame.OK)
	assert.Nil(t, frame.Error)
	assert.NotNil(t, frame.Payload)
}

func TestResponseFrameError(t *testing.T) {
	raw := []byte(`{"type":"res","id":"xyz","ok":false,"error":{"code":"AUTH_FAILED","message":"bad token"}}`)
	var frame ResponseFrame
	require.NoError(t, json.Unmarshal(raw, &frame))

	assert.False(t, frame.OK)
	require.NotNil(t, frame.Error)
	assert.Equal(t, "AUTH_FAILED", frame.Error.Code)
	assert.Equal(t, "bad token", frame.Error.Message)
}

func TestEventFrameUnmarshal(t *testing.T) {
	raw := []byte(`{"type":"event","event":"connect.challenge","payload":{"nonce":"abc123","ts":1700000000000}}`)
	var frame EventFrame
	require.NoError(t, json.Unmarshal(raw, &frame))

	assert.Equal(t, FrameTypeEvent, frame.Type)
	assert.Equal(t, "connect.challenge", frame.Event)

	var challenge ConnectChallengePayload
	require.NoError(t, json.Unmarshal(frame.Payload, &challenge))
	assert.Equal(t, "abc123", challenge.Nonce)
}

func TestGatewayErrorMessage(t *testing.T) {
	err := &GatewayError{Code: "NOT_FOUND", Message: "resource not found"}
	assert.Equal(t, "NOT_FOUND: resource not found", err.Error())
}

func TestIncomingFrameDiscriminator(t *testing.T) {
	cases := []struct {
		raw      string
		wantType FrameType
	}{
		{`{"type":"req","id":"1","method":"health","params":{}}`, FrameTypeRequest},
		{`{"type":"res","id":"1","ok":true}`, FrameTypeResponse},
		{`{"type":"event","event":"tick","payload":{}}`, FrameTypeEvent},
	}

	for _, tc := range cases {
		var f IncomingFrame
		require.NoError(t, json.Unmarshal([]byte(tc.raw), &f))
		assert.Equal(t, tc.wantType, f.Type)
	}
}
