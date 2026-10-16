package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := HashPassword("a sufficiently long passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format %q", h)
	}
	if VerifyPassword("a sufficiently long passphrase", h) != nil {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword("a sufficiently long passphrasE", h) != ErrMismatch {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("a sufficiently long passphrase")
	if h == h2 {
		t.Fatal("hashes must be salted")
	}
}

func TestPasswordPolicy(t *testing.T) {
	for _, bad := range []string{"short", "aaaaaaaaaaaaaaaa", "password12345"} {
		if ValidatePassword(bad) == "" {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if msg := ValidatePassword("correct horse battery staple"); msg != "" {
		t.Errorf("good password rejected: %s", msg)
	}
}

func TestTokensAndPairingCodes(t *testing.T) {
	a, b := NewToken(PrefixAccess), NewToken(PrefixAccess)
	if a == b || !strings.HasPrefix(a, PrefixAccess) || len(a) < 40 {
		t.Fatalf("weak token %q", a)
	}
	if string(HashToken(a)) == string(HashToken(b)) {
		t.Fatal("hash collision")
	}
	code := NewPairingCode()
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("bad pairing code %q", code)
	}
	if NormalizePairingCode(" "+strings.ToLower(code)+" ") != strings.ReplaceAll(code, "-", "") {
		t.Fatal("normalization failed")
	}
}
