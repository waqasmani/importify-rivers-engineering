package marketplaces

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// domainMatches mirrors domainMatches in packages/extractor/src/page.ts.
func domainMatches(patterns []string, rawHost string) bool {
	host := strings.TrimSuffix(strings.ToLower(rawHost), ".")
	bare := strings.TrimPrefix(host, "www.")
	for _, p := range patterns {
		switch {
		case p == "*":
			return true
		case strings.HasPrefix(p, "*."):
			if strings.HasSuffix(host, p[1:]) {
				return true
			}
		case bare == strings.TrimPrefix(p, "www."):
			return true
		}
	}
	return false
}

var required = []string{
	"aliexpress", "alibaba-com", "alibaba-1688", "taobao", "tmall", "jd", "dhgate", "cjdropshipping", "banggood", "geekbuying", "tomtop",
	"saleyee", "temu", "shein", "rosegal", "wish", "walmart", "etsy", "kohls", "costco", "bedbathandbeyond", "bonanza", "ltdcommodities",
	"allstarhealth", "flipkart", "snapdeal", "lazada", "shopee", "amazon", "ebay", "nordstrom",
}

func TestShippedDefinitionsAreEmbeddedAndValid(t *testing.T) {
	defs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(defs))
	for _, d := range defs {
		keys = append(keys, d.Key)
	}
	want := append([]string{}, required...)
	sort.Strings(want)
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("shipped definitions:\n got %v\nwant %v", keys, want)
	}
	root := repoRoot(t)
	for _, d := range defs {
		// Honesty: nothing has been checked against live pages from this repository.
		if d.Verification.Status != "synthetic_fixture" {
			t.Errorf("%s: verification status %q; shipped definitions are only checked against synthetic fixtures", d.Key, d.Verification.Status)
		}
		if d.Verification.Fixture != "testdata/fixtures/marketplaces/"+d.Key+".html" {
			t.Errorf("%s: fixture path %q", d.Key, d.Verification.Fixture)
		}
		if _, err := os.Stat(filepath.Join(root, d.Verification.Fixture)); err != nil {
			t.Errorf("%s: fixture missing: %v", d.Key, err)
		}
		u, err := url.Parse(d.Verification.FixtureURL)
		if err != nil || !domainMatches(d.Domains, u.Hostname()) {
			t.Errorf("%s: fixture URL %q does not match the domains", d.Key, d.Verification.FixtureURL)
		}
		fields := strings.Join(FieldsOf(d.Rules), ",")
		for _, f := range []string{"title", "price", "images"} {
			if !strings.Contains(fields, f) {
				t.Errorf("%s: does not extract %s", d.Key, f)
			}
		}
	}
}

func TestLoadRejectsInvalidDefinitions(t *testing.T) {
	good := `{"key":"shop","name":"Shop","region":"global","description":"d","domains":["shop.example"],"priority":60,"version":1,
"verification":{"status":"synthetic_fixture","note":"assumes h1 and .price on product pages","fixture":"f","fixture_url":"https://shop.example/p"},
"rules":{"schema":1,"product_signal":{"any":[{"css":"h1"}]},"fields":{"title":[{"css":"h1"}],"price":[{"css":".price"}],"images":[{"css":"img"}]}}}`
	if _, err := LoadFS(fstest.MapFS{"m/shop.json": {Data: []byte(good)}}, "m"); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}
	cases := map[string]string{
		"wrong file name":     "m/other.json",
		"unknown key":         "m/shop.json",
		"verified claim":      "m/shop.json",
		"bad domain":          "m/shop.json",
		"invalid rule":        "m/shop.json",
		"unknown region":      "m/shop.json",
		"missing description": "m/shop.json",
	}
	bodies := map[string]string{
		"wrong file name":     good,
		"unknown key":         strings.Replace(good, `"priority":60`, `"priority":60,"script":"x"`, 1),
		"verified claim":      strings.Replace(good, `"synthetic_fixture"`, `"verified"`, 1),
		"bad domain":          strings.Replace(good, `["shop.example"]`, `["*"]`, 1),
		"invalid rule":        strings.Replace(good, `{"css":".price"}`, `{"css":".price","regex":"(a+)+"}`, 1),
		"unknown region":      strings.Replace(good, `"global"`, `"mars"`, 1),
		"missing description": strings.Replace(good, `"note":"assumes h1 and .price on product pages"`, `"note":""`, 1),
	}
	for name, file := range cases {
		if _, err := LoadFS(fstest.MapFS{file: {Data: []byte(bodies[name])}}, "m"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
