package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/ai"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/analysis"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/catalog"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/products"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/webhooks"
)

// The OpenAPI document is generated from the Go request and response types so that it cannot
// drift from the implementation. Tests check that every route is documented and that every
// response produced by the integration tests validates against it.

type opDoc struct {
	Method, Path, ID, Tag, Summary string
	Req                            any
	Resp                           any
	Status                         int
	Query                          []string
	Multipart                      []string // form fields; "file" is the upload
	Produces                       string   // non-JSON response media type
	Public                         bool
	ETag                           bool // answers 304 when If-None-Match matches the ETag
}

func op(method, path, id, tag, summary string, req, resp any, status int, query ...string) opDoc {
	if status == 0 {
		status = http.StatusOK
	}
	return opDoc{Method: method, Path: path, ID: id, Tag: tag, Summary: summary, Req: req, Resp: resp, Status: status, Query: query}
}

func (o opDoc) public() opDoc               { o.Public = true; return o }
func (o opDoc) etag() opDoc                 { o.ETag = true; return o }
func (o opDoc) produces(mt string) opDoc    { o.Produces = mt; return o }
func (o opDoc) multipart(f ...string) opDoc { o.Multipart = f; return o }

var listQ = []string{"q", "limit", "offset", "sort"}

func lq(extra ...string) []string { return append(append([]string{}, listQ...), extra...) }

type noBody struct{}

// Operations documents every API route.
func Operations() []opDoc {
	crud := func(base, plural, singular, tag string, view, page, input any, filters ...string) []opDoc {
		cap := strings.ToUpper(singular[:1]) + singular[1:]
		capP := strings.ToUpper(plural[:1]) + plural[1:]
		return []opDoc{
			op("GET", base, "list"+capP, tag, "List "+plural, nil, page, 200, lq(filters...)...),
			op("POST", base, "create"+cap, tag, "Create a "+singular, input, view, 201),
			op("GET", base+"/{id}", "get"+cap, tag, "Get a "+singular, nil, view, 200),
			op("PUT", base+"/{id}", "update"+cap, tag, "Replace a "+singular, input, view, 200),
			op("DELETE", base+"/{id}", "delete"+cap, tag, "Delete a "+singular, nil, nil, 204),
			op("POST", base+"/bulk", "bulk"+capP, tag, "Apply a bulk action", bulkInput{}, affectedDoc{}, 200),
			op("GET", base+"/export", "export"+capP, tag, "Export as CSV or JSON", nil, nil, 200, append(lq(filters...), "format")...).produces("text/csv,application/json"),
		}
	}
	ops := []opDoc{
		op("GET", "/healthz", "health", "System", "Liveness probe", nil, statusDoc{}, 200).public(),
		op("GET", "/readyz", "ready", "System", "Readiness probe (database and migrations)", nil, statusDoc{}, 200).public(),
		op("GET", "/api/v1/openapi.json", "openapi", "System", "This OpenAPI document", nil, nil, 200).public(),
		op("GET", "/api/v1/setup/status", "setupStatus", "Auth", "Whether first-run setup is needed", nil, setupStatusDoc{}, 200).public(),
		op("POST", "/api/v1/setup", "setup", "Auth", "Create the owner account (first run only)", setupInput{}, meDoc{}, 201).public(),
		op("POST", "/api/v1/auth/login", "login", "Auth", "Sign in with e-mail and password", loginInput{}, meDoc{}, 200).public(),
		op("POST", "/api/v1/auth/logout", "logout", "Auth", "Sign out", nil, nil, 204),
		op("GET", "/api/v1/auth/me", "me", "Auth", "Current user, permissions and CSRF token", nil, meDoc{}, 200),
		op("PUT", "/api/v1/auth/me", "updateMe", "Auth", "Update own profile", updateMeInput{}, meDoc{}, 200),
		op("GET", "/api/v1/auth/me/export", "exportMyData", "Auth", "Download everything stored about me", nil, nil, 200).produces("application/zip"),
		op("POST", "/api/v1/auth/password", "changePassword", "Auth", "Change own password", changePasswordInput{}, nil, 204),
		op("GET", "/api/v1/auth/sessions", "listSessions", "Auth", "Active sessions", nil, Items[SessionView]{}, 200),
		op("DELETE", "/api/v1/auth/sessions/{id}", "revokeSession", "Auth", "Sign out a session", nil, nil, 204),
		op("GET", "/api/v1/auth/tokens", "listTokens", "Auth", "Personal API tokens", nil, Items[tokenView]{}, 200),
		op("POST", "/api/v1/auth/tokens", "createToken", "Auth", "Create a personal API token", createTokenInput{}, tokenCreatedDoc{}, 201),
		op("DELETE", "/api/v1/auth/tokens/{id}", "revokeToken", "Auth", "Revoke a personal API token", nil, nil, 204),
		op("GET", "/api/v1/system/info", "systemInfo", "System", "Server version and features", nil, systemInfoDoc{}, 200),
		op("GET", "/api/v1/permissions", "listPermissions", "Admin", "All permission codes", nil, Items[store.Permission]{}, 200),
		op("GET", "/api/v1/users/directory", "userDirectory", "Admin", "Active users for pickers", nil, Items[DirectoryUser]{}, 200),
		op("GET", "/api/v1/users", "listUsers", "Admin", "List users (q matches name or e-mail)", nil, Page[UserView]{}, 200, "q", "limit", "offset"),
		op("POST", "/api/v1/users", "createUser", "Admin", "Create a user", createUserInput{}, UserView{}, 201),
		op("GET", "/api/v1/users/{id}", "getUser", "Admin", "Get a user", nil, UserView{}, 200),
		op("PUT", "/api/v1/users/{id}", "updateUser", "Admin", "Update a user's name, e-mail, role and status", updateUserInput{}, UserView{}, 200),
		op("POST", "/api/v1/users/{id}/unlock", "unlockUser", "Admin", "Clear a sign-in lockout", nil, nil, 204),
		op("DELETE", "/api/v1/users/{id}", "deleteUser", "Admin", "Delete a user", nil, nil, 204),
		op("POST", "/api/v1/users/{id}/password", "resetUserPassword", "Admin", "Set a user's password and sign them out", resetPasswordInput{}, nil, 204),
		op("POST", "/api/v1/users/{id}/transfer-ownership", "transferOwnership", "Admin", "Make another administrator the owner", nil, nil, 204),
		op("GET", "/api/v1/roles", "listRoles", "Admin", "Roles with their permissions", nil, Items[store.ListRolesRow]{}, 200),
		op("POST", "/api/v1/roles", "createRole", "Admin", "Create a custom role", roleInput{}, store.Role{}, 201),
		op("PUT", "/api/v1/roles/{id}", "updateRole", "Admin", "Update a role", roleInput{}, store.Role{}, 200),
		op("DELETE", "/api/v1/roles/{id}", "deleteRole", "Admin", "Delete an unused custom role", nil, nil, 204),
		op("GET", "/api/v1/settings", "getSettings", "Settings", "All settings with defaults", nil, settingsDoc{}, 200),
		op("PUT", "/api/v1/settings/{key}", "putSetting", "Settings", "Update one setting", map[string]any{}, store.AppSetting{}, 200),
		op("GET", "/api/v1/workspaces", "listWorkspaces", "Analysis", "Workspaces", nil, Items[store.ListWorkspacesRow]{}, 200),
		op("POST", "/api/v1/workspaces", "createWorkspace", "Analysis", "Create a workspace", workspaceInput{}, store.Workspace{}, 201),
		op("PUT", "/api/v1/workspaces/{id}", "updateWorkspace", "Analysis", "Update a workspace", workspaceInput{}, store.Workspace{}, 200),
		op("DELETE", "/api/v1/workspaces/{id}", "deleteWorkspace", "Analysis", "Delete an empty workspace", nil, nil, 204),
		op("GET", "/api/v1/audit-logs", "listAuditLogs", "Admin", "Audit log (newest first, keyset pagination)", nil, auditPageDoc{}, 200, "entity_type", "entity_id", "actor_user_id", "action", "before_id", "limit"),
		op("GET", "/api/v1/activity", "recordActivity", "CRM", "Timeline of one record", nil, Items[store.AuditLog]{}, 200, "entity_type", "entity_id"),

		op("POST", "/api/v1/extension/pairing-codes", "createPairingCode", "Extension", "Create a one-time pairing code", nil, pairingCodeDoc{}, 201),
		op("POST", "/api/v1/extension/pair", "pairExtension", "Extension", "Exchange a pairing code for tokens", pairInput{}, tokenPair{}, 201).public(),
		op("POST", "/api/v1/extension/token/refresh", "refreshExtensionToken", "Extension", "Rotate the extension's tokens", refreshInput{}, tokenPair{}, 200).public(),
		op("GET", "/api/v1/extension/devices", "listDevices", "Extension", "Paired browser extensions", nil, Items[store.ListExtensionDevicesRow]{}, 200, "all"),
		op("DELETE", "/api/v1/extension/devices/{id}", "revokeDevice", "Extension", "Revoke a paired extension", nil, nil, 204),
		op("GET", "/api/v1/extension/config", "extensionConfig", "Extension", "Configuration for the extension", nil, extensionConfigDoc{}, 200),
		op("POST", "/api/v1/extension/disconnect", "disconnectExtension", "Extension", "Revoke the calling extension", nil, nil, 204),

		op("GET", "/api/v1/companies/duplicates", "companyDuplicates", "CRM", "Possible duplicate companies", nil, Items[duplicateCandidate]{}, 200, "name", "domain", "website", "exclude_id"),
		op("POST", "/api/v1/companies/{id}/merge", "mergeCompanies", "CRM", "Merge companies into this one", mergeInput{}, companyView{}, 200),
		op("GET", "/api/v1/contacts/duplicates", "contactDuplicates", "CRM", "Possible duplicate contacts", nil, Items[duplicateCandidate]{}, 200, "name", "first_name", "last_name", "email", "company_id", "exclude_id"),
		op("POST", "/api/v1/contacts/{id}/merge", "mergeContacts", "CRM", "Merge contacts into this one", mergeInput{}, contactView{}, 200),
		op("GET", "/api/v1/suppliers/duplicates", "supplierDuplicates", "CRM", "Possible duplicate suppliers", nil, Items[duplicateCandidate]{}, 200, "name", "domain", "website", "exclude_id"),
		op("POST", "/api/v1/suppliers/{id}/merge", "mergeSuppliers", "CRM", "Merge suppliers into this one", mergeInput{}, supplierView{}, 200),
		op("PATCH", "/api/v1/suppliers/{id}/stage", "setSupplierStage", "CRM", "Move a supplier to a qualification stage", stageInput{}, store.Supplier{}, 200),
		op("GET", "/api/v1/suppliers/{id}/products", "listSupplierProducts", "CRM", "Products linked to a supplier", nil, Items[store.ListSupplierProductsRow]{}, 200),
		op("PUT", "/api/v1/suppliers/{id}/products/{productID}", "linkSupplierProduct", "Products", "Link or update a supplier's product terms", supplierProductInput{}, store.SupplierProduct{}, 200),
		op("DELETE", "/api/v1/suppliers/{id}/products/{productID}", "unlinkSupplierProduct", "Products", "Remove a supplier-product link", nil, nil, 204),
		op("PATCH", "/api/v1/leads/{id}/stage", "setLeadStage", "CRM", "Move a lead", stageInput{}, store.Lead{}, 200),
		op("POST", "/api/v1/leads/{id}/convert", "convertLead", "CRM", "Convert a lead into an opportunity", convertLeadInput{}, convertLeadDoc{}, 201),
		op("PATCH", "/api/v1/opportunities/{id}/stage", "setOpportunityStage", "CRM", "Move an opportunity", stageInput{}, store.Opportunity{}, 200),
		op("GET", "/api/v1/opportunities/{id}/products", "listOpportunityProducts", "CRM", "Quoted products", nil, Items[store.ListOpportunityProductsRow]{}, 200),
		op("PUT", "/api/v1/opportunities/{id}/products/{productID}", "upsertOpportunityProduct", "CRM", "Add or update a quoted product", opportunityProductInput{}, store.OpportunityProduct{}, 200),
		op("DELETE", "/api/v1/opportunities/{id}/products/{productID}", "deleteOpportunityProduct", "CRM", "Remove a quoted product", nil, nil, 204),
		op("GET", "/api/v1/pipelines", "listPipelines", "CRM", "Pipelines with stages", nil, Items[pipelineView]{}, 200, "entity_type"),
		op("POST", "/api/v1/pipelines", "createPipeline", "CRM", "Create a pipeline", pipelineInput{}, pipelineView{}, 201),
		op("PUT", "/api/v1/pipelines/{id}", "updatePipeline", "CRM", "Rename and re-stage a pipeline", pipelineInput{}, pipelineView{}, 200),
		op("DELETE", "/api/v1/pipelines/{id}", "deletePipeline", "CRM", "Delete a non-default pipeline", nil, nil, 204),
		op("GET", "/api/v1/pipelines/{id}/summary", "pipelineSummary", "CRM", "Deal counts and amounts per stage", nil, pipelineSummaryDoc{}, 200),
		op("GET", "/api/v1/tasks/reminders", "taskReminders", "CRM", "My due reminders", nil, Items[store.DueRemindersRow]{}, 200),
		op("GET", "/api/v1/notes", "listNotes", "CRM", "Notes on a record", nil, Items[store.ListNotesRow]{}, 200, "entity_type", "entity_id"),
		op("POST", "/api/v1/notes", "createNote", "CRM", "Add a note or comment", noteInput{}, store.Note{}, 201),
		op("PUT", "/api/v1/notes/{id}", "updateNote", "CRM", "Edit a note", noteUpdateInput{}, store.Note{}, 200),
		op("DELETE", "/api/v1/notes/{id}", "deleteNote", "CRM", "Delete a note", nil, nil, 204),
		op("GET", "/api/v1/attachments", "listAttachments", "CRM", "Attachments on a record", nil, Items[store.ListAttachmentsRow]{}, 200, "entity_type", "entity_id"),
		op("POST", "/api/v1/attachments", "uploadAttachment", "CRM", "Upload an attachment", nil, store.Attachment{}, 201).multipart("file", "entity_type", "entity_id"),
		op("GET", "/api/v1/attachments/{id}/download", "downloadAttachment", "CRM", "Download an attachment", nil, nil, 200, "inline").produces("application/octet-stream"),
		op("DELETE", "/api/v1/attachments/{id}", "deleteAttachment", "CRM", "Delete an attachment", nil, nil, 204),
		op("GET", "/api/v1/tags", "listTags", "CRM", "Tags with usage counts", nil, Items[store.ListTagsRow]{}, 200),
		op("POST", "/api/v1/tags", "createTag", "CRM", "Create a tag", tagInput{}, store.Tag{}, 201),
		op("PUT", "/api/v1/tags/{id}", "updateTag", "CRM", "Rename or recolour a tag", tagInput{}, store.Tag{}, 200),
		op("DELETE", "/api/v1/tags/{id}", "deleteTag", "CRM", "Delete a tag", nil, nil, 204),
		op("PUT", "/api/v1/record-tags", "setRecordTags", "CRM", "Replace a record's tags", recordTagsInput{}, Items[tagRef]{}, 200),
		op("GET", "/api/v1/custom-fields", "listCustomFields", "Settings", "Custom field definitions", nil, Items[store.CustomFieldDefinition]{}, 200, "entity_type"),
		op("POST", "/api/v1/custom-fields", "createCustomField", "Settings", "Define a custom field", customFieldInput{}, store.CustomFieldDefinition{}, 201),
		op("PUT", "/api/v1/custom-fields/{id}", "updateCustomField", "Settings", "Update a custom field", customFieldInput{}, store.CustomFieldDefinition{}, 200),
		op("DELETE", "/api/v1/custom-fields/{id}", "deleteCustomField", "Settings", "Delete a custom field definition", nil, nil, 204),
		op("GET", "/api/v1/saved-filters", "listSavedFilters", "CRM", "Saved filters", nil, Items[store.SavedFilter]{}, 200, "entity_type"),
		op("POST", "/api/v1/saved-filters", "createSavedFilter", "CRM", "Save a filter", savedFilterInput{}, store.SavedFilter{}, 201),
		op("DELETE", "/api/v1/saved-filters/{id}", "deleteSavedFilter", "CRM", "Delete a saved filter", nil, nil, 204),
		op("GET", "/api/v1/search", "search", "CRM", "Search all records you can read", nil, searchDoc{}, 200, "q"),
		op("GET", "/api/v1/dashboard", "dashboard", "CRM", "Dashboard metrics", nil, dashboardDoc{}, 200),
		op("POST", "/api/v1/import/csv/preview", "csvImportPreview", "Data", "Preview a CSV import and suggested mapping", nil, csvPreviewDoc{}, 200).multipart("file", "entity_type"),
		op("POST", "/api/v1/import/csv/commit", "csvImportCommit", "Data", "Import a CSV (optionally as a dry run)", nil, importResult{}, 200).multipart("file", "entity_type", "mapping", "on_duplicate", "dry_run"),

		op("GET", "/api/v1/products/facets", "productFacets", "Products", "Source domains and categories", nil, facetsDoc{}, 200),
		op("POST", "/api/v1/products/{id}/review", "reviewProduct", "Products", "Record a review decision", reviewInput{}, store.Product{}, 200),
		op("GET", "/api/v1/products/{id}/duplicates", "productDuplicates", "Products", "Possible duplicate products", nil, Items[duplicateCandidate]{}, 200),
		op("POST", "/api/v1/products/{id}/merge", "mergeProducts", "Products", "Merge products into this one", mergeInput{}, productDetail{}, 200),
		op("POST", "/api/v1/products/{id}/variants", "createVariant", "Products", "Add a variant", variantInput{}, store.ProductVariant{}, 201),
		op("PUT", "/api/v1/products/{id}/variants/{variantID}", "updateVariant", "Products", "Update a variant", variantInput{}, store.ProductVariant{}, 200),
		op("DELETE", "/api/v1/products/{id}/variants/{variantID}", "deleteVariant", "Products", "Delete a variant", nil, nil, 204),
		op("POST", "/api/v1/products/{id}/images", "addImage", "Products", "Add an image by URL", imageInput{}, store.ProductImage{}, 201),
		op("POST", "/api/v1/products/{id}/images/{imageID}/primary", "setPrimaryImage", "Products", "Make an image primary", nil, Items[store.ProductImage]{}, 200),
		op("DELETE", "/api/v1/products/{id}/images/{imageID}", "deleteImage", "Products", "Remove an image", nil, nil, 204),
		op("GET", "/api/v1/products/{id}/suppliers", "listProductSuppliers", "Products", "Suppliers of a product", nil, Items[store.ListProductSuppliersRow]{}, 200),
		op("GET", "/api/v1/imports", "listImports", "Products", "Import queue", nil, Page[ImportListItem]{}, 200, "status", "source", "created_by", "device_id", "limit", "offset"),
		op("POST", "/api/v1/imports/products/check", "checkProductImport", "Products", "Validate a payload and find duplicates without saving", importRequest{}, checkResponse{}, 200),
		op("POST", "/api/v1/imports/products", "submitProductImport", "Products", "Save a reviewed capture as a draft product (202 when a duplicate needs a decision)", importRequest{}, importOutcome{}, 201),
		op("GET", "/api/v1/imports/{id}", "getImport", "Products", "One import with its payload", nil, store.ProductImport{}, 200),
		op("POST", "/api/v1/imports/{id}/resolve", "resolveImport", "Products", "Decide a parked duplicate", resolveInput{}, store.ProductImport{}, 200),
		op("GET", "/api/v1/captures", "listCaptures", "Products", "Captured pages", nil, Page[store.ListCapturedPagesRow]{}, 200, "q", "domain", "page_type", "device_id", "captured_by", "limit", "offset"),
		op("POST", "/api/v1/captures", "createCapture", "Products", "Store a page capture from the extension", captureInput{}, store.CapturedPage{}, 201),
		op("POST", "/api/v1/captures/purge", "purgeCaptures", "Products", "Delete captures older than a date", purgeInput{}, deletedDoc{}, 200),
		op("GET", "/api/v1/captures/{id}", "getCapture", "Products", "One capture", nil, captureDetailDoc{}, 200),
		op("GET", "/api/v1/captures/{id}/snapshot", "captureSnapshot", "Products", "Sanitized HTML snapshot as plain text", nil, nil, 200).produces("text/plain"),
		op("DELETE", "/api/v1/captures/{id}", "deleteCapture", "Products", "Delete a capture", nil, nil, 204),
		op("GET", "/api/v1/adapters", "listAdapters", "Products", "Source adapters", nil, Items[store.SourceAdapter]{}, 200),
		op("POST", "/api/v1/adapters", "createAdapter", "Products", "Create a selector adapter", adapterInput{}, store.SourceAdapter{}, 201),
		op("PUT", "/api/v1/adapters/{id}", "updateAdapter", "Products", "Update an adapter", adapterInput{}, store.SourceAdapter{}, 200),
		op("DELETE", "/api/v1/adapters/{id}", "deleteAdapter", "Products", "Delete a selector adapter", nil, nil, 204),

		op("GET", "/api/v1/analysis/analyzers", "listAnalyzers", "Analysis", "Analyzers, source kinds and report kinds", nil, analyzersDoc{}, 200),
		op("GET", "/api/v1/analysis/projects", "listProjects", "Analysis", "Analysis projects", nil, Page[projectView]{}, 200, "q", "workspace_id", "status", "limit", "offset"),
		op("POST", "/api/v1/analysis/projects", "createProject", "Analysis", "Create a project (authorization confirmation required)", projectInput{}, store.AnalysisProject{}, 201),
		op("GET", "/api/v1/analysis/projects/{id}", "getProject", "Analysis", "Project with finding counts", nil, projectDetailDoc{}, 200),
		op("PUT", "/api/v1/analysis/projects/{id}", "updateProject", "Analysis", "Update a project", projectInput{}, store.AnalysisProject{}, 200),
		op("DELETE", "/api/v1/analysis/projects/{id}", "deleteProject", "Analysis", "Delete a project and its data", nil, nil, 204),
		op("GET", "/api/v1/analysis/projects/{id}/sources", "listSources", "Analysis", "Sources", nil, Items[store.ListAnalysisSourcesRow]{}, 200),
		op("POST", "/api/v1/analysis/projects/{id}/sources", "createSource", "Analysis", "Add a URL, capture, text, API description or workflow", sourceInput{}, sourceCreatedDoc{}, 201),
		op("POST", "/api/v1/analysis/projects/{id}/sources/upload", "uploadSource", "Analysis", "Upload an archive, document, screenshot or API file", nil, sourceCreatedDoc{}, 201).multipart("file", "kind", "title", "auto_analyze"),
		op("GET", "/api/v1/analysis/sources/{sourceID}", "getSource", "Analysis", "A source with its extracted content", nil, sourceDetailDoc{}, 200),
		op("DELETE", "/api/v1/analysis/projects/{id}/sources/{sourceID}", "deleteSource", "Analysis", "Delete a source", nil, nil, 204),
		op("GET", "/api/v1/analysis/projects/{id}/runs", "listRuns", "Analysis", "Analysis runs", nil, Items[store.ListAnalysisRunsRow]{}, 200),
		op("POST", "/api/v1/analysis/projects/{id}/runs", "createRun", "Analysis", "Queue an analyzer run", runInput{}, store.AnalysisRun{}, 202),
		op("GET", "/api/v1/analysis/runs/{runID}", "getRun", "Analysis", "One run", nil, store.AnalysisRun{}, 200),
		op("POST", "/api/v1/analysis/runs/{runID}/cancel", "cancelRun", "Analysis", "Cancel a queued run", nil, nil, 204),
		op("GET", "/api/v1/analysis/projects/{id}/findings", "listFindings", "Analysis", "Findings", nil, Page[FindingListItem]{}, 200, "q", "category", "classification", "source_id", "origin", "limit", "offset"),
		op("GET", "/api/v1/analysis/projects/{id}/findings/summary", "findingSummary", "Analysis", "Finding counts by category and classification", nil, Items[store.FindingCategoryCountsRow]{}, 200),
		op("POST", "/api/v1/analysis/projects/{id}/findings", "createFinding", "Analysis", "Record a manual finding", findingInput{}, store.AnalysisFinding{}, 201),
		op("GET", "/api/v1/analysis/findings/{findingID}", "getFinding", "Analysis", "One finding", nil, store.AnalysisFinding{}, 200),
		op("PUT", "/api/v1/analysis/findings/{findingID}", "updateFinding", "Analysis", "Edit a finding", findingInput{}, store.AnalysisFinding{}, 200),
		op("DELETE", "/api/v1/analysis/findings/{findingID}", "deleteFinding", "Analysis", "Delete a finding", nil, nil, 204),
		op("GET", "/api/v1/analysis/projects/{id}/reports", "listReports", "Analysis", "Reports", nil, Items[store.ListReportsRow]{}, 200),
		op("POST", "/api/v1/analysis/projects/{id}/reports", "createReport", "Analysis", "Generate a deterministic report or save a manual one", reportInput{}, store.AnalysisReport{}, 201),
		op("GET", "/api/v1/analysis/reports/{reportID}", "getReport", "Analysis", "A report with the findings it cites", nil, reportDetailDoc{}, 200),
		op("GET", "/api/v1/analysis/reports/{reportID}/export", "exportReport", "Analysis", "Download a report as Markdown or JSON", nil, nil, 200, "format").produces("text/markdown,application/json"),
		op("DELETE", "/api/v1/analysis/reports/{reportID}", "deleteReport", "Analysis", "Delete a report", nil, nil, 204),

		op("GET", "/api/v1/ai/status", "aiStatus", "AI", "Whether AI is enabled and configured", nil, aiStatusDoc{}, 200),
		op("GET", "/api/v1/ai/tools", "aiTools", "AI", "AI tool catalogue", nil, aiToolsDoc{}, 200),
		op("GET", "/api/v1/ai/providers", "listAIProviders", "AI", "Configured providers (keys are never returned)", nil, Items[providerView]{}, 200),
		op("POST", "/api/v1/ai/providers", "createAIProvider", "AI", "Add a provider", providerInput{}, providerView{}, 201),
		op("PUT", "/api/v1/ai/providers/{id}", "updateAIProvider", "AI", "Update a provider", providerInput{}, providerView{}, 200),
		op("DELETE", "/api/v1/ai/providers/{id}", "deleteAIProvider", "AI", "Delete a provider", nil, nil, 204),
		op("POST", "/api/v1/ai/providers/{id}/test", "testAIProvider", "AI", "Send a fixed test prompt (no stored data)", nil, providerTestDoc{}, 200),
		op("GET", "/api/v1/ai/jobs", "listAIJobs", "AI", "AI jobs", nil, Page[store.ListAIJobsRow]{}, 200, "status", "tool", "entity_type", "entity_id", "mine", "limit", "offset"),
		op("POST", "/api/v1/ai/jobs", "createAIJob", "AI", "Queue an AI tool run (409 asks for confirmation before data leaves the server)", aiJobInput{}, store.AiJob{}, 202),
		op("GET", "/api/v1/ai/jobs/{id}", "getAIJob", "AI", "An AI job with output and grounding", nil, aiJobDetailDoc{}, 200),
		op("POST", "/api/v1/ai/jobs/{id}/cancel", "cancelAIJob", "AI", "Cancel a queued job", nil, nil, 204),
		op("POST", "/api/v1/ai/jobs/{id}/apply", "applyAIJob", "AI", "Apply reviewed AI output to the record", applyInput{}, aiApplyDoc{}, 200),

		op("GET", "/api/v1/catalog/products", "listCatalogProducts", "Catalog", "Published products in catalog version order (keyset pagination; cursor from next_cursor)", nil, CatalogProductPage{}, 200,
			"cursor", "limit", "updated_since", "tag", "category", "availability", "source_domain", "sku", "fields").etag(),
		op("GET", "/api/v1/catalog/products/by-sku/{sku}", "getCatalogProductBySKU", "Catalog", "A published product by product or variant SKU (409 when several match)", nil, catalog.CatalogProduct{}, 200, "fields").etag(),
		op("GET", "/api/v1/catalog/products/{id}", "getCatalogProduct", "Catalog", "One published product", nil, catalog.CatalogProduct{}, 200, "fields").etag(),
		op("GET", "/api/v1/catalog/changes", "listCatalogChanges", "Catalog", "Ordered change feed with tombstones (410 when the cursor is outside the retained feed)", nil, CatalogChangePage{}, 200, "since", "limit", "fields"),
		op("GET", "/api/v1/integrations/config", "integrationsConfig", "Catalog", "Scopes, event types, API base URL and webhook delivery policy", nil, integrationsConfigDoc{}, 200),
		op("GET", "/api/v1/integrations/keys", "listIntegrationKeys", "Catalog", "Integration keys with usage", nil, Items[IntegrationKeyView]{}, 200),
		op("POST", "/api/v1/integrations/keys", "createIntegrationKey", "Catalog", "Create an integration key (the token is returned only once)", createIntegrationKeyInput{}, integrationKeyCreatedDoc{}, 201),
		op("DELETE", "/api/v1/integrations/keys/{id}", "revokeIntegrationKey", "Catalog", "Revoke an integration key", nil, nil, 204),
		op("GET", "/api/v1/integrations/webhooks", "listWebhooks", "Catalog", "Webhook endpoints", nil, Items[WebhookView]{}, 200),
		op("POST", "/api/v1/integrations/webhooks", "createWebhook", "Catalog", "Register a webhook endpoint (the signing secret is returned only once)", webhookInput{}, webhookCreatedDoc{}, 201),
		op("GET", "/api/v1/integrations/webhooks/{id}", "getWebhook", "Catalog", "One webhook endpoint", nil, WebhookView{}, 200),
		op("PUT", "/api/v1/integrations/webhooks/{id}", "updateWebhook", "Catalog", "Update, enable or disable a webhook endpoint", webhookInput{}, WebhookView{}, 200),
		op("DELETE", "/api/v1/integrations/webhooks/{id}", "deleteWebhook", "Catalog", "Delete a webhook endpoint and its delivery log", nil, nil, 204),
		op("POST", "/api/v1/integrations/webhooks/{id}/rotate-secret", "rotateWebhookSecret", "Catalog", "Issue a new signing secret (the previous one also signs for 24 hours)", nil, webhookCreatedDoc{}, 200),
		op("POST", "/api/v1/integrations/webhooks/{id}/test", "testWebhook", "Catalog", "Send a signed webhook.test event now and return the result", nil, WebhookDeliveryDetail{}, 200),
		op("GET", "/api/v1/integrations/webhooks/{id}/deliveries", "listWebhookDeliveries", "Catalog", "Delivery log of an endpoint (newest first)", nil, Page[WebhookDeliveryView]{}, 200, "status", "limit", "offset"),
		op("GET", "/api/v1/integrations/deliveries/{id}", "getWebhookDelivery", "Catalog", "A delivery with its payload and attempts", nil, WebhookDeliveryDetail{}, 200),
		op("POST", "/api/v1/integrations/deliveries/{id}/redeliver", "redeliverWebhook", "Catalog", "Queue the same event again", nil, WebhookDeliveryDetail{}, 202),

		op("GET", "/api/v1/admin/export", "fullExport", "Data", "Download all data as JSON Lines in a ZIP", nil, nil, 200).produces("application/zip"),
		op("GET", "/api/v1/admin/backups", "listBackups", "Data", "Database backups", nil, backupListDoc{}, 200),
		op("POST", "/api/v1/admin/backups", "createBackup", "Data", "Create a pg_dump backup", nil, backupInfo{}, 201),
		op("GET", "/api/v1/admin/backups/{name}", "downloadBackup", "Data", "Download a backup", nil, nil, 200).produces("application/octet-stream"),
		op("DELETE", "/api/v1/admin/backups/{name}", "deleteBackup", "Data", "Delete a backup", nil, nil, 204),
	}
	ops = append(ops, crud("/api/v1/companies", "companies", "company", "CRM", companyView{}, Page[companyView]{}, companyInput{}, "company_type", "status", "owner_id", "country", "tag_id")...)
	ops = append(ops, crud("/api/v1/contacts", "contacts", "contact", "CRM", contactView{}, Page[contactView]{}, contactInput{}, "company_id", "supplier_id", "status", "owner_id", "decision_maker", "tag_id")...)
	ops = append(ops, crud("/api/v1/suppliers", "suppliers", "supplier", "CRM", supplierView{}, Page[supplierView]{}, supplierInput{}, "supplier_type", "company_id", "stage_id", "country", "min_rating", "owner_id", "tag_id")...)
	ops = append(ops, crud("/api/v1/leads", "leads", "lead", "CRM", leadView{}, Page[leadView]{}, leadInput{}, "stage_id", "pipeline_id", "source", "owner_id", "company_id", "tag_id")...)
	ops = append(ops, crud("/api/v1/opportunities", "opportunities", "opportunity", "CRM", opportunityView{}, Page[opportunityView]{}, opportunityInput{}, "stage_id", "pipeline_id", "status", "owner_id", "company_id", "supplier_id", "tag_id")...)
	ops = append(ops, crud("/api/v1/tasks", "tasks", "task", "CRM", taskView{}, Page[taskView]{}, taskInput{}, "status", "open_only", "assignee_id", "entity_type", "entity_id", "due_before", "task_type")...)
	ops = append(ops, crud("/api/v1/products", "products", "product", "Products", productDetail{}, Page[productView]{}, productInput{}, "status", "review_status", "supplier_id", "source_domain", "category", "availability", "tag_id")...)
	return ops
}

// ---- reflection to JSON Schema ----

type schemaGen struct {
	components map[string]any
	names      map[reflect.Type]string
	used       map[string]reflect.Type
}

var (
	timeType    = reflect.TypeOf(time.Time{})
	uuidType    = reflect.TypeOf(uuid.UUID{})
	decimalType = reflect.TypeOf(decimal.Decimal{})
	rawType     = reflect.TypeOf(json.RawMessage{})
	_           = analysis.ReportKinds
	_           = products.MaxImages
	_           = ai.Tools
)

var genericArgRe = regexp.MustCompile(`\[(.*)\]$`)

func exportName(t reflect.Type) string {
	name := t.Name()
	if m := genericArgRe.FindStringSubmatch(name); m != nil {
		base := name[:strings.Index(name, "[")]
		args := strings.Split(m[1], ",")
		var parts []string
		for _, a := range args {
			a = a[strings.LastIndex(a, "/")+1:]
			a = a[strings.LastIndex(a, ".")+1:]
			parts = append(parts, upperFirst(a))
		}
		return strings.Join(parts, "") + base
	}
	return upperFirst(name)
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func (g *schemaGen) nameFor(t reflect.Type) string {
	if n, ok := g.names[t]; ok {
		return n
	}
	n := exportName(t)
	if other, taken := g.used[n]; taken && other != t {
		pkg := t.PkgPath()[strings.LastIndex(t.PkgPath(), "/")+1:]
		n = upperFirst(pkg) + n
	}
	g.names[t] = n
	g.used[n] = t
	return n
}

func (g *schemaGen) schema(t reflect.Type) map[string]any {
	nullable := false
	for t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}
	s := g.base(t)
	if nullable {
		if _, isRef := s["$ref"]; isRef {
			return map[string]any{"allOf": []any{s}, "nullable": true}
		}
		s["nullable"] = true
	}
	return s
}

func (g *schemaGen) base(t reflect.Type) map[string]any {
	switch t {
	case timeType:
		return map[string]any{"type": "string", "format": "date-time"}
	case uuidType:
		return map[string]any{"type": "string", "format": "uuid"}
	case decimalType:
		return map[string]any{"type": "string", "format": "decimal", "example": "12.50"}
	case rawType:
		return map[string]any{"nullable": true}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return map[string]any{"type": "integer", "format": "int32"}
	case reflect.Int64, reflect.Uint64, reflect.Uint:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte"}
		}
		return map[string]any{"type": "array", "items": g.schema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		name := g.nameFor(t)
		ref := map[string]any{"$ref": "#/components/schemas/" + name}
		if _, done := g.components[name]; done {
			return ref
		}
		g.components[name] = map[string]any{} // placeholder for recursion
		props := map[string]any{}
		var required []string
		g.fields(t, props, &required)
		obj := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			sort.Strings(required)
			obj["required"] = required
		}
		g.components[name] = obj
		return ref
	}
	return map[string]any{}
}

func (g *schemaGen) fields(t reflect.Type, props map[string]any, required *[]string) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		ft := f.Type
		if f.Anonymous && name == "" {
			et := ft
			for et.Kind() == reflect.Pointer {
				et = et.Elem()
			}
			if et.Kind() == reflect.Struct {
				g.fields(et, props, required)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		props[name] = g.schema(ft)
		if !strings.Contains(opts, "omitempty") && !strings.Contains(opts, "omitzero") {
			*required = append(*required, name)
		}
	}
}

// BuildOpenAPI returns the OpenAPI 3.0 document for this server.
func BuildOpenAPI(version string) map[string]any {
	g := &schemaGen{components: map[string]any{}, names: map[reflect.Type]string{}, used: map[string]reflect.Type{}}
	errRef := g.schema(reflect.TypeOf(errorDoc{}))
	paths := map[string]any{}
	for _, o := range Operations() {
		p, _ := paths[o.Path].(map[string]any)
		if p == nil {
			p = map[string]any{}
			paths[o.Path] = p
		}
		params := []any{}
		for _, seg := range regexp.MustCompile(`\{(\w+)\}`).FindAllStringSubmatch(o.Path, -1) {
			typ := map[string]any{"type": "string"}
			params = append(params, map[string]any{"name": seg[1], "in": "path", "required": true, "schema": typ})
		}
		for _, q := range o.Query {
			params = append(params, map[string]any{"name": q, "in": "query", "required": false, "schema": map[string]any{"type": "string"}})
		}
		responses := map[string]any{"default": map[string]any{"description": "Error", "content": map[string]any{"application/json": map[string]any{"schema": errRef}}}}
		desc := http.StatusText(o.Status)
		switch {
		case o.Status == http.StatusNoContent:
			responses["204"] = map[string]any{"description": desc}
		case o.Produces != "":
			content := map[string]any{}
			for _, mt := range strings.Split(o.Produces, ",") {
				if mt == "application/json" {
					content[mt] = map[string]any{"schema": map[string]any{}}
				} else {
					content[mt] = map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}
				}
			}
			responses[itoaStatus(o.Status)] = map[string]any{"description": desc, "content": content}
		case o.Resp == nil:
			responses[itoaStatus(o.Status)] = map[string]any{"description": desc, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{}}}}
		default:
			responses[itoaStatus(o.Status)] = map[string]any{"description": desc, "content": map[string]any{"application/json": map[string]any{"schema": g.schema(reflect.TypeOf(o.Resp))}}}
		}
		if o.ID == "submitProductImport" {
			responses["202"] = map[string]any{"description": "A likely duplicate needs a decision", "content": map[string]any{"application/json": map[string]any{"schema": g.schema(reflect.TypeOf(importOutcome{}))}}}
		}
		if o.ID == "createAIJob" {
			responses["409"] = map[string]any{"description": "AI unavailable, or confirmation required before data leaves the server", "content": map[string]any{"application/json": map[string]any{"schema": g.schema(reflect.TypeOf(externalConfirmDoc{}))}}}
		}
		if o.ETag {
			params = append(params, map[string]any{"name": "If-None-Match", "in": "header", "required": false, "schema": map[string]any{"type": "string"},
				"description": "ETag of a previous response; 304 Not Modified when nothing changed"})
			responses["304"] = map[string]any{"description": "Not modified since the ETag sent in If-None-Match"}
		}
		operation := map[string]any{"operationId": o.ID, "tags": []string{o.Tag}, "summary": o.Summary, "parameters": params, "responses": responses}
		if o.Public {
			operation["security"] = []any{}
		}
		if len(o.Multipart) > 0 {
			fp := map[string]any{}
			for _, f := range o.Multipart {
				if f == "file" {
					fp[f] = map[string]any{"type": "string", "format": "binary"}
				} else {
					fp[f] = map[string]any{"type": "string"}
				}
			}
			operation["requestBody"] = map[string]any{"required": true, "content": map[string]any{"multipart/form-data": map[string]any{
				"schema": map[string]any{"type": "object", "properties": fp, "required": []string{"file"}}}}}
		} else if o.Req != nil {
			if _, isNoBody := o.Req.(noBody); !isNoBody {
				operation["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": g.schema(reflect.TypeOf(o.Req))}}}
			}
		}
		p[strings.ToLower(o.Method)] = operation
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{"title": "REA and Importify CRM API", "version": version,
			"description": "Self-hosted API for REA (Reverse Engineering Anything) and Importify CRM. Browser sessions use the session cookie plus the X-CSRF-Token header on unsafe methods; the browser extension and scripts use Bearer tokens; external systems use integration keys (reai_...) on the Catalog endpoints."},
		"servers":    []any{map[string]any{"url": "/"}},
		"security":   []any{map[string]any{"session": []any{}}, map[string]any{"bearer": []any{}}},
		"paths":      paths,
		"x-webhooks": webhookDoc(g),
		"components": map[string]any{
			"schemas": g.components,
			"securitySchemes": map[string]any{
				"session": map[string]any{"type": "apiKey", "in": "cookie", "name": "rea_session", "description": "__Host-rea_session when served over HTTPS"},
				"bearer":  map[string]any{"type": "http", "scheme": "bearer", "description": "Extension access tokens (reaa_...), personal API tokens (reap_...) and integration keys (reai_...)"},
			},
		},
	}
}

// webhookDoc describes the requests REA sends to registered webhook endpoints (OpenAPI 3.0 has no
// webhooks section, so it is an x- extension; the body schema is CatalogEvent).
func webhookDoc(g *schemaGen) map[string]any {
	header := func(name, desc string) map[string]any {
		return map[string]any{"name": name, "in": "header", "required": true, "description": desc, "schema": map[string]any{"type": "string"}}
	}
	types := []string{}
	for _, e := range catalog.EventTypes {
		types = append(types, e.Type)
	}
	types = append(types, catalog.EventTest)
	return map[string]any{"catalogEvent": map[string]any{"post": map[string]any{
		"summary": "Catalog event delivered to a registered endpoint",
		"description": "POSTed as JSON for every subscribed event (" + strings.Join(types, ", ") + "). Verify " + webhooks.HeaderSignature +
			": v1=hex(HMAC-SHA256(secret, timestamp + \".\" + raw body)); during a secret rotation two v1 values are sent. " +
			"Answer 2xx within the timeout; anything else is retried with exponential backoff. Deduplicate on the event id.",
		"parameters": []any{
			header(webhooks.HeaderEventID, "Event ID, identical across retries and redeliveries"),
			header(webhooks.HeaderEventType, "Event type"),
			header(webhooks.HeaderDeliveryID, "Delivery ID (a redelivery has a new one)"),
			header(webhooks.HeaderTimestamp, "Unix seconds when the request was signed; reject old timestamps"),
			header(webhooks.HeaderSignature, "Comma-separated v1=<hex HMAC-SHA256> signatures"),
		},
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{
			"schema": g.schema(reflect.TypeOf(catalog.CatalogEvent{}))}}},
		"responses": map[string]any{"200": map[string]any{"description": "Any 2xx status acknowledges the delivery"}},
	}}}
}

func itoaStatus(n int) string {
	return string([]byte{byte('0' + n/100), byte('0' + n/10%10), byte('0' + n%10)})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "public, max-age=300")
	return ok(w, BuildOpenAPI(s.cfg.Version))
}
