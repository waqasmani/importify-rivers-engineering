// Fixed value lists that mirror server-side validation.
export const TASK_STATUSES = ['open', 'in_progress', 'done', 'cancelled'] as const
export const TASK_PRIORITIES = ['low', 'normal', 'high', 'urgent'] as const
export const TASK_TYPES = ['todo', 'follow_up', 'call', 'email', 'meeting', 'review'] as const
export const CUSTOM_FIELD_TYPES = ['text', 'number', 'date', 'boolean', 'select', 'url'] as const
export const CUSTOM_FIELD_ENTITIES = ['company', 'contact', 'supplier', 'lead', 'opportunity', 'product'] as const
export const CSV_ENTITIES = ['company', 'contact', 'supplier', 'product'] as const
