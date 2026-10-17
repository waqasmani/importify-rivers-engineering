import { existsSync } from 'node:fs'
import { cp, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import http from 'node:http'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { chromium, type BrowserContext, type Worker } from '@playwright/test'

export const EXTENSION_ID = 'olceibeplclcegianhjffambadoakkno'
export const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
export const DIST = path.join(ROOT, 'dist')
export const FIXTURES = path.resolve(ROOT, '../../testdata/fixtures')
// The full Chromium build is needed to load extensions. Use REA_E2E_CHROMIUM, then a pre-installed
// browser at /opt/pw-browsers/chromium, then Playwright's own download (CI).
export const CHROMIUM: string | undefined =
  process.env['REA_E2E_CHROMIUM'] ?? (existsSync('/opt/pw-browsers/chromium') ? '/opt/pw-browsers/chromium' : undefined)
export const FIXTURE_PORT = Number(process.env['REA_E2E_FIXTURE_PORT'] ?? 18115)
export const FIXTURE_ORIGIN = `http://127.0.0.1:${FIXTURE_PORT}`

export interface Launched {
  context: BrowserContext
  worker: Worker
  extensionId: string
  close: () => Promise<void>
}

async function extensionWorker(context: BrowserContext): Promise<Worker> {
  const existing = context.serviceWorkers().filter((w) => w.url().startsWith('chrome-extension://'))
  return existing.at(-1) ?? (await context.waitForEvent('serviceworker', { timeout: 30_000 }))
}

/** Loads an unpacked extension into a fresh persistent Chromium profile (new headless mode). */
export async function launchExtension(dir: string): Promise<Launched> {
  const profile = await mkdtemp(path.join(os.tmpdir(), 'rea-ext-profile-'))
  const context = await chromium.launchPersistentContext(profile, {
    // Without an explicit binary, headless mode would pick chromium-headless-shell, which cannot
    // load extensions; the "chromium" channel selects Playwright's full Chromium build (CI).
    ...(CHROMIUM ? { executablePath: CHROMIUM } : { channel: 'chromium' }),
    headless: true,
    args: ['--headless=new', `--disable-extensions-except=${dir}`, `--load-extension=${dir}`, '--no-first-run', '--no-default-browser-check'],
  })
  let worker = await extensionWorker(context)
  // The worker can be replaced right after installation; wait until its extension APIs are ready.
  const deadline = Date.now() + 30_000
  for (;;) {
    const ready = await worker.evaluate(() => typeof chrome?.tabs?.create === 'function' && typeof chrome?.runtime?.id === 'string').catch(() => false)
    if (ready) break
    if (Date.now() > deadline) throw new Error('the extension service worker did not become ready')
    await new Promise((r) => setTimeout(r, 200))
    worker = await extensionWorker(context)
  }
  const extensionId = new URL(worker.url()).host
  return {
    context,
    worker,
    extensionId,
    close: async () => {
      await context.close()
      await rm(profile, { recursive: true, force: true })
    },
  }
}

/**
 * Chromium cannot accept the optional host-permission prompt in automation, so the end-to-end run
 * uses a copy of the built extension that additionally declares access to the local fixture
 * origin only. The shipped dist/ keeps no host permissions (see scripts/check-dist.mjs).
 */
export async function testCopyWithFixtureAccess(): Promise<{ dir: string; cleanup: () => Promise<void> }> {
  const dir = await mkdtemp(path.join(os.tmpdir(), 'rea-ext-e2e-'))
  await cp(DIST, dir, { recursive: true })
  const manifest = JSON.parse(await readFile(path.join(dir, 'manifest.json'), 'utf8'))
  manifest.host_permissions = [`${FIXTURE_ORIGIN}/*`]
  await writeFile(path.join(dir, 'manifest.json'), JSON.stringify(manifest, null, 2))
  return { dir, cleanup: () => rm(dir, { recursive: true, force: true }) }
}

/** Serves testdata/fixtures over plain HTTP on 127.0.0.1. */
export async function serveFixtures(): Promise<{ close: () => Promise<void> }> {
  const server = http.createServer(async (req, res) => {
    const name = path.basename(decodeURIComponent((req.url ?? '/').split('?')[0] ?? '/'))
    try {
      const body = await readFile(path.join(FIXTURES, name))
      res.writeHead(200, { 'Content-Type': name.endsWith('.html') ? 'text/html; charset=utf-8' : 'application/octet-stream' })
      res.end(body)
    } catch {
      res.writeHead(404, { 'Content-Type': 'text/plain' })
      res.end('not found')
    }
  })
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject)
    server.listen(FIXTURE_PORT, '127.0.0.1', () => resolve())
  })
  return { close: () => new Promise<void>((r) => server.close(() => r())) }
}

/** Opens a tab from the extension's service worker and returns its tab id. */
export async function openTab(worker: Worker, url: string): Promise<number> {
  const id = await worker.evaluate(async (u) => (await chrome.tabs.create({ url: u, active: true })).id ?? -1, url)
  await worker.evaluate(
    (tabId) =>
      new Promise<void>((resolve) => {
        const check = () => chrome.tabs.get(tabId).then((t) => (t.status === 'complete' ? resolve() : setTimeout(check, 100)))
        check()
      }),
    id,
  )
  return id
}

/** Minimal REA web-session client (cookie + CSRF header), used to set up and verify the server. */
export class ReaSession {
  cookie = ''
  csrf = ''
  constructor(public base: string) {}

  async request<T>(method: string, path: string, body?: unknown): Promise<{ status: number; body: T }> {
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (body !== undefined) headers['Content-Type'] = 'application/json'
    if (this.cookie) headers['Cookie'] = this.cookie
    if (this.csrf && method !== 'GET') headers['X-CSRF-Token'] = this.csrf
    const res = await fetch(this.base + '/api/v1' + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) })
    const setCookie = res.headers.get('set-cookie')
    if (setCookie) this.cookie = setCookie.split(';')[0] ?? ''
    const text = await res.text()
    return { status: res.status, body: (text ? JSON.parse(text) : null) as T }
  }

  /** Runs first-run setup when needed, otherwise signs in. */
  async signIn(user: { email: string; password: string; org: string; name: string }, setupToken: string): Promise<void> {
    const st = await this.request<{ needs_setup: boolean }>('GET', '/setup/status')
    let res: { status: number; body: { csrf_token?: string } }
    if (st.body.needs_setup) {
      res = await this.request('POST', '/setup', {
        setup_token: setupToken, organization_name: user.org, display_name: user.name, email: user.email, password: user.password, workspace_name: 'General',
      })
    } else {
      res = await this.request('POST', '/auth/login', { email: user.email, password: user.password })
    }
    if (res.status >= 300 || !res.body.csrf_token) throw new Error(`sign-in failed with HTTP ${res.status}: ${JSON.stringify(res.body)}`)
    this.csrf = res.body.csrf_token
  }
}
