# PlayArena — Project State & Handoff Document

**Date:** 2026-05-31  
**Build status:** `go build ./...` passing, `go vet ./...` clean  
**Migrations applied:** 000001 – 000015  
**Go version:** 1.25.6  
**Database:** PostgreSQL 17

---

## Table of Contents

1. [Current Architecture](#1-current-architecture)
2. [Database Schema Overview](#2-database-schema-overview)
3. [Authentication Architecture](#3-authentication-architecture)
4. [Multi-Tenant Design](#4-multi-tenant-design)
5. [Modules Implemented](#5-modules-implemented)
6. [Endpoints Implemented](#6-endpoints-implemented)
7. [Outstanding Work](#7-outstanding-work)
8. [Recommended Development Roadmap](#8-recommended-development-roadmap)

---

## 1. Current Architecture

### Pattern

**Modular Monolith.** All domain modules compile into a single binary. Each module owns its handler, service, repository, model, and DTO files. Modules are wired together in `internal/bootstrap/` — the single composition root — rather than through dependency injection containers or service locators.

### Directory Layout

```
backend/
├── cmd/api/main.go                  Entry point — config, DB pool, HTTP server, graceful shutdown
├── db/
│   ├── migrations/                  golang-migrate files (000001–000015, up + down)
│   ├── queries/                     Hand-written SQL (sqlc source)
│   └── sqlc/                        Generated type-safe Go — never edited by hand
├── internal/
│   ├── auth/                        Auth domain (fully implemented)
│   │   ├── errors.go                Typed domain error sentinels
│   │   ├── tokens.go                JWT generation + validation + refresh token helpers
│   │   ├── passwords.go             bcrypt helpers (cost 12)
│   │   ├── model.go                 AuthUser, JWTClaims, OrgSummary
│   │   ├── dto.go                   Request/response structs with validate tags
│   │   ├── repository.go            DB access layer (wraps sqlc Queries + pgxpool)
│   │   ├── service.go               Business logic — login, refresh, logout, me
│   │   ├── middleware.go            RequireAuth() chi middleware + GetAuthUser() helper
│   │   ├── handler.go               HTTP handlers + error-to-status mapping + logging
│   │   └── routes.go                RegisterRoutes() — mounts /api/v1/auth subtree
│   ├── health/                      Health check (fully implemented)
│   ├── bootstrap/
│   │   ├── app.go                   App struct (Config, DB, Log) — composition root
│   │   ├── router.go                Builds chi router + global middleware stack
│   │   └── modules.go               Wires domain modules into router
│   └── platform/
│       ├── config/config.go         ENV-based config with validation
│       ├── database/postgres.go     pgxpool factory with production defaults
│       ├── logger/logger.go         slog (JSON in prod, text in dev)
│       ├── middleware/logging.go    Per-request structured logging
│       ├── response/response.go     JSON write helpers
│       └── validator/validator.go   DecodeJSON — body decode + struct-tag validation
```

All other domain directories (`users/`, `organizations/`, `teams/`, `players/`, `tournaments/`, `matches/`, `media/`, `rankings/`, `news/`) exist as **empty stubs** with placeholder files.

### Request Lifecycle

```
HTTP request
  → chi.RequestID   (X-Request-ID header)
  → chi.RealIP      (populates RemoteAddr from X-Forwarded-For)
  → chi.Recoverer   (panic → 500)
  → RequestLogger   (structured slog per-request logging)
  → [RequireAuth]   (on protected routes only — JWT validation → context)
  → Handler         (decode + validate → service → response)
  → Service         (business logic — no HTTP types)
  → Repository      (SQL via sqlc Queries, transactions via pgxpool)
  → PostgreSQL 17
```

### Technology Stack

| Component | Choice | Version |
|-----------|--------|---------|
| Language | Go | 1.25.6 |
| HTTP Router | chi | v5.2.0 |
| Database | PostgreSQL | 17 |
| Driver | pgx/v5 | v5.7.1 |
| Code generation | sqlc | v1.31.1 |
| Migrations | golang-migrate | — |
| JWT | golang-jwt/jwt | v5.3.1 |
| Password hashing | bcrypt (x/crypto) | cost 12 |
| Config / .env | godotenv | v1.5.1 |
| Logging | log/slog | stdlib |

---

## 2. Database Schema Overview

### Migration History

| # | Migration | Key Tables / Changes |
|---|-----------|----------------------|
| 000001 | Extensions & ENUMs | `pgcrypto`, 15 domain ENUMs |
| 000002 | Organizations | `organizations` — multi-tenant root |
| 000003 | Users & Refresh Tokens | `users`, `refresh_tokens` |
| 000004 | RBAC | `permissions`, `roles`, `role_permissions`, `user_organization_roles` |
| 000005 | Players | `players` |
| 000006 | Teams | `teams` |
| 000007 | Team Memberships | `team_memberships` |
| 000008 | Tournaments | `tournaments` |
| 000009 | Tournament Registrations | `tournament_registrations` |
| 000010 | Matches | `matches` |
| 000011 | Match Events | `match_events` (append-only event log) |
| 000012 | Media Attachments | `media_attachments` (polymorphic) |
| 000013 | Audit Logs | `audit_logs` (immutable ledger) |
| 000014 | Schema Hardening | FK ON DELETE fixes, 4 cross-tenant triggers, 8 indexes, 1 NULL-safety partial index |
| 000015 | Auth Hardening | `user_organization_roles.organization_id` made NULLable; partial unique index for platform grants |

### Table Summary

#### Identity & Auth

**`users`** — Platform-level identity. Not org-scoped. One account per person.  
Key columns: `id`, `email` (unique), `username` (unique), `password_hash` (bcrypt), `status` (`user_status` ENUM), `email_verified_at`, `last_login_at`, `last_login_ip`.

**`refresh_tokens`** — Revocation store for refresh tokens. Stores SHA-256 hash only, never the raw token.  
Key columns: `token_hash` (unique), `expires_at`, `revoked_at` (NULL = valid), `user_id` (CASCADE), `ip_address`, `user_agent`.

#### RBAC

**`permissions`** — Atomic capability definitions. `slug` format: `<resource>.<action>` (e.g. `tournament.create`). Immutable at runtime.

**`roles`** — Named permission groups. `scope` is `platform` | `organization` | `tournament`. Platform roles have `organization_id = NULL`; org roles have a non-NULL FK. `is_system` flags protect seed roles from deletion.

**`role_permissions`** — M:M join. Cascade both sides.

**`user_organization_roles`** — Grants a user a role in a specific org context.  
`organization_id` is **NULLable** (since migration 000015) to allow platform-scoped grants.  
Supports `expires_at` for time-limited grants (e.g. guest scorer per tournament).  
Unique constraints: `(user_id, organization_id, role_id)` for org grants; partial unique index `(user_id, role_id) WHERE organization_id IS NULL` for platform grants.

#### Domain Tables

**`organizations`** — Multi-tenant root. `slug` is unique and immutable. `type`: club / federation / school / corporate / independent. `settings` (JSONB) for per-org feature flags.

**`players`** — Athletic profile scoped to an org. Decoupled from users: a user can have N player profiles across orgs; historical players need no platform account. Links to user via optional `user_id`.

**`teams`** — Org-scoped. Unique `(organization_id, slug)`. `disbanded` status preserved for match history integrity.

**`team_memberships`** — Player ↔ team history. No unique on `(team_id, player_id)` — players can rejoin, each stint is a new row. `organization_id` denormalized and validated by trigger `trg_team_memberships_org_consistency`.

**`tournaments`** — Hosted by an org. `sport` is free-text (not ENUM) for extensibility. `format`: league / knockout / group_knockout / round_robin / double_elimination. One-way status progression: draft → registration_open → registration_closed → ongoing → completed. `settings` (JSONB) for format-specific config.

**`tournament_registrations`** — Team or player entry. `organization_id` is the **registrant's** org (not the tournament host org — cross-org tournaments are supported). Trigger `trg_treg_participant_org_consistency` validates that the team/player belongs to the registrant org.

**`matches`** — Fixtures within a tournament. `organization_id` denormalized from `tournaments.organization_id`, validated by trigger. Supports TBD bracket slots (all participant columns nullable). `winner_team_id` / `winner_player_id` are final-state only.

**`match_events`** — **Append-only immutable event log.** The single source of truth for all scoring, player state, and match statistics. No UPDATE or DELETE ever. Corrections expressed as new `score_correction` events with `cancels_event_id`. `sequence_number` is monotonically increasing per match (requires row-level lock on the parent match during concurrent inserts).

**`media_attachments`** — Polymorphic media store. `(entity_type, entity_id)` soft-FK to any domain entity. Referential integrity enforced at application layer.

**`audit_logs`** — Immutable compliance ledger. `org_id` nullable (NULL = platform action). `user_id` nullable (NULL = system action). Constraint: login/logout rows have no `entity_id`; create/update/delete must have one.

### Key Design Decisions

**Event sourcing for match scoring.** No score columns exist on the `matches` table. All statistics are derived by aggregating `match_events`. Corrections are non-destructive: a `score_correction` event references (via `cancels_event_id`) the event it supersedes; neither row is mutated.

**Denormalization with trigger guards.** `matches.organization_id` and `match_events.organization_id` are denormalized for query performance. Database triggers (`trg_matches_org_consistency`, `trg_match_events_org_consistency`) enforce consistency on INSERT/UPDATE.

**Cross-org tournament registrations.** `tournament_registrations.organization_id` is the registrant's org, not the tournament host org. A federation tournament can accept teams from multiple clubs. The registrant's team/player must still belong to the registrant's org (validated by trigger).

**Soft foreign keys in media.** `media_attachments` uses a polymorphic `(entity_type, entity_id)` reference. No DB-level FK is possible. The application service layer is responsible for orphan cleanup when parent entities are deleted.

---

## 3. Authentication Architecture

### Token Model

| Token | Format | Lifetime | Storage |
|-------|--------|----------|---------|
| Access token | Signed HS256 JWT | 15 minutes | Client only (never persisted) |
| Refresh token | 32 random bytes, base64url-encoded | 7 days | SHA-256 hash stored in `refresh_tokens` table |

**Access token claims** (`JWTClaims`):

```json
{
  "iss": "playarena",
  "sub": "<user_uuid>",
  "exp": <unix>,
  "iat": <unix>,
  "nbf": <unix>,
  "user_id": "<user_uuid>",
  "organization_id": "<org_uuid | empty string for platform tokens>",
  "role": "<role_slug>",
  "email": "<user_email>"
}
```

### Login Flow

```
POST /api/v1/auth/login
  { email, password, organization_id? }

1. Fetch user by email
2. Verify bcrypt password
3. Assert user status is active (blocks: suspended, inactive, pending_verification)
4. resolveOrgContext():
     a. If organization_id provided → validate user has a role in that org
     b. If no org provided → check platform roles first (organization_id IS NULL grants)
     c. If no platform roles → list user's orgs
        - 0 orgs:  return ErrOrganizationRequired (empty list)
        - 1 org:   auto-select
        - N orgs:  return ErrOrganizationRequired (with org list; HTTP 409)
5. GenerateAccessToken() → HS256 JWT with org + role embedded
6. GenerateRefreshToken() → 32 random bytes base64url
7. SHA-256 hash refresh token, INSERT into refresh_tokens
8. Return { access_token, refresh_token, expires_in: 900, token_type: "Bearer" }
```

### Refresh Flow (with token rotation)

```
POST /api/v1/auth/refresh
  { refresh_token, organization_id? }

1. SHA-256 hash the incoming token
2. GetRefreshTokenByHash() (read-only peek for user_id)
3. GetUserByID() → re-validate user status
4. resolveOrgContext() (client may request different org context on refresh)
5. Generate new refresh token raw value
6. RotateRefreshToken() — serializable transaction:
     a. SELECT ... FOR UPDATE on the token row (prevents concurrent rotation)
     b. If revoked_at IS NOT NULL → replay detected:
          RevokeUserRefreshTokens() (wipe all active sessions)
          COMMIT; return ErrTokenReuse
     c. If expires_at < NOW() → return ErrExpiredToken
     d. RevokeRefreshToken() (old token)
     e. CreateRefreshToken() (new token)
     f. COMMIT
7. GenerateAccessToken() with new org + role context
8. Return { access_token, refresh_token, expires_in: 900, token_type: "Bearer" }
```

### JWT Validation (`RequireAuth` middleware)

```
Authorization: Bearer <token>

1. Extract token from header (must be exactly "Bearer <token>")
2. jwt.ParseWithClaims():
     - WithIssuer("playarena")       — rejects tokens from other services
     - WithValidMethods(["HS256"])   — rejects RS256/HS512 algorithm confusion
     - WithExpirationRequired()      — rejects tokens without exp claim
     - Signature verification        — rejects tampered tokens
3. Validate custom claims: user_id and email must be non-empty
4. Build *AuthUser, store in request context
5. On any failure → 401 "authorization required" (no detail leaked)
```

### Password Security

- bcrypt cost factor: **12** (intentionally high for brute-force resistance)
- Minimum password length: 8 characters (validated at HTTP layer)
- Passwords are never logged, never returned in API responses

### Error Model

All errors are typed sentinels in `internal/auth/errors.go`:

| Error | HTTP Status | Meaning |
|-------|------------|---------|
| `ErrInvalidCredentials` | 401 | Wrong email or password |
| `ErrUserSuspended` | 403 | Account administratively blocked |
| `ErrUserInactive` | 403 | Account deactivated |
| `ErrUserPendingVerification` | 403 | Email not yet verified |
| `ErrInvalidToken` | 401 | Malformed or unsigned token |
| `ErrExpiredToken` | 401 | Token past `exp` |
| `ErrRevokedToken` | 401 | Explicitly revoked (logout) |
| `ErrTokenReuse` | 401 | Replay detected; all sessions wiped |
| `ErrInvalidAlgorithm` | 401 | Non-HS256 algorithm in JWT header |
| `ErrOrganizationRequired` (struct) | 409 | Multi-org user needs to pick an org |
| `ErrOrganizationNotFound` | 422 | Org not found or user has no role in it |
| `ErrUserNotFound` | 401 (mapped from Me handler) | Token issued for a deleted user |

---

## 4. Multi-Tenant Design

### Tenant Hierarchy

```
organizations          (tenant root — every domain entity carries organization_id)
  └── roles            (org-scoped; platform roles have organization_id = NULL)
  └── players
  └── teams
      └── team_memberships
  └── tournaments
      └── tournament_registrations   (registrant org ≠ tournament host org)
      └── matches
          └── match_events           (append-only, immutable)
  └── media_attachments
```

`users` and `refresh_tokens` are **platform-level** — they have no `organization_id`. A user's org membership is entirely expressed through `user_organization_roles`.

### Isolation Strategy

1. **FK cascade chain.** Deleting an `organizations` row cascades to all child tables.
2. **Every domain query filters by `organization_id`.** Generated queries in `db/sqlc/` always include an `org_id` parameter on tenant-scoped tables.
3. **Denormalization with trigger guards.** `matches.organization_id` and `match_events.organization_id` are copied from the parent at write time. Triggers (`trg_matches_org_consistency`, `trg_match_events_org_consistency`) reject any row where the denormalized value diverges from the parent.
4. **Cross-org registration guard.** `trg_treg_participant_org_consistency` rejects registrations where the team/player does not belong to the stated registrant org.
5. **Team membership guard.** `trg_team_memberships_org_consistency` rejects memberships where the team and player are not in the same org.

### Platform Admin Path

Users with `role_scope = 'platform'` are granted a role via `user_organization_roles` with `organization_id = NULL` (enabled by migration 000015). During login they receive an access token with `organization_id = ""`. The `AuthUser.IsPlatformUser()` method tests for this state. Future RBAC middleware will use this to bypass org-scoped authorization checks.

### Access Token Org Binding

Each access token carries **exactly one** `organization_id`. The token is org-context-specific. Multi-org users must choose an org at login or refresh. The refresh token is org-agnostic: on each refresh the client can request a token for a different org (subject to the user holding an active role in that org).

---

## 5. Modules Implemented

### Fully Implemented

| Module | Files | Status |
|--------|-------|--------|
| **Auth** | `errors.go`, `tokens.go`, `passwords.go`, `model.go`, `dto.go`, `repository.go`, `service.go`, `middleware.go`, `handler.go`, `routes.go` | Complete |
| **Health** | `handler.go`, `routes.go` | Complete |
| **Platform / Config** | `config.go` | Complete |
| **Platform / Database** | `postgres.go` | Complete |
| **Platform / Logger** | `logger.go` | Complete |
| **Platform / Middleware** | `logging.go` | Complete |
| **Platform / Response** | `response.go` | Complete |
| **Platform / Validator** | `validator.go` | Complete |
| **Bootstrap** | `app.go`, `router.go`, `modules.go` | Complete |
| **Entry Point** | `cmd/api/main.go` | Complete |

### Empty Stubs (files exist, no logic)

`users/`, `organizations/`, `teams/`, `players/`, `tournaments/`, `matches/`, `media/`, `rankings/`, `news/`

Each stub directory contains placeholder `.go` files with the correct package name but only `// TODO` comments. They compile cleanly and are registered nowhere.

---

## 6. Endpoints Implemented

### Auth (`/api/v1/auth`)

| Method | Path | Auth Required | Description |
|--------|------|:---:|-------------|
| `POST` | `/api/v1/auth/login` | No | Authenticate with email + password; supports multi-org selection |
| `POST` | `/api/v1/auth/refresh` | No | Rotate refresh token; issue new access + refresh tokens |
| `POST` | `/api/v1/auth/logout` | No | Revoke refresh token by value |
| `GET` | `/api/v1/auth/me` | Yes | Return profile + role + org context for the authenticated user |

### Health

| Method | Path | Auth Required | Description |
|--------|------|:---:|-------------|
| `GET` | `/api/v1/health` | No | DB connectivity check; returns `{"status":"ok","database":"connected"}` |

### Example Responses

**`POST /api/v1/auth/login` — success:**
```json
{
  "access_token": "<jwt>",
  "refresh_token": "<raw_token>",
  "expires_in": 900,
  "token_type": "Bearer"
}
```

**`POST /api/v1/auth/login` — multi-org user with no `organization_id` (HTTP 409):**
```json
{
  "error": "organization_id is required",
  "code": "organization_required",
  "organizations": [
    { "id": "<uuid>", "name": "Mumbai Raiders", "slug": "mumbai-raiders" },
    { "id": "<uuid>", "name": "Delhi Bulls",    "slug": "delhi-bulls" }
  ]
}
```

**`GET /api/v1/auth/me` — success:**
```json
{
  "id": "<uuid>",
  "email": "alice@example.com",
  "username": "alice",
  "full_name": "Alice Smith",
  "status": "active",
  "role": "org_owner",
  "organization_id": "<uuid>"
}
```

---

## 7. Outstanding Work

### Must-have before first production deployment

- [ ] **Email verification flow.** Users register with `status = pending_verification`. No mechanism exists to send a verification email or transition the status to `active`. Without this, self-registration is unusable.
- [ ] **User registration endpoint.** `POST /api/v1/auth/register` — create user, hash password, set `pending_verification`, trigger email.
- [ ] **Password reset flow.** `POST /api/v1/auth/forgot-password` / `POST /api/v1/auth/reset-password`.
- [ ] **Refresh token cleanup job.** `DeleteExpiredRefreshTokens` is generated and correct but never called. Needs a background goroutine or cron job; without it the `refresh_tokens` table grows unboundedly.
- [ ] **Role / permission seeding.** The `permissions` and `roles` tables are empty. System roles (`org_owner`, `scorer`, `viewer`, etc.) and their permission mappings must be seeded before any RBAC check can function.
- [ ] **CORS configuration.** `internal/platform/middleware/cors.go` is a stub. Browsers will block cross-origin requests.
- [ ] **Rate limiting.** `internal/platform/middleware/ratelimit.go` is a stub. Auth endpoints are currently unprotected against brute-force and credential-stuffing attacks.

### Required for feature completeness

- [ ] **Organizations module** — CRUD for organization entities; the tenant root of every other resource.
- [ ] **Users module** — User management: list, get, update profile, change password, deactivate.
- [ ] **Teams module** — Team CRUD, team membership management (add/remove players, transfer).
- [ ] **Players module** — Player profile CRUD, eligibility queries.
- [ ] **Tournaments module** — Tournament lifecycle management: create, publish, manage registrations, advance status.
- [ ] **Matches module** — Fixture scheduling, status transitions (scheduled → live → completed).
- [ ] **Match scoring** — `match_events` INSERT pipeline; sequence-number generation with row-level locking; score computation from event log.
- [ ] **RBAC middleware** — `RequireRole()` and `RequirePermission()` chi middleware using `GetAuthUser()` from the auth package.
- [ ] **Media module** — File upload coordination, `media_attachments` CRUD, storage backend integration.
- [ ] **Rankings module** — Computed standings; depends on match and tournament modules.
- [ ] **News module** — Stub exists; no business logic.

### Technical debt

- [ ] **`golang-jwt/jwt/v5` declared `indirect` in `go.mod`.** It is a direct dependency. Running `go mod tidy` will correct this.
- [ ] **No test files exist anywhere.** The project has zero tests. Integration tests against a real PostgreSQL instance (testcontainers or Docker Compose) and unit tests for the auth service and validator are the minimum needed before production.
- [ ] **`internal/platform/middleware/auth.go`** contains only a placeholder comment. It can be removed or updated to re-export `auth.RequireAuth` for convenience.
- [ ] **`internal/bootstrap/database.go`** is a stub. It can be removed or used to house DB-level bootstrap helpers if needed.
- [ ] **`internal/platform/cache/redis.go`** is a stub. No Redis dependency is in `go.mod`. If Redis is not planned, delete the file.

---

## 8. Recommended Development Roadmap

The following sequence minimizes blocking dependencies. Each phase can be developed in parallel within a team once the interface contracts (DTOs, service signatures) are agreed.

---

### Phase 4 — Registration & Identity (1–2 weeks)

**Goal:** users can self-register and verify email; RBAC seed data exists.

1. **Seed migrations** — `permissions` and system `roles` (`org_owner`, `scorer`, `viewer`, `platform_admin`). These must exist before any authorization check can function.
2. **`POST /api/v1/auth/register`** — create user, bcrypt password, set `pending_verification`.
3. **Email verification** — token-based (signed short-lived JWT or HMAC URL token); `GET /api/v1/auth/verify-email?token=<...>` sets `email_verified_at` and status → `active`.
4. **Password reset** — `POST /auth/forgot-password` (generates reset token, sends email); `POST /auth/reset-password` (validates token, sets new hash, revokes all refresh tokens).
5. **Refresh token cleanup job** — goroutine that calls `DeleteExpiredRefreshTokens(NOW())` on a schedule (e.g. every hour).
6. **Rate limiting** — implement `ratelimit.go`; apply to `/auth/login`, `/auth/register`, `/auth/forgot-password`.
7. **CORS** — implement `cors.go`; configure allowed origins from environment.

---

### Phase 5 — Organizations & RBAC Middleware (1 week)

**Goal:** tenants can be created; protected endpoints enforce roles.

1. **Organizations module** — `POST /api/v1/organizations`, `GET`, `PATCH`, `DELETE`. Creating an org auto-assigns the creator the `org_owner` role.
2. **`RequireRole(roles ...string)` middleware** — reads `GetAuthUser(ctx)`, checks the `role` claim. Returns 403 on mismatch.
3. **`RequirePermission(slug string)` middleware** — joins `user_organization_roles` → `role_permissions` → `permissions` to check a specific permission slug. (Can be lazy-loaded and cached per request.)
4. **User module** — `GET /api/v1/users/me` (update profile), `PATCH /api/v1/users/me/password`.

---

### Phase 6 — Teams & Players (1–2 weeks)

**Goal:** organizations can manage their rosters.

1. **Players module** — CRUD under `/api/v1/organizations/{orgSlug}/players`. Status management (active / injured / suspended / retired).
2. **Teams module** — CRUD under `/api/v1/organizations/{orgSlug}/teams`. Team membership endpoints: add player, update role, transfer, release.
3. **Media attachments** — team logo and player avatar upload (requires storage backend decision: S3, GCS, or local).

---

### Phase 7 — Tournaments (2–3 weeks)

**Goal:** organizations can host and manage complete tournaments.

1. **Tournament CRUD** — create, publish, manage settings, cancel. Status machine: draft → registration_open → registration_closed → ongoing → completed.
2. **Registration management** — submit registration (team or player), approve/reject. Cross-org registrations supported by schema.
3. **Bracket/fixture generation** — create `matches` rows for the chosen format (league table, knockout bracket, round-robin schedule). This is the most algorithmically complex step.
4. **Match status transitions** — schedule → live → completed; walkover; postpone; cancel.

---

### Phase 8 — Live Scoring (2–3 weeks)

**Goal:** scorers can record match events in real time.

1. **`match_events` INSERT pipeline** — HTTP endpoint `POST /api/v1/matches/{id}/events`. Requires row-level lock on the parent `matches` row to safely compute `MAX(sequence_number) + 1` under concurrent scorers.
2. **Score computation** — aggregate `match_events` to derive live scores, player stats. No denormalized score columns exist; all derived on read.
3. **Score corrections** — `score_correction` event with `cancels_event_id`. The effective event log excludes events whose `id` appears as any `cancels_event_id` within the same match.
4. **Websocket / SSE push** (optional at this phase) — push live score updates to spectators without polling.

---

### Phase 9 — Rankings, Media, News (1–2 weeks)

1. **Rankings** — compute team and player standings from completed match results. Cache aggressively; update on match completion.
2. **Media module** — finalize `media_attachments` CRUD; wire to storage backend; handle primary attachment swaps atomically.
3. **News module** — article CRUD scoped to an organization.

---

### Phase 10 — Hardening & Observability (ongoing)

1. **Test suite** — integration tests using `testcontainers-go` against a real PostgreSQL 17 instance. Priority: auth service, multi-tenant isolation, match event pipeline.
2. **OpenTelemetry tracing** — instrument repository and service layers.
3. **Prometheus metrics** — request latency histograms, DB pool stats, active sessions counter.
4. **Audit log writes** — wire `audit_logs` INSERT into organization/user/tournament mutation paths.
5. **`go mod tidy`** — move `golang-jwt/jwt/v5` from `indirect` to direct in `go.mod`.
6. **`user_organization_roles.expires_at` expiry enforcement** — background job to revoke grants past their `expires_at`; currently only the query filter (`expires_at > NOW()`) prevents expired grants from functioning, but the rows are never cleaned up.

---

## Appendix: Key Files Reference

| File | Purpose |
|------|---------|
| `backend/db/migrations/` | Append-only schema history; never edited after deployment |
| `backend/db/queries/*.sql` | Hand-written SQL; source for sqlc generation |
| `backend/db/sqlc/` | Generated Go — regenerate with `sqlc generate` from `backend/` |
| `backend/internal/auth/errors.go` | Canonical error sentinels for the auth domain |
| `backend/internal/auth/tokens.go` | JWT generation, validation, token hashing |
| `backend/internal/auth/service.go` | Auth business logic including multi-org resolution |
| `backend/internal/auth/middleware.go` | `RequireAuth()` — the gate for all protected routes |
| `backend/internal/bootstrap/modules.go` | Single place to register new domain modules |
| `backend/internal/platform/validator/validator.go` | JSON decode + struct-tag validation (no external deps) |
| `backend/sqlc.yaml` | sqlc configuration |
| `backend/go.mod` | Module definition and direct dependencies |

---

*This document reflects the repository state as of 2026-05-31. It should be updated whenever a phase is completed or significant architectural changes are made.*
