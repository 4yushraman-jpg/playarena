# Decision Record — User ↔ PlayerProfile Cardinality

**Status:** Proposed (decision recommended below). Design only — no code, no roadmap changes.
**Date:** 2026-06-12.
**Resolves:** Open question #5 in [global-player-identity.md](global-player-identity.md) Appendix B.
**Question:** Does one `User` own exactly **one** global `PlayerProfile` (Model A), or **many** (Model B)?

---

## 1. Context

The global-player-identity migration promotes `users` to carry a global `PlayerProfile`. One cardinality question is unresolved. The legacy schema comment on `players` (*"One user may have N player profiles — different orgs, different sports"*) assumed **many**, but that "many" existed **only because players were org-scoped** — one row per org. The migration removes org ownership, which **dissolves the original reason for N**. The sole remaining argument for N is **multi-sport participation**. This record evaluates whether multi-sport justifies many profiles, or whether it is better modeled as a *dimension within a single identity*.

Two models:

- **Model A — One User → One PlayerProfile.** A single canonical identity per human. Multi-sport is handled by **partitioning statistics and rankings by sport** (aggregation key = `player × sport`).
- **Model B — One User → Many PlayerProfiles.** A human may hold several profiles (e.g. one per sport, or arbitrary). Each profile carries its own stats/rankings/history.

The decisive lens is the product's stated **closed-ecosystem ranking** rule: *rankings derived exclusively from PlayArena activity; every player begins equally.*

---

## 2. Dimension-by-dimension analysis

| Dimension | Model A (one profile) | Model B (many profiles) | Favors |
|---|---|---|---|
| **Closed-ecosystem rankings** | One identity → one starting point. "Begins equally" is enforceable; no Sybil/fragmentation. Multi-sport handled by per-sport leaderboards (`player × sport`). | A user can appear multiple times on a board, or **reset a bad record by spawning a fresh profile** — "begins equally" becomes "may begin *again*." Integrity depends on cross-profile dedup, which is just Model A reinvented at query time. | **A (strongly)** |
| **Match history** | One continuous career, displayed sport-partitioned. "All matches this person played" is a single query. | History splits across profiles; a unified career view requires re-linking profiles back to the human — reintroducing the fragmentation the migration exists to kill. | **A** |
| **Statistics** | Per-person, sport-dimensioned. Cricket and kabaddi stats coexist under one identity, namespaced by sport. | Stats fragment per profile; no native cross-sport "career" without an identity-resolution layer. | **A** |
| **Achievements** | Accrue to the person; lifetime/cross-sport badges ("100 matches", "PlayArena veteran") are meaningful and ungameable. | Per-profile; diluted and gameable (split or restart to dodge). | **A** |
| **Notifications** | One inbox keyed by `user_id` — exactly how `users` + notifications already work. | Routing is ambiguous: which profile receives it? Either fan-out duplicates or an arbitrary "active profile." Fights the existing user-keyed grain. | **A** |
| **Team membership** | Team rosters the **person**; many teams via multiple `team_memberships` rows; jersey/role already per-team metadata. One person can be on a cricket team and a kabaddi team with no extra profile. | "Which profile is on this team?" Forces sport↔profile coupling and ambiguity at roster time. | **A** |
| **Tournament registration** | `uq_treg_tournament_player` means *one person per tournament* — correct and enforceable. | Same person can enter once per profile; the unique index can't catch it (distinct `player_id`s) → **double-entry hole**. | **A** |
| **Multi-sport participation** | One identity, rankings/stats split by sport. Person keeps one login, one page, one inbox **and** gets per-sport leaderboards. | Natural-looking ("a profile per sport") but pays for it with a fragmented human everywhere else. | **A (B's only real case, and A serves it better)** |
| **Future public profiles** | One canonical page / handle per human (`users.username` is already globally unique). Clean URL, shareable, impersonation-resistant. | Multiple public pages for one human → confusing identity, weaker sharing/SEO, enables duplicate/impersonation. | **A** |

**Result:** Model A wins or ties on every dimension. Model B's *only* advantage — multi-sport — is better satisfied **inside** Model A by treating **sport as a dimension of statistics and rankings**, not as a reason to fork identity.

---

## 3. The crux: multi-sport is a *sport* axis, not an *identity* axis

The instinct "a person plays two sports → two profiles" conflates two different axes:

- **Identity axis** (who the person is): login, public page, inbox, achievements, team membership, anti-Sybil. This must be **singular** for integrity and UX.
- **Performance axis** (how they perform, where): statistics and rankings, which are only comparable **within a sport**. This must be **partitioned by sport** regardless of the identity model.

Model B collapses both axes onto "profile," which over-forks the identity axis to satisfy the performance axis. Model A separates them: **one identity, sport-partitioned performance.** You cannot rank a cricketer against a kabaddi raider in *either* model — so per-sport partitioning is required *anyway*. Once you have it, multiple profiles add nothing but fragmentation.

> **Therefore Model A carries one hard requirement:** rankings and statistics must aggregate on **`player × sport`**, never on `player` alone. `tournaments.sport` already exists; the stats fact tables must carry (or join to) `sport` so leaderboards are per-sport. Without this, Model A produces nonsensical cross-sport rankings — and that failure would be wrongly blamed on the cardinality choice.

---

## 4. Migration impact

**Model A**
- Enforce `uq_players_user_id WHERE user_id IS NOT NULL` (already proposed in GP-1). Most legacy `players` rows are org-created with `user_id IS NULL` and are unaffected. The conflict set is exactly *the same human linked across multiple orgs* — already the GP-7 merge population. No **new** migration problem is introduced; A simply makes GP-7 the canonical resolution rather than optional.
- **Add a `sport` dimension to the ranking fact tables** (`player_tournament_stats`, `team_tournament_stats`): denormalize `sport` at snapshot time (immutable historical fact; avoids a per-read join and survives any future tournament edits). Backfill from `tournaments.sport`. This is a small additive migration.
- Update the legacy `players` table comment (assumes N) to reflect the single-identity model. *(Noted, not performed — design only.)*

**Model B**
- Cheaper *today*: no unique index, no merge, no forced reconciliation. But the cost is **deferred and structural** — fragmentation, the registration double-entry hole, and an identity-resolution layer required for any "career" or anti-Sybil view become permanent fixtures. You pay forever to avoid paying once.

**Net:** A's incremental migration cost is one nullable-scoped unique index (already planned) + a `sport` column backfill. B's cost is lower at migration time and higher for the life of the product.

---

## 5. Ranking impact

- **Model A:** global aggregation key moves from legacy `(org, player)` to **`(player, sport)`**. Output is a set of **per-sport global leaderboards**; an org board is the same query filtered by org (a view). "Every player begins equally" is enforceable because there is exactly one starting identity per human per sport. Idempotent recompute from the fact tables is unchanged.
- **Model B:** aggregation key is effectively `(profile, sport)`, but a *profile is not a person*, so a leaderboard may list one human several times, and fairness ("begins equally") cannot be guaranteed without deduping profiles back to humans — i.e. reconstructing Model A inside the ranking query. The closed-ecosystem integrity goal is materially weaker under B.

---

## 6. UX impact

- **Model A:** one account, one profile page, one notification inbox, a **sport switcher** on the profile and rankings views. Onboarding says *"create your player profile"* (singular). Multi-sport users toggle a sport context; everything else is unified. Lowest cognitive load; matches how `users`/username/notifications already behave.
- **Model B:** introduces a **profile switcher** (a second "which-am-I-right-now" selector layered on top of the existing org switcher), an ambiguous "active profile" concept, notification-routing confusion, and multiple public URLs per human. More surface area, more ways to confuse users, and an avenue for record-laundering that erodes trust in rankings.

---

## 7. Decision & recommendation

**Adopt Model A: one `User` → one global `PlayerProfile`, enforced by `uq_players_user_id`.** Model multi-sport as a **sport dimension on statistics and rankings (`player × sport`)**, not as multiple identities.

Rationale, in one line: **identity must be singular for closed-ecosystem integrity, notifications, public profiles, and anti-Sybil; the only force pulling toward "many" is multi-sport, which is a performance-partitioning concern that Model A satisfies better than Model B.**

### Binding consequences if A is accepted
1. `players` gains `UNIQUE (user_id) WHERE user_id IS NOT NULL` — one claimed profile per human.
2. Ranking/stat aggregation key is **`player × sport`**; the fact tables must carry `sport`. *(This is a prerequisite, not optional — it is what makes A safe for multi-sport.)*
3. Legacy duplicate-per-org identities are resolved through GP-7 merge (now the canonical path, not an optional one).
4. No profile switcher in the UX; a **sport switcher** instead.
5. "Smurf"/alt competitive identities are **disallowed by design** — this is the price of closed-ecosystem ranking integrity and should be stated as policy.

### Deferred / non-blocking
- **Per-sport display aliases** (a player wanting a different shown name per sport) — if ever needed, add an optional alias on the *sport partition*, **not** a second profile. Not required for v1; noted so it is never used to justify Model B later.
- **Privacy/visibility** of the single profile is tracked separately (Appendix B #1) and is orthogonal to cardinality.

---

## 8. What would change this decision
Model A should be revisited **only** if the product later requires *legally or competitively distinct identities for the same human that must never be linked* (e.g. a person competing in two federations that forbid cross-attribution). That is a federation/external-identity requirement, which the product has explicitly ruled out ("no federation seeding, closed ecosystem"). As long as the ecosystem stays closed, Model A is the correct and durable choice.
