import { createRouter, createWebHistory, type RouteComponent, type RouteRecordRaw } from 'vue-router'
import { useAuth } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    /** Route is reachable without a session. */
    public?: boolean
    /** Permission required to open the page. */
    perm?: string
    /** Any one of these permissions opens the page. */
    anyPerm?: string[]
    title?: string
  }
}

// Views are code-split per page. The glob makes Vite emit one chunk per view file.
const views = import.meta.glob<{ default: RouteComponent }>('../views/**/*.vue')
function v(path: string): () => Promise<{ default: RouteComponent }> {
  const loader = views[`../views/${path}.vue`]
  if (!loader) throw new Error(`Unknown view: ${path}`)
  return loader
}

const appRoutes: RouteRecordRaw[] = [
  { path: '', name: 'dashboard', component: v('DashboardView'), meta: { title: 'Dashboard' } },
  { path: 'search', name: 'search', component: v('SearchView'), meta: { title: 'Search' } },

  // CRM
  { path: 'companies', name: 'companies', component: v('crm/CompaniesView'), meta: { perm: 'crm.read', title: 'Companies' } },
  { path: 'companies/:id', name: 'company', component: v('crm/CompanyDetailView'), meta: { perm: 'crm.read', title: 'Company' }, props: true },
  { path: 'contacts', name: 'contacts', component: v('crm/ContactsView'), meta: { perm: 'crm.read', title: 'Contacts' } },
  { path: 'contacts/:id', name: 'contact', component: v('crm/ContactDetailView'), meta: { perm: 'crm.read', title: 'Contact' }, props: true },
  { path: 'suppliers', name: 'suppliers', component: v('crm/SuppliersView'), meta: { perm: 'crm.read', title: 'Suppliers' } },
  { path: 'suppliers/:id', name: 'supplier', component: v('crm/SupplierDetailView'), meta: { perm: 'crm.read', title: 'Supplier' }, props: true },
  { path: 'leads', name: 'leads', component: v('crm/LeadsView'), meta: { perm: 'crm.read', title: 'Leads' } },
  { path: 'leads/:id', name: 'lead', component: v('crm/LeadDetailView'), meta: { perm: 'crm.read', title: 'Lead' }, props: true },
  { path: 'opportunities', name: 'opportunities', component: v('crm/OpportunitiesView'), meta: { perm: 'crm.read', title: 'Opportunities' } },
  { path: 'opportunities/:id', name: 'opportunity', component: v('crm/OpportunityDetailView'), meta: { perm: 'crm.read', title: 'Opportunity' }, props: true },
  { path: 'pipelines', name: 'pipelines', component: v('crm/PipelineBoardView'), meta: { perm: 'crm.read', title: 'Pipelines' } },
  { path: 'tasks', name: 'tasks', component: v('crm/TasksView'), meta: { perm: 'crm.read', title: 'Tasks' } },
  { path: 'tasks/:id', name: 'task', component: v('crm/TaskDetailView'), meta: { perm: 'crm.read', title: 'Task' }, props: true },
  { path: 'duplicates', name: 'duplicates', component: v('crm/DuplicatesView'), meta: { perm: 'crm.write', title: 'Duplicates' } },

  // Products
  { path: 'imports', name: 'imports', component: v('products/ImportQueueView'), meta: { perm: 'products.read', title: 'Import queue' } },
  { path: 'imports/new', name: 'import-new', component: v('products/NewImportView'), meta: { perm: 'products.write', title: 'New import' } },
  { path: 'imports/:id', name: 'import', component: v('products/ImportDetailView'), meta: { perm: 'products.read', title: 'Import' }, props: true },
  { path: 'products', name: 'products', component: v('products/ProductsView'), meta: { perm: 'products.read', title: 'Products' } },
  { path: 'products/:id', name: 'product', component: v('products/ProductDetailView'), meta: { perm: 'products.read', title: 'Product' }, props: true },
  { path: 'captures', name: 'captures', component: v('products/CapturesView'), meta: { perm: 'products.read', title: 'Captured pages' } },
  { path: 'captures/:id', name: 'capture', component: v('products/CaptureDetailView'), meta: { perm: 'products.read', title: 'Captured page' }, props: true },

  // Analysis
  { path: 'analysis', name: 'analysis', component: v('analysis/ProjectsView'), meta: { perm: 'analysis.read', title: 'Analysis' } },
  { path: 'analysis/new', name: 'analysis-new', component: v('analysis/NewAnalysisView'), meta: { perm: 'analysis.write', title: 'New analysis' } },
  { path: 'analysis/sources/:id', name: 'analysis-source', component: v('analysis/SourceDetailView'), meta: { perm: 'analysis.read', title: 'Source' }, props: true },
  { path: 'analysis/reports/:id', name: 'analysis-report', component: v('analysis/ReportView'), meta: { perm: 'analysis.read', title: 'Report' }, props: true },
  { path: 'analysis/findings/:id', name: 'analysis-finding', component: v('analysis/FindingDetailView'), meta: { perm: 'analysis.read', title: 'Finding' }, props: true },
  { path: 'analysis/:id', name: 'analysis-project', component: v('analysis/ProjectDetailView'), meta: { perm: 'analysis.read', title: 'Analysis project' }, props: true },

  // AI
  { path: 'ai/jobs', name: 'ai-jobs', component: v('ai/AiJobsView'), meta: { perm: 'ai.use', title: 'AI jobs' } },
  { path: 'ai/jobs/:id', name: 'ai-job', component: v('ai/AiJobView'), meta: { perm: 'ai.use', title: 'AI job' }, props: true },

  // Settings and administration
  { path: 'profile', name: 'profile', component: v('settings/ProfileView'), meta: { title: 'Profile' } },
  { path: 'settings', name: 'settings', component: v('settings/GeneralSettingsView'), meta: { perm: 'settings.manage', title: 'Settings' } },
  { path: 'settings/pipelines', name: 'settings-pipelines', component: v('settings/PipelinesSettingsView'), meta: { perm: 'settings.manage', title: 'Pipelines' } },
  { path: 'settings/fields', name: 'settings-fields', component: v('settings/CustomFieldsView'), meta: { perm: 'settings.manage', title: 'Custom fields' } },
  { path: 'settings/tags', name: 'settings-tags', component: v('settings/TagsView'), meta: { perm: 'crm.write', title: 'Tags' } },
  { path: 'settings/adapters', name: 'settings-adapters', component: v('settings/AdaptersView'), meta: { perm: 'products.read', title: 'Source adapters' } },
  { path: 'settings/ai', name: 'settings-ai', component: v('settings/AiProvidersView'), meta: { perm: 'ai.configure', title: 'AI providers' } },
  { path: 'settings/integrations', name: 'settings-integrations', component: v('settings/IntegrationsView'), meta: { perm: 'settings.manage', title: 'Integrations' } },
  { path: 'settings/extension', name: 'settings-extension', component: v('settings/ExtensionView'), meta: { title: 'Browser extension' } },
  { path: 'settings/users', name: 'settings-users', component: v('settings/UsersView'), meta: { perm: 'users.manage', title: 'Users' } },
  { path: 'settings/roles', name: 'settings-roles', component: v('settings/RolesView'), meta: { perm: 'users.manage', title: 'Roles' } },
  { path: 'settings/workspaces', name: 'settings-workspaces', component: v('settings/WorkspacesView'), meta: { perm: 'settings.manage', title: 'Workspaces' } },
  { path: 'settings/audit', name: 'settings-audit', component: v('settings/AuditLogView'), meta: { perm: 'audit.read', title: 'Audit log' } },
  { path: 'settings/system', name: 'settings-system', component: v('settings/SystemView'), meta: { anyPerm: ['backups.manage', 'data.export', 'settings.manage'], title: 'System' } },

  { path: 'forbidden', name: 'forbidden', component: v('ForbiddenView'), meta: { title: 'Access denied' } },
]

const routes: RouteRecordRaw[] = [
  { path: '/login', name: 'login', component: v('auth/LoginView'), meta: { public: true, title: 'Sign in' } },
  { path: '/setup', name: 'setup', component: v('auth/SetupView'), meta: { public: true, title: 'First-run setup' } },
  { path: '/', component: () => import('../layouts/AppLayout.vue'), children: appRoutes },
  { path: '/:pathMatch(.*)*', name: 'not-found', component: v('NotFoundView'), meta: { public: true, title: 'Not found' } },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: (_to, _from, saved) => saved ?? { top: 0 },
})

router.beforeEach(async (to) => {
  const auth = useAuth()
  const signedIn = await auth.load()
  if (to.name === 'setup') {
    if (!auth.needsSetup) return signedIn ? { name: 'dashboard' } : { name: 'login' }
    return true
  }
  if (to.meta.public) {
    if (to.name === 'login' && auth.needsSetup) return { name: 'setup' }
    if (to.name === 'login' && signedIn) return typeof to.query.next === 'string' && to.query.next.startsWith('/') ? to.query.next : { name: 'dashboard' }
    return true
  }
  if (!signedIn) return auth.needsSetup ? { name: 'setup' } : { name: 'login', query: to.fullPath === '/' ? {} : { next: to.fullPath } }
  if (to.meta.perm && !auth.can(to.meta.perm)) return { name: 'forbidden', query: { from: to.fullPath } }
  if (to.meta.anyPerm && !to.meta.anyPerm.some((p) => auth.can(p))) return { name: 'forbidden', query: { from: to.fullPath } }
  return true
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} · REA Importify` : 'REA Importify'
})
