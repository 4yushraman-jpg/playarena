-- =============================================================================
-- Migration  : 000014_schema_hardening (UP)
-- Description: Remediates all issues found during the post-design schema
--              review of migrations 000001–000013. No existing migration files
--              are modified. All changes are additive or constraint-level.
--
-- CHANGES IN THIS MIGRATION
-- ──────────────────────────────────────────────────────────────────────────
-- Section 1 — FK ON DELETE behavior fixes (4 constraints)
-- Section 2 — UNIQUE + NULL correctness fix (1 partial index)
-- Section 3 — Multi-tenancy leak fixes (1 column + 4 triggers)
-- Section 4 — Missing indexes (8 new + 1 duplicate removed)
--
-- PRODUCTION SAFETY NOTES
-- ──────────────────────────────────────────────────────────────────────────
-- Every ALTER TABLE acquires AccessExclusiveLock for the duration of the
-- statement. On a live system with active traffic:
--   • Run during a low-traffic window, OR
--   • Set lock_timeout = '5s' before the migration to fail fast rather
--     than queue indefinitely: SET lock_timeout = '5s';
-- The backfill UPDATE in Section 3A is O(rows in team_memberships). On an
-- empty or small table this is instantaneous. For large tables, batch-update
-- before running this migration.
-- CREATE INDEX statements below use standard mode (not CONCURRENTLY) because
-- golang-migrate wraps migrations in a transaction and CONCURRENTLY cannot
-- run inside a transaction. For very large tables, extract each CREATE INDEX
-- into a separate non-transactional script and run with CONCURRENTLY.
-- =============================================================================


-- =============================================================================
-- SECTION 1: FK ON DELETE BEHAVIOR FIXES
-- =============================================================================
-- Root cause: four FK constraints were declared without an explicit ON DELETE
-- action. PostgreSQL defaults to NO ACTION, which is effectively RESTRICT.
-- Three of these will silently block organization deletion in production;
-- the fourth leaves the cancels_event_id relationship with ambiguous intent.


-- -----------------------------------------------------------------------------
-- 1A  tournament_registrations.organization_id  → ON DELETE CASCADE
-- -----------------------------------------------------------------------------
-- Why needed:
--   tournament_registrations.organization_id is the REGISTRANT'S organization,
--   not the tournament host's org. If a registrant org is deleted, its
--   registrations have no meaningful owner and must be cleaned up.
--   Without CASCADE, deleting a registrant org raises:
--     ERROR: update or delete on table "organizations" violates foreign key
--     constraint "fk_treg_organization" on table "tournament_registrations"
-- Production impact if left unfixed:
--   Deleting any organization that has ever registered for a tournament
--   (even a tournament hosted by a different org) becomes impossible.
--   An org can never be fully removed from the platform.

ALTER TABLE tournament_registrations
    DROP CONSTRAINT fk_treg_organization;

ALTER TABLE tournament_registrations
    ADD CONSTRAINT fk_treg_organization
        FOREIGN KEY (organization_id)
        REFERENCES organizations (id)
        ON DELETE CASCADE;

COMMENT ON CONSTRAINT fk_treg_organization ON tournament_registrations IS
    'Registrant organization. ON DELETE CASCADE: removing an org removes '
    'all registrations it submitted to any tournament.';


-- -----------------------------------------------------------------------------
-- 1B  matches.organization_id  → ON DELETE CASCADE
-- -----------------------------------------------------------------------------
-- Why needed:
--   matches.organization_id is denormalized from tournaments.organization_id.
--   In theory, the cascade chain (org → tournament → match) handles deletion,
--   but the bare FK with NO ACTION is evaluated alongside the cascade, and
--   FK evaluation order in PostgreSQL is not guaranteed to resolve correctly
--   across all versions and constraint configurations.
--   Explicit ON DELETE CASCADE makes the intent unambiguous and safe.
-- Production impact if left unfixed:
--   Depending on FK evaluation order, deleting an org may raise a FK
--   violation on matches even after tournaments have been cascade-deleted.
--   This creates intermittent, hard-to-diagnose deletion failures in prod.

ALTER TABLE matches
    DROP CONSTRAINT fk_matches_organization;

ALTER TABLE matches
    ADD CONSTRAINT fk_matches_organization
        FOREIGN KEY (organization_id)
        REFERENCES organizations (id)
        ON DELETE CASCADE;

COMMENT ON CONSTRAINT fk_matches_organization ON matches IS
    'Denormalized org reference. ON DELETE CASCADE: safe because the '
    'cascade chain (org → tournament → match) handles deletion regardless.';


-- -----------------------------------------------------------------------------
-- 1C  match_events.organization_id  → ON DELETE CASCADE
-- -----------------------------------------------------------------------------
-- Why needed:
--   Same reasoning as 1B. match_events.organization_id is denormalized from
--   matches.organization_id. The cascade chain (org → tournament → match →
--   match_event) covers this, but the bare FK creates ambiguous evaluation
--   order. Explicit CASCADE removes the ambiguity entirely.
-- Production impact if left unfixed:
--   Same class of intermittent FK violation error as 1B, but on the largest
--   and most write-heavy table in the schema.

ALTER TABLE match_events
    DROP CONSTRAINT fk_match_events_organization;

ALTER TABLE match_events
    ADD CONSTRAINT fk_match_events_organization
        FOREIGN KEY (organization_id)
        REFERENCES organizations (id)
        ON DELETE CASCADE;

COMMENT ON CONSTRAINT fk_match_events_organization ON match_events IS
    'Denormalized org reference. ON DELETE CASCADE: cascade chain via '
    'tournament → match already handles deletion.';


-- -----------------------------------------------------------------------------
-- 1D  match_events.cancels_event_id  → explicit ON DELETE RESTRICT
-- -----------------------------------------------------------------------------
-- Why needed:
--   The cancels_event_id FK was declared without ON DELETE, defaulting to
--   NO ACTION. For an immutable table this distinction rarely matters, but
--   making it explicit documents the intent: a correction event that
--   references another event must never have its target silently removed.
--   RESTRICT is semantically identical to NO ACTION in most cases, but is
--   evaluated immediately (before deferred constraints) and communicates
--   the immutability intent clearly to anyone reading the schema.
-- Production impact if left unfixed:
--   No immediate runtime failure. The risk is a future developer adding a
--   bulk-delete path on match_events (e.g. for test data cleanup) and
--   silently orphaning score_correction events without a clear error.

ALTER TABLE match_events
    DROP CONSTRAINT fk_match_events_cancels;

ALTER TABLE match_events
    ADD CONSTRAINT fk_match_events_cancels
        FOREIGN KEY (cancels_event_id)
        REFERENCES match_events (id)
        ON DELETE RESTRICT;

COMMENT ON CONSTRAINT fk_match_events_cancels ON match_events IS
    'Self-referential FK for event corrections. ON DELETE RESTRICT prevents '
    'deleting a target event while a score_correction still references it. '
    'Reinforces the append-only immutability contract.';


-- =============================================================================
-- SECTION 2: UNIQUE CONSTRAINT NULL FIX
-- =============================================================================


-- -----------------------------------------------------------------------------
-- 2A  roles — platform-scoped slug uniqueness
-- -----------------------------------------------------------------------------
-- Why needed:
--   The existing UNIQUE (organization_id, slug) constraint cannot protect
--   against duplicate slugs for platform-scoped roles (organization_id IS NULL).
--   In PostgreSQL, NULL values are never considered equal inside a UNIQUE
--   constraint, so (NULL, 'admin') and (NULL, 'admin') are treated as two
--   distinct rows. This means two platform roles with slug = 'admin' can
--   co-exist, making RBAC slug lookups non-deterministic.
-- Production impact if left unfixed:
--   Duplicate platform role slugs allow two roles named 'admin' or 'superuser'
--   with different permission sets. An auth check for permission 'match.score'
--   could succeed or fail depending on which role row is returned first — a
--   silent, intermittent security bug.
--
-- A partial unique index on slug WHERE organization_id IS NULL plugs the gap
-- without touching the existing constraint (which correctly handles org-scoped
-- roles).

CREATE UNIQUE INDEX uq_roles_platform_slug
    ON roles (slug)
    WHERE organization_id IS NULL;

COMMENT ON INDEX uq_roles_platform_slug IS
    'Enforces slug uniqueness for platform-scoped roles (organization_id IS NULL). '
    'The standard UNIQUE(organization_id, slug) constraint cannot catch this because '
    'PostgreSQL treats two NULLs as non-equal in unique comparisons.';


-- =============================================================================
-- SECTION 3: MULTI-TENANCY LEAK FIXES
-- =============================================================================


-- -----------------------------------------------------------------------------
-- 3A  team_memberships — add organization_id column
-- -----------------------------------------------------------------------------
-- Why needed:
--   team_memberships had no organization_id, meaning a player from org A could
--   be added to a team from org B without any DB-level guard. Every multi-
--   tenant query for "memberships in org X" also required a double JOIN through
--   teams and players, because there was no direct tenant column.
-- Production impact if left unfixed:
--   An application bug (or a compromised session) can silently create cross-
--   tenant memberships. These rows are invisible to tenant-scoped queries,
--   corrupt roster displays, and are impossible to clean up without full-table
--   audits. Any query pattern that touches memberships without joining to teams
--   will return zero results for the affected org, causing silent data loss.
--
-- Backfill strategy:
--   1. Add column as nullable (no rewrite; O(1) in PostgreSQL 11+).
--   2. Backfill from teams.organization_id via team_id FK (always safe because
--      fk_team_memberships_team is ON DELETE CASCADE — no orphaned team_id).
--   3. Guard: raise if any NULLs remain after backfill.
--   4. Promote to NOT NULL and add FK + index.

-- Step 1: Add nullable column first (no rewrite, no lock escalation)
ALTER TABLE team_memberships
    ADD COLUMN organization_id UUID;

-- Step 2: Backfill from teams (safe: team_id FK guarantees every team exists)
UPDATE team_memberships tm
SET    organization_id = t.organization_id
FROM   teams t
WHERE  tm.team_id = t.id
  AND  tm.organization_id IS NULL;

-- Step 3: Guard — fail migration if any rows could not be backfilled
DO $$
DECLARE
    null_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO null_count
    FROM   team_memberships
    WHERE  organization_id IS NULL;

    IF null_count > 0 THEN
        RAISE EXCEPTION
            'Backfill incomplete: % team_memberships rows still have NULL '
            'organization_id. This should not happen if team FK integrity is '
            'intact. Investigate before re-running this migration.',
            null_count;
    END IF;
END;
$$;

-- Step 4a: Promote to NOT NULL
ALTER TABLE team_memberships
    ALTER COLUMN organization_id SET NOT NULL;

-- Step 4b: Add FK (validates all existing rows — safe because backfill is complete)
ALTER TABLE team_memberships
    ADD CONSTRAINT fk_team_memberships_organization
        FOREIGN KEY (organization_id)
        REFERENCES organizations (id)
        ON DELETE CASCADE;

-- Step 4c: Add the cross-tenant guard trigger (see below after all columns are set)
COMMENT ON COLUMN team_memberships.organization_id IS
    'Tenant scope: must equal both teams.organization_id and players.organization_id '
    'for this membership row. Enforced by trg_team_memberships_org_consistency.';


-- -----------------------------------------------------------------------------
-- 3B  Trigger: matches.organization_id must equal tournaments.organization_id
-- -----------------------------------------------------------------------------
-- Why needed:
--   matches.organization_id is intentionally denormalized for query performance,
--   but the schema comment ("must always equal the parent tournament organization")
--   was only an application-layer promise with no DB enforcement. A bug,
--   a direct DB insert, or a future service bypassing the ORM could silently
--   set the wrong org_id on a match row.
-- Production impact if left unfixed:
--   A match with mismatched organization_id is invisible to all org-scoped
--   queries on that tournament's org, and falsely visible (or inaccessible)
--   in queries scoped to the wrong org. This corrupts brackets, scoreboard
--   displays, and analytics for both organizations involved. Discovery
--   requires a full data audit, not an application error.

CREATE OR REPLACE FUNCTION fn_check_matches_org()
RETURNS TRIGGER
LANGUAGE plpgsql AS
$$
DECLARE
    expected_org_id UUID;
BEGIN
    -- On UPDATE, skip the check if neither tournament_id nor organization_id changed.
    IF TG_OP = 'UPDATE'
       AND NEW.organization_id IS NOT DISTINCT FROM OLD.organization_id
       AND NEW.tournament_id   IS NOT DISTINCT FROM OLD.tournament_id
    THEN
        RETURN NEW;
    END IF;

    SELECT organization_id
    INTO   expected_org_id
    FROM   tournaments
    WHERE  id = NEW.tournament_id;

    IF NEW.organization_id IS DISTINCT FROM expected_org_id THEN
        RAISE EXCEPTION
            'matches.organization_id (%) does not match '
            'tournaments.organization_id (%) for tournament_id %. '
            'The denormalized column must always equal the parent tournament org.',
            NEW.organization_id, expected_org_id, NEW.tournament_id;
    END IF;

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_check_matches_org() IS
    'Enforces that matches.organization_id == tournaments.organization_id '
    'for the parent tournament. Fires on INSERT and on UPDATE when either '
    'organization_id or tournament_id changes.';

CREATE TRIGGER trg_matches_org_consistency
    BEFORE INSERT OR UPDATE ON matches
    FOR EACH ROW
    EXECUTE FUNCTION fn_check_matches_org();

COMMENT ON TRIGGER trg_matches_org_consistency ON matches IS
    'Guards the denormalized organization_id against diverging from the '
    'parent tournament. See fn_check_matches_org().';


-- -----------------------------------------------------------------------------
-- 3C  Trigger: match_events.organization_id must equal matches.organization_id
-- -----------------------------------------------------------------------------
-- Why needed:
--   Same class of denormalization leak as 3B, one level deeper in the chain.
--   match_events.organization_id could be set to a different org than the
--   parent match (e.g. a scorer operating across two orgs accidentally
--   recording an event under the wrong tenant context).
-- Production impact if left unfixed:
--   Cross-tenant events are invisible to the correct org's analytics and
--   scoring feeds. Since match_events is the sole source of truth for all
--   statistics, a mismatched org_id silently corrupts every derived metric
--   for that match. It cannot be detected without comparing row counts
--   against the parent match.
--
-- Note: match_events is INSERT-only (no UPDATEs), so only BEFORE INSERT fires.

CREATE OR REPLACE FUNCTION fn_check_match_events_org()
RETURNS TRIGGER
LANGUAGE plpgsql AS
$$
DECLARE
    expected_org_id UUID;
BEGIN
    SELECT organization_id
    INTO   expected_org_id
    FROM   matches
    WHERE  id = NEW.match_id;

    IF NEW.organization_id IS DISTINCT FROM expected_org_id THEN
        RAISE EXCEPTION
            'match_events.organization_id (%) does not match '
            'matches.organization_id (%) for match_id %. '
            'Every event must be recorded under the match''s organization.',
            NEW.organization_id, expected_org_id, NEW.match_id;
    END IF;

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_check_match_events_org() IS
    'Enforces that match_events.organization_id == matches.organization_id '
    'for the parent match. Fires on INSERT only (table is immutable).';

CREATE TRIGGER trg_match_events_org_consistency
    BEFORE INSERT ON match_events
    FOR EACH ROW
    EXECUTE FUNCTION fn_check_match_events_org();

COMMENT ON TRIGGER trg_match_events_org_consistency ON match_events IS
    'Guards the denormalized organization_id against diverging from the '
    'parent match. See fn_check_match_events_org().';


-- -----------------------------------------------------------------------------
-- 3D  Trigger: tournament_registrations participant must belong to org
-- -----------------------------------------------------------------------------
-- Why needed:
--   tournament_registrations.organization_id is the registrant's org, but the
--   schema had no constraint validating that the team_id (or player_id) being
--   registered actually belongs to that org. A bug could register team_id from
--   org B under the org_id of org A.
-- Production impact if left unfixed:
--   A team or player appears in a tournament under the wrong org's banner.
--   The registrant org's roster shows an unknown team; the team's actual org
--   has no record of the registration. Tournament brackets display incorrect
--   org affiliations, which is both a data integrity failure and a visible
--   user-facing error (wrong team name, wrong logo, wrong stats).

CREATE OR REPLACE FUNCTION fn_check_treg_participant_org()
RETURNS TRIGGER
LANGUAGE plpgsql AS
$$
BEGIN
    -- Validate team_id belongs to the registrant organization
    IF NEW.team_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM teams
            WHERE  id              = NEW.team_id
              AND  organization_id = NEW.organization_id
        ) THEN
            RAISE EXCEPTION
                'tournament_registrations.team_id (%) does not belong to '
                'organization_id (%). The registering team must be owned by '
                'the registrant organization.',
                NEW.team_id, NEW.organization_id;
        END IF;
    END IF;

    -- Validate player_id belongs to the registrant organization
    IF NEW.player_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM players
            WHERE  id              = NEW.player_id
              AND  organization_id = NEW.organization_id
        ) THEN
            RAISE EXCEPTION
                'tournament_registrations.player_id (%) does not belong to '
                'organization_id (%). The registering player must be owned by '
                'the registrant organization.',
                NEW.player_id, NEW.organization_id;
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_check_treg_participant_org() IS
    'Validates that team_id / player_id belongs to the registrant organization_id. '
    'Prevents cross-tenant registrations where a team or player is registered '
    'under a different org than their actual owner.';

CREATE TRIGGER trg_treg_participant_org_consistency
    BEFORE INSERT OR UPDATE ON tournament_registrations
    FOR EACH ROW
    EXECUTE FUNCTION fn_check_treg_participant_org();

COMMENT ON TRIGGER trg_treg_participant_org_consistency ON tournament_registrations IS
    'Guards cross-tenant registrations. See fn_check_treg_participant_org().';


-- -----------------------------------------------------------------------------
-- 3E  Trigger: team_memberships player + team must share organization_id
-- -----------------------------------------------------------------------------
-- Why needed:
--   Complements the new organization_id column (3A). A player from org A
--   being added to a team from org B would satisfy the individual FKs but
--   violate tenant isolation. The trigger cross-validates all three at insert.
-- Production impact if left unfixed:
--   A cross-tenant membership row would cause player stats queries (which scope
--   by the membership's organization_id) to return zero results for the player's
--   actual org, while also polluting the team's active roster with a player
--   the org doesn't own.

CREATE OR REPLACE FUNCTION fn_check_team_membership_org()
RETURNS TRIGGER
LANGUAGE plpgsql AS
$$
BEGIN
    -- Validate team belongs to the specified organization
    IF NOT EXISTS (
        SELECT 1 FROM teams
        WHERE  id              = NEW.team_id
          AND  organization_id = NEW.organization_id
    ) THEN
        RAISE EXCEPTION
            'team_memberships.team_id (%) does not belong to organization_id (%). '
            'Team and membership must share the same organization.',
            NEW.team_id, NEW.organization_id;
    END IF;

    -- Validate player belongs to the same organization
    IF NOT EXISTS (
        SELECT 1 FROM players
        WHERE  id              = NEW.player_id
          AND  organization_id = NEW.organization_id
    ) THEN
        RAISE EXCEPTION
            'team_memberships.player_id (%) does not belong to organization_id (%). '
            'Player and membership must share the same organization.',
            NEW.player_id, NEW.organization_id;
    END IF;

    RETURN NEW;
END;
$$;

COMMENT ON FUNCTION fn_check_team_membership_org() IS
    'Validates that both team_id and player_id belong to the membership''s '
    'organization_id. Prevents cross-tenant roster entries.';

CREATE TRIGGER trg_team_memberships_org_consistency
    BEFORE INSERT OR UPDATE ON team_memberships
    FOR EACH ROW
    EXECUTE FUNCTION fn_check_team_membership_org();

COMMENT ON TRIGGER trg_team_memberships_org_consistency ON team_memberships IS
    'Guards against cross-tenant team-player assignments. '
    'See fn_check_team_membership_org().';


-- =============================================================================
-- SECTION 4: MISSING INDEXES
-- =============================================================================
-- All indexes below use standard (non-CONCURRENTLY) mode because this
-- migration runs inside a transaction. For tables exceeding ~1M rows,
-- extract the relevant CREATE INDEX statement, run it manually as
-- CREATE INDEX CONCURRENTLY outside any transaction, then re-run
-- the migration (the standard CREATE INDEX will be a no-op due to the
-- CONCURRENTLY index already existing with the same name).


-- -----------------------------------------------------------------------------
-- 4A  user_organization_roles (role_id)
-- -----------------------------------------------------------------------------
-- Why needed:
--   The auth system must answer "who in this org holds role X?" for admin
--   panels and "does this user hold the scorer role?" for permission checks.
--   Without this index, both queries scan the entire user_organization_roles
--   table, making permission checks O(total_grants) instead of O(1).
-- Production impact if left unfixed:
--   Every API request that checks user permissions (every authenticated
--   endpoint) hits a full table scan. At 1000 users × 3 roles each = 3000
--   rows this is tolerable. At 100k users it becomes the top query in slow
--   log within weeks of launch.

CREATE INDEX idx_uor_role_id
    ON user_organization_roles (role_id);

-- -----------------------------------------------------------------------------
-- 4B  user_organization_roles (expires_at) WHERE NOT NULL
-- -----------------------------------------------------------------------------
-- Why needed:
--   A background cleanup job must periodically revoke expired grants by
--   querying WHERE expires_at <= NOW() AND expires_at IS NOT NULL.
--   Without this partial index the job scans the full table on every run,
--   most of which is rows with expires_at IS NULL (permanent grants).
-- Production impact if left unfixed:
--   Cleanup job degrades linearly with org growth. Expired time-limited grants
--   (e.g. guest scorers for one tournament) stay active past their expiry,
--   silently retaining access for users who should have been revoked.

CREATE INDEX idx_uor_expires_at
    ON user_organization_roles (expires_at)
    WHERE expires_at IS NOT NULL;

-- -----------------------------------------------------------------------------
-- 4C  matches (tournament_id, status) — composite
-- -----------------------------------------------------------------------------
-- Why needed:
--   The most common bracket query is "all live or completed matches in
--   tournament X." Separate indexes on tournament_id and status force the
--   planner to either pick one and filter the other, or do a bitmap AND —
--   neither is as efficient as a covering composite on the exact query shape.
-- Production impact if left unfixed:
--   The live scoreboard page fires this query on every spectator refresh.
--   At 100k spectators polling every 5 seconds, this composite is the
--   difference between an index scan returning 10 rows and a 50k-row
--   tournament_id scan filtered by status.

CREATE INDEX idx_matches_tournament_status
    ON matches (tournament_id, status);

-- -----------------------------------------------------------------------------
-- 4D  matches (winner_team_id) — partial
-- -----------------------------------------------------------------------------
-- Why needed:
--   Team profile pages and ranking computations query "all matches won by
--   team X." without this index these are full table scans.
-- Production impact if left unfixed:
--   Rankings and team profile loads are O(total_matches). With 10 concurrent
--   tournaments at 100 matches each, acceptable. At 1000 tournaments,
--   rankings background jobs time out.

CREATE INDEX idx_matches_winner_team_id
    ON matches (winner_team_id)
    WHERE winner_team_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- 4E  matches (winner_player_id) — partial
-- -----------------------------------------------------------------------------
-- Same reasoning as 4D, for individual-sport tournaments.

CREATE INDEX idx_matches_winner_player_id
    ON matches (winner_player_id)
    WHERE winner_player_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- 4F  tournament_registrations (registered_by) — partial
-- -----------------------------------------------------------------------------
-- Why needed:
--   User self-service ("show me all tournaments I have registered for") and
--   admin audit ("what has this user submitted?") both need this lookup.
-- Production impact if left unfixed:
--   User profile registration history pages do full scans on
--   tournament_registrations. Slow at scale; unacceptable for a paginated
--   self-service endpoint.

CREATE INDEX idx_treg_registered_by
    ON tournament_registrations (registered_by)
    WHERE registered_by IS NOT NULL;

-- -----------------------------------------------------------------------------
-- 4G  tournament_registrations (approved_by) — partial
-- -----------------------------------------------------------------------------
-- Why needed:
--   Audit trail for approval decisions: "all registrations approved by admin X."
--   Used in audit_log cross-referencing and admin accountability reports.
-- Production impact if left unfixed:
--   Compliance or dispute queries that need to trace approval decisions
--   do full scans on tournament_registrations.

CREATE INDEX idx_treg_approved_by
    ON tournament_registrations (approved_by)
    WHERE approved_by IS NOT NULL;

-- -----------------------------------------------------------------------------
-- 4H  tournaments (created_by) — partial
-- -----------------------------------------------------------------------------
-- Why needed:
--   Audit trail and user-facing "tournaments I created" view.
-- Production impact if left unfixed:
--   Full scan on tournaments table for every "my tournaments" dashboard query.

CREATE INDEX idx_tournaments_created_by
    ON tournaments (created_by)
    WHERE created_by IS NOT NULL;

-- -----------------------------------------------------------------------------
-- 4I  team_memberships (organization_id)
-- -----------------------------------------------------------------------------
-- Depends on Section 3A (column added earlier in this migration).
-- Why needed:
--   The primary tenant-scoped query on this table is "all memberships in
--   org X." Without this index that query scans the full table and joins to
--   teams or players to determine org. Now that organization_id is a direct
--   column, this index makes it O(memberships in org).

CREATE INDEX idx_team_memberships_organization_id
    ON team_memberships (organization_id);

-- -----------------------------------------------------------------------------
-- 4J  DROP duplicate index on match_events
-- -----------------------------------------------------------------------------
-- Why needed:
--   Migration 000011 created UNIQUE (match_id, sequence_number), which
--   PostgreSQL implements as a unique B-tree index on (match_id, sequence_number).
--   The same migration also created idx_match_events_match_seq on the exact
--   same columns without the UNIQUE flag. This second index is never preferred
--   by the planner (the unique index is always at least as selective), so it
--   consumes disk space, adds write overhead on every INSERT into the hottest
--   table in the schema, and bloats the WAL stream.
-- Production impact if left unfixed:
--   match_events is the highest-write table (every live match event). Every
--   INSERT maintains two identical index entries. At 1M events/day this wastes
--   ~30–50MB/day of index I/O for zero query benefit.

DROP INDEX IF EXISTS idx_match_events_match_seq;
