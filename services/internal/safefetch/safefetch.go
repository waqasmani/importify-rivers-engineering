// Package safefetch fetches public web pages for analysis while preventing server-side request
// forgery: only http(s) on standard ports, every connection is checked against private, loopback,
// link-local and metadata address ranges at dial time (so DNS rebinding and redirects are covered),
// responses are size-limited, and robots.txt is honoured.
package safefetch

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// UserAgent identifies REA to the sites it fetches.
const UserAgent = "REA/1.0 (self-hosted research tool; respects robots.txt)"

// Errors returned by Fetch.
var (
	ErrBlockedAddress = errors.New("the address resolves to a private or reserved network and is blocked")
	ErrBlockedScheme  = errors.New("only http and https URLs on ports 80, 443, 8080 and 8443 can be fetched")
	ErrRobots         = errors.New("the site's robots.txt does not allow fetching this page")
	ErrTooLarge       = errors.New("the response is larger than the fetch limit")
	ErrContentType    = errors.New("the response is not an HTML, XML, JSON or text document")
)

var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24",
		"192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
		"224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32", "::/128", "::1/128", "::ffff:0:0/96", "64:ff9b::/96",
		"100::/64", "2001::/23", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// IsBlocked reports whether an IP must not be contacted.
func IsBlocked(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

var allowedPorts = map[string]bool{"80": true, "443": true, "8080": true, "8443": true}

// Fetcher performs safe GET requests.
type Fetcher struct {
	client       *http.Client
	maxBytes     int64
	allowPrivate bool
	checkRobots  bool
}

// New creates a Fetcher. allowPrivate permits private ranges (for operators analysing their own LAN).
func New(maxBytes int64, allowPrivate bool) *Fetcher {
	f := &Fetcher{maxBytes: maxBytes, allowPrivate: allowPrivate, checkRobots: true}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: f.control}
	transport := &http.Transport{
		Proxy:                 nil, // never route through an environment proxy that could reach internal hosts
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	}
	f.client = &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return f.validateURL(req.URL)
	}}
	return f
}

// SetTransportProxy routes requests through a proxy (used only when the operator configures one).
func (f *Fetcher) SetTransportProxy(p func(*http.Request) (*url.URL, error)) {
	f.client.Transport.(*http.Transport).Proxy = p
}

func (f *Fetcher) control(network, address string, _ syscall.RawConn) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if !allowedPorts[port] {
		return ErrBlockedScheme
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return ErrBlockedAddress
	}
	if !f.allowPrivate && IsBlocked(a) {
		return ErrBlockedAddress
	}
	return nil
}

// validateURL rejects unsupported schemes and ports, and literal IP hosts in blocked ranges before
// any socket is opened. Host names are checked again at dial time against the resolved address.
func (f *Fetcher) validateURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrBlockedScheme
	}
	if u.User != nil || u.Hostname() == "" {
		return ErrBlockedScheme
	}
	if a, err := netip.ParseAddr(u.Hostname()); err == nil && !f.allowPrivate && IsBlocked(a) {
		return ErrBlockedAddress
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	if !allowedPorts[port] {
		return ErrBlockedScheme
	}
	return nil
}

// Result is a fetched document.
type Result struct {
	FinalURL    string
	StatusCode  int
	ContentType string
	Body        []byte
}

// Fetch retrieves a URL after checking robots.txt.
func (f *Fetcher) Fetch(ctx context.Context, raw string) (Result, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Result{}, err
	}
	if err := f.validateURL(u); err != nil {
		return Result{}, err
	}
	if f.checkRobots {
		allowed, err := f.robotsAllowed(ctx, u)
		if err != nil {
			return Result{}, err
		}
		if !allowed {
			return Result{}, ErrRobots
		}
	}
	return f.get(ctx, u.String(), f.maxBytes, true)
}

func (f *Fetcher) get(ctx context.Context, raw string, max int64, checkType bool) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,text/plain;q=0.8")
	resp, err := f.client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			var oe *net.OpError
			if errors.As(ue.Err, &oe) && (errors.Is(oe.Err, ErrBlockedAddress) || strings.Contains(oe.Error(), ErrBlockedAddress.Error())) {
				return Result{}, ErrBlockedAddress
			}
			if strings.Contains(ue.Error(), ErrBlockedAddress.Error()) {
				return Result{}, ErrBlockedAddress
			}
			if strings.Contains(ue.Error(), ErrBlockedScheme.Error()) {
				return Result{}, ErrBlockedScheme
			}
		}
		return Result{}, err
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)
	if checkType && mt != "" && !(strings.HasPrefix(mt, "text/") || strings.Contains(mt, "html") || strings.Contains(mt, "xml") || strings.Contains(mt, "json")) {
		return Result{}, ErrContentType
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return Result{}, err
	}
	if int64(len(body)) > max {
		return Result{}, ErrTooLarge
	}
	return Result{FinalURL: resp.Request.URL.String(), StatusCode: resp.StatusCode, ContentType: mt, Body: body}, nil
}

// robotsAllowed applies the Disallow/Allow rules for "*" and "REA" from /robots.txt. A missing or
// unreachable robots.txt (4xx) allows fetching; server errors deny, as the convention recommends.
func (f *Fetcher) robotsAllowed(ctx context.Context, u *url.URL) (bool, error) {
	robots := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/robots.txt"}
	res, err := f.get(ctx, robots.String(), 512<<10, false)
	if err != nil {
		if errors.Is(err, ErrBlockedAddress) || errors.Is(err, ErrBlockedScheme) {
			return false, err
		}
		return false, fmt.Errorf("could not read robots.txt: %w", err)
	}
	if res.StatusCode >= 400 && res.StatusCode < 500 {
		return true, nil
	}
	if res.StatusCode >= 500 {
		return false, nil
	}
	return RobotsAllows(string(res.Body), "REA", u.EscapedPath()), nil
}

// RobotsAllows evaluates robots.txt rules using longest-match precedence.
func RobotsAllows(robots, agent, p string) bool {
	if p == "" {
		p = "/"
	}
	type rule struct {
		allow bool
		path  string
	}
	groups := map[string][]rule{}
	var current []string
	inRules := false
	sc := bufio.NewScanner(strings.NewReader(robots))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "user-agent":
			if inRules {
				current = nil
				inRules = false
			}
			current = append(current, strings.ToLower(v))
		case "allow", "disallow":
			inRules = true
			for _, a := range current {
				groups[a] = append(groups[a], rule{allow: k == "allow", path: v})
			}
		}
	}
	rules, ok := groups[strings.ToLower(agent)]
	if !ok {
		rules = groups["*"]
	}
	best, allowed := -1, true
	for _, r := range rules {
		if r.path == "" {
			continue
		}
		if strings.HasPrefix(p, strings.TrimSuffix(r.path, "*")) && len(r.path) > best {
			best, allowed = len(r.path), r.allow
		}
	}
	return allowed
}
