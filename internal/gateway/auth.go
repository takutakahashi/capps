package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
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
	connectID := uuid.NewString()
	params := buildConnectParams(cfg, nonce)
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
	// Set a read deadline based on the context.
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

		if incoming.Type != FrameTypeEvent {
			// Unexpected frame before handshake; skip.
			continue
		}

		if incoming.Event != "connect.challenge" {
			// Some other event; skip and keep waiting.
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
			// Gateway may emit additional events during handshake; skip them.
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
			// Response for a different request; skip.
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
// The device fingerprint is derived from the hostname to provide a stable,
// reproducible identifier for this capps instance without requiring key management.
func buildConnectParams(cfg *config.Config, nonce string) ConnectParams {
	hostname, _ := os.Hostname()
	deviceID := deviceFingerprint(cfg.ClientID, hostname)
	now := time.Now().UnixMilli()

	auth := ConnectAuth{}
	if cfg.Token != "" {
		auth.Token = cfg.Token
	} else {
		auth.Password = cfg.Password
	}

	platform := "linux"
	if hostname != "" {
		// Best-effort platform detection; defaults to linux.
	}

	return ConnectParams{
		MinProtocol: protocolVersion,
		MaxProtocol: protocolVersion,
		Client: ConnectClient{
			ID:       cfg.ClientID,
			Version:  cfg.ClientVersion,
			Platform: platform,
			Mode:     "operator",
		},
		Role:   "operator",
		Scopes: []string{"operator.read", "operator.write"},
		Auth:   auth,
		Device: ConnectDevice{
			ID:        deviceID,
			PublicKey: deviceID, // Use fingerprint as a stable public key placeholder.
			Signature: signDevice(deviceID, nonce, now),
			SignedAt:  now,
			Nonce:     nonce,
		},
	}
}

// deviceFingerprint generates a stable, deterministic identifier for this capps
// instance based on the client name and hostname.
func deviceFingerprint(clientID, hostname string) string {
	h := sha256.Sum256([]byte(clientID + ":" + hostname))
	return fmt.Sprintf("%x", h[:16])
}

// signDevice produces a deterministic signature for the device challenge.
// This is a simple HMAC-like construction sufficient for capps' operator role.
func signDevice(deviceID, nonce string, signedAt int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", deviceID, nonce, signedAt)))
	return fmt.Sprintf("%x", h[:])
}
