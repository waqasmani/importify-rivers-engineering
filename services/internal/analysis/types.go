// Package analysis turns authorized sources (pages, documents, archives, API descriptions,
// workflows) into findings. Every finding states whether it is verified (directly observed in the
// source), inferred (a reasoned conclusion with stated assumptions) or unknown (information the
// source cannot provide). Analyzers never execute code from a source.
package analysis

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Classification values.
const (
	Verified = "verified"
	Inferred = "inferred"
	Unknown  = "unknown"
)

// Finding is one analyzer observation.
type Finding struct {
	Category       string         `json:"category"`
	Title          string         `json:"title"`
	Detail         string         `json:"detail"`
	Evidence       string         `json:"evidence"`
	Locator        string         `json:"evidence_locator"`
	Classification string         `json:"classification"`
	Confidence     float64        `json:"confidence"`
	Assumptions    string         `json:"assumptions"`
	Data           map[string]any `json:"data"`
}

// Result is the output of one analyzer run.
type Result struct {
	Findings []Finding      `json:"findings"`
	Stats    map[string]any `json:"stats"`
}

// File is a source file available to an analyzer (from an archive or a repository path).
type File struct {
	Path    string
	Size    int64
	Content []byte
	Skipped string
}

// Input is everything an analyzer may read. Only one of the content fields is usually set.
type Input struct {
	Kind        string
	Title       string
	URL         string
	MimeType    string
	Text        string         // extracted text, HTML, JSON or YAML content
	Metadata    map[string]any // source metadata (workflow steps, captured structured data...)
	Files       []File         // codebase files
	RawFile     []byte         // uploaded file bytes (documents, screenshots)
	RawFilename string
}

// Analyzer produces findings from an input.
type Analyzer interface {
	Name() string
	Analyze(ctx context.Context, in Input) (Result, error)
}

var registry = map[string]Analyzer{}

func register(a Analyzer) { registry[a.Name()] = a }

// Get returns an analyzer by name.
func Get(name string) (Analyzer, bool) {
	a, ok := registry[name]
	return a, ok
}

// Names lists registered analyzers.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

// DefaultAnalyzer picks the analyzer for a source kind.
func DefaultAnalyzer(kind string) string {
	switch kind {
	case "url", "html", "captured_page":
		return "website"
	case "repository_archive", "repository_path":
		return "codebase"
	case "openapi":
		return "openapi"
	case "api_examples":
		return "api_examples"
	case "document", "text":
		return "document"
	case "screenshot":
		return "screenshot"
	case "workflow":
		return "workflow"
	}
	return ""
}

// builder accumulates findings with bounded evidence sizes.
type builder struct {
	out []Finding
	max int
}

func newBuilder() *builder { return &builder{max: 2000} }

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func (b *builder) add(f Finding) {
	if len(b.out) >= b.max {
		return
	}
	f.Title = clip(f.Title, 480)
	f.Evidence = clip(f.Evidence, 7000)
	f.Detail = clip(f.Detail, 20000)
	f.Locator = clip(f.Locator, 1000)
	if f.Data == nil {
		f.Data = map[string]any{}
	}
	if f.Confidence <= 0 {
		switch f.Classification {
		case Verified:
			f.Confidence = 0.95
		case Inferred:
			f.Confidence = 0.6
		default:
			f.Confidence = 0.2
		}
	}
	b.out = append(b.out, f)
}

func (b *builder) verified(cat, title, detail, evidence, locator string, data map[string]any) {
	b.add(Finding{Category: cat, Title: title, Detail: detail, Evidence: evidence, Locator: locator, Classification: Verified, Data: data})
}

func (b *builder) inferred(cat, title, detail, evidence, locator, assumptions string, conf float64, data map[string]any) {
	b.add(Finding{Category: cat, Title: title, Detail: detail, Evidence: evidence, Locator: locator, Classification: Inferred,
		Confidence: conf, Assumptions: assumptions, Data: data})
}

func (b *builder) unknown(cat, title, detail string) {
	b.add(Finding{Category: cat, Title: title, Detail: detail, Classification: Unknown, Confidence: 0.1,
		Assumptions: "This information cannot be determined from the provided source."})
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
