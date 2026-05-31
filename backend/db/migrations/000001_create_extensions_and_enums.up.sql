-- =============================================================================
-- Migration  : 000001_create_extensions_and_enums (UP)
-- Description: Enables required PostgreSQL extensions and declares every
--              platform-wide ENUM type used across the PlayArena schema.
--              No tables are created here.
-- PostgreSQL : 17
-- =============================================================================


-- -----------------------------------------------------------------------------
-- EXTENSIONS
-- -----------------------------------------------------------------------------

CREATE EXTENSION IF NOT EXISTS pgcrypto;

COMMENT ON EXTENSION pgcrypto IS
    'Provides gen_random_uuid() for UUID primary key generation and '
    'cryptographic helpers (pgp_sym_encrypt, digest, etc.) for future use.';


-- =============================================================================
-- IDENTITY & AUTHENTICATION
-- =============================================================================

-- Lifecycle state of a platform user account.
-- pending_verification is the initial state on self-registration.
-- suspended blocks login but preserves all data for audit purposes.
CREATE TYPE user_status AS ENUM (
    'active',
    'inactive',
    'suspended',
    'pending_verification'
);

COMMENT ON TYPE user_status IS
    'Lifecycle states for a user account. '
    'pending_verification is set on registration until the email is confirmed. '
    'suspended is set administratively and blocks all authenticated access.';


-- Self-reported gender stored on user and player profiles.
-- Never required — the column that references this type must be nullable.
CREATE TYPE gender AS ENUM (
    'male',
    'female',
    'other',
    'prefer_not_to_say'
);

COMMENT ON TYPE gender IS
    'Self-reported gender for user and player profiles. '
    'This field is always optional; the referencing column must be NULLable.';


-- =============================================================================
-- ORGANIZATIONS
-- =============================================================================

-- Operational status of an organization.
-- suspended is propagated by application logic to block all child operations
-- (registrations, matches, scoring) without hard-deleting any data.
CREATE TYPE org_status AS ENUM (
    'active',
    'inactive',
    'suspended'
);

COMMENT ON TYPE org_status IS
    'Operational status of an organization. '
    'A suspended organization cannot host tournaments or register for new events. '
    'All historical data is preserved intact.';


-- Classifies an organization for reporting, feature-gating, and UI grouping.
-- Does not affect access control — role_scope handles that.
CREATE TYPE org_type AS ENUM (
    'club',
    'federation',
    'school',
    'corporate',
    'independent'
);

COMMENT ON TYPE org_type IS
    'Categorical classification of an organization. '
    'Used for reporting and optional feature differentiation (e.g., federation-only tooling). '
    'Does not affect RBAC — use role_scope for access boundaries.';


-- =============================================================================
-- RBAC (Roles & Permissions)
-- =============================================================================

-- Defines the boundary within which a role grants permissions.
-- platform roles have organization_id = NULL and are reserved for super-admins.
-- organization roles are scoped to a single org row.
-- tournament roles are reserved for future fine-grained access per tournament.
CREATE TYPE role_scope AS ENUM (
    'platform',
    'organization',
    'tournament'
);

COMMENT ON TYPE role_scope IS
    'Boundary within which a role is valid. '
    'platform roles have a NULL organization_id and grant cross-tenant access. '
    'organization roles are always paired with a non-NULL organization_id. '
    'tournament scope is reserved for future per-tournament permission grants.';


-- =============================================================================
-- PLAYERS
-- =============================================================================

-- Athletic lifecycle status for a player profile within an organization.
-- injured and suspended are distinct so that the platform can filter
-- availability correctly when building tournament rosters.
CREATE TYPE player_status AS ENUM (
    'active',
    'inactive',
    'injured',
    'suspended',
    'retired'
);

COMMENT ON TYPE player_status IS
    'Athletic lifecycle state of a player profile. '
    'injured and suspended are intentionally separate from inactive so '
    'roster eligibility checks can distinguish voluntary vs forced unavailability. '
    'retired records are kept permanently for historical statistics.';


-- =============================================================================
-- TEAMS
-- =============================================================================

-- Operational status of a team entity.
-- disbanded preserves the record for historical match and ranking data.
CREATE TYPE team_status AS ENUM (
    'active',
    'inactive',
    'disbanded'
);

COMMENT ON TYPE team_status IS
    'Operational status of a team. '
    'disbanded teams are retained because match history and rankings reference them. '
    'Hard deletion of a disbanded team would corrupt historical records.';


-- Functional role of a person within a team roster.
-- captain and vice_captain are on-field designations that may change per tournament.
-- Non-playing roles (coach, manager, support_staff) are included so the full
-- team management structure is represented in team_memberships.
CREATE TYPE membership_role AS ENUM (
    'player',
    'captain',
    'vice_captain',
    'coach',
    'manager',
    'support_staff'
);

COMMENT ON TYPE membership_role IS
    'Functional role of a person within a team. '
    'captain and vice_captain are on-field designations that can vary by tournament. '
    'coach, manager, and support_staff are non-playing roles tracked for full roster management.';


-- Status of a player-to-team membership record.
-- transferred and released are kept distinct so transfer analytics and
-- release history can be queried separately; both set left_at.
CREATE TYPE membership_status AS ENUM (
    'active',
    'inactive',
    'transferred',
    'released'
);

COMMENT ON TYPE membership_status IS
    'Status of a player-to-team membership. '
    'transferred means the player moved to another team (left_at is set). '
    'released means the team ended the contract (left_at is set). '
    'The distinction matters for transfer window reporting.';


-- =============================================================================
-- TOURNAMENTS
-- =============================================================================

-- Full lifecycle of a tournament from creation to conclusion.
-- Status transitions are intentionally one-way in the application layer:
--   draft → registration_open → registration_closed → ongoing → completed
--   Any state → cancelled
CREATE TYPE tournament_status AS ENUM (
    'draft',
    'registration_open',
    'registration_closed',
    'ongoing',
    'completed',
    'cancelled'
);

COMMENT ON TYPE tournament_status IS
    'Lifecycle state of a tournament. Transitions are one-way in application logic. '
    'draft: being configured, not yet public. '
    'registration_open: accepting entries. '
    'registration_closed: entries locked, bracket being finalised. '
    'ongoing: matches are being played. '
    'completed: all matches done, results final. '
    'cancelled: abandoned; registrations are voided by application logic.';


-- Structural format of the tournament.
-- Format-specific configuration (group sizes, points-per-win, tiebreaker rules)
-- is stored in tournaments.settings JSONB, not encoded in the enum.
CREATE TYPE tournament_format AS ENUM (
    'league',
    'knockout',
    'group_knockout',
    'round_robin',
    'double_elimination'
);

COMMENT ON TYPE tournament_format IS
    'Structural format determining how brackets and standings are computed. '
    'league: full points table (home/away or single-leg). '
    'knockout: single-elimination bracket. '
    'group_knockout: group stage followed by knockout rounds. '
    'round_robin: every participant plays every other once. '
    'double_elimination: two losses required for elimination. '
    'Format-specific config lives in tournaments.settings JSONB.';


-- Whether the tournament participants are teams or individual players.
-- Determines which columns are populated in tournament_registrations and matches.
CREATE TYPE participant_type AS ENUM (
    'team',
    'individual'
);

COMMENT ON TYPE participant_type IS
    'Determines whether tournament participants are teams or individual players. '
    'team: team_id is set in registrations; home_team_id/away_team_id set in matches. '
    'individual: player_id is set in registrations; home_player_id/away_player_id set in matches.';


-- Lifecycle state of a single tournament registration entry.
-- Only approved registrations count toward participant limits.
-- disqualified is post-approval and may trigger match result recalculations.
CREATE TYPE registration_status AS ENUM (
    'pending',
    'approved',
    'rejected',
    'withdrawn',
    'disqualified'
);

COMMENT ON TYPE registration_status IS
    'Lifecycle state of a tournament registration. '
    'pending: submitted, awaiting organiser review. '
    'approved: confirmed participant; counts against max_participants. '
    'rejected: denied by the organiser. '
    'withdrawn: voluntarily pulled out by the registrant before the tournament starts. '
    'disqualified: removed after the tournament began; results may be voided.';


-- =============================================================================
-- MATCHES
-- =============================================================================

-- Operational state of a single match fixture.
-- walkover sets the winner without any match_events being required.
-- abandoned means the match started but has no official result.
CREATE TYPE match_status AS ENUM (
    'scheduled',
    'live',
    'completed',
    'cancelled',
    'postponed',
    'abandoned',
    'walkover'
);

COMMENT ON TYPE match_status IS
    'Operational state of a match fixture. '
    'scheduled → live → completed is the normal flow. '
    'walkover sets winner_team_id/winner_player_id without match_events. '
    'abandoned: match started but ended without an official result. '
    'postponed: rescheduled; a new scheduled_at will be set.';


-- Exhaustive set of recordable in-match events for the event-sourced match log.
-- Every scoring action and player-state change MUST be expressed as an event.
-- No direct score columns exist on the matches table.
-- score_correction events invalidate prior events via the cancels_event_id FK.
CREATE TYPE match_event_type AS ENUM (

    -- Match lifecycle
    'match_started',        -- Referee whistle; match clock begins
    'match_ended',          -- Official conclusion of the match
    'half_started',         -- A half or period begins
    'half_ended',           -- A half or period ends
    'timeout_called',       -- A team calls an official timeout
    'timeout_ended',        -- Timeout over; play resumes

    -- Raid events (kabaddi-primary; payload carries outcome details)
    'raid_attempt',         -- Raider crosses the baulk line
    'raid_successful',      -- Raider returns safely with ≥1 defender tagged
    'raid_empty',           -- Raider returns without scoring (no tag, no bonus)
    'bonus_point_awarded',  -- Raider crosses the bonus line; +1 point

    -- Tackle / defence events
    'tackle_successful',    -- Defending team stops the raider
    'super_tackle',         -- Tackle with ≤3 active defenders; +2 points

    -- Compound / special scoring events
    'super_raid',           -- Single raid results in ≥3 points
    'do_or_die_raid',       -- Third consecutive empty raid (raider out on failure)
    'all_out',              -- All players of a team eliminated; +2 bonus to opponent

    -- Player state events
    'player_out',           -- Player eliminated from the court
    'player_revived',       -- Player returns to court after opponent all-out revival
    'player_substituted',   -- Official substitution (payload: out_player_id, in_player_id)
    'player_injured',       -- Injury noted; may be followed by player_substituted

    -- Administrative / correction events
    'penalty_awarded',      -- Technical penalty point(s) granted by referee
    'score_correction'      -- Official correction; references cancels_event_id on the target event
);

COMMENT ON TYPE match_event_type IS
    'Every possible recordable event during a match. '
    'Designed for kabaddi but structurally generic — sport-specific detail lives in the payload JSONB. '
    'This is the ONLY source of truth for scoring and player state. '
    'Never derive statistics from anywhere other than this event log. '
    'score_correction pairs with the is_cancelled flag and cancels_event_id FK on match_events '
    'to allow corrections without mutating or deleting prior events.';


-- =============================================================================
-- MEDIA
-- =============================================================================

-- Discriminator for the polymorphic (entity_type, entity_id) reference in media_attachments.
-- PostgreSQL cannot enforce a FK to multiple tables; integrity is enforced in the service layer.
CREATE TYPE media_entity_type AS ENUM (
    'organization',
    'user',
    'player',
    'team',
    'tournament',
    'match'
);

COMMENT ON TYPE media_entity_type IS
    'Discriminator in the polymorphic media_attachments reference. '
    'Paired with entity_id (UUID) to point at a row in the corresponding table. '
    'No database-level FK is possible for polymorphic references; '
    'referential integrity is enforced by the application service layer.';


-- Broad category of an uploaded file.
-- Pair with the mime_type TEXT column for precise format identification.
-- thumbnail is a derivative of video; kept separate so queries can
-- find thumbnails without scanning video rows.
CREATE TYPE media_type AS ENUM (
    'image',
    'video',
    'document',
    'thumbnail'
);

COMMENT ON TYPE media_type IS
    'Broad category of an uploaded media file. '
    'Pair with the mime_type column for precise format (image/webp, video/mp4, etc.). '
    'thumbnail is treated as distinct from image because it is auto-generated '
    'from a video and has different lifecycle and storage characteristics.';


-- =============================================================================
-- AUDIT
-- =============================================================================

-- Type of action recorded in the immutable audit_logs table.
-- login / logout are user-session events with no entity_id.
-- All others are entity-scoped and carry old_data / new_data JSONB snapshots.
CREATE TYPE audit_action AS ENUM (
    'create',
    'update',
    'delete',
    'login',
    'logout',
    'permission_change'
);

COMMENT ON TYPE audit_action IS
    'Type of action recorded in audit_logs. '
    'create/update/delete: entity-scoped; old_data and new_data carry JSONB snapshots. '
    'login/logout: user-session events; entity_id will be NULL. '
    'permission_change: role or permission granted or revoked; '
    'old_data and new_data carry the before/after role assignment state.';
