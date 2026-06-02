# PlayArena — Project State & Handoff Document

**Last Updated:** 2026-06-02  
**Build status:** `go build ./...` passing, `go vet ./...` clean, `sqlc generate` clean  
**Migrations applied:** 000001 – 000021  
**Go version:** 1.25.6  
**Database:** PostgreSQL 17  
**Phases complete:** 1 – 11

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
│   ├── migrations/                         golang-migrate files (000001–000021, up + down)
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
│   │   ├── dto.go                         Includes StandingsResponse, StandingsRowResponse, PointSystemResponse
│   │   ├── model.go
│   │   ├── repository.go                  Includes GetCompletedMatchesForStandings, GetRegistrationsForStandings
│   │   ├── service.go                     Includes GetStandings, parseStandingsSettings
│   │   ├── handler.go                     Includes GetStandings handler
│   │   └── routes.go                      GET /{id}/standings route added
│   ├── tournament_registrations/           Tournament registrations domain (fully implemented)
│   │   ├── errors.go
│   │   ├── dto.go
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── matches/                            Matches domain (fully implemented)
│   │   ├── errors.go                      Includes ErrWinnerScoreMismatch (Phase 10)
│   │   ├── dto.go                         Response includes home_score, away_score (Phase 10)
│   │   ├── model.go
│   │   ├── repository.go                  UpdateWithAudit performs score snapshot on completion (Phase 10)
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── match_events/                       Match Events domain (fully implemented)
│   │   ├── errors.go
│   │   ├── dto.go
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── scoring/                            Live Scoring engine (fully implemented, Phase 9)
│   │   ├── engine.go                      ScoreEngine — stateless Compute(match, events) → ScoreResult
│   │   ├── rules.go                       Kabaddi scoring rules: participantSide, allOutScore, payloadPoints
│   │   ├── models.go                      ScoreResult — the derived score response type
│   │   └── validation.go                  ValidateScoreEventPayload + ValidateAllOutParticipant (Phase 9 hardening)
│   ├── standings/                          Standings engine (fully implemented, Phase 10)
│   │   ├── models.go                      CompletedMatch, RegistrationInfo, Settings, StandingsRow
│   │   ├── engine.go                      Compute(matches, registrations, settings) → []StandingsRow
│   │   └── tiebreakers.go                 makeLess, h2hCompare, seedCompare — full 7-level tiebreak chain
│   ├── media/                              Media Management (fully implemented, Phase 11)
│   │   ├── errors.go                      Typed domain error sentinels
│   │   ├── model.go                       ListParams, VariantsMeta, size/pagination constants
│   │   ├── dto.go                         UploadRequest (multipart), UpdateRequest, Response, ListResponse
│   │   ├── repository.go                  CreateWithAudit, SwapPrimaryWithAudit, DeleteWithAudit; duplicate-detection retry
│   │   ├── service.go                     Upload pipeline, List, GetByID, Update, Delete; BOLA + entity guards
│   │   ├── handler.go                     HTTP handlers; MaxBytesReader enforcement; multipart parsing
│   │   ├── routes.go                      RegisterRoutes() — mounts /api/v1/organizations/{slug}/media; dev file server
│   │   ├── storage/
│   │   │   ├── backend.go                 Backend interface + New() factory + GenerateKey()
│   │   │   ├── local.go                   LocalBackend — filesystem storage for development
│   │   │   └── s3.go                      S3Backend — S3-compatible storage with inline SigV4 signing
│   │   └── processor/
│   │       └── image.go                   MIME detection, image decode, bilinear resize, JPEG encode, SHA-256 hash
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
`users/`, `rankings/`, `news/`

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
| Image decode (WebP) | golang.org/x/image | v0.41.0 |

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
| 000018 | Match Delete Permission | `match.delete` permission seeded; granted to `platform_admin`, `org_owner`, `org_admin` |
| 000019 | Match Score Snapshots | `matches.home_score INTEGER NOT NULL DEFAULT 0`, `matches.away_score INTEGER NOT NULL DEFAULT 0` — final score columns written once at `live → completed` transition; standings engine reads these instead of re-aggregating `match_events` |
| 000020 | Media Hardening | `media_attachments.storage_key TEXT NOT NULL` — canonical S3 object path, independent of CDN; `media_attachments.content_hash CHAR(64) NOT NULL` — SHA-256 hex for duplicate detection; `uq_media_primary_per_entity` unique partial index enforcing one primary per entity; `media.update` and `media.delete` permissions seeded and granted to `platform_admin`, `org_owner`, `org_admin`, `team_manager`, `coach` |
| 000021 | Media Content Uniqueness | `uq_media_content_per_entity` unique index on `(organization_id, entity_type, entity_id, content_hash)` — DB-level backstop preventing concurrent duplicate uploads from producing multiple rows for the same content |

### Table Summary

#### Identity & Auth

**`users`** — Platform-level identity. Not org-scoped. One account per person.  
Key columns: `id`, `email` (unique), `username` (unique), `password_hash` (bcrypt), `status` (`user_status` ENUM), `email_verified_at`, `last_login_at`, `last_login_ip`.

**`refresh_tokens`** — Revocation store for refresh tokens. Stores SHA-256 hash only, never the raw token.  
Key columns: `token_hash` (unique), `expires_at`, `revoked_at` (NULL = valid), `user_id` (CASCADE), `ip_address`, `user_agent`.

**`email_verification_tokens`** — Single-use email verification tokens. Stores SHA-256 hash only. Valid when `used_at IS NULL AND expires_at > NOW()`.

#### RBAC

**`permissions`** — Atomic capability definitions. `slug` format: `<resource>.<action>` (e.g. `tournament.create`). Immutable at runtime. 18 permissions seeded by migration 000017; `match.delete` added by migration 000018 (19 total).

**`roles`** — Named permission groups. `scope` is `platform` | `organization` | `tournament`. Platform roles have `organization_id = NULL`; org roles have a non-NULL FK. `is_system` flags protect seed roles from deletion. 7 system roles seeded: `platform_admin`, `org_owner`, `org_admin`, `team_manager`, `coach`, `scorer`, `viewer`.

**`role_permissions`** — M:M join. Cascade both sides.

**`user_organization_roles`** — Grants a user a role in a specific org context.  
`organization_id` is **NULLable** (since migration 000015) to allow platform-scoped grants.  
Supports `expires_at` for time-limited grants (e.g. guest scorer per tournament).  
Unique constraints: `(user_id, organization_id, role_id)` for org grants; partial unique index `(user_id, role_id) WHERE organization_id IS NULL` for platform grants.

#### Permission Matrix (complete, as of migration 000020)

| Permission | `platform_admin` | `org_owner` | `org_admin` | `team_manager` | `coach` | `scorer` | `viewer` |
|-----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| `organization.create` | ✓ | — | — | — | — | — | — |
| `organization.update` | ✓ | ✓ | ✓ | — | — | — | — |
| `organization.delete` | ✓ | ✓ | — | — | — | — | — |
| `user.manage` | ✓ | ✓ | ✓ | — | — | — | — |
| `role.assign` | ✓ | ✓ | ✓ | — | — | — | — |
| `team.create` | ✓ | ✓ | ✓ | ✓ | — | — | — |
| `team.update` | ✓ | ✓ | ✓ | ✓ | — | — | — |
| `team.delete` | ✓ | ✓ | ✓ | ✓ | — | — | — |
| `player.create` | ✓ | ✓ | ✓ | ✓ | — | — | — |
| `player.update` | ✓ | ✓ | ✓ | ✓ | ✓ | — | — |
| `player.delete` | ✓ | ✓ | ✓ | ✓ | — | — | — |
| `tournament.create` | ✓ | ✓ | ✓ | — | — | — | — |
| `tournament.update` | ✓ | ✓ | ✓ | — | — | — | — |
| `tournament.delete` | ✓ | ✓ | ✓ | — | — | — | — |
| `match.create` | ✓ | ✓ | ✓ | — | — | — | — |
| `match.update` | ✓ | ✓ | ✓ | — | ✓ | ✓ | — |
| `match.delete` | ✓ | ✓ | ✓ | — | — | — | — |
| `match.score` | ✓ | ✓ | ✓ | — | — | ✓ | — |
| `media.upload` | ✓ | ✓ | ✓ | ✓ | ✓ | — | — |
| `media.update` | ✓ | ✓ | ✓ | ✓ | ✓ | — | — |
| `media.delete` | ✓ | ✓ | ✓ | ✓ | ✓ | — | — |

#### Domain Tables

**`organizations`** — Multi-tenant root. `slug` is unique and immutable. `type`: club / federation / school / corporate / independent. `settings` (JSONB) for per-org feature flags.

**`players`** — Athletic profile scoped to an org. Decoupled from users: a user can have N player profiles across orgs; historical players need no platform account. Links to user via optional `user_id`.

**`teams`** — Org-scoped. Unique `(organization_id, slug)`. `disbanded` status preserved for match history integrity.

**`team_memberships`** — Player ↔ team history. No unique on `(team_id, player_id)` — players can rejoin, each stint is a new row. `organization_id` denormalized and validated by trigger `trg_team_memberships_org_consistency`.

**`tournaments`** — Hosted by an org. `sport` is free-text (not ENUM) for extensibility. `format`: league / knockout / group_knockout / round_robin / double_elimination. One-way status progression: draft → registration_open → registration_closed → ongoing → completed. `settings` (JSONB) for format-specific config.

**`tournament_registrations`** — Team or player entry. `organization_id` is the **registrant's** org (not the tournament host org — cross-org tournaments are supported). Trigger `trg_treg_participant_org_consistency` validates that the team/player belongs to the registrant org.

**`matches`** — Fixtures within a tournament. `organization_id` denormalized from `tournaments.organization_id`, validated by trigger. Supports TBD bracket slots (all participant columns nullable). `winner_team_id` / `winner_player_id` are final-state only. `home_score` / `away_score` (added migration 000019) are snapshotted once at `live → completed` inside a `FOR UPDATE` locked transaction; both are `0` for all non-completed statuses and walkovers. Standings reads these columns exclusively — never `match_events`.

**`match_events`** — **Append-only immutable event log.** The single source of truth for all scoring, player state, and match statistics. No UPDATE or DELETE ever. Corrections expressed as new `score_correction` events with `cancels_event_id`. `sequence_number` is monotonically increasing per match (requires row-level lock on the parent match during concurrent inserts).

**`media_attachments`** — Polymorphic media store. `(entity_type, entity_id)` soft-FK to any domain entity. Referential integrity enforced at application layer. `storage_key` (added migration 000020) is the canonical object path in the storage backend — source of truth independent of CDN domain. `content_hash` (added migration 000020) is the SHA-256 hex digest of raw uploaded bytes used for duplicate detection. `uq_media_primary_per_entity` partial unique index (migration 000020) enforces at most one `is_primary = TRUE` row per entity. `uq_media_content_per_entity` unique index (migration 000021) prevents concurrent duplicate uploads.

**`audit_logs`** — Immutable compliance ledger. `org_id` nullable (NULL = platform action). `user_id` nullable (NULL = system action). Constraint: login/logout rows have no `entity_id`; create/update/delete must have one.

### Key Design Decisions

**Event sourcing for live scoring.** `match_events` is the single source of truth for all scoring during active matches. The live scoring engine (`internal/scoring/`) derives home and away scores on every `GET /matches/{id}/score` request from the effective event log — no score is ever stored for live matches.

**Score snapshot at completion.** When a match transitions `live → completed`, `matches.home_score` and `matches.away_score` are written atomically inside the same transaction, under a `FOR UPDATE` lock on the match row. The lock prevents any concurrent event insertion between the score computation and the status write, making the snapshot permanently consistent with the event log. After completion, no further events can be recorded (`ErrMatchNotLive`), so the snapshot never drifts. Corrections are non-destructive: a `score_correction` event references (via `cancels_event_id`) the event it supersedes; neither row is mutated.

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
| **Matches** | `errors.go`, `dto.go`, `model.go`, `repository.go`, `service.go`, `handler.go`, `routes.go` | Complete |
| **Match Events** | `errors.go`, `dto.go`, `model.go`, `repository.go`, `service.go`, `handler.go`, `routes.go` | Complete |
| **Scoring Engine** | `engine.go`, `rules.go`, `models.go`, `validation.go` | Complete |
| **Standings Engine** | `models.go`, `engine.go`, `tiebreakers.go` | Complete |
| **Media Management** | `errors.go`, `model.go`, `dto.go`, `repository.go`, `service.go`, `handler.go`, `routes.go`; `storage/backend.go`, `storage/local.go`, `storage/s3.go`; `processor/image.go` | Complete |
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

`users/`, `rankings/`, `news/`

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
| `GET` | `/api/v1/auth/admin-only` | Yes | RBAC demonstration endpoint; requires `role.assign` permission; returns caller's principal (user_id, email, role, org_id) |

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
| `GET` | `/tournaments/{id}/standings` | Yes | — | Derive current standings from snapshotted match scores; point system from `tournaments.settings`; 7-level tiebreak chain |
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

### Matches (`/api/v1/organizations/{slug}/matches`)

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/matches` | Yes | `match.create` | Schedule match; validates tournament ongoing, participant eligibility, approved registrations; BOLA-guarded |
| `GET` | `/matches` | Yes | — | List matches (paginated; `?limit`, `?offset`, `?tournament_id`, `?status`, `?search`) |
| `GET` | `/matches/{id}` | Yes | — | Get match by UUID; returns all statuses including cancelled/completed |
| `GET` | `/matches/{id}/score` | Yes | — | Derive current score from effective event log; no persistence; valid for all match statuses. Response now includes `home_score`/`away_score` (snapshotted on completed matches). |
| `PATCH` | `/matches/{id}` | Yes | `match.update` | Partial update; validates status transitions, winner, participant changes; BOLA-guarded |
| `DELETE` | `/matches/{id}` | Yes | `match.delete` | Soft-cancel — sets `status = cancelled`; terminal-state guard; BOLA-guarded |

### Match Events (`/api/v1/organizations/{slug}/matches/{matchId}/events`)

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/events` | Yes | `match.score` | Record event; validates match live, event type, participants, corrections; acquires `FOR UPDATE` lock |
| `GET` | `/events` | Yes | — | List events in sequence order (paginated; `?limit`, `?offset`, `?effective_only`) |
| `GET` | `/events/{eventId}` | Yes | — | Get event by UUID; scoped to match and org |

No PATCH or DELETE endpoints exist. Match Events are append-only by design. Corrections are represented by inserting a `score_correction` event.

### Media (`/api/v1/organizations/{slug}/media`)

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/media` | Yes | `media.upload` | Upload image; multipart/form-data (file, entity_type, entity_id, alt_text, is_primary); MIME detection + image decode + JPEG normalize + thumbnail generation; BOLA-guarded |
| `GET` | `/media` | Yes | — | List attachments (paginated; `?entity_type`, `?entity_id`, `?limit`, `?offset`); org-scoped |
| `GET` | `/media/{id}` | Yes | — | Get attachment by UUID; org-scoped (BOLA-safe) |
| `PATCH` | `/media/{id}` | Yes | `media.update` | Update alt_text, sort_order, is_primary; primary swap is transactional (FOR UPDATE + unique-index backstop); BOLA-guarded |
| `DELETE` | `/media/{id}` | Yes | `media.delete` | Hard-delete DB row + audit log, then delete storage objects; double-delete safe (rows-affected check); BOLA-guarded |

Supported entity types (Phase 11): `organization`, `team`, `player`, `tournament`. `match` and `user` deferred.  
Allowed input MIME types: `image/jpeg`, `image/png`, `image/webp`, `image/gif`. SVG and documents rejected.  
All output stored as JPEG. Thumbnails generated at 150 px (`_sm`) and 400 px (`_md`) width.  
Storage backend: `local` (development, `./uploads/`, served at `/media/files/*`) or `s3` (production, any S3-compatible endpoint via SigV4 signing).

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

### Phase 7C — RBAC Correction (Complete)

- **Problem identified:** Migration 000017 seeded three match-resource permissions (`match.create`, `match.update`, `match.score`) but omitted `match.delete`. Without it, the DELETE endpoint had to be guarded by `match.update`, which is also granted to `coach` and `scorer`. Coaching staff and scorers could have cancelled match fixtures — an unintended privilege escalation contradicted by both role descriptions and the pattern established by every other resource.
- **Migration 000018** added the missing `match.delete` permission and granted it exclusively to `platform_admin`, `org_owner`, and `org_admin` — mirroring `tournament.delete` exactly.
- The `delete` action was already in the `chk_permissions_action` constraint vocabulary from 000017, confirming the omission was accidental. The fix is purely additive: one row in `permissions`, three rows in `role_permissions`. No existing data was touched.
- **Result:** `coach` and `scorer` retain `match.update` (in-match control) and `match.score` (event recording), but cannot schedule or cancel fixtures. The permission model is now consistent across all resources.

### Phase 8A — Matches (Complete)

- Full CRUD for match fixtures scoped to an organization.
- Soft delete: `DELETE` sets `status = cancelled`. No hard deletes.
- `GetByID` **intentionally returns cancelled and completed matches** for historical `match_events` and `audit_logs` reference integrity.

#### Files Added

| File | Purpose |
|------|---------|
| `internal/matches/errors.go` | Typed domain error sentinels |
| `internal/matches/model.go` | `ListParams`, pagination constants |
| `internal/matches/dto.go` | `CreateRequest`, `UpdateRequest`, `Response`, `ListResponse` |
| `internal/matches/repository.go` | DB access: reads, transactional writes, audit helpers |
| `internal/matches/service.go` | Business logic: all validation, lifecycle, BOLA guard |
| `internal/matches/handler.go` | HTTP handlers + error mapping + structured logging |
| `internal/matches/routes.go` | `RegisterRoutes()` — mounts `/api/v1/organizations/{slug}/matches` |

#### SQL Queries Added

**`db/queries/matches.sql`:**

| Query | Purpose |
|-------|---------|
| `CreateMatch` | INSERT fixture; org-consistency enforced by `trg_matches_org_consistency` |
| `UpdateMatch` | Full mutable-field update; includes DB-level terminal-state guard |
| `CancelMatch` | Soft-cancel to `cancelled`; includes DB-level terminal-state guard |
| `ListMatchesPaginated` | Paginated org-scoped listing with optional `tournament_id`, `status`, search filters |
| `CountMatches` | Count matching the same filters as `ListMatchesPaginated` |
| `LockTournamentForShare` | `SELECT status … FOR SHARE` — used inside transactions to prevent tournament cancellation races |

**`db/queries/tournament_registrations.sql`:**

| Query | Purpose |
|-------|---------|
| `GetApprovedRegistrationByTeam` | Verify team holds an approved registration before being assigned as participant |
| `GetApprovedRegistrationByPlayer` | Verify player holds an approved registration before being assigned as participant |

#### Match Status Lifecycle

```
scheduled → live       (stamps started_at; tournament must be ongoing)
scheduled → cancelled

live → completed       (stamps ended_at; tournament must be ongoing)
live → abandoned       (stamps ended_at; tournament must be ongoing)
live → cancelled

completed  → (terminal)
cancelled  → (terminal)
abandoned  → (terminal)
```

Attempting to update or cancel a match in a terminal status returns `ErrMatchNotUpdatable` (HTTP 422).

#### Match Validation Rules

**Participant type compatibility:**
- Team tournaments (`participant_type = team`): `home_team_id` + `away_team_id` required; player IDs forbidden.
- Individual tournaments (`participant_type = individual`): `home_player_id` + `away_player_id` required; team IDs forbidden.

**Duplicate participant protection:** `home == away` (team or player) returns `ErrDuplicateParticipants` (HTTP 422).

**Cross-org participant guard:** Participants are fetched with org scope via `GetTeamByID(id, orgID)` / `GetPlayerByID(id, orgID)`. A participant from another org is not found and returns `ErrParticipantCrossOrg` (HTTP 422).

**Registration eligibility:** Each participant must hold an `approved` registration in the tournament. Unregistered participants return `ErrParticipantNotRegistered` (HTTP 422). Checked on Create and when participant fields change during Update.

**Winner validation:** `winner_team_id` / `winner_player_id` may only be set when the resulting status is `completed`. Validation uses `params.Status` (the merged resulting state, not the raw request field). Winner must equal home or away participant. Violations return HTTP 422.

**Tournament status guard:**
- Match creation requires `tournament.status = ongoing`. Enforced pre-transaction in service and re-enforced inside the transaction under a `FOR SHARE` lock.
- Transitions to `live`, `completed`, or `abandoned` also require `tournament.status = ongoing`, enforced inside the transaction under a `FOR SHARE` lock.

#### Match Security

- **Multi-tenant isolation:** Every match query scopes by `organization_id`. `GetMatchByID`, `ListMatchesPaginated`, `CountMatches`, `UpdateMatch`, and `CancelMatch` all include `organization_id` in their WHERE clauses.
- **BOLA protection:** `assertOrgOwnership(actorOrgID, targetOrgID)` called in `Create`, `Update`, and `Delete`. Platform admins (empty `organizationID`) are unconditionally allowed.
- **Authorization:** `POST → match.create`, `PATCH → match.update`, `DELETE → match.delete`. GET endpoints require auth only.

#### Match Audit Logging

| Operation | Audit action | `old_data` | `new_data` |
|-----------|-------------|-----------|-----------|
| Create | `create` | — | DB-returned row |
| Update | `update` | Pre-update response snapshot | DB-returned row |
| Cancel | `delete` | Pre-cancel response snapshot | — |

All writes are transactional. `EntityType = "matches"`. `new_data` is always derived from the actual DB-returned row, never from request input.

### Phase 8B — Match Events (Complete)

Match Events are the canonical competition activity log and the foundation for Phase 9 Live Scoring and Phase 10 Rankings & Standings. The table is append-only and immutable: no UPDATE or DELETE is ever performed. Corrections are expressed by inserting a `score_correction` event that references the target via `cancels_event_id`.

#### Files Added

| File | Purpose |
|------|---------|
| `db/queries/match_events.sql` | 14 SQL queries for the match_events domain |
| `internal/match_events/errors.go` | 14 typed domain error sentinels |
| `internal/match_events/model.go` | `ListParams`, pagination constants |
| `internal/match_events/dto.go` | `CreateRequest`, `Response`, `ListResponse` |
| `internal/match_events/repository.go` | DB access: reads, `CreateWithAudit` transaction |
| `internal/match_events/service.go` | Business logic, validation, BOLA guard, response mapping |
| `internal/match_events/handler.go` | HTTP handlers + error mapping + structured logging |
| `internal/match_events/routes.go` | `RegisterRoutes()` — mounts `/{slug}/matches/{matchId}/events` |

#### Files Modified

| File | Change |
|------|--------|
| `db/sqlc/match_events.sql.go` | Regenerated — 14 query functions |
| `internal/bootstrap/modules.go` | Added `match_events.RegisterRoutes(...)` |

#### SQL Queries Added (`db/queries/match_events.sql`)

| Query | Purpose |
|-------|---------|
| `LockMatchForUpdate` | `SELECT … FOR UPDATE` — serialises concurrent inserts; returns status + participant fields for validation |
| `GetMaxSequenceNumber` | `COALESCE(MAX(sequence_number), 0)::bigint` — run inside lock to compute next sequence |
| `CreateMatchEvent` | INSERT RETURNING * |
| `GetMatchEventByMatchAndID` | `WHERE id = $1 AND match_id = $2 AND organization_id = $3` — enforces resource hierarchy |
| `GetMatchEventByID` | `WHERE id = $1 AND organization_id = $2` — org-scoped single event read |
| `ListMatchEventsByMatch` | Raw timeline `ORDER BY sequence_number ASC` with pagination |
| `ListEffectiveMatchEventsByMatch` | Raw timeline minus cancelled events (for score display) |
| `CountMatchEventsByMatch` | Raw count for pagination |
| `CountEffectiveMatchEventsByMatch` | Effective count for pagination |
| `CountMatchEventsByType` | Lifecycle uniqueness check inside the lock |
| `GetMatchEventForCorrection` | Fetch target event fields for correction validation |
| `GetEventCancellation` | Check whether target event is already cancelled |
| `IsPlayerOnParticipatingTeam` | `EXISTS` on `team_memberships` — team-format player validation |
| `IsPlayerOnTeam` | `EXISTS` on `team_memberships` — cross-team attribution check |

#### Event Model

The `match_event_type` ENUM defines exactly 21 recordable event types. No new types may be invented at the application layer.

| Category | Event Types |
|----------|-------------|
| Lifecycle | `match_started`, `match_ended`, `half_started`, `half_ended`, `timeout_called`, `timeout_ended` |
| Raid | `raid_attempt`, `raid_successful`, `raid_empty`, `bonus_point_awarded` |
| Tackle | `tackle_successful`, `super_tackle` |
| Compound | `super_raid`, `do_or_die_raid`, `all_out` |
| Player state | `player_out`, `player_revived`, `player_substituted`, `player_injured` |
| Administrative | `penalty_awarded`, `score_correction` |

#### Sequence Number Guarantee

`sequence_number` is never accepted from the client. It is always computed server-side inside the `FOR UPDATE` transaction using the following sequence:

```
LockMatchForUpdate(matchID, organizationID)   ← acquires row lock
  → GetMaxSequenceNumber(matchID)              ← safe under lock
  → nextSeq = max + 1
  → CreateMatchEvent(... sequence_number = nextSeq ...)
  → CreateAuditLog(...)
  → COMMIT
```

The `FOR UPDATE` lock on the match row blocks all concurrent event inserts for the same match until the current transaction commits. Two concurrent inserts for the same match cannot receive the same `sequence_number`. The `uq_match_events_sequence` UNIQUE constraint on `(match_id, sequence_number)` provides an independent DB-level backstop.

**Review status: PASS** — Verified: concurrent inserts for the same match cannot receive the same sequence_number.

#### Match State Enforcement

Events may only be recorded when `match.status = live`. All other statuses (`scheduled`, `completed`, `cancelled`, `abandoned`) return `ErrMatchNotLive` (HTTP 422). This check is performed **inside the transaction** after the `FOR UPDATE` lock is acquired — a concurrent match status transition cannot bypass it.

#### Participant Validation

| Scenario | Rule |
|----------|------|
| `team_id` provided | Must equal `home_team_id` or `away_team_id` of the match |
| `player_id` provided — individual tournament | Must equal `home_player_id` or `away_player_id` of the match |
| `player_id` provided — team tournament | Must have an active `team_memberships` row on either participating team |
| Both `team_id` and `player_id` provided | Player must have an active membership on the stated team specifically |

Cross-team player attribution is rejected (`ErrPlayerNotOnTeam`, HTTP 422).

#### Score Correction Rules

A `score_correction` event must set `cancels_event_id`. The following are validated inside the `FOR UPDATE` transaction:

1. Target event must exist (`ErrCancelsEventNotFound`)
2. Target event must belong to the same match (`ErrCancelsEventCrossMatch`)
3. Target event must not itself be a `score_correction` (`ErrCannotCancelCorrection`)
4. Target event must not already be cancelled (`ErrEventAlreadyCancelled`)

Correction chains are impossible because rule 3 prevents correcting another correction. Double cancellation is impossible because rule 4 runs under the match lock.

#### Effective Timeline

`?effective_only=true` on the List endpoint excludes events whose `id` appears in any `cancels_event_id` within the same match. The raw timeline (including cancelled events) is always available as the default view for audit and replay.

#### Match Events Authorization

| Method | Permission | Notes |
|--------|-----------|-------|
| `POST` | `match.score` | Granted to: `platform_admin`, `org_owner`, `org_admin`, `scorer` |
| `GET /events` | RequireAuth | Any authenticated user |
| `GET /events/{id}` | RequireAuth | Any authenticated user |

Coaches (`match.update`) cannot record events. Scorers (`match.score`) can record events but cannot create or cancel matches.

#### Match Events Audit Logging

Every event INSERT generates one `AuditActionCreate` record with `entity_type = "match_events"`. The audit INSERT is inside the same transaction as the event INSERT — both commit or both roll back. Score corrections generate their own `create` audit records. There are no update or delete audit records for this table.

#### Match Events Multi-Tenant Security

- All event queries scope by `organization_id`.
- `organization_id` on the event row is derived server-side from the locked match row — never from request input.
- `trg_match_events_org_consistency` trigger validates `organization_id == match.organization_id` on every INSERT.
- Cross-org event creation, reads, and corrections are all prevented by overlapping application and DB-level controls.

#### Match Events BOLA Protection

- **Write operations:** `assertOrgOwnership(actorOrgID, orgID)` called in `service.go:Create`.
- **Match validation:** `GetMatchByID(matchID, organizationID)` confirms match belongs to the URL org before opening the transaction.
- **Transaction validation:** `LockMatchForUpdate(matchID, organizationID)` re-confirms inside the transaction.
- **Platform admins** remain exempt via `assertOrgOwnership("", targetID) → nil`.

### Phase 8B Hardening — Resource Hierarchy Fix

**Problem identified during production review:** The original `GetByID` implementation fetched an event by `(event_id, organization_id)` only. A user could retrieve an event from match M_B by specifying any valid match M_A in their org in the URL — the URL's `{matchId}` was validated to exist but was never used to scope the event lookup.

**Fix applied:** A new query `GetMatchEventByMatchAndID` was added to `db/queries/match_events.sql`:

```sql
SELECT * FROM match_events
WHERE  id              = $1
  AND  match_id        = $2
  AND  organization_id = $3
LIMIT  1;
```

`repository.go:GetByID` was updated to accept a `matchID` parameter and call `GetMatchEventByMatchAndID`. `service.go:GetByID` was updated to pass `mid` to the repository. The event must now belong to the match specified in the URL, not just to the organization.

**Review status: PASS** — An event from Match B cannot be retrieved through a Match A URL.

### Phase 8B — Production Hardening Review

Adversarial review performed after implementation and hardening fix. Results:

| Check | Result |
|-------|--------|
| Multi-tenant isolation | **PASS** |
| BOLA protection | **PASS** |
| Match-state enforcement | **PASS** |
| Event-type validation | **PASS** |
| Participant validation | **PASS** |
| Correction integrity | **PASS** |
| Timeline ordering integrity | **PASS** |
| Concurrency safety | **PASS** |
| Audit logging correctness | **PASS** |
| Authorization coverage | **PASS** |
| Data integrity | **PASS** |

### Phase 9 — Live Scoring (Complete)

The live scoring engine derives match scores from the immutable `match_events` event log. No score is ever stored. Every call to the score endpoint recomputes from committed events. The engine is a pure, stateless Go function with no database access or side effects.

#### Architecture Decisions

**Source of truth:** `match_events` remains the sole source of truth for all scores. The `matches` table is not modified by Phase 9; no new columns were added. This preserves the event-sourcing design from Phase 8B.

**Derived-on-read:** Scores are computed on each `GET /matches/{id}/score` request by aggregating the effective event log. For kabaddi matches (100–300 events, indexed by `match_id`), this aggregation is sub-millisecond. No caching, no background jobs, no score persistence.

**Effective-event filtering:** Cancelled events are excluded at the SQL layer via the same `id NOT IN (SELECT cancels_event_id …)` pattern established in Phase 8B. The engine receives only effective events and processes them sequentially.

#### Scoring Package (`internal/scoring/`)

| File | Purpose |
|------|---------|
| `engine.go` | `ScoreEngine` struct; `Compute(match, events) ScoreResult` — the single public entry point |
| `rules.go` | Kabaddi scoring rules: `participantSide`, `sideByID`, `payloadPoints`, `allOutScore` |
| `models.go` | `ScoreResult` — JSON response type carrying `home_score`, `away_score`, `match_status`, `is_walkover`, and participant IDs |
| `validation.go` | `ValidateScoreEventPayload` — write-time payload validation; three exported error sentinels |

#### Kabaddi Scoring Rules (implemented)

| Event Type | Points | Attribution |
|-----------|--------|-------------|
| `raid_successful` | `payload.points` | `event.team_id` (raiding team) |
| `bonus_point_awarded` | +1 | `event.team_id` (raiding team) |
| `tackle_successful` | +1 | `event.team_id` (defending team) |
| `super_tackle` | +2 | `event.team_id` (defending team) |
| `all_out` | `payload.bonus_points` | Opponent of `payload.team_id` (eliminated team) |
| `penalty_awarded` | `payload.points` | `event.team_id` |
| `super_raid` | 0 | Analytics label only — `raid_successful` already carries the points; scoring separately would double-count |
| All other types | 0 | — |

Both team-format (keyed on `team_id`) and individual-format (keyed on `player_id`) tournaments are supported. The engine branches on `match.HomeTeamID.Valid` to select the correct attribution key.

#### Score Correction Handling

`score_correction` events are present in the effective event log (they are not targets of corrections themselves). The engine's `default:` case returns `(0, sideNone)` for them. The targeted event is excluded from the effective log by the SQL subquery — its point contribution disappears from the derived score automatically on the next read. No separate recomputation step is needed.

#### Write-Time Payload Validation

Three scoring event types require specific payload fields. These are validated at write time (in `match_events/service.go:Create`, after `parsePayload`) so malformed events never enter the immutable log:

| Event | Required Fields | Error |
|-------|----------------|-------|
| `raid_successful` | `payload.points > 0` | `ErrInvalidScorePayload` → HTTP 400 |
| `penalty_awarded` | `payload.points > 0` | `ErrInvalidScorePayload` → HTTP 400 |
| `all_out` | `payload.team_id` (non-empty) + `payload.bonus_points > 0` | `ErrInvalidScorePayload` → HTTP 400 |

The engine is fault-tolerant for pre-Phase-9 events with missing payload fields (returns 0 points for unparseable payloads rather than erroring).

#### New SQL Query

`GetEffectiveMatchEventsForScore` — added to `db/queries/match_events.sql`, regenerated into `db/sqlc/match_events.sql.go`. Returns the complete effective event timeline for a match in sequence order, with no pagination limit. Identical effective-log filter to `ListEffectiveMatchEventsByMatch` but without `LIMIT`/`OFFSET`.

#### Files Modified in Phase 9

| File | Change |
|------|--------|
| `db/queries/match_events.sql` | Added `GetEffectiveMatchEventsForScore` query |
| `db/sqlc/match_events.sql.go` | Regenerated — 15 query functions (was 14) |
| `internal/matches/repository.go` | Added `GetEffectiveEventsForScore` method |
| `internal/matches/service.go` | Added `GetScore` method; imported `internal/scoring` |
| `internal/matches/handler.go` | Added `GetScore` handler |
| `internal/matches/routes.go` | Added `GET /{id}/score` route under `RequireAuth` |
| `internal/match_events/errors.go` | Added `ErrInvalidScorePayload` sentinel |
| `internal/match_events/service.go` | Added `ValidateScoreEventPayload` call at write time |
| `internal/match_events/handler.go` | Added `ErrInvalidScorePayload` → HTTP 400 in error switch and `errKind` |

#### Phase 9 Production Review (adversarial, all PASS)

| Check | Result |
|-------|--------|
| Score correctness | **PASS** |
| Effective-event handling | **PASS** |
| score_correction handling | **PASS** |
| Payload validation | **PASS** |
| Multi-tenant isolation | **PASS** |
| BOLA protection | **PASS** |
| Authorization | **PASS** |
| Match-state enforcement | **PASS** |
| Concurrency safety | **PASS** |
| Score derivation correctness | **PASS** |
| Event-type interpretation | **PASS** |
| Super-raid double-count protection | **PASS** |
| All-out attribution correctness | **PASS** |
| Live score consistency | **PASS** |

### Phase 9 Hardening — all_out Payload Participant Validation

**Problem identified during adversarial review:** `ValidateScoreEventPayload` validated that `all_out.team_id` was a non-empty string, but did not verify it was a UUID belonging to one of the match's participants. A scorer with `match.score` permission could submit `{"team_id": "garbage", "bonus_points": 2}`, which passed write-time validation. At score-derivation time, `sideByID("garbage", match)` returned `sideNone` and the bonus points were silently credited to neither team — producing an incorrect live score.

**Fix applied:** `ValidateAllOutParticipant(payload []byte, homeTeamID, awayTeamID string) error` added to `internal/scoring/validation.go`. Called from `match_events/service.go:Create` immediately after `ValidateScoreEventPayload` when `eventType == all_out`. The function re-parses `payload.team_id` and compares it against the match's home and away participant UUIDs. Any non-matching value — including garbage strings, UUIDs from other matches, and empty strings — returns `ErrInvalidScorePayload` (HTTP 400). Walkovers in individual-format matches (where both team slots are empty) are also correctly rejected.

**Review status: PASS.** An `all_out` event with a non-participant `team_id` is now rejected at write time before entering the immutable log.

---

### Phase 10 — Tournament Standings & Rankings (Complete)

Phase 10 adds tournament standings derivation. The implementation is built on the Phase 9 scoring foundation and introduces a score snapshot that bridges the event-sourcing layer to a standings-friendly data model.

#### Architecture Principles

**match_events remains the source of truth for live scoring.** The Phase 9 scoring engine is unchanged. `GET /matches/{id}/score` continues to recompute from the effective event log on every request.

**Standings read only snapshotted scores.** The standings engine (`internal/standings/`) accepts pre-aggregated `CompletedMatch` rows. It never reads `match_events`. This separation keeps the standings query O(completed_matches) regardless of event volume.

**Score snapshot written once at completion.** The `live → completed` transition in `matches/repository.go:UpdateWithAudit` was extended to:
1. Acquire `FOR UPDATE` on the match row (blocks concurrent event inserts).
2. Fetch the full effective event log under the lock.
3. Run `scoring.NewScoreEngine().Compute()`.
4. Validate winner consistency against the computed score.
5. Write `home_score`, `away_score`, and `status = completed` atomically in one `UPDATE`.
6. Release the lock on commit.

Because no events can be appended to a non-live match (`ErrMatchNotLive` in `CreateWithAudit`) and the snapshot is taken while the match row is locked against concurrent inserts, the snapshot is permanently consistent with the event log.

**Winner consistency enforced at completion.** `validateWinnerVsScore` in `matches/repository.go` runs inside the completion transaction and enforces:
- `homeScore > awayScore` → winner must be the home participant → `ErrWinnerScoreMismatch` (HTTP 422) otherwise.
- `awayScore > homeScore` → winner must be the away participant.
- Equal scores → winner must be absent (draw).
- Walkovers are exempt: score is 0-0 by convention; winner is set administratively.

#### Migration Added

| Migration | Change |
|-----------|--------|
| `000019_match_score_snapshots` | Adds `matches.home_score INTEGER NOT NULL DEFAULT 0` and `matches.away_score INTEGER NOT NULL DEFAULT 0`. Both default to 0 and are populated exclusively during the completion transition. |

#### New Package: `internal/standings/`

A pure computation package with no DB access, no HTTP types, and no side effects.

| File | Purpose |
|------|---------|
| `models.go` | `CompletedMatch`, `RegistrationInfo`, `Settings`, `StandingsRow`, `DefaultSettings()` |
| `engine.go` | `Compute(matches, registrations, settings) []StandingsRow` — accumulates W/D/L/scores; sorts via tiebreakers; assigns positions |
| `tiebreakers.go` | `makeLess` (sort comparator), `h2hCompare` (head-to-head), `seedCompare` |

#### Standings Fields

Each `StandingsRow` contains:

| Field | Derivation |
|-------|-----------|
| `position` | Rank after full tiebreaker sort (1-indexed, always unique) |
| `participant_id` | Team or player UUID |
| `played` | Count of completed matches for this participant |
| `wins` | Count where `winner_id == participant_id` |
| `losses` | Count where the other participant won |
| `draws` | `played - wins - losses` |
| `points` | `wins×win_pts + draws×draw_pts + close_losses×close_loss_pts + losses×loss_pts` |
| `score_for` | Sum of this participant's score across completed matches |
| `score_against` | Sum of opponent's score |
| `score_difference` | `score_for - score_against` |

All approved registrants appear in standings regardless of matches played (0s across all stats for unplayed registrants).

#### Point System

Read from `tournaments.settings` JSONB. Absent fields use defaults:

| Setting key | Default | Effect |
|-------------|---------|--------|
| `win_points` | 3 | Points awarded for a win |
| `draw_points` | 1 | Points awarded for a draw |
| `loss_points` | 0 | Points awarded for a loss |
| `close_margin` | 0 (disabled) | When > 0, losses by ≤ this margin score `close_loss_points` instead |
| `close_loss_points` | 0 | Points for a close loss (only when `close_margin > 0`) |

Example kabaddi PKL settings: `{"win_points": 5, "draw_points": 3, "loss_points": 0, "close_margin": 5, "close_loss_points": 1}`.

Walkovers always receive `loss_points` — not `close_loss_points` — because a walkover is a forfeit, not a close competitive result.

#### Tiebreaker Chain

Applied by `makeLess` in `internal/standings/tiebreakers.go`. Fully deterministic — no randomness, no ties possible at level 7.

| Priority | Criterion | Direction |
|----------|-----------|-----------|
| 1 | Tournament points | DESC |
| 2 | Head-to-head (2-way ties only) | Points → Score diff → Score for |
| 3 | Score difference | DESC |
| 4 | Score for | DESC |
| 5 | Wins | DESC |
| 6 | Seed number (`tournament_registrations.seed_number`) | ASC (lower = better); unseeded ranks last |
| 7 | Registration timestamp (`tournament_registrations.registered_at`) | ASC (earlier = better); always unique |

**N-way head-to-head note:** Head-to-head is restricted to strict 2-way ties (exactly two participants share a point total). Applying h2h to 3+ participants risks a non-transitive comparator (`A beats B, B beats C, C beats A`), which can produce an unstable or non-deterministic sort. Full N-way sub-table resolution is deferred.

#### SQL Queries Added

**`db/queries/matches.sql`:**

| Query | Change |
|-------|--------|
| `UpdateMatch` | Gains `home_score` ($18) and `away_score` ($19) parameters. Non-completion transitions pass `0`; the repository fills in computed values for completions before calling this query. |
| `ListCompletedMatchesByTournament` | Returns completed matches for a tournament scoped by `organization_id` and `tournament_id`. Used exclusively by the standings service. |

**`db/queries/tournament_registrations.sql`:**

| Query | Purpose |
|-------|---------|
| `ListApprovedRegistrationsForStandings` | Returns all approved registrations for a tournament ordered by `registered_at ASC`. Scoped by `tournament_id` only (not by org — valid for cross-org tournaments). |

#### Files Modified in Phase 10

| File | Change |
|------|--------|
| `db/migrations/000019_match_score_snapshots.{up,down}.sql` | New — adds `home_score`/`away_score` columns |
| `db/queries/matches.sql` | `UpdateMatch` extended; `ListCompletedMatchesByTournament` added |
| `db/queries/tournament_registrations.sql` | `ListApprovedRegistrationsForStandings` added |
| `db/sqlc/matches.sql.go` | Regenerated — `UpdateMatchParams` gains `HomeScore`/`AwayScore`; `ListCompletedMatchesByTournamentRow` added |
| `db/sqlc/tournament_registrations.sql.go` | Regenerated — `ListApprovedRegistrationsForStandingsRow` added |
| `db/sqlc/models.go` | Regenerated — `db.Match` gains `HomeScore`/`AwayScore int32` |
| `internal/matches/errors.go` | `ErrWinnerScoreMismatch` added |
| `internal/matches/dto.go` | `Response` gains `HomeScore`/`AwayScore int32` |
| `internal/matches/repository.go` | `updateMatchTxParams` gains `isCompletion`/`isWalkover`; `UpdateWithAudit` performs lock→compute→validate→snapshot for completions; `validateWinnerVsScore` helper added; `matchToAuditJSON` includes scores; `scoring` package imported |
| `internal/matches/service.go` | Params include `HomeScore`/`AwayScore`; `isCompletion`/`isWalkover` passed to repo; `matchToResponse` includes scores |
| `internal/matches/handler.go` | `ErrWinnerScoreMismatch` → HTTP 422 in error switch and `errKind` |
| `internal/tournaments/repository.go` | `GetCompletedMatchesForStandings`/`GetRegistrationsForStandings` added |
| `internal/tournaments/service.go` | `GetStandings`, `parseStandingsSettings`, `participantID` added; `standings` package imported |
| `internal/tournaments/dto.go` | `StandingsResponse`, `StandingsRowResponse`, `PointSystemResponse` added |
| `internal/tournaments/handler.go` | `GetStandings` handler added |
| `internal/tournaments/routes.go` | `GET /{id}/standings` route registered under `RequireAuth` |

#### Phase 10 Production Hardening Review (adversarial, all PASS)

| Check | Result |
|-------|--------|
| Standings correctness (W/D/L/score/points) | **PASS** |
| Team vs individual tournament attribution | **PASS** |
| Completed-match-only filtering | **PASS** |
| Score snapshot integrity | **PASS** |
| Point system parsing and defaults | **PASS** |
| Head-to-head correctness (2-way) | **PASS** |
| N-way h2h non-transitivity safety | **PASS** |
| Tiebreaker chain determinism | **PASS** |
| Walkover handling | **PASS** |
| Multi-tenant isolation | **PASS** |
| BOLA protection | **PASS** |
| Concurrency — snapshot race | **PASS** |
| Concurrency — double completion | **PASS** |
| Concurrency — standings during completion | **PASS** |
| Performance (O(matches), no N+1) | **PASS** |

---

### Phase 11 — Media Management (Complete)

Phase 11 implements the full media attachment lifecycle: upload, list, retrieve, update, and delete. Images are validated server-side, normalized to JPEG, and stored with two thumbnail variants in a provider-agnostic object storage backend. All operations are organization-scoped, RBAC-protected, and BOLA-guarded. The module was adversarially reviewed and three correctness defects were identified and resolved before marking the phase production-ready.

#### Architecture

**StorageBackend abstraction.** `internal/media/storage.Backend` is an interface with three methods: `Upload`, `Delete`, and `GetPublicURL`. Two implementations exist:

- `LocalBackend` — writes to the local filesystem (`./uploads/` by default). Intended for development only. A static file server is mounted at `/media/files/*` when the local backend is active.
- `S3Backend` — speaks the S3 REST API directly using AWS Signature Version 4 signing implemented via Go stdlib (`crypto/hmac`, `crypto/sha256`). No vendor SDK dependency. Compatible with AWS S3, MinIO, Cloudflare R2, and any S3-compatible endpoint via `STORAGE_S3_ENDPOINT` override.

The backend is selected at startup via `STORAGE_BACKEND` env var (`local` or `s3`). The failure mode is a panic at startup — a misconfigured storage backend is a fatal configuration error, not a runtime recoverable.

`storage.GenerateKey(orgID, entityType, entityID, fileUUID, suffix)` produces all storage keys. Keys are UUID-based (`crypto/rand`), org-namespaced, and never derived from user-supplied filenames. `path.Join` normalization removes any `..` components.

**Config additions.** Nine new environment variables in `internal/platform/config/config.go`: `STORAGE_BACKEND`, `STORAGE_LOCAL_PATH`, `STORAGE_LOCAL_BASE_URL`, `STORAGE_S3_ENDPOINT`, `STORAGE_S3_REGION`, `STORAGE_S3_BUCKET`, `STORAGE_S3_ACCESS_KEY`, `STORAGE_S3_SECRET_KEY`, `STORAGE_CDN_BASE_URL`.

#### Image Processing (`internal/media/processor/image.go`)

Every upload passes through the processor before any storage write:

1. `io.ReadAll(src)` — reads the bounded body into memory (capped by `MaxBytesReader` at 10 MB + 64 KB overhead applied in the handler before any read).
2. `http.DetectContentType(raw[:512])` — magic-byte MIME detection. Never trusts the client-supplied `Content-Type` header. WebP is detected via a supplemental RIFF/WEBP magic-byte check.
3. Allowlist enforcement — rejects anything not in `{image/jpeg, image/png, image/webp, image/gif}`. SVG is excluded unconditionally.
4. SHA-256 content hash — computed from raw bytes before any transformation; used for duplicate detection and integrity.
5. `image.Decode` — full decode (not header-only). `golang.org/x/image/webp` registered as a blank import to handle WebP input. A file that passes MIME detection but fails decode is rejected. Polyglot files are stripped because output is re-encoded from the decoded pixel buffer.
6. Bilinear resize — two thumbnail variants: 150 px wide (`_sm`) and 400 px wide (`_md`), aspect-ratio-preserving, no upscaling.
7. `jpeg.Encode(quality=85)` — all variants stored as JPEG. Input format is not preserved.

Thumbnail keys are stored in `metadata.variants` JSONB: `{"full": "…", "sm": "…", "md": "…"}`.

#### Security

- **Organization scoping:** every query includes `AND organization_id = $N`. `GetByID` uses `WHERE id = $1 AND organization_id = $2` — media IDs from other orgs are invisible.
- **BOLA guard:** `assertOrgOwnership(actorOrgID, orgID)` called in every mutating service method. Platform admins (empty `organizationID`) are exempt.
- **Entity ownership verification:** before any upload, `assertEntityExists` queries the target entity with org scope. A player, team, or tournament from another org is not found and the upload is rejected. Organization-type uploads require `entity_id == orgID`.
- **RBAC:** `media.upload` gated at the route level via `RequirePermission`. `media.update` and `media.delete` gated on PATCH and DELETE respectively. GET endpoints require `RequireAuth` only. `scorer` and `viewer` have no media write permissions.

#### Concurrency

**Primary-image race** — `SwapPrimaryWithAudit` (`internal/media/repository.go`):
- `LockPrimaryMediaAttachment` acquires `SELECT … FOR UPDATE` on the current primary row before any modification. Concurrent swap requests for the same entity serialize at this lock.
- `uq_media_primary_per_entity` (partial unique index, migration 000020) is the DB-level backstop. Any code path that bypasses the application-layer lock cannot commit two rows with `is_primary = TRUE` for the same entity.

**Duplicate-upload race** — two-layer defense:
- Application layer: `GetByContentHash` before upload; if found, return existing attachment.
- DB layer: `uq_media_content_per_entity` (unique index, migration 000021) on `(organization_id, entity_type, entity_id, content_hash)`. If two concurrent requests both pass the application check, the second `INSERT` fails with SQLSTATE 23505. `CreateWithAudit` detects this via `pgutil.IsUniqueViolation` and re-queries to return the existing row. No duplicate record, no 500 error.

#### SQL Queries Added (`db/queries/media.sql`)

| Query | Purpose |
|-------|---------|
| `CreateMediaAttachment` | INSERT with RETURNING; unique-violation retry in repository |
| `UpdateMediaAttachmentMeta` | UPDATE alt_text, sort_order; scoped by id + org |
| `SetAttachmentAsPrimary` | UPDATE is_primary=TRUE; called inside swap transaction |
| `UnsetPrimaryForEntity` | UPDATE is_primary=FALSE for all entity attachments; called inside swap transaction |
| `DeleteMediaAttachment` | DELETE (:execrows — rows-affected returned for zero-row detection) |
| `GetMediaAttachmentByID` | Org-scoped single row lookup |
| `GetMediaAttachmentByContentHash` | Duplicate detection query |
| `LockPrimaryMediaAttachment` | SELECT … FOR UPDATE on current primary |
| `ListMediaAttachmentsByEntity` | Paginated entity-scoped listing |
| `CountMediaAttachmentsByEntity` | Count for pagination |
| `ListAllMediaByOrg` | Org-wide paginated listing |
| `CountAllMediaByOrg` | Count for org-wide pagination |
| `MediaCheckPlayerExists` | EXISTS check scoped by org |
| `MediaCheckTeamExists` | EXISTS check scoped by org |
| `MediaCheckTournamentExists` | EXISTS check scoped by org |

#### Audit Logging

| Operation | Action | old_data | new_data |
|-----------|--------|----------|----------|
| Upload | `create` | — | DB-returned attachment row |
| Update (metadata) | `update` | Pre-update snapshot | DB-returned row |
| Primary unset (swap) | `update` | Pre-swap snapshot of old primary | `{id, is_primary: false}` |
| Primary set (swap) | `update` | Pre-swap snapshot of new attachment (is_primary=false) | DB-returned row (is_primary=true) |
| Delete | `delete` | Pre-delete snapshot | — |

All audit records are written inside the same transaction as the DB mutation. Storage operations happen outside the transaction after commit.

#### Files Added

| File | Purpose |
|------|---------|
| `db/migrations/000020_media_hardening.{up,down}.sql` | storage_key, content_hash, primary uniqueness index, media.update + media.delete permissions |
| `db/migrations/000021_media_content_uniqueness.{up,down}.sql` | Unique index on (org, entity_type, entity_id, content_hash) |
| `db/queries/media.sql` | 15 SQL queries |
| `db/sqlc/media.sql.go` | Generated — 15 query functions |
| `internal/media/storage/backend.go` | Backend interface, New() factory, GenerateKey() |
| `internal/media/storage/local.go` | Local filesystem backend |
| `internal/media/storage/s3.go` | S3-compatible backend (path-style URL, SigV4 inline) |
| `internal/media/storage/sigv4.go` | AWS Signature V4 signing via stdlib |
| `internal/media/processor/image.go` | MIME detection, decode, bilinear resize, JPEG encode, SHA-256 hash |
| `internal/media/errors.go` | Typed domain error sentinels |
| `internal/media/model.go` | ListParams, VariantsMeta, size constants |
| `internal/media/dto.go` | UploadRequest, UpdateRequest, Response, ListResponse |
| `internal/media/repository.go` | All transactional writes including SwapPrimaryWithAudit |
| `internal/media/service.go` | Upload pipeline, List, GetByID, Update, Delete |
| `internal/media/handler.go` | HTTP handlers + MaxBytesReader + multipart parsing |
| `internal/media/routes.go` | RegisterRoutes() + dev static file server |

#### Files Modified

| File | Change |
|------|--------|
| `internal/platform/config/config.go` | Added 9 storage configuration fields |
| `internal/bootstrap/modules.go` | Wired `media.RegisterRoutes`; storage backend constructed at startup |
| `db/sqlc/models.go` | Regenerated — `MediaAttachment` gains `StorageKey string`, `ContentHash string` |
| `go.mod` | Added `golang.org/x/image v0.41.0` for WebP decode |

### Phase 11 Hardening — Adversarial Review Defects (All Resolved)

An adversarial production review of the Phase 11 implementation identified three correctness defects. All were resolved before the phase was marked complete.

#### Defect 1 — SwapPrimaryWithAudit Audit Constraint Violation (Critical)

**Problem identified:** `SwapPrimaryWithAudit` wrote the "set new primary" `AuditActionUpdate` record with `old_data = nil`. The `audit_logs` table enforces `chk_audit_update_has_both_snapshots`: `action = 'update'` requires both `old_data IS NOT NULL` and `new_data IS NOT NULL`. Every call to `SwapPrimaryWithAudit` failed with a constraint violation and the transaction was rolled back. Setting `is_primary = true` was permanently non-functional. Both `service.Upload` and `service.Update` silently swallowed the error and returned incorrect responses to clients.

**Fix applied:**

Inside `SwapPrimaryWithAudit` (`internal/media/repository.go`), a new step was inserted between locking the old primary and calling `SetAttachmentAsPrimary`: `qtx.GetMediaAttachmentByID` fetches the new attachment's current state (with `is_primary = false`) within the transaction. This snapshot becomes `old_data` for the second audit record. Both audit records now satisfy the constraint.

In `service.Upload` and `service.Update`, the error from `SwapPrimaryWithAudit` is no longer silently discarded — it is propagated to the caller.

**Review status: PASS.** `is_primary` can now be set successfully. The audit constraint is satisfied. Both callers surface errors rather than misreporting success.

#### Defect 2 — Concurrent Duplicate Upload Race (Moderate)

**Problem identified:** The duplicate-detection flow (`GetByContentHash` → ErrNotFound → proceed) is non-atomic. Two concurrent uploads of the same content to the same entity both observe ErrNotFound before either commits, both proceed through image processing and S3 upload, and both insert rows. No unique constraint prevented the race.

**Fix applied:**

Migration 000021 adds `uq_media_content_per_entity` — a unique index on `(organization_id, entity_type, entity_id, content_hash)`. When the second concurrent INSERT hits this index, the INSERT fails with SQLSTATE 23505. `CreateWithAudit` (`internal/media/repository.go`) detects this via `pgutil.IsUniqueViolation(err, "uq_media_content_per_entity")`, rolls back the failed transaction, and re-queries `GetMediaAttachmentByContentHash` to return the existing row committed by the first request. The caller receives the existing attachment — no 500, no duplicate record.

**Review status: PASS.** Concurrent duplicate uploads produce one record, not two.

#### Defect 3 — Double-Delete Audit Drift (Minor)

**Problem identified:** `DeleteMediaAttachment` was declared `:exec`, which discards rows affected. A second concurrent delete of the same attachment would execute a zero-row DELETE, receive no error, and proceed to write a spurious `AuditActionDelete` record for an entity that no longer existed. The caller received 204, not 404.

**Fix applied:**

`DeleteMediaAttachment` changed from `:exec` to `:execrows` in `db/queries/media.sql`. `sqlc generate` regenerated the function to return `(int64, error)`. `DeleteWithAudit` (`internal/media/repository.go`) now checks `rowsAffected == 0` immediately after the DELETE and returns `ErrNotFound` before the audit INSERT is reached. The transaction rolls back with no audit record written.

**Review status: PASS.** A second concurrent delete returns 404. No spurious audit records are written.

#### Phase 11 Production Hardening Review (adversarial, all PASS)

| Check | Result |
|-------|--------|
| Multi-tenant isolation | **PASS** |
| BOLA protection | **PASS** |
| Authorization (upload/update/delete/read) | **PASS** |
| MIME spoofing prevention | **PASS** |
| Image processing correctness | **PASS** |
| File size enforcement | **PASS** |
| Duplicate detection (sequential) | **PASS** |
| Duplicate detection (concurrent race) | **PASS** |
| Primary image uniqueness | **PASS** |
| Concurrent primary swap safety | **PASS** |
| Storage consistency (S3 drift) | **PASS** |
| Delete correctness | **PASS** |
| Double-delete audit safety | **PASS** |
| Storage key security (path traversal) | **PASS** |
| Audit logging correctness | **PASS** |
| Concurrency invariants | **PASS** |
| Performance | **PASS** |

---

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
- **Review status: PASS.** Concurrent requests for the same tournament now block at the `FOR UPDATE` step and see the updated count before deciding whether to insert.

### Concurrency Hardening — Match Tournament Cancellation Race

**Problem:** Match creates and lifecycle transitions need to verify the parent tournament is `ongoing`. A tournament cancellation committed between the service-layer check and the DB write would allow a match to be created — or progressed to `live`/`completed`/`abandoned` — inside a tournament that is no longer active.

**Fix applied:**

- A new SQL query `LockTournamentForShare` was added: `SELECT status FROM tournaments WHERE id = $1 AND organization_id = $2 FOR SHARE`.
- This lock is acquired **inside** the match create/update transaction before any match write executes.
- `FOR SHARE` blocks any concurrent `UPDATE tournaments SET status = 'cancelled'` on the same row until the match transaction commits, while allowing other concurrent match creates to also acquire `FOR SHARE` (compatible; no deadlock).
- Tournament status is re-validated after the lock is held. If already `cancelled`, the transaction rolls back and returns `ErrTournamentNotOngoing` (HTTP 422).
- **Review status: PASS.**

### Concurrency Hardening — Match Terminal-State Race

**Problem:** The service-layer terminal-state guard (`service.go:Update`) reads `current.Status` outside any transaction. Two concurrent PATCH requests that both read a non-terminal status (e.g., `live`) both pass the guard, both enter `UpdateWithAudit`, and both execute `UpdateMatch`. PostgreSQL serialises the two writes on the same row, but the second write has no status filter — it succeeds and overwrites the terminal state committed by the first. A match marked `completed` could be overwritten to `abandoned`, or a `cancelled` match reactivated to `live`.

**Fix applied:**

Both `UpdateMatch` and `CancelMatch` now include a database-level terminal-state guard in their WHERE clauses (`db/queries/matches.sql`):

```sql
AND  status NOT IN ('completed', 'cancelled', 'abandoned')
```

PostgreSQL evaluates this predicate atomically as part of the row-level write. If the match is already terminal (committed by a concurrent request between the service read and this transaction), the WHERE condition matches zero rows. `pgx.ErrNoRows` is returned and the repository maps it to `ErrMatchNotUpdatable` (HTTP 422). The terminal state is preserved.

The service-layer guard remains in place as the first line of defence for sequential requests, avoiding unnecessary transaction overhead.

**Review status: PASS.** Concurrent requests that both pass the service-layer check cannot both write to the same match row if the first write transitions the match to a terminal state.

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
- [x] **match.delete permission** — migration 000018 (Phase 7C)
- [x] **Matches module** — Phase 8A (CRUD, lifecycle, participant/registration validation, concurrency hardening)
- [x] **Match Events module** — Phase 8B (append-only event log, sequence integrity, participant validation, correction rules, production hardening)
- [x] **Audit logging** — wired into all organization, player, team, tournament, registration, match, and match event mutations
- [x] **BOLA protection** — enforced in every write service across all implemented modules
- [x] **Live Scoring engine** — Phase 9 (`internal/scoring/`; `GET /matches/{id}/score`; effective-log derivation; write-time payload validation; all-out inversion; super_raid zero-score protection)
- [x] **all_out participant validation hardening** — Phase 9 post-review (`ValidateAllOutParticipant`; rejects non-participant `team_id` at write time; ErrInvalidScorePayload HTTP 400)
- [x] **Score snapshot at completion** — Phase 10 (`matches.home_score`/`away_score` written once inside locked completion transaction; winner consistency enforced; `ErrWinnerScoreMismatch` HTTP 422)
- [x] **Tournament Standings & Rankings** — Phase 10 (`internal/standings/`; `GET /tournaments/{id}/standings`; 7-level tiebreak chain; point system from `tournaments.settings`; fully deterministic)
- [x] **Media Management** — Phase 11 (`internal/media/`; upload/list/get/update/delete endpoints; StorageBackend abstraction; local + S3-compatible backends; server-side MIME detection + image decode + JPEG normalize + thumbnail generation; content-hash duplicate detection; primary-image swap with FOR UPDATE lock + unique-index backstop; adversarial review: all 17 checks PASS)

### Must-have before first production deployment

- [ ] **Password reset flow** — `POST /api/v1/auth/forgot-password` / `POST /api/v1/auth/reset-password`
- [ ] **Refresh token cleanup job** — `DeleteExpiredRefreshTokens` is generated and correct but never called. Without it the `refresh_tokens` table grows unboundedly.
- [ ] **Email verification token cleanup job** — `DeleteExpiredVerificationTokens` is implemented on the service but never called on a schedule.
- [ ] **CORS configuration** — `internal/platform/middleware/cors.go` is fully implemented (`CORS(allowedOrigins)` + origin matching + OPTIONS preflight handling) but is not wired into the router. `bootstrap/router.go` does not call `r.Use(middleware.CORS(...))`. Browsers will block cross-origin requests until it is added to the middleware stack.
- [ ] **Rate limiting** — `internal/platform/middleware/ratelimit.go` is a stub. Auth endpoints are unprotected against brute-force attacks.
- [ ] **Remove `verification_token` from register response in production** — currently returned for development convenience; must be gated behind `IsDevelopment()` before deployment.

### Required for feature completeness

- [ ] **Users module** — User management: list, get, update profile, change password, deactivate.
- [ ] **News module** — Stub exists; no business logic.
- [ ] **N-way head-to-head resolution** — Phase 10 standings apply head-to-head only to strict 2-way ties. Full sub-table resolution for 3+ tied participants is deferred. When exactly N>2 participants share the same points, tiebreaking falls through to score difference (criterion 3).

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
| Phase 8A | Matches | **COMPLETE** |
| Phase 8B | Match Events | **COMPLETE** |
| Phase 9 | Live Scoring | **COMPLETE** |
| Phase 10 | Rankings & Standings | **COMPLETE** |
| Phase 11 | Media Management | **COMPLETE** |
| Phase 12 | Notifications | NOT STARTED |
| Phase 13 | Hardening, Observability & Tests | NOT STARTED |

---

### Phase 9 — Live Scoring (Complete)

Stateless scoring engine in `internal/scoring/`. Score derived on read from the effective event log. No score columns added to the database. Full kabaddi scoring rule set implemented: `raid_successful`, `bonus_point_awarded`, `tackle_successful`, `super_tackle`, `all_out` (opponent bonus), `penalty_awarded`. `super_raid` explicitly contributes zero (analytics label only). Write-time payload validation added to the `match_events` create path. All 14 adversarial review checks passed.

---

### Phase 10 — Rankings & Standings (Complete)

Tournament-scoped standings derived from snapshotted match scores. Pure computation package (`internal/standings/`) with no DB access. Score snapshot written once at `live → completed` transition under `FOR UPDATE` lock (`matches.home_score`/`away_score`, migration 000019). Standings endpoint `GET /tournaments/{id}/standings` reads snapshotted scores only — never `match_events`. Full 7-level tiebreak chain including head-to-head for strict 2-way ties. Point system configurable via `tournaments.settings` JSONB; defaults to 3/1/0. All 15 adversarial review checks passed.

---

### Phase 11 — Media Management (Complete)

Full media attachment lifecycle implemented and production-hardened. Upload pipeline: server-side MIME detection → image decode validation → bilinear resize → JPEG normalization → three-variant S3 upload (full, 150 px, 400 px) → DB row + audit log. StorageBackend abstraction supports local filesystem (development) and any S3-compatible service (production) via inline SigV4 signing. Adversarial review identified three defects; all resolved before phase marked complete. All 17 adversarial review checks PASS.

---

### Phase 12 — Notifications

*Not designed. Depends on match events and tournament status changes.*

---

### Phase 13 — Hardening, Observability & Tests

1. **Test suite** — integration tests using `testcontainers-go` against a real PostgreSQL 17 instance. Priority: auth service, multi-tenant isolation, tournament registration rules, match lifecycle and concurrency invariants, match event sequence integrity.
2. **OpenTelemetry tracing** — instrument repository and service layers.
3. **Prometheus metrics** — request latency histograms, DB pool stats, active sessions counter.
4. **Password reset flow** — `POST /auth/forgot-password` / `POST /auth/reset-password`.
5. **Rate limiting & CORS** — implement stubs before production deployment.
6. **`go mod tidy`** — move `golang-jwt/jwt/v5` from `indirect` to direct.

---

## 10. Next Recommended Phase

**Phase 12 — Notifications**

Phase 11 (Media Management) is complete and production-hardened. The media module implements the full upload lifecycle with server-side MIME validation, JPEG normalization, thumbnail generation, and a provider-agnostic storage backend. All 17 adversarial review checks passed after three identified defects were resolved. The next step is the notifications module.

**Planning goals:**

- Event-driven notification delivery triggered by domain events: match status transitions, tournament registration approvals, score milestones.
- Notification delivery channels to design: in-app (DB-persisted), email (transactional via SMTP or SendGrid), webhook (org-configurable).
- A `notifications` table scoped by `organization_id` and `user_id` with read/unread state and a `payload` JSONB column for channel-specific metadata.
- A notification preference model — per-user, per-org, per-event-type opt-in/out.
- All notification writes must be transactional with the domain event that triggered them (or queued atomically if async delivery is introduced).

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
| `backend/internal/matches/service.go` | Match lifecycle, participant validation, winner rules, BOLA guard |
| `backend/internal/matches/repository.go` | Match transactional writes; FOR SHARE tournament lock pattern |
| `backend/internal/match_events/service.go` | Event validation, correction rules, participant checks, BOLA guard, write-time payload validation |
| `backend/internal/match_events/repository.go` | `CreateWithAudit` — FOR UPDATE lock, sequence computation, all in-transaction validation |
| `backend/internal/scoring/engine.go` | `ScoreEngine.Compute` — pure stateless score derivation; entry point for Phase 9+ scoring |
| `backend/internal/scoring/rules.go` | Kabaddi scoring rules: `participantSide`, `allOutScore`, `payloadPoints` |
| `backend/internal/scoring/validation.go` | `ValidateScoreEventPayload` + `ValidateAllOutParticipant` — write-time payload guards |
| `backend/internal/standings/engine.go` | `standings.Compute` — pure standings derivation from completed match snapshots; no DB access |
| `backend/internal/standings/tiebreakers.go` | Full 7-level tiebreak comparator; head-to-head for strict 2-way ties |
| `backend/internal/tournaments/service.go` | `GetStandings` — orchestrates standings derivation; `parseStandingsSettings` reads point values from JSONB |
| `backend/db/migrations/000019_match_score_snapshots.up.sql` | Adds `home_score`/`away_score` to `matches`; immutability contract documented in comments |
| `backend/internal/bootstrap/modules.go` | Single place to register new domain modules |
| `backend/internal/platform/pgutil/pgutil.go` | Shared UUID and constraint helpers used by all domain repositories |
| `backend/internal/platform/validator/validator.go` | JSON decode + struct-tag validation (no external deps) |
| `backend/sqlc.yaml` | sqlc configuration |
| `backend/go.mod` | Module definition and direct dependencies |
| `backend/internal/media/repository.go` | `CreateWithAudit` (with duplicate-detection retry), `SwapPrimaryWithAudit` (FOR UPDATE + pre-swap snapshot), `DeleteWithAudit` (rows-affected guard) |
| `backend/internal/media/service.go` | Upload pipeline orchestration; BOLA guard; entity ownership verification; storage + DB sequencing |
| `backend/internal/media/storage/backend.go` | `Backend` interface; `New()` factory; `GenerateKey()` |
| `backend/internal/media/storage/s3.go` | S3-compatible backend; path-style URL; SigV4 signed PUT and DELETE |
| `backend/internal/media/storage/sigv4.go` | AWS Signature V4 implementation using stdlib only |
| `backend/internal/media/processor/image.go` | MIME detection; image decode; bilinear resize; JPEG encode; SHA-256 content hash |
| `backend/db/migrations/000020_media_hardening.up.sql` | storage_key, content_hash, primary uniqueness index, media.update + media.delete RBAC |
| `backend/db/migrations/000021_media_content_uniqueness.up.sql` | Unique index preventing concurrent duplicate upload records |
| `backend/db/queries/media.sql` | 15 media SQL queries including :execrows delete |

---

*This document was last updated on 2026-06-02 (Phase 11 complete). It should be updated whenever a phase is completed or significant architectural changes are made.*
