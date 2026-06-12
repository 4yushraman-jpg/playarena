# Architecture Design — Global Player Identity (Project "GP")

**Status:** Design only. No code in this document is implemented.
**Author:** Architecture review, 2026-06-12.
**Scope:** Migrate PlayArena from an organization-first model where *organizations own players* to a user-first model where *PlayerProfile is a global, user-owned identity and organizations merely use players.*

> Product invariant being introduced:
> **PlayerProfile is global. Organizations use players. Organizations do not own players.**

---

## 0. Executive summary

PlayArena today is structurally organization-first. A "player" is not a person — it is a row scoped to one organization (`players.organization_id NOT NULL`, `ON DELETE CASCADE`). The same human who plays in two organizations has **two unrelated player rows**, and if an org is deleted, its players are deleted with it. There is no global player identity, no player-owned rankings, and no way for a player to exist or act without an organization. The entire authenticated route surface lives under `/api/v1/organizations/{slug}/...` (backend) and `/(app)/[orgSlug]/...` (frontend); a user with zero orgs is funneled into an onboarding flow whose only capability is *create an organization*.

The target model promotes `users` (which is already global and org-agnostic) to carry an optional global `PlayerProfile`, makes organization membership a 0..N relationship, and re-bases rankings, registrations, team rostering, and match references onto the global profile. Organizations remain first-class for organizers (teams, tournaments, members) but lose **ownership** of player identity.

The migration is large but tractable because **every domain FK already points at `players.id`**. The recommended strategy evolves `players` into the global profile *in place* (preserving its primary keys), so match/registration/stats/membership references never have to be rewritten. The hard problem is not FK rewiring — it is (a) **collapsing duplicate per-org player rows for the same user into one global identity**, and (b) **un-overloading the `organization_id == ""` token shape**, which currently means "platform admin OR onboarding" and would otherwise gain a third meaning ("player persona").

---

## 1. Current architecture analysis

### 1.1 The identity model today

| Concept | Table | Ownership today | Global? |
|---|---|---|---|
| Human account | `users` | none — already global | ✅ yes (no `organization_id` column) |
| Org membership | `user_organization_roles` | org-scoped grant | n/a (already a join) |
| Player | `players` | `organization_id NOT NULL`, FK **CASCADE** | ❌ **org-owned** |
| Team | `teams` | `organization_id NOT NULL`, FK CASCADE | ❌ org-owned (intended to stay) |
| Team roster | `team_memberships` | `team_id` + `player_id`, both implicitly same org | ❌ |
| Tournament | `tournaments` | `organization_id NOT NULL` CASCADE | ❌ org-owned (intended to stay) |
| Registration | `tournament_registrations` | `organization_id` = registrant org; `player_id` org-scoped | ❌ |
| Match | `matches` | denormalized `organization_id`; player FKs | ❌ |
| Match event | `match_events` | denormalized `organization_id`; `player_id` | ❌ |
| Player stats | `player_tournament_stats` | `organization_id NOT NULL` (host org) | ❌ org-attributed |

`users` is the bright spot: migration `000003` explicitly states *"users has no organization_id — org membership is managed via user_organization_roles."* The global identity substrate **already exists**. The problem is everything *player-shaped* was built beside it instead of on top of it.

### 1.2 Every place that assumes "Organization owns Player"

**Database schema**

1. `players.organization_id UUID NOT NULL` + `fk_players_organization ... ON DELETE CASCADE` (`000005`). This is the literal ownership edge: deleting an org hard-deletes its players. **This single FK is the spine of the org-first model.**
2. `players.user_id UUID NULL` with `ON DELETE SET NULL`, commented *"One user may have N player profiles (different orgs, different sports)."* The data model **expects** identity fragmentation; there is no uniqueness on `user_id`.
3. `team_memberships` (`000007`) FKs both `team_id` and `player_id` with CASCADE; there is no constraint that the player and team belong to the same org, but the application only ever rosters same-org players (players are listed from `/organizations/{slug}/players`).
4. `tournament_registrations.player_id` (`000009`) references org-scoped players; the partial-unique indexes and the `organization_id` column ("registrant's org") presume the registrant is an org.
5. `matches.home_player_id/away_player_id/winner_player_id` (`000010`) + denormalized `matches.organization_id` (must equal parent tournament org).
6. `match_events.player_id` + denormalized `match_events.organization_id` (`000011`).
7. `player_tournament_stats.organization_id NOT NULL` (`000027`), commented *"organization_id is always the tournament HOST's org. Cross-org participants appear in the host org's rankings, not their own."* — rankings are **org-attributed, not player-owned**.

**Players module** (`internal/players`)

8. Routes mounted at `/api/v1/organizations/{slug}/players` (`routes.go`), behind `RequireAuth` + `RequireOrgScope`. There is **no global player route**.
9. Every query in `db/queries/players.sql` requires `organization_id` "to enforce tenant isolation." `GetPlayerByID`, `ListPlayersByOrganization`, `UpdatePlayer`, `SoftDeletePlayer` all filter/scope by org.
10. `service.go` `assertOrgOwnership(actorOrgID, targetOrgID)` — the BOLA guard treats a player as org property. `Create` requires an org context and inserts `OrganizationID: org.ID`. There is no "create my own profile" path.

**Authorization model** (`internal/auth`)

11. JWT (`JWTClaims`) carries a single `OrganizationID` + `Role`. A principal is bound to **one org at a time**. There is no "player persona."
12. `AuthUser.IsPlatformUser()` = `OrganizationID == "" && Role != OnboardingRole`. The empty-org token shape is overloaded: **platform admin OR onboarding**. (Documented as a security invariant in project memory.)
13. `RequireOrgScope()` rejects onboarding tokens from all 11 org-scoped route trees. A user without an org **cannot reach players, teams, tournaments, registrations, matches, match_events, media, members, notifications, rankings, or webhooks.**
14. RBAC (`user_organization_roles`, `role_permissions`) is **entirely org-scoped**. `player.create/update/delete` permissions only have meaning inside an org. There is no capability a user holds over their *own* identity.

**Rankings** (`internal/rankings`)

15. `ListPlayerRankings` filters `pts.organization_id = $1` — rankings are per-org leaderboards. Routes are under `/organizations/{slug}/rankings`. Closed-ecosystem aggregation already exists (good), but it aggregates **per host org**, not per global player.

**Registrations** (`internal/tournament_registrations`)

16. `registerPlayer` calls `repo.GetPlayerByID(ctx, playerUID, orgID)` — an individual entrant **must be a player of the registrant's org**. Registration is gated on `tournament.update` (an org-admin permission), so **players cannot self-register**; an org admin registers them.

**Matches / Match events / SSE**

17. Match and event services denormalize and verify `organization_id`; the live-scoring + standings pipeline writes `player_tournament_stats` with the host org id.
18. SSE hub keys subscriptions by `subKey{orgID, userID}` and the stream handler 403s platform/onboarding (empty-org) tokens. A player without an org receives **no real-time stream**.

**Frontend**

19. The entire authenticated tree is `app/(app)/[orgSlug]/...`. Players live at `[orgSlug]/players`. There is **no global player profile route**, no "my profile," no player home.
20. `use-auth-guard` routes `role === "onboarding"` → `/onboarding`, whose sole action is *create an organization*. A player-first user has nowhere to land.
21. Query-key factories (`query-keys.ts`) are all org-scoped; org switch does `queryClient.clear()`. Player data is cached per org.

### 1.3 The core defect, stated precisely

> A person is modeled as **one `players` row per organization**, owned by that org, deletable with that org, with rankings attributed to the org rather than the person — and the auth/routing layer makes it **impossible to be a player without first being inside an organization**.

---

## 2. Proposed architecture

### 2.1 Target entity model

```
User (global, already exists)
├─ PlayerProfile (0..1, global, user-owned)   ← the persona
└─ OrganizationMembership (0..N)               ← user_organization_roles

PlayerProfile (global)
├─ Identity (display_name, nationality, dob, bio, physical attrs)
├─ Statistics  ─┐
├─ Match History│  derived from match_events / matches / *_tournament_stats
├─ Rankings    ─┘  aggregated GLOBALLY, not per-org
└─ Achievements

Organization (global owner of organizer concepts)
├─ Members (user_organization_roles)
├─ Teams (org-owned)
└─ Tournaments (org-owned)

OrgRoster (NEW join: organization ↔ player_profile)
└─ how an org "uses" a global player without owning them
```

### 2.2 Key decision — evolve `players` in place (do **not** create a parallel table)

Two candidate shapes were considered:

- **Model A (parallel):** new `player_profiles` table + keep `players` as an org-roster row that points at a profile. *Rejected:* requires rewriting every `player_id` FK in `team_memberships`, `tournament_registrations`, `matches`, `match_events`, `*_tournament_stats`, plus a dual-identity period that doubles every query.
- **Model B (in place) — RECOMMENDED:** `players` *becomes* the global `PlayerProfile`. Its primary key `players.id` is unchanged, so **all existing FKs stay valid with zero rewrites**. Ownership is removed by (a) making `organization_id` nullable and changing CASCADE→SET NULL (reinterpreted as "origin org," then dropped), and (b) introducing `org_player_links` for roster membership.

We adopt **Model B**.

### 2.3 Target schema deltas (conceptual)

```sql
-- players becomes the global PlayerProfile
ALTER TABLE players ALTER COLUMN organization_id DROP NOT NULL;        -- step 1: decouple
-- replace CASCADE ownership edge with a non-owning reference (then drop the column in a later phase)
-- new: at most one CLAIMED profile per user
CREATE UNIQUE INDEX uq_players_user_id ON players (user_id) WHERE user_id IS NOT NULL;
-- new: profile visibility (closed ecosystem, but public/unlisted for privacy)
ALTER TABLE players ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public';

-- NEW: organization rosters a global player (replaces implicit org ownership)
CREATE TABLE org_player_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    player_id       UUID NOT NULL REFERENCES players(id)       ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'active',  -- active|invited|removed
    invited_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, player_id)
);
```

- `player_tournament_stats.organization_id` is **retained** (host-org attribution remains useful for org dashboards), but the **ranking query drops the org filter** to produce the global board; org-scoped boards become a *filtered view* of the same data.
- `uq_players_user_id WHERE user_id IS NOT NULL` enforces "one claimed global profile per user" while still allowing org-created **unclaimed** profiles (historical/scouted athletes with `user_id IS NULL`), which can later be *claimed* and merged.

### 2.4 Authorization target

Introduce an explicit **persona / scope** claim instead of overloading `organization_id == ""`:

```
JWTClaims {
  user_id, email,
  scope: "org" | "platform" | "player" | "onboarding",   // NEW explicit field
  organization_id (only when scope == "org"),
  role (org role when scope == "org"; "" otherwise)
}
```

- `scope == "player"` → the user is acting as themselves (their global profile). No org. Allowed to: read/update *their own* `PlayerProfile`, self-register for tournaments, accept/decline org roster invites, read their global rankings/history.
- New ownership rule for the players domain: **a user owns the profile where `players.user_id == actor.user_id`.** Org admins may edit *roster metadata* (jersey in a team) but not the global identity fields.
- `RequireOrgScope()` keeps its job (blocking onboarding/player tokens from org-admin trees) but the new player-self routes live **outside** org scope under `/api/v1/me/...` or `/api/v1/players/{id}`.

---

## 3. Migration strategy (high level)

Strangler-fig, additive-first, dual-read. Ordering principle: **add new structures and reads before removing old ones; remove ownership last.**

1. **Additive schema** — nullable `organization_id`, `uq_players_user_id`, `org_player_links`, `visibility`, JWT `scope` claim. Old paths untouched.
2. **Backfill** — one `org_player_links` row per existing `(players.organization_id, players.id)`; set `scope` on issued tokens; no profile merging yet.
3. **New surfaces** — global player routes (`/api/v1/me/player`, `/api/v1/players/{id}`), player persona login, player onboarding branch, self-registration. Old org-scoped player routes keep working (dual surface).
4. **Re-base reads** — rankings global board; team rosters read via `org_player_links`; registration accepts a global player.
5. **Identity consolidation (optional, gated)** — a *claim & merge* tool that collapses duplicate per-user profiles, repointing FKs, behind an admin-reviewed, reversible process.
6. **Decouple ownership** — change `fk_players_organization` CASCADE→SET NULL, then drop `players.organization_id` once nothing reads it.

Each phase is independently shippable and reversible (Section 13).

---

## 4. Backward compatibility strategy

- **Dual route surface:** keep `/api/v1/organizations/{slug}/players` (org roster view) alongside the new `/api/v1/players/{id}` (global) and `/api/v1/me/player` (self). The org-scoped list becomes "players this org rosters" (via `org_player_links`) rather than "players this org owns."
- **Token compatibility:** existing tokens lack a `scope` claim. Treat `scope` absent as derived: `organization_id != ""` → `org`; `organization_id == "" && role == "onboarding"` → `onboarding`; `organization_id == "" && role == ""` → `platform`. New tokens always set `scope` explicitly. This preserves the documented invariant *"empty org_id is NOT sufficient for platform admin"* during rollout.
- **`IsPlatformUser()`** must be updated to `scope == "platform"` (not "empty org"), because `player` tokens are also empty-org. This is the single most security-sensitive code change and is called out in Risks.
- **Unclaimed players:** existing `user_id IS NULL` rows remain valid global profiles; nothing forces a user account. The closed-ecosystem rule applies to *rankings*, not to *profile existence*.
- **API DTOs:** add fields (`user_id` always present on player responses, `visibility`, roster context) rather than removing; `organization_id` stays in responses through the dual period.

---

## 5. Data migration plan

### 5.1 Inventory before touching anything
- Count `players` rows total, with/without `user_id`.
- Count distinct `user_id` values having **>1** player row (the duplicate-identity population — the migration's real risk surface).
- Count FK references per table (`team_memberships`, `tournament_registrations`, `matches`, `match_events`, `player_tournament_stats`) to size the consolidation blast radius.

### 5.2 Phase-1 backfill (no identity change, fully reversible)
```sql
-- one roster link per existing org-owned player
INSERT INTO org_player_links (organization_id, player_id, status)
SELECT organization_id, id, 'active' FROM players
WHERE organization_id IS NOT NULL
ON CONFLICT (organization_id, player_id) DO NOTHING;
```
At this point `organization_id` still exists and still owns; `org_player_links` is a redundant mirror. Reads can begin migrating to it safely.

### 5.3 Identity consolidation (Phase GP-7, optional & gated)
The dangerous part: when one human has N player rows (one per org), do we merge them into one global identity? Two policies:

- **Conservative (default):** **do not auto-merge.** Each existing player row becomes its own global profile. A user with two historical org-players sees two profiles and can *request a merge* (reviewed). Rankings for unmerged profiles stay separate. Zero data-loss risk; identity fragmentation persists for legacy data only.
- **Aggressive (opt-in, per-tenant):** auto-merge rows sharing a `user_id`. Requires choosing a **survivor** profile, repointing every `player_id` FK (`team_memberships`, `tournament_registrations` — watch the partial-unique indexes! a merge can create duplicate `(tournament_id, player_id)`), `matches.*_player_id`, `winner_player_id`, `match_events.player_id`, `player_tournament_stats` (watch `uq_player_tournament_stats (player_id, tournament_id)`), then soft-tombstone the losers. **Must run inside one transaction per merged identity, with a recorded `player_merges` audit row enabling reversal.**

> **Hard constraint:** any merge that would violate `uq_treg_tournament_player` or `uq_player_tournament_stats` (same person registered/ranked twice in one tournament via two profiles) must be resolved by a documented dedup rule (keep the most-progressed registration / sum-or-keep-best stats) — never by silently dropping rows. This rule must be specified before GP-7 runs.

### 5.4 Decouple ownership (Phase GP-8)
```sql
ALTER TABLE players DROP CONSTRAINT fk_players_organization;
ALTER TABLE players ADD CONSTRAINT fk_players_origin_org
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE SET NULL;
-- later, once no reader uses it:
ALTER TABLE players DROP COLUMN organization_id;
```

---

## 6. API contract changes

| Area | Today | Target |
|---|---|---|
| Player identity | `POST/GET/PATCH/DELETE /organizations/{slug}/players[/{id}]` (org-owned) | **Self:** `GET/PATCH /api/v1/me/player`, `POST /api/v1/me/player` (create my profile). **Global read:** `GET /api/v1/players/{id}`. **Org roster:** `GET /organizations/{slug}/players` now lists *rostered* players; `POST .../players/{id}/roster` invites a global player. |
| Player create | org admin creates org-owned player | user creates own profile (persona=player); org admin may create an **unclaimed** profile + roster it. |
| Registration (individual) | org admin `POST .../registrations` with org player, gated `tournament.update` | add **self-registration**: player `POST /tournaments/{id}/register` for *their own* profile; org-admin path retained. `registrations.organization_id` nullable for self-registered individuals. |
| Rankings | `GET /organizations/{slug}/rankings/players` (per-org) | add `GET /api/v1/rankings/players` (global). Org board becomes a filter. |
| Auth `/me` | returns role + org | add `scope`, `player_profile_id` (nullable). |
| Login | resolves single org / onboarding | adds **player persona** outcome (see §11). |

All new endpoints are additive; existing ones keep responding through the dual period (Section 4).

---

## 7. Authorization changes

1. **Add explicit `scope` claim** to JWT (`org|platform|player|onboarding`); stop inferring privilege from empty `organization_id`.
2. **Update `IsPlatformUser()`** to `scope == "platform"`. *(Highest-risk change — see Risks R1.)*
3. **New self-ownership rule** in the players domain: edit allowed if `players.user_id == actor.user_id` (persona=player) OR actor is platform admin. Org admins get a *narrower* grant: roster management + per-team metadata, not global identity fields.
4. **New permissions:** `player.profile.create` (self), `roster.invite` / `roster.remove` (org admin over `org_player_links`), `tournament.self_register` (player). Seed via migration, mirror in frontend `ROLE_PERMISSIONS` matrix (UI-gating only).
5. **`RequireOrgScope()` stays** on all 11 org-admin trees. New player-self routes are mounted **outside** org scope and must apply a new `RequirePlayerScope()` guard (rejects `org`/`onboarding` tokens, allows `player`/`platform`).
6. **Onboarding token** gains `player.profile.create` (so a brand-new user can create a profile without an org), mirroring the existing narrow `organization.create` carve-out.

---

## 8. Ranking ownership changes

- Rankings move from **org-attributed** to **player-owned, globally aggregated** — consistent with the closed-ecosystem decision (all rankings derived from PlayArena activity, every player starts equal).
- Keep `player_tournament_stats` as the per-tournament fact table; keep its `organization_id` (host attribution) for org dashboards.
- New global query: drop `WHERE pts.organization_id = $1`, group by `player_id` across **all** tournaments. Tiebreak chain unchanged.
- Org board (`/organizations/{slug}/rankings`) = same query **with** the org filter — a view, not a separate source of truth.
- After GP-7 merges, a merged player's stats unify automatically (rows repointed to survivor) — the global board then reflects one identity.
- No external seeding, no imports, no federation — already true; this change only *broadens the aggregation key* from (org, player) to (player).

---

## 9. Team membership implications

- Teams stay org-owned. `team_memberships.player_id` now references a **global** profile, enabling a team to roster a player from outside the org.
- **Consent boundary (new):** because orgs no longer own players, adding a global player to a team should be an **invite the player accepts**, not a unilateral write — otherwise any org could attach any person to a team. Introduce membership `status='invited'` → player accepts → `active`. (For *unclaimed* `user_id IS NULL` profiles, the org that created them may roster directly; there is no one to consent.)
- `org_player_links` is the gate: a team may only roster a player the org has an `active` link to (or an accepted invite). This keeps "org uses player" explicit and auditable.
- Migration: existing memberships are grandfathered to `active` (already consented by historical fact).

## 10. Tournament registration implications

- **Individual tournaments:** the entrant is a **global player**. Two registration paths:
  - *Self-registration* (new): player registers their own profile (`tournament.self_register`); `registrations.organization_id` nullable / set to the player's origin or null.
  - *Org-managed* (existing): org admin registers a rostered player (gated `tournament.update`), now resolving the player **globally** rather than via `GetPlayerByID(.., orgID)`.
- **Team tournaments:** unchanged in shape (team is org-owned); but team rosters are now global players (§9).
- `uq_treg_tournament_player` continues to prevent double-entry — and becomes *more* meaningful (one global identity, not one per org). The GP-7 merge rule (§5.3) must respect it.
- Approval workflow (`tournament.update`) stays with the host org regardless of who registered.

## 11. Frontend onboarding implications

- **Persona split at onboarding.** `use-auth-guard` currently routes `onboarding` → `/onboarding` (create org). Replace with a **chooser**: *"Continue as Player"* → create global `PlayerProfile` → land on a new **player home** (`/me` or `/players/{id}`); *"Create an Organization"* → existing org-create flow.
- **New non-org route group.** Add `app/(player)/...` (or `app/(app)/me/...`) outside `[orgSlug]` for the global profile, player rankings, match history, and tournament discovery/self-registration. Today **every** authenticated route requires an `orgSlug`; players need a home that has none.
- **Auth store / token:** persist `scope` and `player_profile_id`; player-persona sessions must not be force-redirected into org selection. The existing `attemptTokenRefresh` org-id propagation must learn the `player` scope (refresh without an org).
- **Query keys:** add a non-org-scoped `meKeys` / `playerGlobalKeys` factory; player data must survive org switches (don't `queryClient.clear()` global profile data on org change).
- **SSE:** player-persona users need a non-org stream (or the hub must accept a `player` subscription keyed by `userID` only). Today the stream 403s empty-org tokens.

---

## 12. Risks

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | **Auth shape overload.** Adding `player` (empty-org) tokens makes `organization_id == ""` mean platform OR onboarding OR player. A mistake re-grants platform-admin to players — a privilege-escalation class bug. (Directly touches the documented security invariant.) | **Critical** | Introduce explicit `scope` claim; rewrite `IsPlatformUser()` to `scope=="platform"`; add `RequirePlayerScope()`; add integration tests asserting a `player` token gets 403 on every org-admin and platform-admin route (mirror the existing MT-1 platform-admin gate tests). |
| R2 | **Identity merge data loss / FK corruption** during GP-7 (unique-index collisions on registrations & stats). | High | Default to **no auto-merge**; gate aggressive merge per-tenant; transactional per-identity merge with `player_merges` audit + reversal; pre-flight the dedup rule for `uq_treg_tournament_player` / `uq_player_tournament_stats`. |
| R3 | **BOLA regression.** `assertOrgOwnership` and `RequireOrgScope` are the current tenant guards; new self-routes bypass org scope and need their own ownership check (`user_id == actor`). | High | New `RequirePlayerScope` + service-layer `user_id` ownership assert; reuse the existing BOLA integration-test patterns (Org A actor on Org B / other-user resource → 403). |
| R4 | **Cross-org rostering without consent** — orgs attaching arbitrary people to teams once ownership is gone. | Medium | Invite/accept on `team_memberships` + `org_player_links`; unilateral writes only for `user_id IS NULL` unclaimed profiles. |
| R5 | **Ranking semantics shift** surprises existing org dashboards (numbers change when aggregation broadens). | Medium | Keep org-filtered board as a view; document the global vs org distinction; recompute is idempotent (stats fact table unchanged). |
| R6 | **Dual-surface drift** — org-scoped and global player endpoints diverge during the long migration. | Medium | Single service layer behind both routes; contract tests on both surfaces. |
| R7 | **Privacy.** Global profiles are visible across orgs by default; closed ecosystem ≠ public-by-default. | Medium | `visibility` column (public/unlisted); default decided by product before GP-3 ships. |
| R8 | **`ON DELETE CASCADE` removal** — after decoupling, deleting an org must not orphan match history; but players surviving org deletion is the *point*. | Low | CASCADE→SET NULL on origin org; verify match/stat history resolves with null origin org. |

---

## 13. Rollback strategy

- **Per-phase reversibility.** Every schema migration ships with a tested `.down.sql` (repo convention, `000001`–`000027`). Phases GP-1..GP-6 are purely additive — down-migrations drop new tables/columns/indexes with no data loss to legacy paths.
- **Feature flags.** New surfaces (player persona login, self-registration, global rankings, player home) sit behind config flags; disabling them reverts behavior to org-first without a redeploy of schema.
- **Token dual-read** (Section 4) means old tokens keep working if the `player` persona is switched off mid-rollout.
- **GP-7 (merge) is the only destructive phase** and is gated, off by default, transactional, and audited via `player_merges` for row-level reversal. Do not run GP-7 until GP-1..GP-6 have soaked in production.
- **GP-8 (drop `organization_id`) is the point of no easy return** — run it only after telemetry confirms no reader references the column and after a full backup. Keep the column (nullable, SET NULL) for one full release before dropping.

---

## Phased implementation order

> Sequencing rule: **additive schema → new read surfaces → new write surfaces → re-base reads → (optional) consolidate identity → remove ownership.** Each phase is independently shippable, reversible, and leaves the system green.

### Phase GP-1 — Additive schema & token scope (no behavior change)
- Migration: `organization_id` nullable; `uq_players_user_id WHERE user_id IS NOT NULL`; `players.visibility`; `org_player_links`; backfill links (§5.2).
- Add `scope` to `JWTClaims` (issued, not yet enforced); `scope`-absent inference (§4).
- **Exit:** schema deployed, all existing tests green, no API change. Fully reversible.

### Phase GP-2 — Auth persona foundation
- Rewrite `IsPlatformUser()` → `scope=="platform"`; add `RequirePlayerScope()`; keep `RequireOrgScope()` everywhere it is today.
- Add the **player persona** login outcome + onboarding `player.profile.create` carve-out.
- Integration tests: player token → 403 on all org-admin + platform routes (R1).
- **Exit:** personas exist and are provably sandboxed; no user-visible feature yet.

### Phase GP-3 — Global player profile surface (read + self-edit)
- New routes: `POST/GET/PATCH /api/v1/me/player`, `GET /api/v1/players/{id}`.
- Service: self-ownership rule (`user_id == actor`); profile create for persona=player.
- Org-scoped `players` routes unchanged (dual surface).
- **Exit:** a user can create and own a global profile without an org.

### Phase GP-4 — Frontend player home & onboarding split
- Persona chooser in onboarding; `app/(player)` route group; player home, profile editor, global rankings/history views; non-org query keys; player-scope refresh; non-org SSE.
- **Exit:** a player-first user has a complete, org-free experience.

### Phase GP-5 — Global rankings
- New global ranking queries/routes (drop org filter); org board reframed as a filtered view.
- **Exit:** closed-ecosystem rankings are player-owned and global.

### Phase GP-6 — Re-based rostering & registration
- `org_player_links` invite/accept; team rostering reads links + consent; team_memberships reference global players.
- Self-registration path; org-managed registration resolves players globally (drop `GetPlayerByID(.., orgID)` org constraint).
- **Exit:** orgs *use* global players for teams and tournaments; players self-register.

### Phase GP-7 — Identity consolidation (optional, gated, per-tenant)
- `claim & merge` tooling; `player_merges` audit; transactional FK repoint; dedup rules for `uq_treg_tournament_player` / `uq_player_tournament_stats` (§5.3).
- Off by default; conservative (no auto-merge) is the standing behavior.
- **Exit:** duplicate legacy identities can be unified reversibly.

### Phase GP-8 — Remove organization ownership
- `fk_players_organization` CASCADE→SET NULL; then drop `players.organization_id` after one full release of dual-read confirms no readers remain.
- **Exit:** *PlayerProfile is global; organizations use players; organizations do not own players.* Invariant achieved.

---

## Appendix A — Files/areas touched per phase (orientation, not exhaustive)

- **Schema/queries:** `db/migrations/000028+`, `db/queries/players.sql`, `rankings.sql`, `tournament_registrations.sql`, `team_memberships.sql`, new `org_player_links.sql`.
- **Auth:** `internal/auth/{model,middleware,authorization,service}.go` (scope claim, `IsPlatformUser`, `RequirePlayerScope`).
- **Players:** `internal/players/{routes,service,repository}.go` + new `me`/global handlers.
- **Rankings:** `internal/rankings/*` (global query + route).
- **Registrations:** `internal/tournament_registrations/service.go` (`registerPlayer`, self-register).
- **Teams:** `internal/teams/*` (roster invite/accept via `org_player_links`).
- **Bootstrap:** `internal/bootstrap/{modules,router}.go` (mount global/me trees, `RequirePlayerScope`).
- **Frontend:** new `app/(player)/...`, onboarding chooser, `use-auth-guard`, `auth.store`, `query-keys`, `client.ts` refresh, SSE hook.

## Appendix B — Open product questions to resolve before build
1. **Profile visibility default** (R7): public across the ecosystem, or unlisted until the player opts in?
2. **Merge policy** (R2/GP-7): conservative (no auto-merge) platform-wide, or per-tenant aggressive?
3. **Unclaimed players:** can orgs still create `user_id IS NULL` profiles indefinitely, or only during a grandfather window?
4. **Self-registration eligibility:** open to any player, or only players an org has rostered / invited?
5. **One profile per user, hard:** is `uq_players_user_id` acceptable, or must a user be allowed multiple personas (e.g. different sports)? (The current schema comment assumed multiple; the new product model assumes one.)
```
