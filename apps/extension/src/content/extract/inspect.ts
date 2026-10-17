// Local page inspector: platform, technologies, headings and metadata. Nothing here is sent to
// REA unless the user saves a capture.
import { absUrl, collapse, hostOf, meta, publicMeta, text, truncate } from '../dom'
import type { InspectorData, PageType } from '../../shared/types'
import { readOpenGraph } from './opengraph'
import { microdataSummary, readMicrodata } from './microdata'
import { flattenNodes, hasType, organizationName, typesOf, websiteName, type Node } from './schemaorg'
import { shopifyMarkers } from './shopify'
import { wooMarkers } from './woocommerce'
import type { PageContext } from './types'

const HOST_TECH: [RegExp, string][] = [
  [/(^|\.)cdn\.shopify\.com$/, 'Shopify'], [/(^|\.)googletagmanager\.com$/, 'Google Tag Manager'],
  [/(^|\.)google-analytics\.com$/, 'Google Analytics'], [/(^|\.)js\.stripe\.com$/, 'Stripe'],
  [/(^|\.)connect\.facebook\.net$/, 'Meta Pixel'], [/(^|\.)static\.hotjar\.com$/, 'Hotjar'],
  [/(^|\.)js\.hs-scripts\.com$|(^|\.)js\.hsforms\.net$/, 'HubSpot'], [/(^|\.)widget\.intercom\.io$|(^|\.)js\.intercomcdn\.com$/, 'Intercom'],
  [/(^|\.)cdn\.segment\.com$/, 'Segment'], [/(^|\.)static\.klaviyo\.com$/, 'Klaviyo'], [/(^|\.)assets\.adobedtm\.com$/, 'Adobe Experience Platform'],
  [/(^|\.)cdn\.cookielaw\.org$/, 'OneTrust'], [/(^|\.)js\.braintreegateway\.com$/, 'Braintree'], [/(^|\.)paypal(objects)?\.com$/, 'PayPal'],
  [/(^|\.)maps\.googleapis\.com$/, 'Google Maps'], [/(^|\.)recaptcha\.net$|(^|\.)gstatic\.com$/, 'Google reCAPTCHA / static'],
  [/(^|\.)hcaptcha\.com$/, 'hCaptcha'], [/(^|\.)cdn\.jsdelivr\.net$/, 'jsDelivr CDN'], [/(^|\.)unpkg\.com$/, 'unpkg CDN'],
  [/(^|\.)cdnjs\.cloudflare\.com$/, 'cdnjs'], [/(^|\.)ajax\.googleapis\.com$/, 'Google Hosted Libraries'],
  [/(^|\.)bigcommerce\.com$/, 'BigCommerce'], [/(^|\.)wixstatic\.com$|(^|\.)parastorage\.com$/, 'Wix'],
  [/(^|\.)squarespace(-cdn)?\.com$/, 'Squarespace'], [/(^|\.)alicdn\.com$/, 'Alibaba CDN'], [/(^|\.)cloudflareinsights\.com$/, 'Cloudflare Web Analytics'],
  [/(^|\.)clarity\.ms$/, 'Microsoft Clarity'], [/(^|\.)tiktok\.com$/, 'TikTok Pixel'], [/(^|\.)snap\.licdn\.com$/, 'LinkedIn Insight'],
  [/(^|\.)zdassets\.com$/, 'Zendesk'], [/(^|\.)crisp\.chat$/, 'Crisp'], [/(^|\.)tawk\.to$/, 'tawk.to'],
]

const PATH_TECH: [RegExp, string][] = [
  [/\/_next\//, 'Next.js'], [/\/_nuxt\//, 'Nuxt'], [/\/wp-content\/plugins\/woocommerce\//, 'WooCommerce'],
  [/\/wp-content\/|\/wp-includes\//, 'WordPress'], [/\/static\/version\d+\/frontend\/|\/mage\//, 'Magento'],
  [/\/sites\/default\/files\/|\/core\/misc\/drupal/, 'Drupal'], [/\/media\/jui\/|\/media\/system\/js\//, 'Joomla'],
  [/\/etc\.clientlibs\//, 'Adobe Experience Manager'], [/\/modules\/ps_|\/themes\/classic\/assets\//, 'PrestaShop'],
  [/\/build\/assets\/app-[\w-]+\.js/, 'Laravel Vite'], [/\/packs\/js\//, 'Rails Webpacker'], [/\/assets\/index-[\w-]+\.js$/, 'Vite build'],
]

const GENERATORS: [RegExp, string][] = [
  [/wordpress/i, 'WordPress'], [/woocommerce/i, 'WooCommerce'], [/shopify/i, 'Shopify'], [/wix\.com/i, 'Wix'],
  [/drupal/i, 'Drupal'], [/joomla/i, 'Joomla'], [/squarespace/i, 'Squarespace'], [/hugo/i, 'Hugo'], [/docusaurus/i, 'Docusaurus'],
  [/ghost/i, 'Ghost'], [/webflow/i, 'Webflow'], [/prestashop/i, 'PrestaShop'], [/magento/i, 'Magento'], [/gatsby/i, 'Gatsby'],
  [/mkdocs/i, 'MkDocs'], [/sphinx/i, 'Sphinx'], [/jekyll/i, 'Jekyll'], [/hexo/i, 'Hexo'], [/typo3/i, 'TYPO3'], [/gitbook/i, 'GitBook'],
]

const COMMERCE = ['Shopify', 'WooCommerce', 'Magento', 'BigCommerce', 'PrestaShop', 'Wix', 'Squarespace']

function addTech(list: { name: string; evidence: string }[], name: string, evidence: string): void {
  if (!list.some((t) => t.name === name)) list.push({ name, evidence })
}

export function detectTechnologies(ctx: PageContext): { technologies: { name: string; evidence: string }[]; scriptHosts: string[]; generator: string } {
  const d = ctx.doc
  const tech: { name: string; evidence: string }[] = []
  const generator = Array.from(d.querySelectorAll('meta[name]')).filter((m) => (m.getAttribute('name') ?? '').toLowerCase() === 'generator').map((m) => collapse(m.getAttribute('content') ?? '')).filter(Boolean).join('; ')
  for (const [re, name] of GENERATORS) if (re.test(generator)) addTech(tech, name, `generator meta "${truncate(generator, 80)}"`)
  const hosts = new Set<string>()
  const urls = [
    ...Array.from(d.querySelectorAll('script[src]')).map((s) => s.getAttribute('src') ?? ''),
    ...Array.from(d.querySelectorAll('link[href][rel~="stylesheet"], link[href][rel~="preload"], link[href][rel~="modulepreload"]')).map((l) => l.getAttribute('href') ?? ''),
  ]
  for (const raw of urls.slice(0, 400)) {
    const abs = absUrl(raw, ctx.base)
    if (!abs) continue
    const u = new URL(abs)
    const host = u.hostname.toLowerCase()
    if (host !== new URL(ctx.url).hostname.toLowerCase()) hosts.add(host)
    for (const [re, name] of HOST_TECH) if (re.test(host)) addTech(tech, name, `asset from ${host}`)
    for (const [re, name] of PATH_TECH) if (re.test(u.pathname)) addTech(tech, name, `asset path ${truncate(u.pathname, 80)}`)
  }
  if (d.getElementById('__next') || d.querySelector('script#__NEXT_DATA__')) addTech(tech, 'Next.js', '#__next root element')
  if (d.getElementById('__nuxt') || d.getElementById('__NUXT_DATA__')) addTech(tech, 'Nuxt', '#__nuxt root element')
  if (d.getElementById('___gatsby')) addTech(tech, 'Gatsby', '#___gatsby root element')
  if (d.querySelector('[ng-version]')) addTech(tech, 'Angular', `ng-version ${d.querySelector('[ng-version]')?.getAttribute('ng-version') ?? ''}`.trim())
  if (d.querySelector('[data-reactroot], [data-react-helmet]')) addTech(tech, 'React', 'React root attributes')
  if (d.querySelector('[data-v-app], [data-server-rendered]')) addTech(tech, 'Vue.js', 'Vue root attributes')
  if (d.querySelector('[data-svelte-h], [class*="svelte-"]')) addTech(tech, 'Svelte', 'Svelte class names')
  if (shopifyMarkers(d).length) addTech(tech, 'Shopify', shopifyMarkers(d).join(', '))
  if (wooMarkers(d).length) addTech(tech, 'WooCommerce', wooMarkers(d).join(', '))
  return { technologies: tech.slice(0, 60), scriptHosts: [...hosts].sort().slice(0, 60), generator: truncate(generator, 300) }
}

function platformFor(ctx: PageContext, tech: { name: string; evidence: string }[]): InspectorData['platform'] {
  const host = ctx.host
  const marketplaces: [RegExp, string, string][] = [
    [/(^|\.)alibaba\.com$/, 'alibaba', 'Alibaba.com'], [/(^|\.)aliexpress\.(com|us)$/, 'aliexpress', 'AliExpress'],
    [/(^|\.)amazon\.[a-z.]+$/, 'amazon', 'Amazon'], [/(^|\.)ebay\.[a-z.]+$/, 'ebay', 'eBay'], [/(^|\.)etsy\.com$/, 'etsy', 'Etsy'],
    [/(^|\.)made-in-china\.com$/, 'made-in-china', 'Made-in-China.com'], [/(^|\.)globalsources\.com$/, 'globalsources', 'Global Sources'],
  ]
  for (const [re, key, label] of marketplaces) if (re.test(host)) return { key, label, evidence: `domain ${host}` }
  for (const name of COMMERCE) {
    const t = tech.find((x) => x.name === name)
    if (t) return { key: name.toLowerCase(), label: name, evidence: t.evidence }
  }
  const cms = tech.find((t) => /WordPress|Drupal|Joomla|Ghost|Webflow|Hugo|Docusaurus|TYPO3|GitBook|MkDocs|Sphinx/.test(t.name))
  if (cms) return { key: cms.name.toLowerCase().replace(/\W+/g, '-'), label: cms.name, evidence: cms.evidence }
  const fw = tech.find((t) => /Next\.js|Nuxt|Gatsby|Angular|React|Vue\.js|Svelte/.test(t.name))
  if (fw) return { key: fw.name.toLowerCase().replace(/\W+/g, '-'), label: fw.name, evidence: fw.evidence }
  return null
}

/** The site's name from the last segment of a title such as "Product - Shop Name". */
function titleSuffix(title: string): string {
  const parts = collapse(title).split(/\s+[|–—]\s+|\s+-\s+/)
  if (parts.length < 2) return ''
  const last = (parts[parts.length - 1] ?? '').trim()
  return last && last.split(/\s+/).length <= 4 && last.length <= 60 ? last : ''
}

export function siteNameOf(ctx: PageContext, nodes: Node[]): string {
  return (
    meta(ctx.doc, 'og:site_name') ||
    websiteName(nodes) ||
    organizationName(nodes)?.name ||
    meta(ctx.doc, 'application-name') ||
    titleSuffix(ctx.doc.title) ||
    ''
  )
}

export function inspectPage(ctx: PageContext): InspectorData {
  const d = ctx.doc
  const nodes = flattenNodes(ctx.jsonLd)
  const { technologies, scriptHosts, generator } = detectTechnologies(ctx)
  const headings = Array.from(d.querySelectorAll('h1, h2, h3'))
    .slice(0, 60)
    .map((h) => ({ level: Number(h.tagName.slice(1)), text: text(h, 200) }))
    .filter((h) => h.text)
  let internal = 0
  let external = 0
  const emails = new Set<string>()
  const phones = new Set<string>()
  for (const a of Array.from(d.querySelectorAll('a[href]')).slice(0, 2000)) {
    const href = (a.getAttribute('href') ?? '').trim()
    if (/^mailto:/i.test(href)) {
      const e = decodeURIComponent(href.slice(7).split('?')[0] ?? '').trim().toLowerCase()
      if (/^[^@\s]+@[^@\s]+\.[a-z]{2,}$/i.test(e) && emails.size < 10) emails.add(e)
      continue
    }
    if (/^tel:/i.test(href)) {
      const p = decodeURIComponent(href.slice(4)).replace(/[^\d+]/g, '')
      if (p.length >= 6 && phones.size < 10) phones.add(p)
      continue
    }
    const abs = absUrl(href, ctx.base)
    if (!abs) continue
    if (hostOf(abs) === ctx.host) internal++
    else external++
  }
  const jsonLd = ctx.jsonLd.slice(0, 20).map((data) => {
    const list = flattenNodes([data], 50)
    return { types: [...new Set(list.flatMap((n) => typesOf(n)))].slice(0, 20), data }
  })
  return {
    platform: platformFor(ctx, technologies),
    canonicalUrl: absUrl(d.querySelector('link[rel="canonical"]')?.getAttribute('href'), ctx.base),
    lang: truncate(d.documentElement.getAttribute('lang') ?? '', 20),
    siteName: siteNameOf(ctx, nodes),
    generator,
    jsonLd,
    jsonLdErrors: ctx.jsonLdErrors,
    microdata: microdataSummary(readMicrodata(d, ctx.base)),
    openGraph: readOpenGraph(d),
    meta: publicMeta(d),
    headings,
    technologies,
    scriptHosts,
    links: { internal, external },
    contacts: { emails: [...emails], phones: [...phones] },
  }
}

/** Classifies the page using structured data, URL shape and the product adapters' signals. */
export function detectPageType(ctx: PageContext, productDetected: boolean, generator: string): { type: PageType; reason: string } {
  const nodes = flattenNodes(ctx.jsonLd)
  const products = nodes.filter((n) => hasType(n, 'Product', 'ProductGroup'))
  const path = (() => {
    try {
      return new URL(ctx.url).pathname.toLowerCase()
    } catch {
      return ''
    }
  })()
  const ogType = meta(ctx.doc, 'og:type').toLowerCase()
  if (productDetected && products.length < 3) return { type: 'product', reason: 'product data was found on the page' }
  if (nodes.some((n) => hasType(n, 'ItemList', 'CollectionPage', 'OfferCatalog', 'SearchResultsPage')) || products.length >= 3) {
    return { type: 'listing', reason: 'the page lists several items' }
  }
  if (productDetected) return { type: 'product', reason: 'product data was found on the page' }
  if (nodes.some((n) => hasType(n, 'Article', 'NewsArticle', 'BlogPosting', 'TechArticle')) || ogType === 'article') {
    if (nodes.some((n) => hasType(n, 'TechArticle', 'APIReference'))) return { type: 'documentation', reason: 'technical article markup' }
    return { type: 'article', reason: 'article markup' }
  }
  if (/(^|\.)(docs|developer|developers|dev)\./.test(ctx.host) || /\/(docs?|documentation|api-reference|reference|guides?|manual)(\/|$)/.test(path) ||
    /docusaurus|mkdocs|sphinx|gitbook|readthedocs|vitepress/i.test(generator)) {
    return { type: 'documentation', reason: 'documentation site or path' }
  }
  if (nodes.some((n) => hasType(n, 'ContactPage')) || /\/(contact|contact-us|kontakt|impressum|imprint)(\/|$|\.)/.test(path)) {
    return { type: 'contact', reason: 'contact page' }
  }
  if (nodes.some((n) => hasType(n, 'AboutPage')) || /\/(about|about-us|company|team|who-we-are)(\/|$|\.)/.test(path) ||
    (nodes.some((n) => hasType(n, 'Organization', 'Corporation', 'LocalBusiness')) && (path === '/' || path === ''))) {
    return { type: 'company', reason: 'organization or about page' }
  }
  return { type: 'unknown', reason: 'no product, article or company markers were found' }
}
