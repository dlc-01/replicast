package signing_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/dlc-01/replicast/internal/signing"
)

// — SignRequest ────────────────────────────────────────────────────────

func TestSignRequest_AddsAllHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/events",
		bytes.NewBufferString(`{"event_id":"001"}`))

	if err := signing.SignRequest(req, "node-a", "secret"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Header.Get(signing.HeaderNodeName) != "node-a" {
		t.Errorf("node header = %q, want node-a", req.Header.Get(signing.HeaderNodeName))
	}
	if req.Header.Get(signing.HeaderTimestamp) == "" {
		t.Error("timestamp header missing")
	}
	if req.Header.Get(signing.HeaderSignature) == "" {
		t.Error("signature header missing")
	}
}

func TestSignRequest_BodyRemainsReadable(t *testing.T) {
	body := `{"content":"hello federation"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))

	if err := signing.SignRequest(req, "node-a", "secret"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := io.ReadAll(req.Body)
	if string(got) != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestSignRequest_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := signing.SignRequest(req, "node-a", "secret"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Header.Get(signing.HeaderSignature) == "" {
		t.Error("signature should be set even for empty body")
	}
}

func TestSignRequest_DifferentSecrets_DifferentSignatures(t *testing.T) {
	body := []byte(`{"key":"value"}`)

	req1 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req2 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))

	signing.SignRequest(req1, "node-a", "secret-1")
	signing.SignRequest(req2, "node-a", "secret-2")

	if req1.Header.Get(signing.HeaderSignature) == req2.Header.Get(signing.HeaderSignature) {
		t.Error("different secrets should produce different signatures")
	}
}

// — Verify ─────────────────────────────────────────────────────────────

func TestVerify_ValidSignature(t *testing.T) {
	body := `{"event_type":"post.created"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	signing.SignRequest(req, "node-a", "secret")

	got, err := signing.Verify(req, "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	signing.SignRequest(req, "node-a", "correct-secret")

	_, err := signing.Verify(req, "wrong-secret")
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestVerify_NoHeaders_ReturnsNilNil(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))

	body, err := signing.Verify(req, "secret")
	if err != nil {
		t.Errorf("expected no error for missing headers, got %v", err)
	}
	if body != nil {
		t.Error("expected nil body when headers missing")
	}
}

func TestVerify_OldTimestamp_Rejected(t *testing.T) {
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))

	// Ставим старый timestamp
	oldTS := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	req.Header.Set(signing.HeaderNodeName, "node-a")
	req.Header.Set(signing.HeaderTimestamp, oldTS)
	msg := "node-a:" + oldTS + ":" + signing.SHA256Hex(body)
	req.Header.Set(signing.HeaderSignature, signing.HMACSHA256("secret", msg))

	_, err := signing.Verify(req, "secret")
	if err == nil {
		t.Error("expected error for old timestamp")
	}
}

func TestVerify_FutureTimestamp_Rejected(t *testing.T) {
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))

	futureTS := strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)
	req.Header.Set(signing.HeaderNodeName, "node-a")
	req.Header.Set(signing.HeaderTimestamp, futureTS)
	msg := "node-a:" + futureTS + ":" + signing.SHA256Hex(body)
	req.Header.Set(signing.HeaderSignature, signing.HMACSHA256("secret", msg))

	_, err := signing.Verify(req, "secret")
	if err == nil {
		t.Error("expected error for future timestamp (replay attack)")
	}
}

func TestVerify_TamperedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"original":true}`))
	signing.SignRequest(req, "node-a", "secret")

	// Подменяем тело после подписи
	req.Body = io.NopCloser(bytes.NewBufferString(`{"tampered":true}`))

	_, err := signing.Verify(req, "secret")
	if err == nil {
		t.Error("expected error for tampered body")
	}
}

func TestVerify_InvalidTimestampFormat(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	req.Header.Set(signing.HeaderNodeName, "node-a")
	req.Header.Set(signing.HeaderTimestamp, "not-a-number")
	req.Header.Set(signing.HeaderSignature, "somesig")

	_, err := signing.Verify(req, "secret")
	if err == nil {
		t.Error("expected error for invalid timestamp format")
	}
}

// — HMACSHA256 и SHA256Hex ─────────────────────────────────────────────

func TestHMACSHA256_Deterministic(t *testing.T) {
	sig1 := signing.HMACSHA256("secret", "message")
	sig2 := signing.HMACSHA256("secret", "message")
	if sig1 != sig2 {
		t.Error("HMAC should be deterministic")
	}
}

func TestHMACSHA256_DifferentInputs(t *testing.T) {
	sig1 := signing.HMACSHA256("secret", "message1")
	sig2 := signing.HMACSHA256("secret", "message2")
	if sig1 == sig2 {
		t.Error("different messages should produce different HMACs")
	}
}

func TestSHA256Hex_EmptyInput(t *testing.T) {
	h := signing.SHA256Hex(nil)
	if h == "" {
		t.Error("SHA256 of empty should not be empty string")
	}
	// SHA256 пустой строки — известное значение
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if h != expected {
		t.Errorf("SHA256(empty) = %q, want %q", h, expected)
	}
}

func TestSignThenVerify_RoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		node   string
		secret string
	}{
		{"normal", `{"type":"post.created"}`, "node-a", "secret-32-chars-long-enough!!"},
		{"empty body", "", "node-b", "another-secret-32-chars-long!!"},
		{"unicode", `{"content":"привет мир"}`, "node-c", "third-secret-32-chars-long!!!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tc.body))
			if err := signing.SignRequest(req, tc.node, tc.secret); err != nil {
				t.Fatalf("sign error: %v", err)
			}
			got, err := signing.Verify(req, tc.secret)
			if err != nil {
				t.Fatalf("verify error: %v", err)
			}
			if string(got) != tc.body {
				t.Errorf("body after round trip = %q, want %q", got, tc.body)
			}
		})
	}
}
