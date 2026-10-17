// Package marketplaces holds the declarative "rules" adapters: validation of rule documents
// (mirroring packages/extractor/src/rules/schema.ts), the marketplace definitions shipped in
// db/marketplaces and their synchronisation into source_adapters at start-up.
package marketplaces

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
)

// MaxRulesBytes is the largest rule document accepted.
const MaxRulesBytes = 64000

// RulesSchemaVersion is the rule language version this server understands.
const RulesSchemaVersion = 1

// RuleFields are the fields a rule document may fill (same order as the extractor).
var RuleFields = []string{"title", "description", "brand", "sku", "model", "mpn", "gtin", "price", "currency", "list_price", "availability",
	"category", "images", "specifications", "supplier_name", "supplier_url", "canonical_url", "shipping", "rating", "rating_count"}

var simpleTransforms = set("trim", "strip_html", "number", "cents", "first", "last", "absolute_url", "protocol_fix", "image_original",
	"lowercase", "uppercase", "digits", "stock", "join", "keys")

// sensitiveName mirrors SENSITIVE_NAME in packages/extractor/src/sensitive.ts.
var sensitiveName = regexp.MustCompile(`(?i)(pass(word|wd|code|phrase)|^pass$|^pwd$|secret|token|csrf|xsrf|nonce|session|^sess(id)?$|cookie|credential|api[-_.]?key|private[-_.]?key|signature|^otp$|^auth$|^authorization$|[-_.]auth\b|^auth[-_.]|bearer|^jwt$|[-_.]jwt$)`)

var (
	nameRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	metaRe   = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,100}$`)
	attrRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_:.-]{0,60}$`)
	hostRe   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}\.)*[a-z0-9-]{1,63}\.[a-z]{2,63}$`)
	typeRe   = regexp.MustCompile(`^[A-Za-z]{1,60}$`)
	prefixRe = regexp.MustCompile(`^(https?:)?//[^\s"'<>]+$`)
)

// IsSensitiveName reports whether a key, attribute or meta name looks like a credential or token.
func IsSensitiveName(name string) bool { return sensitiveName.MatchString(strings.TrimSpace(name)) }

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, s := range items {
		m[s] = true
	}
	return m
}

type checker struct {
	problems []string
	sources  map[string]bool
	refs     []ref
}

type ref struct{ name, where string }

func (c *checker) add(where, format string, args ...any) {
	if len(c.problems) < 50 {
		msg := fmt.Sprintf(format, args...)
		if where != "" {
			msg = where + ": " + msg
		}
		c.problems = append(c.problems, msg)
	}
}

func join(where, key string) string {
	if where == "" {
		return key
	}
	return where + "." + key
}

func idx(where string, i int) string { return fmt.Sprintf("%s[%d]", where, i) }

// object checks that v is a JSON object whose keys are all allowed.
func (c *checker) object(where string, v any, allowed ...string) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		c.add(where, "must be an object")
		return nil, false
	}
	allow := set(allowed...)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !allow[k] {
			c.add(join(where, k), "is not a recognised key")
		}
	}
	return m, true
}

func (c *checker) str(where string, v any, min, max int) (string, bool) {
	s, ok := v.(string)
	if !ok {
		c.add(where, "must be a string")
		return "", false
	}
	if n := utf8.RuneCountInString(s); n < min || n > max {
		c.add(where, "must have %d-%d characters", min, max)
		return s, false
	}
	return s, true
}

func (c *checker) boolean(where string, v any) {
	if _, ok := v.(bool); !ok {
		c.add(where, "must be true or false")
	}
}

func (c *checker) number(where string, v any, min, max float64) {
	n, ok := v.(json.Number)
	if !ok {
		c.add(where, "must be a number")
		return
	}
	f, err := n.Float64()
	if err != nil || f < min || f > max {
		c.add(where, "must be between %g and %g", min, max)
	}
}

func (c *checker) array(where string, v any, min, max int) ([]any, bool) {
	a, ok := v.([]any)
	if !ok {
		c.add(where, "must be a list")
		return nil, false
	}
	if len(a) < min || len(a) > max {
		c.add(where, "must have %d-%d entries", min, max)
	}
	return a, true
}

func (c *checker) path(where string, v any) {
	s, ok := c.str(where, v, 0, 300)
	if !ok {
		return
	}
	if _, err := ParsePath(s); err != nil {
		c.add(where, "invalid path %q: %v", truncate(s, 60), err)
	}
}

func (c *checker) selector(where string, v any) {
	s, ok := c.str(where, v, 1, 300)
	if ok && strings.ContainsAny(s, "<>{}") {
		c.add(where, "CSS selectors cannot contain <, >, { or }")
	}
}

func (c *checker) attr(where string, v any) {
	s, ok := c.str(where, v, 1, 61)
	if !ok {
		return
	}
	if !attrRe.MatchString(s) {
		c.add(where, "attribute names are letters, digits, \"-\", \"_\", \":\" or \".\"")
	} else if strings.EqualFold(s, "value") || strings.HasPrefix(strings.ToLower(s), "on") || IsSensitiveName(s) {
		c.add(where, "form values, event handlers and token-like attributes cannot be read")
	}
}

func (c *checker) metaName(where string, v any) {
	s, ok := c.str(where, v, 1, 100)
	if !ok {
		return
	}
	if !metaRe.MatchString(s) {
		c.add(where, "meta names are letters, digits, \":\", \"_\", \".\" or \"-\"")
	} else if IsSensitiveName(s) {
		c.add(where, "token-like meta tags cannot be read")
	}
}

func (c *checker) currency(where string, v any) {
	s, ok := v.(string)
	if !ok || !normalize.ValidCurrency(s) {
		c.add(where, "must be an ISO 4217 currency code such as USD")
	}
}

func (c *checker) sourceRef(where string, v any) {
	s, ok := c.str(where, v, 1, 40)
	if !ok {
		return
	}
	if !nameRe.MatchString(s) {
		c.add(where, "source names are lowercase letters, digits and \"_\" (up to 40)")
		return
	}
	c.refs = append(c.refs, ref{name: s, where: where})
}

func (c *checker) transforms(where string, v any) {
	list, ok := c.array(where, v, 0, 8)
	if !ok {
		return
	}
	for i, t := range list {
		w := idx(where, i)
		switch x := t.(type) {
		case string:
			if !simpleTransforms[x] {
				c.add(w, "unknown transform %q", truncate(x, 40))
			}
		case map[string]any:
			if len(x) != 1 {
				c.add(w, "a parameterised transform has exactly one key: join, split or prefix")
				continue
			}
			for k, p := range x {
				switch k {
				case "join":
					c.str(join(w, k), p, 0, 10)
				case "split":
					c.str(join(w, k), p, 1, 10)
				case "prefix":
					if s, ok := c.str(join(w, k), p, 0, 200); ok && !prefixRe.MatchString(s) {
						c.add(join(w, k), "a prefix must be an http(s) or protocol-relative URL")
					}
				default:
					c.add(join(w, k), "unknown transform")
				}
			}
		default:
			c.add(w, "must be a transform name or object")
		}
	}
}

func (c *checker) source(where string, v any) {
	m, ok := v.(map[string]any)
	if !ok {
		c.add(where, "must be an object")
		return
	}
	kind, _ := m["kind"].(string)
	switch kind {
	case "json_ld":
		c.object(where, v, "kind", "types")
		if list, ok := c.array(join(where, "types"), m["types"], 1, 10); ok {
			for i, t := range list {
				if s, ok := t.(string); !ok || !typeRe.MatchString(s) {
					c.add(idx(join(where, "types"), i), "must be a schema.org type name")
				}
			}
		}
	case "script_json":
		c.object(where, v, "kind", "selector")
		c.selector(join(where, "selector"), m["selector"])
	case "script_marker":
		c.object(where, v, "kind", "marker", "selector")
		c.str(join(where, "marker"), m["marker"], 3, 100)
		if s, ok := m["selector"]; ok {
			c.selector(join(where, "selector"), s)
		}
	case "attr_json":
		c.object(where, v, "kind", "selector", "attr")
		c.selector(join(where, "selector"), m["selector"])
		c.attr(join(where, "attr"), m["attr"])
	default:
		c.add(join(where, "kind"), "must be json_ld, script_json, script_marker or attr_json")
	}
}

func has(m map[string]any, k string) bool { _, ok := m[k]; return ok }

func count(m map[string]any, keys ...string) int {
	n := 0
	for _, k := range keys {
		if has(m, k) {
			n++
		}
	}
	return n
}

func (c *checker) candidate(where string, v any) {
	m, ok := c.object(where, v, "source", "path", "entry", "meta", "css", "attr", "all", "multiline", "table", "pairs", "transforms", "confidence")
	if !ok {
		return
	}
	if count(m, "source", "meta", "css") != 1 {
		c.add(where, "each candidate reads exactly one of \"source\", \"meta\" or \"css\"")
	}
	if !has(m, "source") && (has(m, "path") || has(m, "entry")) {
		c.add(where, "\"path\" and \"entry\" need a \"source\"")
	}
	if !has(m, "css") && count(m, "attr", "all", "multiline", "table", "pairs") > 0 {
		c.add(where, "\"attr\", \"all\", \"multiline\", \"table\" and \"pairs\" need a \"css\" selector")
	}
	if s, ok := m["source"]; ok {
		c.sourceRef(join(where, "source"), s)
	}
	if p, ok := m["path"]; ok {
		c.path(join(where, "path"), p)
	}
	if e, ok := m["entry"]; ok {
		if em, ok := c.object(join(where, "entry"), e, "name", "value"); ok {
			c.path(join(where, "entry.name"), em["name"])
			c.path(join(where, "entry.value"), em["value"])
		}
	}
	if s, ok := m["meta"]; ok {
		c.metaName(join(where, "meta"), s)
	}
	if s, ok := m["css"]; ok {
		c.selector(join(where, "css"), s)
	}
	if a, ok := m["attr"]; ok {
		c.attr(join(where, "attr"), a)
	}
	for _, k := range []string{"all", "multiline", "table"} {
		if b, ok := m[k]; ok {
			c.boolean(join(where, k), b)
		}
	}
	if p, ok := m["pairs"]; ok {
		if pm, ok := c.object(join(where, "pairs"), p, "name", "value"); ok {
			c.selector(join(where, "pairs.name"), pm["name"])
			c.selector(join(where, "pairs.value"), pm["value"])
		}
	}
	if t, ok := m["transforms"]; ok {
		c.transforms(join(where, "transforms"), t)
	}
	if n, ok := m["confidence"]; ok {
		c.number(join(where, "confidence"), n, 0.1, 0.99)
	}
}

func (c *checker) condition(where string, v any) {
	m, ok := c.object(where, v, "source", "path", "css", "meta", "url_path", "equals")
	if !ok {
		return
	}
	if count(m, "source", "css", "meta", "url_path") != 1 {
		c.add(where, "each condition checks exactly one of \"source\", \"css\", \"meta\" or \"url_path\"")
	}
	if has(m, "path") && !has(m, "source") {
		c.add(where, "\"path\" needs a \"source\"")
	}
	if has(m, "equals") && !has(m, "source") && !has(m, "meta") {
		c.add(where, "\"equals\" applies to \"source\" or \"meta\" conditions")
	}
	if s, ok := m["source"]; ok {
		c.sourceRef(join(where, "source"), s)
	}
	if p, ok := m["path"]; ok {
		c.path(join(where, "path"), p)
	}
	if s, ok := m["css"]; ok {
		c.selector(join(where, "css"), s)
	}
	if s, ok := m["meta"]; ok {
		c.metaName(join(where, "meta"), s)
	}
	if s, ok := m["url_path"]; ok {
		c.str(join(where, "url_path"), s, 1, 100)
	}
	if s, ok := m["equals"]; ok {
		c.str(join(where, "equals"), s, 1, 100)
	}
}

func (c *checker) varField(where string, v any) {
	switch x := v.(type) {
	case string:
		c.path(where, x)
	case map[string]any:
		m, _ := c.object(where, v, "path", "transforms")
		c.path(join(where, "path"), m["path"])
		if t, ok := m["transforms"]; ok {
			c.transforms(join(where, "transforms"), t)
		}
	default:
		c.add(where, "must be a path or {path, transforms}")
	}
}

var itemFieldKeys = []string{"sku", "price", "currency", "stock", "availability", "image"}

func (c *checker) itemFields(where string, m map[string]any) {
	for _, k := range itemFieldKeys {
		if v, ok := m[k]; ok {
			c.varField(join(where, k), v)
		}
	}
}

func (c *checker) variants(where string, v any) {
	m, ok := v.(map[string]any)
	if !ok {
		c.add(where, "must be an object")
		return
	}
	switch m["mode"] {
	case "list":
		c.object(where, v, append([]string{"mode", "source", "path", "title", "options", "option_list", "option_positions", "options_rest", "confidence"}, itemFieldKeys...)...)
		c.sourceRef(join(where, "source"), m["source"])
		c.path(join(where, "path"), m["path"])
		if t, ok := m["title"]; ok {
			c.varField(join(where, "title"), t)
		}
		c.itemFields(where, m)
		if o, ok := m["options"]; ok {
			if om, ok := o.(map[string]any); !ok {
				c.add(join(where, "options"), "must be an object")
			} else {
				if len(om) > 10 {
					c.add(join(where, "options"), "at most 10 options")
				}
				for name, f := range om {
					if n := utf8.RuneCountInString(name); n < 1 || n > 60 {
						c.add(join(where, "options"), "option names must have 1-60 characters")
					}
					c.varField(join(join(where, "options"), name), f)
				}
			}
		}
		if o, ok := m["option_list"]; ok {
			if om, ok := c.object(join(where, "option_list"), o, "path", "name", "value"); ok {
				for _, k := range []string{"path", "name", "value"} {
					c.path(join(where, "option_list."+k), om[k])
				}
			}
		}
		if o, ok := m["option_positions"]; ok {
			if om, ok := c.object(join(where, "option_positions"), o, "values", "names"); ok {
				c.path(join(where, "option_positions.values"), om["values"])
				if nm, ok := c.object(join(where, "option_positions.names"), om["names"], "source", "path"); ok {
					c.sourceRef(join(where, "option_positions.names.source"), nm["source"])
					c.path(join(where, "option_positions.names.path"), nm["path"])
				}
			}
		}
		if o, ok := m["options_rest"]; ok {
			if om, ok := c.object(join(where, "options_rest"), o, "exclude"); ok {
				if list, ok := c.array(join(where, "options_rest.exclude"), om["exclude"], 0, 20); ok {
					for i, x := range list {
						c.str(idx(join(where, "options_rest.exclude"), i), x, 1, 60)
					}
				}
			}
		}
	case "property_join":
		c.object(where, v, "mode", "properties", "skus", "offers", "confidence")
		if pm, ok := c.object(join(where, "properties"), m["properties"], "source", "path", "id", "name", "values", "value_id", "value_name", "value_image"); ok {
			w := join(where, "properties")
			c.sourceRef(join(w, "source"), pm["source"])
			c.path(join(w, "path"), pm["path"])
			c.path(join(w, "values"), pm["values"])
			for _, k := range []string{"name", "value_id", "value_name"} {
				c.varField(join(w, k), pm[k])
			}
			for _, k := range []string{"id", "value_image"} {
				if f, ok := pm[k]; ok {
					c.varField(join(w, k), f)
				}
			}
		}
		if sm, ok := c.object(join(where, "skus"), m["skus"], append([]string{"source", "path", "ids", "separator", "match"}, itemFieldKeys...)...); ok {
			w := join(where, "skus")
			c.sourceRef(join(w, "source"), sm["source"])
			c.path(join(w, "path"), sm["path"])
			c.varField(join(w, "ids"), sm["ids"])
			if s, ok := sm["separator"]; ok {
				c.str(join(w, "separator"), s, 1, 10)
			}
			if s, ok := sm["match"]; ok && s != "id" && s != "name" {
				c.add(join(w, "match"), "must be id or name")
			}
			c.itemFields(w, sm)
		}
		if o, ok := m["offers"]; ok {
			if om, ok := c.object(join(where, "offers"), o, append([]string{"source", "path", "key", "match"}, itemFieldKeys...)...); ok {
				w := join(where, "offers")
				c.sourceRef(join(w, "source"), om["source"])
				c.path(join(w, "path"), om["path"])
				c.varField(join(w, "key"), om["key"])
				if p, ok := om["match"]; ok {
					c.path(join(w, "match"), p)
				}
				c.itemFields(w, om)
			}
		}
	default:
		c.add(join(where, "mode"), "must be list or property_join")
	}
	if n, ok := m["confidence"]; ok {
		c.number(join(where, "confidence"), n, 0.1, 0.99)
	}
}

func (c *checker) document(v any) {
	m, ok := c.object("", v, "schema", "product_signal", "sources", "fields", "variants", "currency", "currency_by_host", "confidence")
	if !ok {
		return
	}
	if n, ok := m["schema"].(json.Number); !ok || n.String() != fmt.Sprint(RulesSchemaVersion) {
		c.add("schema", "must be %d", RulesSchemaVersion)
	}
	c.sources = map[string]bool{}
	if s, ok := m["sources"]; ok {
		if sm, ok := s.(map[string]any); !ok {
			c.add("sources", "must be an object")
		} else {
			if len(sm) > 16 {
				c.add("sources", "at most 16 sources")
			}
			for name, def := range sm {
				if !nameRe.MatchString(name) {
					c.add(join("sources", name), "source names are lowercase letters, digits and \"_\" (up to 40)")
				}
				c.sources[name] = true
				c.source(join("sources", name), def)
			}
		}
	}
	if s, ok := m["product_signal"]; !ok {
		c.add("product_signal", "is required")
	} else if sm, ok := c.object("product_signal", s, "any", "all"); ok {
		if !has(sm, "any") && !has(sm, "all") {
			c.add("product_signal", "the product signal needs \"any\" or \"all\" conditions")
		}
		for _, k := range []string{"any", "all"} {
			if list, ok := sm[k]; ok {
				if a, ok := c.array(join("product_signal", k), list, 1, 10); ok {
					for i, cond := range a {
						c.condition(idx(join("product_signal", k), i), cond)
					}
				}
			}
		}
	}
	allowedFields := set(RuleFields...)
	if f, ok := m["fields"]; !ok {
		c.add("fields", "is required")
	} else if fm, ok := f.(map[string]any); !ok {
		c.add("fields", "must be an object")
	} else {
		names := make([]string, 0, len(fm))
		for k := range fm {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			w := join("fields", name)
			if !allowedFields[name] {
				c.add(w, "is not a recognised field")
				continue
			}
			if a, ok := c.array(w, fm[name], 1, 10); ok {
				for i, cand := range a {
					c.candidate(idx(w, i), cand)
				}
			}
		}
		if t, ok := fm["title"].([]any); !ok || len(t) == 0 {
			c.add("fields.title", "a title candidate is required")
		}
	}
	if v, ok := m["variants"]; ok {
		c.variants("variants", v)
	}
	if cur, ok := m["currency"]; ok {
		c.currency("currency", cur)
	}
	if h, ok := m["currency_by_host"]; ok {
		if hm, ok := h.(map[string]any); !ok {
			c.add("currency_by_host", "must be an object")
		} else {
			if len(hm) > 60 {
				c.add("currency_by_host", "at most 60 hosts")
			}
			for host, cur := range hm {
				if !hostRe.MatchString(host) {
					c.add("currency_by_host", "keys are host names such as amazon.co.uk")
				}
				c.currency(join("currency_by_host", host), cur)
			}
		}
	}
	if n, ok := m["confidence"]; ok {
		c.number("confidence", n, 0.3, 0.99)
	}
	for _, r := range c.refs {
		if !c.sources[r.name] {
			c.add(r.where, "unknown source %q", r.name)
		}
	}
}

// ValidateRules checks a rule document and returns the problems found (empty when valid).
func ValidateRules(raw []byte) []string {
	if len(raw) > MaxRulesBytes {
		return []string{fmt.Sprintf("rule documents are limited to %d kB", MaxRulesBytes/1000)}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return []string{"not valid JSON: " + err.Error()}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return []string{"not valid JSON: unexpected data after the document"}
	}
	c := &checker{}
	c.document(v)
	return c.problems
}

// FieldsOf lists the fields a valid rule document fills, in display order, plus "variants".
func FieldsOf(raw []byte) []string {
	var doc struct {
		Fields   map[string]json.RawMessage `json:"fields"`
		Variants json.RawMessage            `json:"variants"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var out []string
	for _, f := range RuleFields {
		if _, ok := doc.Fields[f]; ok {
			out = append(out, f)
		}
	}
	if len(doc.Variants) > 0 && string(doc.Variants) != "null" {
		out = append(out, "variants")
	}
	return out
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
