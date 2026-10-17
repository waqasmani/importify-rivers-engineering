import { describe, expect, it } from 'vitest'
import { buildSnapshot } from '../src/content/snapshot'
import { parse } from './helpers'

const HOSTILE = `<!doctype html><html><head>
  <meta name="csrf-token" content="csrf-secret-1">
  <meta name="_token" content="laravel-secret-2">
  <meta http-equiv="set-cookie" content="sid=cookie-secret-3">
  <meta http-equiv="refresh" content="0;url=https://evil.example">
  <meta name="description" content="A safe description">
  <meta property="og:title" content="Safe title">
  <base href="https://evil.example/">
  <script>window.secret = 'inline-secret-4'</script>
  <script src="https://cdn.example/app.js"></script>
  <noscript><img src="https://tracker.example/pixel"></noscript>
  <style>.x{color:red}</style>
</head><body onload="steal()">
  <!-- api_key=comment-secret-5 -->
  <h1 onclick="alert(1)" style="color:blue">Product</h1>
  <a href="javascript:alert(2)">bad link</a><a href="/good">good link</a>
  <iframe src="https://ads.example/frame" srcdoc="<b>x</b>"></iframe>
  <object data="movie.swf"></object><embed src="x.swf">
  <template><p>template-secret-6</p></template>
  <form action="javascript:void(0)">
    <input name="email" type="email" value="person@example.com">
    <input name="password" type="password" value="password-secret-7">
    <input name="csrf" type="hidden" value="hidden-secret-8">
    <input type="checkbox" checked name="remember">
    <textarea name="notes">textarea-secret-9</textarea>
    <select name="size"><option selected value="L">Large</option></select>
    <button formaction="javascript:x()">Send</button>
  </form>
  <div data-session-id="data-secret-10" data-product="ok" x-auth-token="attr-secret-11" nonce="nonce-secret-12">Visible text</div>
  <img src="https://cdn.example/p.jpg" onerror="steal()" alt="Product photo">
</body></html>`

describe('snapshot sanitizer', () => {
  const snap = buildSnapshot(parse(HOSTILE), true, 1024 * 1024)
  const html = snap.html ?? ''

  it('includes a sanitized document', () => {
    expect(snap.included).toBe(true)
    expect(html.startsWith('<!doctype html>')).toBe(true)
    expect(html).toContain('Visible text')
    expect(html).toContain('A safe description')
    expect(html).toContain('og:title')
    expect(html).toContain('href="/good"')
    expect(html).toContain('https://cdn.example/p.jpg')
    expect(snap.bytes).toBe(new TextEncoder().encode(html).length)
  })

  it('removes scripts, frames, plugins, templates and comments', () => {
    expect(html).not.toMatch(/<script|<iframe|<object|<embed|<noscript|<template|<base/i)
    expect(html).not.toContain('inline-secret-4')
    expect(html).not.toContain('template-secret-6')
    expect(html).not.toContain('comment-secret-5')
    expect(html).not.toContain('srcdoc')
  })

  it('removes event handlers and javascript: URLs', () => {
    expect(html).not.toMatch(/\son[a-z]+=/i)
    expect(html).not.toMatch(/javascript:/i)
  })

  it('removes form values, password and hidden inputs', () => {
    for (const s of ['person@example.com', 'password-secret-7', 'hidden-secret-8', 'textarea-secret-9']) expect(html).not.toContain(s)
    expect(html).not.toMatch(/type="password"|type="hidden"/)
    expect(html).not.toMatch(/\schecked|\sselected/)
    expect(html).toContain('<input name="email" type="email">')
  })

  it('removes token-like meta tags and attributes', () => {
    for (const s of ['csrf-secret-1', 'laravel-secret-2', 'cookie-secret-3', 'evil.example', 'data-secret-10', 'attr-secret-11', 'nonce-secret-12']) {
      expect(html).not.toContain(s)
    }
    expect(html).toContain('data-product="ok"')
  })

  it('never modifies the live page', () => {
    const doc = parse(HOSTILE)
    buildSnapshot(doc, true, 1024 * 1024)
    expect(doc.querySelectorAll('script').length).toBe(2)
    expect(doc.querySelector('input[type="password"]')?.getAttribute('value')).toBe('password-secret-7')
  })

  it('respects the configured size limit', () => {
    const big = `<html><head><style>${'.a{color:red}'.repeat(5000)}</style></head><body><p>${'x'.repeat(2000)}</p></body></html>`
    const slim = buildSnapshot(parse(big), true, 20 * 1024)
    expect(slim.included).toBe(true)
    expect(slim.html).not.toContain('<style>')
    expect(slim.bytes).toBeLessThanOrEqual(20 * 1024)
    const tooBig = buildSnapshot(parse(`<html><body><p>${'y'.repeat(50_000)}</p></body></html>`), true, 16 * 1024)
    expect(tooBig.included).toBe(false)
    expect(tooBig.html).toBeNull()
    expect(tooBig.note).toMatch(/larger than the 16 KB limit/)
  })

  it('is skipped entirely when snapshots are off', () => {
    const off = buildSnapshot(parse(HOSTILE), false, 1024 * 1024)
    expect(off).toEqual({ included: false, bytes: 0, html: null, note: 'HTML snapshots are turned off.' })
  })
})
