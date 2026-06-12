# GP-1 Implementation Specification (Identity & Persona Foundation)

**Status:** Implementation-ready. Design only — no code committed.
**Date:** 2026-06-12.
**Parent:** [gp-blueprint-kabaddi.md](gp-blueprint-kabaddi.md) (source of truth). This spec consolidates the blueprint's GP-1 (identity foundation) + GP-2 (auth scopes) + the *foundations* of GP-3/GP-4 (self-profile + persona onboarding) into one shippable, **feature-flag-gated** phase, per the readiness review's scoping.

**Ranking note:** GP-1 touches no ranking surface. It introduces **no rating-like number** and preserves all historical facts (no `match_events`/stats changes). The 1:1 identity it establishes is exactly what a later Glicko-2 back-computation needs.

**Codebase anchors (verified):** next migration = `000028`; JWT in [auth/model.go](backend/internal/auth/model.go); middleware in [auth/middleware.go](backend/internal/auth/middleware.go); players table from [000005](backend/db/migrations/000005_create_players.up.sql); sqlc queries in `db/queries/`; frontend app tree under `frontend/src/app/(app)/[orgSlug]`.

---

## 0. GP-1 in one paragraph

Make `players` the spine of a **global, user-owned PlayerProfile** (one per user) *without removing org coupling yet*; replace the overloaded "empty `organization_id` ⇒ platform admin" assumption with an explicit **`scope`** claim (`player`/`organizer`/`onboarding`/`platform`); and ship the **persona-onboarding foundation** (create-your-profile path + player home) behind a flag so the default runtime behavior is unchanged. Everything is additive and reversible; ownership removal is only *staged*, never executed in GP-1.

---

## 1. Exact schema changes

Logical model: **the existing `players` table IS the PlayerProfile.** No rename in GP-1 (a rename would touch every sqlc query and Go reference — out of scope, cosmetic, deferred). We add columns and relax/constrain.

| Change | Statement intent | Why |
|---|---|---|
| `players.organization_id` → **nullable** | drop `NOT NULL` | a global profile created by its owner has **no** org (`organization_id IS NULL`). CASCADE FK **retained** in GP-1 (removal staged to GP-7). |
| add `players.visibility TEXT NOT NULL DEFAULT 'private'` | CHECK in (`'public'`,`'unlisted'`,`'private'`) | privacy-safe default (adversarial: never expose by default). No reader consumes it in GP-1. |
| add `players.archived_at TIMESTAMPTZ` (nullable) | marks a non-canonical duplicate identity | enables the 1:1 unique index without deleting/merging legacy rows (no-claim policy). |
| add **partial unique index** `uq_players_user_id` on `(user_id)` `WHERE user_id IS NOT NULL AND archived_at IS NULL` | one **canonical** profile per user | enforces *One User → One PlayerProfile* at the DB; archived dups excluded. |

No other tables change in GP-1. No FK is dropped or altered.

---

## 2. Exact migrations

### `000028_player_profile_foundation.up.sql`
```sql
-- 1) additive columns (safe, no data dependency)
ALTER TABLE players ADD COLUMN visibility   TEXT NOT NULL DEFAULT 'private';
ALTER TABLE players ADD COLUMN archived_at  TIMESTAMPTZ;

ALTER TABLE players
    ADD CONSTRAINT chk_players_visibility
    CHECK (visibility IN ('public', 'unlisted', 'private'));

-- 2) relax org ownership (column still populated; CASCADE FK unchanged in GP-1)
ALTER TABLE players ALTER COLUMN organization_id DROP NOT NULL;

-- 3) dedup BEFORE the unique index: keep the earliest row per user_id, archive the rest.
--    Deterministic survivor: MIN(created_at), tiebreak MIN(id). FK references are NOT
--    repointed (no-claim policy) — archived rows retain their history in place.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY user_id
               ORDER BY created_at ASC, id ASC
           ) AS rn
    FROM   players
    WHERE  user_id IS NOT NULL
      AND  archived_at IS NULL
)
UPDATE players p
SET    archived_at = NOW(),
       updated_at  = NOW()
FROM   ranked r
WHERE  p.id = r.id
  AND  r.rn > 1;

-- 4) enforce 1:1 (canonical, non-archived rows only)
CREATE UNIQUE INDEX uq_players_user_id
    ON players (user_id)
    WHERE user_id IS NOT NULL AND archived_at IS NULL;

COMMENT ON COLUMN players.organization_id IS
    'Nullable from GP-1. NULL = global, user-owned profile with no origin org. '
    'Non-NULL = legacy org-created profile. Ownership semantics removed in GP-7.';
COMMENT ON COLUMN players.archived_at IS
    'Non-NULL marks a non-canonical duplicate identity (legacy multi-org rows for one user). '
    'Archived rows are read-only, non-ranking, retained for history. Never merged (no-claim policy).';
COMMENT ON COLUMN players.visibility IS
    'public | unlisted | private. Default private (privacy-safe). Consumed from GP-3 onward.';
```

### `000028_player_profile_foundation.down.sql`
```sql
DROP INDEX IF EXISTS uq_players_user_id;

ALTER TABLE players DROP CONSTRAINT IF EXISTS chk_players_visibility;
ALTER TABLE players DROP COLUMN IF EXISTS visibility;
ALTER TABLE players DROP COLUMN IF EXISTS archived_at;

-- Restore NOT NULL only if no global (null-org) profiles were created while GP-1 was live.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM players WHERE organization_id IS NULL) THEN
        ALTER TABLE players ALTER COLUMN organization_id SET NOT NULL;
    ELSE
        RAISE NOTICE 'Global profiles exist (organization_id IS NULL); leaving column nullable.';
    END IF;
END $$;
```
> **Pre-flight (run before applying up, no writes):** a **dry-run dedup report** —
> `SELECT user_id, COUNT(*) FROM players WHERE user_id IS NOT NULL GROUP BY user_id HAVING COUNT(*) > 1;`
> Operator reviews the survivor set before the archive runs.

### sqlc query additions (`db/queries/players.sql`)
```sql
-- name: GetPlayerProfileByUserID :one
-- The caller's canonical (non-archived) profile, regardless of org.
SELECT * FROM players
WHERE  user_id = $1 AND archived_at IS NULL
LIMIT  1;

-- name: CreateGlobalPlayerProfile :one
-- Owner-created profile: organization_id is NULL.
INSERT INTO players (user_id, display_name, jersey_number, position,
    height_cm, weight_kg, dominant_hand, nationality, date_of_birth, bio, visibility)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING *;

-- name: UpdateOwnPlayerProfile :one
-- Identity-field update for the owner. organization_id / user_id / status are NOT touched here.
UPDATE players
SET    display_name=$3, jersey_number=$4, position=$5, height_cm=$6, weight_kg=$7,
       dominant_hand=$8, nationality=$9, date_of_birth=$10, bio=$11, visibility=$12,
       updated_at=NOW()
WHERE  id=$1 AND user_id=$2 AND archived_at IS NULL
RETURNING *;
```
Existing org-scoped player queries are **unchanged** (dual surface). `ListPlayersByOrganization` / pagination should add `AND archived_at IS NULL` so archived dups never surface in org lists (small, safe addition).

---

## 3. Exact JWT changes

### Claims (`auth/model.go`)
```go
const (
    ScopePlayer     = "player"
    ScopeOrganizer  = "organizer"
    ScopeOnboarding = "onboarding"
    ScopePlatform   = "platform"
)

type JWTClaims struct {
    UserID          string `json:"user_id"`
    OrganizationID  string `json:"organization_id,omitempty"`
    Role            string `json:"role,omitempty"`
    Email           string `json:"email"`
    Scope           string `json:"scope,omitempty"`              // NEW
    PlayerProfileID string `json:"player_profile_id,omitempty"`  // NEW (present when scope==player)
    jwt.RegisteredClaims
}

type AuthUser struct {
    UserID, OrganizationID, Role, Email string
    Scope, PlayerProfileID              string // NEW
}
```

### Claims emitted per scope (token issuance, auth service)
| Scope | organization_id | role | player_profile_id |
|---|---|---|---|
| `organizer` | the org UUID | org role slug | — |
| `onboarding` | "" | "onboarding" | — |
| `platform` | "" | platform role slug | — |
| `player` | "" | "" | the profile UUID |

### Backward-compatible scope derivation (the critical safety function)
```go
// DeriveScope resolves the scope of any token, including legacy (pre-GP-1)
// tokens that carry no scope claim. Least-privilege on ambiguity.
func DeriveScope(c *JWTClaims, platformRoleSlugs map[string]struct{}) string {
    if c.Scope != "" {
        return c.Scope
    }
    if c.OrganizationID != "" {
        return ScopeOrganizer
    }
    if c.Role == "onboarding" {
        return ScopeOnboarding
    }
    if _, ok := platformRoleSlugs[c.Role]; ok {
        return ScopePlatform
    }
    return ScopeOnboarding // safe fallback: NEVER infer platform from an unknown empty-org token
}
```
- Legacy tokens never had `player` scope (player tokens didn't exist pre-GP-1), so inference can never *create* a player principal — it only ever recognizes scopes that already existed.
- **The platform path requires a recognized platform role slug**; an unknown empty-org/empty-role token falls back to `onboarding` (least privilege), closing the "empty token ⇒ platform" footgun.

### `IsPlatformUser` (the privilege-escalation fix)
```go
func (u *AuthUser) IsPlatformUser() bool { return u.Scope == ScopePlatform } // was: OrgID=="" && Role!="onboarding"
```

---

## 4. Exact middleware changes (`auth/middleware.go`)

1. **`RequireAuth`** — after `ValidateToken`, set `principal.Scope = DeriveScope(claims, cfg.PlatformRoleSlugs)` and `principal.PlayerProfileID = claims.PlayerProfileID`. Everything downstream reads `principal.Scope`.

2. **`RequireOrgScope` (rewrite, behavior-preserving + hardened):**
```go
// Allow only org-acting principals into org-admin trees. Platform admins are
// allowed (they administer all orgs). Player & onboarding are rejected.
func RequireOrgScope() mw {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            p := GetAuthUser(r.Context())
            if p == nil { response.Error(w, 401, "authorization required"); return }
            if p.Scope != ScopeOrganizer && p.Scope != ScopePlatform {
                response.Error(w, 403, "insufficient permissions"); return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```
Legacy parity: onboarding still rejected; organizer/platform still allowed; **player now also rejected** (new, correct).

3. **`RequireScope(allowed ...string)`** — generic guard (403 if `principal.Scope ∉ allowed`).

4. **`RequirePlayerScope()`** = `RequireScope(ScopePlayer)` — reserved for future player-only feature routes (e.g. self-register in GP-6). **GP-1 self-profile routes do not use it** — they use `RequireAuth` + **service-layer ownership** (`user_id == actor`) so an organizer/onboarding/platform user can still create and view *their own* profile.

5. **`RequirePermission` onboarding carve-out** — extend the existing `organization.create` carve-out with a symmetric `profile.create` carve-out gated on a DB-verified `HasNoPlayerProfile(userID)` (mirrors `IsZeroOrgUser`). Used only by the create-profile route.

> **Standing invariant reaffirmed:** every org-scoped route tree keeps `RequireAuth` → `RequireOrgScope`. GP-1 adds **no** new org-scoped tree, so the existing 11 trees are unchanged and the invariant from [[auth-onboarding-remediation]] holds.

---

## 5. Exact API contracts

All new endpoints sit **outside** `/organizations/{slug}` and are gated by `GP_PLAYER_PERSONA_ENABLED` (when false → routes return 404, login behaves exactly as pre-GP-1).

### 5.1 Self-profile (new)
| Method | Path | Auth | Body | Success | Errors |
|---|---|---|---|---|---|
| `POST` | `/api/v1/me/player` | `RequireAuth` + service `HasNoPlayerProfile` | `{display_name*, bio?, nationality?, date_of_birth?, jersey_number?, position?, height_cm?, weight_kg?, dominant_hand?, visibility?}` | `201` PlayerProfile (org_id null) | `409 profile_exists`, `422` validation |
| `GET` | `/api/v1/me/player` | `RequireAuth` | — | `200` caller's canonical profile | `404 no_profile` |
| `PATCH` | `/api/v1/me/player` | `RequireAuth` + ownership (`user_id==actor`) | identity fields + `visibility` (partial) | `200` updated | `404 no_profile`, `422`; `organization_id`/`user_id`/`status` in body → `422 immutable_field` |
| `GET` | `/api/v1/players/{id}` | `RequireAuth` | — | `200` profile **respecting `visibility`** (private → 404 unless owner/platform) | `404` |

Response DTO (additions vs current player response): `user_id` always present; `visibility`; `organization_id` nullable. **No** rating/reputation field (forbidden at launch).

### 5.2 Auth (extended)
**`POST /api/v1/auth/login`** — outcome by state (flag-on):
- 0 orgs + canonical profile → **player** token; 0 orgs + no profile → **onboarding** token; 1 org → **organizer** token (auto, unchanged); N orgs → `409 organization_required` (+ org list, unchanged); platform role → **platform** token.
- Flag-off: identical to today (0 orgs → onboarding; the player branch is inert because no player tokens are minted).

**`POST /api/v1/auth/refresh`** — accept optional `{scope?, organization_id?}` to request a re-scoped token (persona switch). **Entitlement is re-verified before minting:**
- `scope=organizer` + `organization_id` → must hold a role grant in that org (existing org-select check) else `403`.
- `scope=player` → must have a canonical profile else `403`.
- `scope=platform` → must hold a platform role else `403`.
- `scope=onboarding` → only if 0 orgs AND no profile else `403`.
- omitted → preserve current behavior (re-mint same effective scope).

**`GET /api/v1/auth/me`** — add `scope` and `player_profile_id` (nullable) to the response.

---

## 6. Exact frontend route changes

Gated by `NEXT_PUBLIC_GP_PLAYER_PERSONA` (or feature-detected from token `scope`).

- **New top-level route group `app/(player)/`** (route groups add no path segment):
  - `app/(player)/me/page.tsx` → `/me` (player home).
  - `app/(player)/me/profile/page.tsx` → `/me/profile` (create/edit own profile).
- **Onboarding becomes a persona chooser:** `app/(auth)/onboarding/page.tsx` → *"Continue as Player"* (→ `POST /me/player` → refresh to player scope → `/me`) or *"Create an Organization"* (existing flow).
- **`use-auth-guard`** routes by `scope`: `player`→`/me`, `organizer`→`/{orgSlug}`, `onboarding`→`/onboarding`, `platform`→existing. Player-scope sessions are **not** redirected into org selection.
- **`auth.store`** — persist `scope`, `playerProfileId`.
- **`client.ts` `attemptTokenRefresh`** — include `scope` alongside the existing `organization_id` so silent refresh preserves persona.
- **`query-keys.ts`** — add a non-org `meKeys` factory; `/me` data is **not** cleared on org switch.
- **Reserved-slug guard (required before shipping `/me`):** add `me`, `player`, `players`, `onboarding`, `auth`, `api`, `login`, `register`, `admin` to the org-slug denylist (org-create validation), plus a startup check for any existing org whose slug collides. Prevents `/{orgSlug}` shadowing `/me`.

No SSE change in GP-1 (player tokens don't open streams yet; SSE persona support is GP-4 proper).

---

## 7. Exact test plan

**Migration (`testutil` container, postgres:17):**
- Applies on DBs seeded with: no players; players with null `user_id`; **two players same `user_id` across two orgs** (→ exactly one archived, earliest kept — assert survivor by `created_at`); a null-org global profile.
- `uq_players_user_id` present and partial (insert a 2nd canonical row same user → unique violation; insert same user_id with `archived_at` set → allowed).
- Down: drops index/columns; restores `NOT NULL` only when no null-org rows; **no-op + notice** when global profiles exist (assert it does not error).

**JWT / scope:**
- Round-trip emits `scope` + `player_profile_id`.
- `DeriveScope` table test — 6 cases: explicit scope wins; org set→organizer; role=onboarding→onboarding; known platform slug→platform; **unknown empty-org/empty-role→onboarding** (least-privilege); legacy organizer token→organizer.
- **`IsPlatformUser` (P0 escalation):** `scope=player`→false; `scope=platform`→true; legacy platform token→true; legacy organizer→false; legacy onboarding→false.

**Middleware:**
- `RequireOrgScope` table: organizer/platform pass; player/onboarding→403.
- `/me/player` ownership: other user's profile PATCH→403; `organization_id`/`status` in PATCH body→422.

**Endpoints:**
- `POST /me/player` creates org_id-null profile; **second create→409**; GET returns own; PATCH updates; private `GET /players/{id}` by non-owner→404, by owner→200.
- Login: 0-org+profile→player; 0-org+no-profile→onboarding; 1-org→organizer; N-org→409; platform→platform.
- Refresh entitlement: `scope=player` w/o profile→403; `scope=organizer` w/o grant→403; `scope=platform` w/o platform role→403.

**Regression / isolation (must stay green):**
- Full existing suite (org player CRUD, BOLA, MT isolation) unchanged.
- **New MT-style gate:** a `player` token → **403 on every org-admin route and every platform route** (mirror existing MT-1 platform-admin gate).
- **Flag-off:** `/me/*` routes → 404; login mints no player tokens; behavior byte-identical to pre-GP-1.

**Frontend:** persona chooser renders both paths; guard routes by scope; refresh includes scope; `meKeys` survive an org switch; reserved-slug denylist rejects `me`; flag-off hides player routes.

---

## 8. Rollout strategy

Order is load-bearing — **understand `scope` before any player token exists; migrate before code reads new columns:**
1. **Migrate** `000028` (run dry-run dedup report → review → apply). Additive; org-first behavior unchanged.
2. **Deploy backend** with scope plumbing + `IsPlatformUser` fix + `RequireOrgScope` rewrite (all safe via `DeriveScope` legacy inference). Player endpoints + login player-branch **behind `GP_PLAYER_PERSONA_ENABLED=false`.**
3. **Soak** (≥ one access-token TTL, e.g. ≥ ~1h): watch auth error rate, 403 rate on org routes, login distribution. Legacy no-scope access tokens drain within TTL.
4. **Staging E2E** with flag on: full persona onboarding (register→verify→create profile→player home; and create-org path).
5. **Canary** flag on in prod (small %), then **full**. Ship **frontend** flag on only after the backend flag is on.

## 9. Rollback strategy

- **Primary (instant, safe):** set `GP_PLAYER_PERSONA_ENABLED=false`. Player endpoints disabled, no new player tokens minted; system reverts to org-first. Schema can stay (additive columns are inert). **This is the default rollback.**
- **Code rollback constraint (P0-guarded):** reverting the backend binary restores the old `IsPlatformUser` (empty-org ⇒ platform). If **any player tokens were minted while the flag was on**, those empty-org tokens would then be read as platform admin. Therefore code rollback is permitted freely **only while the flag has never been enabled**. If the flag *was* enabled: (a) **revoke all player-scope refresh tokens**, (b) wait out the access-token TTL, *then* roll back code. Short access TTL keeps this window small. (See adversarial AR-2.)
- **Schema rollback:** `000028.down` is safe pre-flag (no null-org rows). Once global profiles exist it **deliberately refuses** to restore `NOT NULL` and leaves the column nullable (no data loss). Treat full schema-down as one-way after profiles exist — consistent with the blueprint's "ownership removal is staged and not freely reversible."

---

## 10. Adversarial review of GP-1 (and the resulting revisions)

Reviewed for: security, migration, privilege escalation, tenant isolation, onboarding dead-ends. Findings carried into the design above; **all P0/P1 resolved**, residuals are P2/P3.

| ID | Sev (raw) | Finding | Resolution (folded into spec) | Status |
|---|---|---|---|---|
| AR-1 | **P0** | Player token shares the empty-org shape → read as **platform admin**. | `IsPlatformUser()=scope=="platform"`; `DeriveScope` never infers platform for a player/unknown token (least-privilege fallback). Escalation tests mandated (§7). | **Resolved** |
| AR-2 | **P0** | **Code rollback** while player tokens exist re-enables empty-org⇒platform → escalation. | Player-token issuance is flag-gated (so the *common* rollback — flag off — is safe); documented hard procedure (revoke player refresh tokens + TTL drain) before code rollback when flag was on; short access TTL. (§9) | **Resolved → P2 residual (operational)** |
| AR-3 | **P0** | `CREATE UNIQUE INDEX` aborts the migration if duplicate `user_id` rows exist. | Deterministic **dedup/archive step ordered before** the index; predicate excludes `archived_at`; mandatory dry-run report. (§2) | **Resolved** |
| AR-4 | **P1** | Player token reaching an **org-admin tree** → tenant-isolation break (org services exempt empty-org as platform admin). | `RequireOrgScope` now allows only `{organizer, platform}` and is mounted on all 11 trees (unchanged invariant); GP-1 adds **no** org tree; player-token-403 gate test added. | **Resolved** |
| AR-5 | **P1** | `DeriveScope` could misclassify an ambiguous legacy empty-org token as platform. | Platform inference requires a **recognized platform role slug**; otherwise → `onboarding` (least privilege). (§3) | **Resolved** |
| AR-6 | **P1** | Refresh-based **scope escalation** (request `platform`/`organizer` without entitlement). | Refresh **re-verifies entitlement** per scope before minting (§5.2); tests for each 403 path. | **Resolved** |
| AR-7 | **P1** | `visibility` default `public` would expose all profiles the moment reads ship. | Default **`private`**; no reader consumes it in GP-1; exposure decided in GP-3. (§1) | **Resolved** |
| AR-8 | **P1** | **Slug collision**: an org with slug `me` shadows the `/me` route. | Reserved-slug denylist on org-create + startup collision check, **required before** the `/me` route ships. (§6) | **Resolved** |
| AR-9 | **P1** | **Onboarding dead-end** — user with neither profile nor org, or organizer wanting to also be a player. | Onboarding scope → chooser (profile **or** org); `POST /me/player` allowed for **any** scope (organizer can become a player); re-login resolves 0-org+profile→player. No terminal state. (§5,§6) | **Resolved** |
| AR-10 | **P1** | Migration **down** restoring `NOT NULL` fails/locks once null-org profiles exist. | Guarded `DO` block: restores `NOT NULL` only if no null-org rows, else no-op + notice. (§2) | **Resolved** |
| AR-11 | **P1** | Dedup archives the **wrong** survivor → identity/attribution corruption. | Deterministic survivor (`MIN(created_at)`, tiebreak `MIN(id)`); archive sets a reversible `archived_at` only; **no FK repoint**; dry-run review. (§2) | **Resolved** |
| AR-12 | P2 | Service-layer `assertOrgOwnership` still treats `actorOrgID==""` as platform admin (defense-in-depth gap if a future org route forgets `RequireOrgScope`). | Middleware fully closes GP-1's surface (no new org route added); tracked to migrate `assertOrgOwnership` to scope-based in a later phase. | **Accepted (P2)** |
| AR-13 | P2 | **History fragmentation**: an archived duplicate keeps its own match/team history (no merge). | Explicit **no-claim** product decision; ranking is post-GP-1 and counts canonical only; legacy org rows are largely historical. | **Accepted (P2)** |
| AR-14 | P2 | Bulk automated signups → many "fresh equal" profiles (smurf substrate). | Existing register rate-limiting + email verification; CAPTCHA/throttle tuning tracked with the broader smurf posture (blueprint §8, accepted residual). | **Accepted (P2)** |
| AR-15 | P3 | `players` not renamed to `player_profiles` (naming/clarity). | Cosmetic; deferred to avoid touching all sqlc/Go refs in a foundation phase. | **Accepted (P3)** |

**Post-revision status: 0 P0, 0 P1 open.** Residuals AR-12…AR-15 are P2/P3 with explicit owners/rationale and do not block GP-1.

---

## 11. Definition of done (GP-1)
- `000028` up/down land with dry-run report; dedup deterministic; `uq_players_user_id` partial-unique enforced.
- `scope` claim emitted on all newly issued tokens; `DeriveScope` covers legacy; `IsPlatformUser()` is scope-based; escalation + MT player-403 tests green.
- `RequireOrgScope` allows only `{organizer, platform}`; all 11 org trees unchanged and passing.
- `/api/v1/me/player` (POST/GET/PATCH) + `GET /players/{id}` honor ownership + visibility + 1:1 (409 on second create); **no rating-like field exposed**.
- Login/refresh persona resolution + entitlement-checked re-scoping.
- Frontend persona chooser, `(player)/me` routes, scope-based guard, reserved-slug denylist — all behind the flag.
- Flag-off runtime is byte-identical to pre-GP-1; full regression suite green.
- This document + the migration dry-run report attached to the GP-1 change.
```
