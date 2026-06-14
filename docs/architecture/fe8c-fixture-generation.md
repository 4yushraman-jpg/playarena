# FE-8C — Tournament Automation & Fixture Generation — Implementation Report

**Status:** Implemented. Review-ready. Not committed. PROJECT_STATE not updated.
**Scope:** FE-8C only (RR, league, knockout, group+knockout generation + wizard + safety + explainability + resolution). Does **not** touch FE-8A walkover, FE-8B progression, or GP architecture. No double-elimination, GP-2, rankings, recruitment, multi-sport, or AI scheduling.

---

## 1. Architecture compliance

| Mandate | How it is met |
|---|---|
| **Deterministic** | The engine is a pure function of (participants, settings). All ordering uses total orders (id is the final tiebreak); no map iteration drives output. Random seeding is a fixed-seed Fisher–Yates — `Generate` is `reflect.DeepEqual`-stable across runs (tested, all formats). |
| **Auditable** | Every generation writes `tournaments.settings.generation`: generated_by, format, seed_strategy, random_seed (random only), legs/groups/qualifiers, generated_at, match_count, seed_order. Exposed via `GET …/fixtures/generation`. |
| **Explainable** | The seed order is the single root of all structure and is returned in every preview + stored. Knockout placement is standard seed-slot order (documented); group→knockout placement is documented (§5). |
| **Reproducible** | `random_seed` is minted server-side on a random preview, echoed to the client, and re-sent on confirm → the persisted bracket is byte-identical to the preview. Seed is masked to 52 bits so it round-trips exactly through a JSON number. |
| **No hidden randomness / no magic** | The only randomness is `SeedRandom`, gated behind an explicit, stored seed. Engine has no `time`, no global `rand`, no DB. |
| **Pure engine** | `internal/fixtures` imports only `errors`, `fmt`, `math/rand`, `sort` — no DB, no I/O. |
| **All-or-nothing** | Generation and qualifier resolution each run in a single transaction; any failure rolls back (tested: blocked generations persist 0 rows). |
| **Reuses FE-8B** | Generated knockouts are inserted with `next_match_id`/`next_match_slot` so FE-8B propagation advances winners with zero new progression code. |

---

## 2. Files created

**Pure engine — `backend/internal/fixtures/`**
- `models.go` — Format/SeedStrategy/Participant/Settings/SlotRef/PlannedMatch/Plan + sentinel errors.
- `seeding.go` — `resolveSeedOrder` (manual / random / registration), deterministic.
- `balance.go` — `orientBalanced`: Eulerian orientation of K_n → home/away within ±1.
- `roundrobin.go` — circle method (single RR + multi-leg league), `circlePairs`, rotation.
- `knockout.go` — `buildKnockout` + `buildBracket` (shared bracket core), `seedSlotOrder`, byes, next-match wiring.
- `groupknockout.go` — snake distribution, per-group RR, RANK-MAJOR qualifier seeding into the bracket core.
- `generator.go` — `Generate` entry: validate → seed → dispatch → number matches.
- `fixtures_test.go` — engine test suite (see §4).

**Generation service — `backend/internal/fixturegen/`**
- `errors.go`, `dto.go`, `repository.go` (Generate + ResolveQualifiers transactions, slot/metadata mapping, group standings), `service.go` (orchestration, seed minting, explainability), `handler.go`, `routes.go`.
- `integration/` — testmain, server, helpers, `generate_test.go`, `resolve_test.go`.

**SQL** — `backend/db/queries/fixturegen.sql` (+ regenerated `db/sqlc/fixturegen.sql.go`).

**Frontend**
- `types/api/fixtures.ts`, `lib/api/fixtures.ts`, `hooks/use-fixture-generation.ts`.
- `components/fixtures/fixture-preview.tsx` (slot resolver + preview render), `generate-wizard.tsx` (5-step wizard), `fixture-generation-panel.tsx` (entry point + audit + resolve).
- `components/fixtures/__tests__/fixtures.test.tsx`.

## 3. Files modified
- `backend/internal/bootstrap/modules.go` — mount `fixturegen.RegisterRoutes`.
- `frontend/src/lib/query-keys.ts` — `tournamentKeys.generation`.
- `frontend/src/app/(app)/[orgSlug]/tournaments/[id]/page.tsx` — mount `FixtureGenerationPanel`.

No FE-8A/FE-8B/GP files modified. (FE-8B's relaxed participant CHECK + bracket columns are reused as-is; no new migration — FE-8C is additive at the query/service layer.)

---

## 4. Tests added

**Engine (`internal/fixtures`, pure, fast):**
- Round-robin: all C(n,2) pairs exactly once for n=2..12, correct round count, odd/even bye handling, no self-matches.
- Home/away balance within ±1 (proves the Eulerian orientation); league = exact mirror (perfect balance, every pair twice).
- Knockout 8/16/32/64: exactly n−1 matches, one Final, full round-1 coverage, **progression-integrity invariant** (every TBD slot fed exactly once, every edge forward to a TBD slot, no orphan/unreachable).
- Byes (n=3,5,6,12,20): correct bye count = nextPow2−n, top seeds protected, each entrant appears once across round 1 + byes.
- Top-seed separation (1 & 2 in opposite halves; round-1 pairs sum to bracket+1).
- Group+knockout: group-match count, members per group, knockout match count, **no same-group round-1 pairing**, 8 distinct first-round qualifiers.
- Seeding: registration order preserved; manual orders by seed number; **random deterministic for same seed** + is-a-permutation + differs across seeds.
- Determinism property across all 4 formats; input validation (too few, dup ids, bad format, bad group config).

**Generation integration (`internal/fixturegen/integration`, Docker):**
- Round-robin dry-run (no persistence) then confirm (15 rows).
- Knockout wiring persisted (6 linked + 1 final).
- Safety: not-registration-closed → 422 (0 rows); fixtures-exist → 422 (count unchanged); no-permission → 403; late-registration (expected mismatch) → 422 (0 rows).
- Explainability + reproducible random: preview returns seed, confirm echoes it, `GET /generation` exposes the full record.
- Group+knockout: generate → complete group stage → resolve-qualifiers fills first knockout round; group-stage-incomplete → 422; resolve on non-group_knockout → 422.

**Frontend (`components/fixtures`):** slot resolver (participant/qualifier/TBD, team vs individual routing); wizard walks format→settings→preview→confirm and echoes `random_seed` + `expected_participants`; preview error keeps the wizard on the settings step; panel shows Generate when reg-closed/empty, hides it for non-managers, and renders the reproducible generation record once fixtures exist.

---

## 5. Algorithm documentation

**Seeding.** Total order: manual (`seed_number` asc, then registered_at, then id); registration (registered_at, then id); random (registration order, then deterministic Fisher–Yates from the stored seed).

**Round-robin / league.** Circle method with a rotating bye for odd counts. Single RR rounds = evenCount−1; league = double RR (legs=2), round numbers continue across legs. Home/away from an Eulerian orientation of K_n (±1 per participant); league legs are exact mirrors → perfect balance.

**Knockout.** `seedSlotOrder` places seeds so top seeds meet latest (1-v-N, recursive interleave). Field padded to nextPow2 with byes assigned to the top seeds (seed protection); a bye is **not** a match — the entrant is placed directly into its round-2 slot. Rounds 2..final are TBD matches wired with `next_match_id`/`next_match_slot` (FE-8B), one outgoing edge per non-final match.

**Group + knockout.** Snake/serpentine distribution spreads seed strength (deal A,B,…,G,G,…,A). Each group is a single RR (group_label set). The knockout field is the top-N of each group as **qualifier slots** (not concrete — results unknown at generation), ordered RANK-MAJOR (all winners in group order, then all runners-up, …). Standard seed-slot placement over that order makes round-1 pair winner-vs-runner-up of **different** groups (no same-group round-1 rematch for an even group count) and places a group's own qualifiers in opposite halves. Qualifier slots carry a `{group,rank}` mapping in `matches.metadata`; **resolve-qualifiers** computes per-group standings once the group stage is complete and fills the knockout's round-1 slots, after which FE-8B propagation takes over.

---

## 6. Tournament Director simulation (Deliverable 9)

Workflow for every case (no manual fixtures):
`Create tournament → Approve registrations → [Generate wizard] → Run → (walkovers/auto-advance) → Complete`.

Wizard click model (from registration_closed): **Generate fixtures** → Next (format) → pick seeding → **Preview** → Next (structure) → Next (fixture list) → **Generate fixtures** = **6 clicks** to a fully wired schedule (one extra click for group config inputs).

| Scenario | Generated | Director steps | Friction | Confidence |
|---|---|---|---|---|
| **8-team knockout** | 7 matches, QF→SF→Final, fully wired | 6 clicks | None. Winners auto-advance via FE-8B. | High — preview shows exact bracket before commit. |
| **16-team knockout** | 15 matches, R16→Final | 6 clicks | None. | High. |
| **32-team knockout** | 31 matches, R32→Final | 6 clicks | The previous manual path was 31 dialog submissions; now one wizard. | High — the headline win. |
| **12-team round robin** | 66 matches, 11 rounds, balanced home/away | 6 clicks | Large match list to scroll in preview; sectioned by round to stay legible. | High — invariants guarantee no missing/dup fixtures. |
| **24-team group+knockout** | 8 groups×3 (24 group matches) + 16-qualifier knockout (15) = 39 | 6 clicks to generate; +1 click **Resolve qualifiers** after the group stage | Two-phase: knockout slots are "Group A #1" until resolution. This is correct (results unknown up front) and is surfaced explicitly with a dedicated action that only appears when the group stage is complete. | High — preview labels qualifier slots; resolution is one click; the standings drive placement. |

**Operator confidence drivers:** nothing is created until the final confirm; the preview is the exact persisted structure (random included, via the echoed seed); the audit record lets a director reproduce/justify any bracket; safety guards make double/partial/stale generation impossible.

**Improvements made during simulation:** (1) sectioned preview by round/group for legibility at 66 matches; (2) explicit "set the tournament to Ongoing afterwards" hint on the confirm step (generation deliberately does not change lifecycle); (3) the resolve action self-reveals only when the group stage is complete and the knockout still has unfilled slots.

---

## 7. Adversarial review (Deliverable 10)

Attacked: seeding, byes, group advancement, progression, duplicate generation, re-generation, walkover interaction, late registration, stale state.

### P0 (data corruption / integrity) — all resolved
- **P0-1 Double / concurrent generation.** Two confirms racing could double-insert. **Resolved:** generation locks the tournament `FOR UPDATE` and re-checks `count(matches)==0` inside the tx; the second blocks then sees fixtures and aborts (`ErrFixturesExist`). Tested.
- **P0-2 Partial generation.** A mid-insert failure could leave a half-built bracket. **Resolved:** entire generation (all inserts + settings) is one transaction; rollback leaves 0 rows. Tested (blocked generations persist nothing).
- **P0-3 Broken progression links / orphan matches.** A mis-wired bracket would strand winners. **Resolved:** the engine's progression-integrity invariant is asserted in tests (one feeder per TBD slot, forward-only edges, single final); persistence inserts successors-first so every `next_match_id` FK resolves.
- **P0-4 Self / cross-tournament links.** **Resolved:** wiring is intra-plan only; persisted edges point within the same generated set; FE-8B's own self-link/cross-tournament guards remain.

### P1 (wrong result / workflow-blocking) — all resolved
- **P1-1 Duplicate / missing / self fixtures (RR).** **Resolved + tested:** every C(n,2) pair exactly once, no self-matches, for n=2..12.
- **P1-2 Bye corruption.** Wrong bye count or a non-top seed getting a bye. **Resolved + tested:** byes = nextPow2−n, assigned to top seeds, each entrant appears once.
- **P1-3 Same-group immediate rematch.** **Resolved + tested:** RANK-MAJOR placement yields no same-group round-1 pairing for even group counts; odd group counts emit a warning (surfaced in the wizard).
- **P1-4 Non-reproducible random.** **Resolved + tested:** seed minted once, echoed, re-sent; 52-bit mask avoids JS precision loss; same seed → identical plan.
- **P1-5 Late registration / stale preview.** Approving a registrant between preview and confirm would silently change the bracket. **Resolved + tested:** the client sends `expected_participants` from the preview; an in-tx mismatch → `ErrRegistrationsChanged`, forcing a re-preview.
- **P1-6 Generation outside the legal window.** **Resolved + tested:** only `registration_closed` is accepted; the check is inside the locked tx.
- **P1-7 Group advancement correctness.** Wrong qualifiers entering the knockout. **Resolved:** resolution uses the existing standings engine per group (deterministic tiebreakers); requires the whole group stage concluded (`ErrGroupStageIncomplete`); idempotent (already-filled slots are skipped). Tested end-to-end.
- **P1-8 Walkover interaction.** A walkover in a generated bracket must advance like a win, and walkovers in the group stage must count toward qualification. **Resolved:** generated matches are ordinary rows — FE-8A walkover + FE-8B propagation apply unchanged; group standings (`status IN ('completed','walkover')`) already include walkovers, so resolution honors them. (Covered by FE-8A/8B suites; group-stage completion in the resolve test admits walkover-concluded matches.)

### P2 (polish / accepted) — documented, not blocking
- **P2-1 Group qualification uses default 3/1/0 points**, not the tournament's custom point system, for ranking qualifiers. Tiebreakers (H2H, score diff, seed, registration) are the engine's and deterministic; for standard Kabaddi this matches. *Follow-up:* read the tournament's point config in resolution if custom scoring is configured.
- **P2-2 Odd group counts / non-power-of-two qualifier fields** can leave one potential same-group early matchup or apply knockout byes; both emit `Plan.Warnings` shown in the wizard rather than being prevented.
- **P2-3 Re-preview required after late registration** rather than auto-merging the new entrant — intentional (the director must see the changed bracket).
- **P2-4 No bracket-tree visualization** — the preview is a sectioned match list, not a graphical tree. Sufficient to audit; a visual bracket is a future enhancement.
- **P2-5 Resolution is manual** (a one-click action), not automatic on the last group result — intentional, so a director can resolve standings ties/protests first.

**All P0 and P1 findings are resolved and covered by tests.** Remaining items are P2 (documented above).

---

## 8. Success criteria

A real organizer can now: Create Tournament → Approve Registrations → **Generate Fixtures (wizard, ~6 clicks)** → Review Bracket (preview) → Run Matches → Handle Walkovers (FE-8A) → Auto-Advance Winners (FE-8B) → [group+knockout: Resolve Qualifiers, 1 click] → Complete Tournament — **without creating a single fixture manually.**
