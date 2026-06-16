package crypto_test

import (
	"strings"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/crypto"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plaintext := []byte("super secret signing key sk_abc123")

	ciphertext, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	key := make([]byte, 32)
	plaintext := []byte("hello")

	c1, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt c1: %v", err)
	}
	c2, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt c2: %v", err)
	}
	if c1 == c2 {
		t.Error("two encryptions of same plaintext should not be identical (nonce must be random)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1

	ct, err := crypto.Encrypt(key1, []byte("hello"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = crypto.Decrypt(key2, ct)
	if err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}

func TestEncryptWrongKeyLength(t *testing.T) {
	key := make([]byte, 31) // AES requires 16, 24, or 32 bytes
	_, err := crypto.Encrypt(key, []byte("hello"))
	if err == nil {
		t.Error("expected error for wrong-length key")
	}
}

func TestDecryptWrongKeyLength(t *testing.T) {
	key := make([]byte, 31)
	_, err := crypto.Decrypt(key, "dGVzdA==")
	if err == nil {
		t.Error("expected error for wrong-length key")
	}
}

func TestDecryptBadBase64(t *testing.T) {
	key := make([]byte, 32)
	_, err := crypto.Decrypt(key, "not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64 input")
	}
	if !strings.Contains(err.Error(), "base64 decode") {
		t.Errorf("expected 'base64 decode' in error, got %q", err.Error())
	}
}

func TestDecryptTooShort(t *testing.T) {
	key := make([]byte, 32)
	// base64 of 3 bytes — well under the 12-byte GCM nonce minimum
	_, err := crypto.Decrypt(key, "AAAA")
	if err == nil {
		t.Error("expected error for ciphertext shorter than nonce")
	}
	if !strings.Contains(err.Error(), "ciphertext too short") {
		t.Errorf("expected 'ciphertext too short' in error, got %q", err.Error())
	}
}
