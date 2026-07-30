// Package middleware provides HTTP middleware for the TaskFlow API.
//
// The x402 middleware implements the x402 protocol (https://x402.org)
// for HTTP 402 Payment Required.
package middleware

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// ─────────────────────────────────────────────────────────────
// EIP-191 ECDSA Signature Verifier
// ─────────────────────────────────────────────────────────────

// signerFields is used for deterministic JSON serialization of the
// payment payload minus the signature. The struct field order
// determines the JSON key order, producing a canonical message.
type signerFields struct {
	Scheme    string `json:"scheme"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Network   string `json:"network"`
	RequestID string `json:"requestId"`
	Sender    string `json:"sender"`
}

// VerifyX402Signature verifies that the signature in the payment
// payload was produced by the claimed sender over the canonical
// JSON representation of the non-signature fields, using EIP-191
// (Ethereum Signed Message) signing.
//
// Steps:
//  1. Serialize the payload fields (excluding Signature) to
//     deterministic JSON via signerFields.
//  2. Compute the EIP-191 hash:
//     keccak256("\x19Ethereum Signed Message:\n" + len(json) + json)
//  3. Decode the hex-encoded 65-byte [R, S, V] signature.
//  4. Recover the signer's public key via Ecrecover.
//  5. Derive the Ethereum address and compare with payload.Sender.
//
// Returns true if the recovered address matches the sender (case-insensitive).
func VerifyX402Signature(payload PaymentPayload) bool {
	// ── 1. Build canonical message ──────────────────────────
	msgBytes, err := json.Marshal(signerFields{
		Scheme:    payload.Scheme,
		Amount:    payload.Amount,
		Currency:  payload.Currency,
		Network:   payload.Network,
		RequestID: payload.RequestID,
		Sender:    payload.Sender,
	})
	if err != nil {
		slog.Error("x402: failed to marshal signer fields", "error", err)
		return false
	}

	// ── 2. Apply EIP-191 prefix and hash ────────────────────
	// Format: "\x19Ethereum Signed Message:\n" + len(message) + message
	eip191Msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msgBytes), msgBytes)
	hash := crypto.Keccak256Hash([]byte(eip191Msg))

	// ── 3. Decode the hex signature ─────────────────────────
	sigHex := strings.TrimPrefix(payload.Signature, "0x")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		slog.Warn("x402: failed to decode signature hex", "error", err)
		return false
	}

	if len(sigBytes) != 65 {
		slog.Warn("x402: invalid signature length", "got", len(sigBytes), "want", 65)
		return false
	}

	// ── 4. Recover the signer's public key ──────────────────
	// crypto.Ecrecover expects a 65-byte [R, S, V] signature.
	// Note: go-ethereum's crypto.Sign produces V as 0 or 1, and
	// Ecrecover expects V as 27 or 28 (or 0/1 — it handles both).
	pubKeyBytes, err := crypto.Ecrecover(hash.Bytes(), sigBytes)
	if err != nil {
		slog.Error("x402: ecrecover failed", "error", err)
		return false
	}

	// ── 5. Derive address and compare ───────────────────────
	// Ecrecover returns the uncompressed public key (65 bytes)
	// starting with 0x04. We need to unmarshal it to get the
	// ECDSA public key, then derive the address.
	pubKey, err := crypto.UnmarshalPubkey(pubKeyBytes)
	if err != nil {
		slog.Error("x402: failed to unmarshal public key", "error", err)
		return false
	}

	recoveredAddr := crypto.PubkeyToAddress(*pubKey)

	// Compare case-insensitively (addresses are 0x-prefixed hex)
	wantAddr := common.HexToAddress(payload.Sender)
	return recoveredAddr == wantAddr
}
