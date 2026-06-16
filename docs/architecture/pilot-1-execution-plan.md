# PILOT-1 — First Real Tournament Pilot — Execution Plan

**Status:** Plan only. No code, no implementation, no PROJECT_STATE change.
**Goal:** run the safest, highest-learning **first** real organizer pilot — process & evidence, not features.
**Posture:** Architecture/Operations/Security/Observability/Supportability 🟢; Reliability 🟡 pending the real-device run; no P0, no known corruption paths. PILOT-1 closes the human/operational unknowns.

---

## Part 1 — Pilot Design (optimize for learning ÷ risk, not size)

**Recommendation: an 8-team single-elimination knockout on ONE court, single scoring stream.**

| Dimension | Choice | Why |
|---|---|---|
| Format | **Knockout (single-elimination)** | Exercises the *entire* pipeline that's riskiest: seeded generation → bracket **auto-advance** → completion → public bracket. Proven green in PHI-1C. |
| Teams | **8** | Smallest field that produces a real multi-round bracket (QF→SF→Final) with no byes. |
| Matches | **7** (4 QF + 2 SF + 1 Final) | Bounded blast radius; enough repetition to surface friction, few enough to supervise every one. |
| Courts / streams | **1 court, 1 live match at a time** | **Deliberately sidesteps the one open P1** (concurrent-scorer divergence): with a single stream there is never a reason for two scorers on one match. |
| Duration | **Half day (~3–4 h)** | Short enough that an operator stays sharp; long enough for device sleep/battery/network realities to appear. |
| Scorers | **1 active** (+1 trained backup, never simultaneous) | One-scorer-per-match discipline is the pilot's core control. |
| Organizers | **1** real Kabaddi organizer (the learning subject) | The whole point: a real, non-engineer organizer runs it. |
| Ops / on-call | **1** (the engineer) on site or on call | Owns runbooks, dashboard, restore. |
| Spectators | **Real, unbounded via the public link** | Free, high-value signal (share-link opens) at zero risk — read-only surface. |
| Walkover | **Plan at least one** (a no-show, or stage one) | Validate the FE-8A/8B walkover→auto-advance path with real humans. |

**Explicitly NOT:** group+knockout (adds the resolve-qualifiers step — defer), multi-court (adds concurrent scoring), large field, or anything needing GP-2/personas/reputation.

---

## Part 2 — Success Criteria (objective thresholds)

**Hard pass/fail (binary — any failure = pilot did not succeed):**
- ✅ Tournament completed end-to-end through the UI (create → … → complete) with **0 manual SQL / admin shortcuts**. *(>0 manual DB interventions = FAIL — this is the headline metric.)*
- ✅ **0 score loss** and **0 data/bracket/standings corruption** — final scores in the DB match the physical scoreboard for every match; bracket advanced correctly; standings correct.
- ✅ **0 P0 incidents**; any incident recovered within its runbook's max time (Part 6) with **0 unrecoverable data**.
- ✅ The public link works for spectators **without accounts**; no private/PII leakage observed.

**Graded targets (measured, inform the go-forward):**
- Organizer satisfaction **≥ 4/5**; active-scorer satisfaction **≥ 4/5**.
- Live-event support requests to the operator **≤ 5**, none blocking for **> 5 min**.
- Offline-scoring recoveries (if any) reconcile with **0 duplicate / 0 lost** events.
- Restore **not** needed; if needed, completed within RTO (≤ 60 min) with ≤ RPO loss (≤ 5 min).
- Share link: **≥ 20 spectator opens** (evidence the public surface delivers adoption value).

---

## Part 3 — Pre-Pilot Checklist (100% green before the event)

**Infrastructure (PHI-1A)**
- ☐ Production stack deployed (`deploy/docker-compose.prod.yml`), `api`/`web`/`caddy` healthy.
- ☐ Domain + **Cloudflare** proxied; **TLS Full(strict)** with origin cert; Always-Use-HTTPS on.
- ☐ `:9090` internal observability **not** publicly reachable (verified).
- ☐ CDN cache rule on `app/t/*`; real client IP visible in API logs (trusted-proxy chain OK).

**Email**
- ☐ Provider (Resend/SES) domain **verified** (SPF/DKIM/DMARC); test register → verification email **lands in inbox** (not spam); password-reset tested.

**Data safety (PHI-1B)**
- ☐ Managed Postgres **PITR enabled** (window confirmed); nightly logical backup cron live + a fresh dump exists.
- ☐ **Restore drill re-run this week** (`restore-drill.sh`) → PASS recorded.

**Observability & alerting (PHI-1B)**
- ☐ Prometheus scraping; **Pilot Day dashboard** loads; Alertmanager → **Slack delivers a live test alert**.
- ☐ **UptimeRobot** monitors green on API, frontend, and a public tournament page.
- ☐ `BackupStale`/`APIUnavailable`/`DatabaseUnavailable`/`LiveMatchesNotScoring` confirmed wired.

**Public surface (PRI-1)**
- ☐ A throwaway public tournament loads logged-out (overview/fixtures/bracket/standings/results/match); private → 404; WhatsApp unfurl renders; 320px clean.

**Devices & network (PHI-1C protocol — Part 4)**
- ☐ Device matrix + offline/lifecycle protocol **executed and signed off** (the current Reliability-🟡 gate).
- ☐ Venue network checked: scorer device gets workable connectivity; mobile-data fallback confirmed.
- ☐ Scorer device: charged, battery-saver behavior tested, screen-timeout extended, browser updated.

**Accounts & setup**
- ☐ Organizer account created + verified; org created; **8 teams + approved registrations** pre-loaded.
- ☐ Tournament pre-created in **draft**; fixtures generation rehearsed on staging (not yet generated in prod, or generated and left at `registration_closed`).
- ☐ Scorer account created with the scorer role.

**Process**
- ☐ **Walkover dry-run** done on staging (organizer knows where the action is + that it's terminal).
- ☐ Runbooks printed/open: `api-down`, `database-down`, `email-outage`, `public-outage`, `scorer-incident`, `rollback`, `restore`.
- ☐ Organizer briefed: **one scorer per match**; how to set visibility **Public** + copy the share link; the "set tournament to Ongoing" step.

---

## Part 4 — Human Validation Protocol (real-world, run T-3..T-1)

Distilled from `deploy/runbooks/pilot-validation-protocol.md`. Run on the **actual** scorer device(s). **All rows must PASS to clear the Reliability gate.** Pass = score reconciles to `/score` with **no loss, no duplicate**, and completion succeeds.

| # | Device | Scenario | Pass criterion |
|---|---|---|---|
| 1 | **Android Chrome** | score 5 online | events sync; score matches |
| 2 | **iPhone Safari** | score 5 online | events sync; score matches |
| 3 | Both | **offline** (airplane), score 5, re-enable | all 5 reconcile exactly once |
| 4 | Both | **reconnect** mid-event | queue flushes; no dup |
| 5 | iPhone Safari | **screen lock** during a raid, unlock | state restored; no loss |
| 6 | Both | **sleep/resume** (background 10 min) | SSE reconnects, score re-fetches, queue flushes |
| 7 | Both | **backgrounding** (switch apps, return) | no duplicate submission |
| 8 | Both | **browser refresh** mid-match | scoreboard + pending queue restored |
| 9 | Any | **WhatsApp share** of public link | unfurls as a rich card; opens to live page |
| 10 | Any | **public pages** at 320px | fixtures/bracket/standings render, no overflow |
| 11 | Ops + scorer | **API restart during a live match** (staging) | scorer keeps scoring offline; on restart events reconcile; **0 loss** |

Sign-off (tester/date/devices/result) required before Go.

---

## Part 5 — Pilot-Day Operations Plan

| When | Actions | Owner | Verify | Rollback |
|---|---|---|---|---|
| **T-7d** | Recruit/confirm friendly organizer + 8 teams; book single court; freeze scope | Eng | organizer committed | postpone |
| **T-3d** | Run Part-4 device protocol; re-run restore drill; live-test Slack alert + uptime | Eng | all rows PASS; alert received | fix before proceeding |
| **T-1d** | Pre-create org/teams/registrations/tournament(draft); print runbooks; brief organizer & scorer (one-scorer rule, share flow, Ongoing step); charge devices | Eng + Organizer | checklist 100% green | no-go if any ☐ |
| **Morning** | Final health: dashboard green, uptime green, backup fresh; generate fixtures → review → set **Ongoing**; set visibility **Public**, share link in team WhatsApp | Eng (gen) / Organizer (share) | bracket correct; public link loads logged-out | regenerate (only pre-play); keep visibility private until ready |
| **Live** | Organizer runs matches; **one** scorer per match scores live → complete; auto-advance observed; walkover used if a no-show; Eng watches **Pilot Day dashboard** + Slack | Organizer + Scorer; Eng monitors | each completed match's score matches the board; bracket advances; standings update | per Part-6 runbooks |
| **Complete** | Final completes → champion shown on public page; confirm standings/bracket final; take a manual backup snapshot | Eng | public results correct end-to-end | restore if corruption found |
| **Post** | Run feedback interviews + form same day; export pilot metrics; write retro | Eng | data captured | — |

---

## Part 6 — Incident Response Plan (who / what / max recovery)

| Incident | Who acts | What happens | Max acceptable recovery |
|---|---|---|---|
| **API outage** | Eng | `api-down.md`: restart container; scorers keep scoring offline | **5 min** (scoring never lost meanwhile) |
| **Database outage** | Eng | `database-down.md`: restart/reconnect; PITR restore if sustained | restart **5 min**; restore **≤ 60 min**, ≤ 5 min data |
| **Internet outage (venue)** | Eng + Organizer | scorer continues **offline**; switch device to mobile data; spectators wait | scoring **0 loss**; connectivity **≤ 15 min** |
| **Scorer issue** (lost device / double scorer) | Organizer | `scorer-incident.md`: one scorer designated; backup takes over from `/score` | **5 min**; 0 loss for the active device |
| **Wrong score** | Scorer | in-app undo / score-correction (new correcting event) | **2 min**, before completion |
| **Wrong walkover / wrong completion** | Eng | terminal → operator DB correction (signed-off), then recompute | **15 min**; announce to organizer |
| **Public page outage** | Eng | `public-outage.md`: is it really *private*? else web/CDN check | **10 min** (spectators are read-only) |
| **Email outage** | Eng | `email-outage.md`: fix provider, or share verification links manually | onboarding **≤ 15 min**; not match-blocking |

Rule: **no ad-hoc SQL during the event without the on-call's sign-off.**

---

## Part 7 — Feedback Collection (same-day, before memory fades)

- **Organizer interview (15 min):** Where did you hesitate? What did you expect that wasn't there? Did you trust the scores/bracket? Would you run another? Would you pay? *(Prioritize: confusion, trust, missing workflows.)*
- **Scorer interview (10 min):** Was scoring fast enough live? Did you ever fear losing data? Any moment of "is this saved?" Offline behavior felt safe? *(Prioritize: friction, trust.)*
- **Spectator micro-survey (2 questions, via the public page footer or WhatsApp):** Could you find fixtures/your team/the result? Would you open it again?
- **Feedback form (organizer + scorer):** 1–5 CSAT + free-text on confusion / friction / what was missing / operational pain.
- **Operator retro (Eng):** every support request, manual intervention, runbook activation, alert — with timestamps.

Theme every note into: **Confusion · Friction · Trust · Missing-workflow · Operational-pain.**

---

## Part 8 — Pilot Metrics (capture; star = decision-driving)

| Metric | Source | Why |
|---|---|---|
| ★ **Manual DB interventions** | operator log | must be **0** — the core "self-service" proof |
| ★ **Score loss / corruption events** | scoreboard vs DB | must be **0** — the trust proof |
| ★ **Organizer & scorer CSAT** | survey | drives "would a real organizer adopt this" |
| ★ **Support requests (count + topic)** | operator log | maps directly to the next backlog (Part 9) |
| Matches scored / events recorded | dashboard (`matches_live`, `match_events_last_minute`) | activity baseline |
| Offline recoveries + dup/loss count | scorer report + reconcile | reliability evidence in the field |
| Public page views / share-link opens | Cloudflare analytics | adoption-loop value |
| Manual interventions / runbook activations / alert activations | operator log + Slack | operational maturity |
| Recovery times (per incident) | operator log | validates RTO targets |
| False-positive alerts | Slack | tune thresholds |

The starred five decide everything. Everything else is context.

---

## Part 9 — Post-Pilot Decision Framework (evidence → next initiative)

Pick the next initiative from the **observed pilot evidence**, not a roadmap guess:

| If the pilot shows… | Then do… | Rationale |
|---|---|---|
| Organizers/players repeatedly ask *"can players see their own history / profile?"*; spectators want to follow a **player** not just a tournament | **A. GP-2 Player Persona** | Real demand for the player-first pivot; the public surface is the proven on-ramp |
| Any concurrent-scorer confusion/divergence, or demand to run **multiple courts**/matches at once | **B. Concurrent-scorer lease** | The one deferred P1 becomes mandatory the moment single-stream discipline can't hold |
| Friction dominated by **scheduling/times/venues**, bulk ops, or format gaps (double-elim) | **C. Additional organizer tooling** (incl. FE-8D scheduling) | Sharpen the proven core for the next, bigger tournament |
| Strong adoption + **repeat** tournaments + a growing body of completed results, players asking *"how good am I / where do I rank?"* | **D. Reputation** | Only meaningful once there's a corpus of completed, real competition |
| Pilot reveals an operational/reliability gap (recovery slow, alerts noisy, a corruption path) | **E. Another hardening cycle** | Stabilize before scaling |

**Sequencing rule:** B unblocks scale; A unblocks the player ecosystem; A must precede D (reputation needs players + a corpus). Let the starred metrics + interview themes rank these — do not start GP-2 until the pilot produces a clear *player-demand* signal.

---

## Part 10 — Recommendation

1. **Pilot structure:** one real organizer, **8-team single-court knockout, 7 matches, ~half day, 1 active scorer**, real spectators via the public link, ≥1 walkover. Smallest field that exercises the whole pipeline while structurally avoiding the only open P1.
2. **Readiness:** 🟢 across Architecture/Operations/Security/Observability/Supportability; **Reliability 🟡** solely pending the Part-4 device run. No P0; no known corruption paths.
3. **Remaining risks:** (a) iOS-Safari/offline browser quirks — *closed by Part 4*; (b) concurrent-scorer divergence — *closed by single-stream + one-scorer discipline*; (c) venue connectivity — *mitigated by offline queue + mobile-data fallback*; (d) operator availability — *one on-call engineer required on the day*; (e) accidental terminal walkover/abandon — *covered by runbook + operator correction*.
4. **Required human validation:** execute the Part-4 protocol on the real Android-Chrome and iPhone-Safari scorer devices (esp. offline, reconnect, screen-lock, sleep/resume, refresh) + the staging "API restart mid-match" — sign off before Go.
5. **Go/No-Go:** **Conditional GO.** GO when (1) the Part-4 device protocol passes, (2) the Part-3 checklist is 100% green, (3) the organizer is briefed on one-scorer-per-match. Until then: **No-Go.**
6. **Exact next action:** **schedule the ~half-day device-validation session (Part 4) on the real scorer phones, and recruit one friendly Kabaddi organizer + 8 teams.** Those two actions, run in parallel, are the entire remaining path to the first pilot — no further engineering required.

---

**Evidence to collect before GP-2 begins:** the starred metrics (0 manual interventions, 0 score loss, organizer/scorer CSAT, support-request topics) **plus** a clear, repeated **player-demand** signal from the interviews. GP-2 starts only when the pilot proves organizers will run tournaments *and* that players want to own their record — not before.

**Stops here.** No GP-2/Reputation/Recruitment/FE-8D. No code. Nothing committed. PROJECT_STATE untouched.
