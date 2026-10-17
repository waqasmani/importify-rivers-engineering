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
)

// AllPermissions lists every permission code.
var AllPermissions = []string{
	PermCRMRead, PermCRMWrite, PermCRMDelete, PermProductsRead, PermProductsWrite, PermProductsDelete,
	PermImportsReview, PermCapturesWrite, PermAnalysisRead, PermAnalysisWrite, PermAnalysisDelete, PermAIUse,
	PermAIConfigure, PermDataImport, PermDataExport, PermAuditRead, PermUsersManage, PermSettingsManage,
	PermBackupsManage, PermExtensionPair,
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
