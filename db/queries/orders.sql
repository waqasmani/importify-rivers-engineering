-- ---- customer orders ----

-- name: CreateOrder :one
-- Returns no row when an order with the same external ID was already received (idempotency).
INSERT INTO orders (channel, integration_key_id, created_by, source_label, external_id, request_hash, customer_name,
  customer_email, customer_phone, ship_name, ship_company, ship_line1, ship_line2, ship_city, ship_region,
  ship_postal_code, ship_country, ship_phone, currency, subtotal, shipping_amount, tax_amount, total, customer_note, ordered_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1;

-- name: LockOrder :one
SELECT * FROM orders WHERE id = $1 FOR UPDATE;

-- name: GetOrderByKeyExternalID :one
SELECT * FROM orders WHERE channel = 'api' AND integration_key_id = $1 AND external_id = $2;

-- name: GetManualOrderByExternalID :one
SELECT * FROM orders WHERE channel = 'manual' AND external_id = $1 AND external_id <> '';

-- name: ListOrders :many
SELECT sqlc.embed(o),
  (SELECT count(*) FROM order_lines l WHERE l.order_id = o.id AND l.cancelled_at IS NULL)::bigint AS line_count,
  (SELECT count(*) FROM order_lines l WHERE l.order_id = o.id AND l.cancelled_at IS NULL AND l.supplier_order_id IS NULL)::bigint AS unassigned_count,
  (SELECT count(*) FROM supplier_orders so WHERE so.order_id = o.id AND so.status <> 'cancelled')::bigint AS supplier_order_count,
  k.name AS integration_key_name,
  count(*) OVER () AS total
FROM orders o LEFT JOIN integration_keys k ON k.id = o.integration_key_id
WHERE (sqlc.narg('key_id')::uuid IS NULL OR o.integration_key_id = sqlc.narg('key_id'))
  AND (sqlc.narg('status')::text IS NULL OR o.status = sqlc.narg('status'))
  AND (sqlc.narg('needs_attention')::boolean IS NULL OR o.needs_attention = sqlc.narg('needs_attention'))
  AND (sqlc.narg('channel')::text IS NULL OR o.channel = sqlc.narg('channel'))
  AND (sqlc.narg('q')::text IS NULL OR o.number ILIKE '%' || sqlc.narg('q') || '%' OR o.external_id ILIKE '%' || sqlc.narg('q') || '%')
  AND (sqlc.narg('ordered_from')::timestamptz IS NULL OR o.ordered_at >= sqlc.narg('ordered_from'))
  AND (sqlc.narg('ordered_to')::timestamptz IS NULL OR o.ordered_at < sqlc.narg('ordered_to'))
ORDER BY o.ordered_at DESC, o.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: SetOrderStatus :exec
UPDATE orders SET status = $2, needs_attention = $3 WHERE id = $1;

-- name: CancelOrder :one
UPDATE orders SET cancelled_at = now(), cancelled_by = sqlc.narg('cancelled_by'), cancelled_by_label = sqlc.arg(cancelled_by_label),
  cancel_reason = sqlc.arg(cancel_reason)
WHERE id = sqlc.arg(id) AND cancelled_at IS NULL RETURNING *;

-- name: EraseOrderCustomer :one
UPDATE orders SET customer_name = sqlc.arg(marker)::text, customer_email = '', customer_phone = '',
  ship_name = CASE WHEN ship_name = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_company = CASE WHEN ship_company = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_line1 = CASE WHEN ship_line1 = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_line2 = CASE WHEN ship_line2 = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_city = CASE WHEN ship_city = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_region = CASE WHEN ship_region = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_postal_code = CASE WHEN ship_postal_code = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_phone = CASE WHEN ship_phone = '' THEN '' ELSE sqlc.arg(marker)::text END,
  customer_note = CASE WHEN customer_note = '' THEN '' ELSE sqlc.arg(marker)::text END,
  cancel_reason = CASE WHEN cancel_reason = '' THEN '' ELSE sqlc.arg(marker)::text END,
  customer_erased_at = now(), customer_erased_by = sqlc.narg('erased_by')
WHERE id = sqlc.arg(id) RETURNING *;

-- name: CountOtherOpenOrdersWithExternalID :one
-- Orders with the same external order ID received another way (another key, or by hand).
SELECT count(*) FROM orders
WHERE external_id = sqlc.arg(external_id) AND external_id <> '' AND id <> sqlc.arg(id) AND cancelled_at IS NULL;

-- ---- order lines ----

-- name: CreateOrderLine :one
INSERT INTO order_lines (order_id, position, external_line_id, sku, title, quantity, unit_price, requested_product_id,
  requested_variant_id, product_id, variant_id, resolution, resolution_note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: ListOrderLines :many
SELECT * FROM order_lines WHERE order_id = $1 ORDER BY position;

-- name: SetOrderLineResolution :exec
UPDATE order_lines SET product_id = sqlc.narg('product_id'), variant_id = sqlc.narg('variant_id'), resolution = sqlc.arg(resolution),
  resolution_note = sqlc.arg(resolution_note)
WHERE id = sqlc.arg(id);

-- name: SetOrderLineSnapshot :exec
UPDATE order_lines SET supplier_id = sqlc.narg('supplier_id'), supplier_name = sqlc.arg(supplier_name), source_domain = sqlc.arg(source_domain),
  source_url = sqlc.narg('source_url'), product_title = sqlc.arg(product_title), variant_title = sqlc.arg(variant_title),
  variant_options = sqlc.arg(variant_options), expected_unit_cost = sqlc.narg('expected_unit_cost'),
  cost_currency = sqlc.narg('cost_currency'), prepared_at = now(), supplier_order_id = sqlc.narg('supplier_order_id')
WHERE id = sqlc.arg(id);

-- name: SetOrderLineQuantity :exec
UPDATE order_lines SET quantity = $2 WHERE id = $1;

-- name: CancelOrderLine :exec
UPDATE order_lines SET cancelled_at = now(), supplier_order_id = NULL WHERE id = $1 AND cancelled_at IS NULL;

-- name: ReleaseSupplierOrderLines :exec
-- A cancelled supplier order gives its lines back; they need attention until mapped, prepared again or cancelled.
UPDATE order_lines SET supplier_order_id = NULL WHERE supplier_order_id = $1;

-- ---- supplier orders ----

-- name: CreateSupplierOrder :one
INSERT INTO supplier_orders (order_id, supplier_id, supplier_name, source_domain, group_key, placement_method, items,
  expected_cost_total, cost_currency, ship_name, ship_company, ship_line1, ship_line2, ship_city, ship_region,
  ship_postal_code, ship_country, ship_phone)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING *;

-- name: UpdateDraftSupplierOrderItems :exec
UPDATE supplier_orders SET supplier_id = sqlc.narg('supplier_id'), supplier_name = sqlc.arg(supplier_name), source_domain = sqlc.arg(source_domain),
  placement_method = sqlc.arg(placement_method), items = sqlc.arg(items), expected_cost_total = sqlc.narg('expected_cost_total'),
  cost_currency = sqlc.narg('cost_currency')
WHERE id = sqlc.arg(id) AND status = 'draft';

-- name: DeleteDraftSupplierOrder :exec
DELETE FROM supplier_orders WHERE id = $1 AND status = 'draft';

-- name: GetSupplierOrder :one
SELECT * FROM supplier_orders WHERE id = $1;

-- name: LockSupplierOrder :one
SELECT * FROM supplier_orders WHERE id = $1 FOR UPDATE;

-- name: ListSupplierOrdersForOrder :many
SELECT * FROM supplier_orders WHERE order_id = $1 ORDER BY created_at, number;

-- name: ListSupplierOrders :many
SELECT sqlc.embed(so), o.number AS order_number, o.external_id AS order_external_id, o.currency AS order_currency,
  count(*) OVER () AS total
FROM supplier_orders so JOIN orders o ON o.id = so.order_id
WHERE (sqlc.narg('status')::text IS NULL OR so.status = sqlc.narg('status'))
  AND (sqlc.narg('statuses')::text[] IS NULL OR so.status = ANY(sqlc.narg('statuses')::text[]))
  AND (sqlc.narg('placement_method')::text IS NULL OR so.placement_method = sqlc.narg('placement_method'))
  AND (sqlc.narg('supplier_id')::uuid IS NULL OR so.supplier_id = sqlc.narg('supplier_id'))
  AND (sqlc.narg('order_id')::uuid IS NULL OR so.order_id = sqlc.narg('order_id'))
  AND (sqlc.narg('q')::text IS NULL OR so.number ILIKE '%' || sqlc.narg('q') || '%' OR o.number ILIKE '%' || sqlc.narg('q') || '%'
       OR so.supplier_name ILIKE '%' || sqlc.narg('q') || '%' OR so.supplier_order_number ILIKE '%' || sqlc.narg('q') || '%')
ORDER BY CASE WHEN so.status = 'draft' THEN so.created_at END ASC, so.updated_at DESC, so.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountSupplierOrderQueues :one
SELECT count(*) FILTER (WHERE status = 'draft')::bigint AS to_confirm,
  count(*) FILTER (WHERE status = 'confirmed')::bigint AS to_place,
  count(*) FILTER (WHERE status = 'placed')::bigint AS awaiting_tracking,
  count(*) FILTER (WHERE status = 'failed')::bigint AS failed
FROM supplier_orders;

-- name: SetDraftSupplierOrderDetails :one
UPDATE supplier_orders SET ship_name = $2, ship_company = $3, ship_line1 = $4, ship_line2 = $5, ship_city = $6, ship_region = $7,
  ship_postal_code = $8, ship_country = $9, ship_phone = $10, note_to_supplier = $11
WHERE id = $1 AND status = 'draft' RETURNING *;

-- name: ConfirmSupplierOrder :one
UPDATE supplier_orders SET status = 'confirmed', content_hash = sqlc.arg(content_hash), confirmed_by = sqlc.arg(confirmed_by),
  confirmed_by_label = sqlc.arg(confirmed_by_label), confirmed_at = now(),
  attempt = CASE WHEN placement_method = 'automation' THEN 1 ELSE 0 END, failure_reason = '', failed_at = NULL
WHERE id = sqlc.arg(id) AND status = 'draft' RETURNING *;

-- name: RevertSupplierOrderToDraft :one
UPDATE supplier_orders SET status = 'draft', content_hash = '', confirmed_by = NULL, confirmed_by_label = '', confirmed_at = NULL,
  failure_reason = '', failed_at = NULL
WHERE id = $1 AND status IN ('confirmed', 'failed') RETURNING *;

-- name: MarkSupplierOrderPlaced :one
UPDATE supplier_orders SET status = 'placed', supplier_order_number = sqlc.arg(supplier_order_number), actual_cost = sqlc.narg('actual_cost'),
  actual_currency = sqlc.narg('actual_currency'), placement_note = sqlc.arg(placement_note), placed_by = sqlc.narg('placed_by'),
  placed_by_key_id = sqlc.narg('placed_by_key_id'), placed_by_label = sqlc.arg(placed_by_label), placed_at = now()
WHERE id = sqlc.arg(id) AND status = 'confirmed' RETURNING *;

-- name: MarkSupplierOrderFailed :one
UPDATE supplier_orders SET status = 'failed', failure_reason = sqlc.arg(failure_reason), failed_at = now()
WHERE id = sqlc.arg(id) AND status = 'confirmed' RETURNING *;

-- name: RetrySupplierOrder :one
UPDATE supplier_orders SET status = 'confirmed', attempt = CASE WHEN placement_method = 'automation' THEN attempt + 1 ELSE attempt END
WHERE id = $1 AND status = 'failed' RETURNING *;

-- name: SwitchSupplierOrderToManual :one
UPDATE supplier_orders SET status = 'confirmed', placement_method = 'manual'
WHERE id = $1 AND status = 'failed' AND placement_method = 'automation' RETURNING *;

-- name: SetSupplierOrderShipment :one
UPDATE supplier_orders SET status = 'shipped', carrier = sqlc.arg(carrier), tracking_number = sqlc.arg(tracking_number),
  tracking_url = sqlc.narg('tracking_url'), shipped_at = coalesce(shipped_at, now())
WHERE id = sqlc.arg(id) AND status IN ('placed', 'shipped') RETURNING *;

-- name: MarkSupplierOrderDelivered :one
UPDATE supplier_orders SET status = 'delivered', delivered_at = now() WHERE id = $1 AND status = 'shipped' RETURNING *;

-- name: CancelSupplierOrder :one
UPDATE supplier_orders SET status = 'cancelled', cancelled_by = sqlc.narg('cancelled_by'), cancelled_by_label = sqlc.arg(cancelled_by_label),
  cancelled_at = now(), cancel_reason = sqlc.arg(cancel_reason)
WHERE id = sqlc.arg(id) AND status IN ('draft', 'confirmed', 'failed', 'placed', 'shipped') RETURNING *;

-- name: SetDraftPlacementForSupplier :many
-- Drafts follow their supplier's placement method until they are confirmed.
UPDATE supplier_orders SET placement_method = $2 WHERE supplier_id = $1 AND status = 'draft' AND placement_method <> $2 RETURNING order_id;

-- name: EraseSupplierOrdersCustomer :exec
UPDATE supplier_orders SET
  ship_name = CASE WHEN ship_name = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_company = CASE WHEN ship_company = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_line1 = CASE WHEN ship_line1 = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_line2 = CASE WHEN ship_line2 = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_city = CASE WHEN ship_city = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_region = CASE WHEN ship_region = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_postal_code = CASE WHEN ship_postal_code = '' THEN '' ELSE sqlc.arg(marker)::text END,
  ship_phone = CASE WHEN ship_phone = '' THEN '' ELSE sqlc.arg(marker)::text END,
  note_to_supplier = CASE WHEN note_to_supplier = '' THEN '' ELSE sqlc.arg(marker)::text END,
  placement_note = CASE WHEN placement_note = '' THEN '' ELSE sqlc.arg(marker)::text END,
  failure_reason = CASE WHEN failure_reason = '' THEN '' ELSE sqlc.arg(marker)::text END,
  cancel_reason = CASE WHEN cancel_reason = '' THEN '' ELSE sqlc.arg(marker)::text END
WHERE order_id = sqlc.arg(order_id);

-- ---- products for preparation ----

-- name: OrderProductsByIDs :many
SELECT p.id, p.title, p.sku, p.status, p.review_status, p.availability, p.supplier_id, p.source_domain, p.source_url,
  p.canonical_url, p.cost, p.cost_currency, s.name AS supplier_name, s.order_placement AS supplier_order_placement,
  (SELECT count(*) FROM product_variants v WHERE v.product_id = p.id)::bigint AS variant_count
FROM products p LEFT JOIN suppliers s ON s.id = p.supplier_id
WHERE p.id = ANY(sqlc.arg(ids)::uuid[]);

-- name: OrderVariantsByIDs :many
SELECT * FROM product_variants WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: FindVariantsBySKU :many
SELECT id, product_id FROM product_variants WHERE lower(sku) = lower(sqlc.arg(sku)::text) AND sku <> '' ORDER BY created_at, id LIMIT 5;

-- name: FindProductsBySKU :many
SELECT id FROM products WHERE lower(sku) = lower(sqlc.arg(sku)::text) AND sku <> '' ORDER BY created_at, id LIMIT 5;

-- ---- activity ----

-- name: InsertOrderEvent :exec
INSERT INTO order_events (order_id, supplier_order_id, kind, summary, from_status, to_status, actor_user_id,
  actor_integration_key_id, actor_label)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListOrderEvents :many
SELECT * FROM order_events WHERE order_id = $1 ORDER BY id DESC LIMIT 500;

-- ---- webhooks ----

-- name: ListWebhookEndpointsForEvent :many
SELECT * FROM webhook_endpoints WHERE enabled AND sqlc.arg(event_type)::text = ANY(events) ORDER BY created_at;

-- name: CreateOrderWebhookDelivery :one
INSERT INTO webhook_deliveries (endpoint_id, event_id, event_type, payload, max_attempts, status, order_id, supplier_order_id)
VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7)
RETURNING *;

-- name: ListOrderWebhookPayloads :many
SELECT id, payload FROM webhook_deliveries WHERE order_id = $1;

-- name: SetWebhookDeliveryPayload :exec
UPDATE webhook_deliveries SET payload = $2 WHERE id = $1;
