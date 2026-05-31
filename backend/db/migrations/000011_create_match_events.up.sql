-- =============================================================================
-- Migration  : 000011_create_match_events (UP)
-- Description: Creates the match_events table — an append-only, immutable
--              event log and the single source of truth for all in-match data.
--
--              IMMUTABILITY CONTRACT:
--              No UPDATE or DELETE is ever performed on this table.
--              Corrections are expressed by inserting a new score_correction
--              event with cancels_event_id pointing to the erroneous event.
--              The erroneous event is never touched.
--
--              STATS DERIVATION:
--              All scores, player stats, and team stats must be computed from
--              this table. No score or statistic column exists on matches.
--              To exclude cancelled events in a query, use:
--
--                WHERE id NOT IN (
--                    SELECT cancels_event_id
--                    FROM   match_events
--                    WHERE  match_id = $1
--                      AND  cancels_event_id IS NOT NULL
--                )
--
--              SEQUENCE ORDERING:
--              sequence_number is monotonically increasing per match (starts at 1).
--              The application must acquire a row-level lock on the parent match
--              (SELECT … FOR UPDATE) before computing MAX(sequence_number) + 1
--              to prevent duplicate sequence numbers under concurrent inserts.
--
-- Depends on : 000002 (organizations), 000003 (users), 000005 (players),
--              000006 (teams), 000010 (matches),
--              000001 (match_event_type ENUM)
-- =============================================================================

CREATE TABLE match_events (
    id               UUID             NOT NULL DEFAULT gen_random_uuid(),
    match_id         UUID             NOT NULL,
    organization_id  UUID             NOT NULL,
    sequence_number  BIGINT           NOT NULL,
    event_type       match_event_type NOT NULL,
    team_id          UUID,
    player_id        UUID,
    period           SMALLINT,
    clock_seconds    INTEGER,
    payload          JSONB            NOT NULL DEFAULT '{}',
    recorded_by      UUID,
    recorded_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    cancels_event_id UUID,
    created_at       TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    -- No updated_at column: this table is append-only. No UPDATE is ever performed.

    CONSTRAINT pk_match_events                             PRIMARY KEY (id),
    CONSTRAINT uq_match_events_sequence                    UNIQUE (match_id, sequence_number),
    CONSTRAINT fk_match_events_match                       FOREIGN KEY (match_id)
                                                           REFERENCES matches       (id) ON DELETE CASCADE,
    CONSTRAINT fk_match_events_organization                FOREIGN KEY (organization_id)
                                                           REFERENCES organizations (id),
    CONSTRAINT fk_match_events_team                        FOREIGN KEY (team_id)
                                                           REFERENCES teams         (id) ON DELETE SET NULL,
    CONSTRAINT fk_match_events_player                      FOREIGN KEY (player_id)
                                                           REFERENCES players       (id) ON DELETE SET NULL,
    CONSTRAINT fk_match_events_recorded_by                 FOREIGN KEY (recorded_by)
                                                           REFERENCES users         (id) ON DELETE SET NULL,
    CONSTRAINT fk_match_events_cancels                     FOREIGN KEY (cancels_event_id)
                                                           REFERENCES match_events  (id),
    CONSTRAINT chk_match_events_sequence_positive          CHECK (sequence_number > 0),
    CONSTRAINT chk_match_events_period_positive            CHECK (period IS NULL OR period > 0),
    CONSTRAINT chk_match_events_clock_non_negative         CHECK (clock_seconds IS NULL OR clock_seconds >= 0),
    -- score_correction events MUST reference the event they invalidate
    CONSTRAINT chk_match_events_correction_requires_target CHECK (
        event_type != 'score_correction' OR cancels_event_id IS NOT NULL
    ),
    -- An event cannot cancel itself
    CONSTRAINT chk_match_events_no_self_cancel             CHECK (
        cancels_event_id IS NULL OR cancels_event_id != id
    )
);

COMMENT ON TABLE match_events IS
    'Append-only immutable event log. The single source of truth for all in-match data. '
    'Every point, player state change, and lifecycle transition is a row here. '
    'No UPDATE or DELETE is ever performed. '
    'score_correction events invalidate prior events via cancels_event_id without mutating them. '
    'All statistics (team scores, player raid counts, tackle rates) must be derived '
    'from this table — never stored redundantly in other tables.';

COMMENT ON COLUMN match_events.sequence_number IS
    'Monotonically increasing integer per match, starting at 1. '
    'Uniquely orders events within a match (guaranteed by uq_match_events_sequence). '
    'Application must hold a row-level lock on the parent match row during insert '
    'to safely compute MAX(sequence_number) + 1 under concurrent scorers.';

COMMENT ON COLUMN match_events.payload IS
    'Event-type-specific structured data. Validated at the application layer. '
    'Examples by event type: '
    'raid_successful  → {"points": 2, "defenders_tagged": ["<uuid>", "<uuid>"]} '
    'tackle_successful → {"tacklers": ["<uuid>"], "raider_out": true} '
    'super_tackle     → {"tacklers": ["<uuid>"], "points": 2} '
    'all_out          → {"team_id": "<uuid>", "bonus_points": 2} '
    'player_substituted → {"out_player_id": "<uuid>", "in_player_id": "<uuid>"} '
    'score_correction → {"reason": "wrong team credited", "original_payload": {...}}';

COMMENT ON COLUMN match_events.cancels_event_id IS
    'Points to the event this record invalidates. Set only on score_correction events. '
    'The referenced event is never mutated — immutability is fully preserved. '
    'To get the effective event log: exclude events whose id appears in any '
    'cancels_event_id within the same match.';

COMMENT ON COLUMN match_events.recorded_by IS
    'The scorer or referee who entered this event. '
    'SET NULL on user deletion: the event record is retained regardless.';

COMMENT ON COLUMN match_events.clock_seconds IS
    'Elapsed match clock in seconds at the moment of the event. '
    'E.g. 245 = 4 minutes 5 seconds into the half. '
    'NULL for lifecycle events (match_started, half_ended, etc.) where a clock value '
    'is not meaningful.';

-- ---------------------------------------------------------------------------
-- Indexes
-- The uq_match_events_sequence index already covers (match_id, sequence_number)
-- efficiently. Additional indexes are kept narrow and targeted.
-- ---------------------------------------------------------------------------

-- Stats aggregation: events for a match in order (primary access pattern)
CREATE INDEX idx_match_events_match_seq        ON match_events (match_id, sequence_number);

-- Team stats: "all scoring events by team X in match Y"
CREATE INDEX idx_match_events_team_id          ON match_events (match_id, team_id)
    WHERE team_id IS NOT NULL;

-- Player stats: "all events involving player X in match Y"
CREATE INDEX idx_match_events_player_id        ON match_events (match_id, player_id)
    WHERE player_id IS NOT NULL;

-- Event type filter: "all raid_successful events in match Y"
CREATE INDEX idx_match_events_event_type       ON match_events (match_id, event_type);

-- Cancellation lookup: "was event X cancelled?"
CREATE INDEX idx_match_events_cancels_event_id ON match_events (cancels_event_id)
    WHERE cancels_event_id IS NOT NULL;

-- Cross-match org queries (admin / analytics)
CREATE INDEX idx_match_events_organization_id  ON match_events (organization_id);
