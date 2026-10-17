// Manifest V3 definition. The public key fixes the extension ID to olceibeplclcegianhjffambadoakkno,
// which the REA server allows for bearer-token CORS requests by default.

export const PERMISSIONS = ['activeTab', 'scripting', 'storage', 'contextMenus', 'sidePanel']
export const OPTIONAL_HOST_PERMISSIONS = ['https://*/*', 'http://*/*']
export const EXTENSION_PAGES_CSP = [
  "default-src 'self'",
  "script-src 'self'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
  "frame-ancestors 'none'",
  "style-src 'self'",
  "img-src 'self' data: https: http:",
  // CSP host sources cannot express IPv6 literals such as http://[::1], so plain http is allowed
  // here and the service worker enforces "https, or http only for localhost/127.0.0.1/[::1]"
  // (src/shared/server-url.ts) before any request is made.
  "connect-src 'self' https: http:",
].join('; ')

export function createManifest({ version, key }) {
  const icons = { 16: 'icons/icon-16.png', 32: 'icons/icon-32.png', 48: 'icons/icon-48.png', 128: 'icons/icon-128.png' }
  return {
    manifest_version: 3,
    name: 'REA Importify',
    short_name: 'REA',
    version,
    description: 'Capture product pages, import products into the REA review queue and research suppliers with your self-hosted REA server.',
    key,
    minimum_chrome_version: '116',
    permissions: PERMISSIONS,
    optional_host_permissions: OPTIONAL_HOST_PERMISSIONS,
    background: { service_worker: 'background.js', type: 'module' },
    action: {
      default_title: 'REA Importify',
      default_popup: 'popup.html',
      default_icon: { 16: icons[16], 32: icons[32], 48: icons[48] },
    },
    side_panel: { default_path: 'sidepanel.html' },
    icons,
    content_security_policy: { extension_pages: EXTENSION_PAGES_CSP },
  }
}
