package orders

import (
	"golang.org/x/text/language"
)

// withdrawn are former ISO 3166-1 codes that CLDR still knows as regions without a single
// successor; addresses must use the current code.
var withdrawn = map[string]bool{"AN": true, "CS": true, "NT": true, "SU": true, "YU": true}

// ValidCountry reports whether code is an upper-case ISO 3166-1 alpha-2 country code such as "DE"
// or "US" (Kosovo's widely used "XK" included). Groups and reserved codes ("EU", "UN", "ZZ"),
// aliases ("UK" for GB) and withdrawn codes are rejected. The region data comes from CLDR via
// golang.org/x/text.
func ValidCountry(code string) bool {
	if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' || withdrawn[code] {
		return false
	}
	r, err := language.ParseRegion(code)
	if err != nil || !r.IsCountry() || r.M49() == 0 {
		return false
	}
	return r.Canonicalize().String() == code
}
