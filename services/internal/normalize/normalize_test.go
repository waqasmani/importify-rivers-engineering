package normalize

import "testing"

func TestCompanyName(t *testing.T) {
	cases := map[string]string{
		"Shenzhen Acme Electronics Co., Ltd.": "shenzhen acme electronics",
		"The Widget Company":                  "widget",
		"ACME, Inc":                           "acme",
		"Ltd":                                 "ltd",
	}
	for in, want := range cases {
		if got := CompanyName(in); got != want {
			t.Errorf("CompanyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDomain(t *testing.T) {
	cases := map[string]string{
		"https://www.Example.com/path?q=1": "example.com",
		"shop.example.co.uk":               "shop.example.co.uk",
		"not a domain":                     "",
		"":                                 "",
		"http://localhost:8080":            "localhost",
	}
	for in, want := range cases {
		if got := Domain(in); got != want {
			t.Errorf("Domain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalURL(t *testing.T) {
	cases := map[string]string{
		"HTTPS://Shop.Example.com:443/p/widget/?utm_source=x&b=2&a=1#reviews": "https://shop.example.com/p/widget?a=1&b=2",
		"http://example.com/":                 "http://example.com",
		"https://example.com/item?fbclid=abc": "https://example.com/item",
		"https://example.com:8443/x":          "https://example.com:8443/x",
	}
	for in, want := range cases {
		got, err := CanonicalURL(in)
		if err != nil || got != want {
			t.Errorf("CanonicalURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"ftp://example.com", "/relative", "javascript:alert(1)", "https://user:pw@example.com"} {
		if _, err := CanonicalURL(bad); err == nil {
			t.Errorf("CanonicalURL(%q) should fail", bad)
		}
	}
}

func TestParsePrice(t *testing.T) {
	cases := []struct {
		in, amount, cur string
		warn            bool
	}{
		{"$1,299.00", "1299", "USD", true},
		{"US $12.50 - 15.00", "12.5", "USD", true},
		{"1.299,95 €", "1299.95", "EUR", false},
		{"£ 49", "49", "GBP", false},
		{"12,50 EUR", "12.5", "EUR", false},
		{"Price: 2,500", "2500", "", false},
		{"CN¥ 88.00", "88", "CNY", false},
	}
	for _, c := range cases {
		r, ok := ParsePrice(c.in, "")
		if !ok || r.Amount.String() != c.amount || r.Currency != c.cur {
			t.Errorf("ParsePrice(%q) = %s %s ok=%v; want %s %s", c.in, r.Amount, r.Currency, ok, c.amount, c.cur)
		}
		if c.warn && len(r.Warnings) == 0 {
			t.Errorf("ParsePrice(%q) expected a warning", c.in)
		}
	}
	if _, ok := ParsePrice("Contact supplier", ""); ok {
		t.Error("ParsePrice should fail without digits")
	}
}

func TestGTINAndAvailability(t *testing.T) {
	if !ValidGTIN("4006381333931") || ValidGTIN("4006381333932") || !ValidGTIN("036000291452") {
		t.Error("GTIN check digit validation is wrong")
	}
	for in, want := range map[string]string{"in_stock": "in_stock", "OUT_OF_STOCK": "out_of_stock", "pre-order": "preorder", "back_order": "backorder",
		"preorder": "preorder", "Currently unavailable": "out_of_stock", "discontinued": "discontinued", "maybe": "unknown"} {
		if got := Availability(in); got != want {
			t.Errorf("Availability(%q) = %q, want %q", in, got, want)
		}
	}
	if Availability("https://schema.org/InStock") != "in_stock" || Availability("Sold out") != "out_of_stock" || Availability("") != "unknown" {
		t.Error("availability mapping is wrong")
	}
}

func TestSlug(t *testing.T) {
	if Slug("Product Sourcing 2026!") != "product-sourcing-2026" {
		t.Error(Slug("Product Sourcing 2026!"))
	}
}
