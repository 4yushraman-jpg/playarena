-- =============================================================================
-- Migration  : 000029_bracket_progression (UP)
-- Description: FE-8B — bracket linkage + winner propagation.
--              Adds the bracket edge (next_match_id + next_match_slot) so a
--              match's winner can be propagated into a designated slot of a
--              downstream match, plus group_label for the group_knockout future
--              path (no group-resolution logic ships in FE-8B).
--
--              Also RELAXES chk_matches_participants to permit a PARTIALLY filled
--              match (exactly one side assigned). This is required because the
--              two feeders of a knockout match complete at different times: the
--              first winner must be written into one slot while the other slot is
--              still TBD. The relaxed constraint still forbids mixing team and
--              player identities in the same match. The application layer (the
--              I1 guard, ErrMatchHasTBDSlot) prevents a partially filled match
--              from ever going live / completed / walkover, so partial state is
--              only ever a transient bracket-progression intermediate.
-- Depends on : 000010 (matches)
-- =============================================================================

-- ── Relax the participant constraint to allow partial (one-sided) fills ───────
ALTER TABLE matches DROP CONSTRAINT chk_matches_participants;

ALTER TABLE matches ADD CONSTRAINT chk_matches_participants CHECK (
    -- A match is either team-typed (no player slots) or player-typed (no team
    -- slots). Within a type, zero, one, or both slots may be filled. The
    -- one-sided case is a transient bracket slot awaiting its second feeder.
    (home_player_id IS NULL AND away_player_id IS NULL)
    OR (home_team_id IS NULL AND away_team_id IS NULL)
);

COMMENT ON CONSTRAINT chk_matches_participants ON matches IS
    'Forbids mixing team and player identities in one match. Permits 0, 1, or 2 '
    'filled slots of a single type — the one-sided case is a transient knockout '
    'slot awaiting its second feeder. The service layer (ErrMatchHasTBDSlot) '
    'prevents a partially filled match from starting or concluding.';

-- ── Bracket edge + group label ───────────────────────────────────────────────
ALTER TABLE matches
    ADD COLUMN next_match_id   UUID,
    ADD COLUMN next_match_slot SMALLINT,
    ADD COLUMN group_label     TEXT;

ALTER TABLE matches
    ADD CONSTRAINT fk_matches_next_match FOREIGN KEY (next_match_id)
        REFERENCES matches (id) ON DELETE SET NULL,
    -- next_match_slot: 1 = the successor's home slot, 2 = its away slot.
    ADD CONSTRAINT chk_matches_next_slot CHECK (
        next_match_slot IS NULL OR next_match_slot IN (1, 2)
    ),
    -- A match may not feed itself (cheap cycle guard; full forward-only structure
    -- is a generation invariant in FE-8C).
    ADD CONSTRAINT chk_matches_no_self_next CHECK (
        next_match_id IS NULL OR next_match_id <> id
    );

COMMENT ON COLUMN matches.next_match_id IS
    'Bracket edge: the match this fixture''s winner advances to. NULL for finals, '
    'round-robin, league, and group-stage matches. ON DELETE SET NULL — a hard '
    'tournament delete cascades; matches are otherwise soft-cancelled, never deleted.';

COMMENT ON COLUMN matches.next_match_slot IS
    'Which slot of next_match_id this winner fills: 1 = home, 2 = away. The slot '
    'is fixed per feeder so propagation writes exactly one deterministic slot — '
    'this is what makes winner propagation idempotent and double-write-safe.';

COMMENT ON COLUMN matches.group_label IS
    'Group identifier (e.g. "A", "B") for group_knockout group-stage matches. '
    'Reserved for the FE-8C group_knockout path; no resolution logic in FE-8B.';

-- Reverse-lookup of a match''s feeders is rare; the forward edge is read per-row.
-- A partial index keeps the linkage queryable without bloating the common path.
CREATE INDEX idx_matches_next_match_id ON matches (next_match_id) WHERE next_match_id IS NOT NULL;
