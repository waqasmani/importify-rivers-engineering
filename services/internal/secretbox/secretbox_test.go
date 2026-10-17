package secretbox

import (
	"bytes"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal([]byte("sk-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("sk-secret")) {
		t.Fatal("plaintext visible in ciphertext")
	}
	out, err := b.Open(sealed)
	if err != nil || string(out) != "sk-secret" {
		t.Fatalf("round trip failed: %v", err)
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := b.Open(sealed); err == nil {
		t.Fatal("tampering not detected")
	}
	other, _ := New(bytes.Repeat([]byte{8}, 32))
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := New([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}
