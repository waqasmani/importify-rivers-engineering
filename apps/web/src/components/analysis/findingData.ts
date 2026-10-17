// Safe readers for the analyzer payload stored in a finding's `data` object. Shapes come from the
// analyzers in services/internal/analysis; anything unexpected reads as empty.
import type { Schemas } from '@/api/client'

export type FindingRow = Schemas['FindingListItem']

export function dataOf(f: { data?: unknown }): Record<string, unknown> {
  return f.data && typeof f.data === 'object' && !Array.isArray(f.data) ? (f.data as Record<string, unknown>) : {}
}
export function str(v: unknown): string {
  return typeof v === 'string' ? v : typeof v === 'number' || typeof v === 'boolean' ? String(v) : ''
}
export function num(v: unknown): number {
  return typeof v === 'number' ? v : Number(v) || 0
}
export function arr(v: unknown): unknown[] {
  return Array.isArray(v) ? v : []
}
export function obj(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {}
}
export function strings(v: unknown): string[] {
  return arr(v).map(str).filter(Boolean)
}
export function byCategory(findings: FindingRow[], ...cats: string[]): FindingRow[] {
  return findings.filter((f) => cats.includes(f.category))
}
export function pretty(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}
