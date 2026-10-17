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

// IsKnownPermission reports whether code is a defined permission.
func IsKnownPermission(code string) bool {
	for _, p := range AllPermissions {
		if p == code {
			return true
		}
	}
	return false
}
