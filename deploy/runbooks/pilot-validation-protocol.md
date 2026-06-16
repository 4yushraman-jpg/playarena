# Pilot Validation Protocol (human-executed)

PHI-1C validates reliability with **evidence**. Two kinds:
- **Automated (executed in CI/dev — results in `docs/architecture/phi-1c-validation-report.md`):** the
  offline queue / reconcile / exactly-once / completion-gate logic, the full
  tournament flow (backend integration), public-surface gating, and the restore
  drill. These prove the *logic* that survives offline/refresh/reconnect.
- **Hardware-dependent (this checklist — must be run by a human on real devices
  before the pilot):** physical screen lock, battery saver, real-radio packet
  loss, Safari storage eviction, WhatsApp/social unfurl, 320px rendering. These
  cannot be executed in the dev environment; they verify browser/OS quirks the
  unit tests can't.

Run every row, tick PASS/FAIL, attach a screenshot/note. **The scorer rows are
go/no-go.**

## Setup
- A staging deployment reachable from phones (the PHI-1A stack).
- One tournament with ≥1 live match; a second device/account for the concurrent test.
- Throttle via Chrome/Safari devtools where a real radio isn't available.

## D2 — Device matrix (open the scorer, record 5 events, complete a match)
| Device / browser | App loads | Score 5 events | Score syncs | Complete match | Notes |
|---|---|---|---|---|---|
| Android · Chrome | ☐ | ☐ | ☐ | ☐ | device model: |
| Android · Samsung Internet | ☐ | ☐ | ☐ | ☐ | |
| iPhone · Safari | ☐ | ☐ | ☐ | ☐ | iOS ver: |
| iPhone · Chrome | ☐ | ☐ | ☐ | ☐ | |
| Desktop · Chrome | ☐ | ☐ | ☐ | ☐ | |
| Desktop · Edge | ☐ | ☐ | ☐ | ☐ | |
| Desktop · Firefox | ☐ | ☐ | ☐ | ☐ | |

## D3 — Network matrix (per device; verify after each: no loss, no dup, score matches `/score`)
| Condition | Procedure | No loss | No dup | Reconciled | Complete OK |
|---|---|---|---|---|---|
| Online | score 5 | ☐ | ☐ | ☐ | ☐ |
| 3G / slow | devtools "Slow 3G", score 5 | ☐ | ☐ | ☐ | ☐ |
| Intermittent | toggle offline/online while scoring | ☐ | ☐ | ☐ | ☐ |
| Offline | airplane mode, score 5, re-enable | ☐ | ☐ | ☐ | ☐ |
| Reconnect | drop mid-event, restore | ☐ | ☐ | ☐ | ☐ |
| High latency | devtools add 2s latency | ☐ | ☐ | ☐ | ☐ |
| Packet loss | (if tooling) 10% loss | ☐ | ☐ | ☐ | ☐ |

## D4 — Mobile lifecycle (score, trigger the event, verify queue+score survive, no dup)
Refresh ☐ · Tab close/reopen ☐ · Browser restart ☐ · Screen lock ☐ · Device sleep ☐ ·
Background app ☐ · Battery saver ☐ · WiFi→Mobile ☐ · Mobile→WiFi ☐ · App switch ☐

## D5 — Live scoring stress (single device)
Rapid 50+ events ☐ · repeated undo ☐ · repeated corrections ☐ · timeouts ☐ ·
penalties ☐ · player events ☐ · long match (≥40 min) ☐ · short match ☐ →
verify queue integrity, event ordering, score correctness, completion.

## D6 — Concurrent scorers (two devices, same live match, score simultaneously)
- Does the **ConcurrentScorerBanner** appear on both? ☐
- Record: observed score divergence, operator confusion, how quickly noticed.
- This is an *assessment*, not a pass/fail — feeds the lease recommendation.

## D7 — Public surface (phone + desktop)
Private → 404 ☐ · Unlisted via link → loads, not indexed ☐ · Public → loads ☐ ·
fixtures/bracket/standings/results/match render ☐ · 320px (no overflow) ☐ ·
WhatsApp paste unfurls (OG card) ☐ · no private data visible (view-source) ☐.

## D8 — Failure injection (during an active match)
For each: note **time-to-detect** (alert/uptime) and **time-to-recover**, and confirm
no score/bracket/standings corruption afterward.
API restart ☐ · Frontend restart ☐ · DB restart ☐ · Email outage ☐ ·
Network interruption ☐ · Browser crash ☐ · Device reboot ☐.

## Sign-off
Tester: ________  Date: ________  Devices: ________  Result: GO / NO-GO
