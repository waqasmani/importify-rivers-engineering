package marketplaces

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"unicode/utf8"
)

// Segment is one step of a rule path (see packages/extractor/src/rules/path.ts).
type Segment struct {
	Kind  string // key, index, wild, filter
	Key   string
	Index int
	Value string
}

const (
	pathMaxLength   = 300
	pathMaxSegments = 40
)

var (
	forbiddenKeys = set("__proto__", "prototype", "constructor")
	indexRe       = regexp.MustCompile(`^-?\d{1,6}$`)
)

func isIdent(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '$' || b == '-'
}

// ParsePath parses a rule path, rejecting malformed paths, prototype keys and token-like names.
func ParsePath(src string) ([]Segment, error) {
	if utf8.RuneCountInString(src) > pathMaxLength {
		return nil, fmt.Errorf("paths are limited to %d characters", pathMaxLength)
	}
	var segs []Segment
	i, n := 0, len(src)
	readIdent := func() string {
		start := i
		for i < n && isIdent(src[i]) {
			i++
		}
		return src[start:i]
	}
	pushKey := func(k string) error {
		if forbiddenKeys[k] {
			return fmt.Errorf("%q cannot be used in a path", k)
		}
		if IsSensitiveName(k) {
			return fmt.Errorf("%q looks like a credential or token name and cannot be read", k)
		}
		segs = append(segs, Segment{Kind: "key", Key: k})
		return nil
	}
	readQuoted := func(q byte) (string, bool) {
		var out []byte
		for i < n && src[i] != q {
			if src[i] == '\\' && i+1 < n {
				i++
			}
			out = append(out, src[i])
			i++
		}
		if i >= n {
			return "", false
		}
		i++
		return string(out), true
	}
	first := true
	for i < n {
		c := src[i]
		switch {
		case c == '[':
			i++
			if i >= n {
				return nil, errors.New("expected an index, \"*\" or a quoted key inside [ ]")
			}
			q := src[i]
			switch {
			case q == '*':
				i++
				segs = append(segs, Segment{Kind: "wild"})
			case q == '?':
				i++
				k := readIdent()
				if k == "" {
					return nil, errors.New("expected a key after \"[?\"")
				}
				if forbiddenKeys[k] || IsSensitiveName(k) {
					return nil, fmt.Errorf("%q cannot be used in a filter", k)
				}
				if i >= n || src[i] != '=' {
					return nil, errors.New("expected \"=\" in a filter")
				}
				i++
				var v string
				if i < n && (src[i] == '"' || src[i] == '\'') {
					vq := src[i]
					i++
					s, ok := readQuoted(vq)
					if !ok {
						return nil, errors.New("unterminated filter value")
					}
					v = s
				} else {
					start := i
					for i < n && src[i] != ']' {
						i++
					}
					v = src[start:i]
				}
				if l := utf8.RuneCountInString(v); l < 1 || l > 100 {
					return nil, errors.New("filter values must have 1-100 characters")
				}
				segs = append(segs, Segment{Kind: "filter", Key: k, Value: v})
			case q == '"' || q == '\'':
				i++
				k, ok := readQuoted(q)
				if !ok {
					return nil, errors.New("unterminated quoted key")
				}
				if l := utf8.RuneCountInString(k); l < 1 || l > 100 {
					return nil, errors.New("quoted keys must have 1-100 characters")
				}
				if err := pushKey(k); err != nil {
					return nil, err
				}
			default:
				start := i
				if src[i] == '-' {
					i++
				}
				for i < n && src[i] >= '0' && src[i] <= '9' {
					i++
				}
				digits := src[start:i]
				if !indexRe.MatchString(digits) {
					return nil, errors.New("expected an index, \"*\" or a quoted key inside [ ]")
				}
				v, _ := strconv.Atoi(digits)
				segs = append(segs, Segment{Kind: "index", Index: v})
			}
			if i >= n || src[i] != ']' {
				return nil, errors.New("expected \"]\"")
			}
			i++
		case c == '.' && !first:
			i++
			if i < n && src[i] == '*' {
				i++
				segs = append(segs, Segment{Kind: "wild"})
			} else {
				k := readIdent()
				if k == "" {
					return nil, fmt.Errorf("expected a key after \".\" at position %d", i)
				}
				if err := pushKey(k); err != nil {
					return nil, err
				}
			}
		case first:
			if c == '*' {
				i++
				segs = append(segs, Segment{Kind: "wild"})
			} else {
				k := readIdent()
				if k == "" {
					return nil, fmt.Errorf("unexpected character %q at position %d", c, i)
				}
				if err := pushKey(k); err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("unexpected character %q at position %d", c, i)
		}
		first = false
		if len(segs) > pathMaxSegments {
			return nil, fmt.Errorf("paths are limited to %d segments", pathMaxSegments)
		}
	}
	return segs, nil
}
