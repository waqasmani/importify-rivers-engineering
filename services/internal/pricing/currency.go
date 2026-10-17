package pricing

import "github.com/shopspring/decimal"

// zeroDecimal and threeDecimal list ISO 4217 currencies whose minor unit is not 1/100.
var (
	zeroDecimal = map[string]bool{"BIF": true, "CLP": true, "DJF": true, "GNF": true, "ISK": true, "JPY": true, "KMF": true,
		"KRW": true, "PYG": true, "RWF": true, "UGX": true, "VND": true, "VUV": true, "XAF": true, "XOF": true, "XPF": true}
	threeDecimal = map[string]bool{"BHD": true, "IQD": true, "JOD": true, "KWD": true, "LYD": true, "OMR": true, "TND": true}
)

// MinorUnits returns the number of decimal places a currency is priced in (2 for USD, 0 for JPY,
// 3 for KWD).
func MinorUnits(currency string) int32 {
	switch {
	case zeroDecimal[currency]:
		return 0
	case threeDecimal[currency]:
		return 3
	}
	return 2
}

// unit is the smallest amount of a currency, e.g. 0.01 for USD.
func unit(currency string) decimal.Decimal { return decimal.New(1, -MinorUnits(currency)) }

// Rates converts amounts into the base currency. Rates[c] is the value of one unit of c in Base:
// with Base USD and Rates["EUR"] = 1.08, 10 EUR is 10.80 USD.
type Rates struct {
	Base  string
	Rates map[string]decimal.Decimal
}

// Rate returns the conversion factor from currency to the base currency.
func (r Rates) Rate(currency string) (decimal.Decimal, bool) {
	if currency == r.Base {
		return decimal.NewFromInt(1), true
	}
	v, ok := r.Rates[currency]
	return v, ok && v.IsPositive()
}

// ToBase converts amount into the base currency, rounded to the 4 decimal places REA stores.
func (r Rates) ToBase(amount decimal.Decimal, currency string) (decimal.Decimal, bool) {
	rate, ok := r.Rate(currency)
	if !ok {
		return decimal.Decimal{}, false
	}
	if currency == r.Base {
		return amount, true
	}
	return amount.Mul(rate).Round(4), true
}

// DeriveCost turns an observed source price into a cost. The cost is converted into the base
// currency when a rate is known; otherwise it stays in the source currency and converted is false.
func (r Rates) DeriveCost(sourcePrice decimal.Decimal, sourceCurrency string) (cost decimal.Decimal, currency string, converted bool) {
	if v, ok := r.ToBase(sourcePrice, sourceCurrency); ok {
		return v, r.Base, true
	}
	return sourcePrice, sourceCurrency, false
}
