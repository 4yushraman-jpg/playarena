-- =============================================================================
-- Migration  : 000009_create_tournament_registrations (UP)
-- Description: Creates the tournament_registrations table.
--              Exactly one of team_id or player_id must be set per row,
--              enforced by chk_treg_one_participant.
--              Partial unique indexes prevent duplicate registrations while
--              correctly handling NULLs (standard UNIQUE cannot do this).
--              organization_id here is the REGISTRANT's org, not the
--              tournament host org.
-- Depends on : 000002 (organizations), 000003 (users), 000005 (players),
--              000006 (teams), 000008 (tournaments),
--              000001 (registration_status ENUM)
-- =============================================================================

CREATE TABLE tournament_registrations (
    id              UUID                NOT NULL DEFAULT gen_random_uuid(),
    tournament_id   UUID                NOT NULL,
    organization_id UUID                NOT NULL,
    team_id         UUID,
    player_id       UUID,
    seed_number     SMALLINT,
    status          registration_status NOT NULL DEFAULT 'pending',
    registered_by   UUID,
    registered_at   TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    approved_by     UUID,
    approved_at     TIMESTAMPTZ,
    notes           TEXT,
    metadata        JSONB               NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ         NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ         NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_tournament_registrations        PRIMARY KEY (id),
    CONSTRAINT fk_treg_tournament                 FOREIGN KEY (tournament_id)
                                                  REFERENCES tournaments   (id) ON DELETE CASCADE,
    CONSTRAINT fk_treg_organization               FOREIGN KEY (organization_id)
                                                  REFERENCES organizations (id),
    CONSTRAINT fk_treg_team                       FOREIGN KEY (team_id)
                                                  REFERENCES teams         (id) ON DELETE CASCADE,
    CONSTRAINT fk_treg_player                     FOREIGN KEY (player_id)
                                                  REFERENCES players       (id) ON DELETE CASCADE,
    CONSTRAINT fk_treg_registered_by              FOREIGN KEY (registered_by)
                                                  REFERENCES users         (id) ON DELETE SET NULL,
    CONSTRAINT fk_treg_approved_by                FOREIGN KEY (approved_by)
                                                  REFERENCES users         (id) ON DELETE SET NULL,
    CONSTRAINT chk_treg_one_participant           CHECK (
        (team_id IS NOT NULL AND player_id IS NULL)
        OR (team_id IS NULL AND player_id IS NOT NULL)
    ),
    CONSTRAINT chk_treg_seed_positive             CHECK (seed_number IS NULL OR seed_number > 0),
    CONSTRAINT chk_treg_approval_consistency      CHECK (
        (approved_at IS NULL AND approved_by IS NULL)
        OR (approved_at IS NOT NULL AND approved_by IS NOT NULL)
    ),
    CONSTRAINT chk_treg_approved_after_registered CHECK (
        approved_at IS NULL OR approved_at >= registered_at
    )
);

COMMENT ON TABLE tournament_registrations IS
    'Records a team or individual player entering a tournament. '
    'Exactly one of team_id / player_id must be non-NULL (chk_treg_one_participant). '
    'organization_id is the registrant''s org — not necessarily the tournament host org. '
    'This distinction matters for cross-org tournaments (e.g. a league with multiple clubs).';

COMMENT ON COLUMN tournament_registrations.organization_id IS
    'The organization that owns the registering team or player. '
    'Distinct from the tournament''s organization_id. '
    'Enables queries like "all registrations submitted by org X".';

COMMENT ON COLUMN tournament_registrations.seed_number IS
    'Seeding position assigned by the tournament organiser after registration closes. '
    'Determines bracket placement for knockout formats. '
    'NULL until explicitly set; positive integer only.';

COMMENT ON COLUMN tournament_registrations.metadata IS
    'Extra registration data: squad list submission, payment reference, documents, etc. '
    'Structure is tournament-specific and validated at the application layer.';

-- ---------------------------------------------------------------------------
-- Partial unique indexes
-- Standard UNIQUE constraints cannot express "unique when NOT NULL".
-- These prevent a team/player from registering for the same tournament twice.
-- ---------------------------------------------------------------------------

CREATE UNIQUE INDEX uq_treg_tournament_team
    ON tournament_registrations (tournament_id, team_id)
    WHERE team_id IS NOT NULL;

CREATE UNIQUE INDEX uq_treg_tournament_player
    ON tournament_registrations (tournament_id, player_id)
    WHERE player_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Indexes
-- ---------------------------------------------------------------------------

-- Tournament management: "all registrations for tournament X"
CREATE INDEX idx_treg_tournament_id   ON tournament_registrations (tournament_id);

-- Org management: "all tournaments org X has registered for"
CREATE INDEX idx_treg_organization_id ON tournament_registrations (organization_id);

-- Team profile: "all tournaments this team has entered"
CREATE INDEX idx_treg_team_id         ON tournament_registrations (team_id)   WHERE team_id   IS NOT NULL;

-- Player profile: "all tournaments this player has entered"
CREATE INDEX idx_treg_player_id       ON tournament_registrations (player_id) WHERE player_id IS NOT NULL;

-- Admin workflow: filter by approval state
CREATE INDEX idx_treg_status          ON tournament_registrations (status);
