package e2e_test

import (
	"strings"
	"testing"

	"github.com/dlc-01/replicast/internal/e2e"
)

// — KeyPair (пользовательские ключи) ──────────────────────────────────

func TestGenerateKeyPair_ReturnsKeys(t *testing.T) {
	kp, err := e2e.GenerateKeyPair()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kp.PublicKey == "" {
		t.Error("public key should not be empty")
	}
	if kp.PrivateKey == "" {
		t.Error("private key should not be empty")
	}
}

func TestGenerateKeyPair_KeysAreDifferent(t *testing.T) {
	kp1, _ := e2e.GenerateKeyPair()
	kp2, _ := e2e.GenerateKeyPair()
	if kp1.PublicKey == kp2.PublicKey {
		t.Error("two generated key pairs should be different")
	}
}

func TestGenerateKeyPair_PublicKeyParseable(t *testing.T) {
	kp, _ := e2e.GenerateKeyPair()
	pub, err := e2e.ParsePublicKey(kp.PublicKey)
	if err != nil {
		t.Fatalf("public key not parseable: %v", err)
	}
	if pub == nil {
		t.Error("parsed public key should not be nil")
	}
}

func TestGenerateKeyPair_PrivateKeyParseable(t *testing.T) {
	kp, _ := e2e.GenerateKeyPair()
	priv, err := e2e.ParsePrivateKey(kp.PrivateKey)
	if err != nil {
		t.Fatalf("private key not parseable: %v", err)
	}
	if priv == nil {
		t.Error("parsed private key should not be nil")
	}
}

func TestParsePublicKey_InvalidPEM(t *testing.T) {
	_, err := e2e.ParsePublicKey("not a pem")
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestParsePrivateKey_InvalidPEM(t *testing.T) {
	_, err := e2e.ParsePrivateKey("not a pem")
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestPublicKeyFormat_IsPEM(t *testing.T) {
	kp, _ := e2e.GenerateKeyPair()
	if !strings.HasPrefix(kp.PublicKey, "-----BEGIN") {
		t.Errorf("public key should start with PEM header, got: %q", kp.PublicKey[:20])
	}
}

func TestPrivateKeyFormat_IsPEM(t *testing.T) {
	kp, _ := e2e.GenerateKeyPair()
	if !strings.HasPrefix(kp.PrivateKey, "-----BEGIN") {
		t.Errorf("private key should start with PEM header, got: %q", kp.PrivateKey[:20])
	}
}

// — NodeKeyPair (ключи узла для межузловой подписи) ───────────────────

func TestGenerateNodeKeyPair(t *testing.T) {
	kp, err := e2e.GenerateNodeKeyPair()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kp.PublicKeyPEM == "" {
		t.Error("public key PEM should not be empty")
	}
	if kp.PrivateKey == nil {
		t.Error("private key should not be nil")
	}
}

func TestGenerateNodeKeyPair_TwoPairsAreDifferent(t *testing.T) {
	kp1, _ := e2e.GenerateNodeKeyPair()
	kp2, _ := e2e.GenerateNodeKeyPair()
	if kp1.PublicKeyPEM == kp2.PublicKeyPEM {
		t.Error("two node key pairs should be different")
	}
}

func TestNodeKeyPair_Sign_ReturnsNonEmpty(t *testing.T) {
	kp, _ := e2e.GenerateNodeKeyPair()
	sig, err := kp.Sign([]byte("node-a:1775217000:abc123"))
	if err != nil {
		t.Fatalf("sign error: %v", err)
	}
	if sig == "" {
		t.Error("signature should not be empty")
	}
}

func TestNodeKeyPair_SignAndVerify_Success(t *testing.T) {
	kp, _ := e2e.GenerateNodeKeyPair()
	data := []byte("node-a:1775217000:bodyHash")

	sig, err := kp.Sign(data)
	if err != nil {
		t.Fatalf("sign error: %v", err)
	}

	if err := e2e.VerifyNodeSignature(kp.PublicKeyPEM, data, sig); err != nil {
		t.Errorf("verify error: %v", err)
	}
}

func TestNodeKeyPair_Verify_WrongKey(t *testing.T) {
	kp1, _ := e2e.GenerateNodeKeyPair()
	kp2, _ := e2e.GenerateNodeKeyPair()

	sig, _ := kp1.Sign([]byte("data"))
	err := e2e.VerifyNodeSignature(kp2.PublicKeyPEM, []byte("data"), sig)
	if err == nil {
		t.Error("expected error when verifying with wrong key")
	}
}

func TestNodeKeyPair_Verify_TamperedData(t *testing.T) {
	kp, _ := e2e.GenerateNodeKeyPair()
	sig, _ := kp.Sign([]byte("original data"))

	err := e2e.VerifyNodeSignature(kp.PublicKeyPEM, []byte("tampered data"), sig)
	if err == nil {
		t.Error("expected error for tampered data")
	}
}

func TestNodeKeyPair_Verify_InvalidSignature(t *testing.T) {
	kp, _ := e2e.GenerateNodeKeyPair()
	err := e2e.VerifyNodeSignature(kp.PublicKeyPEM, []byte("data"), "not-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64 signature")
	}
}

func TestNodeKeyPairFromPrivate_RoundTrip(t *testing.T) {
	kp1, _ := e2e.GenerateNodeKeyPair()
	kp2, err := e2e.NodeKeyPairFromPrivate(kp1.PrivateKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kp1.PublicKeyPEM != kp2.PublicKeyPEM {
		t.Error("public keys should be equal after round trip")
	}
}

func TestNodeKeyPair_SignVerify_LargePayload(t *testing.T) {
	kp, _ := e2e.GenerateNodeKeyPair()
	data := []byte(strings.Repeat("x", 10000))

	sig, err := kp.Sign(data)
	if err != nil {
		t.Fatalf("sign error for large payload: %v", err)
	}
	if err := e2e.VerifyNodeSignature(kp.PublicKeyPEM, data, sig); err != nil {
		t.Errorf("verify error for large payload: %v", err)
	}
}
