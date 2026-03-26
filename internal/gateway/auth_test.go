package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/takutakahashi/capps/internal/config"
)

func TestDeviceFingerprint(t *testing.T) {
	fp1 := deviceFingerprint("capps", "host1")
	fp2 := deviceFingerprint("capps", "host1")
	fp3 := deviceFingerprint("capps", "host2")

	assert.Equal(t, fp1, fp2, "fingerprint should be deterministic")
	assert.NotEqual(t, fp1, fp3, "different hosts should produce different fingerprints")
	assert.Len(t, fp1, 32, "fingerprint should be 16 bytes hex-encoded = 32 chars")
}

func TestSignDevice(t *testing.T) {
	s1 := signDevice("device-id", "nonce1", 1700000000)
	s2 := signDevice("device-id", "nonce1", 1700000000)
	s3 := signDevice("device-id", "nonce2", 1700000000)

	assert.Equal(t, s1, s2, "signature should be deterministic")
	assert.NotEqual(t, s1, s3, "different nonces should produce different signatures")
}

func TestBuildConnectParamsToken(t *testing.T) {
	cfg := &config.Config{
		Token:         "my-secret-token",
		ClientID:      "capps",
		ClientVersion: "0.1.0",
	}

	params := buildConnectParams(cfg, "test-nonce")

	assert.Equal(t, "my-secret-token", params.Auth.Token)
	assert.Empty(t, params.Auth.Password)
	assert.Equal(t, "operator", params.Role)
	assert.Contains(t, params.Scopes, "operator.read")
	assert.Contains(t, params.Scopes, "operator.write")
	assert.Equal(t, protocolVersion, params.MinProtocol)
	assert.Equal(t, protocolVersion, params.MaxProtocol)
	assert.Equal(t, "test-nonce", params.Device.Nonce)
}

func TestBuildConnectParamsPassword(t *testing.T) {
	cfg := &config.Config{
		Password:      "my-password",
		ClientID:      "capps",
		ClientVersion: "0.1.0",
	}

	params := buildConnectParams(cfg, "nonce")

	assert.Empty(t, params.Auth.Token)
	assert.Equal(t, "my-password", params.Auth.Password)
}
