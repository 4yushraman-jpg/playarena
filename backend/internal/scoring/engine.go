// Package scoring implements the PlayArena live scoring engine.
//
// The engine is stateless and has no database access or side effects.
// All score derivation is pure computation over the effective event log
// supplied by the caller.  match_events is the authoritative source of truth;
// this package only reads, never writes.
//
// Kabaddi scoring rules applied by Compute:
//
//	raid_successful    payload.points      → raiding team
//	bonus_point_awarded +1                → raiding team
//	tackle_successful   +1                → defending team
//	super_tackle        +2                → defending team
//	all_out             payload.bonus_pts → opponent of payload.team_id
//	penalty_awarded    payload.points      → attributed team
//	super_raid          0 (analytics only — raid_successful carries the points)
//	all other types     0
package scoring

import (
	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// ScoreEngine derives match scores from an effective event log.
// It is stateless: construct once and reuse freely across goroutines.
type ScoreEngine struct{}

// NewScoreEngine returns a ScoreEngine ready for use.
func NewScoreEngine() *ScoreEngine { return &ScoreEngine{} }

// Compute calculates home and away scores from the provided events.
//
// The caller must pass only effective (non-cancelled) events.  Filtering is
// the repository's responsibility (GetEffectiveMatchEventsForScore query);
// the engine does not re-apply the cancellation filter.
//
// score_correction events present in the slice contribute zero points —
// they exist only to carry the cancels_event_id reference and are already
// excluded from the effective log on the target side.
//
// No data is persisted.  Call this function as often as needed.
func (e *ScoreEngine) Compute(match *db.Match, events []db.MatchEvent) ScoreResult {
	res := ScoreResult{
		MatchID:     pgutil.UUIDToString(match.ID),
		MatchStatus: string(match.Status),
		IsWalkover:  match.IsWalkover,
	}

	if match.HomeTeamID.Valid {
		res.HomeTeamID = pgutil.UUIDToString(match.HomeTeamID)
		res.AwayTeamID = pgutil.UUIDToString(match.AwayTeamID)
	} else {
		res.HomePlayerID = pgutil.UUIDToString(match.HomePlayerID)
		res.AwayPlayerID = pgutil.UUIDToString(match.AwayPlayerID)
	}

	for i := range events {
		pts, s := e.pointsForEvent(&events[i], match)
		switch s {
		case sideHome:
			res.HomeScore += pts
		case sideAway:
			res.AwayScore += pts
		}
	}
	return res
}

// pointsForEvent returns the point value and recipient side for one effective
// event.  Returns (0, sideNone) for non-scoring event types.
func (e *ScoreEngine) pointsForEvent(evt *db.MatchEvent, m *db.Match) (int, side) {
	switch evt.EventType {

	case db.MatchEventTypeRaidSuccessful:
		return payloadPoints(evt.Payload), participantSide(evt, m)

	case db.MatchEventTypeBonusPointAwarded:
		return 1, participantSide(evt, m)

	case db.MatchEventTypeTackleSuccessful:
		return 1, participantSide(evt, m)

	case db.MatchEventTypeSuperTackle:
		return 2, participantSide(evt, m)

	case db.MatchEventTypeAllOut:
		return allOutScore(evt.Payload, m)

	case db.MatchEventTypePenaltyAwarded:
		return payloadPoints(evt.Payload), participantSide(evt, m)

	// super_raid is an analytics/classification label only.
	// The raid's actual point contribution is already recorded via
	// the accompanying raid_successful event.  Scoring super_raid
	// separately would double-count those points.
	case db.MatchEventTypeSuperRaid:
		return 0, sideNone

	default:
		return 0, sideNone
	}
}
