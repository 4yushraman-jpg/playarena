# PlayArena — Project State & Handoff Document

**Last Updated:** 2026-06-01  
**Build status:** `go build ./...` passing, `go vet ./...` clean  
**Migrations applied:** 000001 – 000018  
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
7. [Phase Implementation Notes](#7-phase-implementation-notes)
8. [Outstanding Work](#8-outstanding-work)
9. [Recommended Development Roadmap](#9-recommended-development-roadmap)
10. [Next Recommended Phase](#10-next-recommended-phase)

---

## 1. Current Architecture

### Pattern

**Modular Monolith.** All domain modules compile into a single binary. Each module owns its handler, service, repository, model, and DTO files. Modules are wired together in `internal/bootstrap/` — the single composition root — rather than through dependency injection containers or service locators.

### Directory Layout

```
backend/
├── cmd/api/main.go                         Entry point — config, DB pool, HTTP server, graceful shutdown
├── db/
│   ├── migrations/                         golang-migrate files (000001–000017, up + down)
│   ├── queries/                            Hand-written SQL (sqlc source)
│   └── sqlc/                              Generated type-safe Go — never edited by hand
├── internal/
│   ├── auth/                               Auth domain (fully implemented)
│   │   ├── authorization.go               AuthorizationService — HasRole(), HasPermission()
│   │   ├── errors.go                      Typed domain error sentinels
│   │   ├── tokens.go                      JWT generation + validation + refresh token helpers
│   │   ├── passwords.go                   bcrypt helpers (cost 12)
│   │   ├── model.go                       AuthUser, JWTClaims, OrgSummary
│   │   ├── dto.go                         Request/response structs with validate tags
│   │   ├── repository.go                  DB access layer (wraps sqlc Queries + pgxpool)
│   │   ├── service.go                     Business logic — login, refresh, logout, register, verify, me
│   │   ├── middleware.go                  RequireAuth(), RequireRole(), RequirePermission() chi middleware
│   │   ├── handler.go                     HTTP handlers + error-to-status mapping + logging
│   │   └── routes.go                      RegisterRoutes() — mounts /api/v1/auth subtree
│   ├── health/                             Health check (fully implemented)
│   ├── organizations/                      Organizations domain (fully implemented)
│   │   ├── errors.go
│   │   ├── dto.go
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── players/                            Players domain (fully implemented)
│   │   ├── errors.go
│   │   ├── dto.go
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── teams/                              Teams + memberships domain (fully implemented)
│   │   ├── errors.go
│   │   ├── dto.go
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── tournaments/                        Tournaments domain (fully implemented)
│   │   ├── errors.go
│   │   ├── dto.go
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── tournament_registrations/           Tournament registrations domain (fully implemented)
│   │   ├── errors.go
│   │   ├── dto.go
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── bootstrap/
│   │   ├── app.go                         App struct (Config, DB, Log) — composition root
│   │   ├── router.go                      Builds chi router + global middleware stack
│   │   └── modules.go                     Wires all domain modules into router
│   └── platform/
│       ├── config/config.go               ENV-based config with validation
│       ├── database/postgres.go           pgxpool factory with production defaults
│       ├── logger/logger.go               slog (JSON in prod, text in dev)
│       ├── middleware/logging.go          Per-request structured logging
│       ├── pgutil/pgutil.go               Shared PostgreSQL helpers (UUID parse/format, unique violation check)
│       ├── response/response.go           JSON write helpers
│       └── validator/validator.go         DecodeJSON — body decode + struct-tag validation
```

**Remaining stubs** (files exist with package declaration only, no logic):
`users/`, `matches/`, `media/`, `rankings/`, `news/`

### Request Lifecycle

```
HTTP request
  → chi.RequestID   (X-Request-ID header)
  → chi.RealIP      (populates RemoteAddr from X-Forwarded-For)
  → chi.Recoverer   (panic → 500)
  → RequestLogger   (structured slog per-request logging)
  → [RequireAuth]   (on protected routes only — JWT validation → context)
  → [RequirePermission] (on write routes — DB-backed permission check)
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
| 000016 | Email Verification Tokens | `email_verification_tokens` — single-use, SHA-256 hashed |
| 000017 | Seed RBAC | 18 permissions, 7 system roles, full role→permission mappings |
| 000018 | Match Delete Permission | `match.delete` permission; granted to `platform_admin`, `org_owner`, `org_admin` |

### Table Summary

#### Identity & Auth

**`users`** — Platform-level identity. Not org-scoped. One account per person.  
Key columns: `id`, `email` (unique), `username` (unique), `password_hash` (bcrypt), `status` (`user_status` ENUM), `email_verified_at`, `last_login_at`, `last_login_ip`.

**`refresh_tokens`** — Revocation store for refresh tokens. Stores SHA-256 hash only, never the raw token.  
Key columns: `token_hash` (unique), `expires_at`, `revoked_at` (NULL = valid), `user_id` (CASCADE), `ip_address`, `user_agent`.

**`email_verification_tokens`** — Single-use email verification tokens. Stores SHA-256 hash only. Valid when `used_at IS NULL AND expires_at > NOW()`.

#### RBAC

**`permissions`** — Atomic capability definitions. `slug` format: `<resource>.<action>` (e.g. `tournament.create`). Immutable at runtime. 18 permissions seeded by migration 000017.

**`roles`** — Named permission groups. `scope` is `platform` | `organization` | `tournament`. Platform roles have `organization_id = NULL`; org roles have a non-NULL FK. `is_system` flags protect seed roles from deletion. 7 system roles seeded: `platform_admin`, `org_owner`, `org_admin`, `team_manager`, `coach`, `scorer`, `viewer`.

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
2. Verify bcrypt password (dummy comparison on miss — prevents timing-based enumeration)
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

### RBAC Authorization (`RequirePermission` middleware)

```
RequirePermission(authz, "resource.action")

1. GetAuthUser(ctx) → *AuthUser (already validated by RequireAuth)
2. HasPermission(ctx, userID, organizationID, permSlug):
     - Parses UUIDs; invalid IDs return (false, nil) — treated as unauthorized
     - Single EXISTS query across uor → roles → role_permissions → permissions
     - Evaluates both org-scoped grants AND platform-level grants (org_id IS NULL)
3. If false → 403 "insufficient permissions"
4. If DB error → 500 "internal server error"
```

### Password Security

- bcrypt cost factor: **12** (intentionally high for brute-force resistance)
- Minimum password length: 8 characters (validated at HTTP layer)
- Passwords are never logged, never returned in API responses
- Dummy bcrypt comparison always performed on email-not-found to prevent timing-based enumeration

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
6. **BOLA protection in every write service.** `assertOrgOwnership(actorOrgID, targetOrgID)` is called in every mutating service method. Actor's JWT `organization_id` must match the URL org or be empty (platform admin).

### Platform Admin Path

Users with `role_scope = 'platform'` are granted a role via `user_organization_roles` with `organization_id = NULL` (enabled by migration 000015). During login they receive an access token with `organization_id = ""`. The `AuthUser.IsPlatformUser()` method tests for this state. Platform admins bypass org-scoped authorization checks because `assertOrgOwnership("", targetID)` unconditionally returns nil.

### Access Token Org Binding

Each access token carries **exactly one** `organization_id`. The token is org-context-specific. Multi-org users must choose an org at login or refresh. The refresh token is org-agnostic: on each refresh the client can request a token for a different org (subject to the user holding an active role in that org).

---

## 5. Modules Implemented

### Fully Implemented

| Module | Key Files | Status |
|--------|-----------|--------|
| **Auth** | `errors.go`, `tokens.go`, `passwords.go`, `model.go`, `dto.go`, `repository.go`, `service.go`, `middleware.go`, `authorization.go`, `handler.go`, `routes.go` | Complete |
| **Health** | `handler.go`, `routes.go` | Complete |
| **Organizations** | `errors.go`, `dto.go`, `model.go`, `repository.go`, `service.go`, `handler.go`, `routes.go` | Complete |
| **Players** | `errors.go`, `dto.go`, `model.go`, `repository.go`, `service.go`, `handler.go`, `routes.go` | Complete |
| **Teams** | `errors.go`, `dto.go`, `model.go`, `repository.go`, `service.go`, `handler.go`, `routes.go` | Complete |
| **Tournaments** | `errors.go`, `dto.go`, `model.go`, `repository.go`, `service.go`, `handler.go`, `routes.go` | Complete |
| **Tournament Registrations** | `errors.go`, `dto.go`, `model.go`, `repository.go`, `service.go`, `handler.go`, `routes.go` | Complete |
| **Platform / Config** | `config.go` | Complete |
| **Platform / Database** | `postgres.go` | Complete |
| **Platform / Logger** | `logger.go` | Complete |
| **Platform / Middleware** | `logging.go` | Complete |
| **Platform / PgUtil** | `pgutil.go` | Complete |
| **Platform / Response** | `response.go` | Complete |
| **Platform / Validator** | `validator.go` | Complete |
| **Bootstrap** | `app.go`, `router.go`, `modules.go` | Complete |
| **Entry Point** | `cmd/api/main.go` | Complete |

### Empty Stubs (package declaration only, no logic)

`users/`, `matches/`, `media/`, `rankings/`, `news/`

---

## 6. Endpoints Implemented

### Auth (`/api/v1/auth`)

| Method | Path | Auth | Description |
|--------|------|:----:|-------------|
| `POST` | `/api/v1/auth/register` | No | Create account; stores pending_verification; returns verification token (dev only) |
| `GET` | `/api/v1/auth/verify-email` | No | Consume single-use token; transition status → active |
| `POST` | `/api/v1/auth/login` | No | Authenticate; supports multi-org selection |
| `POST` | `/api/v1/auth/refresh` | No | Rotate refresh token; issue new access + refresh tokens |
| `POST` | `/api/v1/auth/logout` | No | Revoke refresh token by value |
| `GET` | `/api/v1/auth/me` | Yes | Return profile + role + org context for authenticated user |

### Organizations (`/api/v1/organizations`)

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/api/v1/organizations` | Yes | `organization.create` | Create org; auto-grants creator `org_owner` role |
| `GET` | `/api/v1/organizations` | Yes | — | List orgs (paginated; `?limit`, `?offset`, `?search`) |
| `GET` | `/api/v1/organizations/{slug}` | Yes | — | Get org by slug |
| `PATCH` | `/api/v1/organizations/{slug}` | Yes | `organization.update` | Partial update; BOLA-guarded |
| `DELETE` | `/api/v1/organizations/{slug}` | Yes | `organization.delete` | Hard delete with cascade; BOLA-guarded |

### Players (`/api/v1/organizations/{slug}/players`)

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/players` | Yes | `player.create` | Create player profile; BOLA-guarded |
| `GET` | `/players` | Yes | — | List players (paginated; `?limit`, `?offset`, `?status`, `?search`) |
| `GET` | `/players/{id}` | Yes | — | Get player by UUID; returns inactive players for historical access |
| `PATCH` | `/players/{id}` | Yes | `player.update` | Partial update; BOLA-guarded |
| `DELETE` | `/players/{id}` | Yes | `player.delete` | Soft delete — sets `status = inactive`; BOLA-guarded |

### Teams (`/api/v1/organizations/{slug}/teams`)

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/teams` | Yes | `team.create` | Create team with auto-generated slug; BOLA-guarded |
| `GET` | `/teams` | Yes | — | List teams (paginated; `?limit`, `?offset`, `?status`, `?search`) |
| `GET` | `/teams/{id}` | Yes | — | Get team by UUID; returns disbanded teams for historical access |
| `PATCH` | `/teams/{id}` | Yes | `team.update` | Partial update; BOLA-guarded |
| `DELETE` | `/teams/{id}` | Yes | `team.delete` | Soft delete — sets `status = disbanded`; BOLA-guarded |
| `POST` | `/teams/{id}/members` | Yes | `team.update` | Add player to team; enforces org match + empty-team check |
| `GET` | `/teams/{id}/members` | Yes | — | List active team members |
| `DELETE` | `/teams/{id}/members/{membershipId}` | Yes | `team.update` | Soft-remove — sets `status = released`, stamps `left_at` |

### Tournaments (`/api/v1/organizations/{slug}/tournaments`)

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/tournaments` | Yes | `tournament.create` | Create tournament in draft status; BOLA-guarded |
| `GET` | `/tournaments` | Yes | — | List non-cancelled tournaments (paginated; `?status`, `?search`) |
| `GET` | `/tournaments/{id}` | Yes | — | Get by UUID; returns cancelled tournaments for historical access |
| `PATCH` | `/tournaments/{id}` | Yes | `tournament.update` | Partial update including status transitions; BOLA-guarded |
| `DELETE` | `/tournaments/{id}` | Yes | `tournament.delete` | Soft cancel — sets `status = cancelled`; BOLA-guarded |

### Tournament Registrations (`/api/v1/organizations/{slug}/tournaments/{tournamentId}/registrations`)

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/registrations` | Yes | `tournament.update` | Register team; enforces all 7 eligibility rules; BOLA-guarded |
| `GET` | `/registrations` | Yes | — | List registrations (paginated; `?limit`, `?offset`, `?status`) |
| `GET` | `/registrations/{registrationId}` | Yes | — | Get registration by UUID |
| `PATCH` | `/registrations/{registrationId}` | Yes | `tournament.update` | Update status/notes/seed; validates transitions; BOLA-guarded |
| `DELETE` | `/registrations/{registrationId}` | Yes | `tournament.update` | Soft-withdraw — sets `status = withdrawn`; BOLA-guarded |

### Health

| Method | Path | Auth | Description |
|--------|------|:----:|-------------|
| `GET` | `/api/v1/health` | No | DB connectivity check; returns `{"status":"ok","database":"connected"}` |

---

## 7. Phase Implementation Notes

### Phase 4 — Registration, Email Verification & RBAC (Complete)

- `POST /api/v1/auth/register` implemented. User created with `status = pending_verification`. Verification token returned in response body for development; in production this field should be gated behind `IsDevelopment()`.
- `GET /api/v1/auth/verify-email?token=...` consumes the single-use hashed token; transitions user status to `active`.
- Registration and verification are wrapped in a single transaction: no orphaned accounts that can never be verified.
- Migration 000016 adds `email_verification_tokens` table.
- Migration 000017 seeds 18 permissions, 7 system roles, and all role→permission mappings required by the RBAC layer.
- `RequireAuth()`, `RequireRole()`, and `RequirePermission()` chi middleware implemented in `internal/auth/middleware.go`.
- `AuthorizationService` (`authorization.go`) provides `HasRole()` and `HasPermission()` backed by a single EXISTS query — no N+1.

### Phase 5 — Organizations (Complete)

- Full CRUD for organization entities.
- Slug auto-generated from name; unique within the platform; retry loop up to 10 attempts on collision.
- `CreateWithOwnerGrant` transaction atomically inserts the org, looks up the `org_owner` system role, and grants it to the creator.
- BOLA protection: `assertOrgOwnership(actorOrgID, targetOrgID)` compares JWT org context against the URL org before any mutation. Platform admins (empty `organizationID`) are unconditionally allowed.
- Audit logging: `create` / `update` / `delete` audit records written transactionally with each mutation. `new_data` always derived from the actual DB-returned row, not from request input.
- Pagination: `ListOrganizationsPaginated` with `page_limit` / `page_offset`; service caps at `MaxListLimit = 200`, defaults to 50.
- Phase 5.1 hardening verified by focused review: all BOLA, pagination, audit, and authorization checks PASS.

### Phase 6A — Players (Complete)

- Full CRUD for player profiles scoped to an organization.
- Soft delete: `DELETE` sets `status = inactive`. No hard deletes.
- `GetByID` **intentionally returns inactive players** with `"status": "inactive"`. This is by design: player records must remain resolvable indefinitely for team membership history, match event references, and audit logs. This behavior is documented in `service.go:GetByID`, `repository.go:GetByID`, `handler.go:GetByID`, and `dto.go:Response.Status`.
- Pagination and search: `ListPlayersPaginated` with optional `status` filter and `display_name ILIKE` search. Inactive players excluded from default listings.
- Multi-tenant isolation: all queries scope by `organization_id`. Verified by focused review (all 7 checks PASS).

### Phase 6B — Teams & Team Memberships (Complete)

- Full CRUD for team entities. Soft delete sets `status = disbanded`.
- Slug auto-generated from team name; unique within the organization (`uq_teams_org_slug`).
- **Team membership endpoints** (`/teams/{id}/members`): add player, list active members, remove player.
- **Cross-org membership prevention** enforced at two layers:
  1. Service: `GetTeamByID(teamUID, org.ID)` and `GetPlayerByID(playerID, org.ID)` both scope by org; a team or player from another org returns `ErrCrossOrgRegistration` before any write.
  2. Database: trigger `trg_team_memberships_org_consistency` independently validates org consistency on every INSERT.
- **One active team membership per player per organization** enforced by `GetActiveMembershipByPlayer` (no `team_id` filter), which checks for any active membership on any team within the org before creating a new one. Returns `ErrPlayerAlreadyAssigned` (HTTP 409).
- **Membership removal is soft-only**: sets `status = released` and stamps `left_at = NOW()`. Historical rows are permanently preserved.
- Membership audit logging: `create` and `delete` audit records written transactionally for every add/remove. `EntityType = "team_memberships"`.

### Phase 7A — Tournaments (Complete)

- Full CRUD for tournament entities.
- Slug auto-generated from name; unique within the organization.
- **Lifecycle status transition validation** enforced in service layer:
  ```
  draft → registration_open
  registration_open → registration_closed
  registration_closed → ongoing
  ongoing → completed
  any → cancelled (terminal)
  ```
  Invalid transitions return `ErrInvalidStatusTransition` (HTTP 422).
- **Date range validation** applied on Create and Update:
  - `registration_opens_at < registration_closes_at`
  - `registration_closes_at ≤ starts_at`
  - `starts_at ≤ ends_at`
- Soft delete: `DELETE` sets `status = cancelled`. `GetByID` returns cancelled tournaments for historical reference (match brackets, registrations).
- `prize_pool` exposed as a decimal string (`*string`) in the API to preserve numeric precision; serialized from `pgtype.Numeric` via `Value()`.

### Phase 7B — Tournament Registrations (Complete)

- Team registration for tournaments. Phase 7B implements team-based registrations; the schema also supports player-based (future).
- **Seven business rules enforced in service layer** before any DB write:
  1. Tournament and team must belong to the URL org (cross-org registration returns HTTP 422).
  2. Tournament must be in `registration_open` status (HTTP 422).
  3. Current time must satisfy `registration_opens_at ≤ now() ≤ registration_closes_at` (HTTP 422).
  4. Team may not register twice for the same tournament (HTTP 409 `ErrAlreadyRegistered`).
  5. Team must exist and be `active` (HTTP 422).
  6. Team must have at least one active member (HTTP 422 `ErrEmptyTeam`).
  7. Tournament must not have reached `max_participants` capacity (HTTP 409 `ErrTournamentFull`).
- **Registration lifecycle transitions** validated in service:
  ```
  pending → approved | rejected | withdrawn
  approved → withdrawn | disqualified
  rejected, withdrawn, disqualified → (terminal)
  ```
  Transitioning to `approved` automatically stamps `approved_by` and `approved_at`.
- Soft delete: `DELETE` sets `status = withdrawn`. Records never hard-deleted.
- Audit logging: `create` / `update` / `delete` records with `EntityType = "tournament_registrations"`.

### Concurrency Hardening — Registration Capacity

A code review identified a TOCTOU (Time of Check to Time of Use) race condition in the original capacity enforcement:

**Problem:** `CountActiveRegistrations()` ran outside the write transaction. Concurrent requests could both observe count < `max_participants`, both decide "capacity available", and both insert — exceeding the limit.

**Fix applied:**

- A new SQL query `LockTournamentForUpdate` was added: `SELECT id FROM tournaments WHERE id = $1 FOR UPDATE`.
- Capacity enforcement was moved **inside** `CreateWithAudit`, executed under this exclusive row lock.
- The full sequence inside the transaction is now:
  1. `SELECT … FOR UPDATE` on the tournament row — all concurrent registrations for the same tournament serialize here.
  2. `COUNT(*)` active registrations — race-free because no other transaction can commit a new registration while the lock is held.
  3. Reject with `ErrTournamentFull` if count ≥ `max_participants`.
  4. `INSERT` the registration.
  5. `INSERT` the audit record.
  6. `COMMIT` — releases the tournament lock.
- The pre-transaction capacity check was removed from `service.go:Register` entirely.
- Code comment added: *"Capacity enforcement is performed under a tournament row lock to prevent concurrent over-registration."*
- **Review status: PASS.** Concurrent requests for the same tournament now block at the `FOR UPDATE` step and see the updated count before deciding whether to insert.

---

## 8. Outstanding Work

### Completed since initial document

- [x] **User registration endpoint** — `POST /api/v1/auth/register`
- [x] **Email verification flow** — `GET /api/v1/auth/verify-email?token=...`
- [x] **Role / permission seeding** — migration 000017 seeds all system roles and permissions
- [x] **RBAC middleware** — `RequireRole()` and `RequirePermission()` implemented and wired
- [x] **Organizations module** — Phase 5
- [x] **Players module** — Phase 6A
- [x] **Teams module + memberships** — Phase 6B
- [x] **Tournaments module** — Phase 7A
- [x] **Tournament registrations module** — Phase 7B
- [x] **Audit logging** — wired into all organization, player, team, tournament, and registration mutations
- [x] **BOLA protection** — enforced in every write service across all implemented modules

### Must-have before first production deployment

- [ ] **Password reset flow** — `POST /api/v1/auth/forgot-password` / `POST /api/v1/auth/reset-password`
- [ ] **Refresh token cleanup job** — `DeleteExpiredRefreshTokens` is generated and correct but never called. Without it the `refresh_tokens` table grows unboundedly.
- [ ] **Email verification token cleanup job** — `DeleteExpiredVerificationTokens` is implemented on the service but never called on a schedule.
- [ ] **CORS configuration** — `internal/platform/middleware/cors.go` is a stub. Browsers will block cross-origin requests.
- [ ] **Rate limiting** — `internal/platform/middleware/ratelimit.go` is a stub. Auth endpoints are unprotected against brute-force attacks.
- [ ] **Remove `verification_token` from register response in production** — currently returned for development convenience; must be gated behind `IsDevelopment()` before deployment.

### Required for feature completeness

- [ ] **Users module** — User management: list, get, update profile, change password, deactivate.
- [ ] **Matches module** — Fixture scheduling, status transitions (scheduled → live → completed), walkover handling.
- [ ] **Match scoring** — `match_events` INSERT pipeline; sequence-number generation with row-level locking; score computation from event log.
- [ ] **Rankings module** — Computed standings; depends on match and tournament modules.
- [ ] **Media module** — File upload coordination, `media_attachments` CRUD, storage backend integration.
- [ ] **News module** — Stub exists; no business logic.

### Technical debt

- [ ] **`golang-jwt/jwt/v5` declared `indirect` in `go.mod`.** Running `go mod tidy` will correct this.
- [ ] **No test files exist anywhere.** Zero tests across the entire project. Integration tests against a real PostgreSQL instance (testcontainers or Docker Compose) and unit tests for the auth service, validator, and tenant-isolation rules are the minimum needed before production.
- [ ] **`internal/platform/middleware/auth.go`** contains only a placeholder comment. Can be removed or used to re-export `auth.RequireAuth`.
- [ ] **`internal/bootstrap/database.go`** is a stub. Can be removed or used for DB-level bootstrap helpers.
- [ ] **`internal/platform/cache/redis.go`** is a stub. No Redis dependency in `go.mod`. Delete if Redis is not planned.
- [ ] **`user_organization_roles.expires_at` expiry enforcement** — background job to revoke grants past their `expires_at`; only the query filter prevents expired grants from functioning, but rows are never cleaned up.
- [ ] **Validator limitation for pointer types** — `internal/platform/validator/validator.go` uses `fmt.Sprintf("%v", ...)` for field extraction; `omitempty` does not fire on nil `*string` fields. Validation of optional fields is handled in service layers as a workaround.

---

## 9. Recommended Development Roadmap

### Phase Status Summary

| Phase | Description | Status |
|-------|-------------|--------|
| Phase 1 | Foundation, DB schema, migrations | **COMPLETE** |
| Phase 2 | Auth — login, refresh, JWT middleware | **COMPLETE** |
| Phase 3 | Auth hardening — token rotation, timing defenses | **COMPLETE** |
| Phase 4 | Registration, email verification, RBAC | **COMPLETE** |
| Phase 5 | Organizations module | **COMPLETE** |
| Phase 6A | Players module | **COMPLETE** |
| Phase 6B | Teams module + team memberships | **COMPLETE** |
| Phase 7A | Tournaments module | **COMPLETE** |
| Phase 7B | Tournament registrations | **COMPLETE** |
| Phase 7C | RBAC correction — `match.delete` permission | **COMPLETE** |
| Phase 8A | Matches | NOT STARTED |
| Phase 8B | Match Events & Live Scoring | NOT STARTED |
| Phase 9 | Rankings & Standings | NOT STARTED |
| Phase 10 | Media | NOT STARTED |
| Phase 11 | Notifications | NOT STARTED |
| Phase 12 | Hardening, Observability & Tests | NOT STARTED |

---

### Phase 8A — Matches (next)

**Goal:** tournament organizers can schedule fixtures; matches follow a defined status lifecycle.

1. **Match CRUD** — create fixtures within a tournament; link to teams/players from registrations. `organization_id` is denormalized from `tournaments.organization_id` and validated by the existing `trg_matches_org_consistency` trigger.
2. **Status transitions** — `scheduled → live → completed`; also `cancelled`, `postponed`, `abandoned`, `walkover`.
3. **Walkover handling** — set `winner_team_id` / `winner_player_id` without match events; validate `chk_matches_walkover_has_winner`.
4. **Bracket slot support** — TBD fixtures (home/away participant columns nullable); fill in as bracket progresses.

---

### Phase 8B — Match Events & Live Scoring

**Goal:** scorers can record events in real time; scores are derived from the immutable event log.

1. **`match_events` INSERT pipeline** — `POST /api/v1/matches/{id}/events`. Requires row-level lock (`SELECT … FOR UPDATE`) on the parent `matches` row to safely compute `MAX(sequence_number) + 1` under concurrent scorers — same pattern as the registration capacity fix.
2. **Score computation** — aggregate `match_events` to derive live scores, player stats. No denormalized score columns exist; all derived on read.
3. **Score corrections** — `score_correction` event with `cancels_event_id`. Effective event log excludes events whose `id` appears as any `cancels_event_id` within the same match.
4. **WebSocket / SSE push** (optional) — push live score updates to spectators.

---

### Phase 9 — Rankings & Standings

**Goal:** org dashboards show computed team and player standings.

1. **Rankings computation** — derive standings from completed match results per tournament format.
2. **Cache strategy** — rankings are expensive to compute; cache aggressively; invalidate on match completion.

---

### Phase 10 — Media

1. **`media_attachments` CRUD** — finalize polymorphic attachment management; wire to object storage backend (S3, GCS, or local).
2. **Primary attachment swaps** — atomic swap using a single UPDATE; `is_primary` uniqueness enforced at application layer.

---

### Phase 11 — Notifications

*Not designed. Depends on match events and tournament status changes.*

---

### Phase 12 — Hardening, Observability & Tests

1. **Test suite** — integration tests using `testcontainers-go` against a real PostgreSQL 17 instance. Priority: auth service, multi-tenant isolation, tournament registration rules, match event pipeline.
2. **OpenTelemetry tracing** — instrument repository and service layers.
3. **Prometheus metrics** — request latency histograms, DB pool stats, active sessions counter.
4. **Password reset flow** — `POST /auth/forgot-password` / `POST /auth/reset-password`.
5. **Rate limiting & CORS** — implement stubs before production deployment.
6. **`go mod tidy`** — move `golang-jwt/jwt/v5` from `indirect` to direct.

---

## 10. Next Recommended Phase

**Phase 8A — Matches**

All prerequisite modules are complete: organizations, players, teams, team memberships, tournaments, and tournament registrations. The `matches` table exists in the schema with all FK constraints, triggers, and indexes already in place. The next natural step is implementing the Matches module so tournament organizers can schedule and track fixtures, which is also a prerequisite for Phase 8B (live scoring) and Phase 9 (rankings).

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
| `backend/internal/auth/middleware.go` | `RequireAuth()`, `RequireRole()`, `RequirePermission()` — gates for protected routes |
| `backend/internal/auth/authorization.go` | `AuthorizationService` — DB-backed permission checks |
| `backend/internal/organizations/service.go` | BOLA reference implementation (`assertOrgOwnership`) |
| `backend/internal/tournament_registrations/repository.go` | Reference for row-lock capacity enforcement under concurrency |
| `backend/internal/bootstrap/modules.go` | Single place to register new domain modules |
| `backend/internal/platform/pgutil/pgutil.go` | Shared UUID and constraint helpers used by all domain repositories |
| `backend/internal/platform/validator/validator.go` | JSON decode + struct-tag validation (no external deps) |
| `backend/sqlc.yaml` | sqlc configuration |
| `backend/go.mod` | Module definition and direct dependencies |

---

*This document was last updated on 2026-06-01. It should be updated whenever a phase is completed or significant architectural changes are made.*
