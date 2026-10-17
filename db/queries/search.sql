-- name: GlobalSearch :many
(SELECT 'company'::text AS entity_type, id, name AS title, coalesce(domain, '') AS subtitle, updated_at
 FROM companies WHERE rea_search_doc(name, domain, industry, description) @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR name ILIKE '%' || sqlc.arg(q)::text || '%' ORDER BY updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'contact', id, full_name, coalesce(email::text, job_title), updated_at
 FROM contacts WHERE rea_search_doc(first_name, last_name, email::text, job_title) @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR full_name ILIKE '%' || sqlc.arg(q)::text || '%' OR email::text ILIKE '%' || sqlc.arg(q)::text || '%'
 ORDER BY updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'supplier', id, name, country, updated_at
 FROM suppliers WHERE rea_search_doc(name, domain, country, description) @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR name ILIKE '%' || sqlc.arg(q)::text || '%' ORDER BY updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'product', id, title, trim(brand || ' ' || sku), updated_at
 FROM products WHERE rea_search_doc(title, brand, sku, description) @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR title ILIKE '%' || sqlc.arg(q)::text || '%' OR sku ILIKE sqlc.arg(q)::text ORDER BY updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'lead', id, title, source, updated_at
 FROM leads WHERE rea_search_doc(title, source, description, '') @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR title ILIKE '%' || sqlc.arg(q)::text || '%' ORDER BY updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'opportunity', id, name, status, updated_at
 FROM opportunities WHERE rea_search_doc(name, description, outcome_reason, '') @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR name ILIKE '%' || sqlc.arg(q)::text || '%' ORDER BY updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'task', id, title, status, updated_at
 FROM tasks WHERE rea_search_doc(title, description, '', '') @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR title ILIKE '%' || sqlc.arg(q)::text || '%' ORDER BY updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'analysis_project', id, name, status, updated_at
 FROM analysis_projects WHERE rea_search_doc(name, description, objective, '') @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR name ILIKE '%' || sqlc.arg(q)::text || '%' ORDER BY updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'finding', f.id, f.title, f.category, f.updated_at
 FROM analysis_findings f WHERE rea_search_doc(f.title, f.detail, f.evidence, f.category) @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
 ORDER BY f.updated_at DESC LIMIT sqlc.arg(per_type)::int)
UNION ALL
(SELECT 'captured_page', id, title, domain, captured_at
 FROM captured_pages WHERE rea_search_doc(title, description, domain, url) @@ websearch_to_tsquery('simple', sqlc.arg(q)::text)
   OR title ILIKE '%' || sqlc.arg(q)::text || '%' ORDER BY captured_at DESC LIMIT sqlc.arg(per_type)::int);

-- name: DashboardCounts :one
SELECT
  (SELECT count(*) FROM companies) AS companies,
  (SELECT count(*) FROM contacts) AS contacts,
  (SELECT count(*) FROM suppliers) AS suppliers,
  (SELECT count(*) FROM products) AS products,
  (SELECT count(*) FROM products WHERE review_status = 'pending') AS products_pending_review,
  (SELECT count(*) FROM product_imports WHERE created_at > now() - interval '7 days') AS imports_last_7_days,
  (SELECT count(*) FROM leads WHERE converted_at IS NULL) AS open_leads,
  (SELECT count(*) FROM opportunities WHERE status = 'open') AS open_opportunities,
  (SELECT coalesce(sum(amount), 0) FROM opportunities WHERE status = 'open')::numeric AS open_pipeline_amount,
  (SELECT count(*) FROM opportunities WHERE status = 'won' AND closed_at > now() - interval '30 days') AS won_last_30_days,
  (SELECT count(*) FROM tasks WHERE status IN ('open', 'in_progress') AND due_at < now()) AS overdue_tasks,
  (SELECT count(*) FROM tasks WHERE status IN ('open', 'in_progress') AND due_at >= now() AND due_at < now() + interval '1 day') AS tasks_due_today,
  (SELECT count(*) FROM analysis_projects WHERE status IN ('draft', 'active')) AS active_analysis_projects,
  (SELECT count(*) FROM analysis_findings) AS findings,
  (SELECT count(*) FROM captured_pages WHERE captured_at > now() - interval '7 days') AS captures_last_7_days,
  (SELECT count(*) FROM ai_jobs WHERE status IN ('queued', 'running')) AS ai_jobs_active;

-- name: ImportsPerDay :many
SELECT date_trunc('day', created_at)::date AS day, count(*) AS imports
FROM product_imports WHERE created_at > now() - make_interval(days => sqlc.arg(days)::int)
GROUP BY 1 ORDER BY 1;

-- name: RecentActivity :many
SELECT * FROM audit_logs ORDER BY id DESC LIMIT $1;
