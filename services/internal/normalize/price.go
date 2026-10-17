package normalize

import (
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

// currencySymbols maps common symbols and prefixes to ISO 4217 codes. Ambiguous symbols ("$", "¥")
// map to the most common currency and the parser reports the ambiguity as a warning.
var currencySymbols = []struct {
	sym, code string
	ambiguous bool
}{
	{"US$", "USD", false}, {"USD", "USD", false}, {"C$", "CAD", false}, {"CA$", "CAD", false}, {"A$", "AUD", false},
	{"AU$", "AUD", false}, {"NZ$", "NZD", false}, {"HK$", "HKD", false}, {"S$", "SGD", false}, {"R$", "BRL", false},
	{"MX$", "MXN", false}, {"€", "EUR", false}, {"£", "GBP", false}, {"₹", "INR", false}, {"₩", "KRW", false},
	{"₽", "RUB", false}, {"₺", "TRY", false}, {"₫", "VND", false}, {"฿", "THB", false}, {"₱", "PHP", false},
	{"₦", "NGN", false}, {"zł", "PLN", false}, {"kr", "SEK", true}, {"CHF", "CHF", false}, {"RMB", "CNY", false},
	{"CN¥", "CNY", false}, {"JP¥", "JPY", false}, {"¥", "CNY", true}, {"元", "CNY", false}, {"Rs", "INR", true},
	{"AED", "AED", false}, {"SAR", "SAR", false}, {"PKR", "PKR", false}, {"$", "USD", true},
}

// KnownCurrencies is the set of ISO 4217 codes accepted by the API.
var KnownCurrencies = map[string]bool{}

func init() {
	for _, c := range strings.Fields(`AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BRL BSD
BTN BWP BYN BZD CAD CDF CHF CLP CNY COP CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD
GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD
LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN
PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD
TZS UAH UGX USD UYU UZS VES VND VUV WST XAF XCD XOF XPF YER ZAR ZMW ZWL`) {
		KnownCurrencies[c] = true
	}
}

// ValidCurrency reports whether code is a known ISO 4217 code.
func ValidCurrency(code string) bool { return KnownCurrencies[code] }

var numberRe = regexp.MustCompile(`\d[\d\s.,'’]*`)

// PriceResult is the outcome of parsing a human-formatted price.
type PriceResult struct {
	Amount   decimal.Decimal
	Currency string
	Warnings []string
}

// ParsePrice parses strings such as "$1,299.00", "1.299,95 €", "US $12.50 - 15.00" or "¥ 88".
// For ranges it returns the lower bound with a warning. It returns ok=false when no number is found.
func ParsePrice(s string, fallbackCurrency string) (PriceResult, bool) {
	var res PriceResult
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", " "))
	if s == "" {
		return res, false
	}
	upper := strings.ToUpper(s)
	for _, cs := range currencySymbols {
		if strings.Contains(upper, strings.ToUpper(cs.sym)) {
			res.Currency = cs.code
			if cs.ambiguous {
				res.Warnings = append(res.Warnings, "currency symbol \""+cs.sym+"\" is ambiguous; assumed "+cs.code)
			}
			break
		}
	}
	if res.Currency == "" {
		for _, f := range strings.FieldsFunc(upper, func(r rune) bool { return r < 'A' || r > 'Z' }) {
			if len(f) == 3 && KnownCurrencies[f] {
				res.Currency = f
				break
			}
		}
	}
	if res.Currency == "" && fallbackCurrency != "" {
		res.Currency = fallbackCurrency
	}
	nums := numberRe.FindAllString(s, -1)
	if len(nums) == 0 {
		return res, false
	}
	if len(nums) > 1 && (strings.Contains(s, "-") || strings.Contains(s, "–") || strings.Contains(strings.ToLower(s), " to ")) {
		res.Warnings = append(res.Warnings, "price is a range; the lowest value was used")
	}
	amt, ok := parseNumber(nums[0])
	if !ok {
		return res, false
	}
	res.Amount = amt
	return res, true
}

// parseNumber interprets thousands and decimal separators in either US or European style.
func parseNumber(raw string) (decimal.Decimal, bool) {
	n := strings.TrimSpace(raw)
	n = strings.NewReplacer(" ", "", "'", "", "’", "").Replace(n)
	n = strings.TrimRight(n, ".,")
	lastDot := strings.LastIndex(n, ".")
	lastComma := strings.LastIndex(n, ",")
	switch {
	case lastDot >= 0 && lastComma >= 0:
		if lastComma > lastDot { // 1.299,95
			n = strings.ReplaceAll(n, ".", "")
			n = strings.Replace(n, ",", ".", 1)
		} else { // 1,299.95
			n = strings.ReplaceAll(n, ",", "")
		}
	case lastComma >= 0:
		// "12,50" is a decimal comma; "1,299" or "1,299,000" are thousands separators.
		if len(n)-lastComma-1 == 3 && strings.Count(n, ",") >= 1 && !(strings.Count(n, ",") == 1 && len(n) <= 4) {
			n = strings.ReplaceAll(n, ",", "")
		} else if strings.Count(n, ",") == 1 {
			n = strings.Replace(n, ",", ".", 1)
		} else {
			n = strings.ReplaceAll(n, ",", "")
		}
	case lastDot >= 0:
		if strings.Count(n, ".") > 1 { // 1.299.000
			n = strings.ReplaceAll(n, ".", "")
		}
	}
	d, err := decimal.NewFromString(n)
	if err != nil || d.IsNegative() {
		return decimal.Decimal{}, false
	}
	return d, true
}

// Availability maps schema.org availability URLs and common phrases to the stored enum.
func Availability(s string) string {
	l := strings.ToLower(strings.TrimSpace(s))
	l = strings.TrimPrefix(strings.TrimPrefix(l, "https://schema.org/"), "http://schema.org/")
	// Accept REA's own values (in_stock, out_of_stock, …) and common spellings with - or _.
	l = strings.NewReplacer("_", " ", "-", " ").Replace(l)
	switch {
	case l == "":
		return "unknown"
	case strings.Contains(l, "outofstock") || strings.Contains(l, "out of stock") || strings.Contains(l, "sold out") || strings.Contains(l, "unavailable"):
		return "out_of_stock"
	case strings.Contains(l, "preorder") || strings.Contains(l, "pre order") || strings.Contains(l, "presale"):
		return "preorder"
	case strings.Contains(l, "backorder") || strings.Contains(l, "back order"):
		return "backorder"
	case strings.Contains(l, "discontinued"):
		return "discontinued"
	case strings.Contains(l, "instock") || strings.Contains(l, "in stock") || strings.Contains(l, "limitedavailability") ||
		strings.Contains(l, "onlineonly") || strings.Contains(l, "instoreonly") || strings.Contains(l, "available"):
		return "in_stock"
	}
	return "unknown"
}

// ValidGTIN checks the length and check digit of a GTIN-8/12/13/14.
func ValidGTIN(s string) bool {
	if l := len(s); l != 8 && l != 12 && l != 13 && l != 14 {
		return false
	}
	sum := 0
	for i := 0; i < len(s)-1; i++ {
		c := s[len(s)-2-i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i%2 == 0 {
			d *= 3
		}
		sum += d
	}
	last := s[len(s)-1]
	if last < '0' || last > '9' {
		return false
	}
	return (10-sum%10)%10 == int(last-'0')
}
