# FE-8 — Tournament Operations Completeness — Architecture Blueprint

**Status:** Design only. Review-ready. Not implemented. PROJECT_STATE not updated.
**Date:** 2026-06-14
**Scope:** Walkovers, fixture generation, bracket progression, director workflow, adversarial review.
**Spans:** Backend (schema + service) **and** frontend. Per the brief, every capability is backed by real backend support — no frontend-only simulation.

---

## 0. Why this phase exists

FE-7 closed match management and live scoring. An organizer can today:

- create tournaments and approve registrations,
- create fixtures **one at a time** (`CreateFixtureDialog` → `POST /matches`),
- score a live match and complete it,
- read derived standings.

What an organizer **cannot** do — the gap FE-8 closes:

1. **Declare a no-show / walkover at all.** The data model fully supports walkovers; the API surface does not expose them (proven below). This is the single highest-value, lowest-cost fix in the phase.
2. **Generate a bracket or schedule.** Every fixture is hand-entered. A 32-team knockout is 31 manual dialog submissions with hand-computed rounds.
3. **Advance a bracket.** A knockout winner is not propagated to the next round. There is no `next_match_id`. "Winner of Match 3 vs Winner of Match 4" cannot be represented through the API (TBD slots are rejected on create).

FE-8 is therefore: **walkover exposure → fixture generation → bracket progression**, in that dependency order.

---

## 1. Current-state ground truth (verified against the code)

These are load-bearing facts the design builds on. Each was confirmed in the repo, not assumed.

### 1.1 Matches schema — `db/migrations/000010_create_matches.up.sql`
- Columns already present: `round_number`, `round_name`, `match_number`, `home/away_team_id`, `home/away_player_id`, `winner_team_id`, `winner_player_id`, **`is_walkover BOOLEAN NOT NULL DEFAULT FALSE`**, `status match_status`, snapshot `home_score`/`away_score`.
- `chk_matches_participants` **already permits both-NULL participant pairs** — i.e. TBD bracket slots are legal at the DB layer ("Winner of Match 3 vs Winner of Match 4").
- `chk_matches_walkover_has_winner` — a walkover row must declare a winner.
- `chk_matches_winner_is_team_participant` / `_player_participant` — winner must be a participant (NULL-safe for TBD).
- **No `next_match_id` / bracket-link column exists.**

### 1.2 `match_status` enum — `db/migrations/000001`
`scheduled, live, completed, cancelled, postponed, abandoned, walkover`.
**`walkover` and `postponed` are declared but unreachable** through the service.

### 1.3 Match service — `internal/matches/service.go`
- `allowedTransitions`: `scheduled→{live,cancelled}`, `live→{completed,abandoned,cancelled}`. **No path to `walkover` or `postponed`.**
- `parseMatchStatus` accepts only `scheduled|live|completed|cancelled|abandoned`.
- `Create` rule #3: tournament **must be `ongoing`**. Rule #6: **both participants required** (TBD slots rejected via API). Rules #8/#9: each participant must be cross-org-valid and hold an **approved** registration.
- `UpdateRequest` has **no `is_walkover` field**.

### 1.4 Match SQL — `db/queries/matches.sql`
- `CreateMatch`: comment states "is_walkover always FALSE"; column not in the insert list.
- `UpdateMatch`: comment states "`is_walkover` … immutable and never appear in the SET clause." **There is no code path anywhere that sets `is_walkover = TRUE`.**
- `ListCompletedMatchesByTournament` (the standings feed): `WHERE status = 'completed'`. **A row with `status='walkover'` would be invisible to standings.** ← critical interaction.

### 1.5 Standings engine — `internal/standings/engine.go` (pure, already walkover-aware)
- `CompletedMatch.IsWalkover` is consumed.
- `closeLossPoints` explicitly exempts walkovers from the close-loss bonus ("a walkover is a forfeit, not a competitive result").
- Walkover convention is 0–0 score; loser gets `LossPoints`, winner gets `WinPoints`.
- N-way head-to-head tiebreak already implemented (Phase 23D).
- **Conclusion:** the hard part of walkovers (correct points) is *already built*. Only the write path and the standings query filter are missing.

### 1.6 Tournament lifecycle — `internal/tournaments/service.go`
- `draft → registration_open → registration_closed → ongoing → completed`; any → `cancelled`.
- On `→ completed`, `snapshotTournamentStats` feeds rankings.
- Standings (`GetStandings`) recompute on read from completed matches + approved registrations; settings parsed from `tournaments.settings` JSONB.

### 1.7 Seeding — `internal/tournament_registrations`
- `seed_number` is updatable per-registration via `PATCH …/registrations/{id}` (gated on `tournament.update`). **No bulk-seed endpoint.**

### 1.8 Frontend (FE-7) — `frontend/src/components/matches/*`
- `TournamentFixtures`: flat fixture list, "Create fixture" only when `tournament.status === "ongoing"`.
- `MatchActions`: edit/cancel only for `scheduled`; live/terminal owned by the scorer surface.
- No bracket view, no generation, no walkover control, no bye/TBD rendering.

### 1.9 Forward-compatibility constraint — GP-Kabaddi blueprint (design-only)
The platform is slated to pivot player-first / Kabaddi-only with closed-ecosystem **Reputation Points**, and that design states **walkovers award no RP** (anti-farm). FE-8 must keep the **win/loss record** of a walkover while making it trivial for a future ranking pass to **exclude walkover results from RP**. The `is_walkover` flag is exactly that hook — preserve it through to the rankings snapshot.

---

## 2. Deliverable 1 — Walkover Architecture

### 2.1 Problem statement
Walkovers are 80% built and 0% reachable. The model, constraints, and points math exist; the write path and one query filter do not.

### 2.2 Canonical representation (decision)
A walkover is a **terminal match with a result but no event log**:

| Field | Value |
|---|---|
| `status` | `walkover` |
| `is_walkover` | `TRUE` |
| `winner_team_id` / `winner_player_id` | the present side |
| `home_score` / `away_score` | `0` / `0` (forfeit convention) |
| `ended_at` | stamped at declaration |
| `notes` | required reason (e.g. "Away team no-show, 15-min grace expired") |

**Decision: keep `status='walkover'` distinct from `completed`** (rather than overloading `completed` + `is_walkover`). Rationale: the enum value already exists; a distinct status keeps the fixture list, audit, and notifications able to *show* a walkover as a walkover, and keeps `is_walkover` from being the sole discriminator. The cost is one query change (§2.6), which is trivial and explicit.

### 2.3 No-show handling (state model)
Walkover may be declared from **two** source states:

- `scheduled → walkover` — opponent never appeared (the common case; no match was ever started).
- `live → walkover` — match started, then one side withdrew/was disqualified before a result could be scored. (Distinct from `abandoned`, which is "started, *no* official result, *no* winner".)

New allowed transitions added to `allowedTransitions`:
```
scheduled: {live, cancelled, walkover}
live:      {completed, abandoned, cancelled, walkover}
```
`completed`, `cancelled`, `abandoned`, `walkover` remain terminal.

### 2.4 API surface (decision: dedicated endpoint, not generic PATCH)
```
POST /api/v1/organizations/{slug}/matches/{id}/walkover
Body: { "winner": "home" | "away", "reason": "<text, required>" }
Auth: match.update (same gate as scoring/completion)
```
Why dedicated rather than extending `PATCH`:
- A walkover has a tight invariant set (must set winner, must zero scores, must flip `is_walkover`, must stamp `ended_at`, requires a reason). Folding it into the generic partial-update path multiplies an already large validation matrix and invites the bug where `is_walkover` is set without a winner, or a winner is set on a non-terminal status.
- `winner: "home"|"away"` is resolved server-side to the concrete team/player id from the match row — the client never sends a participant id, which removes a class of "winner not a participant" errors.

Double-no-show (neither side appears) is **not** a walkover (no winner is possible). It maps to `cancelled` (or a future `voided`) with a reason — handled by existing `Delete`/cancel, not this endpoint.

### 2.5 Service logic (`Service.Walkover`)
1. BOLA + org ownership (reuse `assertOrgOwnership`).
2. Load match; reject if `terminalStatuses[current.Status]`.
3. Reject if either participant slot is **TBD/NULL** → `ErrWalkoverNeedsParticipants` (can't award a walkover into an unresolved bracket slot).
4. Resolve `winner` ("home"/"away") to the concrete `winner_team_id`/`winner_player_id`.
5. In one tx (mirroring `UpdateWithAudit`, with the tournament `FOR SHARE` lock from `tournamentLockStatuses`):
   - set `status='walkover'`, `is_walkover=TRUE`, winner, `home_score=0`, `away_score=0`, `ended_at=now()`, `notes=reason`;
   - CAS guard on `previous_status` (same race protection as `UpdateMatch`);
   - write `audit_logs` (`update`, old/new snapshot, reason in `new_data`);
   - enqueue outbox `match_walkover` event;
   - **propagate the winner to `next_match_id`** if set (see Deliverable 3) — same tx.
6. Post-commit `DrainOutbox`.

A new SQL `SetMatchWalkover` is required (the existing `UpdateMatch` deliberately excludes `is_walkover`; do **not** widen it — keep walkover writes on their own query for clarity and to preserve the "UpdateMatch never touches is_walkover" invariant).

### 2.6 Standings impact
- **Required query change:** `ListCompletedMatchesByTournament` (and the rankings-snapshot variant `GetCompletedMatchesForStandings`) change `WHERE status = 'completed'` → `WHERE status IN ('completed','walkover')`.
- The engine already does the rest: walkover → win/loss with `LossPoints` (close-loss exempt), 0–0 score contribution. No engine change.
- **Regression guard:** add a standings test asserting a walkover awards `WinPoints`/`LossPoints` and does **not** trigger the close-loss bonus even when `CloseMargin>0`.

### 2.7 Bracket impact
A walkover yields a concrete winner, so it propagates **identically to a scored completion** (Deliverable 3). No special casing in propagation — the propagator keys off "match reached a terminal state with a winner", which is true for both `completed` and `walkover`.

### 2.8 Rankings / RP forward-compat
`snapshotTournamentStats` must carry `is_walkover` through to whatever rankings consume. Today rankings count win/loss; under the future GP RP model, the snapshot pass filters `is_walkover=TRUE` results out of RP while still counting them in W/L. Action for FE-8: ensure the snapshot path **does not discard** `is_walkover` (it's already on `standings.CompletedMatch`). No RP logic is built in FE-8.

### 2.9 Audit trail
- `audit_logs` row per walkover (actor, match entity, before/after, reason).
- `match_walkover` outbox → in-app/email/webhook notification ("Match X awarded to Y by walkover").
- `notes` permanently records the human reason on the match row.

### 2.10 Frontend
- **Award walkover** action on the match detail page (`MatchActions`) for `scheduled` and `live` matches, gated on `match.update`: dialog with winner radio (home/away, showing resolved names) + required reason textarea.
- Also surface in the live scorer (`live-scorer.tsx`) as a secondary action for the "opponent walked off" case.
- `StatusBadge` + fixture list render `walkover` distinctly (e.g. "W/O — Team A"); standings already display correct points.

---

## 3. Deliverable 2 — Fixture Generation

### 3.1 Architecture: a pure generator + a thin persistence endpoint
Mirror the standings pattern: a **pure, DB-free, deterministic** package `internal/fixtures` that takes a participant list + format + settings and returns a **match plan** (rounds, match numbers, participant slots incl. TBD, and bracket links). A thin service persists the plan in one transaction. Pure → exhaustively unit-testable (the brief's adversarial requirements live here).

```
internal/fixtures/
  generator.go   // Generate(participants, format, settings) -> Plan
  bracket.go     // knockout seeding + bye placement + next-match links
  roundrobin.go  // circle method
  models.go      // Participant, Plan, PlannedMatch
```

```go
type Participant struct { ID string; Seed *int16; RegisteredAt time.Time }
type PlannedMatch struct {
    RoundNumber int16; RoundName string; MatchNumber int16
    HomeRef, AwayRef ParticipantRef  // concrete id | TBD(fromMatchNumber, slot) | BYE
    NextMatchNumber *int16; NextSlot *Slot  // bracket link (intra-plan, resolved to UUIDs on insert)
}
type Plan struct { Matches []PlannedMatch; Byes []string; Warnings []string }
```

### 3.2 Seeding (deterministic, the root of integrity)
**Seed order** = participants sorted by: `seed_number ASC NULLS LAST`, then `registered_at ASC`, then `id ASC` (final total-order tiebreak so the generator is fully deterministic — no map iteration, same as the standings engine's discipline).

- Knockout uses **standard bracket seeding** so top seeds meet late: positions filled by the canonical 1-vs-N pattern (1,16,8,9,5,12,4,13,…). Implemented via the recursive seed-slot sequence, not hand-tables, so it generalizes to any bracket size.
- Round-robin/league ignore seed for *pairing* but use seed order for stable round assignment.

### 3.3 Formats

**round_robin** — every pair once; `n(n-1)/2` matches.
- Even `n`: circle method, `n-1` rounds, `n/2` matches/round.
- Odd `n`: add a phantom "BYE"; each round one real participant draws the bye (no match row emitted for the bye pairing). `n` rounds.

**league** — round_robin × `settings.legs` (default 1; `2` = home/away double round-robin, swapping home/away on the second leg). Round numbering continues across legs.

**knockout** (single elimination) —
- Pad participant count up to the next power of two with **byes**.
- Round 1 pairs seeds by standard bracket positions; a seed drawn against a BYE is **placed directly into its round-2 slot** — *no match row is emitted for a bye* (see §3.4 decision).
- Rounds 2…final are **TBD matches**: participant slots NULL, linked by `next_match_id`/slot. Round names auto-derived from distance to final (`Final`, `Semi Final`, `Quarter Final`, else `Round of N`).

**group_knockout** —
- Partition seeded participants into `settings.groups` groups by snake/serpentine distribution (so seed strength is spread, not stacked) — group size from `settings.teams_per_group` or `ceil(n/groups)`.
- Each group = an independent round_robin (group-stage match rows carry a `group_label`).
- Knockout stage = TBD bracket seeded by **group-qualifier placeholders** ("Group A #1", "Group B #2"). These resolve to concrete participants only after the group stage completes (Deliverable 3, §4.6). Cross-pairing rule (A1 vs B2) configurable in settings, default standard.

**double_elimination** — enum exists. **Recommendation: defer to a stretch sub-phase (FE-8D-stretch).** Winners + losers bracket + grand-final reset is materially more complex (loser-drop routing, bracket-link fan-in) and is not needed for the 8/16/32-team director workflows in Deliverable 4. Document the data model is already capable (`next_match_id` + a second `loser_next_match_id` link would be the only addition) but do not build it in the core phase.

### 3.4 Bye handling (decision: no phantom match rows)
A bye is **not** a match. `chk_matches_participants` forbids a one-sided participant pair (home set / away NULL is illegal), so a "bye match" cannot even be stored. Therefore the generator **places the bye recipient directly into the next-round slot** and records the bye in `Plan.Byes` for display ("Team A — bye to Round 2"). This sidesteps the CHECK constraint cleanly and avoids fake completed matches polluting standings/stats.

### 3.5 Odd / awkward counts
- round_robin/league: circle-method phantom bye, as above.
- knockout: pad to power of two; `byes = nextPow2(n) - n`, assigned to the top `byes` seeds.
- `n < 2`: reject (`ErrTooFewParticipants`).
- group_knockout with groups not dividing evenly: distribute remainder to lowest-numbered groups; warn in `Plan.Warnings`.

### 3.6 The lifecycle problem fixture generation forces (important)
Generation must happen **before** play, i.e. in `registration_closed` — but the current `Create` path requires `ongoing` **and** rejects TBD slots **and** requires both participants approved. Generation needs all three relaxed *for the generation path only*:

- **Allowed state:** generation permitted when tournament is `registration_closed` (bracket finalized before going `ongoing`). Going `ongoing` no longer means "now hand-create matches"; it means "the generated schedule is now live."
- **TBD slots:** the generated knockout rounds 2…final are inserted with NULL participants (DB already allows this). The generation insert path bypasses Create rule #6.
- **Eligibility:** the generator's concrete-participant rows are sourced **only** from approved registrations (it reads the approved set), so eligibility is satisfied by construction; TBD rows have no participant to check.

Manual one-off `POST /matches` (FE-7) stays as-is for ad-hoc additions in `ongoing`. Generation is a separate, bulk, pre-play operation.

### 3.7 Endpoint
```
POST /api/v1/organizations/{slug}/tournaments/{id}/fixtures/generate
Body: { "format_override"?: <format>, "settings"?: {...}, "dry_run"?: bool }
Auth: tournament.update (matches the seeding gate)
Pre: tournament.status == registration_closed
```
- `dry_run: true` → returns the `Plan` (rounds, byes, warnings) **without persisting** → powers the frontend preview.
- Persist path: one tx — bulk-insert all match rows, then a second pass to set `next_match_id` (two-pass because TBD successors must exist before they can be referenced). Audit `create` per match (or one batched audit entry referencing the generation).

### 3.8 Regeneration rules
- Allowed **only** when **every** existing match for the tournament is still `scheduled` (none `live`/`completed`/`walkover`/`abandoned`). Guard via a `CountNonScheduledMatchesByTournament` query.
- Regeneration = delete the prior generated set + insert the new plan, in one tx.
- Once any match has started, regeneration is **blocked** (`ErrFixturesLocked`); the organizer must cancel individual fixtures instead.
- Generation is **blocked if any matches already exist** unless `regenerate=true` is explicitly passed → prevents accidental double-generation.

### 3.9 Frontend
- **Generation wizard** launched from `TournamentFixtures` when `status===registration_closed` & `tournament.update`:
  1. **Seed review** — table of approved participants with editable seed inputs; "auto-seed by registration order" button; bulk-save (needs the bulk-seed endpoint, §6.3).
  2. **Format & settings** — format (defaults to `tournament.format`), legs, groups/teams-per-group.
  3. **Preview** — calls `dry_run`; renders the bracket/round-robin grid, byes, and warnings.
  4. **Generate** — persists; on success invalidates match list + navigates to the bracket view.
- **Regeneration** affordance with the "all-scheduled only" guard reflected in the UI (disabled + explanatory tooltip once a match is live).

---

## 4. Deliverable 3 — Bracket Progression

### 4.1 Schema addition (the one structural gap)
```sql
ALTER TABLE matches
  ADD COLUMN next_match_id   UUID     REFERENCES matches(id) ON DELETE SET NULL,
  ADD COLUMN next_match_slot SMALLINT,           -- 1 = home, 2 = away
  ADD COLUMN group_label     TEXT,               -- group_knockout group stage
  ADD CONSTRAINT chk_matches_next_slot CHECK (next_match_slot IS NULL OR next_match_slot IN (1,2));
```
`next_match_id` + `next_match_slot` are the bracket edge: "this match's winner becomes participant *slot* of *next_match_id*." Round-robin/league/group-stage rows leave them NULL.

### 4.2 Automatic advancement (winner propagation)
On any transition into a terminal state **with a winner** (`completed` via scoring, or `walkover`), inside the same completion transaction:
1. If `next_match_id` is NULL → done (final, or a league/RR match).
2. Else load the successor `FOR UPDATE`; reject if successor is no longer `scheduled` (`ErrDownstreamLocked` — see §4.4).
3. Write the winner's participant id into the successor's `home_*`/`away_*` slot per `next_match_slot` (team or player column, matching participant type).
4. The successor stays `scheduled`; it becomes *playable* once **both** slots are non-NULL (a UI/scoring precondition, not a status change).

This lives in the repository tx layer next to `UpdateWithAudit`/`SetMatchWalkover` so propagation is atomic with the result that caused it. A partial failure can never leave a winner recorded but unpropagated.

### 4.3 Bye propagation
Byes are resolved at **generation** time (§3.4) — the bye recipient is already sitting in the round-2 slot. No runtime propagation needed for byes.

### 4.4 Bracket integrity invariants
- **I1 — No completion of a TBD match.** A match with a NULL participant slot cannot transition to `live`/`completed`/`walkover`. Enforced in the service (`ErrMatchHasTBDSlot`). Prevents "completing" a match whose feeders haven't finished.
- **I2 — No double propagation.** A given (feeder → successor.slot) is written exactly once. Re-completing a feeder (see I3) must not append a second participant.
- **I3 — Correction after propagation.** If a completed feeder's winner is corrected (score correction flips the winner):
  - successor still `scheduled` → re-propagate (overwrite the slot with the new winner) in the same tx;
  - successor already `live`/terminal → **block** the correction (`ErrDownstreamLocked`); resolving it requires manual cascade by the director. This is the safe default; a full automatic cascade-rollback is explicitly out of scope.
- **I4 — Deterministic structure.** Generation is pure/deterministic (§3.2), so the bracket shape is reproducible and reviewable.
- **I5 — Org/tournament consistency.** Successor must belong to the same tournament+org as the feeder (trivially true for generated plans; asserted defensively).

### 4.5 Walkover interaction
Already covered: walkover is a terminal-with-winner state, so §4.2 propagates it with zero special casing. A bracket can be entirely advanced by walkovers if needed.

### 4.6 Group → knockout resolution
The knockout stage of a `group_knockout` is generated with **qualifier placeholders**, not `next_match_id` edges from group matches (a group has many matches feeding one standings table, not a single-winner edge). Resolution is a distinct step:
- When **all** group-stage matches for the tournament are terminal, a **resolve-qualifiers** action (manual trigger or automatic on last group result) computes each group's standings (existing engine), maps `Group A #1`, `Group B #2`, … to concrete participants, and writes them into the knockout round-1 slots.
- Manual trigger is the safer default (lets the director resolve standings ties/protests first); automatic is an enhancement.

### 4.7 Frontend
- **Bracket view** component (`tournament-bracket.tsx`): knockout tree with TBD slots rendered as "Winner of M{n}"; clicking a match deep-links to its detail/scorer. Group stage rendered as per-group standings tables + fixtures.
- Fixture list (existing) gains **round grouping** and bye chips.
- "Resolve qualifiers" control for group_knockout once group stage completes.

---

## 5. Deliverable 4 — Tournament Director Workflow simulation

For each size: **setup → generation → execution → completion**, with friction called out.

### 5.1 — 8 teams, single knockout
- **Setup:** create tournament (format `knockout`), open reg, approve 8 teams, close reg.
- **Generation:** seed 1–8 (or auto by reg order). `dry_run` preview shows QF×4 → SF×2 → Final, no byes (8 = 2³). Generate → 7 matches (4 concrete QFs + 2 TBD SFs + 1 TBD Final), bracket links set. Move tournament → `ongoing`.
- **Execution:** score each QF; winners auto-populate SF slots. One team no-shows → **award walkover** in two clicks; winner propagates to SF identically. Score SFs → Final populated. Score Final.
- **Completion:** tournament → `completed`; rankings snapshot runs.
- **Friction found:** none structural at 8. The single friction is **seeding ergonomics** — without bulk-seed, the organizer PATCHes 8 registrations individually (addressed §6.3).

### 5.2 — 16 teams, group + knockout
- **Setup:** format `group_knockout`, `settings={groups:4, teams_per_group:4, qualifiers_per_group:2}`. Approve 16, close reg.
- **Generation:** preview shows 4 groups × 6 RR matches = 24 group matches + an 8-slot knockout (QF→SF→Final) with qualifier placeholders. Generate. Go `ongoing`.
- **Execution:** score 24 group matches (standings per group live via engine). **Resolve qualifiers** once groups complete → A1/B2/… written into QF slots. Score knockout with auto-advance; walkovers as needed.
- **Completion:** as above.
- **Friction found:**
  - **F1 — Qualifier resolution is a real decision point**, not a no-op: group ties must be resolvable *before* locking QF slots. The manual "resolve qualifiers" step (with a standings review) is the right design; an auto-resolve-on-last-result would rob the director of tie/protest handling. → keep manual default.
  - **F2 — Scoring 24 group matches one-by-one is heavy.** The fixture list must group by group_label and surface "next unscored match" to keep flow tight. (UI, not backend.)

### 5.3 — 32 teams, single knockout
- **Setup:** format `knockout`, approve 32, close reg.
- **Generation:** 32 = 2⁵ → clean bracket, R32(16) → R16(8) → QF(4) → SF(2) → Final = 31 matches, no byes. Standard seeding puts 1 vs 32 etc. Generate, go `ongoing`.
- **Execution:** 31 matches with full auto-advance. This is the case that makes manual creation untenable (31 hand dialogs vs one generate).
- **Completion:** as above.
- **Friction found:**
  - **F3 — Non-power-of-two (e.g. 30 teams):** 2 byes to top seeds, placed directly into R16. Preview must clearly show who has a bye. (Covered by §3.4.)
  - **F4 — Bulk scheduling of times/venues.** Generation sets structure but every match needs a `scheduled_at`. Generating 31 fixtures with no times, then PATCHing each, is friction. → **Optional generation input:** a scheduling template (start time, slot duration, venues) that stamps `scheduled_at` across the plan. Mark as enhancement (FE-8 nice-to-have), not a P0.

### 5.4 Cross-cutting friction summary
| ID | Friction | Resolution | Priority |
|---|---|---|---|
| F-seed | Per-registration seeding only | Bulk-seed endpoint + wizard step | P1 |
| F1 | Group ties before qualifier lock | Manual resolve-qualifiers step | designed-in |
| F2 | Scoring many group matches | Round/group-grouped list + "next match" | P1 (UI) |
| F3 | Bye visibility | Preview shows byes explicitly | P1 (UI) |
| F4 | Bulk scheduling | Optional schedule template at generation | P2 |

---

## 6. Deliverable 5 — Adversarial Review

Attempts to break each subsystem, with severity and resolution. P0 = data-corruption/integrity; P1 = wrong result or workflow-blocking; P2 = polish.

### 6.1 Seeding
- **A1 (P1) — Non-deterministic order from equal seeds.** Two participants with the same `seed_number` could order by map iteration → different bracket each run. **Resolve:** total-order tiebreak `seed → registered_at → id` (§3.2); generator never iterates a map for ordering. Add a test that generates twice and asserts identical plans.
- **A2 (P1) — Duplicate seed numbers.** Organizer assigns seed `1` to two teams. **Resolve:** generation validates seeds are unique among approved participants *or* falls back to the deterministic tiebreak; surface a `Plan.Warning`. Do not hard-fail (seeds are advisory).
- **A3 (P2) — Seed out of range** (seed 50 in an 8-team draw). **Resolve:** seeds are relative ordering, not slot indices; large values just sort last. Documented; no crash.

### 6.2 Progression
- **A4 (P0) — Lost winner on partial failure.** Winner recorded but not propagated due to a mid-operation error. **Resolve:** propagation is in the **same transaction** as completion/walkover (§4.2). All-or-nothing.
- **A5 (P0) — Double propagation / slot overwrite race.** Two feeders completing concurrently both write the same successor, or a re-completion appends twice. **Resolve:** successor locked `FOR UPDATE` during propagation; each feeder writes a *fixed* slot (`next_match_slot`), so two feeders write *different* slots; re-completion overwrites its own slot only (I2/I3). CAS guard on feeder status prevents double-fire.
- **A6 (P1) — Completing a TBD match.** Scoring a match whose feeders haven't finished. **Resolve:** I1 — service rejects `live`/`completed`/`walkover` while any slot is NULL (`ErrMatchHasTBDSlot`).
- **A7 (P1) — Correction cascade.** Feeder winner corrected after successor played. **Resolve:** I3 — re-propagate if successor still scheduled; block otherwise (`ErrDownstreamLocked`). No silent corruption.

### 6.3 Walkovers
- **A8 (P0) — Walkover invisible to standings.** `status='walkover'` not matched by the `status='completed'` standings filter → forfeits don't count. **Resolve:** §2.6 query change to `status IN ('completed','walkover')`. **This is the single most important fix in the phase** — without it, walkovers silently corrupt the table. Regression test mandatory.
- **A9 (P1) — Walkover without a winner / into a TBD slot.** **Resolve:** endpoint requires `winner`; service rejects TBD participants (`ErrWalkoverNeedsParticipants`); DB `chk_matches_walkover_has_winner` is the backstop.
- **A10 (P1) — Walkover earns close-loss bonus.** A 0–0 walkover under `CloseMargin>0` could wrongly grant the loser bonus points. **Resolve:** already handled — `closeLossPoints` exempts walkovers. Add explicit test (currently relies on implementation, not a test).
- **A11 (P2) — Double no-show.** Neither side appears; both-walkover is meaningless. **Resolve:** not a walkover → cancel/void with reason (§2.4).
- **A12 (P1, forward) — Walkover farming for RP.** **Resolve:** `is_walkover` preserved to the rankings snapshot; future GP RP pass excludes it (§2.8). No RP logic built now, but the hook is guaranteed present.

### 6.4 Bracket integrity
- **A13 (P0) — Cyclic / self-referential `next_match_id`.** A corrupt link makes a match feed itself or an earlier round. **Resolve:** links are emitted only by the pure generator, which builds strictly forward (round r → round r+1); never user-supplied. Defensive CHECK + a generation invariant test (no back-edges, each non-final match has exactly one outgoing edge, each successor slot fed by exactly one feeder).
- **A14 (P1) — Orphaned successor.** `ON DELETE SET NULL` on `next_match_id` means cancelling a feeder NULLs the edge. **Resolve:** acceptable — the successor becomes a manual-resolution point; surfaced in UI. Document that cancelling a bracket match mid-tournament requires director intervention for the downstream slot.
- **A15 (P1) — Regeneration after play.** Regenerating once matches are live wipes results. **Resolve:** §3.8 — regeneration blocked unless all matches `scheduled` (`CountNonScheduledMatchesByTournament`).
- **A16 (P1) — Generation into a live tournament.** Generating again after `ongoing` creates duplicate fixtures. **Resolve:** generation requires `registration_closed`; "matches already exist" requires explicit `regenerate=true` + the all-scheduled guard.

### 6.5 Standings
- **A17 (P0) — Walkover filter (same as A8).** Resolved by §2.6.
- **A18 (P1) — Participant in a match but not in registrations.** Engine silently ignores (documented). **Resolve:** generation sources participants from approved registrations only, so this can't arise from generated brackets; the engine's defensive skip remains for manual matches.
- **A19 (P1) — Disqualification after results.** A disqualified registrant's played matches still sit in standings. **Resolve:** out of FE-8 scope (it's a results-voiding policy decision), but flag: the `disqualified` status already exists; a future pass should decide void-vs-keep. Documented, not built.

### 6.6 Resolution status
All **P0** (A4, A5, A8, A13, A17) are resolved by design above and are mandatory for the phase. All **P1** are resolved or explicitly scoped with a documented decision. **P2/forward** items (A3, A11, A12, A19, F4) are documented with hooks but not built.

---

## 7. Consolidated impact

### 7.1 Schema impacts (one migration, additive)
- `matches.next_match_id UUID NULL REFERENCES matches(id) ON DELETE SET NULL`
- `matches.next_match_slot SMALLINT NULL` + `chk_matches_next_slot IN (1,2)`
- `matches.group_label TEXT NULL`
- No change to enums (`walkover`/`postponed` already exist). No change to participant CHECK (TBD already legal).

### 7.2 Backend impacts
- **New pure package** `internal/fixtures` (generator + bracket + round-robin + models). Pure, deterministic, fully unit-tested — carries the adversarial seeding/structure tests.
- **Matches service:**
  - `Walkover(...)` use-case + `POST …/matches/{id}/walkover` route.
  - Extend `allowedTransitions` for `walkover`; add `parseMatchStatus` support.
  - Winner-propagation in the completion/walkover repo tx (`next_match_id`).
  - I1 TBD-completion guard; I3 correction guard.
  - New errors: `ErrWalkoverNeedsParticipants`, `ErrMatchHasTBDSlot`, `ErrDownstreamLocked`, `ErrFixturesLocked`, `ErrTooFewParticipants`.
- **Tournaments service:** `GenerateFixtures(...)` + `POST …/tournaments/{id}/fixtures/generate` (+ `dry_run`); regeneration guard; optional `ResolveQualifiers` for group_knockout; bulk-seed endpoint `PATCH …/tournaments/{id}/seeds` (§6.3 / F-seed).
- **SQL (new):** `SetMatchWalkover`, `BulkInsertMatches`, `SetMatchNextLink`, `GetNextMatch`/`PopulateSlot`, `CountNonScheduledMatchesByTournament`, `DeleteScheduledMatchesByTournament` (regeneration), `BulkUpdateSeeds`.
- **SQL (modified):** `ListCompletedMatchesByTournament` + `GetCompletedMatchesForStandings` → `status IN ('completed','walkover')`.
- Standings/rankings snapshot: ensure `is_walkover` carried through (likely already true).

### 7.3 Frontend impacts
- `tournament-bracket.tsx` (knockout tree + group tables + TBD rendering).
- Generation **wizard** (seed review → format/settings → `dry_run` preview → generate) and regeneration controls.
- Bulk-seed UI.
- **Walkover** dialog in `MatchActions` + live scorer.
- Fixture list: round/group grouping, bye chips, "next unscored match".
- `StatusBadge`/labels for `walkover`; "resolve qualifiers" control.
- Query-key/invalidation: generation and propagation invalidate the **params-less** match-list root + standings (heed the FE-6 "undefined key" invalidation trap noted in PROJECT_STATE).

### 7.4 Automation model (what's automatic vs manual)
| Automatic (system) | Manual (director) |
|---|---|
| Winner propagation to next bracket slot | Triggering fixture generation |
| Bye placement (at generation) | Seeding (or accept auto-by-reg-order) |
| Standings/qualifier-table recompute | Declaring a walkover (+ reason) |
| Round naming, match numbering | Scoring/completing matches |
| Successor becomes playable when both slots filled | Resolve-qualifiers (group→KO) default |
| Rankings snapshot on tournament complete | Scheduling times/venues (template optional) |

### 7.5 Operational workflow (end-to-end)
`draft` → open reg → approve participants → close reg → **seed** → **generate (preview → confirm)** → tournament `ongoing` → score/walkover matches (**auto-advance**) → [group_knockout: **resolve qualifiers**] → score knockout → tournament `completed` → rankings snapshot.

---

## 8. Implementation phases (proposed)

Ordered by dependency and value density. Each sub-phase is independently shippable and testable.

- **FE-8A — Walkover exposure (highest value / lowest cost).**
  Standings query fix (A8/A17, the critical one), `SetMatchWalkover`, `Walkover` service + endpoint, transition table, walkover UI, regression tests. *No schema change.* Delivers no-show handling immediately and de-risks the rest.

- **FE-8B — Bracket linkage + progression.**
  Migration (`next_match_id`, `next_match_slot`, `group_label`), propagation in completion/walkover tx, integrity guards (I1–I5), bracket view (read-only render of manually/linked matches). Walkover (8A) now propagates.

- **FE-8C — Fixture generation.**
  Pure `internal/fixtures` (round_robin, league, knockout, group_knockout) + adversarial seeding/structure tests, generate endpoint (+`dry_run`), regeneration guards, generation wizard + bulk-seed, generation-state relaxation (`registration_closed` + TBD inserts). Group→knockout qualifier resolution.

- **FE-8D (stretch) — double_elimination + scheduling template (F4).**
  Losers bracket (`loser_next_match_id`), grand-final reset; optional schedule-template stamping `scheduled_at` across a generated plan. Out of the core commitment.

**Validation gates per sub-phase** (matching project norms): `go build`/`go vet`/`go test ./...` green (integration in Docker); frontend `tsc --noEmit` + ESLint clean + Vitest green + `pnpm build`. Adversarial findings re-reviewed at sub-phase close before marking CLOSED.

---

## 9. Open questions for review
1. **Walkover status vs completed:** confirm the decision to keep `status='walkover'` distinct (with the standings-query change) over overloading `completed`+`is_walkover`. (Recommended: distinct.)
2. **Qualifier resolution:** manual default vs auto-on-last-group-result. (Recommended: manual.)
3. **double_elimination:** in-scope for FE-8 or deferred to stretch? (Recommended: deferred.)
4. **Correction cascade (I3):** block-downstream (recommended) vs build automatic rollback (large).
5. **Disqualification-after-results (A19):** void results vs keep — decide policy now or defer. (Recommended: defer, document.)
6. **Scheduling template (F4):** include in FE-8C or push to FE-8D. (Recommended: FE-8D.)
```

