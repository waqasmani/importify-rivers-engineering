package httpapi

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/ai"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// Named response shapes. Handlers that build JSON maps return exactly these fields; the contract
// tests validate every response against the generated OpenAPI document.

// SessionView is an active sign-in session.
type SessionView struct {
	ID         uuid.UUID `json:"id"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

// DirectoryUser is the minimal user record shown in pickers.
type DirectoryUser struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
}

// SearchHit is one global search result.
type SearchHit struct {
	EntityType string    `json:"entity_type"`
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	Subtitle   string    `json:"subtitle"`
}

// ImportListItem is a product import in the queue (payload omitted).
type ImportListItem struct {
	store.ProductImport
	ProductTitle  *string `json:"product_title,omitempty"`
	CreatedByName *string `json:"created_by_name,omitempty"`
}

// FindingListItem is a finding with its source.
type FindingListItem struct {
	store.AnalysisFinding
	SourceTitle *string `json:"source_title"`
	SourceKind  *string `json:"source_kind"`
}

// Items wraps a non-paginated list.
type Items[T any] struct {
	Items []T `json:"items"`
}

type errorDoc struct {
	Error APIError `json:"error"`
}

type meDoc struct {
	User         UserView   `json:"user"`
	Permissions  []string   `json:"permissions"`
	AuthMethod   string     `json:"auth_method"`
	Organization string     `json:"organization"`
	CSRFToken    string     `json:"csrf_token,omitempty"`
	DeviceID     *uuid.UUID `json:"device_id,omitempty"`
}

type statusDoc struct {
	Status        string `json:"status"`
	SchemaVersion int64  `json:"schema_version,omitempty"`
}

type setupStatusDoc struct {
	NeedsSetup     bool   `json:"needs_setup"`
	SetupTokenHint string `json:"setup_token_hint"`
}

type tokenCreatedDoc struct {
	Token      string    `json:"token"`
	Credential tokenView `json:"credential"`
}

type systemInfoDoc struct {
	Version                string   `json:"version"`
	SchemaVersion          int64    `json:"schema_version"`
	PublicURL              string   `json:"public_url"`
	WorkerMode             string   `json:"worker_mode"`
	ServerFetch            bool     `json:"server_fetch"`
	RepositoryPathsEnabled bool     `json:"repository_paths_enabled"`
	AIProviders            int64    `json:"ai_providers"`
	MaxUploadMB            int64    `json:"max_upload_mb"`
	ExtensionOrigins       []string `json:"extension_origins"`
	Telemetry              string   `json:"telemetry"`
}

type settingsDoc struct {
	Organization OrganizationSettings   `json:"organization"`
	EntityLabels map[string]EntityLabel `json:"entity_labels"`
	Capture      CaptureSettings        `json:"capture"`
	Imports      ImportSettings         `json:"imports"`
	AI           AISettings             `json:"ai"`
}

type auditPageDoc struct {
	Items        []store.AuditLog `json:"items"`
	NextBeforeID *int64           `json:"next_before_id"`
}

type pairingCodeDoc struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	ServerURL string    `json:"server_url"`
}

type extensionConfigDoc struct {
	User          UserView              `json:"user"`
	Permissions   []string              `json:"permissions"`
	DeviceID      *uuid.UUID            `json:"device_id"`
	ServerVersion string                `json:"server_version"`
	Organization  string                `json:"organization"`
	Adapters      []store.SourceAdapter `json:"adapters"`
	Capture       CaptureSettings       `json:"capture"`
	Imports       ImportSettings        `json:"imports"`
	AIEnabled     bool                  `json:"ai_enabled"`
}

type affectedDoc struct {
	Affected int64 `json:"affected"`
}

type deletedDoc struct {
	Deleted int64 `json:"deleted"`
}

type convertLeadDoc struct {
	Lead        store.Lead        `json:"lead"`
	Opportunity store.Opportunity `json:"opportunity"`
}

type pipelineSummaryDoc struct {
	Pipeline store.Pipeline             `json:"pipeline"`
	Stages   []store.PipelineSummaryRow `json:"stages"`
}

type searchDoc struct {
	Items []SearchHit `json:"items"`
	Query string      `json:"query"`
}

type dashboardDoc struct {
	Counts         *store.DashboardCountsRow `json:"counts,omitempty"`
	ImportsPerDay  []store.ImportsPerDayRow  `json:"imports_per_day,omitempty"`
	MyTasks        []taskView                `json:"my_tasks,omitempty"`
	Pipeline       *pipelineSummaryDoc       `json:"pipeline,omitempty"`
	RecentActivity []store.AuditLog          `json:"recent_activity"`
}

type csvPreviewDoc struct {
	EntityType string            `json:"entity_type"`
	Headers    []string          `json:"headers"`
	Mapping    map[string]string `json:"mapping"`
	Fields     []importField     `json:"fields"`
	SampleRows [][]string        `json:"sample_rows"`
	TotalRows  int               `json:"total_rows"`
}

type facetsDoc struct {
	Domains    []store.ProductDomainsRow    `json:"domains"`
	Categories []store.ProductCategoriesRow `json:"categories"`
}

type captureDetailDoc struct {
	Capture     store.CapturedPage `json:"capture"`
	HasSnapshot bool               `json:"has_snapshot"`
}

type analyzersDoc struct {
	Analyzers       []string `json:"analyzers"`
	ReportKinds     []string `json:"report_kinds"`
	SourceKinds     []string `json:"source_kinds"`
	ServerFetch     bool     `json:"server_fetch"`
	RepositoryPaths bool     `json:"repository_paths"`
}

type projectDetailDoc struct {
	Project       store.AnalysisProject            `json:"project"`
	Workspace     store.Workspace                  `json:"workspace"`
	FindingCounts []store.FindingCategoryCountsRow `json:"finding_counts"`
	Tags          []tagRef                         `json:"tags"`
}

type sourceSummaryDoc struct {
	ID             uuid.UUID       `json:"id"`
	ProjectID      uuid.UUID       `json:"project_id"`
	Kind           string          `json:"kind"`
	Title          string          `json:"title"`
	Url            *string         `json:"url"`
	CapturedPageID *uuid.UUID      `json:"captured_page_id"`
	AttachmentID   *uuid.UUID      `json:"attachment_id"`
	ContentSha256  string          `json:"content_sha256"`
	MimeType       string          `json:"mime_type"`
	SizeBytes      int64           `json:"size_bytes"`
	Metadata       json.RawMessage `json:"metadata"`
	Status         string          `json:"status"`
	Error          string          `json:"error"`
	CreatedBy      *uuid.UUID      `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	ContentLength  int             `json:"content_length"`
}

type sourceDetailDoc struct {
	sourceSummaryDoc
	ContentText      string `json:"content_text"`
	ContentTruncated bool   `json:"content_truncated,omitempty"`
}

type sourceCreatedDoc struct {
	Source sourceSummaryDoc   `json:"source"`
	Run    *store.AnalysisRun `json:"run"`
}

type reportDetailDoc struct {
	Report   store.AnalysisReport    `json:"report"`
	Findings []store.AnalysisFinding `json:"findings"`
}

type aiStatusDoc struct {
	Enabled         bool                  `json:"enabled"`
	Configured      bool                  `json:"configured"`
	DefaultProvider *aiDefaultProviderDoc `json:"default_provider"`
	Message         string                `json:"message,omitempty"`
}

type aiDefaultProviderDoc struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Kind              string    `json:"kind"`
	Model             string    `json:"model"`
	IsLocal           bool      `json:"is_local"`
	AllowExternalData bool      `json:"allow_external_data"`
	SupportsVision    bool      `json:"supports_vision"`
}

type aiToolsDoc struct {
	Items           []ai.Tool           `json:"items"`
	SuggestedModels map[string][]string `json:"suggested_models"`
}

type providerTestDoc struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	Model     string `json:"model,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
}

type aiJobDetailDoc struct {
	Job               store.AiJob `json:"job"`
	Tool              ai.Tool     `json:"tool"`
	AIGeneratedNotice string      `json:"ai_generated_notice"`
}

type aiApplyDoc struct {
	Product         *store.Product        `json:"product,omitempty"`
	Tags            []string              `json:"tags,omitempty"`
	Note            *store.Note           `json:"note,omitempty"`
	FindingsCreated *int                  `json:"findings_created,omitempty"`
	Report          *store.AnalysisReport `json:"report,omitempty"`
	Task            *store.Task           `json:"task,omitempty"`
}

type backupListDoc struct {
	Items           []backupInfo `json:"items"`
	PgDumpAvailable bool         `json:"pg_dump_available"`
	Directory       string       `json:"directory"`
	RestoreHint     string       `json:"restore_hint"`
}

type externalConfirmDoc struct {
	Error          APIError          `json:"error"`
	Provider       map[string]string `json:"provider,omitempty"`
	DataCategories []string          `json:"data_categories,omitempty"`
}
