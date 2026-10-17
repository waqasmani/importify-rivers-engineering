import { expect, type Cookie, type Page, type TestInfo } from '@playwright/test'

export const OWNER = { email: 'owner@example.com', password: 'correct horse battery staple 42', name: 'Olivia Owner' }
export const SETUP_TOKEN = 'dev-setup-token'

// Sessions are reused across tests: sign-in is rate limited per IP (20 per minute), which a fast
// suite would otherwise trip.
const sessions = new Map<string, Cookie[]>()

/** Signs in through the API (once per user) so tests start on an authenticated page. */
export async function signIn(page: Page, email = OWNER.email, password = OWNER.password) {
  const cached = sessions.get(email)
  if (cached) {
    await page.context().addCookies(cached)
    return
  }
  const res = await page.request.post('/api/v1/auth/login', { data: { email, password } })
  expect(res.status(), await res.text()).toBe(200)
  sessions.set(email, await page.context().cookies())
}

/** Fails the test on console errors and on server errors, ignoring the expected pre-login 401. */
export function watchForErrors(page: Page, testInfo: TestInfo) {
  const problems: string[] = []
  page.on('console', (msg) => {
    if (msg.type() !== 'error') return
    const text = msg.text()
    if (text.includes('401') || text.includes('Unauthorized')) return
    problems.push(`console: ${text}`)
  })
  page.on('pageerror', (err) => problems.push(`page error: ${err.message}`))
  page.on('response', (res) => {
    if (res.status() >= 500) problems.push(`HTTP ${res.status()} ${res.request().method()} ${res.url()}`)
  })
  return () => expect(problems, problems.join('\n')).toEqual([])
}

/** GET through the signed-in browser context. */
export async function apiGet<T>(page: Page, path: string): Promise<T> {
  const res = await page.request.get(`/api/v1${path}`)
  expect(res.ok(), `${path}: ${res.status()}`).toBeTruthy()
  return (await res.json()) as T
}
