package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/takutakahashi/capps/internal/config"
)

func TestDeviceFingerprintEd25519(t *testing.T) {
	// Two calls with the same public key bytes should yield the same fingerprint.
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i)
	}
	fp1 := deviceFingerprintEd25519(pub)
	fp2 := deviceFingerprintEd25519(pub)

	assert.Equal(t, fp1, fp2, "fingerprint should be deterministic")
	assert.Len(t, fp1, 64, "SHA256 hex should be 64 chars")

	// Different key → different fingerprint.
	pub2 := make([]byte, 32)
	fp3 := deviceFingerprintEd25519(pub2)
	assert.NotEqual(t, fp1, fp3, "different keys should produce different fingerprints")
}

func TestBase64URLEncode(t *testing.T) {
	// base64url with no padding should not contain '+', '/', or '='.
	b := []byte{0xfb, 0xff, 0xfe}
	s := base64URLEncode(b)
	assert.NotContains(t, s, "+")
	assert.NotContains(t, s, "/")
	assert.NotContains(t, s, "=")
}

func TestBuildConnectParamsToken(t *testing.T) {
	cfg := &config.Config{
		Token:         "my-secret-token",
		ClientVersion: "0.1.0",
	}

	params, err := buildConnectParams(cfg, "test-nonce")
	require.NoError(t, err)

	assert.Equal(t, "my-secret-token", params.Auth.Token)
	assert.Empty(t, params.Auth.Password)
	assert.Equal(t, "operator", params.Role)
	assert.Contains(t, params.Scopes, "operator.read")
	assert.Contains(t, params.Scopes, "operator.write")
	assert.Equal(t, protocolVersion, params.MinProtocol)
	assert.Equal(t, protocolVersion, params.MaxProtocol)
	assert.Equal(t, "test-nonce", params.Device.Nonce)

	// Device fields should be populated.
	assert.NotEmpty(t, params.Device.ID)
	assert.NotEmpty(t, params.Device.PublicKey)
	assert.NotEmpty(t, params.Device.Signature)
	assert.Positive(t, params.Device.SignedAt)

	// Client fields should be set.
	assert.Equal(t, "cli", params.Client.ID)
	assert.Equal(t, "cli", params.Client.Mode)
}

func TestBuildConnectParamsPassword(t *testing.T) {
	cfg := &config.Config{
		Password:      "my-password",
		ClientVersion: "0.1.0",
	}

	params, err := buildConnectParams(cfg, "nonce")
	require.NoError(t, err)

	assert.Empty(t, params.Auth.Token)
	assert.Equal(t, "my-password", params.Auth.Password)
}

func TestBuildConnectParamsDeviceUnique(t *testing.T) {
	// Each call generates a fresh Ed25519 key pair, so device IDs differ.
	cfg := &config.Config{Token: "tok", ClientVersion: "0.1.0"}

	p1, err := buildConnectParams(cfg, "nonce")
	require.NoError(t, err)
	p2, err := buildConnectParams(cfg, "nonce")
	require.NoError(t, err)

	assert.NotEqual(t, p1.Device.ID, p2.Device.ID, "each call generates a unique device ID")
	assert.NotEqual(t, p1.Device.PublicKey, p2.Device.PublicKey)
	assert.NotEqual(t, p1.Device.Signature, p2.Device.Signature)
}
