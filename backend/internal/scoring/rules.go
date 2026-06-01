package scoring

import (
	"encoding/json"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// side identifies which match participant an event is attributed to.
type side int

const (
	sideNone side = iota
	sideHome
	sideAway
)

// Kabaddi scoring rules (authoritative):
//
//   raid_successful    → payload.points       attributed to event.team_id (raiding team)
//   bonus_point_awarded → +1                  attributed to event.team_id (raiding team)
//   tackle_successful  → +1                   attributed to event.team_id (defending team)
//   super_tackle       → +2                   attributed to event.team_id (defending team)
//   all_out            → payload.bonus_points  awarded to OPPONENT of payload.team_id
//   penalty_awarded    → payload.points        attributed to event.team_id
//   super_raid         → 0 (analytics label only; raid_successful already carries the points)
//   all other types    → 0

// participantSide resolves which side (home/away) an event is attributed to.
//
// Team tournaments:   keyed on event.TeamID vs match HomeTeamID / AwayTeamID.
// Individual tournaments: keyed on event.PlayerID vs match HomePlayerID / AwayPlayerID.
func participantSide(evt *db.MatchEvent, m *db.Match) side {
	if m.HomeTeamID.Valid {
		id := pgutil.UUIDToString(evt.TeamID)
		if id == "" {
			return sideNone
		}
		if id == pgutil.UUIDToString(m.HomeTeamID) {
			return sideHome
		}
		if id == pgutil.UUIDToString(m.AwayTeamID) {
			return sideAway
		}
		return sideNone
	}
	// Individual tournament — attribute by player_id.
	id := pgutil.UUIDToString(evt.PlayerID)
	if id == "" {
		return sideNone
	}
	if id == pgutil.UUIDToString(m.HomePlayerID) {
		return sideHome
	}
	if id == pgutil.UUIDToString(m.AwayPlayerID) {
		return sideAway
	}
	return sideNone
}

// sideByID resolves which side a raw participant UUID string belongs to.
// Checks both team IDs and player IDs so it works for both tournament formats.
// Used by all_out to identify the eliminated side from payload.team_id.
func sideByID(id string, m *db.Match) side {
	if id == "" {
		return sideNone
	}
	if m.HomeTeamID.Valid && id == pgutil.UUIDToString(m.HomeTeamID) {
		return sideHome
	}
	if m.AwayTeamID.Valid && id == pgutil.UUIDToString(m.AwayTeamID) {
		return sideAway
	}
	if m.HomePlayerID.Valid && id == pgutil.UUIDToString(m.HomePlayerID) {
		return sideHome
	}
	if m.AwayPlayerID.Valid && id == pgutil.UUIDToString(m.AwayPlayerID) {
		return sideAway
	}
	return sideNone
}

// payloadPoints extracts payload.points as a positive integer.
// Returns 0 for missing, zero, or negative values — the engine is fault-tolerant
// for pre-Phase-9 events; write-time validation (ValidateScoreEventPayload)
// prevents new malformed events from entering the log.
func payloadPoints(payload []byte) int {
	var obj struct {
		Points *int `json:"points"`
	}
	if len(payload) == 0 {
		return 0
	}
	if err := json.Unmarshal(payload, &obj); err != nil || obj.Points == nil || *obj.Points <= 0 {
		return 0
	}
	return *obj.Points
}

// allOutScore derives the bonus-point recipient for an all_out event.
// payload.team_id is the ELIMINATED team; the opponent receives the bonus.
// Returns (0, sideNone) on any payload anomaly.
func allOutScore(payload []byte, m *db.Match) (int, side) {
	var obj struct {
		TeamID      *string `json:"team_id"`
		BonusPoints *int    `json:"bonus_points"`
	}
	if len(payload) == 0 {
		return 0, sideNone
	}
	if err := json.Unmarshal(payload, &obj); err != nil || obj.TeamID == nil || *obj.TeamID == "" {
		return 0, sideNone
	}
	if obj.BonusPoints == nil || *obj.BonusPoints <= 0 {
		return 0, sideNone
	}
	// The eliminated side loses; the opponent receives the bonus.
	switch sideByID(*obj.TeamID, m) {
	case sideHome:
		return *obj.BonusPoints, sideAway
	case sideAway:
		return *obj.BonusPoints, sideHome
	default:
		return 0, sideNone
	}
}
