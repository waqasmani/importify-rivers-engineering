// DOM helpers for the content script. They read visible text and public attributes only:
// form field values, password and hidden inputs, scripts and cookies are never read.
import { isSensitiveName, scrubUrl } from '../shared/sensitive'

const SKIP_TEXT = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'TEXTAREA', 'SELECT', 'OPTION', 'INPUT', 'OBJECT', 'EMBED', 'IFRAME', 'SVG', 'CANVAS'])
const BLOCK = new Set(['P', 'DIV', 'LI', 'BR', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'TR', 'SECTION', 'ARTICLE', 'TD', 'TH', 'DT', 'DD', 'UL', 'OL', 'TABLE', 'HEADER', 'FOOTER', 'BLOCKQUOTE', 'PRE'])

export function collapse(s: string): string {
  return s.replace(/\s+/g, ' ').trim()
}

export function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n) : s
}

export function isFormControl(el: Element): boolean {
  const t = el.tagName.toUpperCase()
  return t === 'INPUT' || t === 'TEXTAREA' || t === 'SELECT' || t === 'OPTION'
}

function walkText(node: Node, out: string[], multiline: boolean, budget: { left: number }): void {
  if (budget.left <= 0) return
  if (node.nodeType === 3) {
    const v = node.nodeValue ?? ''
    out.push(v)
    budget.left -= v.length
    return
  }
  if (node.nodeType !== 1) return
  const el = node as Element
  const tag = el.tagName.toUpperCase()
  if (SKIP_TEXT.has(tag) || el.hasAttribute('hidden')) return
  for (let c = el.firstChild; c; c = c.nextSibling) walkText(c, out, multiline, budget)
  if (multiline && BLOCK.has(tag)) out.push('\n')
  else if (tag === 'TD' || tag === 'TH' || tag === 'LI' || tag === 'BR') out.push(' ')
}

/** Visible text of an element with whitespace collapsed. Form controls contribute nothing. */
export function text(el: Element | null | undefined, max = 20000): string {
  if (!el) return ''
  const out: string[] = []
  walkText(el, out, false, { left: max * 2 })
  return truncate(collapse(out.join('')), max)
}

/** Visible text keeping paragraph breaks. */
export function multilineText(el: Element | null | undefined, max = 50000): string {
  if (!el) return ''
  const out: string[] = []
  walkText(el, out, true, { left: max * 2 })
  return truncate(
    out
      .join('')
      .split('\n')
      .map((l) => collapse(l))
      .filter(Boolean)
      .join('\n'),
    max,
  )
}

/** Plain text from an HTML fragment (for example a Shopify description) without running it. */
export function htmlToText(doc: Document, html: string, max = 50000): string {
  const parsed = new (doc.defaultView?.DOMParser ?? DOMParser)().parseFromString(`<body>${html}</body>`, 'text/html')
  return multilineText(parsed.body, max)
}

export function baseUrl(doc: Document, pageUrl: string): string {
  const href = doc.querySelector('base[href]')?.getAttribute('href')
  if (href) {
    try {
      return new URL(href, pageUrl).toString()
    } catch {
      return pageUrl
    }
  }
  return pageUrl
}

/** Resolves a possibly relative reference to an absolute http(s) URL, or ''. */
export function absUrl(ref: string | null | undefined, base: string): string {
  const r = (ref ?? '').trim()
  if (!r || r.startsWith('data:') || r.startsWith('blob:') || /^javascript:/i.test(r)) return ''
  try {
    const u = new URL(r.startsWith('//') ? 'https:' + r : r, base)
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return ''
    return scrubUrl(u.toString())
  } catch {
    return ''
  }
}

/** Content of a meta tag by name or property (case-insensitive). Sensitive names are never read. */
export function meta(doc: Document, key: string): string {
  if (isSensitiveName(key)) return ''
  const k = key.toLowerCase()
  for (const m of Array.from(doc.querySelectorAll('meta'))) {
    const n = (m.getAttribute('property') ?? m.getAttribute('name') ?? m.getAttribute('itemprop') ?? '').toLowerCase()
    if (n === k) return collapse(m.getAttribute('content') ?? '')
  }
  return ''
}

/** All values for a repeated meta tag (for example og:image). */
export function metaAll(doc: Document, key: string): string[] {
  const k = key.toLowerCase()
  const out: string[] = []
  for (const m of Array.from(doc.querySelectorAll('meta'))) {
    const n = (m.getAttribute('property') ?? m.getAttribute('name') ?? '').toLowerCase()
    if (n === k) {
      const v = collapse(m.getAttribute('content') ?? '')
      if (v) out.push(v)
    }
  }
  return out
}

/** Collects non-sensitive meta tags as name -> content. */
export function publicMeta(doc: Document, limit = 80): Record<string, string> {
  const out: Record<string, string> = {}
  let n = 0
  for (const m of Array.from(doc.querySelectorAll('meta'))) {
    const name = (m.getAttribute('property') ?? m.getAttribute('name') ?? m.getAttribute('itemprop') ?? '').trim()
    const equiv = m.getAttribute('http-equiv')
    if (!name || equiv || isSensitiveName(name)) continue
    const content = collapse(m.getAttribute('content') ?? '')
    if (!content || /^(viewport|theme-color|format-detection|referrer|robots|google-site-verification|msvalidate\.01|facebook-domain-verification)$/i.test(name)) continue
    if (out[name] === undefined) {
      out[name] = truncate(content, 1000)
      if (++n >= limit) break
    }
  }
  return out
}

function cssEscape(s: string): string {
  return s.replace(/[^a-zA-Z0-9_-]/g, (c) => '\\' + c)
}

/** A reasonably stable CSS selector for an element. */
export function cssPath(el: Element): string {
  const parts: string[] = []
  let cur: Element | null = el
  let depth = 0
  while (cur && cur.nodeType === 1 && depth < 6) {
    const tag = cur.tagName.toLowerCase()
    if (tag === 'html' || tag === 'body') {
      parts.unshift(tag)
      break
    }
    const id = cur.getAttribute('id')
    if (id && /^[A-Za-z][\w-]{0,60}$/.test(id) && !isSensitiveName(id)) {
      parts.unshift(`#${cssEscape(id)}`)
      break
    }
    let part = tag
    const classes = Array.from(cur.classList).filter((c) => /^[A-Za-z][\w-]{0,40}$/.test(c)).slice(0, 2)
    if (classes.length) part += classes.map((c) => '.' + cssEscape(c)).join('')
    const parent: Element | null = cur.parentElement
    if (parent) {
      const same = Array.from(parent.children).filter((c) => c.tagName === cur!.tagName)
      if (same.length > 1) part += `:nth-of-type(${same.indexOf(cur) + 1})`
    }
    parts.unshift(part)
    cur = parent
    depth++
  }
  return truncate(parts.join(' > '), 1000)
}

/** Rows of a table, definition list or two-column attribute grid, as [label, value] pairs. */
export function tableRows(el: Element, maxRows = 200): string[][] {
  const rows: string[][] = []
  const tag = el.tagName.toUpperCase()
  if (tag === 'TABLE' || el.querySelector('tr')) {
    for (const tr of Array.from(el.querySelectorAll('tr')).slice(0, maxRows)) {
      const cells = Array.from(tr.children).filter((c) => c.tagName === 'TH' || c.tagName === 'TD').map((c) => text(c, 2000))
      if (cells.some(Boolean)) rows.push(cells.slice(0, 50))
    }
    return rows
  }
  if (tag === 'DL' || el.querySelector('dt')) {
    const dts = Array.from(el.querySelectorAll('dt')).slice(0, maxRows)
    for (const dt of dts) {
      let dd = dt.nextElementSibling
      while (dd && dd.tagName !== 'DD' && dd.tagName !== 'DT') dd = dd.nextElementSibling
      rows.push([text(dt, 2000), dd && dd.tagName === 'DD' ? text(dd, 2000) : ''])
    }
    return rows
  }
  for (const child of Array.from(el.children).slice(0, maxRows)) {
    const cells = Array.from(child.children).map((c) => text(c, 2000)).filter(Boolean)
    if (cells.length >= 2) rows.push(cells.slice(0, 50))
  }
  return rows
}

/** Turns [label, value] rows into a specification map. */
export function rowsToSpecs(rows: string[][]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const r of rows) {
    const k = collapse((r[0] ?? '').replace(/:\s*$/, ''))
    const v = collapse(r.slice(1).join(' '))
    if (k && v && k.length <= 120 && out[k] === undefined && !isSensitiveName(k)) out[k] = truncate(v, 2000)
    if (Object.keys(out).length >= 200) break
  }
  return out
}

/** Image source of an <img>, preferring the largest candidate and lazy-loading attributes. */
export function imageSrc(img: Element, base: string): string {
  const candidates = [
    img.getAttribute('data-large_image'),
    img.getAttribute('data-zoom-image'),
    img.getAttribute('data-src'),
    img.getAttribute('data-original'),
    img.getAttribute('src'),
  ]
  const srcset = img.getAttribute('srcset') ?? img.getAttribute('data-srcset')
  if (srcset) {
    const best = srcset
      .split(',')
      .map((p) => p.trim().split(/\s+/))
      .map(([u, d]) => ({ u: u ?? '', w: parseFloat(d ?? '1') || 1 }))
      .sort((a, b) => b.w - a.w)[0]
    if (best?.u) candidates.splice(2, 0, best.u)
  }
  for (const c of candidates) {
    const u = absUrl(c, base)
    if (u) return u
  }
  return ''
}

export function hostOf(url: string): string {
  try {
    return new URL(url).hostname.toLowerCase().replace(/^www\./, '')
  } catch {
    return ''
  }
}

export function firstString(v: unknown): string {
  if (typeof v === 'string') return collapse(v)
  if (typeof v === 'number') return String(v)
  if (Array.isArray(v)) {
    for (const x of v) {
      const s = firstString(x)
      if (s) return s
    }
  }
  if (v && typeof v === 'object') {
    const o = v as Record<string, unknown>
    return firstString(o['name'] ?? o['@value'] ?? o['value'] ?? '')
  }
  return ''
}
