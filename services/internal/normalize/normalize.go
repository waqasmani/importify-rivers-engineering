// Package normalize cleans and canonicalises values used for search and duplicate detection.
package normalize

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	spaceRe    = regexp.MustCompile(`\s+`)
	nonAlnumRe = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

// legalSuffixes are removed from company and supplier names before comparison.
var legalSuffixes = map[string]bool{
	"inc": true, "incorporated": true, "llc": true, "ltd": true, "limited": true, "co": true, "company": true,
	"corp": true, "corporation": true, "gmbh": true, "ag": true, "sa": true, "sas": true, "sarl": true, "srl": true,
	"spa": true, "bv": true, "nv": true, "plc": true, "pty": true, "pte": true, "llp": true, "lp": true, "oy": true,
	"ab": true, "as": true, "kg": true, "kk": true, "the": true,
}

// Text trims, removes control characters and collapses internal whitespace.
func Text(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == ' ' {
			return ' '
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// MultilineText trims and removes control characters but keeps line breaks.
func MultilineText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Key lowercases and strips punctuation, producing a comparison key.
func Key(s string) string {
	return strings.TrimSpace(nonAlnumRe.ReplaceAllString(strings.ToLower(s), " "))
}

// CompanyName returns a comparison key with legal suffixes removed, e.g.
// "Shenzhen Acme Electronics Co., Ltd." -> "shenzhen acme electronics".
func CompanyName(s string) string {
	words := strings.Fields(Key(s))
	for len(words) > 1 && legalSuffixes[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	for len(words) > 1 && words[0] == "the" {
		words = words[1:]
	}
	return strings.Join(words, " ")
}

// ProductTitle returns a comparison key for product titles.
func ProductTitle(s string) string { return Key(s) }

// Domain extracts a lowercase host without "www." from a URL or bare host. It returns "" when the
// input does not contain a usable host name.
func Domain(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimSuffix(host, ".")
	host = strings.TrimPrefix(host, "www.")
	if host == "" || strings.ContainsAny(host, " /\\") {
		return ""
	}
	if net.ParseIP(host) == nil && !strings.Contains(host, ".") && host != "localhost" {
		return ""
	}
	return host
}

// trackingParams are query parameters that do not identify content.
var trackingParams = map[string]bool{
	"gclid": true, "fbclid": true, "msclkid": true, "yclid": true, "dclid": true, "igshid": true, "mc_cid": true,
	"mc_eid": true, "_ga": true, "_gl": true, "ref": true, "ref_": true, "spm": true, "scm": true, "srsltid": true,
	"_hsenc": true, "_hsmi": true, "trk": true, "tracking_id": true, "affiliate": true, "aff_id": true,
}

// ErrInvalidURL is returned for URLs that are not absolute http(s) URLs.
var ErrInvalidURL = errors.New("must be an absolute http or https URL")

// CanonicalURL lowercases the scheme and host, removes default ports, fragments and tracking
// parameters, sorts the remaining query and trims a trailing slash.
func CanonicalURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 2048 {
		return "", errors.New("is longer than 2048 characters")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", ErrInvalidURL
	}
	if u.User != nil {
		return "", errors.New("must not contain credentials")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	u.Fragment = ""
	u.RawFragment = ""
	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || strings.HasPrefix(lk, "pd_rd_") || strings.HasPrefix(lk, "pf_rd_") || trackingParams[lk] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	u.RawQuery = strings.Join(parts, "&")
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
		u.RawPath = ""
	}
	if u.Path == "/" {
		u.Path = ""
	}
	return u.String(), nil
}

// ValidHTTPURL reports whether raw is an absolute http(s) URL without credentials.
func ValidHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

// Slug converts a name to a URL-safe identifier.
func Slug(s string) string {
	s = strings.Trim(nonAlnumRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	if out == "" {
		out = "workspace"
	}
	return out
}

// Truncate shortens s to at most n runes.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
