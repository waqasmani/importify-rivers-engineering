package marketplaces

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/waqasmani/importify-rivers-engineering/db"
)

// Regions groups marketplaces in the dashboard.
var Regions = []string{"global", "china", "cross_border", "north_america", "south_asia", "southeast_asia"}

// VerificationStatuses says how a definition was checked: only against a synthetic fixture that
// reproduces the assumed layout, against a saved real product page, or on live pages.
var VerificationStatuses = []string{"synthetic_fixture", "saved_page", "live_page"}

// MaxDomainPatterns is the most domain patterns a rules adapter may list.
const MaxDomainPatterns = 100

// Verification records how a shipped definition was checked.
type Verification struct {
	Status     string `json:"status"`
	Note       string `json:"note"`
	Fixture    string `json:"fixture"`
	FixtureURL string `json:"fixture_url"`
}

// Definition is one shipped marketplace: db/marketplaces/<key>.json.
type Definition struct {
	Key          string          `json:"key"`
	Name         string          `json:"name"`
	Region       string          `json:"region"`
	Description  string          `json:"description"`
	Domains      []string        `json:"domains"`
	Priority     int32           `json:"priority"`
	Version      int32           `json:"version"`
	Verification Verification    `json:"verification"`
	Rules        json.RawMessage `json:"rules"`
}

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,40}$`)

// ValidDomainPattern accepts host names such as example.com or *.example.com (never "*" alone).
func ValidDomainPattern(p string) bool {
	host := strings.TrimPrefix(p, "*.")
	return p != "*" && host != "" && !strings.ContainsAny(host, "/:*?#@ ") && strings.Contains(host, ".") && p == strings.ToLower(strings.TrimSpace(p))
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Validate returns the problems with a definition (empty when valid).
func (d Definition) Validate() []string {
	var p []string
	add := func(format string, args ...any) { p = append(p, d.Key+": "+fmt.Sprintf(format, args...)) }
	if !keyRe.MatchString(d.Key) {
		add("key must be 2-41 lowercase letters, digits, hyphens or underscores starting with a letter")
	}
	if l := len([]rune(d.Name)); l < 1 || l > 120 {
		add("name must have 1-120 characters")
	}
	if len([]rune(d.Description)) > 1000 {
		add("description is limited to 1000 characters")
	}
	if !contains(Regions, d.Region) {
		add("region must be one of %s", strings.Join(Regions, ", "))
	}
	if len(d.Domains) < 1 || len(d.Domains) > MaxDomainPatterns {
		add("domains must list 1-%d patterns", MaxDomainPatterns)
	}
	for _, dom := range d.Domains {
		if !ValidDomainPattern(dom) {
			add("domain %q must be a lowercase host such as example.com or *.example.com", dom)
		}
	}
	if d.Priority < 0 || d.Priority > 10000 {
		add("priority must be between 0 and 10000")
	}
	if d.Version < 1 {
		add("version must be 1 or higher")
	}
	if !contains(VerificationStatuses, d.Verification.Status) {
		add("verification.status must be one of %s", strings.Join(VerificationStatuses, ", "))
	}
	if len(strings.TrimSpace(d.Verification.Note)) < 20 {
		add("verification.note must describe the assumed layout")
	}
	for _, problem := range ValidateRules(d.Rules) {
		add("rules: %s", problem)
	}
	return p
}

// Load reads every shipped definition and validates it.
func Load() ([]Definition, error) {
	return LoadFS(db.Marketplaces, "marketplaces")
}

// LoadFS reads definitions from dir in fsys (used by Load and by tests).
func LoadFS(fsys fs.FS, dir string) ([]Definition, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var defs []Definition
	var problems []string
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var d Definition
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&d); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		if d.Key+".json" != e.Name() {
			problems = append(problems, fmt.Sprintf("%s: the file name must be the key (%s.json)", e.Name(), d.Key))
		}
		if seen[d.Key] {
			problems = append(problems, fmt.Sprintf("%s: duplicate key", d.Key))
		}
		seen[d.Key] = true
		problems = append(problems, d.Validate()...)
		defs = append(defs, d)
	}
	if len(problems) > 0 {
		return defs, fmt.Errorf("invalid marketplace definitions:\n  %s", strings.Join(problems, "\n  "))
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Key < defs[j].Key })
	return defs, nil
}
