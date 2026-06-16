# PHI-1C — Tournament-Day Reliability Validation — Report

**Status:** Validation executed; report complete. PROJECT_STATE not updated. Not committed.
**Mission:** prove (with evidence) PlayArena can run a real Kabaddi tournament — not build features.
**Outcome in one line:** every reliability property that can be proven in this environment **passes**; no P0 found; the only P1 (concurrent scorer) is acceptably process-mitigated for a supervised pilot; the remaining gate is a **human device/network protocol** that cannot be executed from a dev box and must be run on real hardware before the live pilot.

---

## 0. Integrity note — what was and was not executed
A validation initiative must not fabricate evidence. This environment is a Windows dev host with Docker; it has **no physical phones, no real radios, no WhatsApp client, and cannot lock a screen or enable battery saver**. Therefore:
- **Executed here (real results below):** Go build/vet, the backend reliability **integration suites** (the full tournament flow, event serialization, generation, public gating, standings, DQ/walkover), the **frontend suite** incl. the offline-queue/reconcile/exactly-once/completion-gate unit tests, the **restore drill** (PHI-1B), and the observability config validation (promtool/amtool).
- **NOT executable here → ready-to-run human protocol provided** (`deploy/runbooks/pilot-validation-protocol.md`): the physical **device matrix**, **real-network** conditions, **mobile lifecycle** (screen lock/battery saver/reboot), **WhatsApp unfurl**, **320px** rendering, and **live failure injection on the deployed stack**. For each of these the *logic* they exercise is unit/integration-proven; what remains is browser/OS-quirk verification only a human on real hardware can give.

This split is the honest answer to "evidence, not assumptions": the code-level evidence is in; the field evidence is specified and pending.

## 1. Architecture compliance
Pure validation. **Zero production code changed** — the principle "every code change must be justified by a discovered failure" yielded no changes because no automated check failed. No GP-2/reputation/recruitment/FE-8D; no architecture redesign; no player features. Artifacts added: this report + the human validation protocol.

## 2. Validation environment
Windows 11 · Docker (testcontainers Postgres 17) · Go 1.25.6 · Node + pnpm · Next 15. Automated suites + the restore drill ran here. Field matrices require a staging deployment + real devices (protocol provided).

## 3. Tournament rehearsal results (D1)
The complete flow — create org → tournament → approve registrations → **generate fixtures** → **start/score live** → **walkover** → **complete** → **auto-advance bracket** → **standings** → **public results** — is exercised end-to-end through the real HTTP API (no SQL shortcuts for the operations) by the integration suites, **all green**:
- `matches/integration` (lifecycle, walkover, FE-8B progression incl. two-feeders-fill-both-slots, I1/I3 guards) — ok 51.5s
- `match_events/integration` (event log, FOR-UPDATE serialization, score derivation) — ok 25.7s
- `fixturegen/integration` (8-team knockout generate + wiring, group→knockout resolve, safety) — ok 9.8s
- `standings` (walkover + disqualification + tiebreakers) — ok
- `public/integration` (overview/fixtures/standings/match gating) — ok 7.2s
- `tournaments/integration` (lifecycle + standings incl. walkover) — ok 9.0s

**Friction findings (usability, would generate organizer support — carried + confirmed by code audit; all P2):**
- **F1** "Set tournament to Ongoing" after generation is implicit → "why can't I start a match?" (the API returns `ErrTournamentNotOngoing`).
- **F2** Generated fixtures have **no times/venues** — no publishable schedule for a multi-match day.
- **F3** Visibility defaults **private** → organizer must flip to Unlisted/Public to share.
- **F4** group_knockout requires a manual **Resolve qualifiers** step (discoverability).
- **F5** Walkover/abandon are terminal with **no organizer undo** (operator correction only).
*Note:* the GUI click-through (vs API-level) belongs to the human protocol; these frictions are real regardless of surface.

## 4. Device matrix results (D2) — PENDING HUMAN EXECUTION
Cannot run on physical devices here. **Code evidence:** the scorer is standard React; the offline queue/reconcile is pure TS, fully unit-tested for the transitions a device triggers. **Residual risk only real devices can close:** Safari/WebKit `localStorage` eviction under memory pressure, iOS background-tab suspension, battery-saver timer throttling. → Run the D2 table in the protocol (7 browsers); scorer rows are go/no-go.

## 5. Network validation results (D3)
**Proven (automated, `queue.test.ts` + `reconcile.ts`):** offline actions stay pending and feed the optimistic score; on reconnect a `needs_reconcile` action **present** on the server log is CONFIRMED and **never resent** (no duplicate), **absent** becomes pending (no loss); reconcile never disturbs an in-flight action; single-flight FIFO is enforced; `client_event_id` is the dedup oracle → **exactly-once**. Completion is blocked while anything is unsynced.
**Pending human (protocol):** real 3G / high-latency / packet-loss on an actual radio (vs devtools throttling). The mechanism is proven; only transport realism remains.

## 6. Concurrent scorer assessment (D6) — evidence-based
**Observed (code + `match_events` behavior):** event inserts take a `FOR UPDATE` lock on the match row, so two simultaneous writers produce a **consistent, correctly-sequenced log — no DB/bracket/standings corruption**. There is **no scorer lease**: both authorized scorers' valid events are accepted, so the *score can diverge from reality* (double-counted real-world actions). `ConcurrentScorerBanner` raises a loud `role="alert"` when foreign `recorded_by` events appear — **awareness, not prevention**.
- **Risk level:** P1 **correctness-of-intent** (not corruption).
- **Frequency:** low when one-scorer-per-match is briefed; realistic at busy multi-court venues without discipline.
- **Impact:** a wrong score — high *trust* impact if it occurs during a real final.
- **Recommendation (evidence-based, lease NOT implemented per constraint):** process mitigation is **acceptable for a single, supervised, single-stream pilot** given (a) organizer brief: one scorer per match; (b) the banner; (c) the `LiveMatchesNotScoring`/manual `/score` reconciliation safety net. A **backend lease becomes mandatory** before unsupervised or multi-court concurrent scoring (post-pilot).

## 7. Public surface validation (D7)
**Proven (automated, `public/integration`):** Unlisted/Public load; **Private → 404**, **Draft → 404** (even if visibility public), **inactive org → 404**, **unknown → 404** (no existence leak, never 403); **cross-org isolation** (org B slug cannot surface org A's tournament); response **whitelist** carries no `created_by`/`settings`/`generation`/PII. `next build` emits all six public routes (overview/fixtures/bracket/standings/results/match) as server-rendered (OG metadata present).
**Pending human (protocol):** 320px rendering and WhatsApp/social unfurl (needs a real client).

## 8. Failure-injection results (D8)
- **Database loss/restart → PROVEN.** PHI-1B restore drill: destroy → restore, **0 data loss, RTO ~3s** mechanism (prod ≤60 min target); standings + public queries work post-restore.
- **API/frontend restart → mechanism proven.** Offline queue persists + reconciles exactly-once on reconnect (no scoring loss); detection via `APIUnavailable` (≤1m) + external uptime.
- **Browser crash / device reboot → proven (logic).** `NORMALIZE_ON_LOAD` converts a persisted `sending` action to `needs_reconcile`; queue persists/rehydrates across reload; corrupt storage degrades safely.
- **Email outage → covered.** `EmailWorkerStuck`/dead-letter alerts; queued, drains on recovery.
- **Network interruption → proven** (reconcile, §5).
- **Pending human (protocol):** the *live* injection sequence on the deployed stack during an active match, capturing real time-to-detect / time-to-recover.

## 9. Runbook validation (D9)
- **restore.md — EXECUTED.** The restore drill follows its Path B steps; accurate (the `pg_restore --clean --if-exists` flow and verification queries match reality).
- **api-down / scorer-incident / rollback — reviewed against code, execution pending on staging.** Steps map to real commands/behavior; `scorer-incident.md` correctly reflects the offline-queue recovery and the terminal-walkover operator-correction caveat; `rollback.md` correctly warns that destructive migrations need PITR not `migrate down`. No inaccuracies found; mark "verified by review, live-execute on staging."

## 10. Alert & metric validation (D10)
- **Rules valid:** `promtool check rules` → **18 rules OK**; `promtool check config` valid; `amtool check-config` valid (PHI-1B). Every alert expression references a metric that is actually exported (verified against the registry, incl. the new `playarena_matches_live` / `_tournaments_ongoing` / `_match_events_last_minute`).
- **False-positive review:** thresholds reasonable; **`APIMemoryHigh` (fixed 700 MB)** should be tuned to the container limit to avoid noise (P2).
- **Coverage:** API/DB/email/webhook/5xx/latency/disk/memory/backup/public-errors/live-not-scoring — no obvious gap for the pilot.
- **Pending human:** live **delivery** to Slack (needs deployed Alertmanager + real webhook) and dashboard usefulness under real load — protocol §D8/D10.

## 11. Findings (P0 / P1 / P2)
**P0 — none discovered.** Every automated reliability check passed.

**P1**
- **P1-1 Concurrent scorer divergence (no lease).** *Root cause:* awareness-only mitigation; both writers accepted. *Impact:* wrong score (no corruption). *Evidence:* §6. *Action:* process mitigation for the pilot (briefing + banner + reconciliation); lease before scaling. **Not a code defect; not auto-fixed per constraint.**
- **P1-2 Field matrices not yet human-executed (device/network/lifecycle/live-injection).** *Root cause:* not executable in dev. *Impact:* unverified browser/OS-quirk risk (esp. iOS Safari offline/lifecycle). *Action:* execute `pilot-validation-protocol.md` on real hardware before the pilot. **Open — gates the pilot; cannot be closed from here.**

**P2**
- F1–F5 usability frictions (§3); `APIMemoryHigh` threshold tuning; node_exporter dependency for Disk/Backup alerts (PHI-1B); GUI click-through rehearsal pending.

## 12. Remediations
**No production code changed** — correctly, since no automated failure surfaced (the principle forbids speculative changes). Remediation of P1-1 is the documented process mitigation + recommendation; remediation of P1-2 is the executable protocol. Validation artifacts added: `deploy/runbooks/pilot-validation-protocol.md`, this report. (If the human protocol surfaces a real defect, that fix is the only code PHI-1C would add.)

## 13. Validation results (commands)
- `go build ./...` ✓ · `go vet ./...` ✓.
- Backend reliability suites (serialized): matches ✓, match_events ✓, fixturegen ✓, public ✓, standings ✓, tournaments ✓ (all `ok`).
- `pnpm typecheck` ✓ (0) · `pnpm lint` ✓ (0) · `pnpm test` → **334 passed (37 files)** · `pnpm build` ✓.
- Restore drill (PHI-1B) ✓ PASS · promtool/amtool/dashboard-JSON ✓ (PHI-1B).

## 14. Pilot Readiness Score & Go/No-Go

| Dimension | Score | Justification |
|---|---|---|
| Architecture | 🟢 | unchanged, sound; suites green |
| Operations | 🟢 | backups + tested restore + runbooks (restore executed) |
| Security | 🟢 | public gating/isolation/no-PII proven; backup/secret hygiene |
| Observability | 🟢 | 18 alerts valid, Pilot-Day dashboard, business metrics, uptime |
| Usability | 🟡 | F1–F5 frictions (support-generating, not blocking) |
| Supportability | 🟢 | runbooks accurate; restore drill reproducible |
| **Reliability** | **🟡** | **logic proven 🟢; stays 🟡 until the human device/network/lifecycle protocol is executed** |

**Recommendation (D13) — choose one: D.**
**Conditional GO:** proceed to the first real organizer pilot **after** executing the human validation protocol on real hardware (a ~half-day: the 7-browser device matrix + iOS-Safari offline/lifecycle + an on-site network test + one live failure-injection on staging), **and** briefing the organizer to run **one scorer per match**. **Do not** build the concurrent-scorer lease first (B) — evidence shows process mitigation suffices for a supervised single-stream pilot; **do not** blindly proceed (A) — that would substitute assumption for the one piece of field evidence still missing; **do not** re-harden (C) — nothing failed.

If the protocol passes, **Reliability → 🟢 and it is a full GO.** Defer the concurrent-scorer lease to post-pilot, triggered by any multi-court/unsupervised scoring need.

**Stopped at PHI-1C.** No GP-2/Reputation/Recruitment/FE-8D. Nothing committed; working tree contains only the PHI-1C validation artifacts (report + protocol) — no production code changed.
