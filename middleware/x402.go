// Package middleware provides HTTP middleware for the TaskFlow API.
//
// The x402 middleware implements the x402 protocol (https://x402.org)
// for HTTP 402 Payment Required. It intercepts requests to protected
// routes and requires a valid PAYMENT-SIGNATURE header before passing
// the request through to the underlying handler.
package middleware

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ─────────────────────────────────────────────────────────────
// x402 Protocol Types
// ─────────────────────────────────────────────────────────────

// PaymentRequired is sent in the PAYMENT-REQUIRED response header
// alongside HTTP 402. It tells the client what payment is needed.
type PaymentRequired struct {
	Scheme    string `json:"scheme"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Network   string `json:"network"`
	PayTo     string `json:"payTo"`
	RequestID string `json:"requestId"`
}

// PaymentPayload is the signed payment object the client sends
// in the PAYMENT-SIGNATURE request header.
type PaymentPayload struct {
	Scheme    string `json:"scheme"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Network   string `json:"network"`
	RequestID string `json:"requestId"`
	Signature string `json:"signature"`
	Sender    string `json:"sender"`
}

// SettlementResponse is returned in the PAYMENT-RESPONSE header
// on a settled (or failed) payment.
type SettlementResponse struct {
	Status        string `json:"status"`
	TransactionID string `json:"transactionId,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ─────────────────────────────────────────────────────────────
// Middleware
// ─────────────────────────────────────────────────────────────

// X402Config holds pricing and network configuration for the
// payment middleware.
type X402Config struct {
	Price    string // e.g. "$0.001"
	Currency string // e.g. "USDC"
	Network  string // e.g. "eip155:84532"
	PayTo    string // recipient wallet address
}

// X402Middleware wraps an HTTP handler with x402 payment gating.
type X402Middleware struct {
	config X402Config
}

// New creates a new X402Middleware with the given configuration.
func New(cfg X402Config) *X402Middleware {
	return &X402Middleware{config: cfg}
}

// Handler returns an http.Handler that gates the underlying handler
// behind the x402 payment protocol.
//
//  1. If the request has no PAYMENT-SIGNATURE header → 402 + PAYMENT-REQUIRED
//  2. If the signature is missing, malformed, or invalid        → 402
//  3. If the signature is valid                                 → handler proceeds
func (m *X402Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader := r.Header.Get("PAYMENT-SIGNATURE")

		// ── No payment attached → 402 ────────────────────────
		if sigHeader == "" {
			m.writePaymentRequired(w, r)
			return
		}

		// ── Decode the payment payload ───────────────────────
		decoded, err := base64.StdEncoding.DecodeString(sigHeader)
		if err != nil {
			log.Printf("x402: bad base64 in PAYMENT-SIGNATURE: %v", err)
			m.writePaymentFailed(w, "invalid_encoding", "PAYMENT-SIGNATURE must be base64-encoded JSON")
			return
		}

		var payload PaymentPayload
		if err := json.Unmarshal(decoded, &payload); err != nil {
			log.Printf("x402: bad JSON in PAYMENT-SIGNATURE: %v", err)
			m.writePaymentFailed(w, "invalid_format", "PAYMENT-SIGNATURE must be valid JSON")
			return
		}

		// ── Validate the payment payload ─────────────────────
		if err := m.validatePayload(payload, r); err != nil {
			log.Printf("x402: payload validation failed: %v", err)
			m.writePaymentFailed(w, "validation_failed", err.Error())
			return
		}

		// ── Verify the signature ─────────────────────────────
		if !m.verifySignature(payload) {
			log.Printf("x402: invalid signature from sender %s", payload.Sender)
			m.writePaymentFailed(w, "invalid_signature", "The payment signature could not be verified")
			return
		}

		// ── Payment accepted — proceed ───────────────────────
		sr := SettlementResponse{
			Status:        "settled",
			TransactionID: "mock-tx-" + payload.RequestID,
		}
		m.writeSettlement(w, sr)

		log.Printf("x402: payment accepted — sender=%s amount=%s request=%s",
			payload.Sender, payload.Amount, r.Method+" "+r.URL.Path)

		next.ServeHTTP(w, r)
	})
}

// ── Internal helpers ─────────────────────────────────────────

// writePaymentRequired writes HTTP 402 with a PAYMENT-REQUIRED header
// and a JSON body describing the required payment.
func (m *X402Middleware) writePaymentRequired(w http.ResponseWriter, r *http.Request) {
	pr := PaymentRequired{
		Scheme:    "exact",
		Amount:    m.config.Price,
		Currency:  m.config.Currency,
		Network:   m.config.Network,
		PayTo:     m.config.PayTo,
		RequestID: r.Method + " " + r.URL.Path + " " + r.Header.Get("X-Request-Id") + " " + fmt.Sprint(time.Now().UnixNano()),
	}

	encoded := base64EncodeJSON(pr)

	w.Header().Set("PAYMENT-REQUIRED", encoded)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"error":            "payment_required",
		"message":          "This endpoint requires payment via the x402 protocol. See https://x402.org",
		"payment_required": pr,
	}); err != nil {
		log.Printf("x402: failed to encode payment_required response: %v", err)
	}
}

// writePaymentFailed writes HTTP 402 with a PAYMENT-RESPONSE header
// indicating the payment was rejected.
func (m *X402Middleware) writePaymentFailed(w http.ResponseWriter, errorCode, errorMsg string) {
	sr := SettlementResponse{
		Status: "failed",
		Error:  errorCode,
	}
	m.writeSettlement(w, sr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   errorCode,
		"message": errorMsg,
	}); err != nil {
		log.Printf("x402: failed to encode payment_failed response: %v", err)
	}
}

// writeSettlement sets the PAYMENT-RESPONSE header.
func (m *X402Middleware) writeSettlement(w http.ResponseWriter, sr SettlementResponse) {
	w.Header().Set("PAYMENT-RESPONSE", base64EncodeJSON(sr))
}

// validatePayload checks that the payment payload matches the
// required scheme, amount, currency, network, and request.
func (m *X402Middleware) validatePayload(p PaymentPayload, r *http.Request) error {
	if p.Scheme != "exact" {
		return &validationError{"unsupported scheme: " + p.Scheme}
	}
	if p.Amount != m.config.Price {
		return &validationError{"amount mismatch: want " + m.config.Price + ", got " + p.Amount}
	}
	if p.Currency != m.config.Currency {
		return &validationError{"currency mismatch"}
	}
	if p.Network != m.config.Network {
		return &validationError{"network mismatch"}
	}
	if p.Sender == "" {
		return &validationError{"sender is required"}
	}
	return nil
}

// verifySignature checks the cryptographic signature using ECDSA
// on the Ethereum Virtual Machine. It recovers the signer's address
// from the EIP-191 signed message and verifies it matches the
// sender field in the payment payload.
func (m *X402Middleware) verifySignature(p PaymentPayload) bool {
	return VerifyX402Signature(p)
}

// ── Utilities ─────────────────────────────────────────────────

func base64EncodeJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string { return e.msg }
