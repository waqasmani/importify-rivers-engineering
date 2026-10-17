// Groups duplicate candidates found for a page of records into connected sets, so that A↔B and
// B↔A (or A↔B, B↔C) show up once as a single group.
import type { Schemas } from '@/api/client'
import { contactName, type MergeEntity } from './crm'

type Rec = Record<string, unknown> & { id: string }

export interface DuplicateMember {
  id: string
  label: string
  detail: string
  createdAt: string
}

export interface DuplicateEvidence {
  a: string
  b: string
  score: number
  reasons: string[]
}

export interface DuplicateGroup {
  key: string
  members: DuplicateMember[]
  evidence: DuplicateEvidence[]
  score: number
}

function str(r: Rec, k: string): string {
  const v = r[k]
  return typeof v === 'string' ? v : ''
}

export function recordLabel(type: MergeEntity, r: Rec): string {
  if (type === 'contact') return contactName(r as { full_name?: string | null; first_name?: string; last_name?: string; email?: string | null })
  return str(r, 'name') || 'Untitled'
}

function member(type: MergeEntity, r: Rec): DuplicateMember {
  const detail = type === 'contact' ? [str(r, 'email'), str(r, 'job_title')] : [str(r, 'domain'), str(r, 'country')]
  return { id: r.id, label: recordLabel(type, r), detail: detail.filter(Boolean).join(' · '), createdAt: str(r, 'created_at') }
}

/** Query parameters for GET /<type>/duplicates that describe one record, or null if it has none. */
export function duplicateParams(type: MergeEntity, r: Rec): Record<string, string> | null {
  const p: Record<string, string> = { exclude_id: r.id }
  if (type === 'contact') {
    for (const k of ['first_name', 'last_name', 'email', 'company_id']) if (str(r, k)) p[k] = str(r, k)
    return p.first_name || p.last_name || p.email ? p : null
  }
  if (str(r, 'name')) p.name = str(r, 'name')
  if (str(r, 'domain')) p.domain = str(r, 'domain')
  return p.name || p.domain ? p : null
}

export function groupDuplicates(type: MergeEntity, scanned: { record: Rec; candidates: Schemas['DuplicateCandidate'][] }[]): DuplicateGroup[] {
  const parent = new Map<string, string>()
  const find = (x: string): string => {
    let p = parent.get(x) ?? x
    if (p !== x) {
      p = find(p)
      parent.set(x, p)
    }
    return p
  }
  const union = (a: string, b: string) => {
    const ra = find(a)
    const rb = find(b)
    if (ra !== rb) parent.set(ra, rb)
  }
  const members = new Map<string, DuplicateMember>()
  const evidence = new Map<string, DuplicateEvidence>()
  for (const { record, candidates } of scanned) {
    members.set(record.id, member(type, record))
    for (const c of candidates) {
      const rec = { ...((c.record ?? {}) as Rec), id: c.id }
      if (!members.has(c.id)) members.set(c.id, { ...member(type, rec), label: c.label || recordLabel(type, rec) })
      union(record.id, c.id)
      const pair = [record.id, c.id].sort().join(':')
      const prev = evidence.get(pair)
      const reasons = c.reasons ?? []
      if (!prev || c.score > prev.score) evidence.set(pair, { a: record.id, b: c.id, score: c.score, reasons })
    }
  }
  const groups = new Map<string, DuplicateGroup>()
  for (const [pair, ev] of evidence) {
    const root = find(pair.split(':')[0]!)
    let g = groups.get(root)
    if (!g) {
      g = { key: '', members: [], evidence: [], score: 0 }
      groups.set(root, g)
    }
    g.evidence.push(ev)
    g.score = Math.max(g.score, ev.score)
  }
  for (const [id, m] of members) {
    const g = groups.get(find(id))
    if (g) g.members.push(m)
  }
  const out = [...groups.values()].filter((g) => g.members.length > 1)
  for (const g of out) {
    g.members.sort((a, b) => a.createdAt.localeCompare(b.createdAt))
    g.key = g.members.map((m) => m.id).sort().join(',')
    g.evidence.sort((a, b) => b.score - a.score)
  }
  return out.sort((a, b) => b.score - a.score)
}
