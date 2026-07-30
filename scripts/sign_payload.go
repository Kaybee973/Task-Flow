//go:build ignore

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type signerFields struct {
	Scheme    string `json:"scheme"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Network   string `json:"network"`
	RequestID string `json:"requestId"`
	Sender    string `json:"sender"`
}

type paymentPayload struct {
	Scheme    string   `json:"scheme"`
	Amount    string   `json:"amount"`
	Currency  string   `json:"currency"`
	Network   string   `json:"network"`
	RequestID string   `json:"requestId"`
	Signature string   `json:"signature"`
	Sender    string   `json:"sender"`
}

func main() {
	// Generate a real ECDSA key pair
	privKey, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	sender := crypto.PubkeyToAddress(privKey.PublicKey).Hex()
	privateKeyHex := common.Bytes2Hex(crypto.FromECDSA(privKey))

	requestID := "e2e-test-1"

	// Build the message to sign
	msg, _ := json.Marshal(signerFields{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: requestID,
		Sender:    sender,
	})

	// EIP-191 signing
	eip191 := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hash := crypto.Keccak256Hash([]byte(eip191))
	sig, err := crypto.Sign(hash.Bytes(), privKey)
	if err != nil {
		panic(err)
	}

	payload := paymentPayload{
		Scheme:    "exact",
		Amount:    "$0.001",
		Currency:  "USDC",
		Network:   "eip155:84532",
		RequestID: requestID,
		Signature: "0x" + common.Bytes2Hex(sig),
		Sender:    sender,
	}

	payloadJSON, _ := json.Marshal(payload)
	header := base64.StdEncoding.EncodeToString(payloadJSON)

	fmt.Printf("PRIVATE_KEY=0x%s\n", privateKeyHex)
	fmt.Printf("SENDER=%s\n", sender)
	fmt.Printf("PAYMENT_SIGNATURE=%s\n", header)
	fmt.Printf("CURL_HDR=PAYMENT-SIGNATURE: %s\n", header)

	// Build curl commands
	body := `{"title":"Build login page","description":"Implement OAuth2","project_id":"proj-e2e","assignees":["0xAlice","0xBob"]}`
	escapedBody := strings.ReplaceAll(body, "'", "'\\''")

	fmt.Printf("\n# ---- curl commands ----\n")
	fmt.Printf("echo '=== Create Task ==='\n")
	fmt.Printf("curl -s -w \"\\nHTTP_STATUS: %%{http_code}\\n\\n\" -X POST http://localhost:8080/api/tasks \\\n")
	fmt.Printf("  -H 'Content-Type: application/json' \\\n")
	fmt.Printf("  -H '%s' \\\n", fmt.Sprintf("PAYMENT-SIGNATURE: %s", header))
	fmt.Printf("  -d '%s'\n", escapedBody)

	fmt.Printf("\necho '=== List Project Tasks ==='\n")
	fmt.Printf("curl -s -w \"\\nHTTP_STATUS: %%{http_code}\\n\\n\" http://localhost:8080/api/projects/proj-e2e/tasks \\\n")
	fmt.Printf("  -H '%s'\n", fmt.Sprintf("PAYMENT-SIGNATURE: %s", header))

	fmt.Printf("\necho '=== Update Task (status → in-progress) ==='\n")
	fmt.Printf("curl -s -w \"\\nHTTP_STATUS: %%{http_code}\\n\\n\" -X PUT http://localhost:8080/api/tasks/task-1 \\\n")
	fmt.Printf("  -H 'Content-Type: application/json' \\\n")
	fmt.Printf("  -H '%s' \\\n", fmt.Sprintf("PAYMENT-SIGNATURE: %s", header))
	fmt.Printf("  -d '{\"status\":\"in-progress\",\"description\":\"Implement OAuth2 flow\"}'\n")

	fmt.Printf("\necho '=== Delete Task ==='\n")
	fmt.Printf("curl -s -w \"\\nHTTP_STATUS: %%{http_code}\\n\\n\" -X DELETE http://localhost:8080/api/tasks/task-1 \\\n")
	fmt.Printf("  -H '%s'\n", fmt.Sprintf("PAYMENT-SIGNATURE: %s", header))

	os.Exit(0)
}
