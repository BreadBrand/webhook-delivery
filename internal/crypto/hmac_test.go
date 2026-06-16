package crypto_test

import (
	"strings"
	"testing"

	"github.com/b2randon/webhook-delivery/internal/crypto"
)

func TestSignFormat(t *testing.T) {
	sig := crypto.Sign([]byte("body"), []byte("secret"))
	if !strings.HasPrefix(sig, "sha256=") {
		t.Errorf("signature must start with 'sha256=', got %q", sig)
	}
	if len(sig) != len("sha256=")+64 {
		t.Errorf("unexpected signature length: %d", len(sig))
	}
}

func TestSignDeterministic(t *testing.T) {
	body := []byte(`{"id":"1","type":"order.created"}`)
	secret := []byte("mysecret")
	if crypto.Sign(body, secret) != crypto.Sign(body, secret) {
		t.Error("same inputs must produce same signature")
	}
}

func TestSignDifferentBodyDifferentSig(t *testing.T) {
	secret := []byte("mysecret")
	s1 := crypto.Sign([]byte("body1"), secret)
	s2 := crypto.Sign([]byte("body2"), secret)
	if s1 == s2 {
		t.Error("different bodies must produce different signatures")
	}
}

func TestSignDifferentKeyDifferentSig(t *testing.T) {
	body := []byte("same body")
	s1 := crypto.Sign(body, []byte("key1"))
	s2 := crypto.Sign(body, []byte("key2"))
	if s1 == s2 {
		t.Error("different keys must produce different signatures")
	}
}
