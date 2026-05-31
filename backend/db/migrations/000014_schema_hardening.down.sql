-- =============================================================================
-- Migration  : 000014_schema_hardening (DOWN)
-- Description: Fully reverses all changes made in 000014_schema_hardening.up.
--              Executed in the exact reverse order of the UP migration.
--
-- WARNING: Rolling back Section 3A (team_memberships.organization_id) is a
-- destructive data operation — the column and all its data are dropped.
-- Before running this DOWN migration on a production database, verify that
-- no application code or reports depend on team_memberships.organization_id.
-- =============================================================================


-- =============================================================================
-- REVERSE SECTION 4: DROP ALL NEW INDEXES
-- (Drop indexes before touching the columns they reference)
-- =============================================================================

-- Restore the duplicate index that was removed in 4J
-- (Restores the pre-hardening state even though the index was redundant)
CREATE INDEX idx_match_events_match_seq
    ON match_events (match_id, sequence_number);

DROP INDEX IF EXISTS idx_team_memberships_organization_id;
DROP INDEX IF EXISTS idx_tournaments_created_by;
DROP INDEX IF EXISTS idx_treg_approved_by;
DROP INDEX IF EXISTS idx_treg_registered_by;
DROP INDEX IF EXISTS idx_matches_winner_player_id;
DROP INDEX IF EXISTS idx_matches_winner_team_id;
DROP INDEX IF EXISTS idx_matches_tournament_status;
DROP INDEX IF EXISTS idx_uor_expires_at;
DROP INDEX IF EXISTS idx_uor_role_id;


-- =============================================================================
-- REVERSE SECTION 3: REMOVE MULTI-TENANCY LEAK FIXES
-- =============================================================================

-- 3E (team_memberships cross-org trigger)
DROP TRIGGER  IF EXISTS trg_team_memberships_org_consistency ON team_memberships;
DROP FUNCTION IF EXISTS fn_check_team_membership_org();

-- 3D (tournament_registrations participant-org trigger)
DROP TRIGGER  IF EXISTS trg_treg_participant_org_consistency ON tournament_registrations;
DROP FUNCTION IF EXISTS fn_check_treg_participant_org();

-- 3C (match_events org consistency trigger)
DROP TRIGGER  IF EXISTS trg_match_events_org_consistency ON match_events;
DROP FUNCTION IF EXISTS fn_check_match_events_org();

-- 3B (matches org consistency trigger)
DROP TRIGGER  IF EXISTS trg_matches_org_consistency ON matches;
DROP FUNCTION IF EXISTS fn_check_matches_org();

-- 3A (team_memberships.organization_id column)
-- Drops the FK constraint, index (cascade), and the column itself.
-- DATA LOSS: all organization_id values on team_memberships are permanently lost.
ALTER TABLE team_memberships
    DROP CONSTRAINT IF EXISTS fk_team_memberships_organization;

ALTER TABLE team_memberships
    DROP COLUMN IF EXISTS organization_id;


-- =============================================================================
-- REVERSE SECTION 2: REMOVE UNIQUE PARTIAL INDEX
-- =============================================================================

DROP INDEX IF EXISTS uq_roles_platform_slug;


-- =============================================================================
-- REVERSE SECTION 1: RESTORE ORIGINAL FK CONSTRAINTS
-- (Drop the updated constraints and re-add without ON DELETE clause)
-- Note: PostgreSQL default ON DELETE behaviour is NO ACTION, which matches
-- the original intent of the pre-hardening schema.
-- =============================================================================

-- 1D  match_events.cancels_event_id  (restore: no ON DELETE clause)
ALTER TABLE match_events
    DROP CONSTRAINT IF EXISTS fk_match_events_cancels;

ALTER TABLE match_events
    ADD CONSTRAINT fk_match_events_cancels
        FOREIGN KEY (cancels_event_id)
        REFERENCES match_events (id);


-- 1C  match_events.organization_id  (restore: no ON DELETE clause)
ALTER TABLE match_events
    DROP CONSTRAINT IF EXISTS fk_match_events_organization;

ALTER TABLE match_events
    ADD CONSTRAINT fk_match_events_organization
        FOREIGN KEY (organization_id)
        REFERENCES organizations (id);


-- 1B  matches.organization_id  (restore: no ON DELETE clause)
ALTER TABLE matches
    DROP CONSTRAINT IF EXISTS fk_matches_organization;

ALTER TABLE matches
    ADD CONSTRAINT fk_matches_organization
        FOREIGN KEY (organization_id)
        REFERENCES organizations (id);


-- 1A  tournament_registrations.organization_id  (restore: no ON DELETE clause)
ALTER TABLE tournament_registrations
    DROP CONSTRAINT IF EXISTS fk_treg_organization;

ALTER TABLE tournament_registrations
    ADD CONSTRAINT fk_treg_organization
        FOREIGN KEY (organization_id)
        REFERENCES organizations (id);
