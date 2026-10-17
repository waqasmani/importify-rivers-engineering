package safefetch

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Policy controls which addresses an outbound client may connect to.
type Policy struct {
	// AllowPrivate permits private, loopback, link-local and other reserved unicast ranges.
	AllowPrivate bool
	// AllowLoopback permits loopback addresses only (127.0.0.0/8 and ::1).
	AllowLoopback bool
}

// metadataAddrs are cloud instance metadata services. They stay blocked even when private
// networks are allowed, because they hand out credentials.
var metadataAddrs = []netip.Addr{
	netip.MustParseAddr("169.254.169.254"), // AWS, GCP, Azure, OpenStack, DigitalOcean
	netip.MustParseAddr("169.254.170.2"),   // AWS ECS task metadata
	netip.MustParseAddr("100.100.100.200"), // Alibaba Cloud
	netip.MustParseAddr("fd00:ec2::254"),   // AWS IPv6
}

// IsMetadata reports whether a is a cloud metadata endpoint.
func IsMetadata(a netip.Addr) bool {
	a = a.Unmap()
	for _, m := range metadataAddrs {
		if a == m {
			return true
		}
	}
	return false
}

// Allows reports whether the policy permits connecting to a.
func (p Policy) Allows(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsUnspecified() || a.IsMulticast() || a.IsLinkLocalMulticast() || IsMetadata(a) ||
		a == netip.MustParseAddr("255.255.255.255") {
		return false
	}
	if !IsBlocked(a) {
		return true
	}
	if a.IsLoopback() {
		return p.AllowPrivate || p.AllowLoopback
	}
	return p.AllowPrivate
}

// IsLocalhostName reports whether host is localhost, a *.localhost name or a loopback literal.
func IsLocalhostName(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	a, err := netip.ParseAddr(strings.Trim(host, "[]"))
	return err == nil && a.Unmap().IsLoopback()
}

// CheckHost rejects URLs whose host is a literal address or a localhost name the policy does not
// allow. Other host names are checked again when the connection is made, against the resolved
// address, which also covers DNS rebinding.
func (p Policy) CheckHost(u *url.URL) error {
	host := u.Hostname()
	if host == "" {
		return ErrBlockedScheme
	}
	if IsLocalhostName(host) {
		if p.AllowPrivate || p.AllowLoopback {
			return nil
		}
		return ErrBlockedAddress
	}
	if a, err := netip.ParseAddr(host); err == nil && !p.Allows(a) {
		return ErrBlockedAddress
	}
	return nil
}

// NewClient returns an HTTP client for server-initiated requests to URLs configured by users
// (webhooks). The policy is read for every connection, the resolved address is checked at dial
// time, environment proxies are ignored, redirects are never followed and every request has an
// overall timeout. Callers must still cap how much of the response body they read.
func NewClient(policy func() Policy, timeout time.Duration) *http.Client {
	control := func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		a, err := netip.ParseAddr(host)
		if err != nil || !policy().Allows(a) {
			return ErrBlockedAddress
		}
		return nil
	}
	dialer := &net.Dialer{Timeout: min(timeout, 10*time.Second), Control: control}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   min(timeout, 10*time.Second),
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       60 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

// IsBlockedError reports whether err (from a client created by NewClient) means the address was
// refused by the policy.
func IsBlockedError(err error) bool {
	return errors.Is(err, ErrBlockedAddress) || (err != nil && strings.Contains(err.Error(), ErrBlockedAddress.Error()))
}
