package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

// OrganizationSettings names the organization running this instance.
type OrganizationSettings struct {
	Name string `json:"name"`
}

// EntityLabel renames a CRM concept, for example "Supplier" -> "Vendor".
type EntityLabel struct {
	Singular string `json:"singular"`
	Plural   string `json:"plural"`
}

// CaptureSettings control what the browser extension stores.
type CaptureSettings struct {
	StoreHTMLSnapshots bool `json:"store_html_snapshots"`
	MaxSnapshotKB      int  `json:"max_snapshot_kb"`
	RetentionDays      int  `json:"retention_days"`
}

// ImportSettings control product import behaviour.
type ImportSettings struct {
	DefaultCurrency          string  `json:"default_currency"`
	AutoLinkSupplierByDomain bool    `json:"auto_link_supplier_by_domain"`
	TitleSimilarityThreshold float64 `json:"title_similarity_threshold"`
}

// AISettings is the global AI switch.
type AISettings struct {
	Enabled bool `json:"enabled"`
}

type settingSpec struct {
	def      func() any
	validate func(raw json.RawMessage) (any, error)
}

var entityLabelKeys = []string{"company", "contact", "supplier", "lead", "opportunity", "product"}

func strictUnmarshal(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errBadRequest("Invalid setting value: " + err.Error())
	}
	return nil
}

var settingSpecs = map[string]settingSpec{
	"organization": {
		def: func() any { return OrganizationSettings{Name: ""} },
		validate: func(raw json.RawMessage) (any, error) {
			var v OrganizationSettings
			if err := strictUnmarshal(raw, &v); err != nil {
				return nil, err
			}
			v.Name = normalize.Text(v.Name)
			val := valid.New()
			val.Len("name", v.Name, 1, 120)
			return v, checkValid(val)
		},
	},
	"entity_labels": {
		def: func() any {
			return map[string]EntityLabel{
				"company": {"Company", "Companies"}, "contact": {"Contact", "Contacts"}, "supplier": {"Supplier", "Suppliers"},
				"lead": {"Lead", "Leads"}, "opportunity": {"Opportunity", "Opportunities"}, "product": {"Product", "Products"},
			}
		},
		validate: func(raw json.RawMessage) (any, error) {
			var v map[string]EntityLabel
			if err := strictUnmarshal(raw, &v); err != nil {
				return nil, err
			}
			val := valid.New()
			for k, l := range v {
				known := false
				for _, e := range entityLabelKeys {
					known = known || e == k
				}
				val.Check(known, k, "is not a renameable entity")
				l.Singular, l.Plural = normalize.Text(l.Singular), normalize.Text(l.Plural)
				val.Len(k+".singular", l.Singular, 1, 40)
				val.Len(k+".plural", l.Plural, 1, 40)
				v[k] = l
			}
			return v, checkValid(val)
		},
	},
	"capture": {
		def: func() any { return CaptureSettings{StoreHTMLSnapshots: false, MaxSnapshotKB: 1024, RetentionDays: 0} },
		validate: func(raw json.RawMessage) (any, error) {
			var v CaptureSettings
			if err := strictUnmarshal(raw, &v); err != nil {
				return nil, err
			}
			val := valid.New()
			val.Check(v.MaxSnapshotKB >= 16 && v.MaxSnapshotKB <= 2900, "max_snapshot_kb", "must be between 16 and 2900")
			val.Check(v.RetentionDays >= 0 && v.RetentionDays <= 3650, "retention_days", "must be between 0 (keep forever) and 3650")
			return v, checkValid(val)
		},
	},
	"imports": {
		def: func() any {
			return ImportSettings{DefaultCurrency: "USD", AutoLinkSupplierByDomain: true, TitleSimilarityThreshold: 0.55}
		},
		validate: func(raw json.RawMessage) (any, error) {
			var v ImportSettings
			if err := strictUnmarshal(raw, &v); err != nil {
				return nil, err
			}
			v.DefaultCurrency = strings.ToUpper(v.DefaultCurrency)
			val := valid.New()
			val.Currency("default_currency", v.DefaultCurrency)
			val.Check(v.TitleSimilarityThreshold >= 0.3 && v.TitleSimilarityThreshold <= 0.95, "title_similarity_threshold", "must be between 0.3 and 0.95")
			return v, checkValid(val)
		},
	},
	"ai": {
		def: func() any { return AISettings{Enabled: true} },
		validate: func(raw json.RawMessage) (any, error) {
			var v AISettings
			return v, strictUnmarshal(raw, &v)
		},
	},
}

// loadSetting decodes a stored setting into dst, falling back to the default.
func (s *Server) loadSetting(ctx context.Context, key string, dst any) {
	spec := settingSpecs[key]
	def, _ := json.Marshal(spec.def())
	_ = json.Unmarshal(def, dst)
	if row, err := s.db.GetSetting(ctx, key); err == nil {
		_ = json.Unmarshal(row.Value, dst)
	}
}

func (s *Server) importSettings(ctx context.Context) ImportSettings {
	var v ImportSettings
	s.loadSetting(ctx, "imports", &v)
	return v
}

func (s *Server) captureSettings(ctx context.Context) CaptureSettings {
	var v CaptureSettings
	s.loadSetting(ctx, "capture", &v)
	return v
}

func (s *Server) aiEnabled(ctx context.Context) bool {
	var v AISettings
	s.loadSetting(ctx, "ai", &v)
	return v.Enabled
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) error {
	out := map[string]any{}
	for key, spec := range settingSpecs {
		out[key] = spec.def()
	}
	rows, err := s.db.ListSettings(r.Context())
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, known := settingSpecs[row.Key]; known {
			out[row.Key] = row.Value
		}
	}
	return ok(w, out)
}

func (s *Server) handlePutSetting(w http.ResponseWriter, r *http.Request) error {
	key := chi.URLParam(r, "key")
	spec, known := settingSpecs[key]
	if !known {
		return errNotFound("Setting")
	}
	var raw json.RawMessage
	if err := decode(r, &raw); err != nil {
		return err
	}
	v, err := spec.validate(raw)
	if err != nil {
		return err
	}
	clean, _ := json.Marshal(v)
	var before json.RawMessage
	if old, err := s.db.GetSetting(r.Context(), key); err == nil {
		before = old.Value
	}
	row, err := s.db.UpsertSetting(r.Context(), store.UpsertSettingParams{Key: key, Value: clean, UpdatedBy: userIDPtr(r)})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "settings.update", EntityType: "setting", Summary: "Updated setting " + key,
		Extra: map[string]any{"key": key, "before": before, "after": json.RawMessage(clean)}})
	return ok(w, row)
}
