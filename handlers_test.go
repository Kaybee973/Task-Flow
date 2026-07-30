package main

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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"tessst/service"
	"tessst/storage"
)

// ── Test Setup ─────────────────────────────────────────────────

// testHarness holds all dependencies needed by the integration tests.
type testHarness struct {
	server *httptest.Server
	key    *ecdsa.PrivateKey
	sender string
}

// newTestHarness creates a fully wired test server with a real ECDSA
// key that can sign x402 payment payloads.
func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	// Generate a real ECDSA key for signing
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	sender := crypto.PubkeyToAddress(key.PublicKey).Hex()

	// Wire dependencies like main() does
	store := storage.NewInMemoryTaskStore()
	svc := service.NewTaskService(store)
	api := newAPIHandlers(svc)

	// Use the canonical router builder — same as main().
	// This exercises the real route wiring including /healthz.
	server := httptest.NewServer(newRouter(api, store))
	t.Cleanup(server.Close)

	return &testHarness{
		server: server,
		key:    key,
		sender: sender,
	}
}

// signedHeader builds a PAYMENT-SIGNATURE header value by signing
// the payment payload fields with the harness's ECDSA key.
func (h *testHarness) signedHeader(t *testing.T, requestID string) string {
	t.Helper()

	// Build the canonical message (same as the server's verifier.go)
	msg, err := json.Marshal(struct {
		Scheme    string `json:"scheme"`
		Amount    string `json:"amount"`
		Currency  string `json:"currency"`
		Network   string `json:"network"`
		RequestID string `json:"requestId"`
		Sender    string `json:"sender"`
	}{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: requestID,
		Sender:    h.sender,
	})
	if err != nil {
		t.Fatalf("failed to marshal signer fields: %v", err)
	}

	// EIP-191 signing
	eip191 := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hash := crypto.Keccak256Hash([]byte(eip191))
	sig, err := crypto.Sign(hash.Bytes(), h.key)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	payload := map[string]interface{}{
		"scheme":    "exact",
		"amount":    "$0.001",
		"currency":  "USDC",
		"network":   "eip155:84532",
		"requestId": requestID,
		"signature": "0x" + common.Bytes2Hex(sig),
		"sender":    h.sender,
	}

	b, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(b)
}

// signFields is used for deterministic signing in tests only.
// It mirrors middleware.signerFields but is unexported.
type signFields struct {
	Scheme    string `json:"scheme"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Network   string `json:"network"`
	RequestID string `json:"requestId"`
	Sender    string `json:"sender"`
}

// ── Tests: Unpaid requests → 402 ──────────────────────────────

func TestHandler_UnpaidRequest_Returns402(t *testing.T) {
	h := newTestHarness(t)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"POST /api/tasks", "POST", "/api/tasks", `{"title":"x","project_id":"p"}`},
		{"GET /api/projects/p1/tasks", "GET", "/api/projects/p1/tasks", ""},
		{"PUT /api/tasks/task-1", "PUT", "/api/tasks/task-1", `{"status":"done"}`},
		{"DELETE /api/tasks/task-1", "DELETE", "/api/tasks/task-1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}

			rec := httptest.NewRecorder()
			h.server.Config.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusPaymentRequired {
				t.Fatalf("expected HTTP 402, got %d", rec.Code)
			}

			// Verify PAYMENT-REQUIRED header is present
			if rec.Header().Get("PAYMENT-REQUIRED") == "" {
				t.Error("expected PAYMENT-REQUIRED header")
			}

			// Verify error in body
			var body map[string]interface{}
			json.NewDecoder(rec.Body).Decode(&body)
			if body["error"] != "payment_required" {
				t.Errorf("expected error=payment_required, got %v", body["error"])
			}
		})
	}
}

// ── Tests: Invalid signature → 402 ────────────────────────────

func TestHandler_InvalidSignature_Returns402(t *testing.T) {
	h := newTestHarness(t)

	// Sign with a DIFFERENT key than the sender
	wrongKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	// Use the harness's sender but sign with wrongKey
	msg, _ := json.Marshal(signFields{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: "request-1",
		Sender:    h.sender,
	})
	eip191 := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hash := crypto.Keccak256Hash([]byte(eip191))
	sig, _ := crypto.Sign(hash.Bytes(), wrongKey)

	payload := map[string]interface{}{
		"scheme":    "exact",
		"amount":    "$0.001",
		"currency":  "USDC",
		"network":   "eip155:84532",
		"requestId": "request-1",
		"signature": "0x" + hex.EncodeToString(sig),
		"sender":    h.sender,
	}
	b, _ := json.Marshal(payload)
	badSig := base64.StdEncoding.EncodeToString(b)

	req := httptest.NewRequest("POST", "/api/tasks",
		strings.NewReader(`{"title":"T","project_id":"p"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PAYMENT-SIGNATURE", badSig)

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected HTTP 402, got %d", rec.Code)
	}

	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "invalid_signature" {
		t.Errorf("expected error=invalid_signature, got %v", body["error"])
	}
}

// ── Tests: Full CRUD lifecycle (signed) ───────────────────────

func TestHandler_CreateTask_Signed_Success(t *testing.T) {
	h := newTestHarness(t)

	body := `{"title":"Build feature","description":"Implement it","project_id":"proj-1","assignees":["0xAlice"]}`
	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "create-test"))

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201, got %d", rec.Code)
	}

	var task storage.Task
	if err := json.NewDecoder(rec.Body).Decode(&task); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if task.Title != "Build feature" {
		t.Errorf("expected title %q, got %q", "Build feature", task.Title)
	}
	if task.Status != "open" {
		t.Errorf("expected status \"open\", got %q", task.Status)
	}
	if task.ID == "" {
		t.Error("expected non-empty ID")
	}
	if task.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestHandler_GetProjectTasks_Signed_ReturnsTasks(t *testing.T) {
	h := newTestHarness(t)

	// Pre-create a task using the signed endpoint
	createBody := `{"title":"Task A","description":"First","project_id":"proj-list","assignees":[]}`
	createReq := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "create-for-list"))
	createRec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("failed to create task: %d", createRec.Code)
	}

	// Now list tasks for the project
	req := httptest.NewRequest("GET", "/api/projects/proj-list/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "list-test"))
	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}

	var resp struct {
		Ok        bool           `json:"ok"`
		ProjectID string         `json:"project_id"`
		Tasks     []storage.Task `json:"tasks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.ProjectID != "proj-list" {
		t.Errorf("expected project_id=%q, got %q", "proj-list", resp.ProjectID)
	}
	if len(resp.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
	}
	if resp.Tasks[0].Title != "Task A" {
		t.Errorf("expected title %q, got %q", "Task A", resp.Tasks[0].Title)
	}
}

func TestHandler_UpdateTask_Signed_Success(t *testing.T) {
	h := newTestHarness(t)

	// Create a task first
	createBody := `{"title":"Original","description":"Original desc","project_id":"proj-upd","assignees":["0xBob"]}`
	createReq := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "create-for-upd"))
	createRec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(createRec, createReq)

	var created storage.Task
	json.NewDecoder(createRec.Body).Decode(&created)

	// Partial update: status + description only
	updateBody := `{"status":"in-progress","description":"Updated description"}`
	req := httptest.NewRequest("PUT", "/api/tasks/"+created.ID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "update-test"))

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated storage.Task
	json.NewDecoder(rec.Body).Decode(&updated)

	if updated.Status != "in-progress" {
		t.Errorf("expected status \"in-progress\", got %q", updated.Status)
	}
	if updated.Description != "Updated description" {
		t.Errorf("expected description updated, got %q", updated.Description)
	}
	// Preserved fields
	if updated.Title != "Original" {
		t.Errorf("expected title preserved, got %q", updated.Title)
	}
	if len(updated.Assignees) != 1 || updated.Assignees[0] != "0xBob" {
		t.Errorf("expected assignees preserved, got %v", updated.Assignees)
	}
	// UpdatedAt should be refreshed
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Error("expected UpdatedAt to be after creation")
	}
}

func TestHandler_DeleteTask_Signed_Success(t *testing.T) {
	h := newTestHarness(t)

	// Create a task first
	createBody := `{"title":"Delete me","project_id":"proj-del"}`
	createReq := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "create-for-del"))
	createRec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(createRec, createReq)

	var created storage.Task
	json.NewDecoder(createRec.Body).Decode(&created)

	// Delete
	req := httptest.NewRequest("DELETE", "/api/tasks/"+created.ID, nil)
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "del-test"))
	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Ok     bool   `json:"ok"`
		Status string `json:"status"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.Status != "deleted" {
		t.Errorf("expected status=\"deleted\", got %q", resp.Status)
	}

	// Verify task is gone (should get 404 on update)
	updateReq := httptest.NewRequest("PUT", "/api/tasks/"+created.ID,
		strings.NewReader(`{"status":"done"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "verify-del"))
	updateRec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after deletion, got %d", updateRec.Code)
	}
}

// ── Tests: Error responses ────────────────────────────────────

func TestHandler_CreateTask_EmptyTitle_Returns422(t *testing.T) {
	h := newTestHarness(t)

	body := `{"project_id":"proj-1"}` // missing title
	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "err-test"))

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", rec.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp["error"] != "creation_failed" {
		t.Errorf("expected error=creation_failed, got %v", errResp["error"])
	}
}

func TestHandler_CreateTask_EmptyProjectID_Returns422(t *testing.T) {
	h := newTestHarness(t)

	body := `{"title":"Task without project"}`
	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "err-test2"))

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", rec.Code)
	}
}

func TestHandler_CreateTask_BadJSON_Returns400(t *testing.T) {
	h := newTestHarness(t)

	body := `{invalid json}`
	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "bad-json"))

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d", rec.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp["error"] != "bad_request" {
		t.Errorf("expected error=bad_request, got %v", errResp["error"])
	}
}

func TestHandler_UpdateTask_NotFound_Returns404(t *testing.T) {
	h := newTestHarness(t)

	body := `{"status":"done"}`
	req := httptest.NewRequest("PUT", "/api/tasks/non-existent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "404-test"))

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got %d", rec.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rec.Body).Decode(&errResp)
	if errResp["error"] != "not_found" {
		t.Errorf("expected error=not_found, got %v", errResp["error"])
	}
}

func TestHandler_DeleteTask_NotFound_Returns404(t *testing.T) {
	h := newTestHarness(t)

	req := httptest.NewRequest("DELETE", "/api/tasks/non-existent", nil)
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "del-404"))

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got %d", rec.Code)
	}
}

func TestHandler_GetProjectTasks_EmptyList(t *testing.T) {
	h := newTestHarness(t)

	req := httptest.NewRequest("GET", "/api/projects/empty-project/tasks", nil)
	req.Header.Set("PAYMENT-SIGNATURE", h.signedHeader(t, "empty-list"))

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}

	var resp struct {
		Tasks []storage.Task `json:"tasks"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Tasks == nil {
		t.Error("expected non-nil empty tasks array, got nil")
	}
	if len(resp.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(resp.Tasks))
	}
}

// ── Tests: Health endpoint ────────────────────────────────────

func TestHandler_Healthz_Returns200WithStatus(t *testing.T) {
	h := newTestHarness(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
	if body["service"] != "taskflow" {
		t.Errorf("expected service=taskflow, got %v", body["service"])
	}
	if body["uptime"] == nil || body["uptime"] == "" {
		t.Error("expected non-empty uptime")
	}

	db, ok := body["database"].(map[string]interface{})
	if !ok {
		t.Fatal("expected database field to be an object")
	}
	if db["status"] != "connected" {
		t.Errorf("expected database.status=connected, got %v", db["status"])
	}
}

func TestHandler_Health_AlsoWorks(t *testing.T) {
	h := newTestHarness(t)

	// /health should work the same as /healthz
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 on /health, got %d", rec.Code)
	}
}

// ── Tests: X-Request-Id header ────────────────────────────────

func TestHandler_RequestID_SetOnAllEndpoints(t *testing.T) {
	h := newTestHarness(t)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"GET /healthz (no auth)", "GET", "/healthz", ""},
		{"GET /health (no auth)", "GET", "/health", ""},
		{"POST /api/tasks unpaid", "POST", "/api/tasks", `{"title":"x"}`},
		{"GET /api/projects/p1/tasks unpaid", "GET", "/api/projects/p1/tasks", ""},
		{"PUT /api/tasks/task-x unpaid", "PUT", "/api/tasks/task-x", `{"status":"done"}`},
		{"DELETE /api/tasks/task-x unpaid", "DELETE", "/api/tasks/task-x", ""},
		{"GET / (root redirect)", "GET", "/", ""},
		{"GET /nonexistent (404)", "GET", "/nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}

			rec := httptest.NewRecorder()
			h.server.Config.Handler.ServeHTTP(rec, req)

			reqID := rec.Header().Get("X-Request-Id")
			if reqID == "" {
				t.Error("expected non-empty X-Request-Id header")
			}
			if len(reqID) < 8 {
				t.Errorf("expected X-Request-Id to be at least 8 chars, got %q (len=%d)", reqID, len(reqID))
			}
		})
	}
}

func TestHandler_RequestID_PropagatesFromHeader(t *testing.T) {
	h := newTestHarness(t)

	// When a client sends an X-Request-Id header, the server should
	// echo it back rather than generating a new one.
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Request-Id", "client-provided-id-123")

	rec := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec, req)

	got := rec.Header().Get("X-Request-Id")
	if got != "client-provided-id-123" {
		t.Errorf("expected X-Request-Id to echo client value, got %q", got)
	}
}

func TestHandler_RequestID_UniquePerRequest(t *testing.T) {
	h := newTestHarness(t)

	// Two sequential requests should get different IDs.
	req1 := httptest.NewRequest("GET", "/healthz", nil)
	rec1 := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec1, req1)
	id1 := rec1.Header().Get("X-Request-Id")

	req2 := httptest.NewRequest("GET", "/healthz", nil)
	rec2 := httptest.NewRecorder()
	h.server.Config.Handler.ServeHTTP(rec2, req2)
	id2 := rec2.Header().Get("X-Request-Id")

	if id1 == "" || id2 == "" {
		t.Fatal("expected non-empty request IDs")
	}
	if id1 == id2 {
		t.Errorf("expected unique request IDs, but both were %q", id1)
	}
}
