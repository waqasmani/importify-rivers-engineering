package products

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func strp(s string) *string { return &s }

func TestNormalizeCompletePayload(t *testing.T) {
	conf := 0.92
	p := Payload{
		Title: "  Wireless   Earbuds Pro  ", URL: "https://Shop.Example.com/p/earbuds?utm_source=x&id=7#reviews",
		CanonicalURL: "https://shop.example.com/p/earbuds?id=7", Brand: "Acme", SKU: "EB-100", GTIN: "4006381333931",
		Price: strp("$1,299.00"), Currency: "", Availability: "https://schema.org/InStock",
		Images: []Image{{URL: "https://cdn.example.com/a.jpg"}, {URL: "https://cdn.example.com/a.jpg"}, {URL: "javascript:alert(1)"}, {URL: "data:image/png;base64,AAA"}},
		Variants: []Variant{{Options: map[string]string{"Color": "Black", "Size": "M"}, Price: strp("€10,50")}, {}},
		Specifications: map[string]string{"Battery": "30h", "": "ignored"}, ExtractionMethod: "JSON LD", Confidence: &conf,
	}
	n, errs := Normalize(p, Options{DefaultCurrency: "EUR", RequireURL: true})
	if len(errs) > 0 {
		t.Fatalf("unexpected errors %v", errs)
	}
	if n.Title != "Wireless Earbuds Pro" || n.NormalizedTitle != "wireless earbuds pro" {
		t.Errorf("title %q / %q", n.Title, n.NormalizedTitle)
	}
	if *n.CanonicalURL != "https://shop.example.com/p/earbuds?id=7" || n.SourceDomain != "shop.example.com" {
		t.Errorf("url %v %s", *n.CanonicalURL, n.SourceDomain)
	}
	if n.Price.String() != "1299" || *n.PriceCurrency != "USD" {
		t.Errorf("price %s %s", n.Price, *n.PriceCurrency)
	}
	if n.GTIN == nil || *n.GTIN != "4006381333931" || n.Availability != "in_stock" {
		t.Errorf("gtin/availability wrong")
	}
	if len(n.Images) != 1 {
		t.Errorf("expected 1 safe unique image, got %d", len(n.Images))
	}
	if len(n.Variants) != 1 || n.Variants[0].Title != "Black / M" || n.Variants[0].Price.String() != "10.5" || *n.Variants[0].Currency != "EUR" {
		t.Errorf("variant normalization wrong: %+v", n.Variants)
	}
	if n.ExtractionMethod != "json-ld" || n.Confidence.String() != "0.92" {
		t.Errorf("method/confidence %s %s", n.ExtractionMethod, n.Confidence)
	}
	if len(n.Specifications) != 1 {
		t.Errorf("specs %v", n.Specifications)
	}
}

func TestNormalizeMissingAndInvalid(t *testing.T) {
	n, errs := Normalize(Payload{Title: "", URL: "ftp://x"}, Options{DefaultCurrency: "USD", RequireURL: true})
	if errs["title"] == "" || errs["url"] == "" {
		t.Fatalf("expected title and url errors, got %v", errs)
	}
	n, errs = Normalize(Payload{Title: "Thing", URL: "https://a.example/x", GTIN: "123456789013", PriceAmount: ptrDec("-1")}, Options{DefaultCurrency: "USD"})
	if errs["price_amount"] == "" {
		t.Error("negative price accepted")
	}
	if n.GTIN != nil || n.Identifiers["gtin_unverified"] == "" {
		t.Error("invalid GTIN should be kept as unverified identifier")
	}
	n, _ = Normalize(Payload{Title: "Bare"}, Options{DefaultCurrency: "USD"})
	joined := strings.Join(n.Warnings, "|")
	for _, want := range []string{"no product images", "no SKU"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning %q in %s", want, joined)
		}
	}
}

func TestCanonicalOnOtherSiteIgnored(t *testing.T) {
	n, _ := Normalize(Payload{Title: "X", URL: "https://shop.example.com/a", CanonicalURL: "https://evil.example.net/a"}, Options{DefaultCurrency: "USD"})
	if *n.CanonicalURL != "https://shop.example.com/a" {
		t.Errorf("foreign canonical accepted: %s", *n.CanonicalURL)
	}
}

func TestNoCurrencyAssumesDefault(t *testing.T) {
	n, _ := Normalize(Payload{Title: "X", Price: strp("12.00")}, Options{DefaultCurrency: "GBP"})
	if n.PriceCurrency == nil || *n.PriceCurrency != "GBP" || !strings.Contains(strings.Join(n.Warnings, " "), "assumed GBP") {
		t.Errorf("default currency handling wrong: %+v", n.Warnings)
	}
}

func ptrDec(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
