# PlayArena — Project State & Handoff Document

**Last Updated:** 2026-06-04  
**Build status:** `go build ./...` passing, `go vet ./...` clean, `sqlc generate` clean  
**Migrations applied:** 000001 – 000024  
**Go version:** 1.25.6  
**Database:** PostgreSQL 17  
**Phases complete:** 1 – 12, Auth Security Hotfix v2, Phase 13A, Phase 13B.1, Phase 13B.2A, Phase 13B.2B-A, Phase 13B.2B-B

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
│   ├── migrations/                         golang-migrate files (000001–000024, up + down); embed.go exports FS for test runners
│   ├── queries/                            Hand-written SQL (sqlc source)
│   └── sqlc/                              Generated type-safe Go — never edited by hand
├── internal/
│   ├── auth/                               Auth domain (fully implemented + integration-tested)
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
│   │   ├── routes.go                      RegisterRoutes() — mounts /api/v1/auth subtree
│   │   ├── testmain_test.go               TestMain — per-package testcontainer lifecycle
│   │   ├── repository_test.go             RevokeAndLinkSuccessor invariant test
│   │   └── concurrency_test.go            7 auth concurrency tests (barrier synchronization)
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
│   ├── notifications/                      Notifications domain (fully implemented, Phase 12)
│   │   ├── errors.go                      Typed domain error sentinels
│   │   ├── model.go                       ListParams, prefKey, pagination constants
│   │   ├── dto.go                         Response, ListResponse, PreferenceResponse, UpdatePreferenceRequest
│   │   ├── repository.go                  UpsertPreference (dynamic audit action); DrainOutbox (batch preference load, FOR UPDATE SKIP LOCKED)
│   │   ├── service.go                     List, GetByID, MarkRead, MarkAllRead, Delete, GetPreferences, UpdatePreference, DrainOutbox
│   │   ├── handler.go                     HTTP handlers + error mapping
│   │   ├── routes.go                      RegisterRoutes() — mounts /api/v1/organizations/{slug}/notifications
│   │   └── trigger/
│   │       └── trigger.go                 WriteOutboxEntry — writes notification_outbox rows inside domain transactions
│   ├── testutil/                           Shared integration-test infrastructure (Phase 13B.1)
│   │   ├── container.go                   SetupTestDB — postgres:17-alpine container, migrations, pool; Docker-skip logic
│   │   └── fixtures/
│   │       └── auth.go                    CreateActiveUser, CreatePendingUser, CreateSuspendedUser, CreateInactiveUser,
│   │                                      CreatePlatformAdmin, CreateOrgWithRole, CreateRefreshToken,
│   │                                      CreateExpiredRefreshToken, CreatePasswordResetToken,
│   │                                      CreateExpiredPasswordResetToken, CreateExpiredEmailVerificationToken,
│   │                                      CleanupUser, HashToken
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
| 000022 | Notifications | `notification_event_type` ENUM (9 values), `notification_channel` ENUM (in_app/email/webhook); `notification_outbox` (transactional outbox, written inside domain transactions); `notifications` (personal inbox, written only by DrainOutbox; `UNIQUE (outbox_id, user_id, channel)` — drain idempotency safeguard); `notification_preferences` (per-user opt-out); delivery indexes; `notification.manage` permission granted to `platform_admin`, `org_owner`, `org_admin` |
| 000023 | Refresh Token Successor Tracking | `refresh_tokens.successor_id UUID NULL` — structural replay detection marker; `chk_refresh_tokens_successor CHECK (successor_id IS NULL OR revoked_at IS NOT NULL)` — DB-level state invariant; no FK, no index; replaces time-window replay detection with deterministic state machine |
| 000024 | Password Reset Tokens | `password_reset_tokens` — single-use, SHA-256-hashed, 1-hour expiry; `fk_password_reset_tokens_user` CASCADE; `uq_password_reset_tokens_hash` unique; `idx_password_reset_tokens_user_id` for bulk invalidation; `idx_password_reset_tokens_expires WHERE used_at IS NULL` for cleanup |

### Table Summary

#### Identity & Auth

**`users`** — Platform-level identity. Not org-scoped. One account per person.  
Key columns: `id`, `email` (unique), `username` (unique), `password_hash` (bcrypt), `status` (`user_status` ENUM), `email_verified_at`, `last_login_at`, `last_login_ip`.

**`refresh_tokens`** — Revocation store for refresh tokens. Stores SHA-256 hash only, never the raw token.  
Key columns: `token_hash` (unique), `expires_at`, `revoked_at` (NULL = active), `successor_id` (UUID NULL — set at rotation to the new token's ID; used as a state marker for replay detection), `user_id` (CASCADE), `ip_address`, `user_agent`.  
`successor_id` is **not a foreign key** by design. It is a historical classification signal: `IS NULL` means the token was explicitly revoked (logout); `IS NOT NULL` means it was rotated. The application never follows the reference — it only tests nullness. Added by migration 000023.

**`email_verification_tokens`** — Single-use email verification tokens. Stores SHA-256 hash only. Valid when `used_at IS NULL AND expires_at > NOW()`.

**`password_reset_tokens`** — Single-use password reset tokens (Phase 13A). Stores SHA-256 hash only. Valid when `used_at IS NULL AND expires_at > NOW()`. Expires after 1 hour. Consumption atomically marks the presented token **and all sibling tokens for the same user** as used (`UseAllUserPasswordResetTokens`) inside the same `FOR UPDATE` transaction, preventing stale tokens from being replayed after a successful reset.

#### RBAC

**`permissions`** — Atomic capability definitions. `slug` format: `<resource>.<action>` (e.g. `tournament.create`). Immutable at runtime. 18 permissions seeded by migration 000017; `match.delete` added by migration 000018 (19 total).

**`roles`** — Named permission groups. `scope` is `platform` | `organization` | `tournament`. Platform roles have `organization_id = NULL`; org roles have a non-NULL FK. `is_system` flags protect seed roles from deletion. 7 system roles seeded: `platform_admin`, `org_owner`, `org_admin`, `team_manager`, `coach`, `scorer`, `viewer`.

**`role_permissions`** — M:M join. Cascade both sides.

**`user_organization_roles`** — Grants a user a role in a specific org context.  
`organization_id` is **NULLable** (since migration 000015) to allow platform-scoped grants.  
Supports `expires_at` for time-limited grants (e.g. guest scorer per tournament).  
Unique constraints: `(user_id, organization_id, role_id)` for org grants; partial unique index `(user_id, role_id) WHERE organization_id IS NULL` for platform grants.

#### Permission Matrix (complete, as of migration 000022)

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
| `notification.manage` | ✓ | ✓ | ✓ | — | — | — | — |

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

#### Notification Tables (added migration 000022)

**`notification_outbox`** — Transactional outbox. Written atomically inside domain transactions (matches, tournaments, registrations). Never written by the notifications service. Read exclusively by `DrainOutbox` using `FOR UPDATE SKIP LOCKED`. `processed_at` is NULL for pending entries; set to `NOW()` once fully fanned out. Partial index `idx_notif_outbox_pending` covers only pending rows.

**`notifications`** — Personal notification inbox. Written **only** by `DrainOutbox` after domain transactions commit. Every row is scoped to one user within one organization. `UNIQUE (outbox_id, user_id, channel)` — drain retry idempotency safeguard. Soft-deleted via `deleted_at`; all queries exclude `deleted_at IS NOT NULL`. `sent_at` contract: `in_app` → set to `NOW()` on drain insert; `email`/`webhook` → NULL until future delivery workers confirm send.

**`notification_preferences`** — Per-user, per-org, per-event-type, per-channel opt-in/out. Missing row = enabled (opt-out model). UPSERT semantics (last-writer-wins) on `(organization_id, user_id, event_type, channel)`.

### Key Design Decisions

**Event sourcing for live scoring.** `match_events` is the single source of truth for all scoring during active matches. The live scoring engine (`internal/scoring/`) derives home and away scores on every `GET /matches/{id}/score` request from the effective event log — no score is ever stored for live matches.

**Score snapshot at completion.** When a match transitions `live → completed`, `matches.home_score` and `matches.away_score` are written atomically inside the same transaction, under a `FOR UPDATE` lock on the match row. The lock prevents any concurrent event insertion between the score computation and the status write, making the snapshot permanently consistent with the event log. After completion, no further events can be recorded (`ErrMatchNotLive`), so the snapshot never drifts. Corrections are non-destructive: a `score_correction` event references (via `cancels_event_id`) the event it supersedes; neither row is mutated.

**Denormalization with trigger guards.** `matches.organization_id` and `match_events.organization_id` are denormalized for query performance. Database triggers (`trg_matches_org_consistency`, `trg_match_events_org_consistency`) enforce consistency on INSERT/UPDATE.

**Cross-org tournament registrations.** `tournament_registrations.organization_id` is the registrant's org, not the tournament host org. A federation tournament can accept teams from multiple clubs. The registrant's team/player must still belong to the registrant's org (validated by trigger).

**Soft foreign keys in media.** `media_attachments` uses a polymorphic `(entity_type, entity_id)` reference. No DB-level FK is possible. The application service layer is responsible for orphan cleanup when parent entities are deleted.

**Transactional outbox for notifications.** Domain writes (matches, tournaments, registrations) write a row to `notification_outbox` inside the same transaction as the domain mutation. After the transaction commits, the domain service calls `DrainOutbox` synchronously. The drain opens a fresh transaction, claims pending rows with `FOR UPDATE SKIP LOCKED`, fans out in-app notifications to all org members (filtered by preferences loaded in one batch query per event type), and marks entries processed. This decouples notification delivery from domain transactions: a drain failure never rolls back a committed domain operation, and the outbox entries remain durable for the next drain cycle. Drain idempotency is enforced at the database level by `UNIQUE (outbox_id, user_id, channel)` on `notifications` — a retry can never produce duplicate rows.

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
2. GetRefreshTokenByHash() (read-only peek for user_id — unlocked pre-fetch)
3. GetUserByID() → re-validate user status
4. resolveOrgContext() (client may request different org context on refresh)
5. Generate new refresh token raw value
6. RotateRefreshToken() — transaction with FOR UPDATE row lock:
     a. SELECT ... FOR UPDATE on the token row (serialises concurrent rotation)
     b. Replay state machine (see below):
          revoked_at IS NULL                              → Case 1: proceed
          revoked_at IS NOT NULL, successor_id IS NOT NULL → Case 2: ErrInvalidToken (no wipe)
          revoked_at IS NOT NULL, successor_id IS NULL    → Case 3: wipe all sessions, ErrTokenReuse
     c. If expires_at < NOW() → return ErrExpiredToken
     d. CreateRefreshToken() (new token — inserted first to obtain its ID)
     e. RevokeAndLinkSuccessor(old.ID, new.ID) — atomic UPDATE:
          SET revoked_at = NOW(), successor_id = new.ID WHERE id = old.ID AND revoked_at IS NULL
     f. Assert rows affected == 1 (defensive invariant enforced by the FOR UPDATE lock)
     g. COMMIT
7. GenerateAccessToken() with new org + role context
8. Return { access_token, refresh_token, expires_in: 900, token_type: "Bearer" }
```

### Replay Detection State Machine (Auth Security Hotfix v2)

Replay detection is deterministic and structural — no wall-clock comparisons, no grace windows.  
The `successor_id` column on a revoked token encodes its history:

| `revoked_at` | `successor_id` | Token state | Action |
|:---:|:---:|---|---|
| `IS NULL` | `IS NULL` | **Active** — not yet used or revoked | Allow rotation (Case 1) |
| `IS NOT NULL` | `IS NOT NULL` | **Rotated** — this token was superseded by a rotation; the holder of the successor token is legitimate | `ErrInvalidToken` — no session revocation (Case 2) |
| `IS NOT NULL` | `IS NULL` | **Explicitly revoked** — logout, logout-all, password reset, admin action; any presentation is anomalous | `ErrTokenReuse` — revoke all active sessions (Case 3) |

**Case 2 vs Case 3 distinction eliminates both failure modes of the previous design:**

- *Old design (time-window):* relied on `time.Since(revoked_at) <= 30s`. Within the window, genuine replay attacks were silently accepted. Outside the window, concurrent legitimate duplicate requests (e.g., two browser tabs refreshing simultaneously) triggered false session wipes.
- *New design (structural):* concurrent duplicate requests both see `successor_id IS NOT NULL` → Case 2 → `ErrInvalidToken` (no wipe). Genuine replay of an explicitly revoked token → Case 3 → full wipe. No timing dependency of any kind.

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
  └── notification_outbox            (written inside domain transactions; drained post-commit)
      └── notifications              (personal inbox; written only by DrainOutbox; scoped by user_id)
  └── notification_preferences       (per-user opt-out; scoped by user_id)
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
| **Notifications** | `errors.go`, `model.go`, `dto.go`, `repository.go`, `service.go`, `handler.go`, `routes.go`; `trigger/trigger.go` | Complete |
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
| `POST` | `/api/v1/auth/logout` | No | Revoke refresh token by value; transactional (FOR UPDATE) to prevent concurrent-rotation race |
| `POST` | `/api/v1/auth/forgot-password` | No | Create 1-hour reset token; always returns 200 (enumeration-resistant); token in response body in development only |
| `POST` | `/api/v1/auth/reset-password` | No | Consume reset token; update password; revoke all sessions — single atomic transaction |
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

### Notifications (`/api/v1/organizations/{slug}/notifications`)

All notification endpoints require `RequireAuth` only — no RBAC permission checks. Every query is scoped by **both** `organization_id` (resolved from URL slug) **and** `user_id` (from JWT principal). A user can only read, mark, and delete their own notifications.

| Method | Path | Auth | Description |
|--------|------|:----:|-------------|
| `GET` | `/api/v1/organizations/{slug}/notifications` | Yes | List caller's undeleted notifications, newest first (paginated; `?limit`, `?offset`) |
| `GET` | `/api/v1/organizations/{slug}/notifications/{id}` | Yes | Get single notification scoped to caller and org |
| `PATCH` | `/api/v1/organizations/{slug}/notifications/{id}/read` | Yes | Mark a single notification as read; returns the updated resource |
| `POST` | `/api/v1/organizations/{slug}/notifications/read-all` | Yes | Mark all unread notifications as read; idempotent |
| `DELETE` | `/api/v1/organizations/{slug}/notifications/{id}` | Yes | Soft-delete a notification (sets `deleted_at`); double-delete returns 404 |
| `GET` | `/api/v1/organizations/{slug}/notifications/preferences` | Yes | List all stored preferences for the caller in this org (missing row = enabled by default) |
| `PUT` | `/api/v1/organizations/{slug}/notifications/preferences/{event_type}` | Yes | Upsert a preference for `event_type` + `channel`; UPSERT semantics (last-writer-wins); audited |

Valid `event_type` values: `match_created`, `match_started`, `match_completed`, `match_cancelled`, `match_abandoned`, `tournament_status_changed`, `registration_approved`, `registration_rejected`, `registration_withdrawn`.  
Valid `channel` values: `in_app`, `email`, `webhook`. Phase 12 delivers in_app only; email and webhook are reserved for future workers.

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

### Phase 12 — Notifications (Complete)

Phase 12 implements the Transactional Outbox notification system. In-app notifications are delivered synchronously after domain transactions commit. The architecture preserves compatibility with future email, webhook, and push channels, and with a future outbox worker for crash-recovery delivery.

#### Architecture — Transactional Outbox Pattern

Inline fan-out (writing notifications inside the domain transaction) was rejected because a notification failure would roll back the domain mutation. The transactional outbox separates the two concerns:

```
Domain Transaction
  ├── Domain write (match INSERT / UPDATE, tournament UPDATE, registration UPDATE)
  ├── Audit log INSERT
  └── notification_outbox INSERT       ← atomic with domain; outbox entry never lost

COMMIT

DrainOutbox (synchronous, post-commit, separate transaction)
  ├── SELECT … FOR UPDATE SKIP LOCKED  ← claim pending entries; concurrent drains skip
  ├── GetOrgMembersForNotification     ← one query; all members
  ├── GetNotificationPreferencesForEvent (per unique event type) ← batch; O(event types)
  ├── CreateNotification × N members   ← one insert per notified user per entry
  └── MarkOutboxEntryProcessed × N entries
COMMIT
```

A drain failure logs the error and returns; the domain operation is already committed. Outbox entries remain pending (`processed_at IS NULL`) and are retried on the next drain cycle (the next domain operation in the same org). `UNIQUE (outbox_id, user_id, channel)` on `notifications` guarantees that a retry never creates duplicate rows — the second insert is silently ignored.

#### Delivery Channels

| Channel | Phase 12 | Future |
|---------|----------|--------|
| `in_app` | ✓ Delivered by DrainOutbox; `sent_at = NOW()` on insert | — |
| `email` | Schema reserved | Worker sets `sent_at` on confirmed send |
| `webhook` | Schema reserved | Worker sets `sent_at` on confirmed delivery |

#### Compare-And-Swap on Match Transitions (Phase 12B)

`UpdateMatch` and `CancelMatch` were upgraded from a terminal-state NOT IN guard to a strict compare-and-swap (CAS):

```sql
-- UpdateMatch (previously: AND status NOT IN ('completed','cancelled','abandoned'))
WHERE id = $1 AND organization_id = $2 AND status = $20   -- $20 = previous_status

-- CancelMatch
WHERE id = $1 AND organization_id = $2 AND status = $3    -- $3 = previous_status
```

The service reads `current.Status` before the transaction and passes it as the CAS guard (`Status_2` in the sqlc-generated `UpdateMatchParams`). A concurrent transition that changes the status between the service read and the UPDATE results in 0 rows matched → `ErrMatchNotUpdatable` (HTTP 422). The CAS is required to guarantee that outbox entries carry the correct previous and new status values.

#### Domain Integration

| Domain | Write trigger | Event type(s) |
|--------|--------------|---------------|
| Matches — Create | Match INSERT | `match_created` |
| Matches — Update | Status transition | `match_started`, `match_completed`, `match_cancelled`, `match_abandoned` |
| Matches — Cancel | `DELETE` | `match_cancelled` |
| Tournaments — Update | Status transition | `tournament_status_changed` |
| Tournaments — Cancel | `DELETE` | `tournament_status_changed` |
| Registrations — Update | Status transition | `registration_approved`, `registration_rejected` |
| Registrations — Withdraw | `DELETE` | `registration_withdrawn` |

All outbox writes use `trigger.WriteOutboxEntry(ctx, qtx, ...)` where `qtx` is the transaction-scoped queries handle — the outbox entry is always atomic with the triggering domain write.

#### Preference System

Missing preference row = enabled (opt-out model). `notification_preferences` is the stored exception table. Preferences are resolved during drain from an in-memory map built by a single batch query per unique `(event_type, channel)` pair — no per-user SQL in the fan-out loop.

#### Files Created

| File | Purpose |
|------|---------|
| `db/migrations/000022_notifications.{up,down}.sql` | ENUMs, tables, indexes, `notification.manage` permission, role grants |
| `db/queries/notifications.sql` | 16 SQL queries (outbox, notifications, preferences) |
| `db/sqlc/notifications.sql.go` | Generated — all query functions including `GetNotificationPreferencesForEvent` |
| `internal/notifications/errors.go` | Typed domain error sentinels |
| `internal/notifications/model.go` | `ListParams`, `prefKey`, pagination constants |
| `internal/notifications/dto.go` | `Response`, `ListResponse`, `PreferenceResponse`, `UpdatePreferenceRequest` |
| `internal/notifications/repository.go` | DB access; `DrainOutbox` (batch preference load); `UpsertPreference` (dynamic audit action) |
| `internal/notifications/service.go` | All use-cases; `DrainOutbox` entry point (errors logged, not propagated) |
| `internal/notifications/handler.go` | HTTP handlers + error mapping |
| `internal/notifications/routes.go` | 7 routes, all `RequireAuth` only |
| `internal/notifications/trigger/trigger.go` | `WriteOutboxEntry` — called inside domain transactions |

#### Files Modified

| File | Change |
|------|--------|
| `db/queries/matches.sql` | `UpdateMatch`: `AND status = $20` CAS guard; `CancelMatch`: `AND status = $3` CAS guard |
| `db/sqlc/matches.sql.go` | Regenerated — `UpdateMatchParams.Status_2` (CAS), `CancelMatchParams.Status` (CAS) |
| `internal/matches/repository.go` | Outbox write in Create/Update/Cancel; `Status_2` CAS wiring |
| `internal/matches/service.go` | `Status_2: current.Status`; DrainOutbox call post-commit; accepts `*notifications.Service` |
| `internal/matches/routes.go` | Accepts and passes `*notifications.Service` |
| `internal/tournaments/repository.go` | Outbox write in Update/Cancel when status changes |
| `internal/tournaments/service.go` | `previousStatus` param; DrainOutbox call post-commit; accepts `*notifications.Service` |
| `internal/tournaments/routes.go` | Accepts and passes `*notifications.Service` |
| `internal/tournament_registrations/repository.go` | Outbox write in Update/Withdraw when status changes |
| `internal/tournament_registrations/service.go` | `previousStatus` param; DrainOutbox call post-commit; accepts `*notifications.Service` |
| `internal/tournament_registrations/routes.go` | Accepts and passes `*notifications.Service` |
| `internal/bootstrap/modules.go` | Constructs shared `*notifications.Service` singleton; wires it to matches, tournaments, registrations, and notifications routes |

#### SQL Queries Added (`db/queries/notifications.sql`)

| Query | Purpose |
|-------|---------|
| `CreateNotificationOutboxEntry` | INSERT into outbox inside domain transaction; `:one` RETURNING |
| `DrainOutboxEntries` | `SELECT … FOR UPDATE SKIP LOCKED LIMIT 100` — claim pending entries |
| `MarkOutboxEntryProcessed` | `UPDATE … SET processed_at = NOW()` |
| `GetOrgMembersForNotification` | `SELECT DISTINCT uor.user_id` — all non-expired org members |
| `CreateNotification` | INSERT into notifications; unique violation handled at application layer |
| `GetNotificationByID` | Single notification scoped by `(id, org_id, user_id, deleted_at IS NULL)` |
| `ListNotificationsByUser` | Paginated listing scoped by `(org_id, user_id, deleted_at IS NULL)` |
| `CountNotificationsByUser` | Count for pagination metadata |
| `MarkNotificationRead` | `UPDATE … SET read_at = NOW() WHERE read_at IS NULL` RETURNING |
| `MarkAllNotificationsRead` | Bulk read; `WHERE read_at IS NULL AND deleted_at IS NULL` |
| `SoftDeleteNotification` | `:execrows` — rows-affected check for 404 on double-delete |
| `GetUserPreferences` | All preferences for a user in an org |
| `GetUserPreference` | Single preference row; `ErrNoRows` = enabled by default |
| `UpsertNotificationPreference` | `ON CONFLICT DO UPDATE SET enabled, updated_at` |
| `GetNotificationPreferencesForEvent` | Batch-load all preferences for `(org, event_type, channel)` — eliminates N+1 |

#### Permission Added

| Permission | Slug | Granted to |
|-----------|------|------------|
| Manage Notifications | `notification.manage` | `platform_admin`, `org_owner`, `org_admin` |

Personal notification endpoints (read, delete own, preferences) require only `RequireAuth`.

#### Phase 12 Hardening Review — Defects Found and Resolved

An adversarial production review of the Phase 12 implementation identified two defects. Both were resolved before marking the phase production-ready.

##### Defect 1 — First-Time Preference Creation Fails with Audit Constraint Violation (Critical)

**Problem identified:** `UpsertPreference` always used `AuditActionUpdate`. When creating a preference for the first time, `GetUserPreference` returns `pgx.ErrNoRows`, leaving `oldData = nil`. The audit_logs table enforces `chk_audit_update_has_both_snapshots`: `action = 'update'` requires both `old_data IS NOT NULL` and `new_data IS NOT NULL`. Every first-time preference creation failed with a PostgreSQL constraint violation, rolling back the transaction. The endpoint returned HTTP 500 on first invocation for any user.

**Fix applied:** Added `isCreate bool` flag to `UpsertPreference`. When `GetUserPreference` returns `pgx.ErrNoRows`:
- `isCreate = true` → `AuditActionCreate` with only `new_data` (valid per constraint — `old_data IS NULL` is allowed for `create` actions)

When an existing row is found:
- `isCreate = false` → `AuditActionUpdate` with both `old_data` and `new_data` (constraint satisfied)

**Review status: PASS.** First-time and update paths now both satisfy the audit constraint.

##### Defect 2 — DrainOutbox N+1 Preference Queries (Performance FAIL)

**Problem identified:** The drain loop issued one `GetUserPreference` SQL query per `(outbox_entry × org_member)` pair. At 1,000 org members with 100 pending entries, this is 100,000 sequential round-trips in one transaction. At the 100,000-user scale target, it would produce 10,000,000 queries per drain call — incompatible with production operation.

**Fix applied:** Added `GetNotificationPreferencesForEvent` — a single query returning all preference rows for a given `(organization_id, event_type, channel)`. Before the fan-out loop, `DrainOutbox` collects unique `(event_type, channel)` pairs from pending entries and issues one batch query per unique pair. The result is loaded into an in-memory `map[[16]byte]bool` (userID bytes → enabled). The inner fan-out loop resolves preferences from the map with zero SQL.

**Query complexity:**

| Org size | Preference queries — Before | Preference queries — After |
|----------|----------------------------|---------------------------|
| 100 users, 100 entries | 10,000 | ≤ 9 (one per event type) |
| 1,000 users, 100 entries | 100,000 | ≤ 9 |
| 10,000 users, 100 entries | 1,000,000 | ≤ 9 |
| 100,000 users, 100 entries | 10,000,000 | ≤ 9 |

Database query count for preference resolution is now bounded by the ENUM cardinality (9 event types), independent of organization size.

**Review status: PASS.** No SQL executes inside the per-member loop. Preference resolution is memory-only after the batch load.

#### Phase 12 Production Hardening Review (adversarial, all PASS after hotfixes)

| Check | Result |
|-------|--------|
| Multi-tenant isolation | **PASS** |
| Personal data isolation (org_id + user_id on every query) | **PASS** |
| Outbox correctness (atomic with domain write) | **PASS** |
| Drain idempotency (UNIQUE outbox_id+user_id+channel) | **PASS** |
| FOR UPDATE SKIP LOCKED (concurrent drain safety) | **PASS** |
| Match CAS correctness (no stale status overwrites) | **PASS** |
| Notification preferences — first-time create | **PASS** (hotfix applied) |
| Notification preferences — update existing | **PASS** |
| Audit correctness (AuditActionCreate vs AuditActionUpdate) | **PASS** (hotfix applied) |
| Read / read-all idempotency | **PASS** |
| Delete correctness (double-delete → 404) | **PASS** |
| Delivery consistency (drain-failure isolation) | **PASS** |
| Performance — preference query complexity | **PASS** (hotfix applied; O(event types) not O(members)) |

---

### Auth Security Hotfix v2 — Refresh Token Replay Redesign (Complete)

**Status: COMPLETE. Adversarial review: PASS. Production readiness: PASS.**

#### Problem

The previous replay detection in `RotateRefreshToken` used a 30-second grace window (`rotationWindow = 30 * time.Second`). When a revoked token was presented, the logic compared `time.Since(revoked_at)` against the window:

- If within 30 seconds → `ErrInvalidToken` (no wipe) — assumed concurrent race
- If older than 30 seconds → `ErrTokenReuse` (full wipe) — assumed replay attack

This design had two failure modes:

1. **False negative (security hole):** A stolen token replayed within 30 seconds of the legitimate rotation was silently accepted as a race — no session wipe, the attacker's request returned the same error as a legitimate retry.
2. **False positive (DoS on user sessions):** Any legitimate concurrent refresh (two browser tabs, two devices) that landed more than 30 seconds apart triggered a full session wipe, locking the user out everywhere.

The 30-second window was an arbitrary tuning parameter with no correct value. Under load (connection pool saturation, high DB latency), even legitimate sequential retries could breach the window.

#### Solution

Migration 000023 adds `successor_id UUID NULL` to `refresh_tokens`. When a token is rotated, `RevokeAndLinkSuccessor` atomically sets both `revoked_at = NOW()` and `successor_id = new_token.ID` in a single UPDATE. `successor_id` is a **state classification marker**, not a navigable reference — the application only ever tests `IS NULL` or `IS NOT NULL`. No FK, no ON DELETE behaviour.

The `rotationWindow` constant and all `time.Since(...)` comparisons were removed. The replay state machine (see Section 3) is entirely structural.

#### Transaction Order Change

The new rotation transaction inserts the new token **before** revoking the old one, because `RevokeAndLinkSuccessor` needs the new token's ID as a parameter. If the insert fails, the old token is never revoked and remains valid. If the atomic revoke+link fails, the inserted new token is rolled back. No orphaned state in any failure path.

#### Rows-Affected Invariant

`RevokeAndLinkSuccessor` is declared `:execrows` (returns `int64`). The caller (`RotateRefreshToken`) asserts `rowsAffected == 1` after every call. Under correct execution this assertion is always true — the `FOR UPDATE` lock prevents any concurrent modification of the old token row between the state check and the UPDATE. The assertion is a defense-in-depth guard against future bugs (e.g., calling the function without first holding the lock). Any other count returns a hard error and rolls back the transaction.

#### Files Changed

| File | Change |
|------|--------|
| `db/migrations/000023_refresh_token_successor_tracking.up.sql` | ADD COLUMN `successor_id UUID NULL`; ADD CONSTRAINT `chk_refresh_tokens_successor` |
| `db/migrations/000023_refresh_token_successor_tracking.down.sql` | DROP CONSTRAINT; DROP COLUMN |
| `db/queries/auth.sql` | Added `RevokeAndLinkSuccessor :execrows` query |
| `db/sqlc/auth.sql.go` | Regenerated — `RevokeAndLinkSuccessorParams`, `RevokeAndLinkSuccessor() (int64, error)`; all SELECT scans include `SuccessorID` |
| `db/sqlc/models.go` | Regenerated — `RefreshToken` struct gains `SuccessorID pgtype.UUID` |
| `internal/auth/repository.go` | Removed `rotationWindow` constant; removed `time.Since(...)` comparison; rewrote `RotateRefreshToken` with new state machine; added `"fmt"` import |

No changes to: `service.go`, `handler.go`, `errors.go`, `dto.go`, `tokens.go`, `passwords.go`, `middleware.go`. Service interface is unchanged.

#### Adversarial Review Results

| Check | Result |
|-------|--------|
| Concurrent refresh requests (same token) | **PASS** |
| Concurrent logout + refresh | **PASS** |
| Concurrent logout-all + refresh | **PASS** |
| Rotated token replay | **PASS** |
| Explicitly revoked token replay | **PASS** |
| Rows-affected invariant | **PASS** |
| Transaction rollback behavior (all exit paths) | **PASS** |
| `successor_id` state transitions | **PASS** |
| Remaining replay-detection bypasses | **PASS** |
| Session-revocation bypasses | **PASS** |

No defects introduced. Two pre-existing conditions were identified and logged for Phase 13A:

1. **Logout vs concurrent rotation race (Low):** `service.Logout` uses an unlocked pre-fetch and a plain `RevokeRefreshToken` with no transaction. If a rotation commits between the pre-fetch and the revoke, the revoke lands on an already-rotated token (0 rows affected, no error) and the newly-issued successor token remains active. The user receives HTTP 200 for the logout but their concurrent rotation's token survives. Exploitation requires sub-millisecond concurrent timing and the attacker to already possess the original token.

2. **User suspension vs in-flight refresh (Low):** `assertUserActive` is called in `service.Refresh` before the rotation transaction begins. A suspension that arrives after the status check but before the rotation commits allows one final refresh token to be issued. The attacker has no refresh token to rotate and the issued access token expires in 15 minutes. The next refresh will fail the status check.

Both are Phase 13A hardening items.

---

### Phase 13A — Security & Production Blockers (Complete)

**Status: COMPLETE. Adversarial review: all 12 checks PASS. Production readiness: PASS.**

Phase 13A closed all outstanding production blockers and both low-severity auth races from the Hotfix v2 review.

#### Part 1 — Password Reset

Migration 000024 adds `password_reset_tokens` (id, user_id FK, token_hash UNIQUE, expires_at, used_at, created_at). Tokens expire after 1 hour. The raw token is never stored; only the SHA-256 hash is persisted.

**ForgotPassword flow** (`POST /api/v1/auth/forgot-password`):
- Always returns HTTP 200 with the same message regardless of whether the email exists (prevents user-enumeration)
- In development, the raw token is returned in the response body; in production the field is stripped by the handler (`IsDevelopment()` gate, identical to the verification token pattern)
- Token creation and audit record written atomically in `ForgotPasswordTransaction`

**ResetPassword flow** (`POST /api/v1/auth/reset-password`) — single atomic transaction:
1. `GetPasswordResetTokenByHashForUpdate` — acquires `FOR UPDATE` row lock; serialises concurrent consumption attempts
2. Validates `used_at IS NULL` and `expires_at > NOW()`
3. `UsePasswordResetToken(token.ID)` — marks the presented token used
4. `UseAllUserPasswordResetTokens(token.UserID)` — invalidates all other outstanding tokens for the user; prevents stale tokens from being used after the password changes
5. `UpdateUserPasswordHash` — replaces the bcrypt hash
6. `RevokeUserRefreshTokens` — revokes all active sessions; `successor_id` stays NULL (Case 3 semantics from Hotfix v2)
7. `CreateAuditLog` — `AuditActionUpdate` on `users` entity with both old_data and new_data
8. COMMIT

#### Part 2 — Rate Limiting

`internal/platform/middleware/ratelimit.go` — per-IP token-bucket rate limiter using `golang.org/x/time/rate`. Replaces the stub.

- `IPRateLimiter` struct: `sync.Mutex`-protected `map[string]*ipEntry`; each entry holds a `*rate.Limiter` and `lastSeen time.Time`
- Background goroutine prunes idle entries (not seen for > 10 min) every 5 minutes; memory bounded by `O(active unique IPs)`
- `Stop()` closes a done channel; safe to call multiple times
- Applied to the entire `/api/v1/auth` route group via `r.Use(limiter.Middleware())`
- Config: `RATE_LIMIT_ENABLED` (default true), `RATE_LIMIT_AUTH_RPS` (default 10 req/s), `RATE_LIMIT_AUTH_BURST` (default 20)
- Returns 429 with `{"error":"rate limit exceeded"}` on exhaustion

#### Part 3 — CORS Wiring

`middleware.CORS(cfg.CORSAllowedOrigins)` mounted in `bootstrap/router.go` (after `chimw.RealIP`, before route handlers).

- `Access-Control-Allow-Credentials: true` added; sent only when a specific origin is matched (not for wildcard-only mode, which would be rejected by browsers)
- `Vary: Origin` header always appended
- Config: `CORS_ALLOWED_ORIGINS` (comma-separated; defaults to `http://localhost:3000,http://localhost:5173`)

#### Part 4 — Cleanup Scheduler

`internal/cleanup/scheduler.go` — background token expiry scheduler.

- `Scheduler` struct: `*db.Queries`, configurable `interval`, slog logger, done channel
- `Start()` launches a single goroutine; `Stop()` signals exit (safe to call multiple times)
- `runOnce()` runs under a 30-second context timeout; calls `DeleteExpiredRefreshTokens`, `DeleteExpiredEmailVerificationTokens`, `DeleteExpiredPasswordResetTokens` independently — a failure in one does not prevent the others
- Ticker fires every `CLEANUP_INTERVAL_MINUTES` (default 60); missed ticks from slow cycles are dropped (correct — idempotent cleanup)
- Constructed and started in `App.Handler()`; stopped in `App.Shutdown()` (called from `main.go` before `srv.Shutdown()`)
- Config: `CLEANUP_INTERVAL_MINUTES` (default 60)

#### Part 5 — Auth Hardening Backlog

**Fix 1 — Logout vs concurrent rotation race:**
`service.Logout` previously did an unlocked `GetRefreshTokenByHash` → `RevokeRefreshToken`, leaving a window where a concurrent rotation could issue a successor token that the revoke never reached.

New: `LogoutTransaction` wraps the operation in a `BEGIN` / `GetRefreshTokenByHashForUpdate` (FOR UPDATE) / `RevokeRefreshToken` / `COMMIT` sequence. The rotation transaction and the logout transaction now compete for the same row lock — only one can proceed. If rotation won, logout sees `revoked_at IS NOT NULL` and returns success (idempotent). If logout won, any subsequent rotation sees Case 3 (`revoked_at IS NOT NULL, successor_id IS NULL`) → `ErrTokenReuse` → all sessions wiped.

**Fix 2 — User suspension vs in-flight refresh:**
`assertUserActive` previously ran in `service.Refresh` before the rotation transaction began. The window between that check and the `COMMIT` allowed a suspension to slip through.

New: `GetUserByID` is called inside `RotateRefreshToken` after the `FOR UPDATE` lock on the token row is acquired (Step 3b). This is a READ COMMITTED read — it sees the latest committed user state. The check happens 2–3 DB round-trips before the commit rather than potentially hundreds of milliseconds earlier.

#### Files Changed (Phase 13A)

| File | Change |
|------|--------|
| `db/migrations/000024_password_reset_tokens.{up,down}.sql` | New — password_reset_tokens table + indexes |
| `db/queries/password_reset_tokens.sql` | New — 5 queries: Create, GetForUpdate, UseOne, UseAll, DeleteExpired |
| `db/queries/users.sql` | Added `UpdateUserPasswordHash :exec` |
| `db/sqlc/password_reset_tokens.sql.go` | Generated — 5 query functions |
| `db/sqlc/users.sql.go` | Regenerated — `UpdateUserPasswordHash` added |
| `db/sqlc/models.go` | Regenerated — `PasswordResetToken` struct added |
| `internal/auth/errors.go` | Added `ErrResetTokenInvalid`, `ErrResetTokenExpired`, `ErrResetTokenUsed` |
| `internal/auth/tokens.go` | Added `passwordResetTokenDuration = 1h`, `GetPasswordResetTokenExpiryTime()` |
| `internal/auth/dto.go` | Added `ForgotPasswordRequest`, `ForgotPasswordResponse`, `ResetPasswordRequest` |
| `internal/auth/repository.go` | Added `LogoutTransaction`, `ForgotPasswordTransaction`, `ResetPasswordTransaction`, `DeleteExpiredPasswordResetTokens`; added user-status re-check inside `RotateRefreshToken` (Step 3b) |
| `internal/auth/service.go` | Added `ForgotPassword`, `ResetPassword`; rewired `Logout` → `LogoutTransaction` |
| `internal/auth/handler.go` | Added `ForgotPassword`, `ResetPassword` handlers; extended `writeAuthError` and `errorKind` |
| `internal/auth/routes.go` | Added `/forgot-password`, `/reset-password`; wired `limiter.Middleware()` onto auth group; added `limiter` parameter |
| `internal/platform/config/config.go` | Added `CORSAllowedOrigins`, `RateLimitEnabled/RPS/Burst`, `CleanupIntervalMinutes`; added `getEnvBool/Int/Float/StringSlice` helpers |
| `internal/platform/middleware/cors.go` | Added `Access-Control-Allow-Credentials`, `Vary: Origin`; wildcard-only mode disables credentials header |
| `internal/platform/middleware/ratelimit.go` | Full implementation (replaces stub); per-IP limiter + background cleanup goroutine |
| `internal/cleanup/scheduler.go` | New package — background token cleanup scheduler |
| `internal/bootstrap/app.go` | Added `scheduler`, `rateLimiter` fields; `Handler()` initialises both; `Shutdown()` stops both |
| `internal/bootstrap/router.go` | Wired CORS middleware; added `limiter` parameter |
| `internal/bootstrap/modules.go` | Added `limiter` parameter; passes it to `auth.RegisterRoutes` |
| `cmd/api/main.go` | Calls `app.Shutdown(ctx)` before `srv.Shutdown(ctx)` |
| `go.mod` / `go.sum` | Added `golang.org/x/time v0.15.0` |

#### Phase 13A Adversarial Review Results

| Check | Result |
|-------|--------|
| Password reset replay resistance | **PASS** |
| Concurrent consumption of the same token | **PASS** |
| Concurrent reset requests for same user | **PASS** (PostgreSQL deadlock detected and resolved; no data corruption) |
| Session revocation on password reset | **PASS** |
| Forgot-password enumeration resistance | **PASS** (body/status; timing side-channel documented as low-severity) |
| Rate limiter concurrency correctness | **PASS** |
| Rate limiter memory cleanup | **PASS** |
| Scheduler overlap behavior | **PASS** |
| Scheduler shutdown behavior | **PASS** |
| Logout vs rotation race fix | **PASS** |
| Suspension vs refresh race fix | **PASS** |
| CORS correctness | **PASS** |

Two low-severity findings documented:
1. **Concurrent same-user reset token deadlock** — two tokens for the same user presented simultaneously can deadlock; PostgreSQL handles this correctly (one aborts, one succeeds, no data corruption). Fix: acquire token locks in deterministic order. Low priority.
2. **Timing side-channel in ForgotPassword** — "not found" path is faster than "found" path by ~1 DB query. Body and status code are enumeration-resistant. Timing equalization deferred.

---

### Phase 13B.1 — Test Infrastructure & Deferred Fixes (Complete)

**Status: COMPLETE. Adversarial review: PASS. Production readiness: PASS.**

Phase 13B.1 delivered the testcontainers-based integration-test infrastructure for the auth package and resolved two low-severity findings deferred from Phase 13A.

#### Deferred Fix 1 — Password Reset Deadlock

**Problem (Phase 13A finding):** `ResetPasswordTransaction` locked one token row via `GetPasswordResetTokenByHashForUpdate`, then called `UseAllUserPasswordResetTokens` which needed to UPDATE all sibling rows for the user. Two concurrent reset attempts each holding a different token could create a lock cycle: T1 held token A waiting for B; T2 held token B waiting for A → PostgreSQL deadlock.

**Fix:** Added `LockUserPasswordResetTokens :many` query:

```sql
SELECT * FROM password_reset_tokens
WHERE  user_id = $1
  AND  used_at IS NULL
ORDER BY id
FOR UPDATE;
```

`ResetPasswordTransaction` was rewritten with the following sequence:

1. Non-locking pre-fetch (`GetPasswordResetTokenByHash`) to obtain `user_id` and `token.ID`.
2. `BEGIN`.
3. `LockUserPasswordResetTokens(userID)` — locks all outstanding tokens for the user in ascending `id` order. Both concurrent callers attempt locks in the same sequence; one blocks on the first row instead of both blocking on each other's row.
4. Find target token by ID in the locked set. Not found → `ErrResetTokenUsed` (consumed between steps 1 and 3).
5. Validate `expires_at > NOW()`. Expired → `ErrResetTokenExpired` (ROLLBACK; sibling tokens remain available).
6. `UsePasswordResetToken` + `UseAllUserPasswordResetTokens` → mark all outstanding tokens used.
7. `UpdateUserPasswordHash`, `RevokeUserRefreshTokens`, `CreateAuditLog`.
8. `COMMIT`.

**Review:** Lock ordering is deterministic on UUIDs (PostgreSQL sorts UUIDs consistently). No cycle is possible. `TestConcurrentResetDifferentTokens` is the regression test: a deadlock error is not `ErrResetTokenUsed`, so the test fails if deadlock reappears.

#### Deferred Fix 2 — Forgot-Password Timing Equalization

**Problem (Phase 13A finding):** The "email not found" path in `ForgotPassword` returned `genericResp` after only one failed DB read, several milliseconds faster than the "email found + success" path (which completed a `BEGIN / INSERT / INSERT / COMMIT` transaction). A persistent adversary could enumerate registered addresses by measuring response latency.

**Fix:** `equalizeEnumerationTiming()` — an auth-specific private method on `Repository`:

```
BEGIN
SELECT 1
SELECT NOW()
COMMIT
```

This matches the round-trip profile of `ForgotPasswordTransaction` (4 round-trips each), eliminating the measurable latency gap under normal DB operation. Two query slots are used rather than one to match the two-write profile of the real transaction. The method is called on both the email-not-found path and the `ForgotPasswordTransaction` internal-error path. Errors are intentionally swallowed (correct fail-open behavior for a timing measure).

**Known low-severity residual:** The `ForgotPasswordTransaction` error path calls `equalizeEnumerationTiming` after the partial transaction has already consumed ≥ 1 round-trip, making its total response time ≥ 6 round-trips vs. the success path's 5. This timing difference is only observable when the DB is actively failing (not under attacker control) and is accepted as a low-severity residual. Does not block production readiness.

#### Test Infrastructure

**New package `internal/testutil/`:**

- `SetupTestDB(m *testing.M)` starts a `postgres:17-alpine` container via testcontainers-go, applies all migrations using golang-migrate + `iofs` source (embedded via `db/migrations/embed.go`) + pgx/v5 driver, creates a `pgxpool.Pool`, and returns a teardown function.
- Docker-unavailable policy: `INTEGRATION_SKIP_IF_DOCKER_UNAVAILABLE=true` → exit 0 (local dev skip). Env var absent → `log.Fatal` (CI pipeline fails loudly with zero tests run).

**New package `internal/testutil/fixtures/`:**

Typed helpers for UUID-scoped fixture creation with explicit `t.Cleanup` teardown. No transaction-per-test (repository code calls `pool.Begin()` internally and must observe its own writes).

| Helper | Purpose |
|--------|---------|
| `CreateActiveUser` | Inserts user with `status=active`; random email/username; bcrypt cost 4 for speed |
| `CreateRefreshToken` | Inserts un-revoked refresh token; returns raw value + db row |
| `CreatePasswordResetToken` | Inserts valid (unused, 1-hour expiry) reset token; returns raw value |
| `CreateExpiredPasswordResetToken` | Raw INSERT back-dating both `created_at` and `expires_at` to satisfy `CHECK (expires_at > created_at)` while producing an already-expired token |
| `CleanupUser` | `DELETE FROM users WHERE id = $1`; CASCADE removes all auth rows |
| `HashToken` | SHA-256 hex helper matching `auth.HashTokenForStorage`; avoids import cycle |

**`db/migrations/embed.go`:** Exports `migrations.FS embed.FS` via `//go:embed *.sql` for the test runner. Not imported by the production binary.

#### Auth Concurrency Tests (`internal/auth/`)

All tests use `close(chan struct{})` barrier synchronization. No `time.Sleep`.

| Test | What it verifies |
|------|-----------------|
| `TestConcurrentRefreshSameToken` | 8 goroutines rotate the same token; exactly 1 succeeds; rest return `ErrInvalidToken` (Case 2) |
| `TestReplayRotatedToken` | Re-presenting a rotated token returns `ErrInvalidToken` without wiping sessions |
| `TestReplayRevokedToken` | Re-presenting a logout-revoked token returns `ErrTokenReuse`; all sibling sessions wiped |
| `TestConcurrentLogoutAndRefresh` | Simultaneous logout + rotation; token always ends up revoked; no panics |
| `TestConcurrentResetSameToken` | 6 goroutines consume the same reset token; exactly 1 succeeds; rest return `ErrResetTokenUsed` |
| `TestConcurrentResetDifferentTokens` | Two goroutines present different tokens for the same user; exactly 1 succeeds; loser gets `ErrResetTokenUsed` not a deadlock error (deadlock regression test) |
| `TestConcurrentResetExpiredAndValidToken` | Expired + valid tokens for the same user submitted concurrently; valid always succeeds; expired always fails with `ErrResetTokenExpired` or `ErrResetTokenUsed` depending on lock order |
| `TestRevokeAndLinkSuccessorInvariant` | After rotation: old token has `revoked_at IS NOT NULL` and `successor_id = new.ID`; new token has both NULL (active) |

#### Files Changed (Phase 13B.1)

| File | Change |
|------|--------|
| `db/queries/password_reset_tokens.sql` | Added `GetPasswordResetTokenByHash :one` (non-locking pre-fetch) and `LockUserPasswordResetTokens :many` (ORDER BY id FOR UPDATE) |
| `db/sqlc/password_reset_tokens.sql.go` | Regenerated — two new query functions |
| `db/migrations/embed.go` | New — exports `migrations.FS` for test runners |
| `internal/auth/repository.go` | `ResetPasswordTransaction` rewritten (deterministic lock order); `equalizeEnumerationTiming` added |
| `internal/auth/service.go` | `ForgotPassword` calls `equalizeEnumerationTiming` on email-not-found and `ForgotPasswordTransaction` error paths |
| `internal/auth/testmain_test.go` | New — `TestMain` with per-package container lifecycle |
| `internal/auth/repository_test.go` | New — `TestRevokeAndLinkSuccessorInvariant` |
| `internal/auth/concurrency_test.go` | New — 7 concurrency tests |
| `internal/testutil/container.go` | New — `SetupTestDB` |
| `internal/testutil/fixtures/auth.go` | New — typed fixture helpers |
| `go.mod` / `go.sum` | Added `testcontainers-go v0.42.0`, `testcontainers-go/modules/postgres`, `golang-migrate/migrate/v4 v4.19.1` |

#### Phase 13B.1 Adversarial Review Results

| Check | Result |
|-------|--------|
| Password-reset deadlock fix | **PASS** |
| Lock ordering correctness (UUID ORDER BY determinism) | **PASS** |
| Concurrent reset requests (same token, different tokens) | **PASS** |
| Concurrent reset-token consumption | **PASS** |
| Refresh-token invariant test coverage | **PASS** |
| Timing equalization (not-found and success paths) | **PASS** |
| Timing equalization (transaction-error path) | **PASS with note** — path is equalized but over-compensated; see known residual below |
| Testcontainers lifecycle | **PASS** |
| Migration runner correctness | **PASS** |
| Fixture cleanup isolation | **PASS** |
| Concurrency test validity | **PASS** |

**Known low-severity residual (does not block production readiness):**

The `ForgotPasswordTransaction` internal-error path calls `equalizeEnumerationTiming` after the partial transaction has already consumed at least one DB round-trip, making its response consistently longer than the success path. This timing difference is only observable when the database is actively failing — a condition that is not adversarially controllable and that elevates latency globally. Future hardening candidate.

---

### Phase 13B.2A — Auth Integration Tests: Core Suite (Complete)

**Status: COMPLETE. Adversarial review: all checks PASS. Production readiness: PASS.**

Phase 13B.2A delivered the primary auth integration test suite in `internal/auth/integration/` — 66 tests across 10 test files, all running against a live PostgreSQL 17 instance (testcontainers-go) via the full HTTP stack.

#### New Fixtures (added to `internal/testutil/fixtures/auth.go`)

| Helper | Purpose |
|--------|---------|
| `CreatePendingUser` | Inserts user in `pending_verification` state with an email-verification token |
| `CreateSuspendedUser` | Inserts active user then sets `status = suspended` |
| `CreateInactiveUser` | Inserts active user then sets `status = inactive` |
| `CreatePlatformAdmin` | Inserts active user and grants `platform_admin` role (`organization_id = NULL`) |
| `CreateOrgWithRole` | Creates org, looks up role by slug, grants to user; registers `t.Cleanup` for org deletion; returns org UUID string |
| `CreateExpiredRefreshToken` | Raw INSERT backdating `expires_at` and `created_at` (satisfies table CHECK while expiring the token) |
| `CreateExpiredEmailVerificationToken` | Raw INSERT backdating both columns; satisfies `CHECK (expires_at > created_at)` |

#### New Test Infrastructure (`internal/auth/integration/`)

| File | Purpose |
|------|---------|
| `testmain_test.go` | `TestMain` wiring `testutil.SetupTestDB` and a shared `testPool` for the integration package |
| `server_test.go` | `buildTestServer` — constructs a real chi router + auth routes against the test pool; `testJWTSecret` constant |
| `helpers_test.go` | `assertStatus`, `assertErrorBody`, `decodeBody`, `bearerHeader`, response type structs |
| `token_helpers_test.go` | API-level flow helpers (`apiLogin`, `apiRegister`, `apiMe`, `apiRefresh`, `apiLogout`, `apiForgotPassword`, `apiResetPassword`, `apiVerifyEmail`); token construction helpers (`makeExpiredToken`, `makeTamperedToken`, `makeAlgorithmConfusionToken`, `makeWrongKeyToken`, `makeEmptyUserIDToken`, `makeWrongIssuerToken`) |

#### Test Coverage (66 tests)

| File | Count | Coverage focus |
|------|------:|----------------|
| `lifecycle_test.go` | 11 | Register (success, duplicate email, duplicate username); login (success, wrong password, unknown email); logout (success, revokes token, empty token); me (success); full flow smoke test |
| `middleware_test.go` | 7 | RequireAuth: no header, expired token, tampered token, algorithm confusion (HS512), wrong issuer; RequirePermission: granted, denied |
| `refresh_test.go` | 8 | Success, token rotation, Case 2 rotated-token replay (no session wipe), Case 3 revoked-token replay (full wipe), expired token, invalid token, suspended user blocked, sessions revoked by password reset |
| `password_reset_test.go` | 10 | Forgot-password known/unknown email; reset success; reset expired/used/invalid token; all-sessions revoked; old-password fails after reset; new-password works; concurrent HTTP reset |
| `email_verification_test.go` | 7 | Verify success; enables login; invalid/expired/used token; missing token parameter; concurrent HTTP verification |
| `suspension_test.go` | 5 | Pending-verification blocks login; suspended blocks login; inactive blocks login; suspension blocks login (setup-time); suspension blocks mid-flight refresh |
| `multitenant_test.go` | 6 | Single-org auto-select; multi-org 409 with org list; multi-org explicit login; wrong org ID rejected; platform admin login (empty org_id); cross-org refresh denied |
| `concurrency_test.go` | 3 | Concurrent refresh (HTTP barrier sync); concurrent logout + refresh (HTTP); concurrent password reset different tokens (HTTP) |
| `cors_test.go` | 4 | Preflight allowed origin; preflight disallowed origin; request allowed origin; request disallowed origin |
| `rate_limit_test.go` | 5 | Login exhaustion (429); register exhaustion; refresh exhaustion; forgot-password exhaustion; all auth routes share same IP bucket |
| **Total** | **66** | |

All tests use `t.Parallel()` (except `TestLogin_UnknownEmail` which avoids bcrypt cost-12 contention). All fixture teardown uses `t.Cleanup`. No `time.Sleep`.

#### Adversarial Review Results

Adversarial review performed against the complete 66-test suite. All regression gates were validated: each test was confirmed to catch the specific regression it is designed for (i.e., the test would fail if the guarded check in the production code were removed).

| Check | Result |
|-------|--------|
| Full lifecycle happy path (register → verify → login → me → refresh → logout) | **PASS** |
| Credential rejection and anti-enumeration | **PASS** |
| Token expiry rejection (exp claim) | **PASS** |
| Tampered-token rejection (signature) | **PASS** |
| Algorithm confusion prevention (HS512 rejected) | **PASS** |
| Wrong-issuer rejection | **PASS** |
| RBAC permission grant / deny at HTTP layer | **PASS** |
| Account-status gates (pending, suspended, inactive) | **PASS** |
| Multi-tenant org context resolution | **PASS** |
| Single-org auto-select and multi-org picker (409) | **PASS** |
| Cross-org refresh denied | **PASS** |
| Refresh-token state machine (Case 2 / Case 3) via HTTP | **PASS** |
| Password reset flow end-to-end | **PASS** |
| Email verification flow end-to-end | **PASS** |
| Concurrent HTTP refresh safety | **PASS** |
| Concurrent HTTP password reset safety | **PASS** |
| Rate-limit exhaustion (429) and IP-bucket sharing | **PASS** |
| CORS preflight and request header correctness | **PASS** |
| Regression gate correctness (all 66 gates validated) | **PASS** |

No defects requiring code fixes were identified during the adversarial review.

---

### Phase 13B.2B-A — Auth Integration Tests: JWT Claims & Security Invariants (Complete)

**Status: COMPLETE. Adversarial review: all checks PASS. Production readiness: PASS.**

Phase 13B.2B-A added 6 targeted tests that each protect a named security invariant in the auth middleware and session lifecycle. All tests use real fixture users (establishing a 200-baseline before the negative case) so the 401/403 cannot originate from user-not-found.

#### New File

`internal/auth/integration/jwt_test.go` — 3 tests covering JWT token validation edge cases.

#### Tests Added

| Test | File | Security invariant protected |
|------|------|------------------------------|
| `TestJWT_WrongSigningKey` | `jwt_test.go` | Signature verification — a JWT signed with the wrong HMAC secret is rejected at `ParseToken` before any handler runs; regression gate: if keyFunc bypassed, the real-user baseline returns 200 instead of 401 |
| `TestJWT_EmptyUserIDClaim` | `jwt_test.go` | `claims.UserID == ""` check in `ParseToken` (`tokens.go:101-103`) — a token with user_id = "" is rejected by the middleware; regression gate: removal causes `uid.Scan("")` to fail in the handler returning "unauthorized" instead of "authorization required" |
| `TestJWT_EmptyEmailClaim` | `jwt_test.go` | `claims.Email == ""` check in `ParseToken` (`tokens.go:104-106`) — a token with email = "" and a valid user_id is rejected by the middleware; regression gate: removal allows the handler to find the user by user_id and return 200, failing `assertStatus(401)` |
| `TestLogout_Idempotent` | `lifecycle_test.go` | `LogoutTransaction` idempotency — presenting an already-revoked refresh token to `/logout` returns 200 (not 401 or 500); state-corruption guard: the token must remain revoked after the second call |
| `TestMultiTenant_RoleRevocationDeniesPermission` | `multitenant_test.go` | `HasPermission` live-DB query — revoking all `user_organization_roles` for a user immediately denies permission-gated endpoints; JWT remains valid (`/me` still returns 200), so blast radius is limited to permission-checked routes |
| `TestMultiTenant_OrgMembershipRemovalDeniesPermission` | `multitenant_test.go` | Same `HasPermission` EXISTS query from the org-scoped membership angle — deleting one specific `(user_id, organization_id)` row denies access; confirms no caching path exists |

#### New Token Helper

`makeEmptyEmailToken` added to `token_helpers_test.go` — generates a correctly-signed HS256 JWT with `email = ""` and a valid `user_id`. Pattern mirrors `makeEmptyUserIDToken`.

#### Adversarial Review Results

| Check | Result |
|-------|--------|
| Signature verification gate (`TestJWT_WrongSigningKey`) | **PASS** |
| Empty `user_id` claim gate (`TestJWT_EmptyUserIDClaim`) | **PASS** |
| Empty `email` claim gate (`TestJWT_EmptyEmailClaim`) | **PASS** |
| Logout idempotency and state-corruption guard (`TestLogout_Idempotent`) | **PASS** |
| Live-DB permission check — role revocation (`TestMultiTenant_RoleRevocationDeniesPermission`) | **PASS** |
| Live-DB permission check — membership removal (`TestMultiTenant_OrgMembershipRemovalDeniesPermission`) | **PASS** |
| JWT validity independent of role revocation (JWT-only `/me` still passes post-revocation) | **PASS** |
| `go fmt`, `go vet`, `go build` clean | **PASS** |
| Full test suite (`go test ./...`) passing | **PASS** |

No defects requiring code fixes were identified. `tokens.go` already contained the `claims.Email == ""` guard; the test validates and gates it.

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

**Initial fix (Phase 8A):** Added a NOT IN terminal-state guard:

```sql
AND  status NOT IN ('completed', 'cancelled', 'abandoned')
```

**Phase 12B upgrade — Compare-And-Swap:** The NOT IN guard was replaced with a strict compare-and-swap that asserts the match is still in the exact state the service observed before entering the transaction:

```sql
-- UpdateMatch
AND  status = $20    -- $20 = previous_status (Status_2 in sqlc params)

-- CancelMatch
AND  status = $3     -- $3 = previous_status
```

The CAS is strictly stronger than the NOT IN guard: it rejects any concurrent transition, not only those to terminal states. This was required for Phase 12 outbox correctness — the outbox entry must carry the verified previous and new status values, which is only guaranteed when the write is CAS-protected.

**Review status: PASS.** Concurrent requests that both pass the service-layer check cannot both write to the same match row if the first write transitions the match to any other status.

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
- [x] **Notifications module** — Phase 12 (Transactional Outbox Pattern; `notification_outbox`/`notifications`/`notification_preferences` tables; 7 API endpoints; in-app delivery via synchronous post-commit DrainOutbox with FOR UPDATE SKIP LOCKED; drain idempotency via UNIQUE(outbox_id,user_id,channel); match CAS upgrade; batch preference loading — O(event types) queries not O(members); adversarial review: 2 defects identified and resolved; all 13 checks PASS)
- [x] **Auth Security Hotfix v2** — Replaced time-window refresh-token replay detection with deterministic structural state machine; `successor_id UUID NULL` column on `refresh_tokens` (migration 000023); `RevokeAndLinkSuccessor :execrows` query; removed `rotationWindow` constant and all `time.Since(...)` logic; `RotateRefreshToken` rewritten with 3-case state machine; rows-affected invariant enforced; adversarial review: all 10 checks PASS; no defects introduced
- [x] **Phase 13A — Security & Production Blockers** — Password reset flow (`POST /forgot-password` / `POST /reset-password`; migration 000024; single-use `FOR UPDATE` transaction; atomic sibling-token invalidation; full session revocation; audit logging); per-IP rate limiting (`golang.org/x/time/rate`; configurable RPS + burst; background cleanup; 429 on exhaustion); CORS wired (`Access-Control-Allow-Credentials`, `Vary: Origin`; wildcard-only mode disables credentials); cleanup scheduler (`internal/cleanup/`; hourly by default; graceful shutdown via done channel); logout race fixed (`LogoutTransaction` with FOR UPDATE); suspension race fixed (user status re-checked inside `RotateRefreshToken` transaction); adversarial review: all 12 checks PASS; two low-severity findings documented
- [x] **Phase 13B.1 — Test Infrastructure & Deferred Fixes** — testcontainers-go integration-test infrastructure (`internal/testutil/`); per-package `postgres:17-alpine` container; golang-migrate + embedded migrations; UUID-scoped fixtures with `t.Cleanup` isolation (`internal/testutil/fixtures/`); password-reset deadlock fixed via `LockUserPasswordResetTokens ORDER BY id FOR UPDATE` (deterministic lock order eliminates concurrent same-user reset cycle); forgot-password timing equalization strengthened (`equalizeEnumerationTiming` — 4-round-trip profile matching `ForgotPasswordTransaction`, called on email-not-found and transaction-error paths); 8 auth concurrency tests covering refresh replay, logout race, reset deadlock regression, and `RevokeAndLinkSuccessor` invariant; adversarial review: all 11 checks PASS; one low-severity residual documented (transaction-error path over-compensated, does not block production readiness)
- [x] **Phase 13B.2A — Auth Integration Tests: Core Suite** — 66-test integration suite in `internal/auth/integration/`; 10 test files covering lifecycle, middleware, refresh, password-reset, email-verification, suspension, multi-tenant, concurrency, CORS, and rate-limiting flows; 7 new fixture helpers (`CreatePendingUser`, `CreateSuspendedUser`, `CreateInactiveUser`, `CreatePlatformAdmin`, `CreateOrgWithRole`, `CreateExpiredRefreshToken`, `CreateExpiredEmailVerificationToken`); all tests run against a live PostgreSQL 17 instance; adversarial review: all 19 checks PASS; no defects
- [x] **Phase 13B.2B-A — Auth Integration Tests: JWT Claims & Security Invariants** — 6 targeted tests protecting named security invariants: JWT wrong-key rejection, empty `user_id` claim rejection, empty `email` claim rejection (`TestJWT_EmptyEmailClaim` added in final session), logout idempotency with state-corruption guard, and live-DB permission enforcement under role revocation and org-membership removal; `makeEmptyEmailToken` helper added to `token_helpers_test.go`; adversarial review: all 9 checks PASS; no defects
- [x] **Phase 13B.2B-B — Auth Integration Tests: Validation & Edge Cases** — 20 tests closing all P1 validator-path, decode-error, and account-state gaps; `assertValidationError` + `postRaw` helpers added; new `validation_test.go` (18 tests, no DB access for 16); `TestRefresh_InactiveUserBlocked` and `TestLogin_ZeroOrgs` added to existing files; `ErrPasswordTooLong` → 422 covered via multibyte-unicode boundary test; ForgotPassword always-200 boundary gated at both validator and decode paths; adversarial review: all 20 checks PASS; no defects; total auth integration tests: 92

### Previously tracked auth hardening items (resolved in Phase 13A)

- [x] **Logout vs concurrent rotation race** — Fixed by `LogoutTransaction` (FOR UPDATE lock). Adversarial review: PASS.
- [x] **User suspension vs in-flight refresh** — Fixed by user-status re-check inside `RotateRefreshToken` transaction (Step 3b). Adversarial review: PASS.

### Must-have before first production deployment

- [x] **Password reset flow** — Implemented in Phase 13A (`POST /forgot-password`, `POST /reset-password`, migration 000024)
- [x] **Refresh token cleanup job** — `DeleteExpiredRefreshTokens` called by the cleanup scheduler (`internal/cleanup/`; hourly by default)
- [x] **Email verification token cleanup job** — `DeleteExpiredEmailVerificationTokens` called by the cleanup scheduler
- [x] **CORS configuration** — `middleware.CORS(cfg.CORSAllowedOrigins)` wired in `bootstrap/router.go`; `Access-Control-Allow-Credentials` support added
- [x] **Rate limiting** — Per-IP token-bucket limiter implemented and wired onto the `/api/v1/auth` route group
- [x] **Remove `verification_token` from register response in production** — Gated behind `IsDevelopment()` in `handler.Register`

### Required for feature completeness

- [ ] **Users module** — User management: list, get, update profile, change password, deactivate.
- [ ] **News module** — Stub exists; no business logic.
- [ ] **N-way head-to-head resolution** — Phase 10 standings apply head-to-head only to strict 2-way ties. Full sub-table resolution for 3+ tied participants is deferred. When exactly N>2 participants share the same points, tiebreaking falls through to score difference (criterion 3).

### Technical debt

- [ ] **`golang-jwt/jwt/v5` declared `indirect` in `go.mod`.** Running `go mod tidy` will correct this.
- [x] **Auth integration test infrastructure** — `internal/testutil/` (testcontainers-go + golang-migrate + pgxpool); `internal/testutil/fixtures/` (typed auth fixtures); 8 concurrency tests in `internal/auth/`. Phase 13B.1.
- [x] **Auth integration test suite** — 92 tests across 12 files in `internal/auth/integration/`; full lifecycle, middleware, refresh state-machine (Cases 2 & 3), password-reset, email-verification, suspension, multi-tenant, concurrency, CORS, rate-limiting, JWT-claims, and complete validation-path coverage. Phases 13B.2A + 13B.2B-A + 13B.2B-B.
- [ ] **Integration tests for non-auth domains** — multi-tenant isolation, tournament registration rules, match lifecycle and concurrency invariants, match event sequence integrity, notification drain correctness. Deferred to future phases.
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
| Phase 12 | Notifications | **COMPLETE** |
| Auth Security Hotfix v2 | Refresh token replay redesign — structural state machine | **COMPLETE** |
| Phase 13A | Security & Production Blockers | **COMPLETE** |
| Phase 13B.1 | Test Infrastructure & Deferred Fixes | **COMPLETE** |
| Phase 13B.2A | Auth Integration Tests — core suite (66 tests) | **COMPLETE** |
| Phase 13B.2B-A | Auth Integration Tests — JWT claims & security invariants (6 tests) | **COMPLETE** |
| Phase 13B.2B-B | Auth Integration Tests — validation, malformed payloads, edge cases (20 tests) | **COMPLETE** |
| Phase 14 | Users module — list, get, update profile, change password, deactivate | NOT STARTED |

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

### Phase 12 — Notifications (Complete)

Transactional Outbox notification system implemented and production-hardened. Domain transactions (matches, tournaments, registrations) write atomically to `notification_outbox`. After commit, a synchronous `DrainOutbox` call fans out in-app notifications to all org members in a separate transaction using `FOR UPDATE SKIP LOCKED`. `UNIQUE (outbox_id, user_id, channel)` enforces drain idempotency at the DB level. Match `UpdateMatch` and `CancelMatch` upgraded to compare-and-swap guards (`AND status = $previous_status`). Preference resolution batch-loaded in O(distinct event types) queries — independent of org size. Adversarial review identified 2 defects (preference audit constraint violation; N+1 preference queries); both resolved before phase marked complete. All 13 adversarial review checks PASS.

---

### Auth Security Hotfix v2 — Refresh Token Replay Redesign (Complete)

Replaced the 30-second time-window replay detection in `RotateRefreshToken` with a deterministic structural state machine. `refresh_tokens` gains `successor_id UUID NULL` (migration 000023, no FK). Token rotation atomically sets both `revoked_at` and `successor_id` via `RevokeAndLinkSuccessor :execrows`. The three-case state matrix (`IS NULL` / `IS NOT NULL + successor` / `IS NOT NULL + no successor`) eliminates false positives from concurrent legitimate refreshes and closes the 30-second replay blind spot. New token is inserted before the old token is revoked so its ID is available for the link. `rowsAffected == 1` is asserted as a defensive invariant. `go fmt`, `go vet`, `go build` clean. Adversarial review: all 10 checks PASS. No defects introduced.

---

### Phase 13A — Security & Production Blockers (Complete)

All pre-production blockers resolved. Password reset, rate limiting, CORS, cleanup scheduler, logout race, and suspension race — all implemented and adversarially reviewed (12/12 PASS). See Phase 13A implementation notes in Section 7 for full details.

New dependencies: `golang.org/x/time v0.15.0` (rate limiting).  
New package: `internal/cleanup/` (token expiry scheduler).  
New migration: `000024_password_reset_tokens`.  
New config fields: `CORS_ALLOWED_ORIGINS`, `RATE_LIMIT_ENABLED`, `RATE_LIMIT_AUTH_RPS`, `RATE_LIMIT_AUTH_BURST`, `CLEANUP_INTERVAL_MINUTES`.

---

### Phase 13B.1 — Test Infrastructure & Deferred Fixes (Complete)

testcontainers-go integration infrastructure for the auth package; password-reset deadlock fix; forgot-password timing equalization strengthened to 4-round-trip profile; 8 auth concurrency tests. All deferred Phase 13A low-severity findings resolved. Adversarial review: all 11 checks PASS.

New dependencies: `testcontainers-go v0.42.0`, `testcontainers-go/modules/postgres v0.42.0`, `golang-migrate/migrate/v4 v4.19.1`.  
New packages: `internal/testutil/`, `internal/testutil/fixtures/`.  
New file: `db/migrations/embed.go`.

---

### Phase 13B.2A — Auth Integration Tests: Core Suite (Complete)

66-test suite in `internal/auth/integration/`. Lifecycle, middleware, refresh (Cases 2 & 3), password-reset, email-verification, suspension, multi-tenant, concurrency, CORS, and rate-limiting flows — all exercised end-to-end through the HTTP API against a real PostgreSQL 17 instance. Seven new fixture helpers added to `internal/testutil/fixtures/auth.go`. Adversarial review: all 19 checks PASS. No defects.

---

### Phase 13B.2B-A — Auth Integration Tests: JWT Claims & Security Invariants (Complete)

6 targeted tests protecting named security invariants. `jwt_test.go` (new file): `TestJWT_WrongSigningKey`, `TestJWT_EmptyUserIDClaim`, `TestJWT_EmptyEmailClaim`. Additions to existing files: `TestLogout_Idempotent` (lifecycle), `TestMultiTenant_RoleRevocationDeniesPermission` and `TestMultiTenant_OrgMembershipRemovalDeniesPermission` (multitenant). `makeEmptyEmailToken` helper added. Total auth integration test count: 72. Adversarial review: all 9 checks PASS. No defects.

---

### Phase 13B.2B-B — Auth Integration Tests: Validation & Edge Cases (Complete)

**Status: COMPLETE. Adversarial review: all 20 checks PASS. Production readiness: PASS.**

Phase 13B.2B-B closed the remaining P1 auth integration coverage gaps: validator-path errors, malformed-JSON decode errors, the ForgotPassword always-200 boundary, the `ErrPasswordTooLong` sentinel, and two account-state / org-context edge cases. Total auth integration test count: **92**.

#### New Infrastructure

| Added | Location | Purpose |
|-------|----------|---------|
| `postRaw` method | `helpers_test.go` | Sends a raw string body (no marshaling) with `Content-Type: application/json`; required for malformed-JSON and empty-body tests |
| `assertValidationError` function | `helpers_test.go` | Asserts `{"error":"validation failed","fields":{"<field>":"<msg>"}}` shape; catches same-status-code regressions that bare `assertStatus` would miss |

#### New File

`internal/auth/integration/validation_test.go` — 18 tests, no fixture users, no DB access for the 16 validator/decode tests.

#### Tests Added (20 total)

| Test | File | Status | Key invariant |
|------|------|--------|---------------|
| `TestLogin_InvalidEmailFormat` | validation | P1-MUST | 400 + `fields.email`; regression → 401 if rule removed |
| `TestLogin_PasswordTooShort` | validation | P1-MUST | 400 + `fields.password` |
| `TestLogin_NonUUIDOrgID` | validation | P1-MUST | 400 via `omitempty,uuid`; regression → 422 if removed |
| `TestLogin_MalformedJSON` | validation | P1-MUST | 400 plain body (no "fields"); not a ValidationError |
| `TestLogin_MissingPassword` | validation | P1-NTH | 400 via `required` on absent field vs short value |
| `TestRegister_InvalidEmailFormat` | validation | P1-MUST | 400 + `fields.email` |
| `TestRegister_UsernameTooShort` | validation | P1-MUST | 400 via `min=3` |
| `TestRegister_UsernameInvalidChars` | validation | P1-MUST | 400 via `alphanum_under`; regression → 500 if removed (DB CHECK fires) |
| `TestRegister_PasswordTooShort` | validation | P1-MUST | 400 via `min=8` |
| `TestRegister_FullNameEmpty` | validation | P1-NTH | 400 via `required` on `full_name` |
| `TestRegister_PasswordTooLongBytes` | validation | P1-NTH | 422 via `ErrPasswordTooLong`; 37 × 'é' = 37 runes (passes `max=72`) but 74 bytes (fails `HashPassword`) |
| `TestRefresh_EmptyRefreshToken` | validation | P1-MUST | 400 (validator); distinct from `TestRefresh_InvalidToken` which is 401 (service) |
| `TestRefresh_NonUUIDOrgID` | validation | P1-MUST | 400 via `omitempty,uuid` before refresh token is validated |
| `TestRefresh_MissingBody` | validation | P1-NTH | 400 plain body ("request body is required"); EOF decode path |
| `TestForgotPassword_InvalidEmailFormat` | validation | P1-MUST | **400, not 200** — validator boundary; always-200 is service-level only |
| `TestForgotPassword_MalformedJSON` | validation | P1-MUST | **400, not 200** — decode boundary; always-200 is service-level only |
| `TestResetPassword_PasswordTooShort` | validation | P1-MUST | 400 via `min=8`; token not consumed |
| `TestResetPassword_PasswordTooLongBytes` | validation | P1-NTH | 422 via `ErrPasswordTooLong`; `HashPassword` fails before `ResetPasswordTransaction` runs |
| `TestRefresh_InactiveUserBlocked` | refresh_test.go | P1-MUST | 403 "account inactive"; closes `ErrUserInactive` gap for refresh (only suspended was tested) |
| `TestLogin_ZeroOrgs` | multitenant_test.go | P1-MUST | 409 with code "organization_required" and empty org list |

#### Key findings from adversarial review

- **`assertValidationError` is superior to bare `assertStatus`** for catching same-status-code regressions. `TestResetPassword_PasswordTooShort` illustrates this: if `min=8` is removed, the service returns `ErrResetTokenInvalid` → 400 "invalid password reset token". Same status, different body. `assertValidationError` catches it because `body.Error != "validation failed"`.
- **ForgotPassword always-200 boundary** is explicitly gate-tested at both the validator path (`TestForgotPassword_InvalidEmailFormat`) and the JSON-decode path (`TestForgotPassword_MalformedJSON`). Both confirm the service is never called.
- **`ErrPasswordTooLong` boundary** arithmetic verified: `'é'` is U+00E9, 2 UTF-8 bytes. 37 × 2 = 74 bytes > 72. 37 runes ≤ 72. Dual-layer validator (rune-count) / `HashPassword` (byte-count) boundary confirmed end-to-end.
- **No DB access** for 16 of the 20 tests — the validator fires before any DB call. Tests are extremely fast (~10 ms each).

#### Adversarial Review Results

| Check | Result |
|-------|--------|
| `assertValidationError` helper correctness | **PASS** |
| `postRaw` helper correctness (malformed JSON, empty body) | **PASS** |
| ValidationError shape assertion end-to-end (14 tests) | **PASS** |
| Non-ValidationError decode error shape (3 tests) | **PASS** |
| ForgotPassword always-200 boundary gates (2 tests) | **PASS** |
| `ErrPasswordTooLong` via register (rune/byte boundary) | **PASS** |
| `ErrPasswordTooLong` via reset-password | **PASS** |
| `TestRefresh_EmptyRefreshToken` vs `TestRefresh_InvalidToken` path distinction | **PASS** |
| `TestRefresh_InactiveUserBlocked` — DB state, parallel safety, cleanup | **PASS** |
| `TestLogin_ZeroOrgs` — null vs empty list handling, code field | **PASS** |
| Regression gate validity for all 20 tests | **PASS** |
| No false positives | **PASS** |
| No production code modified | **PASS** |
| `go fmt`, `go vet`, `go build` clean | **PASS** |
| Full test suite (`go test ./...`) 92 tests passing | **PASS** |
| Same-status-code regression detection via body-shape assertions | **PASS** |
| No DB access for validator-only tests | **PASS** |
| ErrPasswordTooLong byte-count arithmetic verified | **PASS** |
| `assertValidationError` non-fatal (t.Errorf) — full error reporting | **PASS** |
| Double-close pattern follows established codebase convention | **PASS** |

No defects identified. No production code modified.

---

## 10. Next Recommended Phase

**Phase 14 — Users Module**

The auth integration suite is complete at **92 tests** across 12 files. All auth domain flows are covered and adversarially reviewed. The `internal/users/` stub has existed since Phase 1 with package declaration only. Phase 14 closes the largest remaining feature-completeness gap.

**Phase 14 scope:**

1. **User profile read** — `GET /api/v1/users/me` or augment `/api/v1/auth/me` with editable profile fields.
2. **User profile update** — `PATCH /api/v1/users/{id}` — update `first_name`, `last_name`, `username`; BOLA-guarded (self-only or `user.manage`).
3. **Password change** — `POST /api/v1/users/{id}/change-password` — verify current password, hash new, revoke all active refresh tokens; atomically transacted.
4. **User list** — `GET /api/v1/users` — platform admin only (`user.manage`); paginated.
5. **User deactivate** — `POST /api/v1/users/{id}/deactivate` — admin action; sets `status = inactive`; revokes all sessions; audit logged.
6. **RBAC** — self-update requires auth only; deactivate requires `user.manage`; list requires `user.manage`.

---

## Appendix: Key Files Reference

| File | Purpose |
|------|---------|
| `backend/db/migrations/` | Append-only schema history; never edited after deployment |
| `backend/db/queries/*.sql` | Hand-written SQL; source for sqlc generation |
| `backend/db/sqlc/` | Generated Go — regenerate with `sqlc generate` from `backend/` |
| `backend/internal/auth/errors.go` | Canonical error sentinels for the auth domain |
| `backend/internal/auth/tokens.go` | JWT generation, validation, token hashing |
| `backend/internal/auth/repository.go` | `RotateRefreshToken` — FOR UPDATE lock + 3-case replay state machine + `RevokeAndLinkSuccessor` + rows-affected invariant (Auth Security Hotfix v2) |
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
| `backend/db/migrations/000022_notifications.up.sql` | notification_event_type/channel ENUMs, outbox/notifications/preferences tables, idempotency constraint, indexes, notification.manage RBAC |
| `backend/db/migrations/000023_refresh_token_successor_tracking.up.sql` | Adds `successor_id UUID NULL` and `chk_refresh_tokens_successor` CHECK to `refresh_tokens`; enables structural replay detection |
| `backend/db/migrations/000024_password_reset_tokens.up.sql` | `password_reset_tokens` table; single-use, SHA-256-hashed, 1-hour expiry; user FK CASCADE; two indexes |
| `backend/db/queries/auth.sql` | Refresh token lifecycle queries; `RevokeAndLinkSuccessor :execrows` (Auth Security Hotfix v2) |
| `backend/db/queries/password_reset_tokens.sql` | 7 queries: Create, GetByHash (non-locking), GetForUpdate, LockUserPasswordResetTokens (ORDER BY id FOR UPDATE), UseOne, UseAll, DeleteExpired |
| `backend/db/migrations/embed.go` | Exports `migrations.FS embed.FS` via `//go:embed *.sql`; used by `internal/testutil` to run migrations without filesystem access |
| `backend/internal/auth/repository.go` | `RotateRefreshToken` (replay state machine + user status re-check); `LogoutTransaction` (FOR UPDATE); `ForgotPasswordTransaction`; `ResetPasswordTransaction` (deterministic lock-order deadlock fix, Phase 13B.1); `equalizeEnumerationTiming` (4-round-trip no-op, Phase 13B.1) |
| `backend/internal/auth/concurrency_test.go` | 7 barrier-synchronized concurrency tests: refresh replay (Cases 2 & 3), concurrent refresh, logout+refresh race, reset same-token, reset different-tokens (deadlock regression), reset expired+valid |
| `backend/internal/auth/repository_test.go` | `TestRevokeAndLinkSuccessorInvariant` — asserts `revoked_at`, `successor_id`, and new-token active state after rotation |
| `backend/internal/auth/testmain_test.go` | `TestMain` wiring `testutil.SetupTestDB` into the auth test package |
| `backend/internal/auth/integration/jwt_test.go` | `TestJWT_WrongSigningKey`, `TestJWT_EmptyUserIDClaim`, `TestJWT_EmptyEmailClaim` — JWT token validation security gates with real-user baselines |
| `backend/internal/auth/integration/lifecycle_test.go` | 12 tests: full auth flow smoke test, register/login/logout/me lifecycle cases, `TestLogout_Idempotent` |
| `backend/internal/auth/integration/multitenant_test.go` | 9 tests: org-context resolution (single-org, multi-org 409, platform admin, wrong org, zero orgs), `TestMultiTenant_RoleRevocationDeniesPermission`, `TestMultiTenant_OrgMembershipRemovalDeniesPermission`, `TestLogin_ZeroOrgs` |
| `backend/internal/auth/integration/token_helpers_test.go` | API flow helpers and token-construction helpers including `makeEmptyEmailToken`, `makeEmptyUserIDToken`, `makeWrongKeyToken`, `makeWrongIssuerToken`, `makeAlgorithmConfusionToken` |
| `backend/internal/auth/integration/validation_test.go` | 18 tests covering all validator-path and decode-error 400 responses; `assertValidationError` asserts `{"error":"validation failed","fields":{...}}` shape; `postRaw` for malformed-JSON and empty-body scenarios; `ErrPasswordTooLong` via multibyte-unicode boundary; ForgotPassword always-200 boundary gates |
| `backend/internal/auth/integration/helpers_test.go` | HTTP client + assertion helpers; `assertValidationError` (checks body shape + named field); `postRaw` (sends raw string body without marshaling) |
| `backend/internal/testutil/container.go` | `SetupTestDB` — starts `postgres:17-alpine`, applies migrations via golang-migrate + pgx/v5, returns `*pgxpool.Pool` and teardown; Docker-unavailable skip/fail policy |
| `backend/internal/testutil/fixtures/auth.go` | Typed auth fixtures: `CreateActiveUser`, `CreateRefreshToken`, `CreatePasswordResetToken`, `CreateExpiredPasswordResetToken`, `CleanupUser`, `HashToken` |
| `backend/internal/platform/middleware/ratelimit.go` | `IPRateLimiter` — per-IP token bucket; sync.Mutex-protected map; background cleanup goroutine; Stop() via done channel |
| `backend/internal/platform/middleware/cors.go` | `CORS()` middleware — origin reflection; `Allow-Credentials` (specific origins only); `Vary: Origin`; preflight 204 |
| `backend/internal/cleanup/scheduler.go` | `Scheduler` — background token expiry cleanup; configurable interval; graceful shutdown via done channel |
| `backend/internal/bootstrap/app.go` | `App.Handler()` initialises rate limiter + cleanup scheduler; `App.Shutdown()` stops both |
| `backend/db/queries/notifications.sql` | 16 notification SQL queries; includes `GetNotificationPreferencesForEvent` for O(event_types) batch preference loading |
| `backend/internal/notifications/repository.go` | `DrainOutbox` (FOR UPDATE SKIP LOCKED, batch prefs, UNIQUE conflict handling); `UpsertPreference` (dynamic audit action: AuditActionCreate vs AuditActionUpdate) |
| `backend/internal/notifications/service.go` | `DrainOutbox` entry point — errors swallowed to preserve domain operation result |
| `backend/internal/notifications/trigger/trigger.go` | `WriteOutboxEntry` — must be called with a transaction-scoped `*db.Queries` handle |

---

*This document was last updated on 2026-06-04 (Phase 13B.2B-B complete). It should be updated whenever a phase is completed or significant architectural changes are made.*
