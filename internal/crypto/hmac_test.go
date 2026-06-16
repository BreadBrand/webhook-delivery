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

// TestSignKnownAnswer verifies the exact HMAC-SHA256 output against a
// pre-computed value. Derivation:
//
//	echo -n '{"id":"1","type":"order.created"}' | openssl dgst -sha256 -hmac 'mysecret'
//	=> 47a687046c23b4b7db37e5ecb9f9e25c2df058c681c1408f94aaca6cb9fa326c
func TestSignKnownAnswer(t *testing.T) {
	body := []byte(`{"id":"1","type":"order.created"}`)
	secret := []byte("mysecret")
	const want = "sha256=47a687046c23b4b7db37e5ecb9f9e25c2df058c681c1408f94aaca6cb9fa326c"
	got := crypto.Sign(body, secret)
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
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
