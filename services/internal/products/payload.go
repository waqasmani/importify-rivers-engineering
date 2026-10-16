// Package products validates and normalizes captured product data before it is stored.
package products

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// Image is an image reference observed on the source page.
type Image struct {
	URL    string `json:"url"`
	Alt    string `json:"alt"`
	Width  *int32 `json:"width"`
	Height *int32 `json:"height"`
}

// Variant is an observable option combination (size, colour...).
type Variant struct {
	Title        string            `json:"title"`
	SKU          string            `json:"sku"`
	Options      map[string]string `json:"options"`
	Price        *string           `json:"price"`
	PriceAmount  *decimal.Decimal  `json:"price_amount"`
	Currency     string            `json:"currency"`
	Availability string            `json:"availability"`
	ImageURL     string            `json:"image_url"`
}

// Payload is what the browser extension (or another client) submits.
type Payload struct {
	Title            string             `json:"title"`
	URL              string             `json:"url"`
	CanonicalURL     string             `json:"canonical_url"`
	Description      string             `json:"description"`
	Brand            string             `json:"brand"`
	SKU              string             `json:"sku"`
	Model            string             `json:"model"`
	MPN              string             `json:"mpn"`
	GTIN             string             `json:"gtin"`
	Identifiers      map[string]string  `json:"identifiers"`
	Price            *string            `json:"price"`
	PriceAmount      *decimal.Decimal   `json:"price_amount"`
	Currency         string             `json:"currency"`
	Availability     string             `json:"availability"`
	Category         string             `json:"category"`
	Images           []Image            `json:"images"`
	Variants         []Variant          `json:"variants"`
	Specifications   map[string]string  `json:"specifications"`
	SupplierName     string             `json:"supplier_name"`
	SupplierURL      string             `json:"supplier_url"`
	ExtractionMethod string             `json:"extraction_method"`
	AdapterKey       string             `json:"adapter_key"`
	Confidence       *float64           `json:"confidence"`
	FieldConfidence  map[string]float64 `json:"field_confidence"`
	Evidence         map[string]string  `json:"evidence"`
	Warnings         []string           `json:"warnings"`
}

// NormVariant is a validated variant.
type NormVariant struct {
	Title        string            `json:"title"`
	SKU          string            `json:"sku"`
	Options      map[string]string `json:"options"`
	Price        *decimal.Decimal  `json:"price"`
	Currency     *string           `json:"currency"`
	Availability string            `json:"availability"`
	ImageURL     *string           `json:"image_url"`
}

// Normalized is a validated product ready to persist.
type Normalized struct {
	Title            string            `json:"title"`
	NormalizedTitle  string            `json:"normalized_title"`
	SourceURL        *string           `json:"source_url"`
	CanonicalURL     *string           `json:"canonical_url"`
	SourceDomain     string            `json:"source_domain"`
	Description      string            `json:"description"`
	Brand            string            `json:"brand"`
	SKU              string            `json:"sku"`
	Model            string            `json:"model"`
	MPN              string            `json:"mpn"`
	GTIN             *string           `json:"gtin"`
	Identifiers      map[string]string `json:"identifiers"`
	Price            *decimal.Decimal  `json:"price"`
	PriceCurrency    *string           `json:"price_currency"`
	Availability     string            `json:"availability"`
	Category         string            `json:"category"`
	Images           []Image           `json:"images"`
	Variants         []NormVariant     `json:"variants"`
	Specifications   map[string]string `json:"specifications"`
	SupplierName     string            `json:"supplier_name"`
	SupplierDomain   string            `json:"supplier_domain"`
	ExtractionMethod string            `json:"extraction_method"`
	AdapterKey       string            `json:"adapter_key"`
	Confidence       *decimal.Decimal  `json:"confidence"`
	Evidence         map[string]string `json:"evidence"`
	Warnings         []string          `json:"warnings"`
}

// Limits keep stored payloads bounded.
const (
	MaxImages   = 40
	MaxVariants = 250
	MaxSpecs    = 200
)

var methodRe = strings.NewReplacer(" ", "-")

// Options controls normalization.
type Options struct {
	DefaultCurrency string
	RequireURL      bool
}

// Normalize validates a payload and returns the normalized product, warnings and field errors.
func Normalize(p Payload, opt Options) (Normalized, valid.Errors) {
	v := valid.New()
	var w []string
	for _, pw := range p.Warnings {
		if pw = normalize.Text(pw); pw != "" && len(w) < 30 {
			w = append(w, normalize.Truncate(pw, 300))
		}
	}
	n := Normalized{Identifiers: map[string]string{}, Specifications: map[string]string{}, Evidence: map[string]string{},
		Images: []Image{}, Variants: []NormVariant{}, Warnings: []string{}}

	n.Title = normalize.Truncate(normalize.Text(p.Title), 500)
	v.Check(n.Title != "", "title", "is required; select the product title on the page if it was not detected")
	n.NormalizedTitle = normalize.ProductTitle(n.Title)

	if strings.TrimSpace(p.URL) != "" {
		cu, err := normalize.CanonicalURL(p.URL)
		if err != nil {
			v.Add("url", err.Error())
		} else {
			raw := strings.TrimSpace(p.URL)
			n.SourceURL = &raw
			n.CanonicalURL = &cu
			n.SourceDomain = normalize.Domain(cu)
		}
	} else if opt.RequireURL {
		v.Add("url", "is required")
	}
	if p.CanonicalURL != "" && n.CanonicalURL != nil {
		if cu, err := normalize.CanonicalURL(p.CanonicalURL); err == nil {
			if sameSite(normalize.Domain(cu), n.SourceDomain) {
				n.CanonicalURL = &cu
			} else {
				w = append(w, "the page's canonical URL points to another site and was ignored")
			}
		}
	}

	n.Description = normalize.Truncate(normalize.MultilineText(p.Description), 50000)
	n.Brand = normalize.Truncate(normalize.Text(p.Brand), 200)
	n.SKU = normalize.Truncate(normalize.Text(p.SKU), 120)
	n.Model = normalize.Truncate(normalize.Text(p.Model), 200)
	n.MPN = normalize.Truncate(normalize.Text(p.MPN), 120)
	n.Category = normalize.Truncate(normalize.Text(p.Category), 300)
	for k, val := range p.Identifiers {
		k, val = normalize.Truncate(normalize.Text(k), 60), normalize.Truncate(normalize.Text(val), 200)
		if k != "" && val != "" && len(n.Identifiers) < 30 {
			n.Identifiers[k] = val
		}
	}
	if g := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, p.GTIN); g != "" {
		if normalize.ValidGTIN(g) {
			n.GTIN = &g
		} else {
			n.Identifiers["gtin_unverified"] = normalize.Truncate(p.GTIN, 40)
			w = append(w, "the barcode (GTIN) failed its check digit and was stored as an unverified identifier")
		}
	}

	// Price: prefer a numeric amount, otherwise parse the visible text.
	cur := strings.ToUpper(strings.TrimSpace(p.Currency))
	if cur != "" && !normalize.ValidCurrency(cur) {
		w = append(w, fmt.Sprintf("currency %q is not an ISO 4217 code and was ignored", normalize.Truncate(p.Currency, 10)))
		cur = ""
	}
	if p.PriceAmount != nil {
		if p.PriceAmount.IsNegative() {
			v.Add("price_amount", "must not be negative")
		} else {
			amt := p.PriceAmount.Round(4)
			n.Price = &amt
		}
	} else if p.Price != nil && strings.TrimSpace(*p.Price) != "" {
		res, ok := normalize.ParsePrice(*p.Price, cur)
		if ok {
			amt := res.Amount.Round(4)
			n.Price = &amt
			if cur == "" {
				cur = res.Currency
			}
			w = append(w, res.Warnings...)
		} else {
			w = append(w, "the price text could not be read as a number")
		}
	}
	if n.Price != nil {
		if cur == "" {
			cur = opt.DefaultCurrency
			w = append(w, "no currency was shown with the price; assumed "+cur)
		}
		n.PriceCurrency = &cur
		if n.Price.GreaterThan(decimal.NewFromInt(1_000_000_000)) {
			v.Add("price", "is unrealistically large")
		}
	} else {
		w = append(w, "no price was found")
	}

	n.Availability = normalize.Availability(p.Availability)

	seen := map[string]bool{}
	for _, img := range p.Images {
		u := strings.TrimSpace(img.URL)
		if !normalize.ValidHTTPURL(u) || len(u) > 2048 || seen[u] {
			continue
		}
		seen[u] = true
		if len(n.Images) >= MaxImages {
			w = append(w, fmt.Sprintf("only the first %d images were kept", MaxImages))
			break
		}
		img.URL = u
		img.Alt = normalize.Truncate(normalize.Text(img.Alt), 300)
		if img.Width != nil && *img.Width <= 0 {
			img.Width = nil
		}
		if img.Height != nil && *img.Height <= 0 {
			img.Height = nil
		}
		n.Images = append(n.Images, img)
	}
	if len(n.Images) == 0 {
		w = append(w, "no product images were found")
	}

	for i, pv := range p.Variants {
		if i >= MaxVariants {
			w = append(w, fmt.Sprintf("only the first %d variants were kept", MaxVariants))
			break
		}
		nv := NormVariant{Title: normalize.Truncate(normalize.Text(pv.Title), 300), SKU: normalize.Truncate(normalize.Text(pv.SKU), 120),
			Options: map[string]string{}, Availability: normalize.Availability(pv.Availability)}
		keys := make([]string, 0, len(pv.Options))
		for k := range pv.Options {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			kk, vv := normalize.Truncate(normalize.Text(k), 60), normalize.Truncate(normalize.Text(pv.Options[k]), 200)
			if kk != "" && vv != "" && len(nv.Options) < 10 {
				nv.Options[kk] = vv
			}
		}
		if nv.Title == "" {
			parts := make([]string, 0, len(nv.Options))
			for _, k := range keys {
				if val, ok := nv.Options[normalize.Truncate(normalize.Text(k), 60)]; ok {
					parts = append(parts, val)
				}
			}
			nv.Title = strings.Join(parts, " / ")
		}
		vcur := strings.ToUpper(strings.TrimSpace(pv.Currency))
		if !normalize.ValidCurrency(vcur) {
			vcur = ""
		}
		if pv.PriceAmount != nil && !pv.PriceAmount.IsNegative() {
			amt := pv.PriceAmount.Round(4)
			nv.Price = &amt
		} else if pv.Price != nil {
			if res, ok := normalize.ParsePrice(*pv.Price, vcur); ok {
				amt := res.Amount.Round(4)
				nv.Price = &amt
				if vcur == "" {
					vcur = res.Currency
				}
			}
		}
		if nv.Price != nil {
			if vcur == "" && n.PriceCurrency != nil {
				vcur = *n.PriceCurrency
			}
			if vcur != "" {
				nv.Currency = &vcur
			}
		}
		if normalize.ValidHTTPURL(pv.ImageURL) {
			u := pv.ImageURL
			nv.ImageURL = &u
		}
		if nv.Title == "" && nv.SKU == "" {
			continue
		}
		n.Variants = append(n.Variants, nv)
	}

	for k, val := range p.Specifications {
		k, val = normalize.Truncate(normalize.Text(k), 120), normalize.Truncate(normalize.Text(val), 2000)
		if k == "" || val == "" {
			continue
		}
		if len(n.Specifications) >= MaxSpecs {
			w = append(w, fmt.Sprintf("only %d specification rows were kept", MaxSpecs))
			break
		}
		n.Specifications[k] = val
	}

	n.SupplierName = normalize.Truncate(normalize.Text(p.SupplierName), 200)
	if p.SupplierURL != "" && normalize.ValidHTTPURL(p.SupplierURL) {
		n.SupplierDomain = normalize.Domain(p.SupplierURL)
	}

	n.ExtractionMethod = normalize.Truncate(strings.ToLower(methodRe.Replace(normalize.Text(p.ExtractionMethod))), 60)
	if n.ExtractionMethod == "" {
		n.ExtractionMethod = "manual"
	}
	n.AdapterKey = normalize.Truncate(normalize.Text(p.AdapterKey), 60)
	if p.Confidence != nil {
		c := *p.Confidence
		if c < 0 {
			c = 0
		}
		if c > 1 {
			c = 1
		}
		d := decimal.NewFromFloat(c).Round(3)
		n.Confidence = &d
		if c < 0.5 {
			w = append(w, "extraction confidence is low; review every field before approving")
		}
	}
	for k, val := range p.Evidence {
		k = normalize.Truncate(normalize.Text(k), 60)
		if k != "" && len(n.Evidence) < 40 {
			n.Evidence[k] = normalize.Truncate(normalize.Text(val), 1000)
		}
	}
	if n.SKU == "" && n.MPN == "" && n.GTIN == nil && len(n.Identifiers) == 0 {
		w = append(w, "no SKU, MPN or barcode was found; duplicate detection relies on URL and title")
	}
	n.Warnings = dedupe(w)
	return n, v.Errs
}

func dedupe(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// sameSite reports whether two hosts share a registrable-looking suffix (a.shop.com vs shop.com).
func sameSite(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

// ResolveURL makes a possibly relative URL absolute against base.
func ResolveURL(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}
