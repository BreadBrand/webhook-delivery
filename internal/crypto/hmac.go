package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign returns "sha256=<hex>" HMAC-SHA256 of body using secret.
// body must be the exact []byte that will be sent as the HTTP request body —
// never re-marshal the payload after calling Sign.
func Sign(body, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
