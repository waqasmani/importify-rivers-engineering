# Progress log

## Phase 1 — Repository assessment
- The repository held only a README. Toolchain versions were verified and recorded in `docs/implementation-plan.md`.

## Phase 2 — Foundation
- Files: `go.mod`, `sqlc.yaml`, `db/migrations/0000{1..4}_*.sql`, `db/queries/*.sql`, `services/internal/{config,store,migrate,auth,secretbox,normalize,valid,ratelimit,filestore,httpapi,worker,app,testutil}`, `services/api/main.go`, `services/worker/main.go`.
- Done: full schema for all modules, embedded goose migrations, sqlc queries, argon2id passwords, cookie sessions with CSRF tokens, idle and absolute session expiry, account lockout, login rate limiting, roles and permissions, first-run setup with a one-time token written to the data directory, users and roles administration, ownership transfer, personal API tokens, settings registry, workspaces, audit log, extension pairing with rotating refresh tokens and reuse detection, CORS, security headers, structured logging, graceful shutdown, health and readiness endpoints, job worker skeleton.
- Tests: `go test ./services/...` with `REA_TEST_DATABASE_URL` set — setup, login, CSRF, content-type enforcement, CORS, permissions, lockout and rate limiting, extension pairing, refresh rotation, reuse detection and revocation, audit redaction, settings validation, password hashing, tokens, encryption, normalization.
