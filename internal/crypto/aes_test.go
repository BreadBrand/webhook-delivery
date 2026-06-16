package crypto_test

import (
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

	c1, _ := crypto.Encrypt(key, plaintext)
	c2, _ := crypto.Encrypt(key, plaintext)
	if c1 == c2 {
		t.Error("two encryptions of same plaintext should not be identical (nonce must be random)")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1

	ct, _ := crypto.Encrypt(key1, []byte("hello"))
	_, err := crypto.Decrypt(key2, ct)
	if err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}
