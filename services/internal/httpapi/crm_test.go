package httpapi_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCompanyLifecycle(t *testing.T) {
	e := newEnv(t)
	c := e.owner

	bad := c.Do("POST", "/api/v1/companies", map[string]any{"name": "", "email": "nope", "company_type": "alien"}).Must(t, 422)
	for _, f := range []string{"name", "email", "company_type"} {
		if bad.Get("error.fields."+f) == nil {
			t.Errorf("expected field error for %s: %s", f, bad.Body)
		}
	}

	co := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Shenzhen Acme Electronics Co., Ltd.", "website": "www.acme-electronics.example",
		"company_type": "manufacturer", "country": "China", "tag_ids": []string{}}).Must(t, 201)
	id := co.Str("id")
	if co.Str("domain") != "acme-electronics.example" || co.Str("website") != "https://www.acme-electronics.example" {
		t.Fatalf("domain/website not derived: %s", co.Body)
	}
	upd := c.Do("PUT", "/api/v1/companies/"+id, map[string]any{"name": "Shenzhen Acme Electronics", "website": "https://www.acme-electronics.example",
		"company_type": "manufacturer", "country": "China", "industry": "Consumer electronics"}).Must(t, 200)
	if upd.Str("industry") != "Consumer electronics" {
		t.Fatal("update not applied")
	}

	list := c.Do("GET", "/api/v1/companies?q=acme", nil).Must(t, 200)
	if list.Get("total") != float64(1) {
		t.Fatalf("search failed: %s", list.Body)
	}
	c.Do("GET", "/api/v1/companies?company_type=customer", nil).Must(t, 200)

	dups := c.Do("GET", "/api/v1/companies/duplicates?name=ACME+electronics+ltd&domain=acme-electronics.example", nil).Must(t, 200)
	if len(dups.Items()) != 1 || dups.Items()[0]["score"].(float64) < 0.99 {
		t.Fatalf("duplicate detection failed: %s", dups.Body)
	}

	// Activity timeline records create and update.
	act := c.Do("GET", "/api/v1/activity?entity_type=company&entity_id="+id, nil).Must(t, 200)
	if len(act.Items()) < 2 {
		t.Fatalf("expected activity entries: %s", act.Body)
	}

	// Notes are removed with the company by the cleanup trigger.
	c.Do("POST", "/api/v1/notes", map[string]any{"entity_type": "company", "entity_id": id, "body": "Met at the Canton Fair"}).Must(t, 201)
	c.Do("DELETE", "/api/v1/companies/"+id, nil).Must(t, 204)
	c.Do("GET", "/api/v1/companies/"+id, nil).Must(t, 404)
	var notes int
	if err := e.db.Pool.QueryRow(t.Context(), "SELECT count(*) FROM notes WHERE entity_id = $1", id).Scan(&notes); err != nil || notes != 0 {
		t.Fatalf("notes not cleaned up: %d %v", notes, err)
	}
}

func TestMergeCompaniesMovesRelations(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	keep := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Globex"}).Must(t, 201).Str("id")
	dup := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Globex Inc.", "website": "https://globex.example", "phone": "+1 555"}).Must(t, 201).Str("id")
	contact := c.Do("POST", "/api/v1/contacts", map[string]any{"first_name": "Hank", "last_name": "Scorpio", "company_id": dup}).Must(t, 201).Str("id")
	c.Do("POST", "/api/v1/notes", map[string]any{"entity_type": "company", "entity_id": dup, "body": "Important history"}).Must(t, 201)

	c.Do("POST", "/api/v1/companies/"+keep+"/merge", map[string]any{"source_ids": []string{keep}}).Must(t, 422)
	merged := c.Do("POST", "/api/v1/companies/"+keep+"/merge", map[string]any{"source_ids": []string{dup}}).Must(t, 200)
	if merged.Str("domain") != "globex.example" || merged.Str("phone") != "+1 555" {
		t.Fatalf("empty fields not filled from source: %s", merged.Body)
	}
	if c.Do("GET", "/api/v1/contacts/"+contact, nil).Must(t, 200).Str("company_id") != keep {
		t.Fatal("contact not moved to merged company")
	}
	if len(c.Do("GET", "/api/v1/notes?entity_type=company&entity_id="+keep, nil).Must(t, 200).Items()) != 1 {
		t.Fatal("note not moved")
	}
	c.Do("GET", "/api/v1/companies/"+dup, nil).Must(t, 404)
}

func TestBulkActionsAndTags(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, c.Do("POST", "/api/v1/companies", map[string]any{"name": fmt.Sprintf("Bulk %d", i)}).Must(t, 201).Str("id"))
	}
	tag := c.Do("POST", "/api/v1/tags", map[string]any{"name": "Priority", "color": "#ff0000"}).Must(t, 201).Str("id")
	c.Do("POST", "/api/v1/companies/bulk", map[string]any{"action": "add_tag", "ids": ids, "value": tag}).Must(t, 200)
	c.Do("POST", "/api/v1/companies/bulk", map[string]any{"action": "set_status", "ids": ids[:2], "value": "inactive"}).Must(t, 200)
	c.Do("POST", "/api/v1/companies/bulk", map[string]any{"action": "explode", "ids": ids}).Must(t, 422)
	tagged := c.Do("GET", "/api/v1/companies?tag_id="+tag, nil).Must(t, 200)
	if tagged.Get("total") != float64(3) {
		t.Fatalf("tag filter failed: %s", tagged.Body)
	}
	inactive := c.Do("GET", "/api/v1/companies?status=inactive", nil).Must(t, 200)
	if inactive.Get("total") != float64(2) {
		t.Fatalf("bulk status failed: %s", inactive.Body)
	}
	set := c.Do("PUT", "/api/v1/record-tags", map[string]any{"entity_type": "company", "entity_id": ids[0], "tag_names": []string{"New tag"}}).Must(t, 200)
	if len(set.Items()) != 1 || set.Items()[0]["name"] != "New tag" {
		t.Fatalf("record tags replace failed: %s", set.Body)
	}
	c.Do("POST", "/api/v1/companies/bulk", map[string]any{"action": "delete", "ids": ids}).Must(t, 200)
	if c.Do("GET", "/api/v1/companies", nil).Must(t, 200).Get("total") != float64(0) {
		t.Fatal("bulk delete failed")
	}
}

func TestViewerCannotWriteCRM(t *testing.T) {
	e := newEnv(t)
	id := e.owner.Do("POST", "/api/v1/companies", map[string]any{"name": "Read only"}).Must(t, 201).Str("id")
	v := e.userWithRole("viewer", "v@example.com")
	v.Do("GET", "/api/v1/companies/"+id, nil).Must(t, 200)
	v.Do("POST", "/api/v1/companies", map[string]any{"name": "Nope"}).Must(t, 403)
	v.Do("PUT", "/api/v1/companies/"+id, map[string]any{"name": "Nope"}).Must(t, 403)
	v.Do("DELETE", "/api/v1/companies/"+id, nil).Must(t, 403)
	v.Do("POST", "/api/v1/notes", map[string]any{"entity_type": "company", "entity_id": id, "body": "x"}).Must(t, 403)
	v.Do("GET", "/api/v1/companies/export", nil).Must(t, 403)
	m := e.userWithRole("member", "m@example.com")
	m.Do("DELETE", "/api/v1/companies/"+id, nil).Must(t, 403)
}

func TestSupplierPipelineAndContacts(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	su := c.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Ningbo Widgets Factory", "supplier_type": "manufacturer",
		"website": "https://ningbo-widgets.example", "rating": 4, "certifications": []string{"ISO 9001", " ", "CE"}}).Must(t, 201)
	sid := su.Str("id")
	if su.Get("pipeline_stage_id") == nil || len(su.Get("certifications").([]any)) != 2 {
		t.Fatalf("supplier defaults wrong: %s", su.Body)
	}
	c.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Bad", "rating": 9}).Must(t, 422)
	pipes := c.Do("GET", "/api/v1/pipelines?entity_type=supplier", nil).Must(t, 200)
	stages := pipes.Items()[0]["stages"].([]any)
	approved := stages[4].(map[string]any)["id"].(string)
	moved := c.Do("PATCH", "/api/v1/suppliers/"+sid+"/stage", map[string]any{"stage_id": approved}).Must(t, 200)
	if moved.Str("pipeline_stage_id") != approved {
		t.Fatal("stage not set")
	}
	leadStage := c.Do("GET", "/api/v1/pipelines?entity_type=lead", nil).Must(t, 200).Items()[0]["stages"].([]any)[0].(map[string]any)["id"].(string)
	c.Do("PATCH", "/api/v1/suppliers/"+sid+"/stage", map[string]any{"stage_id": leadStage}).Must(t, 422)

	ct := c.Do("POST", "/api/v1/contacts", map[string]any{"first_name": "Li", "last_name": "Wei", "email": "LI.WEI@ningbo-widgets.example",
		"supplier_id": sid, "is_decision_maker": true}).Must(t, 201)
	if ct.Str("email") != "li.wei@ningbo-widgets.example" {
		t.Fatalf("email not normalized: %s", ct.Body)
	}
	dups := c.Do("GET", "/api/v1/contacts/duplicates?email=li.wei@ningbo-widgets.example", nil).Must(t, 200)
	if len(dups.Items()) != 1 {
		t.Fatalf("contact duplicate by email not found: %s", dups.Body)
	}
	list := c.Do("GET", "/api/v1/contacts?supplier_id="+sid+"&decision_maker=true", nil).Must(t, 200)
	if list.Get("total") != float64(1) || list.Items()[0]["supplier_name"] != "Ningbo Widgets Factory" {
		t.Fatalf("contact filters failed: %s", list.Body)
	}
}

func TestLeadConversionAndOpportunityStages(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	co := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Buyer Co"}).Must(t, 201).Str("id")
	lead := c.Do("POST", "/api/v1/leads", map[string]any{"title": "Retail chain wants 5k units", "company_id": co, "source": "referral",
		"estimated_value": "25000.00", "currency": "usd"}).Must(t, 201)
	if lead.Str("currency") != "USD" || lead.Str("stage_name") != "" && lead.Get("pipeline_stage_id") == nil {
		t.Fatalf("lead defaults wrong: %s", lead.Body)
	}
	conv := c.Do("POST", "/api/v1/leads/"+lead.Str("id")+"/convert", map[string]any{}).Must(t, 201)
	oppID := conv.Str("opportunity.id")
	if conv.Get("lead.converted_at") == nil || conv.Str("opportunity.amount") != "25000" {
		t.Fatalf("conversion wrong: %s", conv.Body)
	}
	c.Do("POST", "/api/v1/leads/"+lead.Str("id")+"/convert", map[string]any{}).Must(t, 409)

	var won string
	pipes := c.Do("GET", "/api/v1/pipelines?entity_type=opportunity", nil).Must(t, 200)
	for _, st := range pipes.Items()[0]["stages"].([]any) {
		if st.(map[string]any)["category"] == "won" {
			won = st.(map[string]any)["id"].(string)
		}
	}
	o := c.Do("PATCH", "/api/v1/opportunities/"+oppID+"/stage", map[string]any{"stage_id": won}).Must(t, 200)
	if o.Str("status") != "won" || o.Get("closed_at") == nil || o.Get("probability") != float64(100) {
		t.Fatalf("won stage not applied: %s", o.Body)
	}
	sum := c.Do("GET", "/api/v1/pipelines/"+pipes.Items()[0]["id"].(string)+"/summary", nil).Must(t, 200)
	if len(sum.Get("stages").([]any)) == 0 {
		t.Fatal("pipeline summary empty")
	}
	dash := c.Do("GET", "/api/v1/dashboard", nil).Must(t, 200)
	if dash.Get("counts.won_last_30_days") != float64(1) {
		t.Fatalf("dashboard counts wrong: %s", dash.Body)
	}
}

func TestPipelineEditing(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	p := c.Do("POST", "/api/v1/pipelines", map[string]any{"entity_type": "opportunity", "name": "Software projects",
		"stages": []map[string]any{{"name": "Scoping"}, {"name": "Delivered", "category": "won"}}}).Must(t, 201)
	st := p.Get("stages").([]any)
	first := st[0].(map[string]any)["id"]
	upd := c.Do("PUT", "/api/v1/pipelines/"+p.Str("id"), map[string]any{"name": "Software projects", "stages": []map[string]any{
		{"name": "Discovery"}, {"id": first, "name": "Scoping"}, {"name": "Cancelled", "category": "lost"}}}).Must(t, 200)
	got := upd.Get("stages").([]any)
	if len(got) != 3 || got[1].(map[string]any)["id"] != first || got[0].(map[string]any)["name"] != "Discovery" {
		t.Fatalf("stage sync wrong: %s", upd.Body)
	}
	c.Do("POST", "/api/v1/pipelines", map[string]any{"entity_type": "opportunity", "name": "Dupes",
		"stages": []map[string]any{{"name": "A"}, {"name": "a"}}}).Must(t, 422)
}

func TestTasksAndReminders(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	co := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Follow-up Ltd"}).Must(t, 201).Str("id")
	me := c.Do("GET", "/api/v1/auth/me", nil).Str("user.id")
	past := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	task := c.Do("POST", "/api/v1/tasks", map[string]any{"title": "Ask for samples", "task_type": "follow_up", "due_at": past, "remind_at": past,
		"assignee_id": me, "entity_type": "company", "entity_id": co}).Must(t, 201)
	if task.Get("overdue") != true {
		t.Fatal("task should be overdue")
	}
	c.Do("POST", "/api/v1/tasks", map[string]any{"title": "Orphan", "entity_type": "company"}).Must(t, 422)
	c.Do("POST", "/api/v1/tasks", map[string]any{"title": "Ghost", "entity_type": "company", "entity_id": "00000000-0000-0000-0000-000000000001"}).Must(t, 404)
	mine := c.Do("GET", "/api/v1/tasks?assignee_id=me&open_only=true", nil).Must(t, 200)
	if mine.Get("total") != float64(1) {
		t.Fatalf("my tasks wrong: %s", mine.Body)
	}
	rem := c.Do("GET", "/api/v1/tasks/reminders", nil).Must(t, 200)
	if len(rem.Items()) != 1 {
		t.Fatalf("reminder not due: %s", rem.Body)
	}
	done := c.Do("PUT", "/api/v1/tasks/"+task.Str("id"), map[string]any{"title": "Ask for samples", "status": "done", "assignee_id": me,
		"entity_type": "company", "entity_id": co}).Must(t, 200)
	if done.Get("completed_at") == nil || done.Get("overdue") != false {
		t.Fatalf("completion wrong: %s", done.Body)
	}
	// Deleting the company keeps the task but unlinks it.
	c.Do("DELETE", "/api/v1/companies/"+co, nil).Must(t, 204)
	if c.Do("GET", "/api/v1/tasks/"+task.Str("id"), nil).Must(t, 200).Get("entity_id") != nil {
		t.Fatal("task should be unlinked")
	}
}

func TestCustomFieldsValidation(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	c.Do("POST", "/api/v1/custom-fields", map[string]any{"entity_type": "company", "key": "tier", "label": "Tier", "field_type": "select",
		"options": []string{"Gold", "Silver"}, "required": true}).Must(t, 201)
	c.Do("POST", "/api/v1/custom-fields", map[string]any{"entity_type": "company", "key": "founded", "label": "Founded", "field_type": "date"}).Must(t, 201)
	c.Do("POST", "/api/v1/companies", map[string]any{"name": "No tier"}).Must(t, 422)
	bad := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Bad", "custom_fields": map[string]any{"tier": "Bronze", "founded": "yesterday", "nope": 1}}).Must(t, 422)
	for _, f := range []string{"custom_fields.tier", "custom_fields.founded", "custom_fields.nope"} {
		if _, ok := bad.Get("error.fields").(map[string]any)[f]; !ok {
			t.Errorf("expected error on %s: %s", f, bad.Body)
		}
	}
	good := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Good", "custom_fields": map[string]any{"tier": "Gold", "founded": "1999-05-01"}}).Must(t, 201)
	if good.Str("custom_fields.tier") != "Gold" {
		t.Fatalf("custom field not stored: %s", good.Body)
	}
}

func TestAttachmentsAreServedSafely(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	co := c.Do("POST", "/api/v1/companies", map[string]any{"name": "Files Inc"}).Must(t, 201).Str("id")
	up := c.Upload("/api/v1/attachments", "file", "../../evil.html", []byte("<html><script>alert(1)</script></html>"),
		map[string]string{"entity_type": "company", "entity_id": co}).Must(t, 201)
	if up.Str("filename") != "evil.html" || up.Str("content_type") != "text/plain" {
		t.Fatalf("upload not sanitized: %s", up.Body)
	}
	dl := c.Do("GET", "/api/v1/attachments/"+up.Str("id")+"/download", nil).Must(t, 200)
	if !strings.HasPrefix(dl.Header.Get("Content-Disposition"), "attachment") || strings.Contains(dl.Header.Get("Content-Type"), "html") {
		t.Fatalf("unsafe download headers: %v", dl.Header)
	}
	c.Upload("/api/v1/attachments", "file", "setup.exe", []byte("MZ"), map[string]string{"entity_type": "company", "entity_id": co}).Must(t, 422)
	list := c.Do("GET", "/api/v1/attachments?entity_type=company&entity_id="+co, nil).Must(t, 200)
	if len(list.Items()) != 1 {
		t.Fatal("attachment not listed")
	}
	c.Do("DELETE", "/api/v1/attachments/"+up.Str("id"), nil).Must(t, 204)
}

func TestCSVExportIsInjectionSafe(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	c.Do("POST", "/api/v1/companies", map[string]any{"name": "=HYPERLINK(\"http://evil\")", "description": "+cmd"}).Must(t, 201)
	res := c.Do("GET", "/api/v1/companies/export?format=csv", nil).Must(t, 200)
	body := string(res.Body)
	if !strings.Contains(body, `'=HYPERLINK`) || !strings.Contains(body, `'+cmd`) {
		t.Fatalf("formula injection not neutralised: %s", body)
	}
	js := c.Do("GET", "/api/v1/companies/export?format=json", nil).Must(t, 200)
	if len(js.Items()) != 1 {
		t.Fatalf("json export wrong: %s", js.Body)
	}
}

func TestCSVImportWithDryRunAndDuplicates(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	c.Do("POST", "/api/v1/companies", map[string]any{"name": "Existing Corp", "website": "https://existing.example"}).Must(t, 201)
	csv := "Company Name,Website,Country,Employees,Tags\n" +
		"Existing Corp,https://existing.example,USA,50,Customer\n" +
		"New Widgets,new-widgets.example,Germany,12,Prospect;EU\n" +
		",missing-name.example,France,,\n" +
		"Bad Employees,bad.example,Spain,lots,\n"
	prev := c.Upload("/api/v1/import/csv/preview", "file", "companies.csv", []byte(csv), map[string]string{"entity_type": "company"}).Must(t, 200)
	if prev.Str("mapping.Company Name") != "name" || prev.Str("mapping.Employees") != "employee_count" || prev.Get("total_rows") != float64(4) {
		t.Fatalf("mapping suggestion wrong: %s", prev.Body)
	}
	dry := c.Upload("/api/v1/import/csv/commit", "file", "companies.csv", []byte(csv), map[string]string{"entity_type": "company", "dry_run": "true"}).Must(t, 200)
	if dry.Get("created") != float64(1) || dry.Get("skipped") != float64(1) || dry.Get("failed") != float64(2) {
		t.Fatalf("dry run result wrong: %s", dry.Body)
	}
	if c.Do("GET", "/api/v1/companies", nil).Get("total") != float64(1) {
		t.Fatal("dry run wrote data")
	}
	res := c.Upload("/api/v1/import/csv/commit", "file", "companies.csv", []byte(csv), map[string]string{"entity_type": "company", "on_duplicate": "update"}).Must(t, 200)
	if res.Get("created") != float64(1) || res.Get("updated") != float64(1) || res.Get("failed") != float64(2) {
		t.Fatalf("import result wrong: %s", res.Body)
	}
	errs := res.Get("errors").([]any)
	if errs[0].(map[string]any)["row"] != float64(4) {
		t.Fatalf("row numbers wrong: %v", errs)
	}
	nw := c.Do("GET", "/api/v1/companies?q="+url.QueryEscape("New Widgets"), nil).Must(t, 200).Items()
	if len(nw) != 1 || len(nw[0]["tags"].([]any)) != 2 || nw[0]["domain"] != "new-widgets.example" {
		t.Fatalf("imported company wrong: %v", nw)
	}
	contacts := "Name,Email,Company\nAda Lovelace,ada@engines.example,Analytical Engines\n"
	cr := c.Upload("/api/v1/import/csv/commit", "file", "contacts.csv", []byte(contacts), map[string]string{"entity_type": "contact"}).Must(t, 200)
	if cr.Get("created") != float64(1) {
		t.Fatalf("contact import failed: %s", cr.Body)
	}
	ada := c.Do("GET", "/api/v1/contacts?q=ada", nil).Must(t, 200).Items()
	if len(ada) != 1 || ada[0]["company_name"] != "Analytical Engines" {
		t.Fatalf("contact company not created/linked: %v", ada)
	}
}

func TestGlobalSearchRespectsPermissions(t *testing.T) {
	e := newEnv(t)
	e.owner.Do("POST", "/api/v1/companies", map[string]any{"name": "Zircon Holdings"}).Must(t, 201)
	res := e.owner.Do("GET", "/api/v1/search?q=zircon", nil).Must(t, 200)
	if len(res.Items()) != 1 || res.Items()[0]["entity_type"] != "company" {
		t.Fatalf("search failed: %s", res.Body)
	}
	// A role with only analysis permissions must not see CRM hits.
	role := e.owner.Do("POST", "/api/v1/roles", map[string]any{"key": "auditor", "name": "Auditor", "permissions": []string{"analysis.read"}}).Must(t, 201)
	e.owner.Do("POST", "/api/v1/users", map[string]any{"email": "a@example.com", "display_name": "A", "password": ownerPassword, "role_id": role.Str("id")}).Must(t, 201)
	a := e.login("a@example.com", ownerPassword)
	if len(a.Do("GET", "/api/v1/search?q=zircon", nil).Must(t, 200).Items()) != 0 {
		t.Fatal("search leaked CRM records")
	}
}

// TestListsWithMinimalRecords is a regression test: list and export endpoints must work for
// records that have none of their optional links (no contact, no primary contact, no image,
// no owner, no stage, no assignee).
func TestListsWithMinimalRecords(t *testing.T) {
	e := newEnv(t)
	c := e.owner
	c.Do("POST", "/api/v1/companies", map[string]any{"name": "Bare Co"}).Must(t, 201)
	c.Do("POST", "/api/v1/contacts", map[string]any{"first_name": "Solo"}).Must(t, 201)
	c.Do("POST", "/api/v1/suppliers", map[string]any{"name": "Bare Supplier"}).Must(t, 201)
	c.Do("POST", "/api/v1/leads", map[string]any{"title": "Bare lead"}).Must(t, 201)
	c.Do("POST", "/api/v1/opportunities", map[string]any{"name": "Bare deal"}).Must(t, 201)
	c.Do("POST", "/api/v1/products", map[string]any{"title": "Bare product"}).Must(t, 201)
	c.Do("POST", "/api/v1/tasks", map[string]any{"title": "Bare task"}).Must(t, 201)
	for _, path := range []string{"companies", "contacts", "suppliers", "leads", "opportunities", "products", "tasks"} {
		items := c.Do("GET", "/api/v1/"+path, nil).Must(t, 200).Items()
		if len(items) != 1 {
			t.Fatalf("%s: expected 1 item, got %d", path, len(items))
		}
		for _, k := range []string{"contact_name", "primary_contact_name", "primary_image_url"} {
			if v, ok := items[0][k]; ok && v != nil {
				t.Errorf("%s: %s should be absent or null for a record without it, got %v", path, k, v)
			}
		}
		for _, format := range []string{"csv", "json"} {
			c.Do("GET", "/api/v1/"+path+"/export?format="+format, nil).Must(t, 200)
		}
	}
	c.Do("GET", "/api/v1/products/facets", nil).Must(t, 200)
	c.Do("GET", "/api/v1/dashboard", nil).Must(t, 200)
	c.Do("GET", "/api/v1/search?q=Bare", nil).Must(t, 200)
}
