package middleware

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// ── Test: no payment → 402 ───────────────────────────────────

func TestX402Middleware_NoPayment_Returns402(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	// Should get 402
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402, got %d", rec.Code)
	}

	// Must include PAYMENT-REQUIRED header
	if rec.Header().Get("PAYMENT-REQUIRED") == "" {
		t.Error("expected PAYMENT-REQUIRED header, got empty")
	}

	// Body should contain payment_required details
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] != "payment_required" {
		t.Errorf("expected error=payment_required, got %v", body["error"])
	}
	if body["payment_required"] == nil {
		t.Error("expected payment_required object in body")
	}

	// Should NOT have PAYMENT-RESPONSE header (payment never started)
	if rec.Header().Get("PAYMENT-RESPONSE") != "" {
		t.Error("expected no PAYMENT-RESPONSE header for initial 402")
	}
}

// ── Test: valid ECDSA-signed payment → 200 ───────────────────

func TestX402Middleware_ValidPayment_Proceeds(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	// Generate a real ECDSA key pair for signing
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	senderAddr := crypto.PubkeyToAddress(privKey.PublicKey).Hex()

	payload := PaymentPayload{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: "POST /api/tasks",
		Sender:    senderAddr,
	}

	// Sign the payload with the private key
	signPayload(&payload, privKey, t)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", base64EncodeJSON(payload))
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	// Should get 200
	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	// Must include PAYMENT-RESPONSE header with settlement info
	srHeader := rec.Header().Get("PAYMENT-RESPONSE")
	if srHeader == "" {
		t.Fatal("expected PAYMENT-RESPONSE header, got empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(srHeader)
	if err != nil {
		t.Fatalf("PAYMENT-RESPONSE is not valid base64: %v", err)
	}

	var sr SettlementResponse
	if err := json.Unmarshal(decoded, &sr); err != nil {
		t.Fatalf("PAYMENT-RESPONSE is not valid JSON: %v", err)
	}
	if sr.Status != "settled" {
		t.Errorf("expected settlement status=settled, got %q", sr.Status)
	}
	if !strings.HasPrefix(sr.TransactionID, "mock-tx-") {
		t.Errorf("expected transactionId starting with mock-tx-, got %q", sr.TransactionID)
	}
}

// ── Test: invalid ECDSA signature → 402 ──────────────────────

func TestX402Middleware_InvalidSignature_Returns402(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with invalid payment")
	})

	// Generate a real key to get a valid sender address
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	senderAddr := crypto.PubkeyToAddress(privKey.PublicKey).Hex()

	// Use a DIFFERENT key to sign → signature won't match sender
	wrongKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate wrong key: %v", err)
	}

	payload := PaymentPayload{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: "POST /api/tasks",
		Sender:    senderAddr,
	}

	// Sign with wrongKey — the recovered address won't match senderAddr
	signPayload(&payload, wrongKey, t)

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", base64EncodeJSON(payload))
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402, got %d", rec.Code)
	}

	// Should have PAYMENT-RESPONSE indicating failure
	srHeader := rec.Header().Get("PAYMENT-RESPONSE")
	if srHeader == "" {
		t.Fatal("expected PAYMENT-RESPONSE header with failure details")
	}

	decoded, _ := base64.StdEncoding.DecodeString(srHeader)
	var sr SettlementResponse
	_ = json.Unmarshal(decoded, &sr)
	if sr.Status != "failed" {
		t.Errorf("expected settlement status=failed, got %q", sr.Status)
	}
	if sr.Error != "invalid_signature" {
		t.Errorf("expected error=invalid_signature, got %q", sr.Error)
	}
}

// ── Test: malformed PAYMENT-SIGNATURE → 402 ──────────────────

func TestX402Middleware_BadEncoding_Returns402(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with bad encoding")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", "this-is-not-base64!!!")
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402, got %d", rec.Code)
	}
}

// ── Test: GET /projects/{id}/tasks route ─────────────────────

func TestX402Middleware_ProjectTasks_RequiresPayment(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tasks":[]}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/projects/proj-123/tasks", nil)
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402 for unpaid project tasks request, got %d", rec.Code)
	}
}

// ── Test: amount mismatch → 402 ─────────────────────────────

func TestX402Middleware_AmountMismatch_Returns402(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with wrong amount")
	})

	payload := PaymentPayload{
		Scheme:    "exact",
		Amount:    "$0.002", // wrong amount — config wants $0.001
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: "POST /api/tasks",
		Signature: "mock-valid-signature",
		Sender:    "0xSender",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", base64EncodeJSON(payload))
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402 for amount mismatch, got %d", rec.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "validation_failed" {
		t.Errorf("expected error=validation_failed, got %v", body["error"])
	}
}

// ── Test: currency mismatch → 402 ────────────────────────────

func TestX402Middleware_CurrencyMismatch_Returns402(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with wrong currency")
	})

	payload := PaymentPayload{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "DAI", // wrong currency — config wants USDC
		Network:   "eip155:84532",
		RequestID: "POST /api/tasks",
		Signature: "mock-valid-signature",
		Sender:    "0xSender",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", base64EncodeJSON(payload))
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402 for currency mismatch, got %d", rec.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "validation_failed" {
		t.Errorf("expected error=validation_failed, got %v", body["error"])
	}
}

// ── Test: network mismatch → 402 ─────────────────────────────

func TestX402Middleware_NetworkMismatch_Returns402(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with wrong network")
	})

	payload := PaymentPayload{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:1", // wrong network — config wants eip155:84532
		RequestID: "POST /api/tasks",
		Signature: "mock-valid-signature",
		Sender:    "0xSender",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", base64EncodeJSON(payload))
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402 for network mismatch, got %d", rec.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "validation_failed" {
		t.Errorf("expected error=validation_failed, got %v", body["error"])
	}
}

// ── Test: empty sender → 402 ─────────────────────────────────

func TestX402Middleware_EmptySender_Returns402(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with empty sender")
	})

	payload := PaymentPayload{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: "POST /api/tasks",
		Signature: "mock-valid-signature",
		Sender:    "", // empty — required field
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", base64EncodeJSON(payload))
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402 for empty sender, got %d", rec.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "validation_failed" {
		t.Errorf("expected error=validation_failed, got %v", body["error"])
	}
}

// ── Test: unsupported scheme → 402 ───────────────────────────

func TestX402Middleware_UnsupportedScheme_Returns402(t *testing.T) {
	mw := newTestMiddleware()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with bad scheme")
	})

	payload := PaymentPayload{
		Scheme:    "per-call", // not "exact"
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: "POST /api/tasks",
		Signature: "mock-valid-signature",
		Sender:    "0xSender",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", base64EncodeJSON(payload))
	rec := httptest.NewRecorder()

	mw.Handler(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402 for unsupported scheme, got %d", rec.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "validation_failed" {
		t.Errorf("expected error=validation_failed, got %v", body["error"])
	}
}

// ── Helpers ───────────────────────────────────────────────────

// newTestMiddleware creates an X402Middleware with test configuration
// using real ECDSA signature verification (no mock mode).
func newTestMiddleware() *X402Middleware {
	return New(X402Config{
		Price:    "$0.001",
		Currency: "USDC",
		Network:  "eip155:84532",
		PayTo:    "0xTaskFlowPayToAddress",
	})
}

// signPayload signs the non-signature fields of the payment payload
// using the provided ECDSA private key, following EIP-191 signing.
// It sets the payload.Signature field to the 0x-prefixed hex signature.
func signPayload(payload *PaymentPayload, privKey *ecdsa.PrivateKey, t testing.TB) {
	t.Helper()

	msg, err := json.Marshal(signerFields{
		Scheme:    payload.Scheme,
		Amount:    payload.Amount,
		Currency:  payload.Currency,
		Network:   payload.Network,
		RequestID: payload.RequestID,
		Sender:    payload.Sender,
	})
	if err != nil {
		t.Fatalf("failed to marshal signer fields: %v", err)
	}

	// Apply EIP-191 prefix: \x19Ethereum Signed Message:\n + len + message
	eip191Msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hash := crypto.Keccak256Hash([]byte(eip191Msg))

	sig, err := crypto.Sign(hash.Bytes(), privKey)
	if err != nil {
		t.Fatalf("failed to sign payload: %v", err)
	}

	payload.Signature = "0x" + hex.EncodeToString(sig)
}
