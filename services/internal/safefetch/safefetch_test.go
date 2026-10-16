package safefetch

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestBlockedRanges(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.5", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fd00::1", "fe80::1", "0.0.0.0", "::ffff:127.0.0.1"} {
		if !IsBlocked(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"93.184.216.34", "1.1.1.1", "2606:4700:4700::1111"} {
		if IsBlocked(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestFetchBlocksInternalTargets(t *testing.T) {
	f := New(1<<20, false)
	f.checkRobots = false
	for _, u := range []string{"http://127.0.0.1/", "http://localhost/", "http://169.254.169.254/latest/meta-data/", "http://[::1]/",
		"http://10.0.0.1/"} {
		_, err := f.Fetch(context.Background(), u)
		if !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("%s: expected blocked address, got %v", u, err)
		}
	}
	if _, err := f.Fetch(context.Background(), "http://2130706433/"); err == nil {
		t.Error("decimal-encoded loopback address must not be fetched")
	}
	for _, u := range []string{"file:///etc/passwd", "gopher://x", "http://example.com:22/", "http://user:pw@example.com/"} {
		if _, err := f.Fetch(context.Background(), u); !errors.Is(err, ErrBlockedScheme) {
			t.Errorf("%s: expected blocked scheme, got %v", u, err)
		}
	}
}

func TestRobotsRules(t *testing.T) {
	robots := "User-agent: *\nDisallow: /private\nAllow: /private/ok\n\nUser-agent: BadBot\nDisallow: /\n"
	cases := map[string]bool{"/": true, "/private/x": false, "/private/ok/y": true, "/public": true}
	for p, want := range cases {
		if got := RobotsAllows(robots, "REA", p); got != want {
			t.Errorf("%s: got %v want %v", p, got, want)
		}
	}
	if RobotsAllows("User-agent: REA\nDisallow: /\n", "REA", "/x") {
		t.Error("agent-specific disallow ignored")
	}
}
