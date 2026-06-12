# GP-1 Implementation Report

**Status:** Complete, validated, adversarially reviewed. GP-2 NOT started.
**Date:** 2026-06-12.
**Spec:** [gp-1-spec.md](gp-1-spec.md). This report records what was actually built and verified.
**Default runtime behavior is unchanged** — every new surface is gated by `GP_PLAYER_PERSONA_ENABLED` (default `false`).

---

## 1. Architecture compliance report

| Approved decision | Implementation | Status |
|---|---|---|
| Kabaddi-only; no sport dimension | No sport columns/keys added anywhere. | ✅ |
| One User → one PlayerProfile | Partial unique index `uq_players_user_id` on `(user_id) WHERE user_id IS NOT NULL AND archived_at IS NULL`; pre-check + unique-index backstop in `CreateOwn`. | ✅ |
| PlayerProfile is global, user-owned | `players.organization_id` made nullable; `CreateGlobalPlayerProfile` inserts `organization_id = NULL`; ownership = `user_id == actor`. | ✅ |
| Closed ecosystem; no imported players / no claim workflows / no seeded reputation | No claim/merge endpoints. Legacy dupes archived (`archived_at`), never merged. **No rating/reputation field exposed** anywhere. | ✅ |
| Evolve `players` in place; preserve id + FKs | Columns added in place; `players.id` and all FKs untouched (zero FK rewrites). | ✅ |
| Auth scopes player/organizer/onboarding/platform | Explicit `scope` JWT claim + `DeriveScope` legacy inference; `IsPlatformUser()` ⇒ `scope=="platform"`. | ✅ |
| `RequireOrgScope` permits organizer+platform, rejects player+onboarding | Rewritten accordingly; all 11 org trees unchanged. | ✅ |
| Self-profile routes use ownership, not scope checks | `/me/player` gated by `RequireAuth` + service `user_id == actor`; `RequirePlayerScope` reserved for later. | ✅ |
| Persona onboarding foundations (backend only; no player UI) | Login/refresh persona resolution + minimal `/me` route foundation; no player dashboard. | ✅ |
| Do NOT start GP-2 / global rankings / recruitment / etc. | None implemented. | ✅ |

---

## 2. Files changed

**Backend — schema & generated**
- `db/migrations/000028_player_profile_foundation.up.sql` (new)
- `db/migrations/000028_player_profile_foundation.down.sql` (new) — refuses unsafe NOT NULL restore when null-org profiles exist
- `db/reports/000028_dedup_dryrun.sql` (new) — read-only pre-flight report
- `db/queries/players.sql` (4 new queries + `archived_at IS NULL` on list/count/org-list)
- `db/sqlc/players.sql.go`, `db/sqlc/models.go` (regenerated via `sqlc generate`)

**Backend — auth**
- `internal/auth/model.go` — scope consts, `JWTClaims`/`AuthUser` scope+profile fields, `IsPlatformUser`=scope-based, `DeriveScope`, `IsPlatformRoleSlug`
- `internal/auth/tokens.go` — `GenerateAccessToken` takes scope + playerProfileID
- `internal/auth/middleware.go` — populate scope in `RequireAuth`; rewrite `RequireOrgScope`; add `RequireScope`, `RequirePlayerScope`
- `internal/auth/service.go` — `principalContext`, `resolvePrincipal`, `resolvePrincipalForScope`, `resolveExplicitOrg`; login/refresh wiring
- `internal/auth/dto.go` — `Scope` on Login/Refresh responses; `Scope` on `RefreshRequest`
- `internal/auth/handler.go` — `/me` returns scope + player_profile_id; 403 mapping for `ErrScopeNotEntitled`
- `internal/auth/errors.go` — `ErrScopeNotEntitled`
- `internal/auth/repository.go` — `GetPlayerProfileID`

**Backend — players (self-profile)**
- `internal/players/selfprofile.go` (new) — `SelfService` (CreateOwn/GetOwn/UpdateOwn/GetByIDGlobal) + DTOs + visibility helper
- `internal/players/selfprofile_handler.go` (new) — `SelfHandler` + immutable-field rejection
- `internal/players/selfprofile_routes.go` (new) — `RegisterMeRoutes` (flag-gated)
- `internal/players/repository.go` — `GetProfileByUserID/GetProfileByID/CreateGlobalProfile/UpdateOwnProfile`
- `internal/players/dto.go` — `Visibility` on `Response`
- `internal/players/service.go` — `playerToResponse` sets Visibility
- `internal/players/errors.go` — `ErrProfileExists`, `ErrInvalidVisibility`, `ErrImmutableField`

**Backend — config, slugs, wiring**
- `internal/platform/config/config.go` — `PlayerPersonaEnabled` (`GP_PLAYER_PERSONA_ENABLED`)
- `internal/organizations/service.go` — reserved-slug guard in `generateSlug`
- `internal/bootstrap/modules.go` — mount `players.RegisterMeRoutes`

**Backend — tests**
- `internal/auth/model_test.go` (rewritten), `internal/auth/middleware_scope_test.go` (new)
- `internal/platform/config/config_test.go` (+2 tests), `internal/organizations/slug_test.go` (new)
- `internal/players/integration/selfprofile_test.go` (new)

**Frontend**
- `src/types/api/auth.ts` — `Scope` type; scope+profile fields on JwtClaims/AuthUser/TokenResponse/RefreshRequest
- `src/stores/auth.store.ts` — decode scope/player_profile_id; `selectScope`, `selectPlayerProfileId`
- `src/lib/api/client.ts` — send `scope` on silent refresh
- `src/lib/query-keys.ts` — `meKeys`, `playerProfileKeys` (non-org-scoped)
- `src/lib/reserved-slugs.ts` (new); guard in `src/app/(app)/[orgSlug]/layout.tsx`
- `src/app/(player)/layout.tsx` + `src/app/(player)/me/page.tsx` (new foundation)
- `src/stores/__tests__/auth-scope.test.ts` (new); 4 existing JwtClaims mock fixtures updated

---

## 3. Migrations

`000028` order exactly as specified: add columns → relax `organization_id` NOT NULL → (dry-run report) → deduplicate (archive non-canonical, keep earliest by `created_at,id`) → create partial unique index. DOWN drops the additions and restores NOT NULL **only if no null-org profiles exist** (else no-op + notice). Verified end-to-end by the integration suite, which runs all migrations including 000028 against a real Postgres 17 container.

---

## 4. Tests added

- **Unit (no DB):** `IsPlatformUser` (incl. player-token-not-platform), `DeriveScope` (7 cases incl. least-privilege fallback), `IsPlatformRoleSlug`; `RequireOrgScope`/`RequirePlayerScope`/`RequireScope` via real token chains; reserved-slug generation; config flag default/enable; frontend scope decoding + reserved-slug util (6 vitest cases).
- **Integration (Postgres container):** `TestSelfProfile_CreateGetUpdate` (create → 409 dup → player-scope login transition → visibility patch → 422 immutable), `TestSelfProfile_VisibilityAndIsolation` (private hidden as 404, owner/public visible, **player token → 403 on org-admin route**), `TestSelfProfile_RefreshScopeEscalation` (player→platform/organizer = 403; player→player = 200).

---

## 5. Validation results

**Backend**
- `go fmt` — clean on all touched files.
- `go vet ./...` — clean.
- `go build ./...` — clean.
- `go test ./... -p 1` — **all packages PASS** (23 packages incl. every integration suite against Postgres 17). Note: `go test ./...` in parallel flakes on Windows testcontainers (provider init race, unrelated to code); serial `-p 1` is green.

**Frontend**
- `pnpm typecheck` — 0 errors.
- `pnpm lint` — 0 problems.
- `pnpm test` — **178 passed** (25 files), incl. new scope tests.
- `pnpm build` — clean; `/me` route emitted.

---

## 6. Token compatibility matrix

Old Token = pre-GP-1 (no `scope` claim). New Token = GP-1 (has `scope`). Old Binary = pre-GP-1 code (`IsPlatformUser` = empty-org && role≠onboarding). New Binary = GP-1 code (scope-based).

| Persona | Old Token → Old Binary | Old Token → New Binary | New Token → New Binary | New Token → Old Binary |
|---|---|---|---|---|
| **platform** (org="", role=platform_admin) | ✅ safe | ✅ safe — `DeriveScope` recognizes platform_admin → platform | ✅ safe — scope=platform | ✅ safe — old code reads org+role; resolves platform |
| **organizer** (org set, role=org_owner) | ✅ safe | ✅ safe — org set → organizer | ✅ safe — scope=organizer | ✅ safe — old code reads org; RBAC intact |
| **onboarding** (org="", role=onboarding) | ✅ safe — rejected by org guard | ✅ safe — scope=onboarding, org guard rejects | ✅ safe | ✅ safe — old code keys on role=onboarding |
| **player** (org="", role="", scope=player) | N/A — never existed pre-GP-1 | N/A | ✅ safe — scope=player; not platform; org guard rejects; self routes via ownership | ⛔ **UNSAFE / BLOCKED** — old `IsPlatformUser` (empty org && role≠onboarding) would treat a player token as **platform admin** |

**The single unsafe cell** — New player token → Old binary — is the rollback hazard. **Mitigations (enforced):** player tokens are only minted when `GP_PLAYER_PERSONA_ENABLED=true`; the common rollback (flag → false) mints no player tokens and is fully safe; rolling back the **binary** while the flag was on requires **revoking all player-scope refresh tokens and waiting out the 15-minute access-token TTL first**. Short access TTL keeps the exposure window small.

---

## 7. Adversarial review findings (re-verified against the built code)

| ID | Severity | Finding | Verified in code |
|---|---|---|---|
| AR-1 | P0 | Player token (empty org) treated as platform admin | `IsPlatformUser()=scope=="platform"`; `DeriveScope` never infers platform for player/unknown; unit + integration (player→403 on org route) green |
| AR-2 | P0 | Code rollback re-opens empty-org⇒platform while player tokens live | Flag-gated minting; documented revoke+TTL rollback procedure (matrix §6) |
| AR-3 | P0 | Unique-index creation aborts on legacy duplicate user_ids | Dedup/archive ordered **before** index in 000028; dry-run report provided |
| AR-4 | P1 | Player token reaching an org-admin tree | `RequireOrgScope` allows only {organizer, platform}; integration-verified 403 |
| AR-5 | P1 | `DeriveScope` misclassifies ambiguous legacy empty-org token as platform | Requires recognized platform role slug; else → onboarding (least privilege); unit-tested |
| AR-6 | P1 | Refresh-based scope escalation | `resolvePrincipalForScope` re-verifies entitlement; integration-verified 403 for player→platform/organizer |
| AR-7 | P1 | `visibility` public-by-default exposure | Default `private`; CHECK constraint; visibility-aware read hides private as 404 |
| AR-8 | P1 | Org slug `me`/`players` shadows non-org routes | Reserved-slug guard in `generateSlug` (backend) + `[orgSlug]` layout guard (frontend); both tested |
| AR-9 | P1 | Onboarding dead-end / can't become a player | `/me/player` accepts any authenticated scope; 0-org+profile re-login → player; verified in integration |
| AR-10 | P1 | DOWN migration fails once null-org profiles exist | Guarded `DO` block; no-op + notice instead of failure |
| AR-11 | P1 | Dedup archives wrong survivor | Deterministic `MIN(created_at), MIN(id)`; reversible `archived_at`; no FK repoint |
| AR-12 | P2 (accepted) | Service `assertOrgOwnership` still trusts empty-org as platform | Middleware fully closes GP-1's surface (no new org route added); tracked for a later phase |
| AR-13 | P2 (accepted) | Archived duplicate keeps its own history (no merge) | Explicit no-claim policy; ranking is post-GP-1 |
| AR-14 | P2 (accepted) | Bulk signups → smurf substrate | Existing rate-limit + email verification; broader posture tracked |

**Result: 0 P0 and 0 P1 open.** All P0/P1 are implemented and test-verified; residuals are P2 with explicit rationale.

---

## 8. Remediations applied

Every P0/P1 in §7 was designed-in and is present in the shipped code (not deferred):
- Scope-based `IsPlatformUser` + least-privilege `DeriveScope` (AR-1, AR-5).
- Flag-gated player-token issuance + documented binary-rollback procedure (AR-2).
- Dedup-before-index migration ordering + dry-run report (AR-3, AR-11).
- `RequireOrgScope` {organizer, platform} only (AR-4).
- Entitlement re-verification on refresh (AR-6).
- `visibility` default private + visibility-aware 404 (AR-7).
- Reserved-slug guards on both backend and frontend (AR-8).
- Any-scope profile creation + persona re-login (AR-9).
- Guarded DOWN migration (AR-10).

Out of scope by instruction and **not** started: GP-2, global rankings, team consent workflows, recruitment, tournament redesign, reputation system, match changes. `PROJECT_STATE.md` not modified.
