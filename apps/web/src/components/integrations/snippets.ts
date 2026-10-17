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

export function orderSnippets(apiBase: string): Snippet[] {
  const setup = `export REA_API='${apiBase}'\nexport REA_KEY='reai_…'   # an integration key with orders.write`
  const auth = '-H "Authorization: Bearer $REA_KEY"'
  const bot = '-H "Authorization: Bearer $REA_BOT_KEY"'
  return [
    { key: 'orders-setup', title: 'Set up', description: 'An integration key with orders.write for your storefront or ERP.', code: setup },
    {
      key: 'orders-submit',
      title: 'Send a customer order',
      description: 'Lines resolve by catalog variant_id or product_id, else by SKU. Sending the same external_id again returns the same order (200); different contents are refused (409).',
      code:
        `curl -fsS ${auth} -H 'Content-Type: application/json' "$REA_API/orders" -d '{\n` +
        `  "external_id": "1001", "currency": "EUR", "total": "39.80",\n` +
        `  "customer": {"name": "Ada Lovelace", "email": "ada@example.org"},\n` +
        `  "shipping_address": {"name": "Ada Lovelace", "line1": "12 Example Street", "city": "London", "postal_code": "N1 7AA", "country": "GB"},\n` +
        `  "lines": [{"sku": "MUG-RED-L", "quantity": 2, "unit_price": "19.90"}]}'`,
    },
    {
      key: 'orders-status',
      title: 'Read its status and tracking',
      description: 'By REA ID or by your external_id. A key sees only the orders it submitted.',
      code: `curl -fsS ${auth} "$REA_API/orders/by-external-id/1001"`,
    },
    {
      key: 'orders-automation',
      title: 'Report from your purchasing automation',
      description:
        'Optional, for suppliers set to automated placement. After a person confirms a supplier order, REA sends supplier_order.confirmed to your webhook. Your automation places it through your official channel and reports back with a key that has supplier_orders.report.',
      code:
        `curl -fsS ${bot} "$REA_API/supplier-orders?queue=to_place"\n` +
        `curl -fsS ${bot} -H 'Content-Type: application/json' "$REA_API/supplier-orders/SUPPLIER_ORDER_ID/placed" -d '{"supplier_order_number": "A-778812", "actual_cost": "11.20", "currency": "EUR"}'\n` +
        `curl -fsS ${bot} -H 'Content-Type: application/json' "$REA_API/supplier-orders/SUPPLIER_ORDER_ID/failed" -d '{"reason": "The listing is no longer available"}'\n` +
        `curl -fsS ${bot} -H 'Content-Type: application/json' "$REA_API/supplier-orders/SUPPLIER_ORDER_ID/shipment" -d '{"carrier": "DHL", "tracking_number": "JD014600006281", "tracking_url": "https://www.dhl.com/track?id=JD014600006281"}'`,
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
