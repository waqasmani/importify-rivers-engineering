import { describe, expect, it } from 'vitest'
import { apiUrl, normalizeServerUrl, webUrl } from '../src/shared/server-url'

describe('server URL validation', () => {
  it.each([
    ['https://rea.example.com', 'https://rea.example.com'],
    ['  https://REA.Example.com/  ', 'https://rea.example.com'],
    ['rea.example.com', 'https://rea.example.com'],
    ['rea.example.com:8443', 'https://rea.example.com:8443'],
    ['https://rea.example.com:443/', 'https://rea.example.com'],
    ['https://example.com/rea/', 'https://example.com/rea'],
    ['https://rea.example.com/api/v1', 'https://rea.example.com'],
    ['https://rea.example.com/api/v1/?x=1#frag', 'https://rea.example.com'],
    ['http://localhost:8080', 'http://localhost:8080'],
    ['localhost:8080', 'http://localhost:8080'],
    ['http://127.0.0.1:18105', 'http://127.0.0.1:18105'],
    ['127.0.0.1:18105', 'http://127.0.0.1:18105'],
    ['http://[::1]:8080/', 'http://[::1]:8080'],
    ['[::1]:8080', 'http://[::1]:8080'],
  ])('accepts %s', (input, expected) => {
    const r = normalizeServerUrl(input)
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.url).toBe(expected)
  })

  it.each([
    ['', /Enter the address/],
    ['http://rea.example.com', /Use https:\/\//],
    ['http://192.168.1.10:8080', /Use https:\/\//],
    ['http://localhost.evil.example', /Use https:\/\//],
    ['http://127.0.0.2', /Use https:\/\//],
    ['ftp://rea.example.com', /must start with https/],
    ['javascript:alert(1)', /must start with https/],
    ['https://user:pass@rea.example.com', /user name and password/],
    ['https://', /not a valid web address/],
    ['https://' + 'a'.repeat(2050), /too long/],
  ])('rejects %s', (input, message) => {
    const r = normalizeServerUrl(input)
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.error).toMatch(message)
  })

  it('flags local development servers', () => {
    const r = normalizeServerUrl('http://localhost:3000')
    expect(r.ok && r.local).toBe(true)
    const s = normalizeServerUrl('https://rea.example.com')
    expect(s.ok && s.local).toBe(false)
  })

  it('builds API and web links', () => {
    expect(apiUrl('https://example.com/rea', '/extension/config')).toBe('https://example.com/rea/api/v1/extension/config')
    expect(webUrl('https://rea.example.com', 'imports/1')).toBe('https://rea.example.com/imports/1')
  })
})
