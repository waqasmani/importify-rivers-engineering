package auth

// Permission codes. They mirror the rows seeded by the foundation migration.
const (
	PermCRMRead        = "crm.read"
	PermCRMWrite       = "crm.write"
	PermCRMDelete      = "crm.delete"
	PermProductsRead   = "products.read"
	PermProductsWrite  = "products.write"
	PermProductsDelete = "products.delete"
	PermImportsReview  = "imports.review"
	PermCapturesWrite  = "captures.write"
	PermAnalysisRead   = "analysis.read"
	PermAnalysisWrite  = "analysis.write"
	PermAnalysisDelete = "analysis.delete"
	PermAIUse          = "ai.use"
	PermAIConfigure    = "ai.configure"
	PermDataImport     = "data.import"
	PermDataExport     = "data.export"
	PermAuditRead      = "audit.read"
	PermUsersManage    = "users.manage"
	PermSettingsManage = "settings.manage"
	PermBackupsManage  = "backups.manage"
	PermExtensionPair  = "extension.pair"
	// Orders carry customer personal data. Only orders.place authorizes confirming a supplier order
	// for placement (spending money), and only from a dashboard session.
	PermOrdersRead  = "orders.read"
	PermOrdersWrite = "orders.write"
	PermOrdersPlace = "orders.place"
)

// AllPermissions lists every permission code.
var AllPermissions = []string{
	PermCRMRead, PermCRMWrite, PermCRMDelete, PermProductsRead, PermProductsWrite, PermProductsDelete,
	PermImportsReview, PermCapturesWrite, PermAnalysisRead, PermAnalysisWrite, PermAnalysisDelete, PermAIUse,
	PermAIConfigure, PermDataImport, PermDataExport, PermAuditRead, PermUsersManage, PermSettingsManage,
	PermBackupsManage, PermExtensionPair, PermOrdersRead, PermOrdersWrite, PermOrdersPlace,
}

// ExtensionScopes is the maximum a browser extension token can do. The effective permissions of a
// token are the intersection of these scopes and the user's role permissions at request time, so
// downgrading a user immediately downgrades their extension.
var ExtensionScopes = []string{
	PermCRMRead, PermCRMWrite, PermProductsRead, PermProductsWrite, PermCapturesWrite,
	PermAnalysisRead, PermAnalysisWrite, PermAIUse,
}

// Integration scopes are granted to integration keys (organization-owned machine clients). They are
// not role permissions: a key's scopes are fixed when an administrator creates it and are never
// intersected with anyone's role. Users reach the catalog through products.read (see
// AddCatalogPermissions).
const (
	ScopeCatalogRead      = "catalog.read"
	ScopeCatalogReadCosts = "catalog.read_costs"
	ScopeImportsWrite     = "imports.write"
	// ScopeOrdersWrite lets the user's storefront or ERP submit and cancel customer orders and read
	// the orders submitted with the same key. It has the same code as the role permission.
	ScopeOrdersWrite = "orders.write"
	// ScopeSupplierOrdersReport lets the user's own automation read confirmed supplier orders of
	// suppliers that use automated placement and report placements, failures and shipments.
	ScopeSupplierOrdersReport = "supplier_orders.report"
	// No scope and no token type can confirm a supplier order: that needs orders.place in a
	// dashboard session.
)

// IntegrationScope describes a scope that can be granted to an integration key.
type IntegrationScope struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// IntegrationScopes lists every scope an integration key can hold.
var IntegrationScopes = []IntegrationScope{
	{ScopeCatalogRead, "Read published products (status active, review approved) and the change feed"},
	{ScopeCatalogReadCosts, "Also see cost, margin, supplier and source URL fields (requires catalog.read)"},
	{ScopeImportsWrite, "Submit products to the import queue for review; nothing is published without approval"},
	{ScopeOrdersWrite, "Submit and cancel customer orders and read the orders submitted with this key (includes customer names and addresses); supplier orders are only handed off after a person confirms them"},
	{ScopeSupplierOrdersReport, "For your purchasing automation: read confirmed supplier orders of suppliers with automated placement (with ship-to) and report placements, failures and shipments"},
}

// IsIntegrationScope reports whether code is a defined integration scope.
func IsIntegrationScope(code string) bool {
	for _, s := range IntegrationScopes {
		if s.Code == code {
			return true
		}
	}
	return false
}

// AddCatalogPermissions grants the catalog read scopes to users (sessions, personal and extension
// tokens) who can read products: they already see costs and suppliers in the dashboard.
func AddCatalogPermissions(perms map[string]bool) {
	if perms[PermProductsRead] {
		perms[ScopeCatalogRead] = true
		perms[ScopeCatalogReadCosts] = true
	}
}

// IsKnownPermission reports whether code is a defined permission.
func IsKnownPermission(code string) bool {
	for _, p := range AllPermissions {
		if p == code {
			return true
		}
	}
	return false
}
