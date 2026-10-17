package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/marketplaces"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// ---- source adapters ----
//
// Kinds: "builtin" (code adapters in @rea/extractor), "selector" (CSS selectors per field) and
// "rules" (declarative rule documents; marketplace definitions are shipped rules adapters). Shipped
// adapters cannot be deleted or redefined, only switched on or off and re-prioritised.

var selectorKeys = map[string]bool{"title": true, "price": true, "currency": true, "sku": true, "brand": true, "description": true,
	"images": true, "availability": true, "model": true, "mpn": true, "gtin": true, "category": true, "specifications": true,
	"supplier_name": true, "variants": true}

type adapterInput struct {
	Key            string            `json:"key"`
	Name           string            `json:"name"`
	DomainPatterns []string          `json:"domain_patterns"`
	Enabled        bool              `json:"enabled"`
	Priority       int32             `json:"priority"`
	Selectors      map[string]string `json:"selectors"`
	Description    string            `json:"description"`
	// Kind is "selector" (default) or "rules"; it cannot change after creation.
	Kind string `json:"kind,omitempty"`
	// Rules is the rule document for kind "rules" (see docs/marketplaces.md).
	Rules json.RawMessage `json:"rules,omitempty"`
}

func (in *adapterInput) kind() string {
	if in.Kind == "" {
		return "selector"
	}
	return in.Kind
}

func (in *adapterInput) validate(creating bool, kind string) error {
	in.Name, in.Description = normalize.Text(in.Name), normalize.Text(in.Description)
	v := valid.New()
	if creating {
		v.Check(len(in.Key) >= 2 && len(in.Key) <= 41 && strings.Trim(in.Key, "abcdefghijklmnopqrstuvwxyz0123456789_-") == "" && in.Key[0] >= 'a' && in.Key[0] <= 'z',
			"key", "must be 2-41 lowercase letters, digits, hyphens or underscores")
	}
	v.OneOf("kind", kind, "selector", "rules")
	v.Len("name", in.Name, 1, 120)
	v.Max("description", in.Description, 1000)
	v.Check(in.Priority >= 0 && in.Priority <= 10000, "priority", "must be between 0 and 10000")
	maxPatterns := 50
	if kind == "rules" {
		maxPatterns = marketplaces.MaxDomainPatterns
	}
	v.Check(len(in.DomainPatterns) >= 1 && len(in.DomainPatterns) <= maxPatterns, "domain_patterns", "must contain between 1 and "+strconv.Itoa(maxPatterns)+" patterns")
	for i, p := range in.DomainPatterns {
		p = strings.ToLower(strings.TrimSpace(p))
		in.DomainPatterns[i] = p
		v.Check(marketplaces.ValidDomainPattern(p), "domain_patterns", "must be host names such as example.com or *.example.com")
	}
	switch kind {
	case "rules":
		v.Check(len(in.Selectors) == 0, "selectors", "rules adapters describe fields in their rule document, not as selectors")
		if len(in.Rules) == 0 || string(in.Rules) == "null" {
			v.Add("rules", "a rule document is required")
		} else if problems := marketplaces.ValidateRules(in.Rules); len(problems) > 0 {
			if len(problems) > 5 {
				problems = append(problems[:5], "and more")
			}
			v.Add("rules", strings.Join(problems, "; "))
		}
	default:
		v.Check(len(in.Rules) == 0 || string(in.Rules) == "null", "rules", "only rules adapters have a rule document")
		for k, sel := range in.Selectors {
			v.Check(selectorKeys[k], "selectors", "contains an unknown field "+k)
			v.Check(len(sel) > 0 && len(sel) <= 300 && !strings.ContainsAny(sel, "<>{}"), "selectors", "selectors must be CSS selectors up to 300 characters")
		}
		v.Check(in.Selectors["title"] != "", "selectors", "must include a title selector")
	}
	return checkValid(v)
}

func (s *Server) handleListAdapters(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListSourceAdapters(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) handleCreateAdapter(w http.ResponseWriter, r *http.Request) error {
	var in adapterInput
	if err := decode(r, &in); err != nil {
		return err
	}
	kind := in.kind()
	if err := in.validate(true, kind); err != nil {
		return err
	}
	if _, err := s.db.GetSourceAdapterByKey(r.Context(), in.Key); err == nil {
		return errConflict("adapter_key_taken", "An adapter with this key already exists.")
	}
	params := store.CreateSourceAdapterParams{Key: in.Key, Name: in.Name, Kind: kind, DomainPatterns: in.DomainPatterns,
		Enabled: in.Enabled, Priority: in.Priority, Selectors: jsonOf(in.Selectors), Description: in.Description}
	if kind == "rules" {
		params.Selectors, params.Rules = json.RawMessage("{}"), in.Rules
	}
	a, err := s.db.CreateSourceAdapter(r.Context(), params)
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "adapter.create", EntityType: "source_adapter", EntityID: &a.ID, Summary: "Created adapter " + a.Name, After: a})
	return created(w, a)
}

func (s *Server) handleUpdateAdapter(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in adapterInput
	if err := decode(r, &in); err != nil {
		return err
	}
	before, err := s.db.GetSourceAdapter(r.Context(), id)
	if err != nil {
		return err
	}
	params := store.UpdateSourceAdapterParams{ID: id, Name: before.Name, DomainPatterns: before.DomainPatterns, Selectors: before.Selectors,
		Rules: before.Rules, Enabled: in.Enabled, Priority: in.Priority, Description: normalize.Text(in.Description)}
	if before.Shipped {
		// Shipped adapters (built-in code and marketplace definitions) keep their name, domains and
		// extraction definition; only enabling, priority and description can change. Marketplace
		// definitions are updated by shipping a newer version with REA.
		v := valid.New()
		v.Max("description", params.Description, 1000)
		v.Check(in.Priority >= 0 && in.Priority <= 10000, "priority", "must be between 0 and 10000")
		if err := checkValid(v); err != nil {
			return err
		}
	} else {
		if in.Kind != "" && in.Kind != before.Kind {
			return errField("kind", "cannot change after the adapter is created")
		}
		if err := in.validate(false, before.Kind); err != nil {
			return err
		}
		params.Name, params.DomainPatterns, params.Description = in.Name, in.DomainPatterns, in.Description
		if before.Kind == "rules" {
			params.Rules = in.Rules
		} else {
			params.Selectors = jsonOf(in.Selectors)
		}
	}
	a, err := s.db.UpdateSourceAdapter(r.Context(), params)
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "adapter.update", EntityType: "source_adapter", EntityID: &id, Summary: "Updated adapter " + a.Name, Before: before, After: a})
	return ok(w, a)
}

func (s *Server) handleDeleteAdapter(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	n, err := s.db.DeleteSourceAdapter(r.Context(), id)
	if err != nil {
		return err
	}
	if n == 0 {
		return errConflict("adapter_builtin", "Built-in adapters and shipped marketplace definitions cannot be deleted; disable them instead.")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "adapter.delete", EntityType: "source_adapter", EntityID: &id, Summary: "Deleted an adapter"})
	return noContent(w)
}
