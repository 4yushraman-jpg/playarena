package standings

import "sort"

// Compute derives an ordered standings table from completed matches and
// registration metadata.  It is a pure function: no DB access, no I/O,
// no randomness.
//
// All approved registrants appear in the result even when they have played
// zero matches (Played = 0, all other stats = 0).  Participants present in
// matches but absent from registrations are silently ignored — this indicates
// a data anomaly that should be addressed upstream, not by the engine.
//
// The returned slice is ordered from highest to lowest position (position 1
// is best).  Positions are sequential integers starting at 1; no positions
// are shared because the tiebreaker chain is fully deterministic.
func Compute(
	matches []CompletedMatch,
	registrations []RegistrationInfo,
	settings Settings,
) []StandingsRow {
	// Build per-participant accumulators keyed by participant UUID.
	accum := make(map[string]*StandingsRow, len(registrations))
	for _, reg := range registrations {
		reg := reg // capture loop variable
		accum[reg.ParticipantID] = &StandingsRow{
			ParticipantID: reg.ParticipantID,
			SeedNumber:    reg.SeedNumber,
			RegisteredAt:  reg.RegisteredAt,
		}
	}

	for _, m := range matches {
		home := accum[m.HomeParticipantID]
		away := accum[m.AwayParticipantID]
		if home == nil || away == nil {
			continue
		}

		home.Played++
		away.Played++

		// Walkovers have 0-0 scores by convention; these contribute 0 to
		// score_for / score_against for both sides.
		home.ScoreFor += m.HomeScore
		home.ScoreAgainst += m.AwayScore
		away.ScoreFor += m.AwayScore
		away.ScoreAgainst += m.HomeScore

		margin := m.HomeScore - m.AwayScore
		if margin < 0 {
			margin = -margin
		}

		switch {
		case m.WinnerID == "":
			home.Draws++
			away.Draws++
			home.Points += settings.DrawPoints
			away.Points += settings.DrawPoints

		case m.WinnerID == m.HomeParticipantID:
			home.Wins++
			away.Losses++
			home.Points += settings.WinPoints
			away.Points += closeLossPoints(settings, margin, m.IsWalkover)

		default:
			away.Wins++
			home.Losses++
			away.Points += settings.WinPoints
			home.Points += closeLossPoints(settings, margin, m.IsWalkover)
		}
	}

	rows := make([]StandingsRow, 0, len(accum))
	for _, row := range accum {
		row.ScoreDifference = row.ScoreFor - row.ScoreAgainst
		rows = append(rows, *row)
	}

	sort.SliceStable(rows, makeLess(rows, matches, settings))

	for i := range rows {
		rows[i].Position = i + 1
	}

	return rows
}

// closeLossPoints returns the points awarded to the losing side.
// Walkovers always receive LossPoints regardless of CloseMargin because a
// walkover is a forfeit, not a competitive result.
func closeLossPoints(s Settings, margin int, isWalkover bool) int {
	if !isWalkover && s.CloseMargin > 0 && margin <= s.CloseMargin {
		return s.CloseLossPoints
	}
	return s.LossPoints
}
