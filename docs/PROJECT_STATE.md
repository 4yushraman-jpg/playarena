# PlayArena — Project State & Handoff Document

**Last Updated:** 2026-06-10  
**Build status:** `go build ./...` passing, `go vet ./...` clean, `sqlc generate` clean  
**Migrations applied:** 000001 – 000027  
**Go version:** 1.25.6  
**Database:** PostgreSQL 17  
**Phases complete:** 1 – 12, Auth Security Hotfix v2, Phase 13A, Phase 13B.1, Phase 13B.2A, Phase 13B.2B-A, Phase 13B.2B-B, Phase 14, Phase 15A, Phase 15A Remediation, Phase 16, Phase 17, Phase 18, Phase 19, Phase 19 Remediation, Phase 20, Phase 20 Remediation, Phase 21, Phase 22, Phase 23A, Phase 23B, Phase 23C, Phase 23D

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
│   ├── migrations/                         golang-migrate files (000001–000027, up + down); embed.go exports FS for test runners
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
│   │   ├── handler.go                     HTTP handlers + error-to-status mapping + logging; sync.WaitGroup tracks in-flight email goroutines; DrainEmail(ctx) for graceful shutdown
│   │   ├── routes.go                      RegisterRoutes() — mounts /api/v1/auth subtree; returns *Handler for DrainEmail wiring
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
│   ├── notifications/                      Notifications domain (fully implemented, Phase 12 + Phase 18 + Phase 20)
│   │   ├── errors.go                      Typed domain error sentinels
│   │   ├── model.go                       ListParams, prefKey, pagination constants
│   │   ├── dto.go                         Response, ListResponse, PreferenceResponse, UpdatePreferenceRequest
│   │   ├── repository.go                  UpsertPreference (dynamic audit action); DrainOutbox (multi-channel fan-out: in_app + email + webhook; FOR UPDATE SKIP LOCKED)
│   │   ├── service.go                     List, GetByID, MarkRead, MarkAllRead, Delete, GetPreferences, UpdatePreference, DrainOutbox; nil-safe Hub.Publish post-commit
│   │   ├── handler.go                     HTTP handlers + error mapping; ListNotifications filters channel='in_app'; Stream (SSE — JWT via ?token= or Bearer; org-scoped; 25s keepalive; X-Accel-Buffering: no)
│   │   ├── routes.go                      RegisterRoutes() — mounts /api/v1/organizations/{slug}/notifications; /stream registered outside RequireAuth group (EventSource cannot set headers)
│   │   └── trigger/
│   │       └── trigger.go                 WriteOutboxEntry — writes notification_outbox rows inside domain transactions
│   ├── email/                              Email delivery (fully implemented, Phase 14 + Phase 18)
│   │   ├── email.go                       Provider interface; NoOpProvider (test capture: Sent, SentTo, Count, Reset); NewSender factory; NewSenderWithProvider (test injection)
│   │   ├── sender.go                      Sender struct; SenderConfig; SendVerificationEmail, SendPasswordResetEmail, SendResendVerificationEmail, SendNotificationEmail
│   │   ├── ses.go                         sesProvider — AWS SES v2 backend (awsconfig + sdk/sesv2)
│   │   ├── smtp.go                        smtpProvider — net/smtp backend; STARTTLS + implicit TLS modes; context-aware via goroutine+channel; multipart/alternative MIME when both bodies present
│   │   ├── templates.go                   //go:embed templates; html/template + text/template loading; renderNotificationEvent
│   │   ├── email_test.go                  9 unit tests for NoOpProvider and Sender
│   │   └── templates/
│   │       ├── verify_email.{html,txt}        Email verification template pair
│   │       ├── password_reset.{html,txt}      Password reset template pair
│   │       └── notification_event.{html,txt}  Notification event email template pair (Phase 18)
│   ├── notifworker/                        Email notification delivery worker (Phase 18)
│   │   ├── repository.go                  ClaimBatch, RecordSuccess, RecordFailure, GetUserByID, GetOrgSlugByID (wraps sqlc Queries)
│   │   ├── worker.go                      EmailWorker — Start/Stop/Drain lifecycle; runOnce tick loop; deliver (send + record); retryDelay (1m→5m); permanent failure dead-letter; structured slog
│   │   └── integration/
│   │       ├── testmain_test.go           Standard testcontainers TestMain
│   │       └── worker_test.go             5 integration tests: happy path, retry on failure, permanent failure, duplicate idempotency, outbox drain cascade
│   ├── webhooks/                           Webhook endpoint management (Phase 19)
│   │   ├── errors.go                      Typed domain error sentinels (ErrSSRFBlocked, ErrInvalidURL, ErrWebhookNotFound, …)
│   │   ├── dto.go                         CreateRequest, UpdateActiveRequest, Response, CreateResponse (RawSecret shown once), ListResponse
│   │   ├── ssrf.go                        ValidateURL (registration-time: HTTPS-only, blocked CIDRs, localhost check); SSRFSafeTransport (delivery-time: DNS resolution + all-IPs-public check + dial-by-resolved-IP)
│   │   ├── crypto.go                      GenerateSecret (32-byte CSPRNG, base64url); EncryptSecret / DecryptSecret (AES-256-GCM, random nonce)
│   │   ├── repository.go                  GetOrgBySlug, Create, GetByID, List, UpdateActive, Delete — all scoped by organization_id
│   │   ├── service.go                     NewService (decodes 32-byte AES key); Create (ValidateURL → GenerateSecret → EncryptSecret → repo); GetByID, List, UpdateActive, Delete
│   │   ├── handler.go                     5 HTTP handlers; resolveOrgID; writeError maps domain errors to HTTP codes
│   │   ├── routes.go                      RegisterRoutes() — mounts /api/v1/organizations/{slug}/webhooks; RequireAuth + RequirePermission(webhook.*)
│   │   └── integration/
│   │       ├── testmain_test.go           Standard testcontainers TestMain
│   │       └── webhook_test.go            12 integration tests: CRUD, BOLA, SSRF URL blocking, secret not exposed, crypto round-trip
│   ├── webhookworker/                      Webhook delivery worker (Phase 19)
│   │   ├── repository.go                  ClaimBatch, GetEndpoint, RecordSuccess, RecordFailure (wraps sqlc Queries)
│   │   ├── worker.go                      WebhookWorker — Start/Stop/Drain lifecycle; runOnce tick loop; deliver (decrypt secret → build envelope → HMAC-SHA256 → POST); retryDelay (1m→5m→15m); permanent failure dead-letter
│   │   └── integration/
│   │       ├── testmain_test.go           Standard testcontainers TestMain
│   │       └── worker_test.go             10 integration tests: delivery, idempotency, signature verification, retry on 5xx, permanent failure on 4xx, 429 retry, dead-letter after 3 attempts, concurrent workers, start/stop, tenant isolation
│   ├── realtime/                           In-process pub/sub Hub for SSE (Phase 20)
│   │   └── hub.go                         Hub — sync.RWMutex-protected map[subKey]map[chan []byte]struct{}; Subscribe/Unsubscribe/Publish/Shutdown/Done; buffer 32; non-blocking publish (drop on full; RLock held through fan-out to prevent data race + send-on-closed panic); WithMetrics for Prometheus instrumentation
│   ├── rankings/                           Rankings domain (fully implemented, Phase 22)
│   │   ├── errors.go                      ErrOrganizationNotFound sentinel
│   │   ├── dto.go                         ListParams, PlayerRankingEntry, TeamRankingEntry, PlayerRankingsResponse, TeamRankingsResponse
│   │   ├── model.go                       StatsRow (tournament-agnostic stats carrier for snapshot)
│   │   ├── models.go                      sqlc-generated type aliases for query results
│   │   ├── repository.go                  ListPlayerRankings, ListTeamRankings (SQL RANK() window function + GROUP BY), SnapshotPlayerStats, SnapshotTeamStats (ON CONFLICT DO UPDATE upsert)
│   │   ├── service.go                     ListPlayerRankings, ListTeamRankings — validates org, computes win_rate in Go, returns ranked list
│   │   ├── handler.go                     HTTP handlers for GET /players and GET /teams; parseListParams
│   │   └── routes.go                      RegisterRoutes() — GET /api/v1/organizations/{slug}/rankings/players|teams; RequireAuth
│   ├── users/                             Users domain (fully implemented, Phase 16)
│   │   ├── errors.go
│   │   ├── dto.go
│   │   ├── model.go
│   │   ├── repository.go
│   │   ├── service.go
│   │   ├── handler.go
│   │   └── routes.go
│   ├── news/                              News domain (stub — package declaration only, no business logic)
│   ├── testutil/                           Shared integration-test infrastructure (Phase 13B.1)
│   │   ├── container.go                   SetupTestDB — postgres:17-alpine container, migrations, pool; Docker-skip logic
│   │   └── fixtures/
│   │       └── auth.go                    CreateActiveUser, CreatePendingUser, CreateSuspendedUser, CreateInactiveUser,
│   │                                      CreatePlatformAdmin, CreateOrgWithRole, CreateRefreshToken,
│   │                                      CreateExpiredRefreshToken, CreatePasswordResetToken,
│   │                                      CreateExpiredPasswordResetToken, CreateExpiredEmailVerificationToken,
│   │                                      CleanupUser, HashToken
│   ├── bootstrap/
│   │   ├── app.go                         App struct (reg, internalServer, scraperDone, scheduler, 3 limiters, authHandler, notifEmailWorker, notifWebhookWorker, realtimeHub fields); Handler() starts all workers + scrapers + internal server; Shutdown() drains all workers then stops all
│   │   ├── router.go                      Builds chi router + global middleware stack (RequestID, TrustedRealIP, Recoverer, RequestLogger, CORS, Metrics); returns (http.Handler, *auth.Handler, *notifworker.EmailWorker, *webhookworker.WebhookWorker, *realtime.Hub, *notifications.Repository, *webhookworker.Repository)
│   │   ├── modules.go                     Wires all domain modules; constructs EmailWorker + WebhookWorker + Hub + rankingsRepo; BodySizeLimit + writeLimiter on domain write group; returns all workers + repos for metrics scrapers
│   │   └── observability.go               newInternalServer (:9090, /metrics + /ready + /live + optional /debug/pprof/*); startDBPoolScraper (15s interval, pgxpool.Stat()); startOutboxMetricsScraper (30s interval, pending outbox rows + dead letters)
│   └── platform/
│       ├── config/config.go               ENV-based config with validation; includes AppInternalPort, PprofEnabled, DrainTimeoutSeconds, AuditLogRetentionDays
│       ├── database/postgres.go           pgxpool factory with production defaults
│       ├── logger/logger.go               slog (JSON in prod, text in dev)
│       ├── metrics/metrics.go             Prometheus Registry — playarena_* prefix; HTTP counters/histogram/in-flight; rate-limit rejections; DB pool gauges; auth counters; notification drain metrics; email/webhook worker metrics; realtime hub metrics; WithMetrics() pattern used by Hub, limiters, workers, notifSvc
│       ├── metrics/metrics_test.go        Unit tests for metrics registry construction
│       ├── middleware/bodysize.go         BodySizeLimit(maxBytes) — caps body reads; maps *http.MaxBytesError → ErrBodyTooLarge → 413
│       ├── middleware/cors.go             CORS() middleware — origin reflection; Allow-Credentials; Vary: Origin; preflight 204
│       ├── middleware/logging.go          RequestLogger — per-request structured slog (method, path, status, latency, request_id)
│       ├── middleware/metrics.go          Metrics(reg) — Prometheus HTTP middleware; instruments HTTPRequests counter, HTTPDuration histogram, HTTPInFlight gauge
│       ├── middleware/ratelimit.go        IPRateLimiter — per-IP token bucket; sync.Map + atomic.Int64 lastSeen; Middleware() (all methods); WriteMiddleware() (writes only); Retry-After: 1 on 429; WithMetrics() injects rate-limit rejection counter
│       ├── middleware/realip.go           TrustedRealIP(trustedCIDRs) — rewrites RemoteAddr from X-Forwarded-For only for connections from trusted CIDRs; falls back to chi.RealIP when unconfigured
│       ├── pgutil/pgutil.go               Shared PostgreSQL helpers (UUID parse/format, unique violation check)
│       ├── response/response.go           JSON write helpers
│       └── validator/validator.go         DecodeJSON — body decode + struct-tag validation; ErrBodyTooLarge sentinel
```

**Remaining stubs** (files exist with package declaration only, no logic):
`news/`

### Request Lifecycle

```
HTTP request
  → chi.RequestID       (X-Request-ID header)
  → TrustedRealIP       (rewrites RemoteAddr from X-Forwarded-For only for trusted proxy CIDRs; falls back to chi.RealIP when TRUSTED_PROXY_CIDRS unset)
  → chi.Recoverer       (panic → 500)
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
| Rate limiting | golang.org/x/time | v0.15.0 |
| AWS SDK (SES v2) | aws-sdk-go-v2 | latest |
| Metrics | prometheus/client_golang | v1.22.0 |

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
| 000025 | Notification Email Delivery | Adds 4 delivery-state columns to `notifications`: `attempt_count INT NOT NULL DEFAULT 0`, `last_attempted_at TIMESTAMPTZ`, `lease_expires_at TIMESTAMPTZ`, `failed_permanently BOOL NOT NULL DEFAULT FALSE`; `idx_notifications_email_pending` partial index on `(last_attempted_at NULLS FIRST, created_at) WHERE channel = 'email' AND sent_at IS NULL AND failed_permanently = FALSE` |
| 000026 | Webhook Notifications | `webhook_endpoints` (id, organization_id, url, secret_ciphertext BYTEA, description, active, created_by, created_at, updated_at); `webhook_deliveries` (id, organization_id, endpoint_id, outbox_id, event_type, entity_type, entity_id, payload JSONB, attempt_count, last_attempted_at, lease_expires_at, sent_at, failed_permanently, created_at); `UNIQUE(outbox_id, endpoint_id)`; partial indexes for pending delivery and active endpoints; seeds 4 webhook RBAC permissions (webhook.create/read/update/delete) for platform_admin, org_owner, org_admin |
| 000027 | Rankings Stats | `player_tournament_stats` + `team_tournament_stats` — per-tournament participation rows upserted at completion; `UNIQUE(player_id, tournament_id)` / `UNIQUE(team_id, tournament_id)`; columns: position, matches_played, matches_won, matches_drawn, matches_lost, points, score_for, score_against, snapshotted_at; covering indexes `(organization_id, player_id)` / `(organization_id, team_id)` for ranking query performance; no CASCADE from tournaments (orphaned stats are harmless; FK prevents accidental loss) |

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

#### Permission Matrix (complete, as of migration 000026)

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
| `webhook.create` | ✓ | ✓ | ✓ | — | — | — | — |
| `webhook.read` | ✓ | ✓ | ✓ | — | — | — | — |
| `webhook.update` | ✓ | ✓ | ✓ | — | — | — | — |
| `webhook.delete` | ✓ | ✓ | ✓ | — | — | — | — |

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

**`notifications`** — Personal notification inbox. Written **only** by `DrainOutbox` after domain transactions commit. Every row is scoped to one user within one organization. `UNIQUE (outbox_id, user_id, channel)` — drain retry idempotency safeguard. Soft-deleted via `deleted_at`; all queries exclude `deleted_at IS NOT NULL`. `sent_at` contract: `in_app` → set to `NOW()` on drain insert; `email` → NULL until `EmailWorker` confirms delivery. Email delivery-state columns (migration 000025): `attempt_count INT NOT NULL DEFAULT 0`, `last_attempted_at TIMESTAMPTZ`, `lease_expires_at TIMESTAMPTZ` (soft-lease for crash recovery), `failed_permanently BOOL NOT NULL DEFAULT FALSE` (dead-letter after 3 attempts). Inbox queries (`ListNotificationsByUser`, `CountNotificationsByUser`) filter `channel = 'in_app'` — email rows are delivery tracking only and not surfaced in the in-app inbox. `outbox_id` FK has `ON DELETE CASCADE` — outbox retention purge auto-removes matching notification rows.

**`notification_preferences`** — Per-user, per-org, per-event-type, per-channel opt-in/out. Missing row = enabled (opt-out model). UPSERT semantics (last-writer-wins) on `(organization_id, user_id, event_type, channel)`.

#### Webhook Tables (added migration 000026)

**`webhook_endpoints`** — Registered webhook receiver URLs per organization. `secret_ciphertext BYTEA` stores the per-endpoint HMAC signing key encrypted with AES-256-GCM (12-byte nonce prepended: `nonce || ciphertext`). The raw secret is shown once at creation and never again. `active BOOLEAN` controls whether the endpoint receives new deliveries (existing pending `webhook_deliveries` rows are unaffected by deactivation). FK to `organizations` ON DELETE CASCADE; FK to `users` (created_by). Partial index on `(organization_id) WHERE active = TRUE` for efficient fan-out queries.

**`webhook_deliveries`** — One row per `(outbox_id, endpoint_id)` pair. Written atomically inside `DrainOutbox` as part of the outbox transaction. Separate from `notifications` because webhook deliveries are endpoint-centric (not user-centric) and would conflict with the `UNIQUE(outbox_id, user_id, channel)` constraint. `UNIQUE(outbox_id, endpoint_id)` — drains are idempotent via `ON CONFLICT DO NOTHING`. Delivery-state columns (`attempt_count`, `last_attempted_at`, `lease_expires_at`, `sent_at`, `failed_permanently`) mirror the email worker model. Partial index on `(last_attempted_at NULLS FIRST, created_at) WHERE sent_at IS NULL AND failed_permanently = FALSE` for worker claim query. FK to `webhook_endpoints` ON DELETE CASCADE — deleting an endpoint cascades to all pending/completed deliveries.

#### Rankings Tables (added migration 000027)

**`player_tournament_stats`** — Final standings row for one player in one completed tournament. Upserted (ON CONFLICT DO UPDATE) when a tournament transitions to `completed`, so crash-and-retry is idempotent. `UNIQUE(player_id, tournament_id)`. Columns: `position INT` (1 = winner), `matches_played`, `matches_won`, `matches_drawn`, `matches_lost`, `points`, `score_for`, `score_against`, `snapshotted_at`. Rankings list queries aggregate across tournaments via `GROUP BY player_id` + `RANK()` window function at read time. Covering index on `(organization_id, player_id)`.

**`team_tournament_stats`** — Mirrors `player_tournament_stats` for team-based tournaments. `UNIQUE(team_id, tournament_id)`. Covering index on `(organization_id, team_id)`.

### Key Design Decisions

**Event sourcing for live scoring.** `match_events` is the single source of truth for all scoring during active matches. The live scoring engine (`internal/scoring/`) derives home and away scores on every `GET /matches/{id}/score` request from the effective event log — no score is ever stored for live matches.

**Score snapshot at completion.** When a match transitions `live → completed`, `matches.home_score` and `matches.away_score` are written atomically inside the same transaction, under a `FOR UPDATE` lock on the match row. The lock prevents any concurrent event insertion between the score computation and the status write, making the snapshot permanently consistent with the event log. After completion, no further events can be recorded (`ErrMatchNotLive`), so the snapshot never drifts. Corrections are non-destructive: a `score_correction` event references (via `cancels_event_id`) the event it supersedes; neither row is mutated.

**Denormalization with trigger guards.** `matches.organization_id` and `match_events.organization_id` are denormalized for query performance. Database triggers (`trg_matches_org_consistency`, `trg_match_events_org_consistency`) enforce consistency on INSERT/UPDATE.

**Cross-org tournament registrations.** `tournament_registrations.organization_id` is the registrant's org, not the tournament host org. A federation tournament can accept teams from multiple clubs. The registrant's team/player must still belong to the registrant's org (validated by trigger).

**Soft foreign keys in media.** `media_attachments` uses a polymorphic `(entity_type, entity_id)` reference. No DB-level FK is possible. The application service layer is responsible for orphan cleanup when parent entities are deleted.

**Transactional outbox for notifications.** Domain writes (matches, tournaments, registrations) write a row to `notification_outbox` inside the same transaction as the domain mutation. After the transaction commits, the domain service calls `DrainOutbox` synchronously. The drain opens a fresh transaction, claims pending rows with `FOR UPDATE SKIP LOCKED`, fans out in-app notifications to all org members (filtered by preferences loaded in one batch query per event type), and marks entries processed. This decouples notification delivery from domain transactions: a drain failure never rolls back a committed domain operation, and the outbox entries remain durable for the next drain cycle. Drain idempotency is enforced at the database level by `UNIQUE (outbox_id, user_id, channel)` on `notifications` — a retry can never produce duplicate rows.

**Snapshot-on-completion for rankings.** When a tournament transitions to `completed`, `tournaments.Service.Update` calls `snapshotTournamentStats` after the DB write succeeds. This derives the final standings (using the existing `standings.Compute` engine) and upserts rows into `player_tournament_stats` / `team_tournament_stats`. Rankings are then derived at read time by aggregating across tournaments with `GROUP BY + RANK()` in SQL. This approach is correct by construction (one upsert per tournament per participant), read-efficient (no event log re-aggregation at query time), and idempotent (a retry after a crash re-upserts the same values). `win_rate` is intentionally kept out of the SQL `ORDER BY` and computed in the Go service layer to avoid float precision issues in PostgreSQL sort keys. The `rankingsRepo` field on `tournaments.Service` is nil-safe — integration tests that don't need rankings pass `nil` and skip the snapshot.

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
  └── webhook_endpoints              (registered receiver URLs; secret encrypted AES-256-GCM)
      └── webhook_deliveries         (one row per outbox_id × endpoint_id; endpoint-centric fan-out)
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
| **Email** | `email.go`, `sender.go`, `ses.go`, `smtp.go`, `templates.go`; `templates/verify_email.{html,txt}`, `templates/password_reset.{html,txt}`, `templates/notification_event.{html,txt}` | Complete |
| **Notification Worker** | `repository.go`, `worker.go`; `integration/testmain_test.go`, `integration/worker_test.go` | Complete |
| **Webhook Endpoints** | `errors.go`, `dto.go`, `ssrf.go`, `crypto.go`, `repository.go`, `service.go`, `handler.go`, `routes.go`; `integration/testmain_test.go`, `integration/webhook_test.go` | Complete |
| **Webhook Worker** | `repository.go`, `worker.go`; `integration/testmain_test.go`, `integration/worker_test.go` | Complete |
| **Realtime Hub** | `hub.go` — in-process pub/sub; Subscribe/Unsubscribe/Publish/Shutdown/Done; RLock through entire fan-out (data-race fix) | Complete |
| **Platform / Config** | `config.go` | Complete |
| **Platform / Database** | `postgres.go` | Complete |
| **Platform / Logger** | `logger.go` | Complete |
| **Platform / Middleware** | `bodysize.go`, `cors.go`, `logging.go`, `ratelimit.go` | Complete |
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
| `POST` | `/api/v1/auth/resend-verification` | No | Re-issue email verification token for pending_verification accounts; always returns 200 (enumeration-resistant); sends email async |
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
| `GET` | `/api/v1/organizations/{slug}/notifications/stream` | JWT only (`?token=` query param or `Authorization: Bearer`) | Server-Sent Events stream; authenticated; org-scoped; broadcasts new in_app notifications in real time; 25s keepalive frames; graceful shutdown via Hub.Done() |

Valid `event_type` values: `match_created`, `match_started`, `match_completed`, `match_cancelled`, `match_abandoned`, `tournament_status_changed`, `registration_approved`, `registration_rejected`, `registration_withdrawn`.  
Valid `channel` values: `in_app`, `email`, `webhook`.

**SSE stream notes:** `EventSource` (browser native SSE API) cannot set `Authorization` headers; authentication is via `?token=<jwt>` query parameter with `Authorization: Bearer` fallback for curl/testing. JWT must carry an `organization_id` matching the URL org — platform admin tokens (empty `organization_id`) are rejected with 403. Each SSE frame format: `event: notification\ndata: <JSON>\n\n`. Keepalive frames: `:\n\n` every 25 seconds. `X-Accel-Buffering: no` prevents nginx proxy buffering.

### Webhooks (`/api/v1/organizations/{slug}/webhooks`)

All webhook endpoints require `RequireAuth` and `RequirePermission(webhook.*)`. All queries are scoped by `organization_id` (resolved from URL slug). The raw secret is returned only on `POST` creation and never again.

| Method | Path | Auth | Permission | Description |
|--------|------|:----:|-----------|-------------|
| `POST` | `/api/v1/organizations/{slug}/webhooks` | Yes | `webhook.create` | Register a new webhook endpoint; validates HTTPS-only URL, blocks private IPs at registration; generates 32-byte CSPRNG secret (AES-256-GCM encrypted at rest); returns `raw_secret` once only |
| `GET` | `/api/v1/organizations/{slug}/webhooks` | Yes | `webhook.read` | List all webhook endpoints for the org (no secret or ciphertext in response) |
| `GET` | `/api/v1/organizations/{slug}/webhooks/{webhookID}` | Yes | `webhook.read` | Get single endpoint by UUID; BOLA-guarded by org scope |
| `PATCH` | `/api/v1/organizations/{slug}/webhooks/{webhookID}/active` | Yes | `webhook.update` | Toggle `active` flag; deactivation stops fan-out of new deliveries but does not cancel pending ones |
| `DELETE` | `/api/v1/organizations/{slug}/webhooks/{webhookID}` | Yes | `webhook.delete` | Hard delete endpoint; cascades to all `webhook_deliveries` rows for this endpoint |

**Delivery protocol:** `POST` to the registered URL, `Content-Type: application/json`. Signed with HMAC-SHA256 using the raw secret as key. Canonical string: `<unix_timestamp>\n<delivery_uuid>\n<body_bytes>`. Headers: `X-PlayArena-Signature` (hex), `X-PlayArena-Timestamp` (Unix seconds), `X-PlayArena-Event-ID` (delivery UUID). Both `X-PlayArena-Timestamp` and `payload.timestamp` (RFC3339) represent the same instant.

### Rankings (`/api/v1/organizations/{slug}/rankings`)

All rankings endpoints require `RequireAuth`. Org scope resolved from URL slug. Rankings aggregate across all completed tournaments hosted by the org. `win_rate` is computed in the Go service layer (not SQL).

| Method | Path | Auth | Description |
|--------|------|:----:|-------------|
| `GET` | `/api/v1/organizations/{slug}/rankings/players` | Yes | Paginated player leaderboard for the org; ordered by `tournaments_won DESC → podium_finishes DESC → total_points DESC → total_wins DESC → total_matches DESC`; `RANK()` window function assigns ranks (ties share a rank); returns `organization_id`, `rankings[]`, `total`, `limit`, `offset` |
| `GET` | `/api/v1/organizations/{slug}/rankings/teams` | Yes | Same as above for teams |

### Health

| Method | Path | Auth | Description |
|--------|------|:----:|-------------|
| `GET` | `/api/v1/health` | No | DB connectivity check; returns `{"status":"ok","database":"connected"}` |

### Observability (internal port `:9090`, never public)

| Method | Path | Auth | Description |
|--------|------|:----:|-------------|
| `GET` | `/metrics` | None (internal only) | Prometheus metrics exposition; `playarena_*` prefix |
| `GET` | `/ready` | None (internal only) | Kubernetes/Docker readiness probe — 200 if DB reachable, 503 otherwise |
| `GET` | `/live` | None (internal only) | Kubernetes/Docker liveness probe — always 200 if process responds |
| `GET` | `/debug/pprof/*` | None (internal only, `PPROF_ENABLED=true` required) | Go runtime profiling endpoints |

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

`internal/cleanup/scheduler.go` — background token expiry + outbox retention scheduler.

- `Scheduler` struct: `*db.Queries`, configurable `interval`, slog logger, done channel
- `Start()` launches a single goroutine; `Stop()` signals exit (safe to call multiple times)
- `runOnce()` runs under a 30-second context timeout; calls `DeleteExpiredRefreshTokens`, `DeleteExpiredEmailVerificationTokens`, `DeleteExpiredPasswordResetTokens`, `DeleteOldProcessedOutboxEntries` independently — a failure in one does not prevent the others
- `outboxRetentionDays = 90`; ON DELETE CASCADE on `notifications.outbox_id` auto-removes matching notification rows when outbox rows are purged
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

### Phase 14 — Email Infrastructure (Complete)

**Status: COMPLETE. Adversarial review: all P1 defects identified and fixed in Phase 15A. Production readiness: PASS (after Phase 15A).**

Phase 14 delivered the full email delivery system for the auth domain: provider abstraction, AWS SES v2 and SMTP implementations, HTML + text templates, transactional outbox-backed resend-verification endpoint, and integration/unit test coverage.

#### New Package: `internal/email/`

| File | Purpose |
|------|---------|
| `email.go` | `Provider` interface (`Send(ctx, Message) error`); `NoOpProvider` (in-memory capture for tests); `NewSender` factory (constructs provider from config); `NewSenderWithProvider` (test injection) |
| `sender.go` | `Sender` struct + `SenderConfig`; `SendVerificationEmail`, `SendPasswordResetEmail`, `SendResendVerificationEmail` — renders templates and delegates to `Provider.Send` |
| `ses.go` | `sesProvider` — AWS SES v2 backend; `awsconfig.LoadDefaultConfig` with optional static credentials override; optional HTML body |
| `smtp.go` | `smtpProvider` — `net/smtp` backend; `useTLS=false` → STARTTLS (`smtp.SendMail`); `useTLS=true` → implicit TLS (`tls.Dial` + port 465) |
| `templates.go` | `//go:embed templates` FS; `html/template` for HTML bodies (auto-escaping); `text/template` for text bodies |
| `email_test.go` | 9 unit tests: `NoOpProvider` record/filter/reset; `Sender` send/content/nil-safety |

#### Email Provider Abstraction

The `Provider` interface decouples delivery from the auth domain. The production binary picks the provider at startup based on `EMAIL_PROVIDER`:

| `EMAIL_PROVIDER` | Provider | Use |
|-----------------|----------|-----|
| `ses` | `sesProvider` (AWS SES v2) | Production — AWS-hosted deployments |
| `smtp` | `smtpProvider` (net/smtp) | Production — self-hosted / third-party SMTP |
| `log` | `LogProvider` (slog) | Development — logs email to stdout, default |
| `noop` | `NoOpProvider` | Tests — captures messages in memory |

Production mode (`APP_ENV=production`) blocks `noop` and `log` providers via config validation.

#### New Auth Endpoint: POST /api/v1/auth/resend-verification

Resends the email verification token for accounts still in `pending_verification` state.

- Always returns HTTP 200 regardless of whether the account exists (enumeration-resistant; body identical on all paths)
- Service-layer outcomes:
  - Email not found → `equalizeEnumerationTiming` → return `("", nil)`
  - Account not `pending_verification` → `equalizeEnumerationTiming` → return `("", nil)`
  - DB error → `equalizeEnumerationTiming` → return `("", nil)`
  - Success → new token created → `equalizeEnumerationTiming` → return `(rawToken, nil)`
- Timing equalization: success path now also calls `equalizeEnumerationTiming` so all paths have comparable round-trip profiles
- Email sent asynchronously in a goroutine with a 30-second context timeout (Phase 15A fix)

#### Bootstrap Integration

`bootstrap/modules.go` constructs `emailSender` once at startup:

```go
emailSender, err := email.NewSender(cfg, log)
// panics if EMAIL_PROVIDER is misconfigured — mirrors media storage panic pattern
auth.RegisterRoutes(r, pool, cfg, log, authLimiter, emailSender)
```

`auth.RegisterRoutes` accepts `*email.Sender` (nil-safe — nil sender skips email sends without panicking).

#### New Config Fields (Phase 14)

| Variable | Default | Required | Purpose |
|----------|---------|----------|---------|
| `EMAIL_PROVIDER` | `log` | No | Selects provider: `ses`, `smtp`, `log`, `noop` |
| `EMAIL_FROM_ADDRESS` | — | **Yes** | Sender address (e.g. `noreply@playarena.com`) |
| `EMAIL_FROM_NAME` | `PlayArena` | No | Sender display name |
| `APP_BASE_URL` | — | **Yes** | Frontend base URL for link construction (must be `https://` in production) |
| `EMAIL_SES_REGION` | `us-east-1` | No | AWS region (provider=ses) |
| `EMAIL_SES_ACCESS_KEY` | — | No | AWS access key ID; empty → IAM role chain (provider=ses) |
| `EMAIL_SES_SECRET_KEY` | — | No | AWS secret key; empty → IAM role chain (provider=ses) |
| `EMAIL_SMTP_HOST` | `localhost` | No | SMTP hostname (provider=smtp) |
| `EMAIL_SMTP_PORT` | `1025` | No | SMTP port (provider=smtp) |
| `EMAIL_SMTP_USERNAME` | — | No | SMTP auth username (provider=smtp) |
| `EMAIL_SMTP_PASSWORD` | — | No | SMTP auth password (provider=smtp) |
| `EMAIL_SMTP_TLS` | `false` | No | `true` → implicit TLS / port 465 (provider=smtp) |

#### New Config Fields (Phase 15A — write + media rate limiters)

| Variable | Default | Purpose |
|----------|---------|---------|
| `RATE_LIMIT_WRITE_RPS` | `30.0` | Sustained req/s per IP for domain write endpoints (POST/PUT/PATCH/DELETE) |
| `RATE_LIMIT_WRITE_BURST` | `60` | Burst size for domain write endpoints |
| `RATE_LIMIT_MEDIA_RPS` | `5.0` | Sustained req/s per IP for media upload endpoints |
| `RATE_LIMIT_MEDIA_BURST` | `10` | Burst size for media upload endpoints |

#### Integration Tests (Phase 14)

`internal/auth/integration/email_delivery_test.go` — 8 integration tests:

- `TestRegister_EmailDelivered` — verifies async email sent after registration
- `TestRegister_BodySizeLimit_*` — body > 64 KB returns 413 (not 400)
- `TestResendVerification_EmailDelivered` — verifies resend sends email for pending account
- Additional enumeration-resistance and async-delivery assertions

New dependencies added: `aws-sdk-go-v2/aws`, `aws-sdk-go-v2/config`, `aws-sdk-go-v2/credentials`, `aws-sdk-go-v2/service/sesv2`.

---

### Phase 15A — P1 Defect Fixes from Phase 14 Adversarial Review (Complete)

**Status: COMPLETE. All 5 P1 defects resolved. Production readiness: PASS.**

Phase 15A fixed every P1 (production blocker) defect surfaced by the Phase 14 adversarial security review. No new features were added; changes were surgical and minimal.

#### Fix 1 — Async Register Email with Goroutine Timeout

**Problem:** `handler.Register` called `h.emailSender.SendVerificationEmail` synchronously, blocking the HTTP response for the full SES/SMTP round-trip (~100–500 ms). Under load this starves the server goroutine pool and exposes users to correlated latency spikes with the email provider.

**Fix:** Email send moved to a goroutine. `rawToken` captured before the production strip. 30-second `context.WithTimeout` guards the goroutine. Errors logged with `slog.ErrorContext`; never returned to the caller.

```go
rawToken := resp.VerificationToken
go func() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    if err := h.emailSender.SendVerificationEmail(ctx, toEmail, toName, rawToken); err != nil {
        h.log.ErrorContext(context.Background(), "auth.register.email_failed", slog.String("error", err.Error()))
    }
}()
```

#### Fix 2 — ResendVerification Timing Equalization on Success Path

**Problem:** `service.ResendVerification` called `equalizeEnumerationTiming` on all failure paths (not-found, wrong-status, DB error) but not on the success path. The success path (SELECT + INSERT ≈ 2 DB ops) was measurably faster than the not-found path (SELECT + equalization ≈ 5 DB ops), allowing a timing adversary to enumerate `pending_verification` accounts.

**Fix:** `equalizeEnumerationTiming` added to the success path after `CreateEmailVerificationToken` succeeds. All paths now complete ~5–6 equivalent DB round-trips.

#### Fix 3 — Goroutine Timeouts on ForgotPassword + ResendVerification

**Problem:** The goroutines in `handler.ForgotPassword` and `handler.ResendVerification` launched goroutines without context timeouts. A hung email provider would cause goroutines to leak indefinitely.

**Fix:** `context.WithTimeout(context.Background(), 30*time.Second)` added to both goroutines, matching the pattern established in Fix 1.

#### Fix 4 — Apply writeLimiter + mediaLimiter to Routes

**Problem:** `writeLimiter` and `mediaLimiter` were constructed in `bootstrap/app.go` and passed to `NewRouter`, but `bootstrap/modules.go` discarded them with `_ = writeLimiter; _ = mediaLimiter`. Domain write endpoints and media upload endpoints were unprotected against write-amplification attacks.

**Fix:** Domain modules wrapped in `r.Group` with `writeLimiter.WriteMiddleware()`. Media module wrapped in a separate `r.Group` with `mediaLimiter.WriteMiddleware()`. Both groups nil-guard their respective limiters.

```go
r.Group(func(r chi.Router) {
    if writeLimiter != nil { r.Use(writeLimiter.WriteMiddleware()) }
    organizations.RegisterRoutes(...)
    // ... 7 more domain modules
})
r.Group(func(r chi.Router) {
    if mediaLimiter != nil { r.Use(mediaLimiter.WriteMiddleware()) }
    media.RegisterRoutes(...)
})
```

`WriteMiddleware()` applies rate limiting only to POST/PUT/PATCH/DELETE; GET/HEAD pass through unconditionally.

#### Fix 5 — Body Size Limit Returns 413 (not 400)

**Problem:** `BodySizeLimit` middleware wraps `r.Body` with `http.MaxBytesReader`. When the body exceeds the limit, `DecodeJSON` returns `errors.New("request body too large")` (a plain untyped error). `writeDecodeError` had no check for this case and fell through to `http.StatusBadRequest` (400). RFC 9110 specifies 413 Content Too Large for this condition.

**Fix applied across three files:**

1. `validator.go`: Added `ErrBodyTooLarge = errors.New("request body too large")` sentinel. `DecodeJSON` now returns this sentinel (instead of an anonymous error) when `*http.MaxBytesError` is detected.
2. `handler.go` (`writeDecodeError`): Added `errors.Is(err, validator.ErrBodyTooLarge)` as the first check → returns `http.StatusRequestEntityTooLarge` (413).
3. `bodysize.go`: Updated comment to reference 413, not 400.

#### Files Changed (Phase 15A)

| File | Change |
|------|--------|
| `internal/platform/validator/validator.go` | Added `ErrBodyTooLarge` sentinel; `DecodeJSON` returns it on `*http.MaxBytesError` |
| `internal/platform/middleware/ratelimit.go` | Added `WriteMiddleware()` — POST/PUT/PATCH/DELETE only |
| `internal/platform/middleware/bodysize.go` | Updated comment: 400 → 413 |
| `internal/auth/handler.go` | `writeDecodeError`: 413 check first; `Register`: async email + 30s timeout; `ForgotPassword`: 30s timeout; `ResendVerification`: 30s timeout |
| `internal/auth/service.go` | `ResendVerification`: `equalizeEnumerationTiming` on success path |
| `internal/bootstrap/modules.go` | `writeLimiter` and `mediaLimiter` wired via `r.Group` + `WriteMiddleware()` |

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
- [x] **Phase 14 — Email Infrastructure** — `internal/email/` package with `Provider` interface, `sesProvider` (AWS SES v2), `smtpProvider` (net/smtp), `LogProvider`, `NoOpProvider`; `Sender` with `SendVerificationEmail`, `SendPasswordResetEmail`, `SendResendVerificationEmail`; HTML + text templates via Go embed; `POST /api/v1/auth/resend-verification` endpoint with timing equalization; 9 unit tests + 8 integration tests; wired into bootstrap; new config fields: `EMAIL_PROVIDER`, `EMAIL_FROM_ADDRESS`, `APP_BASE_URL`, and provider-specific fields
- [x] **Phase 15A — P1 Defect Fixes** — Async `Register` email with 30s goroutine timeout; `equalizeEnumerationTiming` on `ResendVerification` success path; goroutine timeouts on `ForgotPassword` + `ResendVerification`; `writeLimiter` + `mediaLimiter` applied via `r.Group`/`WriteMiddleware()`; `ErrBodyTooLarge` sentinel → HTTP 413; new config fields: `RATE_LIMIT_WRITE_RPS/BURST`, `RATE_LIMIT_MEDIA_RPS/BURST`
- [x] **Phase 15A Remediation** — Domain write BodySizeLimit (P0-1); SMTP context-aware via goroutine+channel (P1-1); `equalizeResendVerificationTiming` 5-RT variant for full path equalization (P1-2); nil-sender service bypass removed (P1-3); `sync.WaitGroup` email goroutine drain + `DrainEmail` wired through shutdown chain (P1-4); `sync.Map` + `atomic.Int64` rate limiter cleanup (P1-5); `TrustedRealIP` middleware (P1-6); `Retry-After: 1` on 429 (P2-1); multipart/alternative MIME (P2-4); 5 regression tests added; new config field: `TRUSTED_PROXY_CIDRS`
- [x] **Phase 16 — Users Module** — Full users domain: list users (org-scoped, paginated, searchable), get user, update profile, change password, deactivate account; all endpoints permission-gated; BOLA guards; integration-tested
- [x] **Phase 17 — Domain Integration Tests** — 14+ integration tests for notifications (`internal/notifications/integration/`); tournament registration, match lifecycle, and match event coverage across domain packages; testcontainers-go with real PostgreSQL 17; all tests pass
- [x] **Phase 18 — Email Notification Delivery Workers** — Transactional outbox pattern extended to multi-channel (in_app + email); migration 000025 (4 delivery-state columns + partial index); `internal/notifworker/` package (`EmailWorker` with Start/Stop/Drain, soft-lease claim, 3-attempt retry, dead-letter); P2-5 auth fix (`UseAllUserEmailVerificationTokens` atomic with create); notification email templates; 90-day outbox retention in cleanup scheduler; 5 notifworker integration tests; new config field: `NOTIF_WORKER_INTERVAL_SECONDS` (default 30)
- [x] **Phase 20 — Real-Time Notifications (SSE)** — `internal/realtime.Hub` (in-process pub/sub; composite `subKey{orgID,userID}` map; buffer 32; non-blocking publish; `Shutdown` + `Done`); SSE stream endpoint `GET /notifications/stream?token=` (JWT via query param + Bearer fallback; org-scoped; 25s keepalive; platform-admin 403; graceful shutdown via `hub.Done()`); `DrainOutbox` calls `hub.Publish` post-commit; hub wired through bootstrap (`modules.go`, `app.go`); adversarial review: all findings PASS; 31 stream integration tests (14 inbox + 17 stream)
- [x] **Phase 20 Remediation** — Hub.Publish data race fixed (RLock held through entire fan-out); MT-1 `TestStream_PlatformAdminToken_Forbidden` (platform-admin token → 403 regression gate); MT-2 `TestStream_DrainOutbox_Idempotent_NoDuplicateSSE` strengthened (full first-frame drain before 300ms window); MT-3 `TestStream_SubscribeAfterShutdown_ImmediateDisconnect` (subscribe after shutdown → immediate EOF); mini adversarial review: all findings PASS
- [x] **Phase 21 — Observability and Operations** — `internal/platform/metrics.Registry` with 30+ `playarena_*` Prometheus metrics; `Metrics(reg)` HTTP middleware; `RequestLogger` structured slog middleware; internal observability server (`:9090`; `/metrics`, `/ready`, `/live`, optional `/debug/pprof/*`); DB pool scraper (15s); outbox + dead-letter scraper (30s); `health.Handler` with Check/Ready/Live methods; 5 Grafana dashboards; 12 Prometheus alert rules; CI/CD pipeline (`.github/workflows/ci.yml`); docker-compose updated with Prometheus v3 + Grafana 11.3; `WithMetrics()` injection on Hub, limiters, workers, notifications service
- [x] **Phase 22 — Rankings Module** — `internal/rankings/` full module (handler, service, repository, routes, dto, errors, models); migration 000027 (`player_tournament_stats` + `team_tournament_stats`); snapshot-on-completion in `tournaments.Service.Update`; nullable `rankingsRepo` field (nil = skip snapshot in tests); SQL `RANK()` window function + GROUP BY for leaderboards; win_rate computed in Go service layer; 10 integration tests including E2E snapshot-on-completion test
- [x] **Phase 23A — Role Assignment API** — `internal/members/` module (errors, model, dto, repository, service, handler, routes); 5 new SQL queries in `db/queries/rbac.sql` (`GrantOrgRole`, `ListOrgMembersWithRoles`, `GetUserGrantsInOrg`, `RevokeRoleFromUserInOrg`, `CountActiveOrgOwnersByOrg`); 4 endpoints under `/api/v1/organizations/{slug}/members`; all gate on `role.assign` permission; last-owner guard prevents removing the final `org_owner`; transactional audit logging with `AuditActionPermissionChange`; wired into `bootstrap/modules.go`
- [x] **Phase 23B — Media Entity Types Expanded** — Added `user` and `match` to `supportedEntityTypes` in `internal/media/service.go`; `MatchExists` and `UserExists` repo methods using existing `GetMatchByID` / `GetUserByID` queries; `user` entities check existence only (no org scope); `match` entities are org-scoped
- [x] **Phase 23C — Player-Based Individual Tournaments** — `tournament_registrations.CreateRequest` now accepts either `team_id` or `player_id`; service routes to `registerTeam` / `registerPlayer` based on `tournament.ParticipantType`; new `GetRegistrationByPlayer` SQL query; `ErrPlayerNotFound`, `ErrPlayerNotActive`, `ErrWrongParticipantType` errors added; handler error cases updated
- [x] **Phase 23D — N-Way Head-to-Head Standings** — Replaced strict 2-way h2h guard in `internal/standings/tiebreakers.go` with `buildH2HRanks()` that computes full sub-table ranking for all N tied participants; cyclic results (A>B>C>A) produce shared rank → fall through to score difference; `h2hCompare` removed

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
- [x] **Transactional email delivery** — `internal/email/` package with SES v2, SMTP, and LogProvider backends; verification + password-reset emails sent from auth handlers; async with 30-second timeout goroutines

### Required for feature completeness

- [x] **Users module** — Implemented in Phase 16: list, get, update profile, change password, deactivate; fully integration-tested.
- [x] **Rankings module** — Implemented in Phase 22: player and team leaderboards, snapshot-on-completion, 10 integration tests.
- [ ] **News module** — Stub exists; no business logic.
- [x] **N-way head-to-head resolution** — Implemented in Phase 23D. Full sub-table resolution for all N tied participants now handled by `buildH2HRanks()` in `internal/standings/tiebreakers.go`.

### Technical debt

- [ ] **`golang-jwt/jwt/v5` declared `indirect` in `go.mod`.** Running `go mod tidy` will correct this.
- [x] **Auth integration test infrastructure** — `internal/testutil/` (testcontainers-go + golang-migrate + pgxpool); `internal/testutil/fixtures/` (typed auth fixtures); 8 concurrency tests in `internal/auth/`. Phase 13B.1.
- [x] **Auth integration test suite** — 92 tests across 12 files in `internal/auth/integration/`; full lifecycle, middleware, refresh state-machine (Cases 2 & 3), password-reset, email-verification, suspension, multi-tenant, concurrency, CORS, rate-limiting, JWT-claims, and complete validation-path coverage. Phases 13B.2A + 13B.2B-A + 13B.2B-B.
- [x] **Integration tests for non-auth domains** — 16 integration test packages total: `auth/integration`, `matches/integration`, `match_events/integration`, `media/integration`, `members/integration`, `notifications/integration`, `notifworker/integration`, `organizations/integration`, `players/integration`, `rankings/integration`, `teams/integration`, `tournaments/integration`, `tournament_registrations/integration`, `users/integration`, `webhooks/integration`, `webhookworker/integration`. Coverage depth varies by package.
- [ ] **`internal/platform/middleware/auth.go`** contains only a placeholder comment. Can be removed or used to re-export `auth.RequireAuth`.
- [ ] **`internal/bootstrap/database.go`** is a stub. Can be removed or used for DB-level bootstrap helpers.
- [ ] **`internal/platform/cache/redis.go`** is a stub. No Redis dependency in `go.mod`. Delete if Redis is not planned.
- [ ] **`user_organization_roles.expires_at` expiry enforcement** — background job to revoke grants past their `expires_at`; only the query filter prevents expired grants from functioning, but rows are never cleaned up.
- [ ] **Validator limitation for pointer types** — `internal/platform/validator/validator.go` uses `fmt.Sprintf("%v", ...)` for field extraction; `omitempty` does not fire on nil `*string` fields. Validation of optional fields is handled in service layers as a workaround.
- [ ] **sqlc CASE-expression parameter naming** — Parameters inside CASE expressions (e.g. `CASE WHEN $13::TIMESTAMPTZ IS NOT NULL`) are named `Column{N}` by sqlc because it cannot determine field names for CASE expressions. Consuming service files (`matches/service.go`, `tournament_registrations/service.go`) reference `Column13`/`Column7` directly. Adding `sqlc.arg()` inside CASE expressions fails with `could not determine data type of parameter $N`.

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
| Phase 14 | Email Infrastructure — Provider abstraction, SES/SMTP/Log/NoOp, templates, resend-verification endpoint | **COMPLETE** |
| Phase 15A | P1 Defect Fixes — async email goroutines, timing equalization, 413 body limit, write/media rate limiters wired | **COMPLETE** |
| Phase 15A Remediation | P0-1/P1/P2 Defect Fixes from Phase 15A adversarial review — domain BodySizeLimit, SMTP ctx, ResendVerification timing, nil-sender fix, goroutine drain, sync.Map ratelimit, TrustedRealIP, Retry-After, multipart MIME | **COMPLETE** |
| Phase 16 | Users module — list, get, update profile, change password, deactivate | **COMPLETE** |
| Phase 17 | Domain integration tests — notifications, tournament registrations, matches, match events | **COMPLETE** |
| Phase 18 | Email notification delivery workers — EmailWorker, multi-channel DrainOutbox, migration 000025, outbox retention | **COMPLETE** |
| Phase 19 | Webhook notification delivery | **COMPLETE** |
| Phase 19 Remediation | Timestamp consistency fix, signature verification test, 429 retry test | **COMPLETE** |
| Phase 20 | Real-Time Notifications — SSE stream, in-process Hub, 31 integration tests | **COMPLETE** |
| Phase 20 Remediation | Hub data race fix; platform-admin 403 gate test; full-frame drain assertion; subscribe-after-shutdown test | **COMPLETE** |
| Phase 21 | Observability and Operations — Prometheus metrics, Grafana dashboards, health/readiness/liveness, pprof, CI/CD pipeline | **COMPLETE** |
| Phase 22 | Rankings Module — snapshot-on-completion, player/team leaderboards, 10 integration tests | **COMPLETE** |

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

### Phase 14 — Email Infrastructure (Complete)

Email delivery system implemented and production-hardened. `internal/email/` package provides a `Provider` interface with four implementations: `sesProvider` (AWS SES v2), `smtpProvider` (net/smtp with STARTTLS and implicit TLS modes), `LogProvider` (slog — dev default), `NoOpProvider` (in-memory capture for tests). `Sender` renders HTML + text templates via Go embed FS and delegates to the provider. Auth domain sends verification emails on register, password reset emails on `ForgotPassword`, and resend-verification emails on `ResendVerification`. New `POST /api/v1/auth/resend-verification` endpoint added (enumeration-resistant; always 200). Production config blocks `noop`/`log` providers and enforces `https://` on `APP_BASE_URL`. 9 unit tests + 8 integration tests. All P1 defects fixed in Phase 15A.

New dependency: `aws-sdk-go-v2` (SES v2 provider).  
New package: `internal/email/`.  
New config fields: `EMAIL_PROVIDER`, `EMAIL_FROM_ADDRESS`, `EMAIL_FROM_NAME`, `APP_BASE_URL`, `EMAIL_SES_*`, `EMAIL_SMTP_*`.

---

### Phase 15A — P1 Defect Fixes (Complete)

Five P1 production blockers from the Phase 14 adversarial review fixed. (1) `Register` email moved to async goroutine with 30-second context timeout. (2) `ResendVerification` timing equalization added to success path — all enumeration paths now have comparable round-trip profiles. (3) 30-second timeouts added to `ForgotPassword` and `ResendVerification` goroutines. (4) `writeLimiter` and `mediaLimiter` wired into domain modules via `r.Group`/`WriteMiddleware()` — domain write endpoints and media uploads now rate-limited. (5) `ErrBodyTooLarge` sentinel added to validator; `writeDecodeError` now returns HTTP 413 instead of 400 when the body limit is exceeded. All changes surgical — no new features, no signature changes to module APIs.

New config fields: `RATE_LIMIT_WRITE_RPS`, `RATE_LIMIT_WRITE_BURST`, `RATE_LIMIT_MEDIA_RPS`, `RATE_LIMIT_MEDIA_BURST`.

Subsequently adversarially reviewed; 9 additional defects found and remediated in Phase 15A Remediation.

---

### Phase 15A Remediation — Phase 15A Adversarial Review Findings (Complete)

**Status: COMPLETE. Adversarial review: all 9 findings PASS. Production readiness: PASS.**

Nine defects from the Phase 15A adversarial review fixed. No architecture changes; no new features.

#### Fix 1 — P0-1: Domain Write BodySizeLimit

Applied `r.Use(middleware.BodySizeLimit(64 * 1024))` as the first middleware in the domain write `r.Group` in `modules.go`, before the write-limiter. Covers organizations, players, teams, tournaments, tournament registrations, matches, match events, and notifications. Auth routes already had their own 64 KB limit.

#### Fix 2 — P1-1: SMTP Context Cancellation

`smtpProvider.Send` now honours `ctx` via a goroutine+channel pattern. An inner goroutine performs the blocking SMTP exchange and sends the result on a buffered channel. The outer function selects on the channel or `ctx.Done()`. Context cancellation returns `ctx.Err()` within the deadline. The inner goroutine may linger until the OS TCP timeout (~2 min) — documented, bounded, accepted.

#### Fix 3 — P1-2: ResendVerification Timing Equalization

Added `equalizeResendVerificationTiming` (5 DB round-trips: BEGIN + SELECT 1 + SELECT NOW() + SELECT 1 + COMMIT) in `repository.go`. All five `ResendVerification` paths now total 6 round-trips:

| Path | Round-trips |
|------|------------|
| Not-found | 1 (SELECT) + 5 (equalize) = 6 |
| Already-active | 1 (SELECT) + 5 (equalize) = 6 |
| Token-gen-failure | 1 (SELECT) + 5 (equalize) = 6 |
| INSERT-failure | 1 (SELECT) + 1 (INSERT) + 4 (equalizeEnumeration) = 6 |
| Success | 1 (SELECT) + 1 (INSERT) + 4 (equalizeEnumeration) = 6 |

#### Fix 4 — P1-3: Nil-emailSender Service Bypass

`handler.ResendVerification` previously gated the entire service call on `h.emailSender != nil`. Timing equalization and token creation were skipped when no sender was configured. Fixed: service call is now unconditional. The email goroutine is spawned only when `h.emailSender != nil && svcErr == nil && rawToken != ""`.

#### Fix 5 — P1-4: Goroutine Drain on Graceful Shutdown

`Handler` gains `wg sync.WaitGroup`. All three email goroutines (`Register`, `ForgotPassword`, `ResendVerification`) call `h.wg.Add(1)` before spawning and `defer h.wg.Done()` inside. `DrainEmail(ctx)` blocks on `wg.Wait()` with context timeout. Wired through the full chain: `RegisterRoutes` returns `*Handler`; `registerModules` returns `*auth.Handler`; `NewRouter` returns `(http.Handler, *auth.Handler)`; `App` stores `authHandler`; `App.Shutdown` calls `authHandler.DrainEmail(ctx)` before stopping rate limiters. Integration test cleanup now calls `DrainEmail(ctx)` too.

#### Fix 6 — P1-5: Rate Limiter Cleanup Lock Contention

Replaced the mutex-protected `map[string]*rate.Limiter` with `sync.Map` (key: IP string, value: `*ipEntry`). `ipEntry.lastSeen` is an `atomic.Int64` (Unix nanoseconds) — updated lock-free on every request. `cleanup` uses `sync.Map.Range`, which does not hold a global mutex during iteration, eliminating the O(n) stall that previously blocked all concurrent rate-limit checks during each cleanup cycle.

#### Fix 7 — P1-6: TrustedRealIP Middleware

New `middleware/realip.go`. `TrustedRealIP(trustedCIDRs []string)` rewrites `RemoteAddr` from `X-Forwarded-For` / `X-Real-IP` only when the direct TCP peer is in a trusted CIDR, preventing rate-limit bypass via spoofed forwarding headers from untrusted clients. When `trustedCIDRs` is empty, falls back to `chimw.RealIP` for backward compatibility. Replaces `chimw.RealIP` in `router.go`. New config field: `TRUSTED_PROXY_CIDRS` (comma-separated CIDR list, optional).

#### Fix 8 — P2-1: Retry-After Header on 429

Both `Middleware()` and `WriteMiddleware()` now set `Retry-After: 1` before `WriteHeader(429)`. Static value; communicates minimum back-off to clients.

#### Fix 9 — P2-4: Multipart/Alternative MIME

`smtpProvider.buildMessage` emits `multipart/alternative` when both `TextBody` and `HTMLBody` are present. Boundary generated from `time.Now().UnixNano()` (unique per message). Plain-text part is listed before HTML (RFC 2046 preferred-last ordering). Single-part messages are unaffected.

#### Files Changed (Phase 15A Remediation)

| File | Change |
|------|--------|
| `internal/email/smtp.go` | Context-aware `Send` (goroutine+channel); `buildMessage` emits multipart/alternative; added `"time"` import |
| `internal/auth/repository.go` | Added `equalizeResendVerificationTiming` (5-RT variant) |
| `internal/auth/service.go` | `ResendVerification`: per-path equalization using both 4-RT and 5-RT variants |
| `internal/auth/handler.go` | `sync.WaitGroup wg`; `DrainEmail(ctx)`; P1-3 fix (service always called); `wg.Add/Done` in all email goroutines |
| `internal/auth/routes.go` | `RegisterRoutes` return type changed to `*Handler`; added `return h` |
| `internal/platform/middleware/ratelimit.go` | `sync.Map` + `atomic.Int64 lastSeen`; `Retry-After: 1` header on 429 |
| `internal/platform/middleware/realip.go` | **New file** — `TrustedRealIP` middleware |
| `internal/platform/config/config.go` | Added `TrustedProxyCIDRs []string` field; `getEnvStringSlice("TRUSTED_PROXY_CIDRS", nil)` |
| `internal/bootstrap/router.go` | `TrustedRealIP` replaces `chimw.RealIP`; returns `(http.Handler, *auth.Handler)` |
| `internal/bootstrap/app.go` | `authHandler *auth.Handler` field; `DrainEmail(ctx)` in `Shutdown` |
| `internal/bootstrap/modules.go` | `BodySizeLimit(64*1024)` in domain write group; returns `*auth.Handler` |
| `internal/auth/integration/server_test.go` | `handler *auth.Handler` field; `DrainEmail` in cleanup; `buildServerWithNilSender` added |
| `internal/auth/integration/remediation_test.go` | **New file** — 5 regression tests (P0-1, P1-3, P1-4 ×2, P2-1) |

#### Adversarial Review Results (Phase 15A Remediation)

All 9 findings PASS. Remaining P2 items (not blocking production):

| Defect | Description |
|--------|-------------|
| D1 — P2 | `TrustedRealIP` calls `chimw.RealIP(next)` per trusted request instead of pre-computing the wrapper once; unnecessary per-request allocation |
| D2 — P2 | `parseCIDRs` silently drops malformed CIDR strings; misconfigured `TRUSTED_PROXY_CIDRS` fails open |
| D3 — P2 | `Retry-After: 1` is static; does not reflect actual token-bucket refill time (`rate.Limiter.Reserve().Delay()`) |
| D4 — P2 | MIME body parts omit `Content-Transfer-Encoding` headers; non-ASCII bodies may be mis-encoded by strict SMTP relays |
| MT1 — P2 | No integration test for oversized body on domain write routes (only auth routes tested by `TestBodySizeLimit_Auth_Regression`) |
| MT2 — P2 | No test for SMTP context cancellation (requires mock hanging SMTP server; deferred) |
| MT3 — P2 | No test for TrustedRealIP trusted/untrusted CIDR behaviour |
| MT4 — P2 | No unit test for multipart MIME structure from `buildMessage` |

Deferred from this pass: **P2-5 — token accumulation** (`email_verification_tokens` rows are not expired per-user before a new one is inserted on `ResendVerification`; causes unbounded table growth under sustained load). **Resolved in Phase 18** — `CreateEmailVerificationToken` now atomically runs `UseAllUserEmailVerificationTokens` + insert in a single transaction.

---

### Phase 16 — Users Module (Complete)

**Status: COMPLETE.**

Full users domain implemented. Closes the largest remaining feature-completeness gap that existed since the `internal/users/` stub was created in Phase 1.

**Deliverables:**

- `GET /api/v1/users` — platform admin only (`user.manage`); paginated with search
- `GET /api/v1/users/{id}` — self or `user.manage`; BOLA-guarded
- `PATCH /api/v1/users/{id}` — update `full_name`, `username`; self-only or `user.manage`; BOLA-guarded
- `POST /api/v1/users/{id}/change-password` — verify current password, hash new, revoke all active refresh tokens; single atomic transaction
- `POST /api/v1/users/{id}/deactivate` — admin action; sets `status = inactive`; revokes all sessions; audit logged
- Full integration test suite; all tests PASS

---

### Phase 17 — Domain Integration Tests (Complete)

**Status: COMPLETE.**

Delivered integration-test coverage for the notifications domain and cross-domain interaction paths using real PostgreSQL 17 via testcontainers-go.

**Deliverables:**

- `internal/notifications/integration/` — 14 tests covering:
  - End-to-end notification lifecycle (outbox write → drain → inbox delivery)
  - Multi-channel fan-out (in_app + email rows created per drain)
  - Preference opt-out (disabled event type suppresses delivery)
  - Drain idempotency (re-draining same outbox entry produces no duplicates)
  - Notification CRUD (mark read, mark all read, soft delete, list, count)
  - Tournament registration approval triggering `tournament_registration_approved` event
  - Cross-domain notification triggers (matches, tournaments, registrations)
- All 14 tests PASS (transient Docker timing issue investigated and confirmed non-code)

---

### Phase 18 — Email Notification Delivery Workers (Complete)

**Status: COMPLETE. Adversarial review: PASS. `go build ./...` + `go vet ./...` clean.**

Implemented async email delivery for notification rows via an embedded `EmailWorker`. Closes the email delivery channel that was left as "future delivery workers" after Phase 12.

**Architecture decisions (locked):**

- **Embedded worker** — same binary as API server; lifecycle through `App.Start`/`App.Shutdown`; no separate process or queue service
- **At-least-once delivery** — `sent_at IS NULL` guard on `RecordSuccess` prevents duplicate state writes; actual re-send possible on crash-before-record
- **Soft-lease claim** — `FOR UPDATE SKIP LOCKED` inside a subquery; `lease_expires_at` prevents permanent lock on worker crash
- **Retry strategy** — 1 min → 5 min → 3 attempts max → `failed_permanently = TRUE` (dead-letter); manual reset required
- **Observability** — structured `slog` only; no metrics library

**Deliverables:**

| Component | Description |
|-----------|-------------|
| Migration 000025 | 4 delivery-state columns + `idx_notifications_email_pending` partial index on `notifications` |
| `db/queries/notifications.sql` | `ClaimEmailNotificationsForDelivery`, `RecordEmailDeliverySuccess`, `RecordEmailDeliveryFailure`, `DeleteOldProcessedOutboxEntries` |
| `notifications/repository.go` | DrainOutbox extended to fan-out to `in_app` + `email` channels; conditional `sent_at` (non-null for in_app, null for email) |
| `email/templates/notification_event.{html,txt}` | New notification event email template pair |
| `email/sender.go` | Added `SendNotificationEmail` method |
| `notifworker/repository.go` | `ClaimBatch`, `RecordSuccess`, `RecordFailure`, `GetUserByID`, `GetOrgSlugByID` |
| `notifworker/worker.go` | `EmailWorker` — `Start`/`Stop`/`Drain`; `runOnce` tick loop; `deliver`; `retryDelay`; `eventLabel`; `recordFailure` |
| `notifworker/integration/worker_test.go` | 5 integration tests: happy path, retry on provider failure, permanent failure after 3 attempts, duplicate idempotency, outbox drain cascade |
| `platform/config/config.go` | `NOTIF_WORKER_INTERVAL_SECONDS` (default 30) |
| `bootstrap/modules.go` | Constructs `EmailWorker`; returns `(*auth.Handler, *notifworker.EmailWorker)` |
| `bootstrap/router.go` | Returns `(http.Handler, *auth.Handler, *notifworker.EmailWorker)` |
| `bootstrap/app.go` | `notifEmailWorker` field; `Handler()` calls `Start()`; `Shutdown()` calls `Stop()` + `Drain(ctx)` |
| `cleanup/scheduler.go` | 90-day outbox retention via `DeleteOldProcessedOutboxEntries`; ON DELETE CASCADE removes notification rows |
| `auth/repository.go` | P2-5 fix — `CreateEmailVerificationToken` wraps `UseAllUserEmailVerificationTokens` + insert in transaction |
| `auth/integration/remediation_test.go` | P2-5 regression test updated to count all tokens (not just unused) to correctly detect P1-3 regression |

**Adversarial review findings (no P0 issues):**
- CASCADE FK on `notifications.outbox_id` confirmed from migration 000022 — outbox retention purge correctly cascades
- `perm := n.AttemptCount >= maxAttempts` (3 >= 3 = true after 3rd attempt) — retry/permanent-failure boundary correct
- `FOR UPDATE SKIP LOCKED` subquery pattern is correct PostgreSQL concurrency idiom
- `nameOrEmail` fallback handles empty display names correctly

---

### Phase 19 — Webhook Notification Delivery (Complete)

**Status: COMPLETE. Adversarial review: PASS (after remediation). `go build ./...` + `go vet ./...` clean.**

Phase 19 closes the third notification delivery channel. Webhook endpoints are registered per organization and receive signed HTTP POST payloads for every domain event processed by `DrainOutbox`.

#### Architecture Decisions

- **Separate `webhook_deliveries` table** — `notifications` has `UNIQUE(outbox_id, user_id, channel)` + `user_id NOT NULL`. Webhook delivery is endpoint-centric (one row per endpoint, not per user), requiring its own table with `UNIQUE(outbox_id, endpoint_id)`.
- **AES-256-GCM at rest** — Webhook secrets must be decryptable at delivery time for HMAC signing (bcrypt/PBKDF2 would be one-way). Secret stored as `nonce || ciphertext || tag` in `secret_ciphertext BYTEA`. Raw secret returned once at creation via `CreateResponse.RawSecret`; never retrievable again.
- **Two-layer SSRF protection** — Registration-time: `ValidateURL` rejects non-HTTPS, localhost/.localhost, and known-private IP literals. Delivery-time: `SSRFSafeTransport` resolves DNS, verifies ALL resolved IPs are public, then dials the first resolved IP directly (no re-resolution) to defeat DNS rebinding attacks.
- **Injectable `http.Client`** — Production worker uses `SSRFSafeTransport`; tests inject `http.DefaultTransport` to reach plain-HTTP `httptest.Server` without SSRF blocking.
- **HMAC-SHA256 signing** — Canonical string: `<unix_ts>\n<delivery_uuid>\n<body_bytes>`. Single `time.Now()` captured once for both `payload.timestamp` (RFC3339) and `X-PlayArena-Timestamp` (Unix) — timestamp divergence was identified as a P1 finding and fixed in Phase 19 Remediation.
- **Retry semantics** — 1 min → 5 min → 15 min → dead-letter after 3 attempts. HTTP 4xx (except 429) → immediate permanent failure. HTTP 429 + 5xx + network errors → retry. Mirrors EmailWorker pattern.
- **Fan-out atomicity** — `GetActiveWebhookEndpointsForOrg` + `CreateWebhookDelivery` (ON CONFLICT DO NOTHING) run inside the `DrainOutbox` transaction before `MarkOutboxEntryProcessed`. Fan-out is atomic with drain completion.
- **Cleanup** — `DeleteOldWebhookDeliveries` added to cleanup scheduler; retains 30 days of delivered rows.

#### Deliverables

| Component | Description |
|-----------|-------------|
| Migration 000026 | `webhook_endpoints` + `webhook_deliveries` tables; 4 webhook RBAC permissions seeded |
| `db/queries/webhooks.sql` | 12 queries: ClaimWebhookDeliveriesForDelivery (FOR UPDATE SKIP LOCKED), CreateWebhookDelivery (ON CONFLICT DO NOTHING), GetActiveWebhookEndpointsForOrg, RecordWebhookDeliverySuccess, RecordWebhookDeliveryFailure, + CRUD |
| `db/sqlc/webhooks.sql.go` | Generated — `WebhookEndpoint` + `WebhookDelivery` structs + all query functions |
| `webhooks/ssrf.go` | `ValidateURL` (registration-time); `SSRFSafeTransport` (delivery-time DNS validation + dial-by-resolved-IP) |
| `webhooks/crypto.go` | `GenerateSecret` (32-byte CSPRNG, base64url); `EncryptSecret` / `DecryptSecret` (AES-256-GCM) |
| `webhooks/` (service, repo, handler, routes, dto, errors) | Full CRUD module; `toResponse` never exposes `secret_ciphertext` |
| `webhooks/integration/webhook_test.go` | 12 integration tests: CRUD happy paths, BOLA, tenant isolation, secret not exposed, crypto round-trip, SSRF URL blocking |
| `webhookworker/worker.go` | `WebhookWorker` — Start/Stop/Drain; decrypt secret → build `webhookPayload` envelope → HMAC-SHA256 → POST → record |
| `webhookworker/integration/worker_test.go` | 10 integration tests (after remediation): delivery, idempotency, **signature verification**, retry on 5xx, permanent failure on 4xx, **HTTP 429 retry**, dead-letter after 3 attempts, concurrent workers, start/stop, tenant isolation |
| `notifications/repository.go` | DrainOutbox extended with webhook fan-out loop (after user-centric fan-out, before MarkOutboxEntryProcessed) |
| `platform/config/config.go` | `WEBHOOK_SECRET_KEY` (required) + `WEBHOOK_WORKER_INTERVAL_SECONDS` (default 30) |
| `bootstrap/` (modules, router, app) | Constructs and wires `WebhookWorker` alongside `EmailWorker` |
| `cleanup/scheduler.go` | 30-day webhook delivery retention via `DeleteOldWebhookDeliveries` |

#### Phase 19 Adversarial Review (initial)

| Area | Result |
|------|--------|
| SSRF — registration-time | PASS |
| SSRF — delivery-time DNS + dial-by-IP | PASS |
| SSRF — DNS rebinding protection | PASS |
| SSRF — redirect handling | PASS |
| Secret storage (AES-256-GCM) | PASS |
| Secret exposure via API | PASS |
| Fan-out correctness (endpoint-centric, atomic) | PASS |
| Fan-out idempotency (ON CONFLICT DO NOTHING) | PASS |
| Worker lease/claim (SKIP LOCKED) | PASS |
| Worker retry semantics | PASS |
| Worker dead-letter (3 attempts) | PASS |
| Worker 4xx immediate dead-letter | PASS |
| Worker idempotency (sent_at IS NULL guard) | PASS |
| Tenant isolation (CRUD + worker delivery) | PASS |
| Signature timestamp consistency | **FAIL** (P1-001 — fixed in remediation) |
| Signature verification test | **FAIL** (P1-002 — fixed in remediation) |
| HTTP 429 retry test | **FAIL** (P1-003 — fixed in remediation) |

---

### Phase 19 Remediation (Complete)

**Status: COMPLETE. All P1 findings resolved. Build and tests clean.**

#### P1-001 — Timestamp Consistency Fix

`deliver()` in `webhookworker/worker.go` previously called `time.Now()` twice — once for `envelope.Timestamp` (RFC3339 body field) and once for `tsUnix` (HMAC canonical string). A receiver implementing signature verification against `payload.timestamp` would fail on every delivery.

Fix: single `now := time.Now().UTC()` captured before envelope construction, used for both `now.Format(time.RFC3339)` and `strconv.FormatInt(now.Unix(), 10)`.

#### P1-002 — Signature Verification Test

`TestWebhookWorker_SignatureVerification` added to `webhookworker/integration/worker_test.go`:
1. Captures `X-PlayArena-Signature`, `X-PlayArena-Timestamp`, `X-PlayArena-Event-ID`, and body from the test server.
2. Recomputes `HMAC-SHA256(testRawSecret, ts+"\n"+eventID+"\n"+body)` and asserts equality.
3. Asserts `bodyTime.Unix() == headerUnix` — timestamp consistency between body and header.
4. Mutates the body and asserts the signature changes.

#### P1-003 — HTTP 429 Retry Test

`TestWebhookWorker_HTTP429_Retry` added: endpoint returns 429, asserts `failed_permanently = FALSE`, `sent_at = NULL`, `attempt_count = 1`. Regression gate: moving 429 into the permanent-failure branch would set `failed_permanently = TRUE` and fail the test.

#### P2 Cleanups Applied

- Removed `time.Sleep(200ms/100ms)` from `TestWebhookWorker_ConcurrentWorkers` and `TestWebhookWorker_TenantIsolation` (sleeps were unnecessary — `deliver()` is synchronous inside `Drain`).
- Fixed misleading `// 3xx, 5xx, 429 → retry` comment (Go client follows redirects; raw 3xx is never the final status).
- Removed false test expectation for `localhost.example.com` in `TestWebhook_SSRF_ValidateURL` — `localhost.example.com` is a legitimate public domain that is NOT blocked at registration time; DNS rebinding protection at delivery time covers it.

#### Self-Review Results (post-remediation)

| Question | Answer |
|----------|--------|
| Can a signature-format regression go undetected? | No — `TestWebhookWorker_SignatureVerification` recomputes and compares |
| Can timestamp divergence go undetected? | No — `bodyTime.Unix() == headerUnix` assertion catches two-`time.Now()` regression |
| Can 429 be silently converted to permanent failure? | No — `TestWebhookWorker_HTTP429_Retry` asserts `failed_permanently = FALSE` |

---

### Phase 20 — Real-Time Notifications (Complete)

**Status: COMPLETE. Adversarial review: PASS (after remediation). `go build ./...` + `go vet ./...` clean. 31 stream integration tests.**

Phase 20 adds a Server-Sent Events push channel so connected browser clients receive new notifications in real time without polling.

#### Architecture Decisions

- **In-process pub/sub Hub** (`internal/realtime.Hub`) — composite `subKey{orgID [16]byte, userID [16]byte}` map of buffered channels (size 32); `sync.RWMutex`; at-most-once delivery; non-blocking publish drops events on full buffer (client recovers via REST GET).
- **RLock held through entire fan-out** — `Publish` holds the read lock for the full iteration and all channel sends. This eliminates a data race (concurrent `Unsubscribe` modifying the inner map) and a send-on-closed-channel panic that would occur if `Unsubscribe`/`Shutdown` closed a channel between its read from the map and the send.
- **SSE over WebSocket** — EventSource has built-in reconnect; no bidirectional messaging needed; simpler proxy compatibility. Keepalive `:\n\n` frames every 25 seconds prevent proxy idle timeouts.
- **JWT via `?token=` query param** — `EventSource` cannot set `Authorization` headers. `Authorization: Bearer` fallback supported for curl/testing.
- **Tenant isolation at auth time** — `claims.OrganizationID` compared against DB-resolved `org.ID` (UUID string). Platform admin tokens (`OrganizationID == ""`) are rejected with 403. Subscription uses the DB-authoritative org UUID, not the JWT claim.
- **Graceful shutdown** — `Hub.Shutdown()` closes the `done` channel then closes all subscriber channels under write lock. SSE handlers select on `hub.Done()` as a fourth case; they return immediately on shutdown regardless of client presence.
- **Publish-after-commit** — `DrainOutbox` calls `hub.Publish` after the DB transaction commits. Nil hub is a no-op.
- **No migration** — Phase 20 is purely in-process; no new DB tables or columns.

#### Deliverables

| Component | Description |
|-----------|-------------|
| `internal/realtime/hub.go` | `Hub` — Subscribe, Unsubscribe, Publish (RLock-through-fan-out), Shutdown, Done |
| `notifications/handler.go` — `Stream` | SSE handler; JWT auth via `?token=`/Bearer; org resolution + tenant check; 25s keepalive ticker; 4-case select loop |
| `notifications/routes.go` | `/stream` registered outside `RequireAuth` group (before `/{id}` to prevent routing collision) |
| `notifications/service.go` | `DrainOutbox` calls `s.hub.Publish` for each `in_app` row created; nil-safe |
| `bootstrap/modules.go` | `hub := realtime.NewHub()` wired into `notifSvc` and `notifications.RegisterRoutes` |
| `bootstrap/app.go` | `realtimeHub *realtime.Hub` field; `a.realtimeHub.Shutdown()` in `Shutdown()` (after email/webhook workers, before scheduler) |
| `notifications/integration/stream_test.go` | 17 SSE stream integration tests |

#### SSE Stream Tests (17)

| Test | What it guards |
|------|---------------|
| `TestStream_NoToken_Unauthorized` | Missing token → 401 |
| `TestStream_InvalidToken_Unauthorized` | Garbage token → 401 |
| `TestStream_ExpiredToken_Unauthorized` | Expired JWT → 401 |
| `TestStream_WrongOrg_Forbidden` | Valid JWT for Org A, URL for Org B → 403 |
| `TestStream_Connect_ContentType` | `text/event-stream` response header |
| `TestStream_Connect_ReceivesKeepalive` | Initial `:\n\n` keepalive frame delivered |
| `TestStream_BearerHeaderAuth` | `Authorization: Bearer` fallback works |
| `TestStream_DrainOutbox_DeliversSseEvent` | End-to-end: outbox drain → SSE `event: notification` frame |
| `TestStream_ActorExcluded_NoSseEvent` | Actor not notified of their own action |
| `TestStream_HubShutdown_DisconnectsClients` | Hub shutdown closes active connections |
| `TestStream_CrossUserIsolation` | User A's events not delivered to User B's stream |
| `TestStream_MultipleConcurrentClients` | Multiple simultaneous clients each receive their own events |
| `TestStream_ClientDisconnect_Cleanup` | Disconnect + reconnect succeeds cleanly |
| `TestStream_DrainOutbox_Idempotent_NoDuplicateSSE` | Second drain produces no duplicate SSE event (full-frame consumed before 300ms window — MT-2) |
| `TestStream_PlatformAdminToken_Forbidden` | Platform admin token (empty orgID) → 403 (MT-1) |
| `TestStream_SubscribeAfterShutdown_ImmediateDisconnect` | Connect to shut-down hub → immediate EOF (MT-3) |

---

### Phase 20 Remediation (Complete)

**Status: COMPLETE. Mini adversarial review: all findings PASS.**

Four correctness and regression-resistance improvements identified during adversarial review of Phase 20.

#### Hub.Publish Data Race Fix (P1)

**Problem:** Original `Publish` released the `RLock` before iterating `h.subs[key]`. Between the unlock and the iteration, a concurrent `Unsubscribe` (or `Shutdown`) could: (a) modify the inner channel map → data race detectable by `-race`; (b) close the channel → send-on-closed-channel panic.

**Fix:** `defer h.mu.RUnlock()` — the read lock is held for the entire duration of the fan-out (map iteration + all channel sends). Non-blocking sends (`select { default: }`) prevent deadlock even under the lock.

#### MT-1 — Platform Admin Token Gate Test

`makePlatformAdminToken(t, jwtSecret)` constructs a valid HS256 JWT with `OrganizationID: ""`. `TestStream_PlatformAdminToken_Forbidden` connects it to a real org's SSE stream and asserts 403. Regression gate: removing the `claims.OrganizationID != pgutil.UUIDToString(org.ID)` check at `handler.go:228` causes the test to receive 200 and fail.

#### MT-2 — Full First-Frame Drain Before Duplicate Window

`TestStream_DrainOutbox_Idempotent_NoDuplicateSSE` previously started the 300ms duplicate-check window after consuming only the `event:` line. The buffered `data:` line from the first SSE frame immediately fired the `select`, making the test exit before the window expired. Fix: explicitly consume the `data:` line via `waitLine` and the blank-line terminator via a dedicated `select { case <-lines: }` before opening the 300ms window.

#### MT-3 — Subscribe-After-Shutdown Immediate Disconnect Test

`TestStream_SubscribeAfterShutdown_ImmediateDisconnect` calls `ts.hub.Shutdown()`, then connects a new SSE stream. The `hub.Done()` channel is already closed, so the handler's `case <-h.hub.Done(): return` fires on the first select iteration. The test asserts the response body reaches EOF within 2 seconds. Regression gate: removing the `hub.Done()` case from the handler loop causes the connection to stay open indefinitely (ticker keeps firing), and the test times out at 2s.

---

### Phase 21 — Observability and Operations (Complete)

**Status: COMPLETE. `go build ./...` + `go vet ./...` clean. All metrics wired end-to-end.**

Phase 21 delivered the full production observability stack: Prometheus metrics instrumentation across every subsystem, Grafana dashboards, health + readiness + liveness probes, CI/CD pipeline, pprof profiling, and the docker-compose compose file updated with Prometheus and Grafana services.

#### Architecture Decisions

- **Separate internal server** — Observability endpoints (`:9090`) are served by a distinct `*http.Server` with its own address, timeouts, and router. Never exposed via the public load balancer or load balancer port mapping.
- **Custom Prometheus registry** — All metrics use a fresh `prometheus.NewRegistry()` (not `prometheus.DefaultRegisterer`) to prevent cross-test pollution and avoid implicit registration side-effects. Go and process collectors registered alongside application metrics.
- **`playarena_` prefix** — All application metrics use the `playarena_` prefix per Prometheus naming convention.
- **WithMetrics() injection pattern** — Components that produce metrics accept a `*metrics.Registry` and expose a `WithMetrics(reg, ...)` builder method. `nil` registry is always safe (components check for `nil` before recording). This avoids import cycles and keeps metrics registration co-located with the component rather than in bootstrap.
- **Cardinality rules** — Route labels use chi route patterns (`/api/v1/organizations/{slug}`), never actual URL values. Worker, result, and channel labels drawn from bounded enums. user_id, org_id, request_id, delivery_id are never used as labels.
- **Background scrapers** — DB pool stats and outbox depth are read by background goroutines on fixed intervals (15s for pool, 30s for outbox/dead-letters) without blocking request paths. A shared `done` channel ties scraper lifetime to `App.Shutdown`.

#### Deliverables

| Component | Description |
|-----------|-------------|
| `internal/platform/metrics/metrics.go` | `Registry` — 30+ metrics across HTTP, rate-limiting, DB pool, auth, notifications, email worker, webhook worker, realtime hub; `New()` constructor |
| `internal/platform/metrics/metrics_test.go` | Unit tests for registry construction |
| `internal/platform/middleware/metrics.go` | `Metrics(reg)` middleware — `playarena_http_requests_total`, `playarena_http_request_duration_seconds`, `playarena_http_requests_in_flight` |
| `internal/platform/middleware/logging.go` | `RequestLogger(log)` — per-request slog structured logging (method, path, status, latency, request_id) |
| `internal/bootstrap/observability.go` | `newInternalServer` (`:9090`); `startDBPoolScraper` (15s, pgxpool.Stat()); `startOutboxMetricsScraper` (30s, pending+dead-letter counts) |
| `internal/health/handler.go` | `Check` (`GET /api/v1/health` — DB ping, 200/503); `Ready` (Kubernetes readiness, `/ready`); `Live` (liveness, `/live`) |
| `internal/health/handler_test.go` | Unit tests for health handlers |
| `deploy/prometheus/prometheus.yml` | Prometheus config scraping `api:9090/metrics` every 15s |
| `deploy/prometheus/alerts.yaml` | 10 alert rules: DatabaseUnavailable, HighErrorRate5xx, EmailWorkerStuck, WebhookWorkerStuck, EmailDeadLetterBurst, WebhookDeadLetterBurst, HighP99Latency, DBPoolNearSaturation, AuthLoginAnomalySpike, TokenReplayDetected, OutboxBacklogHigh, RealtimeDroppedEvents |
| `deploy/grafana/dashboards/*.json` | 5 Grafana dashboards: api-overview, auth, infrastructure, notifications, realtime |
| `deploy/grafana/provisioning/` | Auto-provision datasource (Prometheus) and dashboards |
| `docker-compose.yml` | Updated with `prometheus` (prom/prometheus:v3.0.0) and `grafana` (grafana/grafana:11.3.0) services; api exposes `:9090` |
| `.github/workflows/ci.yml` | CI pipeline: `go vet ./...` → `go build ./...` → `go test -race` for unit tests (platform, email, realtime, webhooks); triggered on push/PR to `main` for `backend/**` |
| `internal/platform/config/config.go` | Added `AppInternalPort`, `PprofEnabled`, `DrainTimeoutSeconds`, `AuditLogRetentionDays` config fields |

#### New Config Fields (Phase 21)

| Variable | Default | Purpose |
|----------|---------|---------|
| `APP_INTERNAL_PORT` | `9090` | Internal observability server port |
| `PPROF_ENABLED` | `false` | Enables `/debug/pprof/*` on internal server |
| `DRAIN_TIMEOUT_SECONDS` | `30` | Timeout for DrainOutbox operations (surfaced in metrics) |
| `AUDIT_LOG_RETENTION_DAYS` | `90` | How many days of audit log rows to keep in cleanup scheduler |

#### Metrics Instrumented

| Metric | Type | Labels | Source |
|--------|------|--------|--------|
| `playarena_http_requests_total` | Counter | method, route, status_code | Metrics middleware |
| `playarena_http_request_duration_seconds` | Histogram | method, route, status_class | Metrics middleware |
| `playarena_http_requests_in_flight` | Gauge | — | Metrics middleware |
| `playarena_rate_limit_rejections_total` | Counter | limiter | IPRateLimiter.WithMetrics |
| `playarena_db_pool_open/acquired/idle/constructing/max_connections` | Gauge | — | DBPoolScraper |
| `playarena_db_pool_empty_acquire_total` | Counter | — | DBPoolScraper |
| `playarena_auth_login_attempts_total` | Counter | result | auth.Handler |
| `playarena_auth_token_refresh_total` | Counter | result | auth.Handler |
| `playarena_auth_token_replay_detections_total` | Counter | — | auth.Repository |
| `playarena_auth_password_reset_requests_total` | Counter | — | auth.Handler |
| `playarena_notification_outbox_pending_rows` | Gauge | — | OutboxScraper |
| `playarena_notification_drain_duration_seconds` | Histogram | — | notifications.Service.WithMetrics |
| `playarena_notification_drain_total` | Counter | result | notifications.Service.WithMetrics |
| `playarena_email_worker_tick_total` | Counter | result | notifworker.EmailWorker |
| `playarena_email_worker_deliveries_total` | Counter | status | notifworker.EmailWorker |
| `playarena_email_worker_batch_size` | Histogram | — | notifworker.EmailWorker |
| `playarena_email_dead_letters_total` | Gauge | — | OutboxScraper |
| `playarena_webhook_worker_tick_total` | Counter | result | webhookworker.WebhookWorker |
| `playarena_webhook_worker_deliveries_total` | Counter | status | webhookworker.WebhookWorker |
| `playarena_webhook_worker_batch_size` | Histogram | — | webhookworker.WebhookWorker |
| `playarena_webhook_dead_letters_total` | Gauge | — | OutboxScraper |
| `playarena_realtime_active_subscribers` | Gauge | — | realtime.Hub.WithMetrics |
| `playarena_realtime_subscribe/unsubscribe/publish/dropped_total` | Counter | — | realtime.Hub.WithMetrics |

---

### Phase 22 — Rankings Module (Complete)

**Status: COMPLETE. All 10 integration tests passing. `go build ./...` + `go vet ./...` clean.**

Phase 22 implemented the full global rankings system: snapshot-on-completion that writes per-tournament stats when a tournament is marked completed, and two paginated `GET` endpoints that aggregate those stats into ranked leaderboards.

#### Architecture Decisions

- **Per-tournament stats rows, ranked at read time** — `player_tournament_stats` / `team_tournament_stats` store one final-standings row per (participant, tournament). Rankings aggregate via SQL `GROUP BY + RANK()` window function at query time. This makes the upsert idempotent-by-construction and enables future date-range filtering without any schema changes.
- **Snapshot triggered by HTTP PATCH** — `tournaments.Service.Update` calls `snapshotTournamentStats` after `UpdateWithAudit` succeeds when `status == completed`. Errors are logged but do not fail the PATCH response (snapshot is best-effort, non-blocking for the tournament status update).
- **Nullable `rankingsRepo` on `tournaments.Service`** — Tests that don't exercise ranking pass `nil` for the `rankings.Repository` parameter; `snapshotTournamentStats` is skipped when the repo is nil. Only the 3 integration test server files that use tournaments needed updating to pass `nil`.
- **win_rate computed in Go** — Intentionally excluded from the SQL ORDER BY to avoid float precision in PostgreSQL sort keys. The Go service computes `(total_wins / total_matches)` after fetching the aggregate row.
- **Five-level tiebreak** — `tournaments_won DESC → podium_finishes DESC → total_points DESC → total_wins DESC → total_matches DESC`. SQL `RANK()` handles equal-rank ties; `win_rate` is appended in the response but is not a sort key.
- **sqlc CASE-expression column naming regression** — sqlc regeneration during Phase 22 renamed CASE-expression parameters: `EndedAt → Column13` in `UpdateMatchParams`, `ApprovedAt → Column7` in `UpdateRegistrationParams`. Cannot use `sqlc.arg()` inside CASE expressions (sqlc type inference fails). Consuming service files (`matches/service.go`, `tournament_registrations/service.go`) were updated to use the generated column names.

#### Deliverables

| Component | Description |
|-----------|-------------|
| Migration 000027 | `player_tournament_stats` + `team_tournament_stats` tables; UNIQUE constraints; covering indexes |
| `db/queries/rankings.sql` | `ListPlayerRankings` (CTE + GROUP BY + RANK() + pagination); `ListTeamRankings`; `SnapshotPlayerStats` / `SnapshotTeamStats` (INSERT ON CONFLICT DO UPDATE) |
| `db/sqlc/rankings.sql.go` | Generated — all query functions |
| `internal/rankings/` | Full module: errors, dto, model, models, repository, service, handler, routes |
| `internal/rankings/repository.go` | `ListPlayerRankings`, `ListTeamRankings` (aggregate queries with RANK() window); `SnapshotPlayerStats`, `SnapshotTeamStats` (upsert) |
| `internal/rankings/service.go` | Validates org by slug; calls repository; computes `win_rate` in Go; returns ranked response |
| `internal/rankings/handler.go` | `ListPlayerRankings`, `ListTeamRankings` HTTP handlers; `parseListParams` (limit/offset from query string) |
| `internal/rankings/routes.go` | `RegisterRoutes()` — `GET /api/v1/organizations/{slug}/rankings/players|teams`; `RequireAuth` |
| `internal/tournaments/service.go` | Added `rankingsRepo *rankings.Repository` field; `snapshotTournamentStats` method called in `Update` on completion |
| `internal/tournaments/routes.go` | Updated to accept and forward `*rankings.Repository` parameter |
| `internal/bootstrap/modules.go` | Constructs `rankingsRepo`; passes to `tournaments.RegisterRoutes`; calls `rankings.RegisterRoutes` |
| `internal/rankings/integration/testmain_test.go` | Standard testcontainers TestMain |
| `internal/rankings/integration/server_test.go` | `buildTestServer` wires real `rankingsRepo` into tournaments for E2E snapshot testing; response structs; HTTP helpers |
| `internal/rankings/integration/rankings_test.go` | 10 integration tests: unauthenticated (401), org not found (404), empty lists (200), direct DB insert stats, ranking order, field correctness, pagination, E2E snapshot-on-completion |

#### Integration Tests (10)

| Test | Coverage |
|------|---------|
| `TestRankings_Players_Unauthenticated` | 401 without token |
| `TestRankings_Teams_Unauthenticated` | 401 without token |
| `TestRankings_Players_OrgNotFound` | 404 for non-existent org slug |
| `TestRankings_Teams_OrgNotFound` | 404 for non-existent org slug |
| `TestRankings_Players_Empty` | 200 with `total=0` and empty `rankings` array |
| `TestRankings_Teams_Empty` | 200 with `total=0` and empty `rankings` array |
| `TestRankings_Teams_WithStats` | Ranking order, TournamentsWon field, rank assignment via direct DB insert |
| `TestRankings_Players_WithStats` | Player ranking list, win_rate > 0, rank=1 correctness |
| `TestRankings_Teams_Pagination` | `limit=2` returns 2 rows, `total=3` |
| `TestRankings_Snapshot_OnTournamentCompletion` | E2E: PATCH tournament to completed → snapshot triggered → GET rankings returns 2 teams with correct TournamentsWon=1 |

---

## 10. Frontend Application

**Status: FE-1/FE-2/FE-3/FE-4/FE-5 complete. FE-5 CLOSED. FE-6 is next.**
**Last validated:** 2026-06-11
**Typecheck:** `tsc --noEmit` — 0 errors
**Lint:** `eslint .` — 0 errors, 0 warnings
**Tests:** 108/108 passing (`vitest run`) — 15 test files
**Build:** `next build` — clean, 20 routes (12 dynamic ƒ, 8 static ○)

---

### Tech Stack

| Technology | Version | Role |
|---|---|---|
| Next.js | 16 | App Router framework |
| React | 19 | UI library (concurrent features) |
| TypeScript | 5 | Static typing throughout |
| Tailwind CSS | 4 | CSS-first utility styling (`@theme inline`) |
| shadcn/ui | radix-nova | Component library (unified `radix-ui` package) |
| Zustand | 5 | Client state: auth session, UI state |
| TanStack Query | 5 | Server state, caching, invalidation |
| TanStack Table | 8 | Headless data table with server-side pagination |
| React Hook Form | 7 | Form state management |
| Zod | 4 | Schema validation |
| Axios | 1 | HTTP client with interceptors |
| Vitest | 3 | Test runner |
| Testing Library | 16 | Component and hook testing |

---

### Completed Frontend Phases

#### FE-1 — Architecture Foundation

| File | Purpose |
|---|---|
| `frontend/src/lib/api/client.ts` | Axios singleton; Bearer-token request interceptor; 401 response interceptor with `refreshPromise` deduplication; `tokenManager` (access: sessionStorage, refresh: localStorage); exported `attemptTokenRefresh` |
| `frontend/src/lib/api/query-client.ts` | TanStack Query client factory; `getQueryClient()` browser singleton (server always creates new); `staleTime: 30s`, `gcTime: 5min`, `retry: 1` |
| `frontend/src/lib/api/auth.ts` | Auth API layer: login, register, logout, me, verifyEmail, forgotPassword, resetPassword, resendVerification |
| `frontend/src/lib/api/organizations.ts` | Organizations API: list (used for orgSlug resolution after login), getBySlug, create, update, delete |
| `frontend/src/lib/query-keys.ts` | Org-scoped query key factories for all 11 domain resources; `orgKeys` and `userKeys` not org-scoped (intentional) |
| `frontend/src/stores/auth.store.ts` | Zustand auth store: claims, orgSlug, pendingOrgSelection, isHydrating; setSession, hydrateClaims, setOrgSlug, clearSession, setPendingOrgSelection |
| `frontend/src/stores/ui.store.ts` | Zustand UI store: sidebarOpen, commandOpen |
| `frontend/src/types/api/` | TypeScript DTOs for all 15 backend resource types; audited against live backend DTOs |
| `frontend/src/types/common.ts` | Shared Role enum, PaginatedResponse |
| `frontend/src/lib/utils.ts` | `cn()` — Tailwind class merge utility |
| `frontend/src/lib/api-error.ts` | `extractApiError()`, `isOrgRequiredError()` for HTTP 409 multi-org detection |

#### FE-2 — Design System

| File | Purpose |
|---|---|
| `frontend/src/app/globals.css` | oklch design tokens; `@theme inline {}` Tailwind 4 mapping; full dark mode; status color tokens; sidebar tokens |
| `frontend/src/components/ui/status-badge.tsx` | Semantic domain status badges: MatchStatus (5), TournamentStatus (6), RegistrationStatus (5), PlayerStatus (2), TeamStatus (2); every variant renders a visible text label — no color-only state |
| `frontend/src/components/ui/data-table.tsx` | TanStack Table v8 wrapper; server-side and client-side pagination/sort; SkeletonRows while loading |
| `frontend/src/components/ui/form-field.tsx` | RHF Controller wrappers: FormField, FormTextarea, FormSelect, FormDatePicker; FieldWrapper with aria-invalid/aria-describedby |
| `frontend/src/components/ui/page-header.tsx` | Title + shadcn Breadcrumb + optional action slot |
| `frontend/src/components/ui/empty-state.tsx` | Dashed-border placeholder with icon, title, description, action |
| `frontend/src/components/ui/confirm-dialog.tsx` | Controlled dialog; `destructive` prop for red confirm button |
| `frontend/src/components/ui/loading-skeleton.tsx` | TableSkeleton, CardSkeleton, StatSkeleton, PageSkeleton |
| `frontend/src/components/ui/button.tsx` | shadcn Button (CVA variants) |
| `frontend/src/components/ui/input.tsx` | shadcn Input |
| `frontend/src/components/ui/label.tsx` | shadcn Label |
| `frontend/src/components/ui/select.tsx` | shadcn Select |
| `frontend/src/components/ui/dialog.tsx` | shadcn Dialog (radix, focus-trapped) |
| `frontend/src/components/ui/dropdown-menu.tsx` | shadcn DropdownMenu |
| `frontend/src/components/ui/breadcrumb.tsx` | shadcn Breadcrumb |
| `frontend/src/components/ui/table.tsx` | shadcn Table |
| `frontend/src/components/ui/badge.tsx` | Generic badge primitive |
| `frontend/src/components/ui/separator.tsx` | shadcn Separator |
| `frontend/src/components/ui/skeleton.tsx` | shadcn Skeleton |

#### FE-3 — Core App Infrastructure

| File | Purpose |
|---|---|
| `frontend/src/app/layout.tsx` | Root layout: QueryClientProvider, ThemeProvider, Toaster |
| `frontend/src/app/page.tsx` | Root redirect: runs useAuthGuard to restore session from stored tokens; redirects to /{orgSlug} or /login; shows PageSkeleton while hydrating |
| `frontend/src/app/(auth)/layout.tsx` | Centered auth card layout |
| `frontend/src/app/(auth)/login/page.tsx` | Login form (RHF + Zod); multi-org 409 flow with sessionStorage credential hand-off; redirects to /{orgSlug} |
| `frontend/src/app/(auth)/register/page.tsx` | Registration form |
| `frontend/src/app/(auth)/verify-email/page.tsx` | Token-based email verification; auto-verifies on mount when `?token=` present; resend flow |
| `frontend/src/app/(auth)/forgot-password/page.tsx` | Forgot password form |
| `frontend/src/app/(auth)/reset-password/page.tsx` | Reset password form |
| `frontend/src/app/(auth)/org-select/page.tsx` | Multi-org picker: reads pending orgs from store; re-calls login with organization_id; guard checks orgSlug before redirecting to prevent race with router.push |
| `frontend/src/app/(app)/layout.tsx` | App shell: runs useAuthGuard; shows PageSkeleton while hydrating |
| `frontend/src/app/(app)/[orgSlug]/layout.tsx` | Org layout: sidebar + header + mobile backdrop overlay; SSE stream mount; closes sidebar on mobile init; `lg:ml-60` content offset; `useSyncExternalStore` matchMedia for desktop detection; `inert` on main content when mobile drawer open; Escape key closes; focus moves to first sidebar element on open |
| `frontend/src/hooks/use-auth-guard.ts` | Hydration guard: refresh token check → me() silent refresh → hydrateClaims() → orgSlug resolution → setHydrating(false); one-shot on mount |
| `frontend/src/hooks/use-notification-stream.ts` | SSE hook: connects with `?token=` query param; exponential backoff on onerror; on auth_error calls `attemptTokenRefresh()` explicitly before reconnecting to prevent infinite loop; `connectRef` pattern avoids stale closures; `getQueryClient().clear()` on forced logout from SSE |
| `frontend/src/components/layout/org-sidebar.tsx` | Fixed sidebar: memoized nav (`useMemo`); `aria-current="page"` on active link; closes drawer on mobile nav-click; `data-sidebar="true"` + `tabIndex={-1}` for focus management; `role="dialog"` + `aria-modal="true"` when acting as mobile overlay |
| `frontend/src/components/layout/org-header.tsx` | Sticky header: sidebar toggle; theme toggle; logout (`getQueryClient().clear()` → `clearSession()` → `/login`) |

---

#### FE-4 — Core App Pages

| File | Purpose |
|---|---|
| `frontend/src/app/(app)/[orgSlug]/page.tsx` | Dashboard: welcome banner with role badge; role-filtered quick actions (`hasPermission`); notification/tournament/match widgets with error retry; unread count badge via `useUnreadCount`; `Button asChild` pattern for link-button; `focus-visible:ring` on quick action cards |
| `frontend/src/app/(app)/[orgSlug]/notifications/page.tsx` | Notification center: `useInfiniteQuery` with Load More pagination; optimistic mark-read, mark-all-read, delete (with rollback); `role="feed"` list; Retry button on error |
| `frontend/src/app/(app)/[orgSlug]/settings/profile/page.tsx` | Profile settings: RHF + Zod form; dirty-state tracking; cancel reverts to server values; read-only email uses `<p>` not `<label>`; `useCurrentUser` for greeting |
| `frontend/src/app/(app)/[orgSlug]/settings/security/page.tsx` | Password change: show/hide toggles; password strength meter with 4 rules; `aria-live` + `aria-label` on strength; field-level error for wrong current password; form reset on success |
| `frontend/src/app/(app)/[orgSlug]/settings/notifications/page.tsx` | Notification preferences: opt-out model (default enabled); optimistic toggle with rollback; webhook channel intentionally excluded; Retry button on load error |
| `frontend/src/components/layout/org-switcher.tsx` | Org switcher: single-org static display; multi-org dropdown; org switch = logout → `queryClient.clear()` → `/login` (no dead-code query params) |
| `frontend/src/components/notifications/notification-item.tsx` | Notification item: `role="article"`; action buttons visible on hover, focus-within, AND touch devices (`[@media(hover:none)]:opacity-100`); `aria-label` on all actions |
| `frontend/src/hooks/use-unread-count.ts` | Unread badge hook: `notificationKeys.list(orgSlug, { limit: 50, offset: 0 })`; single source of truth for all badge displays |
| `frontend/src/hooks/use-current-user.ts` | Current user hook: `userKeys.detail(userId)`; enabled only when userId is known |
| `frontend/src/lib/permissions.ts` | Shared RBAC helpers: `hasPermission(role, perm)`, `ROLE_LABELS`, `ROLE_VARIANTS`; extracted from dashboard for reuse |

---

### Authentication Architecture

| Aspect | Detail |
|---|---|
| Access token storage | sessionStorage (tab-scoped, cleared on tab close) |
| Refresh token storage | localStorage (persists across page loads) |
| JWT decoding | Inline base64url decode; `claims.exp * 1000 - Date.now() > 60_000` for expiry check |
| Refresh deduplication | Module-level `refreshPromise` in client.ts — concurrent 401s await the same in-flight refresh |
| Multi-org 409 flow | Login → 409 → store pending orgs + sessionStorage credentials → org-select → re-login with organization_id → clear credentials immediately |
| Silent session restore | useAuthGuard decodes token → calls me() → 401 interceptor refreshes → hydrateClaims() writes decoded claims to store |
| Platform admin orgId | `null` (not empty string) — `decoded.organization_id \|\| null` normalises both absent field and empty string |
| SSE auth_error | `attemptTokenRefresh()` called explicitly before reconnect; `getQueryClient().clear()` + `clearSession()` + redirect to /login if refresh fails |
| Org switch | logout → `getQueryClient().clear()` → `clearSession()` → `/login`; no dead-code `?email`/`?next` params |

### Query Architecture

| Aspect | Detail |
|---|---|
| Cache keys | All org-scoped; `orgKeys` and `userKeys` intentionally not org-scoped |
| SSE invalidation | `useNotificationStream` invalidates `notificationKeys.all(orgSlug)` on every message (covers all notification query variants); per-event-type invalidation of match/tournament/registration keys |
| Cache on logout | `getQueryClient().clear()` called before `clearSession()` — stale data cannot survive the session boundary |
| Cache on SSE forced logout | `getQueryClient().clear()` called in `auth_error` handler when refresh fails — consistent with explicit logout |
| Unread count | `useUnreadCount(orgSlug)` is the single source of truth for all unread badge displays (header bell, sidebar badge, dashboard, notification center) |
| Infinite query | Notification center uses `useInfiniteQuery` with offset-based `getNextPageParam`; optimistic mutations map over `InfiniteData.pages` |

---

### Testing

| Test file | Tests | What it regresses |
|---|---|---|
| `src/hooks/__tests__/use-auth-guard.test.ts` | 5 | Claims populated after me(); orgSlug resolved; redirect on no refresh token; redirect on me() failure; no duplicate me() when claims valid |
| `src/app/(auth)/org-select/__tests__/org-select.test.tsx` | 5 | Org list rendered; redirect to /login on empty+no-orgSlug; successful selection → /{orgSlug} not /login; credential cleanup; no /login redirect when orgSlug set |
| `src/hooks/__tests__/use-notification-stream.test.ts` | 7 | SSE key invalidates `notificationKeys.all`; EventSource URL has token; attemptTokenRefresh on auth_error; reconnect with new token; redirect + no reconnect on refresh failure; **query cache cleared on forced logout**; no infinite auth_error loop |
| `src/components/layout/__tests__/org-header.test.tsx` | 3 | Query cache cleared on logout; cache cleared before session cleared; redirect to /login after logout |
| `src/components/layout/__tests__/org-switcher.test.tsx` | 4 | Single-org static display; dropdown trigger for multi-org; other orgs listed; **logout + cache clear + redirect to /login (no dead query params)** |
| `src/app/(app)/[orgSlug]/__tests__/dashboard.test.tsx` | 7 | Welcome/role badge; quick actions by role; widget headings; empty states; unread count badge (correct `{ limit: 50 }` key); **widget error states show retry button** |
| `src/app/(app)/[orgSlug]/notifications/__tests__/notification-center.test.tsx` | 8 | Loading skeleton; empty state; list renders; mark-all-read optimistic update; SSE cache update (InfiniteData shape); Load More button |
| `src/app/(app)/[orgSlug]/settings/__tests__/notifications.test.tsx` | 6 | Skeleton; event groups; column headers; preference state in toggles; opt-out default; updatePreference API call |
| `src/app/(app)/[orgSlug]/settings/__tests__/profile.test.tsx` | 7 | Loading skeleton; form fields; save button disabled when clean; save enables when dirty; unsaved indicator; cancel button; form resets on cancel; validation |
| `src/app/(app)/[orgSlug]/settings/__tests__/security.test.tsx` | 7 | Renders all fields; submit button; strength meter; "Strong" label; correct API args; field error on wrong password; form reset after success |
| `src/components/notifications/__tests__/notification-item.test.tsx` | 7 | Event label; mark-as-read only for unread; delete always present; **`[@media(hover:none)]:opacity-100` class present for touch accessibility**; callbacks fire with correct id |

**Total: 67 tests across 11 files** (FE-1 through FE-4 scope).

Test infrastructure: Vitest 3, `@testing-library/react` 16, jsdom, `@testing-library/jest-dom`; `vitest.config.ts` at `frontend/`; `src/test/setup.ts` + `src/test/test-utils.tsx`.

---

#### FE-5 — Players & Teams

**Status: CLOSED.** Final adversarial review + remediation pass complete (2026-06-11).

| File | Purpose |
|---|---|
| `frontend/src/app/(app)/[orgSlug]/players/page.tsx` | Player directory: DataTable with server-side pagination/sort/filter; URL-driven state; debounced search; `localSearch` drives `hasFilters` (P1-1 fix); viewer-gated create |
| `frontend/src/app/(app)/[orgSlug]/players/[playerId]/page.tsx` | Player profile: avatar, status badge, detail rows, delete confirmation; `avatar_url` from `GetByID` media query |
| `frontend/src/app/(app)/[orgSlug]/players/[playerId]/edit/page.tsx` | Player edit form (RHF + Zod); dirty-state guard; cancel reverts |
| `frontend/src/app/(app)/[orgSlug]/players/new/page.tsx` | Player create form; RBAC-gated |
| `frontend/src/app/(app)/[orgSlug]/teams/page.tsx` | Team directory: same URL-driven pattern as players; `LogoDisplay` in table rows |
| `frontend/src/app/(app)/[orgSlug]/teams/[teamId]/page.tsx` | Team profile: `TeamLogo` (upload-capable), detail grid, disband `ConfirmDialog`, `MembersSection` |
| `frontend/src/app/(app)/[orgSlug]/teams/[teamId]/edit/page.tsx` | Team edit form; color pickers |
| `frontend/src/app/(app)/[orgSlug]/teams/new/page.tsx` | Team create form |
| `frontend/src/components/players/player-avatar.tsx` | `PlayerAvatar` (upload-capable) + `AvatarDisplay` (read-only) |
| `frontend/src/components/teams/team-logo.tsx` | `TeamLogo` (upload-capable, P1-B fixed: `primaryColor` applied to single div) + `LogoDisplay` (read-only) |
| `frontend/src/components/teams/members-section.tsx` | Active roster with member count, remove confirmation; passes `existingMemberPlayerIds` to duplicate guard |
| `frontend/src/components/teams/add-member-dialog.tsx` | Player picker; `existingMemberPlayerIds` disables already-added players with "On team" badge (P1-2 fix) |
| `frontend/src/hooks/use-players.ts` | `usePlayerList`, `usePlayer`, `useCreatePlayer`, `useUpdatePlayer`, `useDeletePlayer` |
| `frontend/src/hooks/use-teams.ts` | `useTeamList`, `useTeam`, `useCreateTeam`, `useUpdateTeam`, `useDeleteTeam` |
| `frontend/src/hooks/use-team-members.ts` | `useTeamMembers`, `useAddTeamMember`, `useRemoveTeamMember` |
| `frontend/src/hooks/use-media-upload.ts` | `useMediaUpload`: multipart POST to `/media`; progress tracking; invalidates `mediaKeys.list` on success |
| `frontend/src/lib/api/players.ts` | Players API layer |
| `frontend/src/lib/api/teams.ts` | Teams API layer; `TeamMemberListResponse` uses `members[]` (no total/limit/offset) |
| `frontend/src/lib/api/media.ts` | Media API layer; no manual `Content-Type` header (P1-3 fix) |
| `frontend/src/types/api/players.ts` | `Player` DTO including `avatar_url: string \| null` |
| `frontend/src/types/api/teams.ts` | `Team`, `TeamMember` DTOs; `player_display_name: string` on `TeamMember` |
| `frontend/src/components/ui/media-upload.tsx` | Drag-and-drop / click file picker; progress overlay; size validation |

**Architecture decisions:**
- `avatar_url` is present on `Player` DTO and populated by `GetByID` only. The list endpoint intentionally returns null (no N+1 per-row media query). List view always renders initials at thumbnail size.
- Backend `ListActiveMembersWithNames` JOIN query returns `player_display_name` with each membership — no separate player lookup on the frontend.

**FE-5 tests added (41 tests across 4 new files):**

| Test file | Tests | What it regresses |
|---|---|---|
| `src/app/(app)/[orgSlug]/players/__tests__/players.test.tsx` | 14 | P0-2: avatar renders; P1-1: localSearch drives Clear; P1-A: list shows initials only; C-1: upload invalidates player detail key; permission gates; cache seeding |
| `src/app/(app)/[orgSlug]/teams/__tests__/teams.test.tsx` | 10 | Directory listing; P1-4 clear button; D-1: viewer cannot create teams |
| `src/app/(app)/[orgSlug]/teams/__tests__/members.test.tsx` | 11 | P0-1: display names not UUIDs; P1-2: duplicate guard; C-3: error state (toast.error + dialog stays open); add/remove flows; permission gates |
| `src/app/(app)/[orgSlug]/teams/__tests__/team-detail.test.tsx` | 9 | C-2: disband confirm + cancel; D-2: TeamLogo primaryColor applied to single div |

**Total after FE-5: 108 tests across 15 files.**

---

## 11. Next Recommended Phase

**Phases 1 – 22 are complete.** All four notification delivery channels (in_app, email, webhook, SSE), the full observability stack, and the rankings module are implemented and production-hardened.

**Phases 23A–D are complete.** All four backend blockers that prevented frontend development are resolved.

**Frontend phases FE-1 through FE-5 are complete.** Players & Teams pages, roster management, and media upload are implemented, reviewed, and passing 108 tests.

**Next frontend phase: FE-6 — Tournaments & Matches**

Scope:
- Tournament list, tournament detail, tournament create/edit/delete (RBAC-gated)
- Match list, match detail, match scoring interface
- Tournament registration management
- Match event log (scorer view)
- Cross-linking player tournament history on player profile (placeholder exists at `PlayerProfilePage` "Teams" card)

Backend APIs available: `/organizations/{slug}/tournaments`, `/organizations/{slug}/matches`, `/organizations/{slug}/matches/{id}/events`, `/organizations/{slug}/tournament-registrations`.

**Candidate backend scope (no recommendation):**

1. **News module** (`internal/news/`) — stub exists with only a package declaration; no business logic.
2. **Redis pub/sub for multi-instance SSE** — Phase 20 Hub is in-process (single binary). Horizontal scaling requires a Redis pub/sub bridge so events published by one instance reach SSE clients on another.
3. **Integration test coverage gaps** — Organizations, players, teams, matches, match_events, media, users, and tournament_registrations integration packages exist but test coverage depth varies. The new `internal/members/` module has no integration tests yet.

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
| `backend/internal/standings/tiebreakers.go` | Full 7-level tiebreak comparator; N-way h2h sub-table via `buildH2HRanks()`; cyclic ties fall through to score difference |
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
| `backend/internal/auth/repository.go` | `RotateRefreshToken` (replay state machine + user status re-check); `LogoutTransaction` (FOR UPDATE); `ForgotPasswordTransaction`; `ResetPasswordTransaction` (deterministic lock-order deadlock fix, Phase 13B.1); `equalizeEnumerationTiming` (4-RT, Phase 13B.1); `equalizeResendVerificationTiming` (5-RT, Phase 15A Remediation) |
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
| `backend/internal/platform/middleware/ratelimit.go` | `IPRateLimiter` — per-IP token bucket; `sync.Map` + `atomic.Int64 lastSeen`; O(1) lock-free reads; O(active IPs) cleanup via `sync.Map.Range`; `Retry-After: 1` on 429; Stop() via done channel |
| `backend/internal/platform/middleware/realip.go` | `TrustedRealIP(trustedCIDRs)` — rewrites RemoteAddr only for connections from trusted CIDR ranges; prevents X-Forwarded-For spoofing from untrusted clients; falls back to `chimw.RealIP` when unconfigured |
| `backend/internal/platform/middleware/cors.go` | `CORS()` middleware — origin reflection; `Allow-Credentials` (specific origins only); `Vary: Origin`; preflight 204 |
| `backend/internal/cleanup/scheduler.go` | `Scheduler` — background token expiry + 90-day outbox retention cleanup; configurable interval; graceful shutdown via done channel |
| `backend/internal/bootstrap/app.go` | `App.Handler()` initialises rate limiter + cleanup scheduler + EmailWorker; `App.Shutdown()` calls `DrainEmail` + `EmailWorker.Stop`/`Drain` then stops all background services |
| `backend/db/queries/notifications.sql` | 19 notification SQL queries (16 original + 3 email worker queries); includes `GetNotificationPreferencesForEvent` for O(event_types) batch preference loading |
| `backend/internal/notifications/repository.go` | `DrainOutbox` (FOR UPDATE SKIP LOCKED, batch prefs, UNIQUE conflict handling, multi-channel fan-out: in_app + email); `UpsertPreference` (dynamic audit action: AuditActionCreate vs AuditActionUpdate) |
| `backend/internal/notifications/service.go` | `DrainOutbox` entry point — errors swallowed to preserve domain operation result |
| `backend/internal/notifications/trigger/trigger.go` | `WriteOutboxEntry` — must be called with a transaction-scoped `*db.Queries` handle |
| `backend/internal/notifworker/worker.go` | `EmailWorker` — embedded async email delivery; Start/Stop/Drain lifecycle; soft-lease claim; retry (1m→5m); dead-letter after 3 attempts |
| `backend/internal/notifworker/repository.go` | `ClaimBatch`, `RecordSuccess`, `RecordFailure`, `GetUserByID`, `GetOrgSlugByID` |
| `backend/db/migrations/000025_notification_email_delivery.up.sql` | 4 delivery-state columns + partial index on `notifications` for email worker claim query |
| `backend/internal/email/email.go` | `Provider` interface; `NoOpProvider` (in-memory capture + `Sent`/`SentTo`/`Count`/`Reset` helpers); `NewSender` factory; `NewSenderWithProvider` (test injection) |
| `backend/internal/email/sender.go` | `Sender` + `SenderConfig`; `SendVerificationEmail`, `SendPasswordResetEmail`, `SendResendVerificationEmail`, `SendNotificationEmail` — renders templates then calls `Provider.Send` |
| `backend/internal/email/ses.go` | `sesProvider` — AWS SES v2; `awsconfig.LoadDefaultConfig` with optional static credential override; optional HTML body |
| `backend/internal/email/smtp.go` | `smtpProvider` — `net/smtp`; `useTLS=false` → STARTTLS; `useTLS=true` → implicit TLS (`tls.Dial`); context-aware via goroutine+channel select; `buildMessage` emits `multipart/alternative` when both bodies are present |
| `backend/internal/email/templates.go` | `//go:embed templates` FS; `html/template` for HTML (auto-escaping); `text/template` for plain text |
| `backend/internal/auth/integration/email_delivery_test.go` | 8 integration tests covering email delivery on register, body size 413, and resend-verification |
| `backend/internal/auth/integration/remediation_test.go` | 5 regression tests: `TestResendVerification_NilSender_ServiceStillCalled` (P1-3), `TestBodySizeLimit_Auth_Regression` (P0-1), `TestRateLimit_RetryAfterHeader` (P2-1), `TestHandler_DrainEmail_CompletesWithinTimeout` (P1-4), `TestRegister_DrainEmailAfterDelivery` (P1-4) |
| `backend/internal/platform/middleware/bodysize.go` | `BodySizeLimit(maxBytes)` — `http.MaxBytesReader` wrapper; downstream `*http.MaxBytesError` → `ErrBodyTooLarge` → 413 |
| `backend/internal/platform/validator/validator.go` | `ErrBodyTooLarge` sentinel (Phase 15A); `DecodeJSON` returns it on `*http.MaxBytesError` |

---

| `backend/db/migrations/000026_webhook_notifications.up.sql` | `webhook_endpoints` + `webhook_deliveries` tables; RBAC seeding for webhook.create/read/update/delete |
| `backend/db/queries/webhooks.sql` | 12 webhook SQL queries including `ClaimWebhookDeliveriesForDelivery` (FOR UPDATE SKIP LOCKED) and `CreateWebhookDelivery` (ON CONFLICT DO NOTHING) |
| `backend/internal/webhooks/ssrf.go` | `ValidateURL` (registration-time SSRF guard); `SSRFSafeTransport` (delivery-time DNS validation + dial-by-resolved-IP; defeats DNS rebinding) |
| `backend/internal/webhooks/crypto.go` | `GenerateSecret` (32-byte CSPRNG, base64url); `EncryptSecret` / `DecryptSecret` (AES-256-GCM, random nonce per encryption) |
| `backend/internal/webhooks/service.go` | `NewService` (decodes 32-byte AES key); `Create` (ValidateURL → GenerateSecret → EncryptSecret); `toResponse` (never exposes ciphertext) |
| `backend/internal/webhookworker/worker.go` | `WebhookWorker.deliver` — decrypt secret → build envelope → single `time.Now()` for both timestamp fields → HMAC-SHA256 canonical string → POST → record |
| `backend/internal/webhookworker/integration/worker_test.go` | 10 integration tests; `TestWebhookWorker_SignatureVerification` verifies HMAC correctness and timestamp consistency end-to-end |
| `backend/internal/realtime/hub.go` | `Hub` — in-process pub/sub; `subKey{orgID,userID [16]byte}` composite map key; Subscribe/Unsubscribe/Publish/Shutdown/Done; RLock held through entire Publish fan-out (data-race + send-on-closed fix) |
| `backend/internal/notifications/handler.go` — `Stream` | SSE handler: `?token=`/Bearer JWT auth; org-scoped tenant check (platform-admin → 403); 25s keepalive ticker; 4-case select (`ch`, `ticker`, `ctx.Done`, `hub.Done`) |
| `backend/internal/notifications/routes.go` | `/stream` outside `RequireAuth` group; registered before `/{id}` routes to prevent routing collision |
| `backend/internal/notifications/integration/stream_test.go` | 17 SSE stream integration tests including MT-1 (platform-admin 403 gate), MT-2 (full-frame drain), MT-3 (subscribe-after-shutdown) |

---

| `backend/internal/platform/metrics/metrics.go` | `Registry` — 30+ Prometheus metrics with `playarena_` prefix; HTTP, rate-limit, DB pool, auth, notifications, email worker, webhook worker, realtime hub; `New()` registers against a fresh non-default registry |
| `backend/internal/platform/middleware/metrics.go` | `Metrics(reg)` — Prometheus HTTP instrumentation middleware; records requests total, request duration, in-flight gauge; uses chi route pattern as label |
| `backend/internal/platform/middleware/logging.go` | `RequestLogger(log *slog.Logger)` — per-request structured slog logging (method, path, status, latency, request_id) |
| `backend/internal/bootstrap/observability.go` | `newInternalServer` (:9090); `startDBPoolScraper` (15s, pgxpool.Stat()); `startOutboxMetricsScraper` (30s, CountPendingOutboxRows + CountEmailDeadLetters + CountDeadLetters) |
| `backend/internal/health/handler.go` | `Handler.Check` (GET /api/v1/health); `Handler.Ready` (K8s readiness — 200/503 based on DB ping); `Handler.Live` (K8s liveness — always 200) |
| `backend/deploy/prometheus/prometheus.yml` | Scrapes `api:9090/metrics` every 15s |
| `backend/deploy/prometheus/alerts.yaml` | 12 alert rules across critical + warning groups: DatabaseUnavailable, HighErrorRate5xx, EmailWorkerStuck, WebhookWorkerStuck, EmailDeadLetterBurst, WebhookDeadLetterBurst, HighP99Latency, DBPoolNearSaturation, AuthLoginAnomalySpike, TokenReplayDetected, OutboxBacklogHigh, RealtimeDroppedEvents |
| `backend/deploy/grafana/dashboards/*.json` | 5 pre-provisioned Grafana dashboards: api-overview, auth, infrastructure, notifications, realtime |
| `.github/workflows/ci.yml` | Go CI: vet → build → unit tests (`-race`) on ubuntu-latest; triggered on push/PR to `main` for `backend/**` |
| `backend/db/migrations/000027_rankings_stats.up.sql` | `player_tournament_stats` + `team_tournament_stats` tables; UNIQUE per-tournament constraints; covering indexes for ranking list queries |
| `backend/db/queries/rankings.sql` | `ListPlayerRankings` / `ListTeamRankings` (CTE + GROUP BY + RANK() window function + LIMIT/OFFSET); `SnapshotPlayerStats` / `SnapshotTeamStats` (INSERT ON CONFLICT DO UPDATE) |
| `backend/internal/rankings/repository.go` | `ListPlayerRankings`, `ListTeamRankings` (aggregate + rank), `SnapshotPlayerStats`, `SnapshotTeamStats` (upsert) |
| `backend/internal/rankings/service.go` | Org validation by slug; delegates to repo; computes `win_rate` in Go after fetch |
| `backend/internal/rankings/handler.go` | HTTP handlers; `parseListParams` (limit/offset from query string) |
| `backend/internal/rankings/routes.go` | `GET /api/v1/organizations/{slug}/rankings/players|teams`; `RequireAuth` |
| `backend/internal/tournaments/service.go` | `snapshotTournamentStats` — derives standings + upserts stats rows when tournament transitions to completed; nil-safe (skipped when `rankingsRepo == nil`) |
| `backend/internal/rankings/integration/rankings_test.go` | 10 integration tests: 401 unauthenticated, 404 org not found, 200 empty, ranking order + fields, pagination, E2E snapshot-on-completion |

---

*This document was last updated on 2026-06-09 (Phase 21 complete — Observability and Operations; Phase 22 complete — Rankings Module with snapshot-on-completion, player/team leaderboards, 10 integration tests). It should be updated whenever a phase is completed or significant architectural changes are made.*
