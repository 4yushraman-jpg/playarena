# GP Implementation Blueprint — Kabaddi Platform (Source of Truth)

**Status:** Approved blueprint. Design only — no code, no migrations, no implementation.
**Date:** 2026-06-12.
**Supersedes:** [global-player-identity.md](global-player-identity.md) and [decision-user-player-cardinality.md](decision-user-player-cardinality.md). Where they conflict with this document, **this document wins.**

## Finalized product axioms (binding on everything below)

1. **Kabaddi-only.** No `player_sports`, no `sport_ratings`, no `player × sport` keys, no multi-sport abstraction. Every player is a Kabaddi player implicitly. Drop the `sport` dimension entirely from rankings/stats design.
2. **One User → one PlayerProfile** (`player_profiles.user_id UNIQUE NOT NULL`). The profile is the canonical player identity.
3. **PlayerProfile is global and user-owned.** May exist with zero orgs; may affiliate with many orgs over time; owns its reputation, history, achievements. Organizations *use* players; they do not own them.
4. **Closed ecosystem.** Reputation is earned only from PlayArena activity. No external ratings, federation imports, seeded ratings, **claim-profile workflows**, or imported reputation. All players begin equal.
5. **Two personas, one account.** A `User` may be a Player and/or an Organizer. Persona is a *mode of the active token*, not a separate account.

> Consequence of #4: the previous roadmap's **GP-7 "claim & merge"** phase is **deleted.** No machinery exists to claim or merge identities. Legacy org-created playerless rows are handled by a one-way, no-claim policy (Section 7).

---

## 1. Revised domain model

```
User (account / global identity)
│  email, username (globally unique), password, status, verification
│
├── 1 ──── 0..1 ── PlayerProfile         (the player persona; user_owned, global)
│                   │  display_name, bio, avatar, physical attrs, nationality, dob,
│                   │  visibility, reputation_points (derived), created_at
│                   │
│                   ├── team_memberships (0..N over time)  ──► Team
│                   ├── tournament_registrations (individual)──► Tournament
│                   ├── match participation (home/away/winner)─► Match
│                   ├── achievements (0..N)                     (earned, idempotent)
│                   └── player_tournament_stats (per finished tournament; immutable fact)
│
└── 0..N ── user_organization_roles  ──► Organization   (the organizer persona; RBAC grants)

Organization (organizer-owned container)
│  name, slug (global-unique), status, verified (ranked-sanction flag)
│
├── Teams (0..N, org-owned)
│      └── team_memberships  ──► PlayerProfile   (invite/accept; jersey, role, status, joined/left)
│
└── Tournaments (0..N, org-owned)
       │  participant_type: team | individual
       │  status, ranked_eligible (derived), registration window, capacity
       │
       ├── tournament_registrations ──► (Team)  | (PlayerProfile)   (exactly one)
       ├── Matches ──► (Team vs Team) | (PlayerProfile vs PlayerProfile)
       │      └── match_events (append-only source of truth)
       └── team_tournament_stats / player_tournament_stats  (immutable per-tournament facts)

Ranking (derived, global, single Kabaddi ladder)
   PlayerLeaderboard  = fold over player_tournament_stats (ALL orgs, ranked-eligible only)
   TeamLeaderboard    = fold over team_tournament_stats   (ALL orgs, ranked-eligible only)
```

### Relationship summary

| Relationship | Cardinality | Owner | Notes |
|---|---|---|---|
| User → PlayerProfile | 1 → 0..1 | User | `user_id UNIQUE NOT NULL`; created on demand, never claimed/merged |
| User → Organization (roles) | 1 → 0..N | join (`user_organization_roles`) | organizer persona; existing RBAC |
| Organization → Team | 1 → 0..N | Organization | org-owned, CASCADE retained |
| Organization → Tournament | 1 → 0..N | Organization | org-owned, CASCADE retained |
| Team ↔ PlayerProfile | M ↔ N over time | join (`team_memberships`) | invite/accept; history preserved; **org affiliation is derived from this** |
| Tournament ↔ Team | M ↔ N | `tournament_registrations` (team) | team tournaments |
| Tournament ↔ PlayerProfile | M ↔ N | `tournament_registrations` (player) | individual tournaments |
| Match → participants | team-vs-team OR player-vs-player | Tournament | global player/team refs |
| PlayerProfile → reputation/history/achievements | owned | PlayerProfile | global, closed-ecosystem |

### Deliberate simplifications vs. the prior blueprint

- **No `org_player_links` table.** Org affiliation is *derived* — a player is affiliated with an org iff they hold an active `team_memberships` row on one of that org's teams. Kabaddi is team-centric; an explicit org-roster table would be a redundant second affiliation surface. (If product later needs "signed but unassigned" players, add it then.)
- **No sport columns anywhere.** Ranking key is `player`, not `player × sport`.
- **No claim/merge subsystem.** Identity is created once, by its owner.

---

## 2. Revised auth model

Today the token overloads `organization_id == ""` to mean "platform admin OR onboarding." The revised model makes the **active mode explicit** with a `scope` claim. A single human may switch modes; switching re-issues a token (the existing refresh-with-org mechanism generalized).

```
JWTClaims {
  user_id, email,
  scope: "player" | "organizer" | "onboarding" | "platform",   // explicit
  organization_id   (present & non-empty ONLY when scope == "organizer"),
  org_role          (present ONLY when scope == "organizer"),
  player_profile_id (present when scope == "player" and a profile exists)
}
```

`IsPlatformUser()` becomes `scope == "platform"` — **never** "empty org." This closes the privilege-escalation surface that the player scope (also empty-org) would otherwise open.

### Scope definitions and exact permissions

**`player` scope** — the user acting as themselves.
- Allowed: read/update **their own** `PlayerProfile` (`profile.user_id == user_id`); set visibility; upload own avatar.
- Allowed: accept/decline team invites; request to join a team; leave a team.
- Allowed: self-register / self-withdraw for **individual** ranked or unranked tournaments (their own profile only).
- Allowed: read global leaderboards, their own match/tournament/team history, their own achievements; browse public profiles, orgs, teams, tournaments.
- Denied: anything org-administrative (create teams/tournaments, approve registrations, grant roles, manage members, webhooks, media for other entities). Mounted behind a new `RequirePlayerScope()` that rejects `organizer`/`onboarding` tokens.
- BOLA rule: service-layer assert `profile.user_id == actor.user_id` (platform may override). No org check applies — player routes live **outside** `/organizations/{slug}`.

**`organizer` scope** — the user acting inside one organization (current behavior, unchanged).
- Carries `organization_id` + org `role`; all existing org-scoped RBAC (`player.*` roster perms, `team.*`, `tournament.*`, `match.*`, `role.assign`, `webhook.*`, `media.*`) evaluated against `user_organization_roles`.
- `RequireOrgScope()` continues to guard **all** org-admin trees (rejects player/onboarding tokens). This is the standing security invariant — do not regress.
- Note: org `player.create/update/delete` permissions now govern **roster/registration actions on global players the org is entitled to** (members of its teams / its registrations), **never** edits to a player's global identity fields.

**`onboarding` scope** — a fresh, verified user with neither a profile nor an org, mid-setup.
- Allowed: exactly two terminal actions — `POST /me/player` (create own profile) **or** `POST /organizations` (create org). Both via narrow, DB-verified carve-outs (mirrors the current `organization.create` + `IsZeroOrgUser` pattern; add an analogous `profile.create` + `HasNoPlayerProfile` check).
- Denied: everything else. The token is single-use in spirit: once a profile or org exists, the next login resolves to `player`/`organizer`.

**`platform` scope** — super-admin.
- Platform-wide grants via `user_organization_roles` with `organization_id IS NULL` (existing). Full cross-tenant authority, audited. Only this scope is "platform admin."

### Scope/permission matrix (summary)

| Capability | player | organizer | onboarding | platform |
|---|---|---|---|---|
| Edit own PlayerProfile | ✅ (self) | ❌ | create-only | ✅ |
| Create org | via onboarding only* | ❌ | ✅ | ✅ |
| Manage org teams/tournaments/members | ❌ | ✅ (RBAC) | ❌ | ✅ |
| Invite player to team | ❌ | ✅ | ❌ | ✅ |
| Accept team invite | ✅ (self) | ❌ | ❌ | ✅ |
| Self-register (individual tournament) | ✅ (self) | ❌ | ❌ | ✅ |
| Register a team for a tournament | ❌ | ✅ | ❌ | ✅ |
| Approve registrations | ❌ | ✅ | ❌ | ✅ |
| Read global leaderboard/history | ✅ | ✅ | ❌ | ✅ |
| Platform admin actions | ❌ | ❌ | ❌ | ✅ |

\* A player who wants to also organize obtains an `organizer` token by **creating/joining an org** (re-scoped session), not by escalating their player token.

---

## 3. Revised onboarding model

```
Register ──► Verify email ──► [Persona chooser]
                                   │
              ┌────────────────────┴─────────────────────┐
        "I'm a Player"                              "I'm an Organizer"
              │                                            │
   POST /me/player (create profile)            POST /organizations (create org)
   → re-scope token to `player`                → re-scope token to `organizer`
   → enter Player Home (/me)                   → enter Org Home (/{slug})
```

- **Login resolution (revised contract):**
  - has PlayerProfile + 0 orgs → `player` token, land on Player Home.
  - has 0 profile + 1 org → `organizer` token (auto), land on org.
  - has 0 profile + N orgs → 409 `organization_required` + org list (existing multi-org flow), `organizer` after selection.
  - has profile **and** org(s) → default to **last-used persona** (persisted), with an in-app **persona switcher**.
  - has neither → `onboarding` token → persona chooser.
  - platform role → `platform` token.

- **Persona switching** generalizes today's org switch:
  - Player → Organizer: pick org (or create one) → `POST /auth/refresh` with `{scope: organizer, organization_id}` → new token → `queryClient.clear()` for org-scoped data (keep global player data cached).
  - Organizer → Player: `POST /auth/refresh` with `{scope: player}` → player token → navigate to `/me`.
  - The refresh endpoint validates the user is *entitled* to the requested scope (has the profile / has a grant in that org) before minting.
- A user **without** a profile who is an organizer can later create one (the "Become a player" action triggers the player onboarding sub-flow); symmetrically a player can create/join an org. Neither requires a second account.

---

## 4. Ranking architecture (Kabaddi-only, closed ecosystem)

### 4.1 Principles
- **Source of truth = matches.** `match_events` (append-only) → match result → per-tournament standings → immutable `*_tournament_stats` snapshot at tournament completion (existing pipeline; drop org as a *ranking* key, keep `tournament_id`).
- **Pure, recomputable fold.** A player's rating is a deterministic function over their immutable per-tournament facts. Re-running after a rule change is idempotent and auditable — essential when there is no external anchor.
- **Begin equal.** Every profile starts at 0 reputation; rating is *earned*, never seeded.

### 4.2 Player rating — Reputation Points (RP), not Elo
Recommended model: **cumulative, placement-weighted Reputation Points** with anti-farm shaping. Rejected alternative: Elo/Glicko — zero-sum skill ratings reward *seal-clubbing* and are highly smurf-exploitable, and they obscure auditability. RP is transparent, monotone-friendly, and farm-resistant.

Per **ranked-eligible** tournament a player earns:
```
RP_tournament =
      base_participation
    + placement_bonus(position, field_size)        // 1st > 2nd > podium > rest
    + per_match_wins * win_weight
    × opponent_strength_factor                      // beating stronger fields counts more
    × diminishing_returns(opponent_repetition)      // repeatedly farming the same/weak opponents decays toward 0
```
- `RP_total = Σ RP_tournament` over ranked-eligible tournaments (optionally within a **season** window for the live ladder; all-time retained).
- Walkovers/forfeits grant **no** RP (only schedule effects).
- Optional **inactivity decay** for the *live seasonal* ladder (not all-time), to keep the board current.
- Tiebreakers (reuse the existing chain): tournaments_won → podium_finishes → total_points → total_wins → matches_played.

### 4.3 `ranked_eligible` — the anti-abuse gate (added by adversarial review, Section 8)
Only tournaments flagged `ranked_eligible` contribute RP. A tournament is ranked-eligible iff **all** hold: hosted by a **verified** organization; ≥ `MIN_RANKED_PARTICIPANTS`; reached `completed` status; and not flagged by anti-abuse review. Unranked tournaments are fully supported (history, stats) but yield **0 RP**. New/unverified orgs run unranked until verified — this is the primary structural defense against fabricated-tournament rating farming.

### 4.4 Leaderboards
- **Single global Kabaddi player ladder** and **single global team ladder** (no per-sport split). Org-filtered and tournament-filtered views are *filters* over the same fold, never separate sources.
- Surfaces: all-time and current-season.

### 4.5 Achievements
Earned, idempotent badges keyed to `PlayerProfile` (and `Team`), derived from facts:
- Career milestones (matches played, tournaments entered), titles (tournament wins), podiums, win streaks, raid/tackle/MVP records derived from `match_events`.
- Stored as `(profile_id, achievement_key, awarded_at, context)` with `UNIQUE(profile_id, achievement_key[, context])` so re-derivation is idempotent. Closed-ecosystem only — no imported badges.

### 4.6 Player history
All keyed to the global profile: **match history** (via player FKs on `matches`), **tournament history** (registrations + final placement), **team history** (membership timeline with joined/left, transfers). One continuous career — the payoff of the 1:1 identity decision.

---

## 5. Team model

- **Team is org-owned** (`teams.organization_id`, CASCADE retained). Players are global.
- **Joining is bilateral (consent on both sides):**
  - *Org recruits:* organizer searches players (by username/profile), sends an **invite** → `team_memberships(status='invited')`. Player **accepts** → `active`, or **declines** → `declined`.
  - *Player applies:* player requests to join a team → pending request → organizer **approves** → `active`.
  - Either way, an `active` membership requires **affirmative action by the player** (accept or initiate). No org can unilaterally attach a global player. (This replaces the old org-ownership write.)
- **Membership record** (evolve existing `team_memberships`): `team_id`, `player_id` (global profile), `role` (player/captain/vice_captain), `jersey_number` (per-team override), `status` (invited/active/left/transferred/released/declined), `joined_at`, `left_at`, `notes`. No `UNIQUE(team_id, player_id)` — rejoining creates a new row; full history preserved (existing behavior).
- **Multiple concurrent teams** are allowed (a player may be on teams in different orgs). Conflicts are resolved at **tournament eligibility** (Section 6), not by forbidding membership.
- **Transfer/history:** a transfer = close current membership (`left_at`, status `transferred`) + open a new one on the new team. The timeline is the player's team history; nothing is deleted. Org affiliation history is the union of team histories.

---

## 6. Tournament participation model

- **`participant_type` retained:** `team` and `individual`. (Both exist in Kabaddi: club/team events and individual skill events.) No sport field needed.
- **Team tournaments:** the **Team** registers; performed by an organizer with `tournament.update`/registration permission on the host org or the team's org. The team's **eligible roster** = `active` members at **registration close** (roster lock).
- **Individual tournaments:** the **PlayerProfile** registers. Two paths:
  - *Self-registration* (player scope) — the player enters their own profile.
  - *Org-managed* — an organizer registers a player **only if** that player is an `active` member of one of the org's teams (consent already given) — never an arbitrary global player.
- **Registration ownership / actions:**
  - Create/withdraw: self (individual self-reg) or the registering organizer (team / org-managed).
  - Approve/reject/seed: the **host org** (`tournament.update`), regardless of who registered. Approval authority always sits with the tournament host.
  - `tournament_registrations.organization_id` (registrant org) becomes **nullable** — null for player self-registrations; set for org-managed/team registrations.
- **Eligibility rules:**
  - `uq_treg_tournament_player` / `uq_treg_tournament_team` — one entry per identity per tournament (now meaningful because identity is singular).
  - A player may not appear for **two different teams in the same tournament**, nor as both an individual and a team member where the format forbids it — enforced at registration time against the tournament's existing entries.
  - Profile must be active and (for ranked tournaments) the registrant/team must satisfy the host's rules; roster locked at registration close.
  - Ranked-eligibility of the tournament (Section 4.3) does not affect *who may enter*, only whether the result yields RP.

---

## 7. Migration strategy — revised GP phases

Re-evaluated against the Kabaddi axioms. **Removed:** the sport dimension (was a GP-5 add-on) and the entire claim/merge phase (old GP-7). **Net phases: GP-1 … GP-7.** Strangler-fig, additive-first, ownership removed last. Every schema migration ships a tested `.down.sql` (repo convention).

> **Legacy-data policy (no-claim):** existing `players` rows are reinterpreted as follows. Rows **with** `user_id` → become that user's single `PlayerProfile` (if a user somehow has multiple, keep the earliest as canonical and mark the rest `archived`, non-ranking — **no claim/merge UX**, a one-time data decision). Rows **without** `user_id` (org-created historical/scouted) → become **unclaimed roster entries**: usable for historical record and team history, **never ranked**, and **never** auto-attached to a real user. New profiles are created only by their owner. This honors "no claim-profile workflows" while preserving history.

### GP-1 — Identity foundation (additive, no behavior change)
- **Objective:** make PlayerProfile a global, user-keyed identity without removing org coupling yet.
- **Schema:** add `UNIQUE(user_id) WHERE user_id IS NOT NULL`; add `visibility`; make `players.organization_id` **nullable** (still populated, still CASCADE for now). Add `scope` to issued JWTs (not enforced). Backfill: archive any extra same-user rows per legacy policy.
- **API:** none (claims emitted, not enforced).
- **Frontend:** none.
- **Risks:** unique-index violation on legacy duplicates → resolve via the one-time archive backfill **before** adding the index. **Rollback:** drop index/column/claim; fully reversible.

### GP-2 — Auth scopes (un-overload the token)
- **Objective:** explicit `player/organizer/onboarding/platform` scopes; eliminate empty-org ambiguity.
- **Schema:** none.
- **API:** `IsPlatformUser()` → `scope=="platform"`; add `RequirePlayerScope()`; keep `RequireOrgScope()` on all org trees; refresh endpoint validates requested scope entitlement. Back-compat: infer scope for legacy tokens (org set→organizer; onboarding role→onboarding; else platform).
- **Frontend:** persist `scope`; refresh sends desired scope.
- **Risks (Critical):** privilege escalation if a player/onboarding token is mistaken for platform. **Mitigation/tests:** assert player & onboarding tokens get 403 on every org-admin and platform route (mirror existing MT-1 gate tests). **Rollback:** feature-flag scope enforcement; revert to legacy inference.

### GP-3 — Player self-service surface
- **Objective:** users own and edit their global profile without an org.
- **Schema:** none beyond GP-1.
- **API:** `POST/GET/PATCH /api/v1/me/player`; `GET /api/v1/players/{id}` (global, visibility-aware). Service: self-ownership assert (`user_id == actor`). Onboarding carve-out `profile.create` + `HasNoPlayerProfile`.
- **Frontend:** none yet (API-only) — or a minimal profile editor.
- **Risks:** BOLA on self routes. **Mitigation:** service-layer ownership check + integration tests (other-user profile edit → 403). **Rollback:** unmount routes (flag).

### GP-4 — Frontend personas & player home
- **Objective:** complete org-free player experience + persona switch.
- **Schema:** none.
- **API:** none beyond GP-3.
- **Frontend:** persona chooser at onboarding; new `app/(player)` route group (player home, profile editor, history, leaderboard); persona switcher; non-org query keys (`meKeys`) that survive org switches; player-scope refresh; non-org SSE (hub accepts a `player` subscription keyed by `user_id`).
- **Risks:** persona confusion / stuck users; SSE 403 for player tokens. **Mitigation:** explicit switcher + last-persona memory; hub player-subscription path. **Rollback:** hide player routes behind flag; org flow untouched.

### GP-5 — Global Kabaddi rankings, achievements, history
- **Objective:** player-owned global ranking + reputation + achievements + history; introduce `ranked_eligible`.
- **Schema:** drop org from the *ranking key* (keep `organization_id` on stats as host-attribution/filter only); add `tournaments.ranked_eligible` (derived) and `organizations.verified`; add `achievements` table (`UNIQUE(profile_id, achievement_key[, context])`); optional `season` dimension on the live ladder. Backfill `ranked_eligible` from completion + verification + min-participants.
- **API:** `GET /api/v1/rankings/players` & `/teams` (global); achievement & history reads on `/me` and `/players/{id}`. Org board reframed as a filter.
- **Frontend:** leaderboard, profile reputation/achievements/history views.
- **Risks:** rating semantics shift; farm vectors. **Mitigation:** `ranked_eligible` gate + RP anti-farm shaping (Section 4); recompute is idempotent. **Rollback:** keep org board; flag global ladder.

### GP-6 — Bilateral rostering & self-registration
- **Objective:** consent-based team membership; players self-register; org resolves global players.
- **Schema:** extend `team_memberships.status` (invited/declined); make `tournament_registrations.organization_id` nullable.
- **API:** invite/accept/apply/approve on memberships; self-register/self-withdraw on individual tournaments; org-managed registration restricted to players on the org's teams; drop the `GetPlayerByID(.., orgID)` org constraint in favor of global resolution + eligibility checks.
- **Frontend:** invite UI (organizer), invites/requests inbox (player), self-register flow.
- **Risks:** non-consensual rostering / arbitrary-player registration (R4). **Mitigation:** affirmative-action requirement + eligibility checks; grandfather existing memberships to `active`. **Rollback:** flag self-reg and invite flows; legacy org-managed path remains.

### GP-7 — Remove organization ownership of players (final, point-of-no-easy-return)
- **Objective:** achieve the invariant — orgs use, don't own, players.
- **Schema:** `fk_players_organization` CASCADE → SET NULL (reinterpret column as nullable "origin org"); after one full release with no readers, **drop `players.organization_id`**. Verify match/stat/team history resolves with null origin org.
- **API:** remove residual org-scoping from player reads; org "players" list = derived team-roster view.
- **Frontend:** org player views read rosters, not owned players.
- **Risks:** orphaned history on org deletion (the *intended* outcome — players survive); accidental data loss on column drop. **Mitigation:** SET NULL first, soak one release, full backup before drop. **Rollback:** restore column from backup; re-point. Treat as irreversible in practice — gate on telemetry proving zero reads.

---

## 8. Adversarial review (then design updates)

### 8.1 Findings

**Security**
- **S1 — Scope escalation.** Player/onboarding tokens share the empty-org shape; a missed check re-grants platform admin. *(Critical.)*
- **S2 — Persona-switch entitlement.** A user could request an `organizer` token for an org they don't belong to, or a `player` token without a profile, if refresh doesn't re-verify entitlement.
- **S3 — Onboarding carve-out abuse.** The narrow `profile.create`/`organization.create` exceptions could be reused to act beyond first-setup if not strictly DB-verified and single-shot.
- **S4 — Cross-tenant BOLA on new self routes.** `/me/player` and `/players/{id}` bypass org scope; without an ownership assert, one user edits another's profile.

**Identity**
- **I1 — Multi-account smurfing.** 1:1 user↔profile does **not** stop one human from registering several accounts (several emails) → several "fresh, equal" profiles. This is the structural limit of a closed ecosystem.
- **I2 — Account sharing / takeover.** A strong profile is a valuable target; shared/compromised accounts distort rankings.
- **I3 — Username squatting / impersonation** on public profiles (one canonical handle per human raises the value of the handle).
- **I4 — Legacy duplicate identities** (same user across orgs) under a no-claim policy — must be resolved by data decision, not UX.

**Ranking abuse**
- **R-A — Fabricated tournaments.** A colluding organizer spins up tournaments with fake/controlled entrants to farm RP for a target.
- **R-B — Match-throwing / collusion.** Real entrants deliberately lose to inflate a confederate.
- **R-C — Opponent farming / seal-clubbing.** Repeatedly beating weak or repeated opponents to grind RP.
- **R-D — Walkover/forfeit farming.** Harvesting wins without play.
- **R-E — Self-match / tiny-field farming.** 2-player "tournaments" run repeatedly.

**Smurfing**
- **SM1 — New-account seal-clubbing** in open/unranked events to build records.
- **SM2 — Rating reset evasion** — abandon a bad profile, start a new account (enabled by I1).

**Onboarding**
- **O1 — Persona dead-ends** (user stuck with no profile and no org, or unsure which to pick).
- **O2 — Email enumeration / verification bypass** on the register/verify path.
- **O3 — Bulk automated signups** feeding smurf farms (ties to I1/R-A).

**Migration**
- **M1 — Unique-index failure** on legacy duplicates at GP-1.
- **M2 — Ownership-drop data loss** at GP-7 (column drop / CASCADE change).
- **M3 — Dual-surface drift** between org-scoped and global player APIs during the long migration.
- **M4 — Unranked-legacy confusion** — historical org-created players appearing in or missing from boards unexpectedly.

### 8.2 Design updates (folded into the blueprint above)

- **Against S1/S4:** explicit `scope` claim; `IsPlatformUser()==scope=="platform"`; `RequirePlayerScope()`; service-layer `user_id` ownership assert on all self routes; mandated escalation tests (GP-2/GP-3). *(Sections 2, 7.)*
- **Against S2:** the refresh endpoint **must re-verify entitlement** against `user_organization_roles` (organizer) / profile existence (player) before minting a re-scoped token. Added as an explicit GP-2 requirement.
- **Against S3:** onboarding carve-outs stay DB-verified (`IsZeroOrgUser`, `HasNoPlayerProfile`) and become **no-ops the instant** a profile/org exists; covered by tests.
- **Against R-A/R-E:** introduced **`ranked_eligible`** + **organization `verified`** + `MIN_RANKED_PARTICIPANTS`; only completed, verified, sufficiently-large tournaments yield RP. New orgs are unranked until verified. *(Section 4.3 — the single most important anti-abuse control.)*
- **Against R-B/R-C/R-D:** RP shaping — `opponent_strength_factor`, `diminishing_returns(opponent_repetition)`, **no RP for walkovers/forfeits**, optional seasonal decay; plus an **anti-abuse review flag** that can un-rank a flagged tournament. *(Section 4.2–4.3.)*
- **Against I1/SM1/SM2:** acknowledge smurfing cannot be *eliminated* in a closed ecosystem, only made expensive. Controls: mandatory **email verification** (exists); RP only from **verified-org** competition (a smurf must beat real, ranked fields to gain reputation — clubbing fresh accounts yields nothing); **cumulative, non-resettable** RP (no rating to "reset," so abandoning a profile forfeits all progress — removes the incentive behind SM2); optional **phone/device verification** and **organizer-vouching** as future trust signals; anomaly monitoring on rapid RP gain. Documented as residual risk, not "solved."
- **Against I2/I3:** profile `visibility`; standard session-revocation on password change (exists); reserve/validate `username` uniqueness (exists) and add impersonation reporting to the platform-admin surface (future).
- **Against I4/M1/M4:** the **no-claim legacy policy** (Section 7): user-linked extras archived as non-ranking before the unique index; unclaimed org-created players are non-ranking historical records, never auto-linked. Boards show only real, user-owned, ranked-eligible profiles → no surprise legacy entries.
- **Against O1:** persona chooser + always-available "Become a player"/"Create an organization" actions + last-persona memory; no terminal dead-end.
- **Against O2/O3:** keep existing enumeration-timing equalization and rate limiting on auth; extend rate limiting / consider CAPTCHA on register to blunt bulk signups (ties to smurf cost).
- **Against M2:** GP-7 staged (SET NULL → soak one release → backup → drop), gated on telemetry proving zero reads; treated as practically irreversible.
- **Against M3:** single shared service layer behind both org-scoped and global player routes; contract tests on both surfaces for the dual period.

---

## Final approved blueprint — summary

- **Identity:** `User` 1—0..1 `PlayerProfile` (`user_id UNIQUE NOT NULL`), global, user-owned, created once by its owner, never claimed or merged. No sport dimension.
- **Auth:** explicit `scope` (`player`/`organizer`/`onboarding`/`platform`); platform-admin ⇔ `scope=="platform"` only; `RequirePlayerScope` for self routes, `RequireOrgScope` for org trees; refresh re-verifies scope entitlement.
- **Onboarding:** verified user → persona chooser → create profile (player) or org (organizer); seamless persona switching via re-scoped refresh; no dead-ends.
- **Rankings:** closed-ecosystem **Reputation Points** (not Elo), global single Kabaddi ladder, **`ranked_eligible` gated by verified orgs + min field + completion**, anti-farm shaping, idempotent recompute; achievements and history owned by the global profile.
- **Teams:** org-owned; **bilateral consent** (invite/accept or apply/approve) to roster a global player; full transfer history; org affiliation derived from team membership (no separate roster table).
- **Tournaments:** team and individual; **self-registration** for players, org-managed only for the org's own rostered players; host org approves; one-entry-per-identity eligibility.
- **Migration:** GP-1 identity foundation → GP-2 auth scopes → GP-3 self-service profile → GP-4 frontend personas → GP-5 global rankings → GP-6 bilateral rostering & self-registration → GP-7 remove org ownership. Additive-first, ownership removed last, no claim/merge phase.
- **Residual risk (accepted):** multi-account smurfing cannot be eliminated in a closed ecosystem; it is made economically pointless (RP only from verified competition; cumulative, non-resettable reputation) and monitored — not claimed solved.

This document is the source of truth for GP implementation.
```
