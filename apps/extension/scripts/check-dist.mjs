// Verifies the built extension: fixed key and ID, least-privilege permissions, a strict CSP,
// every referenced file present, and no source maps, private keys or cookie access shipped.
import { createHash } from 'node:crypto'
import { readdir, readFile, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { EXTENSION_PAGES_CSP, OPTIONAL_HOST_PERMISSIONS, PERMISSIONS } from './manifest.mjs'

export const EXPECTED_ID = 'olceibeplclcegianhjffambadoakkno'

export function extensionIdFromKey(keyB64) {
  const der = Buffer.from(keyB64, 'base64')
  const hex = createHash('sha256').update(der).digest('hex').slice(0, 32)
  return [...hex].map((c) => String.fromCharCode(97 + parseInt(c, 16))).join('')
}

async function walk(dir, base = dir) {
  const out = []
  for (const e of await readdir(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) out.push(...(await walk(p, base)))
    else out.push(path.relative(base, p).split(path.sep).join('/'))
  }
  return out
}

const sameSet = (a, b) => a.length === b.length && [...a].sort().join('\n') === [...b].sort().join('\n')

export async function checkDist(dist, { keyFile } = {}) {
  const problems = []
  const fail = (m) => problems.push(m)
  const exists = async (rel) => {
    try {
      return (await stat(path.join(dist, rel))).isFile()
    } catch {
      return false
    }
  }
  const manifest = JSON.parse(await readFile(path.join(dist, 'manifest.json'), 'utf8'))
  if (manifest.manifest_version !== 3) fail('manifest_version must be 3')
  if (!manifest.key) fail('manifest.key is missing')
  else {
    if (keyFile) {
      const expected = (await readFile(keyFile, 'utf8')).trim()
      if (manifest.key !== expected) fail('manifest.key does not match manifest-key.txt')
    }
    const id = extensionIdFromKey(manifest.key)
    if (id !== EXPECTED_ID) fail(`manifest.key yields extension ID ${id}, expected ${EXPECTED_ID}`)
  }
  if ('host_permissions' in manifest) fail('host_permissions must not be declared')
  if (!sameSet(manifest.permissions ?? [], PERMISSIONS)) fail(`permissions must be exactly ${PERMISSIONS.join(', ')}`)
  if (!sameSet(manifest.optional_host_permissions ?? [], OPTIONAL_HOST_PERMISSIONS)) fail(`optional_host_permissions must be exactly ${OPTIONAL_HOST_PERMISSIONS.join(', ')}`)
  if ('optional_permissions' in manifest) fail('optional_permissions must not be declared')
  for (const k of ['externally_connectable', 'content_scripts', 'web_accessible_resources', 'update_url', 'oauth2', 'sandbox']) {
    if (k in manifest) fail(`${k} must not be declared`)
  }
  if (manifest.background?.type !== 'module' || !manifest.background?.service_worker) fail('background must be an ES module service worker')
  const csp = manifest.content_security_policy?.extension_pages ?? ''
  if (csp !== EXTENSION_PAGES_CSP) fail('content_security_policy.extension_pages differs from the expected strict policy')
  if (/unsafe-eval|unsafe-inline|wasm-unsafe-eval/.test(csp) || !/script-src 'self'(;|$)/.test(csp)) fail('CSP must only allow scripts from the extension itself')

  const referenced = new Set([manifest.background?.service_worker, manifest.action?.default_popup, manifest.side_panel?.default_path, 'content.js'])
  for (const p of Object.values(manifest.icons ?? {})) referenced.add(p)
  for (const p of Object.values(manifest.action?.default_icon ?? {})) referenced.add(p)
  for (const html of [manifest.action?.default_popup, manifest.side_panel?.default_path].filter(Boolean)) {
    if (!(await exists(html))) continue
    const src = await readFile(path.join(dist, html), 'utf8')
    for (const m of src.matchAll(/\b(?:src|href)="([^"]+)"/g)) {
      const ref = m[1]
      if (/^(https?:)?\/\//i.test(ref)) fail(`${html} references remote resource ${ref}`)
      else if (!ref.startsWith('#') && !ref.startsWith('data:')) referenced.add(path.posix.normalize(path.posix.join(path.posix.dirname(html), ref.split(/[?#]/)[0])))
    }
    if (/<script(?![^>]*\bsrc=)[^>]*>/i.test(src)) fail(`${html} contains an inline script`)
  }
  for (const rel of referenced) if (rel && !(await exists(rel))) fail(`referenced file is missing: ${rel}`)

  const files = await walk(dist)
  for (const f of files) {
    if (/\.map$/i.test(f)) fail(`source map shipped: ${f}`)
    if (/\.(pem|key|crx|p12|pfx)$/i.test(f)) fail(`key material shipped: ${f}`)
    if (/\.(js|html|css|json|txt)$/i.test(f)) {
      const text = await readFile(path.join(dist, f), 'utf8')
      if (/-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(text)) fail(`private key found in ${f}`)
      if (/sourceMappingURL=/.test(text)) fail(`source map reference in ${f}`)
    }
  }
  const content = await readFile(path.join(dist, 'content.js'), 'utf8').catch(() => '')
  if (/^\s*(import|export)\s/m.test(content)) fail('content.js must be a classic script (IIFE), not an ES module')
  if (/\.cookie\b|document\[["']cookie["']\]/.test(content)) fail('content.js must not access cookies')
  if (/\bfetch\(|XMLHttpRequest|sendBeacon/.test(content)) fail('content.js must not make network requests')
  if (/__reaContent/.test(content) === false) fail('content.js does not define the content API')
  return { problems, files, manifest }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
  const { problems, files } = await checkDist(path.join(root, 'dist'), { keyFile: path.join(root, 'manifest-key.txt') })
  if (problems.length) {
    console.error('dist check failed:\n  - ' + problems.join('\n  - '))
    process.exit(1)
  }
  console.log(`dist check passed (${files.length} files, extension ID ${EXPECTED_ID})`)
}
