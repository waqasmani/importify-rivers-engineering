// Client-side sanitizer for optional HTML snapshots. The snapshot is built from a detached copy
// of the page: scripts, frames, event handlers, form values, password and hidden inputs,
// token-like attributes and meta tags are removed before anything leaves the page. The server
// sanitizes again and stores the result as inert text.
import { isSensitiveName } from '@rea/extractor'
import type { SnapshotInfo } from '../shared/types'

const DROP = 'script, noscript, iframe, frame, frameset, object, embed, applet, template, portal, base, link[rel~="import"], link[rel~="modulepreload"], link[rel~="preload"][as="script"]'
const URL_ATTRS = new Set(['href', 'src', 'action', 'formaction', 'xlink:href', 'poster', 'background', 'data', 'ping', 'cite', 'longdesc', 'manifest'])
const FORM_VALUE_ATTRS = new Set(['value', 'checked', 'selected', 'placeholder', 'autocomplete', 'list'])

function dangerousUrl(v: string): boolean {
  const s = v.replace(/[\u0000- ]/g, '').toLowerCase()
  return s.startsWith('javascript:') || s.startsWith('vbscript:') || s.startsWith('data:text/html') || s.startsWith('data:application/')
}

/** Removes active and sensitive content from a detached element tree, in place. */
export function sanitizeTree(root: Element): void {
  for (const el of Array.from(root.querySelectorAll(DROP))) el.remove()
  for (const el of Array.from(root.querySelectorAll('input'))) {
    const type = (el.getAttribute('type') ?? '').trim().toLowerCase()
    if (type === 'password' || type === 'hidden') el.remove()
  }
  for (const el of Array.from(root.querySelectorAll('meta'))) {
    const name = el.getAttribute('name') ?? el.getAttribute('property') ?? el.getAttribute('itemprop') ?? ''
    const equiv = (el.getAttribute('http-equiv') ?? '').toLowerCase()
    if (isSensitiveName(name) || equiv === 'set-cookie' || equiv === 'refresh' || equiv === 'content-security-policy') el.remove()
  }
  // Comments can hide markup or tokens.
  const doc = root.ownerDocument
  const walker = doc.createTreeWalker(root, 128)
  const comments: Node[] = []
  for (let n = walker.nextNode(); n; n = walker.nextNode()) comments.push(n)
  for (const c of comments) c.parentNode?.removeChild(c)

  const all = [root, ...Array.from(root.querySelectorAll('*'))]
  for (const el of all) {
    const tag = el.tagName.toUpperCase()
    const isField = tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || tag === 'OPTION' || tag === 'BUTTON' || tag === 'OUTPUT'
    for (const attr of Array.from(el.attributes)) {
      const name = attr.name.toLowerCase()
      const value = attr.value
      const drop =
        name.startsWith('on') ||
        name === 'srcdoc' ||
        name === 'nonce' ||
        name === 'integrity' ||
        (name === 'style' && /expression\(|javascript:/i.test(value)) ||
        (isField && FORM_VALUE_ATTRS.has(name)) ||
        isSensitiveName(name) ||
        (name.startsWith('data-') && isSensitiveName(name.slice(5))) ||
        (URL_ATTRS.has(name) && dangerousUrl(value)) ||
        (name === 'srcset' && dangerousUrl(value)) ||
        (name === 'content' && tag === 'META' && dangerousUrl(value))
      if (drop) el.removeAttribute(attr.name)
    }
    if (tag === 'TEXTAREA' || tag === 'OUTPUT') el.textContent = ''
    if (tag === 'INPUT' && /^(email|tel|search|text|number|url|date|datetime-local|month|week|time|file|color|range)$/i.test(el.getAttribute('type') ?? 'text')) {
      el.removeAttribute('value')
    }
  }
}

function byteLength(s: string): number {
  return new TextEncoder().encode(s).length
}

function serialize(root: Element): string {
  return '<!doctype html>\n' + root.outerHTML
}

/**
 * Builds a sanitized snapshot of the document, or explains why none was included. Snapshots
 * larger than maxBytes are first slimmed (styles, SVG, inline styles) and otherwise dropped.
 */
export function buildSnapshot(doc: Document, enabled: boolean, maxBytes: number): SnapshotInfo {
  if (!enabled) return { included: false, bytes: 0, html: null, note: 'HTML snapshots are turned off.' }
  const clone = doc.documentElement.cloneNode(true) as Element
  sanitizeTree(clone)
  let html = serialize(clone)
  let bytes = byteLength(html)
  if (bytes > maxBytes) {
    for (const el of Array.from(clone.querySelectorAll('style, svg, link[rel~="stylesheet"], picture source'))) el.remove()
    for (const el of Array.from(clone.querySelectorAll('[style]'))) el.removeAttribute('style')
    html = serialize(clone)
    bytes = byteLength(html)
  }
  if (bytes > maxBytes) {
    return { included: false, bytes, html: null, note: `The sanitized snapshot (${Math.ceil(bytes / 1024)} KB) is larger than the ${Math.floor(maxBytes / 1024)} KB limit and was not included.` }
  }
  return { included: true, bytes, html, note: `Sanitized snapshot of ${Math.ceil(bytes / 1024)} KB (scripts, frames, event handlers and form values removed).` }
}
