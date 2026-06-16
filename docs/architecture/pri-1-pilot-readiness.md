# PRI-1 — Pilot Readiness Initiative — Architecture Design

**Status:** Design only. No code. Review-ready. PROJECT_STATE untouched.
**Goal:** Make PlayArena pilot-ready by (a) giving every tournament a public, shareable, read-only surface and (b) removing the forced-organization-creation onboarding — **without** activating players, adding an auth scope, or weakening GP-1.

**Hard constraints honoured throughout:** `GP_PLAYER_PERSONA_ENABLED` stays FALSE; no GP-2 / player profiles / reputation / recruitment; **no new auth scope**; GP-1 invariants preserved; organization permissions and isolation unchanged.

---

## 0. The one load-bearing decision

Everything in PRI-1 hangs off a single architectural rule:

> **The public surface is a separate, principal-less read path. It never reuses an org-scoped service, and it never carries an auth principal.**

Why this is non-negotiable: org-scoped services treat an **empty `organization_id` as a platform-admin exemption** from tenant-ownership checks (see `RequireOrgScope` and the org services' platform bypass). An anonymous public caller has *no* principal — exactly the empty-org shape — so routing public reads through existing org code could be read as "platform admin" and expose every org. PRI-1 therefore introduces a dedicated **`internal/public`** module with its own GET-only handlers, its own read-only queries, and **authorization expressed as SQL `WHERE` clauses**, not as service-layer principal checks.

This rule is what makes "public access" safe without touching the auth model.

---

## 1. Public surface — module & routing architecture

**New backend module `internal/public`** (handler / service / repository), mounted **outside** the `/api/v1/organizations/{slug}/...` tree and **without** `RequireAuth` / `RequireOrgScope`. It has its own per-IP rate limiter and emits cacheable responses.

```
GET /api/v1/public/orgs/{orgSlug}/tournaments/{tournamentSlug}            → overview
GET /api/v1/public/orgs/{orgSlug}/tournaments/{tournamentSlug}/fixtures   → fixtures
GET /api/v1/public/orgs/{orgSlug}/tournaments/{tournamentSlug}/bracket    → bracket graph
GET /api/v1/public/orgs/{orgSlug}/tournaments/{tournamentSlug}/standings  → standings table
GET /api/v1/public/orgs/{orgSlug}/tournaments/{tournamentSlug}/results    → completed results
```

Addressing is by **(org slug, tournament slug)** — both human-readable, both already unique (`uq_tournaments_org_slug`), neither a sequential id (so no enumeration of ids). The org slug also provides natural branding context.

**Frontend:** a new **`(public)` route group** with **no auth guard** (parallel to `(auth)` and `(app)`), served at a short shareable path:

```
/t/{orgSlug}/{tournamentSlug}             (overview, with tabs)
  ?tab=fixtures | bracket | standings | results   (deep-linkable)
```

Public pages **reuse the existing render components in read-only mode** (the bracket renderer with `next_match` wiring, the standings card, the fixture list, status badges). The crucial difference: a public page cannot call org-scoped team/player lists to resolve names, so **the public API embeds resolved display names** in every payload (the standings DTO already does this via `participant_name`; fixtures/bracket payloads gain the same). No client-side name resolution against private data.

---

## 2. The five public views (field whitelist)

Every public query **SELECTs only whitelisted columns** — never `SELECT *`, never private/PII/operational fields.

### 2.1 Public tournament page (overview)
**Exposes:** tournament name, sport, format, participant_type, status, banner_url, description, rules, venue/city/country, start/end dates, prize_pool/currency *(organizer-controlled, see §4)*, organizer name/slug/logo (branding), participant count (approved only), and a `visibility` echo. **Hides:** `created_by`, the `settings.generation` audit object, registrant identities/PII, registration_counts breakdown, internal notes, audit data.

### 2.2 Public fixtures
**Exposes per match:** round_number, round_name, match_number, group_label, **home/away display names** (server-resolved; TBD → "TBD", qualifier → "Group A #1"), scheduled_at, venue, status, final score (completed/walkover only), is_walkover, winner name. **Hides:** participant org, notes, metadata, `recorded_by`, the event log, internal ids beyond what the client needs to draw the tree.

### 2.3 Public bracket
**Exposes:** the knockout graph — matches with their FE-8B `next_match` linkage (so the tree draws), participant/TBD/qualifier slot labels, scores, winners, round names. Qualifier `{group,rank}` labels are non-sensitive and shown ("Group A #1"). Reuses the exact structure FE-8B/8C already produce; the public renderer is the read-only twin of the organizer bracket view.

### 2.4 Public standings
**Exposes:** the computed table — position, participant **name**, played/W/L/D, points, score for/against/difference. Computed by the **existing pure standings engine** over completed+walkover matches (FE-8A semantics intact). No raw participant UUIDs needed for display.

### 2.5 Public results
A filtered, completed-only view: concluded matches with final scores, walkover markers, winners, and — once the tournament is `completed` — the **champion** (final's winner) surfaced prominently. This is the shareable "who won" artifact.

---

## 3. Visibility model

A tournament-level **`visibility`** dimension (recommended small migration: enum `visibility ∈ {private, unlisted, public}`, default `unlisted`).

| | **private** | **unlisted** | **public** |
|---|---|---|---|
| Reachable by direct link | ✗ | ✓ | ✓ |
| Appears in a future public directory / indexed by search | ✗ | ✗ | ✓ |
| `noindex` sent to crawlers | n/a | ✓ | ✗ |

**Gate (enforced in SQL, every public query):**
```
publicly readable  ⇔  status <> 'draft'
                    AND visibility IN ('unlisted','public')
                    AND owning organization.status = 'active'
```
- **Draft is always private** — it is the organizer's working state, regardless of `visibility`.
- **Default `unlisted`** — the instant a tournament leaves draft it is link-shareable but not discoverable or indexed: a safe default that needs no organizer action to start sharing, yet leaks nothing to crawlers.
- **`private`** is the explicit off switch; **`public`** opts into discovery/indexing (a future public directory; PRI-1 ships the flag and the `noindex` behavior, not the directory).
- **Suspended org → hidden** (org status gate), so a suspended tenant's data cannot be read publicly.
- **Cancelled tournaments remain viewable** (showing "cancelled") if their visibility allows — transparency for participants who were told an event was on.

**404, never 403.** A non-public or non-existent tournament returns **404** from the public API — the public path must not distinguish "exists but private" from "does not exist," preventing enumeration of private tournaments.

The migration is additive and independent of GP/auth; it touches only the `tournaments` table.

---

## 4. Shareable links

- **Canonical public URL** per tournament: `/t/{orgSlug}/{tournamentSlug}` — short, stable, human-typable, shareable over WhatsApp/SMS (the dominant channel for grassroots Kabaddi in India).
- **Organizer "Share" control** on the org-side tournament detail page: shows the public URL + copy button **and** the `visibility` selector (private / unlisted / public). When `status=draft` or `visibility=private`, it shows "Not publicly visible yet" with the one toggle to enable. Setting visibility goes through the existing org-scoped `tournament.update` permission (no new authz).
- **Open Graph / link-preview metadata** server-rendered on public pages (title = tournament name, description, banner image): a shared link unfurls into a rich card. This is a deliberate **growth-loop** lever — a good preview is what makes a forwarded link get clicked. OG tags render for `unlisted` and `public`; `unlisted` additionally sends `noindex` (preview works, search indexing does not).
- **Deep links** to each tab (`?tab=bracket`, etc.) so an organizer can share "here are the live standings" specifically.
- The org-scoped tournament response gains a computed `public_url` + `visibility` so the UI can render the share control and the public state.

---

## 5. Security model

| Control | Design |
|---|---|
| **No principal on the public path** | Public routes mount **neither** `RequireAuth` **nor** `RequireOrgScope`. No `AuthUser` is ever constructed, so the empty-org⇒platform exemption cannot be triggered. |
| **Separate read module** | `internal/public` has its **own** repository + queries. It **must not import or call** org-scoped services. The visibility/ownership gate lives in the SQL `WHERE`, not in service authz. |
| **Field whitelist** | Every query selects only the public columns enumerated in §2. No `SELECT *`. No PII (emails, contacts, registrant org, audit, `created_by`, generation audit). |
| **Existence privacy** | Non-public/unknown → **404** (no "private vs missing" distinction). |
| **Org isolation** | Queries are scoped to a single resolved tournament under one org; a public read can never traverse to another org's data. |
| **Rate limiting** | A dedicated per-IP limiter on the public tree (separate from the auth limiter) to blunt scraping. |
| **Caching** | `Cache-Control` + `ETag` on public GETs; short TTL (~15–30 s) for live-changing views (fixtures/standings), longer for static metadata. Cacheable + CDN-frontable, which also absorbs scrape/spike load. |
| **GET-only** | The public tree has **no** write endpoints, ever. |
| **Crawler control** | `noindex` for `unlisted`; indexable for `public`. |

**Explicitly preserved GP-1 / auth invariants:** `DeriveScope`, `IsPlatformUser() ⇔ scope==platform`, the onboarding scope, the org-create gating, and `RequireOrgScope` on every org tree are **unchanged**. PRI-1 adds an anonymous read path and a frontend routing change; it does not modify token issuance, scopes, or any existing authorization.

**Top risks called out:**
1. *Reusing org services for public reads* → mandated separate module (the §0 rule). **The** invariant to test.
2. *Over-exposure* (PII, draft, generation audit) → whitelist + visibility gate + tests asserting absence of private fields.
3. *Enumeration* → slug addressing + 404-on-private + rate limit.
4. *Neutral-landing change altering auth* → the landing is pure frontend routing on the unchanged onboarding token; no backend/auth change.

---

## 6. Neutral onboarding

**Problem (from the audit):** a brand-new user (0 orgs, flag off) gets an `onboarding`-scope token and is routed straight to `/onboarding` ("Create your organization") — forcing org creation on everyone.

**Design (frontend-only; token/scope/gating unchanged):**
- Login with no org + `role==="onboarding"` now routes to a **neutral landing** (`/welcome`) instead of `/onboarding`.
- The neutral landing presents **explicit choices**, not a forced gate:
  - **"Create an organization"** → the existing `/onboarding` org-creation form (backend unchanged; `organization.create` still gated to onboarding + zero-org).
  - **"Browse tournaments"** → the public surface (a logged-in user can view public pages too).
  - **"Player profiles — coming soon"** → an honest, disabled placeholder while the flag is off. It is **scope-aware**, so when GP-2 later flips the flag and issues `player`-scope tokens, this branch activates with zero rework.
- Existing flows are untouched: single-org → auto-select; multi-org → org-select; platform → platform; already-org'd users → their dashboard.

**Why not a binary Organizer/Player chooser now:** with the flag off, no player scope, and no profiles, a "Player" choice dead-ends or implies an activation that violates the constraints. The neutral landing achieves the same de-forcing **and** is forward-compatible with GP-2, without promising anything that does not exist.

**GP-2 compatibility:** the neutral landing is the seam GP-2 plugs into (it gains a live "Player" branch); the public pages become GP-2's "claim your record" entry point. Nothing in PRI-1 has to be undone for GP-2.

---

## 7. Pilot-readiness requirements

Beyond the public surface + neutral onboarding, the pilot ("a real organizer runs a real tournament next month; players/parents get links") needs:

1. **Email deliverability verified** — SES domain + DKIM/SPF so verification/reset emails actually arrive (the onboarding front door). Today this is configured-but-unproven.
2. **Disqualification / withdrawal-after-results policy** *(correctness; decision required).* Recommended minimal, defensible rule for the pilot: a participant disqualified **before** playing is removed from standings (no results); disqualified **after** matches are played **remains** in standings with results intact but **flagged "disqualified"** — never silently rewrite opponents' already-played records mid-event. Document it; surface the flag in standings (public + private).
3. **Backup / restore + a one-page runbook** — this is the system of record for someone's real event.
4. **Bulk fixture scheduling** *(conditional)* — generated matches have no times; only needed if the pilot event is multi-day. Borrow the FE-8D scheduling slice if so; otherwise defer.
5. **Observability on the public tree** — the existing Prometheus/Grafana/health stack should label and scrape the public endpoints (latency, 404 rate, cache hit-rate).
6. **Scorer real-device QA** — the offline/exactly-once scorer is tested in CI but never used on an actual phone at a venue by a non-technical scorer; a dry run is part of pilot readiness.
7. **Terms / privacy note** — public pages display participant (team/player) names; a short privacy statement covers public display of competitive data.

Items 1, 3, 5, 6, 7 are operational; 2 and 4 are scoped engineering decisions inside PRI-1.

---

## 8. Consolidated impact

**Schema** — one additive migration: `tournaments.visibility` (enum, default `unlisted`) + an index supporting the public gate (status/visibility). No GP/auth schema touched.

**Backend** — new `internal/public` module (routes, handler, service, read-only repository), mounted outside the org tree, unauthenticated, GET-only, rate-limited, cached. New public read queries (whitelisted) for overview/fixtures/bracket/results; public standings via the existing engine. Org-side `tournament.update` extended to accept `visibility`; org tournament response gains `visibility` + `public_url`. **No** change to auth, scopes, RBAC, or org services.

**Frontend** — new `(public)` route group + pages (no guard), reusing read-only render components fed by name-embedding public DTOs; OG metadata + `noindex` per visibility; a neutral `/welcome` landing + the login routing change; a Share + visibility control on the organizer tournament page.

**Security** — §5 in full: isolation by separate module, SQL-gate authorization, whitelist, 404-privacy, rate limit, GET-only, GP-1/scope invariants preserved.

**Testing** — public endpoints expose only whitelisted fields and only non-draft/non-private/active-org tournaments; anonymous access cannot reach any org-scoped route or any private/draft/PII data; cross-org public reads cannot leak; 404-on-private (no existence leak); rate-limit + cache headers; the neutral landing does not alter token/scope and org creation still works exactly as before; GP-1 invariant suite stays green.

**Success criteria** — an organizer shares **one public link**; players/parents/spectators open it with **no account** and see live fixtures/bracket/standings/results and the champion; a new sign-up is **never forced** into org creation; GP-1 and all isolation tests remain green; the pilot completes without an "I had to create an org to see the bracket" support ticket.

---

## 9. Sequencing within PRI-1 (suggested build order)

1. **`visibility` migration + org-side Share/visibility control** (smallest, unblocks everything; lets organizers mark a tournament shareable).
2. **`internal/public` module + the five read endpoints** (the core; security-reviewed against §0/§5).
3. **`(public)` frontend pages** reusing read-only components + OG metadata + deep-link tabs.
4. **Neutral onboarding landing** (frontend routing) — independent, can land in parallel.
5. **Pilot-hardening** (email verification, disqualification policy, runbook/backups, optional scheduling, observability labels).

PRI-1 ships ahead of GP-2, Reputation, Recruitment, and FE-8D: it is the prerequisite that makes the pilot runnable and shareable, and it produces the first public competitive records the rest of the roadmap depends on.
