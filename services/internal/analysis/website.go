package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func init() { register(websiteAnalyzer{}) }

type websiteAnalyzer struct{}

func (websiteAnalyzer) Name() string { return "website" }

// techSignature identifies a client-visible technology marker.
type techSignature struct {
	name, category string
	needles        []string
	confidence     float64
}

var techSignatures = []techSignature{
	{"WordPress", "cms", []string{"/wp-content/", "/wp-includes/", "content=\"wordpress"}, 0.9},
	{"WooCommerce", "ecommerce", []string{"woocommerce", "/wc-ajax", "wc-block"}, 0.85},
	{"Shopify", "ecommerce", []string{"cdn.shopify.com", "shopify.theme", "myshopify.com", "shopify-section"}, 0.9},
	{"Magento / Adobe Commerce", "ecommerce", []string{"/static/version", "mage/cookies", "magento_"}, 0.75},
	{"BigCommerce", "ecommerce", []string{"cdn11.bigcommerce.com", "bigcommerce.com/s-"}, 0.85},
	{"PrestaShop", "ecommerce", []string{"prestashop", "/modules/ps_"}, 0.75},
	{"Squarespace", "site_builder", []string{"static.squarespace.com", "squarespace-cdn"}, 0.9},
	{"Wix", "site_builder", []string{"wixstatic.com", "wix-code"}, 0.9},
	{"Webflow", "site_builder", []string{"assets.website-files.com", "webflow.js", "data-wf-page"}, 0.9},
	{"Drupal", "cms", []string{"drupal-settings-json", "/sites/default/files", "drupal.js"}, 0.8},
	{"Next.js", "frontend_framework", []string{"/_next/static", "__next_data__"}, 0.9},
	{"Nuxt", "frontend_framework", []string{"/_nuxt/", "__nuxt"}, 0.85},
	{"Gatsby", "frontend_framework", []string{"___gatsby", "/page-data/"}, 0.8},
	{"React", "frontend_library", []string{"data-reactroot", "react-dom", "__react"}, 0.6},
	{"Vue.js", "frontend_library", []string{"data-v-", "vue.runtime", "vue.global", "data-server-rendered"}, 0.6},
	{"Angular", "frontend_framework", []string{"ng-version=", "ng-app", "_nghost"}, 0.8},
	{"Svelte", "frontend_framework", []string{"svelte-", "__sveltekit"}, 0.6},
	{"jQuery", "frontend_library", []string{"jquery.min.js", "jquery.js", "/jquery-"}, 0.8},
	{"Bootstrap", "ui_framework", []string{"bootstrap.min.css", "bootstrap.bundle", "bootstrap.min.js"}, 0.75},
	{"Google Tag Manager", "analytics", []string{"googletagmanager.com"}, 0.9},
	{"Google Analytics", "analytics", []string{"google-analytics.com", "gtag("}, 0.85},
	{"Meta Pixel", "analytics", []string{"connect.facebook.net", "fbq("}, 0.85},
	{"Stripe", "payments", []string{"js.stripe.com"}, 0.9},
	{"PayPal", "payments", []string{"paypal.com/sdk", "paypalobjects.com"}, 0.85},
	{"Cloudflare", "infrastructure", []string{"/cdn-cgi/", "cdnjs.cloudflare.com"}, 0.6},
	{"reCAPTCHA", "security", []string{"google.com/recaptcha", "g-recaptcha"}, 0.9},
	{"hCaptcha", "security", []string{"hcaptcha.com"}, 0.9},
	{"Intercom", "support", []string{"widget.intercom.io"}, 0.9},
	{"Zendesk", "support", []string{"zdassets.com"}, 0.85},
	{"Hotjar", "analytics", []string{"static.hotjar.com"}, 0.9},
}

type page struct {
	base       *url.URL
	title      string
	lang       string
	metas      map[string]string
	canonical  string
	headings   []heading
	navLinks   []link
	links      []link
	forms      []form
	images     int
	missingAlt int
	scripts    []string
	styles     []string
	jsonLD     []string
	rawLower   string
	itemTypes  []string
	tables     int
}

type heading struct {
	level int
	text  string
}

type link struct {
	text, href string
}

type form struct {
	id, name, action, method string
	fields                   []formField
	locator                  string
}

type formField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Label    string `json:"label,omitempty"`
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		if n.Type == html.ElementNode && (n.DataAtom == atom.Script || n.DataAtom == atom.Style) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func locatorFor(n *html.Node) string {
	loc := n.Data
	if id := attr(n, "id"); id != "" {
		return loc + "#" + id
	}
	if cls := strings.Fields(attr(n, "class")); len(cls) > 0 {
		loc += "." + cls[0]
	}
	if name := attr(n, "name"); name != "" {
		loc += "[name=\"" + name + "\"]"
	}
	return loc
}

func parsePage(src, pageURL string) (*page, error) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, err
	}
	p := &page{metas: map[string]string{}, rawLower: strings.ToLower(src)}
	if pageURL != "" {
		p.base, _ = url.Parse(pageURL)
	}
	var inNav int
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Html:
				p.lang = attr(n, "lang")
			case atom.Title:
				if p.title == "" {
					p.title = textOf(n)
				}
			case atom.Meta:
				key := strings.ToLower(attr(n, "name"))
				if key == "" {
					key = strings.ToLower(attr(n, "property"))
				}
				if key != "" && len(p.metas) < 200 {
					p.metas[key] = attr(n, "content")
				}
			case atom.Link:
				rel := strings.ToLower(attr(n, "rel"))
				if rel == "canonical" {
					p.canonical = attr(n, "href")
				}
				if strings.Contains(rel, "stylesheet") {
					p.styles = append(p.styles, attr(n, "href"))
				}
			case atom.H1, atom.H2, atom.H3:
				if t := textOf(n); t != "" && len(p.headings) < 200 {
					p.headings = append(p.headings, heading{int(n.Data[1] - '0'), t})
				}
			case atom.Nav, atom.Header:
				inNav++
				defer func() { inNav-- }()
			case atom.A:
				href := attr(n, "href")
				if href != "" && !strings.HasPrefix(strings.ToLower(href), "javascript:") && len(p.links) < 5000 {
					l := link{text: textOf(n), href: href}
					p.links = append(p.links, l)
					if inNav > 0 && len(p.navLinks) < 100 {
						p.navLinks = append(p.navLinks, l)
					}
				}
			case atom.Img:
				p.images++
				if strings.TrimSpace(attr(n, "alt")) == "" {
					p.missingAlt++
				}
			case atom.Script:
				if src := attr(n, "src"); src != "" {
					p.scripts = append(p.scripts, src)
				}
				if strings.EqualFold(attr(n, "type"), "application/ld+json") && n.FirstChild != nil {
					p.jsonLD = append(p.jsonLD, n.FirstChild.Data)
				}
			case atom.Table:
				p.tables++
			case atom.Form:
				p.forms = append(p.forms, parseForm(n))
			}
			if it := attr(n, "itemtype"); it != "" {
				p.itemTypes = append(p.itemTypes, it)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return p, nil
}

func parseForm(n *html.Node) form {
	f := form{id: attr(n, "id"), name: attr(n, "name"), action: attr(n, "action"), method: strings.ToUpper(attr(n, "method")), locator: locatorFor(n)}
	if f.method == "" {
		f.method = "GET"
	}
	labels := map[string]string{}
	var collect func(*html.Node)
	collect = func(c *html.Node) {
		if c.Type == html.ElementNode {
			switch c.DataAtom {
			case atom.Label:
				if forID := attr(c, "for"); forID != "" {
					labels[forID] = textOf(c)
				}
			case atom.Input, atom.Select, atom.Textarea:
				typ := strings.ToLower(attr(c, "type"))
				if c.DataAtom == atom.Select {
					typ = "select"
				} else if c.DataAtom == atom.Textarea {
					typ = "textarea"
				} else if typ == "" {
					typ = "text"
				}
				if typ == "hidden" || typ == "submit" || typ == "button" || typ == "image" || typ == "reset" {
					break
				}
				name := attr(c, "name")
				if name == "" {
					name = attr(c, "id")
				}
				_, req := attrPresent(c, "required")
				ff := formField{Name: name, Type: typ, Required: req, Label: labels[attr(c, "id")]}
				if ff.Label == "" {
					ff.Label = attr(c, "aria-label")
				}
				if ff.Label == "" {
					ff.Label = attr(c, "placeholder")
				}
				if len(f.fields) < 100 {
					f.fields = append(f.fields, ff)
				}
			}
		}
		for ch := c.FirstChild; ch != nil; ch = ch.NextSibling {
			collect(ch)
		}
	}
	collect(n)
	for i := range f.fields {
		if f.fields[i].Label == "" {
			if l, ok := labels[f.fields[i].Name]; ok {
				f.fields[i].Label = l
			}
		}
	}
	return f
}

func attrPresent(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val, true
		}
	}
	return "", false
}

func (p *page) resolve(href string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return nil, false
	}
	if p.base != nil {
		u = p.base.ResolveReference(u)
	}
	return u, u.Scheme == "http" || u.Scheme == "https"
}

func (websiteAnalyzer) Analyze(ctx context.Context, in Input) (Result, error) {
	b := newBuilder()
	stats := map[string]any{}
	src := in.Text
	if strings.TrimSpace(src) == "" {
		b.unknown("coverage", "No page content was available",
			"The source has no captured HTML. Capture the page with the browser extension, upload a saved HTML file, or enable server-side fetching for public pages.")
		addCapturedFindings(b, in)
		return Result{Findings: b.out, Stats: map[string]any{"html_bytes": 0}}, nil
	}
	p, err := parsePage(src, in.URL)
	if err != nil {
		return Result{}, fmt.Errorf("parse HTML: %w", err)
	}
	stats["html_bytes"] = len(src)
	stats["links"] = len(p.links)
	stats["forms"] = len(p.forms)
	stats["images"] = p.images

	// Metadata.
	meta := map[string]any{"title": p.title, "lang": p.lang, "canonical": p.canonical}
	var metaLines []string
	if p.title != "" {
		metaLines = append(metaLines, "Title: "+p.title)
	}
	for _, k := range []string{"description", "robots", "generator", "viewport", "og:title", "og:type", "og:site_name", "og:description", "twitter:card"} {
		if v := p.metas[k]; v != "" {
			meta[k] = v
			metaLines = append(metaLines, k+": "+clip(v, 300))
		}
	}
	if p.canonical != "" {
		metaLines = append(metaLines, "canonical: "+p.canonical)
	}
	if p.lang != "" {
		metaLines = append(metaLines, "lang: "+p.lang)
	}
	if len(metaLines) > 0 {
		b.verified("metadata", "Page metadata", "Metadata declared in the page head.", strings.Join(metaLines, "\n"), "head", meta)
	}
	if gen := p.metas["generator"]; gen != "" {
		b.verified("technology", "Generator declared: "+clip(gen, 120), "The page declares the software that generated it in a meta tag.",
			`<meta name="generator" content="`+clip(gen, 200)+`">`, `meta[name="generator"]`, map[string]any{"technology": gen})
	}
	if strings.Contains(strings.ToLower(p.metas["robots"]), "noindex") {
		b.verified("metadata", "Page asks search engines not to index it", "", "robots: "+p.metas["robots"], `meta[name="robots"]`, nil)
	}

	// Structure.
	if len(p.headings) > 0 {
		var lines []string
		var outline []map[string]any
		for _, h := range p.headings {
			lines = append(lines, strings.Repeat("  ", h.level-1)+fmt.Sprintf("H%d %s", h.level, clip(h.text, 200)))
			outline = append(outline, map[string]any{"level": h.level, "text": clip(h.text, 200)})
		}
		h1 := 0
		for _, h := range p.headings {
			if h.level == 1 {
				h1++
			}
		}
		b.verified("structure", "Content outline ("+plural(len(p.headings), "heading")+")", fmt.Sprintf("The page has %d H1 heading(s).", h1),
			strings.Join(lines, "\n"), "h1, h2, h3", map[string]any{"outline": outline})
	}

	// Navigation.
	if len(p.navLinks) > 0 {
		var lines []string
		var items []map[string]string
		seen := map[string]bool{}
		for _, l := range p.navLinks {
			key := l.text + "|" + l.href
			if seen[key] || l.text == "" {
				continue
			}
			seen[key] = true
			lines = append(lines, fmt.Sprintf("%s → %s", clip(l.text, 80), clip(l.href, 200)))
			items = append(items, map[string]string{"text": clip(l.text, 80), "href": clip(l.href, 300)})
		}
		if len(items) > 0 {
			b.verified("navigation", "Primary navigation ("+plural(len(items), "link")+")", "Links found inside <nav> or <header> elements.",
				strings.Join(lines, "\n"), "nav a, header a", map[string]any{"links": items})
		}
	}

	// Links and third-party domains.
	internal, external := 0, 0
	extDomains := map[string]int{}
	var host string
	if p.base != nil {
		host = strings.TrimPrefix(p.base.Hostname(), "www.")
	}
	for _, l := range p.links {
		u, ok := p.resolve(l.href)
		if !ok {
			continue
		}
		h := strings.TrimPrefix(u.Hostname(), "www.")
		if host != "" && (h == host || strings.HasSuffix(h, "."+host)) {
			internal++
		} else {
			external++
			extDomains[h]++
		}
	}
	stats["internal_links"], stats["external_links"] = internal, external
	thirdParty := map[string]int{}
	for _, src := range append(append([]string{}, p.scripts...), p.styles...) {
		if u, ok := p.resolve(src); ok {
			h := strings.TrimPrefix(u.Hostname(), "www.")
			if host == "" || !(h == host || strings.HasSuffix(h, "."+host)) {
				thirdParty[h]++
			}
		}
	}
	if len(thirdParty) > 0 {
		names := sortedKeys(thirdParty)
		b.verified("integration", "Third-party script and style hosts ("+plural(len(names), "host")+")",
			"Assets are loaded from these external hosts. Each host is an observable integration point.",
			strings.Join(names, "\n"), "script[src], link[rel=stylesheet]", map[string]any{"hosts": names})
	}

	// Technology markers.
	for _, sig := range techSignatures {
		for _, needle := range sig.needles {
			if idx := strings.Index(p.rawLower, needle); idx >= 0 {
				start := max(0, idx-60)
				end := min(len(src), idx+len(needle)+60)
				b.inferred("technology", sig.name+" detected", "A client-side marker associated with "+sig.name+" ("+strings.ReplaceAll(sig.category, "_", " ")+") appears in the page.",
					clip(src[start:end], 300), "page source", "Detected from a markup or asset URL pattern. It shows what the browser receives, not how the server is built; markers can also come from third-party embeds.",
					sig.confidence, map[string]any{"technology": sig.name, "kind": sig.category, "marker": needle})
				break
			}
		}
	}

	// Forms.
	for i, f := range p.forms {
		label := f.id
		if label == "" {
			label = f.name
		}
		if label == "" {
			label = fmt.Sprintf("#%d", i+1)
		}
		var fl []string
		hasPassword, hasEmail := false, false
		for _, fd := range f.fields {
			req := ""
			if fd.Required {
				req = " (required)"
			}
			fl = append(fl, fmt.Sprintf("%s: %s%s", fd.Name, fd.Type, req))
			hasPassword = hasPassword || fd.Type == "password"
			hasEmail = hasEmail || fd.Type == "email" || strings.Contains(strings.ToLower(fd.Name), "email")
		}
		action := f.action
		if action == "" {
			action = "(same page)"
		}
		b.verified("form", "Form "+label+": "+f.method+" "+clip(action, 120), fmt.Sprintf("%s. Field values are never captured.", plural(len(f.fields), "visible field")),
			strings.Join(fl, "\n"), f.locator, map[string]any{"method": f.method, "action": f.action, "fields": f.fields})
		if hasPassword {
			kind := "Sign-in form"
			if len(f.fields) > 3 {
				kind = "Account registration or sign-in form"
			}
			b.verified("authentication", kind+" present", "The page contains a password field, so the site has user accounts.",
				strings.Join(fl, "\n"), f.locator, map[string]any{"has_email_field": hasEmail})
			b.unknown("authentication", "Authentication mechanism is not observable",
				"Whether the site uses sessions, tokens, single sign-on or multi-factor authentication cannot be determined from the form markup.")
		}
	}

	// Structured data.
	products := 0
	for i, raw := range p.jsonLD {
		var v any
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &v); err != nil {
			b.verified("structured_data", fmt.Sprintf("Invalid JSON-LD block #%d", i+1), "The block could not be parsed: "+err.Error(), clip(raw, 500), "script[type=\"application/ld+json\"]", nil)
			continue
		}
		for _, node := range flattenLD(v) {
			types := ldTypes(node)
			if len(types) == 0 {
				continue
			}
			b.verified("structured_data", "Schema.org "+strings.Join(types, ", "), "Structured data embedded as JSON-LD.",
				clip(mustJSON(node), 3000), "script[type=\"application/ld+json\"]", map[string]any{"types": types})
			if containsFold(types, "Product") {
				products++
				addProductFinding(b, node)
			}
			if containsFold(types, "Organization") || containsFold(types, "LocalBusiness") {
				addOrganizationFinding(b, node)
			}
		}
	}
	if len(p.itemTypes) > 0 {
		b.verified("structured_data", "Microdata item types", "", strings.Join(uniq(p.itemTypes), "\n"), "[itemtype]", map[string]any{"types": uniq(p.itemTypes)})
	}
	if og := p.metas["og:type"]; strings.Contains(og, "product") || p.metas["product:price:amount"] != "" || p.metas["og:price:amount"] != "" {
		price := firstNonEmpty(p.metas["product:price:amount"], p.metas["og:price:amount"])
		cur := firstNonEmpty(p.metas["product:price:currency"], p.metas["og:price:currency"])
		b.verified("product", "Open Graph product: "+clip(firstNonEmpty(p.metas["og:title"], p.title), 200),
			"Product tags in the page head.", fmt.Sprintf("price: %s %s", price, cur), `meta[property^="product:"]`,
			map[string]any{"name": firstNonEmpty(p.metas["og:title"], p.title), "price": price, "currency": cur})
	}
	stats["products"] = products

	// Accessibility and content.
	if p.images > 0 {
		b.verified("accessibility", fmt.Sprintf("%d of %s lack alternative text", p.missingAlt, plural(p.images, "image")), "", "", "img:not([alt])",
			map[string]any{"images": p.images, "missing_alt": p.missingAlt})
	}
	if _, ok := p.metas["viewport"]; ok {
		b.inferred("structure", "Responsive layout intended", "A viewport meta tag is declared.", "viewport: "+p.metas["viewport"], `meta[name="viewport"]`,
			"A viewport tag suggests, but does not prove, a mobile-friendly layout.", 0.7, nil)
	}

	addCapturedFindings(b, in)

	// What a rendered page cannot reveal.
	b.unknown("backend", "Server-side implementation is not observable",
		"Programming languages, frameworks, business rules and hosting cannot be confirmed from page markup alone. Analyze an authorized codebase or API description for these details.")
	b.unknown("data_model", "Database schema is not observable",
		"Entities may be inferred from visible content, but tables, fields and relationships cannot be determined from a rendered page.")
	return Result{Findings: b.out, Stats: stats}, nil
}

// addCapturedFindings turns user selections from the extension into findings.
func addCapturedFindings(b *builder, in Input) {
	if in.Metadata == nil {
		return
	}
	sels, _ := in.Metadata["selections"].([]any)
	for i, s := range sels {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := m["kind"].(string)
		text, _ := m["text"].(string)
		sel, _ := m["selector"].(string)
		label, _ := m["label"].(string)
		title := fmt.Sprintf("User-selected %s #%d", kind, i+1)
		if label != "" {
			title = "User-selected " + kind + ": " + label
		}
		if rows, ok := m["rows"].([]any); ok && len(rows) > 0 {
			var lines []string
			for _, r := range rows {
				if cells, ok := r.([]any); ok {
					var cs []string
					for _, c := range cells {
						cs = append(cs, fmt.Sprint(c))
					}
					lines = append(lines, strings.Join(cs, " | "))
				}
			}
			text = strings.Join(lines, "\n")
		}
		if src, _ := m["src"].(string); src != "" && text == "" {
			text = src
		}
		b.verified("selection", title, "Evidence the analyst selected on the page with the REA extension.", text, sel, map[string]any{"kind": kind})
	}
	if ex, ok := in.Metadata["extracted"].(map[string]any); ok && len(ex) > 0 {
		if t, _ := ex["title"].(string); t != "" {
			b.verified("product", "Extension extraction: "+clip(t, 200), "Fields the browser extension extracted from the live page.",
				clip(mustJSON(ex), 4000), "extension capture", map[string]any{"extracted": ex})
		}
	}
}

func addProductFinding(b *builder, node map[string]any) {
	name := str(node["name"])
	data := map[string]any{"name": name, "sku": str(node["sku"]), "mpn": str(node["mpn"]), "gtin": firstNonEmpty(str(node["gtin13"]), str(node["gtin"]), str(node["gtin12"]))}
	if br, ok := node["brand"].(map[string]any); ok {
		data["brand"] = str(br["name"])
	} else {
		data["brand"] = str(node["brand"])
	}
	offers := node["offers"]
	if arr, ok := offers.([]any); ok && len(arr) > 0 {
		offers = arr[0]
	}
	if o, ok := offers.(map[string]any); ok {
		data["price"] = firstNonEmpty(str(o["price"]), str(o["lowPrice"]))
		data["currency"] = str(o["priceCurrency"])
		data["availability"] = str(o["availability"])
	}
	b.verified("product", "Product: "+clip(name, 200), "Product data declared with schema.org markup.", clip(mustJSON(node), 3000),
		"script[type=\"application/ld+json\"] Product", data)
}

func addOrganizationFinding(b *builder, node map[string]any) {
	data := map[string]any{"name": str(node["name"]), "url": str(node["url"]), "telephone": str(node["telephone"]), "email": str(node["email"])}
	if addr, ok := node["address"].(map[string]any); ok {
		data["country"] = str(addr["addressCountry"])
		data["city"] = str(addr["addressLocality"])
	}
	b.verified("organization", "Organization: "+clip(str(node["name"]), 200), "Organization details declared with schema.org markup.",
		clip(mustJSON(node), 2000), "script[type=\"application/ld+json\"] Organization", data)
}

func flattenLD(v any) []map[string]any {
	var out []map[string]any
	switch t := v.(type) {
	case []any:
		for _, it := range t {
			out = append(out, flattenLD(it)...)
		}
	case map[string]any:
		if g, ok := t["@graph"]; ok {
			out = append(out, flattenLD(g)...)
		}
		if _, ok := t["@type"]; ok {
			out = append(out, t)
		}
	}
	return out
}

func ldTypes(n map[string]any) []string {
	switch t := n["@type"].(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", t), "0"), ".")
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mustJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
