# Decision Record — Ranking System (Reputation vs Rating)

**Status:** Proposed (recommendation below). Design only — no code, no roadmap changes.
**Date:** 2026-06-12.
**Scope:** Evaluates the ranking *model* only. Refines, does not alter, Section 4 of [gp-blueprint-kabaddi.md](gp-blueprint-kabaddi.md).
**Question:** Should PlayArena rank players by (1) Reputation Points only, (2) Elo/Glicko only, or (3) both?

---

## 1. The fundamental distinction

The three options are not three flavors of one thing — they measure **two different quantities**:

- **Reputation Points (RP)** measure *accumulated accomplishment* — effort × success integrated over time. Cumulative, monotone-friendly, "begins at 0." Answers **"how much have you achieved on PlayArena?"**
- **Elo/Glicko Rating** measures *current relative skill* — predictive of head-to-head outcomes. Zero-sum (Elo) or uncertainty-aware (Glicko), self-correcting, "begins at the mean." Answers **"how strong are you right now?"**

Every criterion below turns on which question the product actually needs answered — and at what stage.

Two structural facts about this specific ecosystem dominate the analysis:
- **Stage:** pre-launch, low volume, all begin equal, closed ecosystem.
- **Graph shape:** competition is **tournament-clustered and historically org-siloed**. The "who-played-whom" graph is sparse and poorly connected early on — which is exactly the condition under which skill ratings are unreliable and reputation/placement currencies still work.

---

## 2. Option-by-option analysis

### Option 1 — Reputation-only (current proposal)

| Criterion | Assessment |
|---|---|
| **Abuse resistance** | **High**, given the blueprint gates. Cumulative + **non-resettable** makes profile-abandonment pointless (kills reset-evasion). RP only from `ranked_eligible` verified-org events; opponent-strength weighting + diminishing returns + zero RP for walkovers neutralize seal-clubbing and farming. Residual: in-org collusion / fabricated results (an organizer-trust problem, not a rating-math problem). |
| **Competitive integrity** | **Moderate.** Honest measure of *accomplishment*, weak measure of *skill*. A very active mid-tier player can outrank a dominant but less active one. Does not predict head-to-head outcomes. Strength-of-schedule is only a weighting heuristic, not native. |
| **Leaderboard quality** | **Good for engagement**, biased toward activity at the top. Inflates over time → needs seasons/decay. A genuinely elite newcomer cannot reach #1 quickly regardless of talent (must accumulate volume). |
| **Onboarding impact** | **Excellent — its best dimension.** Start at 0, every event only adds, no rating-loss anxiety, clear climb. Actively encourages participation. |
| **Tournament impact** | **Mixed.** Drives entering many events (platform liquidity ↑). But RP ≠ skill, so it gives organizers **no real seeding signal** — seeding by RP seeds by activity, producing lopsided early rounds. |
| **Long-term scalability** | **Operationally simple** (pure idempotent fold over immutable facts — trivially recomputable after rule changes). Needs ongoing inflation/decay/weighting calibration. |

### Option 2 — Elo/Glicko-only

| Criterion | Assessment |
|---|---|
| **Abuse resistance** | **Weakest.** Zero-sum rating is the *most* exploitable: smurfs farm rating off weak players; boosting/throwing transfers rating precisely and efficiently. **Abandonment costs nothing** (rating isn't cumulative; a bad-rated player restarts near the mean) — the opposite of the deterrent RP provides. Glicko's RD dampens new-account volatility but does not stop deliberate transfer. |
| **Competitive integrity** | **Strongest — *if* the graph is connected and volume sufficient.** Predictive, self-correcting, transitively comparable. But in a sparse, org-siloed Kabaddi graph, cross-cluster ratings are unreliable until enough inter-org play accumulates. Best ceiling, worst early reliability. |
| **Leaderboard quality** | **High fidelity once mature** (top = strongest), but volatile and **counter-intuitive** to casual users ("why did I drop 30 points for losing?"). Disconnected pools yield questionable cross-comparisons; low-volume players must be hidden by RD threshold. |
| **Onboarding impact** | **Poor — its worst dimension.** New ratings are uncertain/volatile; losing rating discourages; "everyone starts at 1500 and most go down" feels punishing. **Rating-protection suppresses participation** — players avoid risky events to guard their number. Wrong incentive for a growth-stage platform. |
| **Tournament impact** | **Excellent for seeding / balanced brackets** (Elo's killer feature). But suppresses entry into risky events, and **individual Elo doesn't yield a team rating** — team tournaments need a separate team-rating design. |
| **Long-term scalability** | **Algorithmically strong** (no inflation, self-correcting), but heavier ops: needs volume, a connected graph, separate team/individual ratings, RD/volatility tuning, anti-boost detection, and order-dependent updates (less trivially recomputable than a fold; batchable via Glicko rating periods). |

### Option 3 — Reputation + Rating (two tracks)

Design intent: **RP is the headline** (career/leaderboard/recognition); **Glicko Rating is a secondary skill metric** used for seeding/matchmaking and surfaced only once established (RD below a threshold).

| Criterion | Assessment |
|---|---|
| **Abuse resistance** | **Best ceiling, largest surface.** RP headline (non-resettable) keeps reset-evasion pointless; rating, never the sole prize, attracts less boosting. Crucially, the two **cross-check**: high-rating + low-RP = smurf signal; RP/rating divergence = collusion signal. The second metric becomes an **abuse-detection feature**, not just a second target. More to defend, but more eyes. |
| **Competitive integrity** | **Best.** Answers both questions — accomplishment (RP) and skill (rating) — with the right tool for each. |
| **Leaderboard quality** | **Best, if disciplined.** Intuitive RP ladder up front; a "skill rating / form" stat for competitive users; multiple views (all-time RP, season RP, established-rating). Risk: two co-equal numbers confuse casual users — RP must be primary, rating contextual. |
| **Onboarding impact** | **Good *if* RP is the face and rating is hidden until established.** Otherwise it inherits Elo's onboarding pain. Requires UX discipline. |
| **Tournament impact** | **Best.** Rating gives organizers real seeding; RP drives participation. Covers fairness *and* engagement. |
| **Long-term scalability** | **Most capable, highest cost.** Two pipelines, team-rating design, RD tuning, dual-metric UX. The rating track is **low-value early** (sparse graph, low volume) and high-cost — it pays off only at scale. |

---

## 3. Recommendation

**Ship Reputation-only now (Option 1). Architect it as Phase 1 of an eventual Reputation + Rating (Option 3). Explicitly reject Elo/Glicko-only (Option 2) as the sole system.**

### Why not Option 2 (as the sole system)
It is wrong for this platform's *stage and abuse model* on three counts that all bind today: it **punishes participation** (rating-protection) when the platform needs growth; it is the **most smurf-exploitable** option (cheap reset, efficient transfer) when smurfing is the accepted top risk; and it needs a **connected, high-volume graph** the org-siloed ecosystem will not have at launch. Its one genuine strength — seeding — does not require it to be the *ranking* system.

### Why Option 1 now
It matches the product's own language ("build reputation, earn rankings"), has the **best abuse resistance** (with the blueprint's verified-org + non-resettable + anti-farm gates) and the **best onboarding**, and is **operationally trivial** (idempotent fold). Its two real weaknesses — skill fidelity and seeding — are **low-impact at launch**: early tournaments are small and **organizer-seeded by hand**, so algorithmic seeding is a scale problem, not a launch problem.

### Why Option 3 is the destination, not the start
Two co-equal metrics is the right *long-term* answer, but the rating track is **premature**: while volume and inter-org connectivity are low it produces a noisy, unreliable number while adding the most surface to attack and the most to maintain. Build it when the data can support it — not before.

### The decisive cost asymmetry
Choosing RP-only now **forecloses nothing**. Because match results are stored **append-only** and per-tournament stats are **immutable facts**, a Glicko pipeline can be **back-computed over full history** whenever it is added — no re-architecture, no data loss, no migration of the RP system. RP-only is a strictly reversible, low-regret first move toward Option 3.

---

## 4. Trigger conditions to add the Rating track (Option 1 → Option 3)

Add a **Glicko-2** skill rating (not classic Elo) when **all** hold:
1. **Volume:** median ranked matches per active player crosses a threshold (e.g. ≥ ~15–20) so ratings stabilize.
2. **Connectivity:** the inter-organization play graph is sufficiently connected that cross-cluster ratings are meaningful (measurable: largest connected component covers the bulk of ranked players).
3. **Demand:** organizers actually need **algorithmic seeding** / players ask for a skill metric.

**Glicko-2 over Elo** specifically because its rating-deviation and volatility handle **new and intermittently-active players** — exactly Kabaddi's tournament-burst, sparse-play pattern — far better than classic Elo, which assumes steady, frequent play.

When added: keep **RP as the headline ladder**; use rating for **seeding and as a secondary "skill/form" stat**, shown only once RD is below threshold; treat **RP/rating divergence as an anti-abuse signal**.

---

## 5. Design hooks to preserve now (so Option 3 stays cheap later)
- Keep `match_events` append-only and `*_tournament_stats` immutable per-tournament facts (already the design) — these are the substrate a future rating pipeline back-computes from.
- Record, per ranked match, the data Glicko needs (participants, outcome, timestamp/rating-period bucket) — already implied by the match + events model.
- Keep the `ranked_eligible` gate as the single source of "what counts" — both RP and a future rating consume the same gated set, so they never disagree on eligibility.
- Do **not** surface any skill-rating-shaped number at launch; introducing and later redefining one erodes trust. Reputation only, until the triggers fire.

---

## 6. One-line answer
**Reputation-only is the correct launch ranking system; build it as the first phase of an eventual Reputation + (Glicko-2) Rating, and never adopt Elo/Glicko as the sole ranking — it punishes participation, is the most smurf-exploitable, and needs a connected, high-volume graph the ecosystem won't have until much later.**
