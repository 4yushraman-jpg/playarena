# PRI-1 — Public Tournament Surface & Pilot Readiness — Implementation Report

**Status:** Implemented. Review-ready. Not committed. PROJECT_STATE not updated.
**Scope:** PRI-1 only — public read surface, neutral onboarding, shareability, visibility model, disqualification policy, pilot hardening. **No** GP-2 / player profiles / reputation / recruitment. **No** new auth scope, **no** JWT/`DeriveScope`/`IsPlatformUser` change, **no** weakening of `RequireOrgScope` or tenant isolation.

---

## 1. Architecture compliance

| Mandate | How it is met |
|---|---|
| **Separate principal-less read path** | New `internal/public` module: own routes, handlers, DTOs, queries, tests. Mounted **outside** every authenticated group in `bootstrap/modules.go` — **no** `RequireAuth`, **no** `RequireOrgScope`, no principal ever constructed. |
| **No service reuse / no middleware bypass** | The public repo calls only whitelisted public queries + pure batch reads (name lookups, completed-match feed). It never imports or invokes an org-scoped service. |
| **Authorization via SQL, not principal checks** | Every entry query carries the gate `status<>'draft' AND visibility IN ('unlisted','public') AND organization.status='active'`. A failing row simply isn't returned. |
| **404 not 403 (no existence leak)** | The module exposes a single `ErrNotFound`; private/draft/inactive-org/unknown all map to 404, indistinguishably. |
| **GP-1 preserved** | No change to scopes, `DeriveScope`, `IsPlatformUser`, onboarding scope, or `organization.create` gating. Neutral onboarding is a frontend routing change on the unchanged token. |
| **GET-only, rate-limited, cacheable** | Public routes are GET-only with a dedicated per-IP `publicLimiter` and `Cache-Control: public, max-age=20`. |

---

## 2. Files created

**Backend**
- `db/migrations/000030_tournament_visibility.{up,down}.sql` — `tournament_visibility` enum + `tournaments.visibility` column + partial public-lookup index.
- `db/queries/public.sql` — `GetPublicTournament`, `ListPublicMatches`, `GetPublicMatch`, `ListPublicMatchEvents` (all whitelisted, SQL-gated). (+ generated `db/sqlc/public.sql.go`.)
- `internal/public/{errors,dto,repository,service,handler,routes,util}.go` — the public module.
- `internal/public/integration/{testmain,server,public}_test.go` — public integration suite.

**Frontend**
- `types/api/public.ts`, `lib/api/public.ts` (fetch-based, no auth, SSR-capable).
- `app/(public)/t/[orgSlug]/[tournamentSlug]/` — `layout.tsx` (+ OG `generateMetadata`), `page.tsx` (overview), `fixtures/`, `bracket/`, `standings/`, `results/`, `matches/[matchId]/`.
- `components/public/{public-tabs,public-fixtures,public-standings,public-bracket,public-match-detail}.tsx`.
- `components/tournaments/share-tournament.tsx` (Share + visibility control).
- `app/(app)/welcome/page.tsx` (neutral landing).
- `components/public/__tests__/public.test.tsx`.

## 3. Files modified

**Backend**
- `db/queries/tournaments.sql` — `UpdateTournament` sets `visibility`; new `ListStandingsRegistrations` (approved + disqualified) in `tournament_registrations.sql`.
- `internal/tournaments/{dto,service,errors,handler}.go` — `visibility` on `UpdateRequest`/`Response`, `parseVisibility`, `ErrInvalidVisibility`, response mapping; `GetStandings` + snapshot now use `GetStandingsRegistrations` and set the disqualified flag.
- `internal/standings/{models,engine,tiebreakers}.go` — `Disqualified` on `RegistrationInfo`/`StandingsRow`; drop DQ-before-play, retain+flag+rank-last DQ-after-play.
- `internal/bootstrap/{app,router,modules}.go` — `publicLimiter` lifecycle + mount `public.RegisterRoutes` outside the authed groups.

**Frontend**
- `app/(auth)/login/page.tsx` — onboarding no-org users route to `/welcome` (was `/onboarding`).
- `app/(app)/[orgSlug]/tournaments/[id]/page.tsx` — mount `ShareTournament` (canUpdate).
- `types/api/tournaments.ts` — `TournamentVisibility` + `visibility` on `Tournament`/`UpdateTournamentRequest`.
- `lib/query-keys.ts` — (FE-8C) generation key (pre-existing in tree).
- Two test factories + the login test updated for the new fields/redirect.

## 4. Migration details

`000030_tournament_visibility` (additive, independent of GP/auth):
- `CREATE TYPE tournament_visibility AS ENUM ('private','unlisted','public')`.
- `ALTER TABLE tournaments ADD COLUMN visibility ... NOT NULL DEFAULT 'private'` — **default private**: nothing is exposed until the organizer deliberately shares (safer posture for a results system showing participant names). *(Note: the PRI-1 design doc suggested `unlisted` as default; the stricter `private` was chosen at implementation for explicit opt-in.)*
- Partial index `idx_tournaments_public_lookup ON (organization_id, slug) WHERE visibility IN ('unlisted','public') AND status <> 'draft'` — small, supports the public lookup.

## 5. Tests added

**Backend**
- `internal/standings` unit: DQ-before-play removed; DQ-after-play preserved, flagged, ranked last, opponent result intact.
- `internal/public/integration`: unlisted/public accessible; private → 404; draft → 404 (even if visibility public); inactive org → 404; unknown → 404; overview/matches whitelist (no `created_by`/`settings`/`generation`/PII); names resolved in matches; standings computed; standings on private → 404; **cross-org isolation** (org B slug + org A tournament slug → 404).

**Frontend**
- `components/public`: fixtures (score, W/O, results-only filter), standings (table + disqualified flag), bracket (knockout rounds, group matches excluded), match detail (scoreboard/winner/timeline), Share control (link shown when shareable, hidden+explained for draft, visibility change calls update).
- `login` test updated: onboarding → `/welcome`.

## 6. Validation results

- Backend: `go build ./...` ✓, `go vet ./...` ✓, `gofmt` clean; `internal/public/integration`, `internal/standings`, `internal/tournaments/integration` green (Docker). *(Whole-suite `go test ./...` on this Windows box hits the known testcontainers "rootless Docker" parallel limit — suites pass in isolation; CI/Linux runs them together.)*
- Frontend: `tsc --noEmit` ✓ (0), `pnpm lint` ✓ (0). `pnpm test` + `pnpm build`: see run output (in progress at time of writing — to be confirmed green).

## 7. Security findings & 8. Remediations (adversarial review)

Attacked: public visibility, tenant isolation, slug enumeration, draft leakage, PII leakage, service reuse, empty-org platform exemption, cross-org reads.

| # | Vector | Severity | Finding → Remediation |
|---|--------|----------|------------------------|
| S1 | **Empty-org platform exemption** | P0 | Anonymous callers have the same empty-org shape org services treat as platform-admin. **Remediated by construction:** the public module never calls org services and never builds a principal; it's mounted outside the authed groups. Asserted by integration tests (anonymous, no Authorization header) + cross-org isolation test. |
| S2 | **Draft leakage** | P0 | A draft (organizer working state) must never be public. **Remediated:** SQL gate `status<>'draft'`; test confirms a draft with `visibility=public` still 404s. |
| S3 | **PII / private-field leakage** | P0 | **Remediated:** every query selects an explicit whitelist (no `SELECT *`); DTOs carry no `created_by`/`settings`/`generation`/emails/registrant data. Test scans response bodies for forbidden fields. |
| S4 | **Cross-org read** | P0 | **Remediated:** queries resolve a single tournament by (org slug, tournament slug); test proves org B's slug cannot surface org A's tournament. |
| S5 | **Existence leak / slug enumeration** | P1 | **Remediated:** 404 (never 403) for private/missing — no private-vs-missing distinction; slugs (not sequential ids) + a per-IP read limiter + short cache reduce scraping. |
| S6 | **Inactive-org exposure** | P1 | **Remediated:** gate includes `organization.status='active'`; suspended-org test 404s. |
| S7 | **Stale token / refresh on public path** | P1 | **Remediated:** the frontend public client uses plain `fetch` (no Authorization header, no axios refresh interceptor) so anonymous visits never trigger auth flows. |
| S8 | **Settings (generation audit) exposure via standings** | P1 | The public point system needs `settings`, which also holds the generation audit. **Remediated:** settings are read server-side **only** to derive the point numbers and are never serialized; the overview/standings payloads contain no `settings`/`generation` (whitelist test covers it). |

**All P0 and P1 findings are remediated and covered by tests.**

## 9. Remaining P2 findings (documented, non-blocking)

- **P2-1** Unlisted slugs are guessable (org+tournament slug). Mitigated by 404-on-private, rate limiting, and `noindex`; an opaque share token is a future hardening if needed.
- **P2-2** Unresolved group→knockout qualifier slots render as "TBD" publicly (qualifier metadata is intentionally not whitelisted). Acceptable; spectators see TBD until resolution.
- **P2-3** No public tournament directory/discovery yet — `public` visibility is indexable but there is no in-app listing. By design for PRI-1 (link-first).
- **P2-4** Match-detail timeline exposes effective scoring events (whitelisted, capped at 500); very long matches truncate. Acceptable for a spectator summary.
- **P2-5** Disqualification group standings use standard 3/1/0 only when custom points absent — consistent with the public point-system parse; documented.

## 10. Pilot readiness assessment

**Now possible end-to-end:** an organizer creates a tournament → generates fixtures → runs matches → **opens the Share control, sets Unlisted/Public, copies one link** → a player/parent/spectator opens it with **no account** and sees overview, fixtures, bracket, standings, results, and any single match (with a shareable rich-link preview via OG tags). A brand-new sign-up lands on a neutral `/welcome` and is **never forced** to create an organization.

**Hardening status (Deliverable 9):**
- ✅ Public route observability — `publicLimiter` is metric-labelled ("public"); public routes flow through the existing Prometheus HTTP metrics middleware.
- ✅ Public route rate limiting — dedicated per-IP limiter + cacheable responses.
- ✅ Disqualification policy — implemented + unit-tested (Deliverable 10).
- ⏳ **Operational items deferred to deployment (not code):** verified SES email deliverability (domain/DKIM/SPF), backup/restore + runbook, scorer real-device dry run. These are environment/process tasks for the pilot runbook, not PRI-1 code, and are the remaining gate before the live pilot.

**Verdict:** the two strategic adoption blockers (no public surface; forced org onboarding) are removed in code and tested. PlayArena can now be shared with the world; the residual pilot work is operational (email/backup/runbook/device QA), to be completed as part of standing up the pilot environment.

**Stopped at PRI-1.** GP-2 / Reputation / Recruitment / FE-8D not started. Nothing committed; the working tree contains only PRI-1 changes (plus the prior, already-reviewed FE-8A/8B/8C work that remains uncommitted in this session).
