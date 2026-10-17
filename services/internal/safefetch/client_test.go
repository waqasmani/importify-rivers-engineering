package safefetch

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestClientBlocksPrivateTargetsAtDialTime(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	// "localhost" is a host name: the check happens after resolution, at dial time.
	url := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	strict := NewClient(func() Policy { return Policy{} }, 5*time.Second)
	for _, u := range []string{srv.URL, url} {
		_, err := strict.Get(u)
		if !IsBlockedError(err) {
			t.Fatalf("%s: expected a blocked address, got %v", u, err)
		}
	}
	if hits != 0 {
		t.Fatal("blocked request reached the server")
	}
	allowed := NewClient(func() Policy { return Policy{AllowLoopback: true} }, 5*time.Second)
	resp, err := allowed.Get(url)
	if err != nil {
		t.Fatalf("loopback allowed by policy failed: %v", err)
	}
	resp.Body.Close()
	if hits != 1 {
		t.Fatal("allowed request did not arrive")
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	target := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			target++
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer srv.Close()
	c := NewClient(func() Policy { return Policy{AllowPrivate: true} }, 5*time.Second)
	resp, err := c.Get(srv.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || target != 0 {
		t.Fatalf("redirect followed (status %d, target hits %d)", resp.StatusCode, target)
	}
}

func TestPolicy(t *testing.T) {
	for _, s := range []string{"10.0.0.1", "192.168.1.1", "172.16.0.1", "127.0.0.1", "::1", "169.254.1.1", "fd00::1", "100.64.0.1"} {
		a := netip.MustParseAddr(s)
		if (Policy{}).Allows(a) {
			t.Errorf("%s allowed by the default policy", s)
		}
		if !(Policy{AllowPrivate: true}).Allows(a) {
			t.Errorf("%s blocked although private networks are allowed", s)
		}
	}
	for _, s := range []string{"169.254.169.254", "100.100.100.200", "fd00:ec2::254", "0.0.0.0", "224.0.0.1", "255.255.255.255"} {
		if (Policy{AllowPrivate: true, AllowLoopback: true}).Allows(netip.MustParseAddr(s)) {
			t.Errorf("%s must always be blocked", s)
		}
	}
	if (Policy{AllowLoopback: true}).Allows(netip.MustParseAddr("10.0.0.1")) || !(Policy{AllowLoopback: true}).Allows(netip.MustParseAddr("127.0.0.1")) {
		t.Error("loopback-only policy wrong")
	}
	if !(Policy{}).Allows(netip.MustParseAddr("93.184.216.34")) {
		t.Error("public address blocked")
	}
}
