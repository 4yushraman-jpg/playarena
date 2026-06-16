-- =============================================================================
-- Migration  : 000030_tournament_visibility (UP)
-- Description: PRI-1 — adds a tournament-level visibility dimension powering the
--              public (unauthenticated) read surface. The public path authorizes
--              purely through SQL visibility constraints; this column is the root
--              of that gate.
--
--                private  — never publicly accessible (default; explicit opt-in
--                           required to share, so nothing leaks by accident).
--                unlisted — reachable by exact link; not indexed/discoverable.
--                public   — reachable by link AND indexable/discoverable.
--
--              Public eligibility (enforced in every public query, never in app
--              code): status <> 'draft' AND visibility IN ('unlisted','public')
--              AND owning organization is active. Draft is always private.
-- Depends on : 000008 (tournaments)
-- =============================================================================

CREATE TYPE tournament_visibility AS ENUM ('private', 'unlisted', 'public');

COMMENT ON TYPE tournament_visibility IS
    'Public-surface visibility (PRI-1). private = never public; unlisted = '
    'link-only, not indexed; public = link + discoverable/indexed. Draft '
    'tournaments are private regardless of this value.';

-- Default 'private': a tournament is never exposed until the organizer
-- deliberately shares it. This is the safer posture for a results system that
-- displays participant names publicly.
ALTER TABLE tournaments
    ADD COLUMN visibility tournament_visibility NOT NULL DEFAULT 'private';

COMMENT ON COLUMN tournaments.visibility IS
    'PRI-1 public-surface gate. Combined with status<>draft and an active org, '
    'determines whether the tournament is readable on the anonymous public path.';

-- Partial index supporting the public lookup by (org, slug): only rows that can
-- possibly be public are indexed, keeping it small.
CREATE INDEX idx_tournaments_public_lookup
    ON tournaments (organization_id, slug)
    WHERE visibility IN ('unlisted', 'public') AND status <> 'draft';
