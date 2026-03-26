package gateway

import "encoding/json"

// FrameType represents the type field of a WebSocket frame.
type FrameType string

const (
	FrameTypeRequest  FrameType = "req"
	FrameTypeResponse FrameType = "res"
	FrameTypeEvent    FrameType = "event"
)

// RequestFrame is sent from capps to the openclaw gateway.
type RequestFrame struct {
	Type   FrameType `json:"type"`
	ID     string    `json:"id"`
	Method string    `json:"method"`
	Params any       `json:"params"`
}

// ResponseFrame is received from the openclaw gateway in reply to a RequestFrame.
type ResponseFrame struct {
	Type    FrameType        `json:"type"`
	ID      string           `json:"id"`
	OK      bool             `json:"ok"`
	Payload json.RawMessage  `json:"payload,omitempty"`
	Error   *GatewayError    `json:"error,omitempty"`
}

// EventFrame is an unsolicited message pushed by the gateway (e.g. connect.challenge).
type EventFrame struct {
	Type         FrameType       `json:"type"`
	Event        string          `json:"event"`
	Payload      json.RawMessage `json:"payload"`
	Seq          *int            `json:"seq,omitempty"`
	StateVersion *int            `json:"stateVersion,omitempty"`
}

// GatewayError represents an error returned by the openclaw gateway.
type GatewayError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

func (e *GatewayError) Error() string {
	return e.Code + ": " + e.Message
}

// IncomingFrame is used to discriminate the type of an incoming WebSocket message.
type IncomingFrame struct {
	Type FrameType `json:"type"`
	// For events
	Event string `json:"event,omitempty"`
}

// ConnectChallengePayload is the payload of the "connect.challenge" event.
type ConnectChallengePayload struct {
	Nonce string `json:"nonce"`
	Ts    int64  `json:"ts"`
}

// ConnectParams is the params object of the "connect" RPC request.
type ConnectParams struct {
	MinProtocol int              `json:"minProtocol"`
	MaxProtocol int              `json:"maxProtocol"`
	Client      ConnectClient    `json:"client"`
	Role        string           `json:"role"`
	Scopes      []string         `json:"scopes"`
	Auth        ConnectAuth      `json:"auth"`
	Device      ConnectDevice    `json:"device"`
}

// ConnectClient describes the connecting client.
type ConnectClient struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

// ConnectAuth holds authentication credentials.
type ConnectAuth struct {
	Token    string `json:"token,omitempty"`
	Password string `json:"password,omitempty"`
}

// ConnectDevice describes the device fingerprint used in the challenge-response.
type ConnectDevice struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
	SignedAt  int64  `json:"signedAt"`
	Nonce     string `json:"nonce"`
}

// HelloOKPayload is the payload of the successful connect response.
type HelloOKPayload struct {
	Type     string         `json:"type"`
	Protocol int            `json:"protocol"`
	Policy   HelloOKPolicy  `json:"policy"`
	Auth     *HelloOKAuth   `json:"auth,omitempty"`
}

// HelloOKPolicy contains server-side policy settings.
type HelloOKPolicy struct {
	TickIntervalMs int `json:"tickIntervalMs"`
}

// HelloOKAuth contains auth tokens returned by the server.
type HelloOKAuth struct {
	DeviceToken string `json:"deviceToken,omitempty"`
}
