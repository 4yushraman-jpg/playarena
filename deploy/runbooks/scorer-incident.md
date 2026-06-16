# Runbook — Scorer Incident

The scorer is mission-critical. Scoring is offline-tolerant and exactly-once
(events queue in the browser with a `client_event_id` and reconcile on
reconnect), so most "incidents" are recoverable without data loss.

**Symptoms:** `LiveMatchesNotScoring` alert (live matches, 0 events/min for 10 m);
organizer says "the score isn't updating" / "two of us are scoring" / "my phone died".

**Diagnosis**
- Pilot Day dashboard: "Scoring Events / min" — zero while matches are live?
- Ask the scorer: connectivity? did the tab/app background or the device sleep?
  is more than one person scoring the same match?

**Immediate actions**
- **Lost connectivity / sleep / refresh:** reassure — keep the scorer app open;
  when the network returns the queued events sync automatically (no loss). A page
  refresh restores the pending queue.
- **Dead/replaced device:** the queue is per-device, so any unsynced events on a
  dead device are only recoverable if it comes back online. Have a **second**
  scorer take over on a fresh device from the current authoritative score
  (`GET /score` is server-truth); re-enter only the few events since the last sync.
- **Two concurrent scorers (the known risk):** designate ONE; the others close the
  match tab immediately. The awareness banner flags foreign events.

**Recovery actions**
- Verify the authoritative score (`/score`) matches the physical scoreboard; if a
  wrong event slipped in, correct it via the in-app undo / score-correction (a
  new correcting event — never a delete).
- **Accidental walkover/abandon** (terminal, no UI undo): this needs an operator
  DB correction — escalate (platform admin) to revert the terminal match and
  recompute; do not attempt ad-hoc SQL during the event without sign-off.

**Escalation**
- Operator correction of a terminal match, or suspected event-path bug →
  platform owner.

**Verification**
- Events/min returns >0 for live matches; `/score` matches the physical board;
  `LiveMatchesNotScoring` resolves.
