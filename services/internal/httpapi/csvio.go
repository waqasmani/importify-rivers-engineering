package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// ---- export ----

// exportSpec describes how to page through an entity list and which columns to write.
type exportSpec struct {
	columns []string
	fetch   func(r *http.Request, limit, offset int32) ([]any, error)
}

func toAnySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

func (s *Server) exportSpecs() map[string]exportSpec {
	return map[string]exportSpec{
		"company": {strings.Fields("id name domain website industry company_type status country city address phone email description employee_count owner_name tags custom_fields created_at updated_at"),
			func(r *http.Request, l, o int32) ([]any, error) { v, _, err := s.listCompanies(r, l, o); return toAnySlice(v), err }},
		"contact": {strings.Fields("id first_name last_name email phone job_title company_name supplier_name is_decision_maker linkedin_url country status tags custom_fields created_at updated_at"),
			func(r *http.Request, l, o int32) ([]any, error) { v, _, err := s.listContacts(r, l, o); return toAnySlice(v), err }},
		"supplier": {strings.Fields("id name supplier_type website domain country city address email phone primary_contact_name stage_name rating payment_terms lead_time_days moq_notes certifications description company_name product_count tags custom_fields created_at updated_at"),
			func(r *http.Request, l, o int32) ([]any, error) { v, _, err := s.listSuppliers(r, l, o); return toAnySlice(v), err }},
		"lead": {strings.Fields("id title company_name contact_name source stage_name estimated_value currency description owner_name converted_at tags custom_fields created_at updated_at"),
			func(r *http.Request, l, o int32) ([]any, error) { v, _, err := s.listLeads(r, l, o); return toAnySlice(v), err }},
		"opportunity": {strings.Fields("id name company_name supplier_name contact_name stage_name status amount currency expected_close_date probability closed_at outcome_reason owner_name tags custom_fields created_at updated_at"),
			func(r *http.Request, l, o int32) ([]any, error) { v, _, err := s.listOpportunities(r, l, o); return toAnySlice(v), err }},
		"task": {strings.Fields("id title description status priority task_type due_at remind_at completed_at assignee_name entity_type entity_id created_at"),
			func(r *http.Request, l, o int32) ([]any, error) { v, _, err := s.listTasks(r, l, o); return toAnySlice(v), err }},
		"product": {strings.Fields("id title original_title brand sku model mpn gtin category source_url source_domain supplier_name cost cost_currency price price_currency margin_amount margin_percent availability status review_status extraction_method extraction_confidence variant_count primary_image_url description specifications tags first_seen_at last_checked_at"),
			func(r *http.Request, l, o int32) ([]any, error) { v, _, err := s.listProducts(r, l, o); return toAnySlice(v), err }},
	}
}

const maxExportRows = 100000

// csvSafe neutralises spreadsheet formula injection by prefixing risky leading characters.
func csvSafe(v string) string {
	if v == "" {
		return v
	}
	switch v[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + v
	}
	return v
}

func cellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(t))
		for _, it := range t {
			if m, ok := it.(map[string]any); ok {
				if n, ok := m["name"].(string); ok {
					parts = append(parts, n)
					continue
				}
			}
			parts = append(parts, cellString(it))
		}
		return strings.Join(parts, "; ")
	case map[string]any:
		if len(t) == 0 {
			return ""
		}
		b, _ := json.Marshal(t)
		return string(b)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func (s *Server) handleExport(entity string) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		spec, ok := s.exportSpecs()[entity]
		if !ok {
			return errNotFound("Export")
		}
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "csv"
		}
		if format != "csv" && format != "json" {
			return errField("format", "must be csv or json")
		}
		// Validate filters by fetching the first page before writing headers.
		first, err := spec.fetch(r, 500, 0)
		if err != nil {
			return err
		}
		stamp := time.Now().UTC().Format("20060102-150405")
		filename := fmt.Sprintf("rea-%s-%s.%s", entity, stamp, format)
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.Header().Set("Cache-Control", "no-store")
		var cw *csv.Writer
		if format == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			_, _ = w.Write([]byte("\ufeff"))
			cw = csv.NewWriter(w)
			_ = cw.Write(spec.columns)
		} else {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"entity":"` + entity + `","exported_at":"` + time.Now().UTC().Format(time.RFC3339) + `","items":[`))
		}
		count := 0
		page := first
		for offset := int32(0); ; {
			for _, rec := range page {
				if format == "csv" {
					m := toMap(rec)
					row := make([]string, len(spec.columns))
					for i, c := range spec.columns {
						row[i] = csvSafe(cellString(m[c]))
					}
					_ = cw.Write(row)
				} else {
					if count > 0 {
						_, _ = w.Write([]byte(","))
					}
					b, _ := json.Marshal(rec)
					_, _ = w.Write(b)
				}
				count++
			}
			if len(page) < 500 || count >= maxExportRows {
				break
			}
			offset += 500
			if page, err = spec.fetch(r, 500, offset); err != nil {
				s.log.Error("export page failed", "error", err.Error())
				break
			}
		}
		if cw != nil {
			cw.Flush()
		} else {
			_, _ = w.Write([]byte("]}"))
		}
		_ = s.audit(r.Context(), nil, r, auditEvent{Action: "data.export", EntityType: entity, Summary: fmt.Sprintf("Exported %d %s records as %s", count, entity, format)})
		return nil
	}
}

// ---- CSV import ----

type importField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Required bool     `json:"required"`
	Aliases  []string `json:"-"`
}

var importFields = map[string][]importField{
	"company": {
		{"name", "Name", true, []string{"company", "company name", "organization", "organisation", "business name"}},
		{"website", "Website", false, []string{"url", "web", "homepage", "site"}},
		{"domain", "Domain", false, nil},
		{"industry", "Industry", false, []string{"sector"}},
		{"company_type", "Type", false, []string{"type", "category"}},
		{"status", "Status", false, nil},
		{"country", "Country", false, nil},
		{"city", "City", false, nil},
		{"address", "Address", false, []string{"street"}},
		{"phone", "Phone", false, []string{"telephone", "tel"}},
		{"email", "E-mail", false, []string{"email address", "mail"}},
		{"description", "Description", false, []string{"notes", "about"}},
		{"employee_count", "Employees", false, []string{"employees", "staff", "headcount"}},
		{"tags", "Tags", false, []string{"labels"}},
	},
	"contact": {
		{"first_name", "First name", false, []string{"first", "given name", "firstname"}},
		{"last_name", "Last name", false, []string{"last", "surname", "family name", "lastname"}},
		{"full_name", "Full name", false, []string{"name", "contact name", "contact"}},
		{"email", "E-mail", false, []string{"email address", "mail"}},
		{"phone", "Phone", false, []string{"telephone", "mobile", "tel"}},
		{"job_title", "Job title", false, []string{"title", "position", "role"}},
		{"company", "Company", false, []string{"company name", "organization", "organisation", "employer"}},
		{"supplier", "Supplier", false, []string{"supplier name", "vendor"}},
		{"linkedin_url", "LinkedIn URL", false, []string{"linkedin"}},
		{"country", "Country", false, nil},
		{"is_decision_maker", "Decision maker", false, []string{"decision maker"}},
		{"tags", "Tags", false, []string{"labels"}},
	},
	"supplier": {
		{"name", "Name", true, []string{"supplier", "supplier name", "vendor", "company", "manufacturer"}},
		{"supplier_type", "Type", false, []string{"type"}},
		{"website", "Website", false, []string{"url", "web", "homepage", "store url"}},
		{"domain", "Domain", false, nil},
		{"country", "Country", false, nil},
		{"city", "City", false, nil},
		{"address", "Address", false, nil},
		{"email", "E-mail", false, []string{"email address"}},
		{"phone", "Phone", false, []string{"telephone"}},
		{"payment_terms", "Payment terms", false, []string{"terms"}},
		{"lead_time_days", "Lead time (days)", false, []string{"lead time"}},
		{"moq_notes", "MOQ", false, []string{"moq", "minimum order"}},
		{"certifications", "Certifications", false, []string{"certificates"}},
		{"rating", "Rating", false, nil},
		{"description", "Description", false, []string{"notes"}},
		{"tags", "Tags", false, []string{"labels"}},
	},
	"product": {
		{"title", "Title", true, []string{"name", "product", "product name", "product title"}},
		{"source_url", "Source URL", false, []string{"url", "link", "product url"}},
		{"brand", "Brand", false, []string{"manufacturer"}},
		{"sku", "SKU", false, []string{"item number", "item no", "article number"}},
		{"model", "Model", false, nil},
		{"mpn", "MPN", false, []string{"part number"}},
		{"gtin", "GTIN", false, []string{"ean", "upc", "barcode"}},
		{"description", "Description", false, nil},
		{"category", "Category", false, nil},
		{"cost", "Cost", false, []string{"unit cost", "purchase price", "buy price"}},
		{"cost_currency", "Cost currency", false, nil},
		{"price", "Price", false, []string{"retail price", "sell price", "selling price"}},
		{"price_currency", "Price currency", false, []string{"currency"}},
		{"availability", "Availability", false, []string{"stock", "in stock"}},
		{"supplier", "Supplier", false, []string{"supplier name", "vendor"}},
		{"image_url", "Image URL", false, []string{"image", "picture", "main image"}},
		{"tags", "Tags", false, []string{"labels"}},
	},
}

const maxImportRows = 10000

type parsedCSV struct {
	Headers []string
	Rows    [][]string
}

func parseCSV(data []byte) (parsedCSV, error) {
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if !utf8.Valid(data) {
		return parsedCSV{}, errField("file", "must be UTF-8 encoded CSV")
	}
	sample := string(data[:min(len(data), 4096)])
	cr := csv.NewReader(bytes.NewReader(data))
	if strings.Count(sample, ";") > strings.Count(sample, ",") {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true
	var out parsedCSV
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, errField("file", "is not valid CSV: "+err.Error())
		}
		if out.Headers == nil {
			for _, h := range rec {
				out.Headers = append(out.Headers, normalize.Text(h))
			}
			continue
		}
		empty := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				empty = false
				break
			}
		}
		if empty {
			continue
		}
		if len(out.Rows) >= maxImportRows {
			return out, errField("file", fmt.Sprintf("contains more than %d rows; split it into smaller files", maxImportRows))
		}
		out.Rows = append(out.Rows, rec)
	}
	if len(out.Headers) == 0 {
		return out, errField("file", "has no header row")
	}
	return out, nil
}

func suggestMapping(entity string, headers []string) map[string]string {
	out := map[string]string{}
	used := map[string]bool{}
	for _, h := range headers {
		key := normalize.Key(h)
		for _, f := range importFields[entity] {
			if used[f.Key] {
				continue
			}
			match := key == normalize.Key(f.Key) || key == normalize.Key(f.Label)
			for _, a := range f.Aliases {
				match = match || key == a
			}
			if match {
				out[h] = f.Key
				used[f.Key] = true
				break
			}
		}
	}
	return out
}

// readImportUpload reads the multipart file into memory (bounded by the upload limit).
func (s *Server) readImportUpload(r *http.Request) ([]byte, map[string]string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, s.cfg.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return nil, nil, errBadRequest("Upload the CSV as multipart/form-data with a 'file' field.")
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	f, _, err := r.FormFile("file")
	if err != nil {
		return nil, nil, errField("file", "is required")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, s.cfg.MaxUploadBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > s.cfg.MaxUploadBytes {
		return nil, nil, &APIError{Status: http.StatusRequestEntityTooLarge, Code: "file_too_large", Message: "The CSV file is too large."}
	}
	fields := map[string]string{}
	for k, v := range r.MultipartForm.Value {
		if len(v) > 0 {
			fields[k] = v[0]
		}
	}
	return data, fields, nil
}

func importPerm(entity string) string {
	if entity == "product" {
		return auth.PermProductsWrite
	}
	return auth.PermCRMWrite
}

func (s *Server) handleImportPreview(w http.ResponseWriter, r *http.Request) error {
	data, fields, err := s.readImportUpload(r)
	if err != nil {
		return err
	}
	entity := fields["entity_type"]
	if _, ok := importFields[entity]; !ok {
		return errField("entity_type", "must be company, contact, supplier or product")
	}
	if err := s.can(r, importPerm(entity)); err != nil {
		return err
	}
	p, err := parseCSV(data)
	if err != nil {
		return err
	}
	sample := p.Rows
	if len(sample) > 10 {
		sample = sample[:10]
	}
	return ok(w, map[string]any{"entity_type": entity, "headers": p.Headers, "mapping": suggestMapping(entity, p.Headers),
		"fields": importFields[entity], "sample_rows": sample, "total_rows": len(p.Rows)})
}

type importRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type importResult struct {
	DryRun  bool             `json:"dry_run"`
	Total   int              `json:"total"`
	Created int              `json:"created"`
	Updated int              `json:"updated"`
	Skipped int              `json:"skipped"`
	Failed  int              `json:"failed"`
	Errors  []importRowError `json:"errors"`
}

var errDryRun = fmt.Errorf("dry run")

func (s *Server) handleImportCommit(w http.ResponseWriter, r *http.Request) error {
	data, fields, err := s.readImportUpload(r)
	if err != nil {
		return err
	}
	entity := fields["entity_type"]
	if _, ok := importFields[entity]; !ok {
		return errField("entity_type", "must be company, contact, supplier or product")
	}
	if err := s.can(r, importPerm(entity)); err != nil {
		return err
	}
	onDup := fields["on_duplicate"]
	if onDup == "" {
		onDup = "skip"
	}
	if onDup != "skip" && onDup != "update" && onDup != "create" {
		return errField("on_duplicate", "must be skip, update or create")
	}
	dryRun := fields["dry_run"] == "true"
	p, err := parseCSV(data)
	if err != nil {
		return err
	}
	mapping := map[string]string{}
	if raw := fields["mapping"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &mapping); err != nil {
			return errField("mapping", "must be a JSON object of column header to field key")
		}
	} else {
		mapping = suggestMapping(entity, p.Headers)
	}
	validKeys := map[string]bool{}
	for _, f := range importFields[entity] {
		validKeys[f.Key] = true
	}
	colIndex := map[string]int{}
	for i, h := range p.Headers {
		if key, ok := mapping[h]; ok && key != "" {
			if !validKeys[key] {
				return errField("mapping", "maps a column to unknown field "+key)
			}
			colIndex[key] = i
		}
	}
	for _, f := range importFields[entity] {
		if f.Required {
			if _, ok := colIndex[f.Key]; !ok {
				return errField("mapping", "must map a column to the required field "+f.Label)
			}
		}
	}
	if entity == "contact" {
		_, a := colIndex["first_name"]
		_, b := colIndex["last_name"]
		_, c := colIndex["full_name"]
		_, d := colIndex["email"]
		if !a && !b && !c && !d {
			return errField("mapping", "must map a name or e-mail column for contacts")
		}
	}
	res := importResult{DryRun: dryRun, Total: len(p.Rows), Errors: []importRowError{}}
	err = s.db.Tx(r.Context(), func(q *store.Queries, tx pgx.Tx) error {
		for i, rec := range p.Rows {
			row := map[string]string{}
			for key, idx := range colIndex {
				if idx < len(rec) {
					row[key] = strings.TrimSpace(rec[idx])
				}
			}
			sp, err := tx.Begin(r.Context())
			if err != nil {
				return err
			}
			outcome, err := s.importRow(r, q.WithTx(sp), entity, row, onDup)
			if err != nil {
				_ = sp.Rollback(r.Context())
				res.Failed++
				if len(res.Errors) < 500 {
					res.Errors = append(res.Errors, importRowError{Row: i + 2, Message: describeImportError(err)})
				}
				continue
			}
			if err := sp.Commit(r.Context()); err != nil {
				return err
			}
			switch outcome {
			case "created":
				res.Created++
			case "updated":
				res.Updated++
			default:
				res.Skipped++
			}
		}
		if dryRun {
			return errDryRun
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "data.import", EntityType: entity,
			Summary: fmt.Sprintf("Imported CSV: %d created, %d updated, %d skipped, %d failed", res.Created, res.Updated, res.Skipped, res.Failed)})
	})
	if err != nil && err != errDryRun {
		return err
	}
	return ok(w, res)
}

func describeImportError(err error) string {
	if ae := toAPIError(err); ae != nil {
		if len(ae.Fields) > 0 {
			keys := make([]string, 0, len(ae.Fields))
			for k := range ae.Fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, k+" "+ae.Fields[k])
			}
			return strings.Join(parts, "; ")
		}
		return ae.Message
	}
	return "could not be imported"
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ';' || r == ',' || r == '|' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "x":
		return true
	}
	return false
}

func optInt32(v string, field string) (*int32, error) {
	if v == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(v), ",", ""))
	if err != nil {
		return nil, errField(field, "must be a whole number")
	}
	i := int32(n)
	return &i, nil
}

func (s *Server) addTagNames(ctx context.Context, q *store.Queries, entity string, id uuid.UUID, names string) error {
	for _, n := range splitList(names) {
		if len(n) > 60 {
			continue
		}
		t, err := q.UpsertTag(ctx, store.UpsertTagParams{Name: n, Color: "#6366f1"})
		if err != nil {
			return err
		}
		if err := q.TagRecordsBulk(ctx, store.TagRecordsBulkParams{TagID: t.ID, EntityType: entity, EntityIds: []uuid.UUID{id}}); err != nil {
			return err
		}
	}
	return nil
}

// findOrCreateCompany resolves a company by exact normalized name, creating it when missing.
func (s *Server) findOrCreateCompany(r *http.Request, q *store.Queries, name string) (*uuid.UUID, error) {
	name = normalize.Text(name)
	if name == "" {
		return nil, nil
	}
	cands, err := s.companyDuplicates(r.Context(), q, name, nil, nil)
	if err != nil {
		return nil, err
	}
	for _, c := range cands {
		if c.Score >= 0.95 {
			return &c.ID, nil
		}
	}
	in := companyInput{Name: name}
	p, err := s.prepareCompany(r.Context(), q, &in)
	if err != nil {
		return nil, err
	}
	c, err := s.createCompany(r, q, p)
	if err != nil {
		return nil, err
	}
	return &c.ID, nil
}

func (s *Server) findOrCreateSupplier(r *http.Request, q *store.Queries, name string) (*uuid.UUID, error) {
	name = normalize.Text(name)
	if name == "" {
		return nil, nil
	}
	cands, err := s.supplierDuplicates(r.Context(), q, name, nil, nil)
	if err != nil {
		return nil, err
	}
	for _, c := range cands {
		if c.Score >= 0.95 {
			return &c.ID, nil
		}
	}
	in := supplierInput{Name: name}
	p, err := s.prepareSupplier(r.Context(), q, &in)
	if err != nil {
		return nil, err
	}
	su, err := s.createSupplier(r, q, p)
	if err != nil {
		return nil, err
	}
	return &su.ID, nil
}

// importRow creates, updates or skips one CSV row and reports the outcome.
func (s *Server) importRow(r *http.Request, q *store.Queries, entity string, row map[string]string, onDup string) (string, error) {
	ctx := r.Context()
	switch entity {
	case "company":
		emp, err := optInt32(row["employee_count"], "employee_count")
		if err != nil {
			return "", err
		}
		in := companyInput{Name: row["name"], Website: strPtr(row["website"]), Domain: strPtr(row["domain"]), Industry: row["industry"],
			CompanyType: strings.ToLower(row["company_type"]), Status: strings.ToLower(row["status"]), Country: row["country"],
			City: row["city"], Address: row["address"], Phone: row["phone"], Email: row["email"], Description: row["description"], EmployeeCount: emp}
		p, err := s.prepareCompany(ctx, q, &in)
		if err != nil {
			return "", err
		}
		var existing *uuid.UUID
		if onDup != "create" {
			cands, err := s.companyDuplicates(ctx, q, p.Name, p.Domain, nil)
			if err != nil {
				return "", err
			}
			for _, c := range cands {
				if c.Score >= 0.95 {
					existing = &c.ID
					break
				}
			}
		}
		if existing != nil {
			if onDup == "skip" {
				return "skipped", nil
			}
			cur, err := q.GetCompany(ctx, *existing)
			if err != nil {
				return "", err
			}
			merged := in
			ci := recordAs[companyInput](cur)
			fillEmpty(&merged, &ci)
			up, err := s.prepareCompany(ctx, q, &merged)
			if err != nil {
				return "", err
			}
			up.ID = *existing
			if _, err := q.UpdateCompany(ctx, up); err != nil {
				return "", err
			}
			return "updated", s.addTagNames(ctx, q, "company", *existing, row["tags"])
		}
		c, err := s.createCompany(r, q, p)
		if err != nil {
			return "", err
		}
		return "created", s.addTagNames(ctx, q, "company", c.ID, row["tags"])

	case "contact":
		first, last := row["first_name"], row["last_name"]
		if first == "" && last == "" && row["full_name"] != "" {
			parts := strings.Fields(row["full_name"])
			first = parts[0]
			if len(parts) > 1 {
				last = strings.Join(parts[1:], " ")
			}
		}
		companyID, err := s.findOrCreateCompany(r, q, row["company"])
		if err != nil {
			return "", err
		}
		supplierID, err := s.findOrCreateSupplier(r, q, row["supplier"])
		if err != nil {
			return "", err
		}
		in := contactInput{FirstName: first, LastName: last, Email: strPtr(row["email"]), Phone: row["phone"], JobTitle: row["job_title"],
			CompanyID: companyID, SupplierID: supplierID, LinkedinURL: row["linkedin_url"], Country: row["country"],
			IsDecisionMaker: parseBool(row["is_decision_maker"])}
		p, err := s.prepareContact(ctx, q, &in)
		if err != nil {
			return "", err
		}
		if onDup != "create" && p.Email != nil {
			cands, err := s.contactDuplicates(ctx, q, strings.TrimSpace(first+" "+last), p.Email, nil, nil)
			if err != nil {
				return "", err
			}
			for _, c := range cands {
				if len(c.Reasons) > 0 && c.Reasons[0] == "same e-mail address" {
					if onDup == "skip" {
						return "skipped", nil
					}
					cur, err := q.GetContact(ctx, c.ID)
					if err != nil {
						return "", err
					}
					merged := in
					ci := recordAs[contactInput](cur)
					fillEmpty(&merged, &ci)
					up, err := s.prepareContact(ctx, q, &merged)
					if err != nil {
						return "", err
					}
					up.ID = c.ID
					if _, err := q.UpdateContact(ctx, up); err != nil {
						return "", err
					}
					return "updated", s.addTagNames(ctx, q, "contact", c.ID, row["tags"])
				}
			}
		}
		c, err := s.createContact(r, q, p)
		if err != nil {
			return "", err
		}
		return "created", s.addTagNames(ctx, q, "contact", c.ID, row["tags"])

	case "supplier":
		lt, err := optInt32(row["lead_time_days"], "lead_time_days")
		if err != nil {
			return "", err
		}
		var rating *int16
		if row["rating"] != "" {
			n, err := strconv.Atoi(row["rating"])
			if err != nil {
				return "", errField("rating", "must be a whole number from 1 to 5")
			}
			v := int16(n)
			rating = &v
		}
		companyID, err := s.findOrCreateCompany(r, q, row["company"])
		if err != nil {
			return "", err
		}
		in := supplierInput{Name: row["name"], SupplierType: strings.ToLower(strings.ReplaceAll(row["supplier_type"], " ", "_")),
			Website: strPtr(row["website"]), Domain: strPtr(row["domain"]), Country: row["country"], City: row["city"],
			Address: row["address"], Email: row["email"], Phone: row["phone"], PaymentTerms: row["payment_terms"], LeadTimeDays: lt,
			MoqNotes: row["moq_notes"], Certifications: splitList(row["certifications"]), Rating: rating,
			Description: row["description"], CompanyID: companyID}
		p, err := s.prepareSupplier(ctx, q, &in)
		if err != nil {
			return "", err
		}
		if onDup != "create" {
			cands, err := s.supplierDuplicates(ctx, q, p.Name, p.Domain, nil)
			if err != nil {
				return "", err
			}
			for _, c := range cands {
				if c.Score < 0.95 {
					continue
				}
				if onDup == "skip" {
					return "skipped", nil
				}
				cur, err := q.GetSupplier(ctx, c.ID)
				if err != nil {
					return "", err
				}
				merged := in
				ci := recordAs[supplierInput](cur)
				fillEmpty(&merged, &ci)
				up, err := s.prepareSupplier(ctx, q, &merged)
				if err != nil {
					return "", err
				}
				up.ID = c.ID
				if _, err := q.UpdateSupplier(ctx, up); err != nil {
					return "", err
				}
				return "updated", s.addTagNames(ctx, q, "supplier", c.ID, row["tags"])
			}
		}
		su, err := s.createSupplier(r, q, p)
		if err != nil {
			return "", err
		}
		return "created", s.addTagNames(ctx, q, "supplier", su.ID, row["tags"])

	case "product":
		return s.importProductRow(r, q, row, onDup)
	}
	return "", errBadRequest("unsupported entity")
}
