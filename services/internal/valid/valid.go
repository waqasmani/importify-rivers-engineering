// Package valid collects field validation errors for API input.
package valid

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
)

// Errors maps field names to messages.
type Errors map[string]string

// V accumulates validation errors.
type V struct{ Errs Errors }

// New returns an empty validator.
func New() *V { return &V{Errs: Errors{}} }

// OK reports whether no errors were recorded.
func (v *V) OK() bool { return len(v.Errs) == 0 }

// Add records an error for field unless one is already present.
func (v *V) Add(field, msg string) {
	if _, ok := v.Errs[field]; !ok {
		v.Errs[field] = msg
	}
}

// Check records msg when ok is false.
func (v *V) Check(ok bool, field, msg string) {
	if !ok {
		v.Add(field, msg)
	}
}

// Required requires a non-blank string.
func (v *V) Required(field, val string) {
	v.Check(strings.TrimSpace(val) != "", field, "is required")
}

// Len enforces a rune length range on a string.
func (v *V) Len(field, val string, min, max int) {
	n := utf8.RuneCountInString(val)
	if n < min {
		if min == 1 {
			v.Add(field, "is required")
		} else {
			v.Add(field, fmt.Sprintf("must be at least %d characters", min))
		}
	} else if n > max {
		v.Add(field, fmt.Sprintf("must be at most %d characters", max))
	}
}

// Max enforces a maximum rune length.
func (v *V) Max(field, val string, max int) { v.Len(field, val, 0, max) }

// OneOf requires val to be one of the options.
func (v *V) OneOf(field, val string, opts ...string) {
	for _, o := range opts {
		if val == o {
			return
		}
	}
	v.Add(field, "must be one of: "+strings.Join(opts, ", "))
}

// Email validates an optional e-mail address.
func (v *V) Email(field, val string) {
	if val == "" {
		return
	}
	a, err := mail.ParseAddress(val)
	v.Check(err == nil && a.Address == val && len(val) <= 320, field, "must be a valid e-mail address")
}

// URL validates an optional absolute http(s) URL.
func (v *V) URL(field, val string) {
	if val == "" {
		return
	}
	v.Check(len(val) <= 2048 && normalize.ValidHTTPURL(val), field, "must be an absolute http or https URL")
}

// Currency validates an ISO 4217 code.
func (v *V) Currency(field, val string) {
	v.Check(normalize.ValidCurrency(val), field, "must be a three-letter ISO 4217 currency code")
}

// IntRange validates an optional integer.
func (v *V) IntRange(field string, val *int32, min, max int32) {
	if val != nil && (*val < min || *val > max) {
		v.Add(field, fmt.Sprintf("must be between %d and %d", min, max))
	}
}
