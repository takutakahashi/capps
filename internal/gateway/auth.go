package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/takutakahashi/capps/internal/config"
)

const protocolVersion = 3

// performHandshake carries out the openclaw WebSocket challenge/connect handshake.
//
// Flow:
//  1. Wait for the "connect.challenge" event from the server.
//  2. Build and send a "connect" RPC request.
//  3. Wait for the "hello-ok" response.
func performHandshake(ctx context.Context, conn *websocket.Conn, cfg *config.Config) error {
	// Step 1: receive the challenge event.
	nonce, err := receiveChallenge(ctx, conn)
	if err != nil {
		return fmt.Errorf("receive challenge: %w", err)
	}

	// Step 2: send the connect request.
	connectID := newRequestID()
	params, err := buildConnectParams(cfg, nonce)
	if err != nil {
		return fmt.Errorf("build connect params: %w", err)
	}
	req := RequestFrame{
		Type:   FrameTypeRequest,
		ID:     connectID,
		Method: "connect",
		Params: params,
	}
	if err := conn.WriteJSON(req); err != nil {
		return fmt.Errorf("send connect request: %w", err)
	}

	// Step 3: wait for the hello-ok response.
	if err := receiveHelloOK(ctx, conn, connectID); err != nil {
		return fmt.Errorf("receive hello-ok: %w", err)
	}

	return nil
}

// receiveChallenge waits for the "connect.challenge" event and returns the nonce.
func receiveChallenge(ctx context.Context, conn *websocket.Conn) (string, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
		defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("read: %w", err)
		}

		var incoming IncomingFrame
		if err := json.Unmarshal(raw, &incoming); err != nil {
			return "", fmt.Errorf("unmarshal: %w", err)
		}

		if incoming.Type != FrameTypeEvent || incoming.Event != "connect.challenge" {
			continue
		}

		var evt EventFrame
		if err := json.Unmarshal(raw, &evt); err != nil {
			return "", fmt.Errorf("unmarshal challenge event: %w", err)
		}

		var challenge ConnectChallengePayload
		if err := json.Unmarshal(evt.Payload, &challenge); err != nil {
			return "", fmt.Errorf("unmarshal challenge payload: %w", err)
		}

		return challenge.Nonce, nil
	}
}

// receiveHelloOK waits for the connect response frame and validates it.
func receiveHelloOK(ctx context.Context, conn *websocket.Conn, connectID string) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
		defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var incoming IncomingFrame
		if err := json.Unmarshal(raw, &incoming); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}

		if incoming.Type == FrameTypeEvent {
			continue
		}

		if incoming.Type != FrameTypeResponse {
			return fmt.Errorf("unexpected frame type during handshake: %s", incoming.Type)
		}

		var res ResponseFrame
		if err := json.Unmarshal(raw, &res); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}

		if res.ID != connectID {
			continue
		}

		if !res.OK {
			if res.Error != nil {
				return fmt.Errorf("connect rejected: %s", res.Error.Error())
			}
			return fmt.Errorf("connect rejected with no error details")
		}

		return nil
	}
}

// buildConnectParams constructs the params for the "connect" RPC method.
//
// It uses an Ed25519 key pair (generated once per process) to produce the
// device identity and signature required by the openclaw protocol v3.
func buildConnectParams(cfg *config.Config, nonce string) (ConnectParams, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return ConnectParams{}, fmt.Errorf("generate ed25519 key: %w", err)
	}

	// Device ID = SHA256 of the raw 32-byte Ed25519 public key, hex-encoded.
	deviceID := deviceFingerprintEd25519(pub)

	// Public key transmitted in the connect request = raw 32 bytes, base64url (no padding).
	pubKeyB64 := base64URLEncode(pub)

	signedAtMs := time.Now().UnixMilli()

	token := cfg.Token
	platform := "linux"
	deviceFamily := ""

	// V3 signing payload: all fields joined by "|".
	payload := strings.Join([]string{
		"v3",
		deviceID,
		"cli",   // clientId  = GATEWAY_CLIENT_IDS.CLI
		"cli",   // clientMode = GATEWAY_CLIENT_MODES.CLI
		"operator",
		"operator.read,operator.write",
		fmt.Sprintf("%d", signedAtMs),
		token,
		nonce,
		platform,
		deviceFamily,
	}, "|")

	sig := ed25519.Sign(priv, []byte(payload))
	sigB64 := base64URLEncode(sig)

	auth := ConnectAuth{}
	if cfg.Token != "" {
		auth.Token = cfg.Token
	} else {
		auth.Password = cfg.Password
	}

	return ConnectParams{
		MinProtocol: protocolVersion,
		MaxProtocol: protocolVersion,
		Client: ConnectClient{
			// "cli"/"cli" matches GATEWAY_CLIENT_IDS.CLI / GATEWAY_CLIENT_MODES.CLI
			// constants defined in the openclaw protocol schema.
			ID:       "cli",
			Version:  cfg.ClientVersion,
			Platform: platform,
			Mode:     "cli",
		},
		Role:   "operator",
		Scopes: []string{"operator.read", "operator.write"},
		Auth:   auth,
		Device: ConnectDevice{
			ID:        deviceID,
			PublicKey: pubKeyB64,
			Signature: sigB64,
			SignedAt:  signedAtMs,
			Nonce:     nonce,
		},
	}, nil
}

// deviceFingerprintEd25519 returns the SHA256 hex of the raw 32-byte Ed25519 public key.
func deviceFingerprintEd25519(pub ed25519.PublicKey) string {
	h := sha256.Sum256([]byte(pub))
	return fmt.Sprintf("%x", h[:])
}

// base64URLEncode encodes bytes as base64url with no padding (RFC 4648 §5).
func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// newRequestID generates a random UUID-like request ID.
func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
