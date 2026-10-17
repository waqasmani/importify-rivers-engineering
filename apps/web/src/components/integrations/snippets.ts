// Copy-ready examples for the "Connect your system" panel. They mirror docs/catalog-api.md and use
// shell variables for the key so it never has to be pasted into a document.

export interface Snippet {
  key: string
  title: string
  description: string
  code: string
}

export function curlSnippets(apiBase: string): Snippet[] {
  const setup = `export REA_API='${apiBase}'\nexport REA_KEY='reai_…'   # an integration key with catalog.read`
  const auth = '-H "Authorization: Bearer $REA_KEY"'
  return [
    { key: 'setup', title: 'Set up', description: 'Store the API address and your integration key in the shell.', code: setup },
    {
      key: 'list',
      title: 'List the catalog',
      description: 'Published products, oldest change first. Repeat with cursor=<next_cursor> while has_more is true; keep change_cursor from the first page.',
      code: `curl -fsS ${auth} "$REA_API/catalog/products?limit=100"\ncurl -fsS ${auth} "$REA_API/catalog/products?limit=100&cursor=NEXT_CURSOR"`,
    },
    {
      key: 'changes',
      title: 'Follow the change feed',
      description: 'Apply events in order: product.unpublished and product.deleted remove the product, every other event upserts it. Store next_cursor after each page.',
      code: `curl -fsS ${auth} "$REA_API/catalog/changes?since=CHANGE_CURSOR&limit=200"`,
    },
    {
      key: 'lookup',
      title: 'Look up one product',
      description: 'By ID or by product or variant SKU. Send If-None-Match with the last ETag to get 304 when nothing changed.',
      code: `curl -fsS ${auth} "$REA_API/catalog/products/PRODUCT_ID"\ncurl -fsS ${auth} "$REA_API/catalog/products/by-sku/MUG-RED-L"`,
    },
    {
      key: 'import',
      title: 'Submit a product for review',
      description: 'Needs a key with imports.write. The product lands in the import queue as a draft; nothing is published until someone approves it.',
      code:
        `curl -fsS ${auth} -H 'Content-Type: application/json' "$REA_API/imports/products" \\\n` +
        `  -d '{"payload":{"url":"https://supplier.example/p/1","title":"Steel bottle 750 ml","price":"19.90","currency":"EUR"}}'`,
    },
  ]
}

export function verifySnippets(signatureHeader: string, timestampHeader: string): Snippet[] {
  const sig = signatureHeader.toLowerCase()
  const ts = timestampHeader.toLowerCase()
  return [
    {
      key: 'node',
      title: 'Verify a delivery (Node.js)',
      description: 'Compute HMAC-SHA256 over "timestamp.body" with the signing secret and compare it with every v1= value. Use the raw request body, before JSON parsing.',
      code: `import { createHmac, timingSafeEqual } from 'node:crypto'

export function verifyREA(rawBody, headers, secret, toleranceSeconds = 300) {
  const timestamp = headers['${ts}']
  const header = headers['${sig}'] ?? ''
  if (!timestamp || Math.abs(Date.now() / 1000 - Number(timestamp)) > toleranceSeconds) return false
  const expected = createHmac('sha256', secret).update(\`\${timestamp}.\`).update(rawBody).digest('hex')
  return header.split(',').some((part) => {
    const value = part.trim().replace(/^v1=/, '')
    return value.length === expected.length && timingSafeEqual(Buffer.from(value), Buffer.from(expected))
  })
}`,
    },
    {
      key: 'go',
      title: 'Verify a delivery (Go)',
      description: 'The same check in Go. Reject the request when it returns false, and deduplicate on the event ID.',
      code: `func verifyREA(body []byte, timestamp, header, secret string) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || math.Abs(float64(time.Now().Unix()-ts)) > 300 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, part := range strings.Split(header, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "v1="); ok && hmac.Equal([]byte(v), []byte(want)) {
			return true
		}
	}
	return false
}`,
    },
  ]
}
