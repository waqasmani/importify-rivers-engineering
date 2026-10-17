// Record types that can own notes, tags, tasks, attachments and activity, with their routes and
// permissions. Labels for CRM concepts are configurable in settings; these are the defaults.
export type EntityType =
  | 'company'
  | 'contact'
  | 'supplier'
  | 'lead'
  | 'opportunity'
  | 'product'
  | 'task'
  | 'captured_page'
  | 'analysis_project'
  | 'analysis_source'
  | 'finding'

export interface EntityMeta {
  singular: string
  plural: string
  /** Route prefix of the detail page, without the id. */
  path?: string
  read: string
  write: string
  delete: string
  /** Whether the user can configure custom fields for this type. */
  customFields?: boolean
}

export const ENTITY: Record<EntityType, EntityMeta> = {
  company: { singular: 'Company', plural: 'Companies', path: '/companies', read: 'crm.read', write: 'crm.write', delete: 'crm.delete', customFields: true },
  contact: { singular: 'Contact', plural: 'Contacts', path: '/contacts', read: 'crm.read', write: 'crm.write', delete: 'crm.delete', customFields: true },
  supplier: { singular: 'Supplier', plural: 'Suppliers', path: '/suppliers', read: 'crm.read', write: 'crm.write', delete: 'crm.delete', customFields: true },
  lead: { singular: 'Lead', plural: 'Leads', path: '/leads', read: 'crm.read', write: 'crm.write', delete: 'crm.delete', customFields: true },
  opportunity: { singular: 'Opportunity', plural: 'Opportunities', path: '/opportunities', read: 'crm.read', write: 'crm.write', delete: 'crm.delete', customFields: true },
  product: { singular: 'Product', plural: 'Products', path: '/products', read: 'products.read', write: 'products.write', delete: 'products.delete', customFields: true },
  task: { singular: 'Task', plural: 'Tasks', path: '/tasks', read: 'crm.read', write: 'crm.write', delete: 'crm.delete' },
  captured_page: { singular: 'Captured page', plural: 'Captured pages', path: '/captures', read: 'products.read', write: 'captures.write', delete: 'products.delete' },
  analysis_project: { singular: 'Analysis project', plural: 'Analysis projects', path: '/analysis', read: 'analysis.read', write: 'analysis.write', delete: 'analysis.delete' },
  analysis_source: { singular: 'Source', plural: 'Sources', path: '/analysis/sources', read: 'analysis.read', write: 'analysis.write', delete: 'analysis.delete' },
  finding: { singular: 'Finding', plural: 'Findings', read: 'analysis.read', write: 'analysis.write', delete: 'analysis.delete' },
}

export function entityLink(type: string | null | undefined, id: string | null | undefined): string | undefined {
  if (!type || !id) return undefined
  const meta = ENTITY[type as EntityType]
  return meta?.path ? `${meta.path}/${id}` : undefined
}

export function entityLabel(type: string | null | undefined): string {
  if (!type) return ''
  return ENTITY[type as EntityType]?.singular ?? type.replace(/_/g, ' ')
}
