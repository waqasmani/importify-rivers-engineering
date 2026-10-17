# Product and security rules (must always hold)

Responsible analysis
- Collect content only through permitted access and user-initiated workflows.
- Analyze source code only when the user owns it or is authorized; every source records the
  authorization confirmation and basis.
- Never extract credentials, cookies, access tokens or passwords from target pages.
- Never access private endpoints without authorization; never bypass authentication, paywalls,
  rate limits or other access restrictions (server fetches stop on any HTTP error status).
- Never run arbitrary commands or execute uploaded code.
- Keep verified evidence, inferences and unknowns separate; never claim knowledge of hidden
  implementation from a visible website or screenshot.
- Label AI-generated content as generated, with provider and model.

Product
- Never silently publish products, submit purchases, send messages or take other irreversible
  external actions. Approval only changes REA's own review status; drafts are never sent.
- Everything works without AI. External AI providers need administrator permission and a
  per-request user confirmation naming the host and data categories.
- Self-hosted only: no SaaS billing, vendor login, cloud account or telemetry.

Security
- Every endpoint: authentication, authorization, validation, rate limits.
- Secrets only in environment variables; never in source control or logs (audit redaction).
- Uploads size-limited and validated; archives protected against traversal and bombs; SSRF
  blocked at dial time.
- Argon2id passwords, HttpOnly/SameSite/Secure cookies with CSRF tokens, least-privilege roles,
  strict CORS, security headers, audit events for state changes, user data export and deletion,
  revocable extension access.

Engineering
- Do not fabricate test results or mark incomplete features as done.
- No TODO comments, mock APIs, placeholder buttons or static demo data in shipped code.
