// Reading JSON that pages embed in inline scripts, without running any of it.
//
// - scanBalanced: a string-aware brace scanner that finds where the object or array starting at
//   a position ends (quotes, escapes and comments are honoured, so braces inside strings do not
//   count).
// - decodeJsString: decodes a JavaScript string literal ("..." or '...'), for data stored as
//   JSON.parse("{\"a\":1}").
// - parseLoose: a recursive-descent parser for JSON plus the JavaScript object-literal forms
//   pages commonly use (unquoted or single-quoted keys, single-quoted strings, trailing commas,
//   undefined, !0/!1, comments). Anything else (functions, calls, expressions) is rejected.
// - valueAfterMarker: finds a literal marker such as "window.runParams" in script text and reads
//   the value assigned to it.
//
// Every function is bounded: inputs above MAX_SCRIPT_BYTES are refused and nesting deeper than
// MAX_DEPTH is an error. Markers are plain strings, never regular expressions.

export const MAX_SCRIPT_BYTES = 2_000_000
export const MAX_DEPTH = 64
const MAX_NODES = 400_000

export class JsonScanError extends Error {}

function isSpace(c: string | undefined): boolean {
  return c === ' ' || c === '\n' || c === '\r' || c === '\t' || c === '\f' || c === '\v' || c === ' ' || c === '﻿' || c === ' ' || c === ' '
}

/**
 * Returns the index just after the object or array that starts at `start`. Throws
 * JsonScanError when it is unbalanced, nested deeper than MAX_DEPTH or longer than maxBytes.
 */
export function scanBalanced(text: string, start: number, maxBytes = MAX_SCRIPT_BYTES): number {
  const open = text[start]
  if (open !== '{' && open !== '[') throw new JsonScanError('expected "{" or "["')
  const stack: string[] = []
  const limit = Math.min(text.length, start + maxBytes)
  let i = start
  while (i < limit) {
    const c = text[i]!
    if (c === '"' || c === "'" || c === '`') {
      i++
      while (i < limit && text[i] !== c) {
        if (text[i] === '\\') i++
        i++
      }
      if (i >= limit) break
      i++
      continue
    }
    if (c === '/' && text[i + 1] === '/') {
      while (i < limit && text[i] !== '\n') i++
      continue
    }
    if (c === '/' && text[i + 1] === '*') {
      const end = text.indexOf('*/', i + 2)
      if (end < 0 || end >= limit) break
      i = end + 2
      continue
    }
    if (c === '{' || c === '[') {
      stack.push(c === '{' ? '}' : ']')
      if (stack.length > MAX_DEPTH) throw new JsonScanError(`the data is nested deeper than ${MAX_DEPTH} levels`)
    } else if (c === '}' || c === ']') {
      if (stack.pop() !== c) throw new JsonScanError('mismatched brackets')
      if (stack.length === 0) return i + 1
    }
    i++
  }
  if (i >= start + maxBytes) throw new JsonScanError(`the data is larger than ${Math.round(maxBytes / 1000)} kB`)
  throw new JsonScanError('the data is not closed')
}

/** Decodes the JavaScript string literal starting at `start`. */
export function decodeJsString(text: string, start: number, maxBytes = MAX_SCRIPT_BYTES): { value: string; end: number } {
  const q = text[start]
  if (q !== '"' && q !== "'" && q !== '`') throw new JsonScanError('expected a string')
  const limit = Math.min(text.length, start + maxBytes)
  let out = ''
  let i = start + 1
  while (i < limit) {
    const c = text[i]!
    if (c === q) return { value: out, end: i + 1 }
    if (c === '\\') {
      const e = text[i + 1]
      i += 2
      switch (e) {
        case 'n': out += '\n'; break
        case 't': out += '\t'; break
        case 'r': out += '\r'; break
        case 'b': out += '\b'; break
        case 'f': out += '\f'; break
        case 'v': out += '\v'; break
        case '0': out += '\0'; break
        case '\r': if (text[i] === '\n') i++; break
        case '\n': case ' ': case ' ': break
        case 'x': {
          const h = text.slice(i, i + 2)
          if (!/^[0-9a-fA-F]{2}$/.test(h)) throw new JsonScanError('invalid \\x escape')
          out += String.fromCharCode(parseInt(h, 16))
          i += 2
          break
        }
        case 'u': {
          if (text[i] === '{') {
            const close = text.indexOf('}', i)
            const h = close > 0 ? text.slice(i + 1, close) : ''
            if (!/^[0-9a-fA-F]{1,6}$/.test(h) || parseInt(h, 16) > 0x10ffff) throw new JsonScanError('invalid \\u{} escape')
            out += String.fromCodePoint(parseInt(h, 16))
            i = close + 1
          } else {
            const h = text.slice(i, i + 4)
            if (!/^[0-9a-fA-F]{4}$/.test(h)) throw new JsonScanError('invalid \\u escape')
            out += String.fromCharCode(parseInt(h, 16))
            i += 4
          }
          break
        }
        case undefined: throw new JsonScanError('unterminated string')
        default: out += e
      }
      continue
    }
    if ((c === '\n' || c === '\r') && q !== '`') throw new JsonScanError('unterminated string')
    out += c
    i++
  }
  throw new JsonScanError('unterminated string')
}

class LooseParser {
  private i = 0
  private nodes = 0
  constructor(private readonly s: string) {}

  parse(): unknown {
    const v = this.value(0)
    this.ws()
    if (this.i < this.s.length) throw new JsonScanError(`unexpected "${this.s[this.i]}" after the data`)
    return v
  }

  private ws(): void {
    const s = this.s
    for (;;) {
      while (isSpace(s[this.i])) this.i++
      if (s[this.i] === '/' && s[this.i + 1] === '/') {
        while (this.i < s.length && s[this.i] !== '\n') this.i++
      } else if (s[this.i] === '/' && s[this.i + 1] === '*') {
        const end = s.indexOf('*/', this.i + 2)
        if (end < 0) throw new JsonScanError('unterminated comment')
        this.i = end + 2
      } else return
    }
  }

  private value(depth: number): unknown {
    if (depth > MAX_DEPTH) throw new JsonScanError(`the data is nested deeper than ${MAX_DEPTH} levels`)
    if (++this.nodes > MAX_NODES) throw new JsonScanError('the data has too many values')
    this.ws()
    const s = this.s
    const c = s[this.i]
    if (c === '{') return this.object(depth)
    if (c === '[') return this.array(depth)
    if (c === '"' || c === "'") {
      const { value, end } = decodeJsString(s, this.i)
      this.i = end
      return value
    }
    if (c === '!' && (s[this.i + 1] === '0' || s[this.i + 1] === '1')) {
      this.i += 2
      return s[this.i - 1] === '0'
    }
    if (c === '-' || c === '+' || c === '.' || (c !== undefined && c >= '0' && c <= '9')) return this.number()
    const word = this.word()
    switch (word) {
      case 'true': return true
      case 'false': return false
      case 'null': case 'undefined': case 'NaN': case 'Infinity': return null
      case '': throw new JsonScanError(c === undefined ? 'unexpected end of data' : `unexpected "${c}"`)
      default:
        if (word === 'function' || word === 'new' || s[this.i] === '(' || s[this.i] === '`') throw new JsonScanError('the data contains code, not values')
        // A reference to a variable defined elsewhere in the page script: its value is unknown.
        return null
    }
  }

  private word(): string {
    const start = this.i
    while (this.i < this.s.length && /[A-Za-z0-9_$.]/.test(this.s[this.i]!)) this.i++
    return this.s.slice(start, this.i)
  }

  private number(): number | null {
    const m = /^[+-]?(?:0[xX][0-9a-fA-F]+|(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?)/.exec(this.s.slice(this.i, this.i + 400))
    if (!m) throw new JsonScanError('invalid number')
    this.i += m[0].length
    const n = Number(m[0])
    return Number.isFinite(n) ? n : null
  }

  private key(): string {
    this.ws()
    const c = this.s[this.i]
    if (c === '"' || c === "'") {
      const { value, end } = decodeJsString(this.s, this.i)
      this.i = end
      return value
    }
    const start = this.i
    while (this.i < this.s.length && /[A-Za-z0-9_$]/.test(this.s[this.i]!)) this.i++
    if (this.i === start) throw new JsonScanError(c === undefined ? 'unexpected end of data' : `unexpected "${c}" where a key was expected`)
    return this.s.slice(start, this.i)
  }

  private object(depth: number): Record<string, unknown> {
    const out: Record<string, unknown> = {}
    this.i++
    for (;;) {
      this.ws()
      if (this.s[this.i] === '}') {
        this.i++
        return out
      }
      const k = this.key()
      this.ws()
      if (this.s[this.i] !== ':') throw new JsonScanError(`expected ":" after "${k.slice(0, 40)}"`)
      this.i++
      const v = this.value(depth + 1)
      if (k !== '__proto__') out[k] = v
      this.ws()
      if (this.s[this.i] === ',') this.i++
      else if (this.s[this.i] !== '}') throw new JsonScanError('expected "," or "}"')
    }
  }

  private array(depth: number): unknown[] {
    const out: unknown[] = []
    this.i++
    for (;;) {
      this.ws()
      if (this.s[this.i] === ']') {
        this.i++
        return out
      }
      if (this.s[this.i] === ',') {
        this.i++
        out.push(null)
        continue
      }
      out.push(this.value(depth + 1))
      this.ws()
      if (this.s[this.i] === ',') this.i++
      else if (this.s[this.i] !== ']') throw new JsonScanError('expected "," or "]"')
    }
  }
}

/** Parses JSON or a plain JavaScript object/array literal. Never evaluates anything. */
export function parseLoose(text: string): unknown {
  if (text.length > MAX_SCRIPT_BYTES) throw new JsonScanError(`the data is larger than ${Math.round(MAX_SCRIPT_BYTES / 1000)} kB`)
  return new LooseParser(text).parse()
}

/** JSON.parse first (fast and exact), then the lenient object-literal parser. */
export function parseData(text: string): unknown {
  if (text.length > MAX_SCRIPT_BYTES) throw new JsonScanError(`the data is larger than ${Math.round(MAX_SCRIPT_BYTES / 1000)} kB`)
  try {
    return JSON.parse(text)
  } catch {
    return parseLoose(text)
  }
}

function skipSpace(text: string, i: number): number {
  while (i < text.length && isSpace(text[i])) i++
  return i
}

/** Reads the value at `i`: an object or array literal, a string holding JSON, or JSON.parse("..."). */
function readValueAt(text: string, i: number): unknown {
  i = skipSpace(text, i)
  const c = text[i]
  if (c === '{' || c === '[') return parseData(text.slice(i, scanBalanced(text, i)))
  if (c === '"' || c === "'" || c === '`') {
    const inner = decodeJsString(text, i).value.trim()
    if (inner.startsWith('{') || inner.startsWith('[')) return parseData(inner)
    throw new JsonScanError('the string after the marker does not contain JSON')
  }
  if (text.startsWith('JSON.parse', i)) {
    const j = skipSpace(text, i + 'JSON.parse'.length)
    if (text[j] !== '(') throw new JsonScanError('expected "(" after JSON.parse')
    return readValueAt(text, j + 1)
  }
  throw new JsonScanError('no object, array or JSON string follows the marker')
}

/**
 * Finds `marker` in script text and reads the value that follows it, after an optional closing
 * quote (for keys such as "productIntroData":) and one "=", ":" or "(". Later occurrences are
 * tried when an earlier one is not followed by data.
 */
export function valueAfterMarker(text: string, marker: string): { value: unknown } | { error: string } {
  if (!marker) return { error: 'empty marker' }
  let from = 0
  let lastError = 'the marker was not found'
  for (let n = 0; n < 20; n++) {
    const at = text.indexOf(marker, from)
    if (at < 0) break
    from = at + marker.length
    let i = at + marker.length
    if ((text[i] === '"' || text[i] === "'") && /[:=]/.test(text[skipSpace(text, i + 1)] ?? '')) i++
    i = skipSpace(text, i)
    if (text[i] === '=' && text[i + 1] !== '=') i++
    else if (text[i] === ':' || text[i] === '(') i++
    try {
      return { value: readValueAt(text, i) }
    } catch (e) {
      lastError = e instanceof Error ? e.message : String(e)
    }
  }
  return { error: lastError }
}
