package marketplaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// TestValidationCasesMatchTheExtractor runs the cases shared with packages/extractor so both
// validators accept and reject the same rule documents.
func TestValidationCasesMatchTheExtractor(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "testdata", "rules", "validation-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Name  string          `json:"name"`
			Valid bool            `json:"valid"`
			Doc   json.RawMessage `json:"doc"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) < 30 {
		t.Fatalf("expected the shared validation cases, got %d", len(file.Cases))
	}
	for _, c := range file.Cases {
		problems := ValidateRules(c.Doc)
		if (len(problems) == 0) != c.Valid {
			t.Errorf("%s: valid=%v but problems=%v", c.Name, c.Valid, problems)
		}
	}
}

func TestValidateRulesReportsLocations(t *testing.T) {
	doc := `{"schema":1,"product_signal":{"any":[{"css":"#p"}]},"fields":{"title":[{"css":"h1","attr":"value"}],"price":[{"source":"state","path":"a"}]}}`
	got := strings.Join(ValidateRules([]byte(doc)), "\n")
	for _, want := range []string{
		"fields.title[0].attr: form values, event handlers and token-like attributes cannot be read",
		`fields.price[0].source: unknown source "state"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if p := ValidateRules([]byte(`{"schema":1} trailing`)); len(p) != 1 || !strings.Contains(p[0], "not valid JSON") {
		t.Errorf("trailing data must be rejected: %v", p)
	}
	big := `{"schema":1,"fields":{"title":[{"css":"` + strings.Repeat("a", MaxRulesBytes) + `"}]}}`
	if p := ValidateRules([]byte(big)); len(p) != 1 || !strings.Contains(p[0], "limited") {
		t.Errorf("oversized documents must be rejected: %v", p)
	}
}

func TestParsePath(t *testing.T) {
	segs, err := ParsePath(`a.b[0][*]["c d"][?name='Brand Name'].*`)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{}
	for _, s := range segs {
		kinds = append(kinds, s.Kind)
	}
	if strings.Join(kinds, ",") != "key,key,index,wild,key,filter,wild" || segs[4].Key != "c d" || segs[5].Value != "Brand Name" {
		t.Fatalf("unexpected segments %+v", segs)
	}
	for _, bad := range []string{"a..b", "a.", ".a", "a[", "a[x]", "a b", "a.b()", "a.__proto__", "csrfToken", "user['session_id']", "a[?csrf=1]", "a[?k='x]"} {
		if _, err := ParsePath(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if _, err := ParsePath(""); err != nil {
		t.Errorf("the empty path is the value itself: %v", err)
	}
}

func TestFieldsOf(t *testing.T) {
	doc := []byte(`{"fields":{"price":[],"title":[],"images":[]},"variants":{"mode":"list"}}`)
	if got := strings.Join(FieldsOf(doc), ","); got != "title,price,images,variants" {
		t.Fatalf("FieldsOf = %s", got)
	}
}

func TestSensitiveNamesMirrorTheExtractor(t *testing.T) {
	for _, n := range []string{"csrfToken", "_csrf", "sessionId", "password", "api_key", "x-auth", "jwt", "nonce", "Authorization"} {
		if !IsSensitiveName(n) {
			t.Errorf("%s should be sensitive", n)
		}
	}
	for _, n := range []string{"author", "title", "skuPropIds", "priceText", "authority"} {
		if IsSensitiveName(n) {
			t.Errorf("%s should not be sensitive", n)
		}
	}
}
