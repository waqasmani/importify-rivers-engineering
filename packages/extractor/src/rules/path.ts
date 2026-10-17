// A small, safe path language for reading values out of parsed JSON. Nothing is evaluated:
// a path is parsed into segments and walked over plain data.
//
//   a.b.c            object keys (letters, digits, "_", "$", "-")
//   a[0].b  a[-1]    array index (negative counts from the end)
//   a[*].b  a.*.b    every element of an array, or every value of an object
//   a["odd key"]     quoted key (single or double quotes)
//   ""               the value itself
//
// A path with a wildcard returns an array of every match; otherwise the single value or undefined.
import { isSensitiveName } from '../sensitive'

export type Segment = { t: 'key'; k: string } | { t: 'index'; i: number } | { t: 'wild' }

export const PATH_LIMITS = { length: 300, segments: 40, matches: 500 } as const

/** Keys that could reach object prototypes; never followed. */
const FORBIDDEN_KEYS = new Set(['__proto__', 'prototype', 'constructor'])

const IDENT = /^[A-Za-z0-9_$-]$/

export class PathError extends Error {}

export function parsePath(src: string): Segment[] {
  if (src.length > PATH_LIMITS.length) throw new PathError(`paths are limited to ${PATH_LIMITS.length} characters`)
  const segs: Segment[] = []
  let i = 0
  const n = src.length
  const readIdent = (): string => {
    const start = i
    while (i < n && IDENT.test(src[i]!)) i++
    return src.slice(start, i)
  }
  const pushKey = (k: string) => {
    if (FORBIDDEN_KEYS.has(k)) throw new PathError(`"${k}" cannot be used in a path`)
    if (isSensitiveName(k)) throw new PathError(`"${k}" looks like a credential or token name and cannot be read`)
    segs.push({ t: 'key', k })
  }
  let first = true
  while (i < n) {
    const c = src[i]!
    if (c === '[') {
      i++
      const q = src[i]
      if (q === '*') {
        i++
        segs.push({ t: 'wild' })
      } else if (q === '"' || q === "'") {
        i++
        let k = ''
        while (i < n && src[i] !== q) {
          if (src[i] === '\\' && i + 1 < n) i++
          k += src[i]
          i++
        }
        if (src[i] !== q) throw new PathError('unterminated quoted key')
        i++
        if (!k || k.length > 100) throw new PathError('quoted keys must have 1-100 characters')
        pushKey(k)
      } else {
        const start = i
        if (src[i] === '-') i++
        while (i < n && src[i]! >= '0' && src[i]! <= '9') i++
        const digits = src.slice(start, i)
        if (!/^-?\d{1,6}$/.test(digits)) throw new PathError('expected an index, "*" or a quoted key inside [ ]')
        segs.push({ t: 'index', i: parseInt(digits, 10) })
      }
      if (src[i] !== ']') throw new PathError('expected "]"')
      i++
    } else if (c === '.' && !first) {
      i++
      if (src[i] === '*') {
        i++
        segs.push({ t: 'wild' })
      } else {
        const k = readIdent()
        if (!k) throw new PathError(`expected a key after "." at position ${i}`)
        pushKey(k)
      }
    } else if (first) {
      if (c === '*') {
        i++
        segs.push({ t: 'wild' })
      } else {
        const k = readIdent()
        if (!k) throw new PathError(`unexpected character "${c}" at position ${i}`)
        pushKey(k)
      }
    } else {
      throw new PathError(`unexpected character "${c}" at position ${i}`)
    }
    first = false
    if (segs.length > PATH_LIMITS.segments) throw new PathError(`paths are limited to ${PATH_LIMITS.segments} segments`)
  }
  return segs
}

/** Returns an error message when the path is not valid, or null. */
export function pathError(src: string): string | null {
  try {
    parsePath(src)
    return null
  } catch (e) {
    return e instanceof Error ? e.message : String(e)
  }
}

function isObject(v: unknown): v is Record<string, unknown> {
  return !!v && typeof v === 'object' && !Array.isArray(v)
}

function own(o: Record<string, unknown>, k: string): unknown {
  return Object.prototype.hasOwnProperty.call(o, k) && !FORBIDDEN_KEYS.has(k) ? o[k] : undefined
}

const cache = new Map<string, Segment[]>()

function compiled(path: string): Segment[] {
  let segs = cache.get(path)
  if (!segs) {
    segs = parsePath(path)
    if (cache.size > 2000) cache.clear()
    cache.set(path, segs)
  }
  return segs
}

/** Context for paths evaluated against an entry of an object being iterated: "$key" is its key. */
export interface PathScope {
  key?: string
}

/** Walks a path over data. Throws PathError for an invalid path. */
export function evalPath(root: unknown, path: string, scope: PathScope = {}): unknown {
  if (path === '$key') return scope.key
  const segs = compiled(path)
  let cur: unknown[] = [root]
  let multi = false
  for (const seg of segs) {
    const next: unknown[] = []
    for (const v of cur) {
      if (seg.t === 'key') {
        if (isObject(v)) next.push(own(v, seg.k))
        else if (Array.isArray(v) && /^\d+$/.test(seg.k)) next.push(v[parseInt(seg.k, 10)])
      } else if (seg.t === 'index') {
        if (Array.isArray(v)) next.push(v[seg.i < 0 ? v.length + seg.i : seg.i])
      } else {
        if (Array.isArray(v)) next.push(...v.slice(0, PATH_LIMITS.matches))
        else if (isObject(v)) for (const k of Object.keys(v).slice(0, PATH_LIMITS.matches)) next.push(own(v, k))
      }
      if (next.length >= PATH_LIMITS.matches) break
    }
    if (seg.t === 'wild') multi = true
    cur = next.filter((x) => x !== undefined).slice(0, PATH_LIMITS.matches)
  }
  return multi ? cur : cur[0]
}

/** Iterates an array, or the entries of an object (exposing each key as "$key"). */
export function entriesOf(v: unknown, limit: number): { value: unknown; key?: string }[] {
  if (Array.isArray(v)) return v.slice(0, limit).map((value) => ({ value }))
  if (isObject(v)) {
    return Object.keys(v)
      .filter((k) => !FORBIDDEN_KEYS.has(k))
      .slice(0, limit)
      .map((key) => ({ key, value: v[key] }))
  }
  return []
}
