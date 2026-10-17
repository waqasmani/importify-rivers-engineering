package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/config"
)

func TestSignMatchesIndependentHMAC(t *testing.T) {
	secret := NewSecret()
	if !strings.HasPrefix(secret, "whsec_") || len(secret) < 40 || NewSecret() == secret {
		t.Fatalf("weak secret %q", secret)
	}
	body := []byte(`{"id":"e1","type":"product.updated"}`)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("1700000000." + string(body)))
	want := hex.EncodeToString(m.Sum(nil))
	if got := Sign(secret, 1700000000, body); got != want {
		t.Fatalf("signature %s, want %s", got, want)
	}
	header := SignatureHeader([]string{secret, "whsec_old"}, 1700000000, body)
	if !strings.HasPrefix(header, "v1="+want+",v1=") {
		t.Fatalf("header %q", header)
	}
	now := time.Unix(1700000100, 0)
	if !Verify(secret, header, 1700000000, body, now, 5*time.Minute) || !Verify("whsec_old", header, 1700000000, body, now, 5*time.Minute) {
		t.Fatal("verify rejected a valid signature")
	}
	if Verify(secret, header, 1700000000, append(body, ' '), now, 5*time.Minute) || Verify("whsec_other", header, 1700000000, body, now, 5*time.Minute) {
		t.Fatal("verify accepted a wrong body or secret")
	}
	if Verify(secret, header, 1700000000, body, now.Add(time.Hour), 5*time.Minute) {
		t.Fatal("verify accepted an old timestamp")
	}
}

func TestBackoffIsExponentialCappedAndJittered(t *testing.T) {
	cfg := &config.Config{WebhookRetryBase: time.Minute, WebhookRetryMax: time.Hour}
	for attempt, base := range map[int]time.Duration{1: time.Minute, 2: 2 * time.Minute, 3: 4 * time.Minute, 7: time.Hour, 20: time.Hour} {
		seen := map[time.Duration]bool{}
		for i := 0; i < 50; i++ {
			d := Backoff(cfg, attempt)
			if d < base*8/10 || d > base*12/10 {
				t.Fatalf("attempt %d: delay %s outside %s ±20%%", attempt, d, base)
			}
			seen[d] = true
		}
		if len(seen) < 5 {
			t.Fatalf("attempt %d: no jitter", attempt)
		}
	}
}

func TestValidateURL(t *testing.T) {
	strict := &config.Config{}
	for _, bad := range []string{"http://example.com/hook", "https://127.0.0.1/hook", "https://10.1.2.3/hook", "https://169.254.169.254/latest",
		"https://[::1]/hook", "https://localhost/hook", "https://app.localhost/hook", "ftp://example.com/x", "https://user:pw@example.com/",
		"https://example.com/#frag", "/relative", "https://192.168.1.10:8443/x", "https://[fd00::1]/x", "https://0.0.0.0/"} {
		if ValidateURL(strict, bad) == nil {
			t.Errorf("%s accepted by default", bad)
		}
	}
	for _, good := range []string{"https://shop.example.com/hooks/rea", "https://erp.example.com:9443/in", "https://93.184.216.34/hook"} {
		if err := ValidateURL(strict, good); err != nil {
			t.Errorf("%s rejected: %v", good, err)
		}
	}
	local := &config.Config{WebhookAllowHTTPLocalhost: true}
	if ValidateURL(local, "http://localhost:3000/hook") != nil || ValidateURL(local, "http://127.0.0.1:3000/hook") != nil {
		t.Error("http to localhost should be allowed with the flag")
	}
	if ValidateURL(local, "http://example.com/hook") == nil || ValidateURL(local, "https://10.0.0.1/hook") == nil {
		t.Error("the localhost flag must not allow other hosts")
	}
	private := &config.Config{WebhookAllowPrivate: true}
	if ValidateURL(private, "https://10.0.0.1/hook") != nil || ValidateURL(private, "http://127.0.0.1/hook") == nil {
		t.Error("private flag: https to private allowed, plain http still not")
	}
	if ValidateURL(private, "https://169.254.169.254/") == nil {
		t.Error("metadata address must stay blocked")
	}
}
