package standings

// makeLess returns the comparator function passed to sort.SliceStable.
//
// Tiebreaker chain (highest priority first):
//
//  1. Points DESC — primary ranking criterion.
//  2. Head-to-head result — applied ONLY when exactly two participants share
//     the same point total (strict 2-way tie).  N-way head-to-head sub-table
//     resolution is deferred to a future phase.
//  3. Score difference DESC (ScoreFor - ScoreAgainst across all matches).
//  4. Score for DESC (total points scored).
//  5. Wins DESC.
//  6. Seed number ASC — lower seed number = higher standing; unseeded
//     participants (nil) rank below any seeded participant.
//  7. Registration timestamp ASC — earlier registration = higher standing.
//     Because registered_at is unique per registration (separate transactions),
//     this criterion guarantees a fully deterministic sort with no ties.
//
// The comparator is both consistent (a<b implies b>a) and transitive.
// Head-to-head is restricted to strict 2-way ties to avoid the classic
// non-transitivity problem that arises with 3+ participants in a cycle.
func makeLess(rows []StandingsRow, matches []CompletedMatch, s Settings) func(i, j int) bool {
	// Pre-compute how many participants share each point total.
	// Used to detect strict 2-way ties for head-to-head application.
	pointsFreq := make(map[int]int, len(rows))
	for _, r := range rows {
		pointsFreq[r.Points]++
	}

	return func(i, j int) bool {
		ri, rj := rows[i], rows[j]

		// 1. Points DESC
		if ri.Points != rj.Points {
			return ri.Points > rj.Points
		}

		// 2. Head-to-head (strict 2-way ties only)
		if pointsFreq[ri.Points] == 2 {
			if cmp := h2hCompare(ri.ParticipantID, rj.ParticipantID, matches, s); cmp != 0 {
				return cmp > 0
			}
		}

		// 3. Score difference DESC
		if ri.ScoreDifference != rj.ScoreDifference {
			return ri.ScoreDifference > rj.ScoreDifference
		}

		// 4. Score for DESC
		if ri.ScoreFor != rj.ScoreFor {
			return ri.ScoreFor > rj.ScoreFor
		}

		// 5. Wins DESC
		if ri.Wins != rj.Wins {
			return ri.Wins > rj.Wins
		}

		// 6. Seed number ASC (nil = unseeded, ranks last)
		if cmp := seedCompare(ri.SeedNumber, rj.SeedNumber); cmp != 0 {
			return cmp < 0
		}

		// 7. Registration timestamp ASC (always unique — guarantees termination)
		return ri.RegisteredAt.Before(rj.RegisteredAt)
	}
}

// h2hCompare returns the head-to-head comparison between participants a and b,
// considering only matches played between them.  Uses the tournament's own
// point system so the sub-ranking is consistent with the main table.
//
// Return values:
//
//	+1  a ranks higher in head-to-head
//	-1  b ranks higher in head-to-head
//	 0  inconclusive (no h2h matches played, or still tied after h2h criteria)
func h2hCompare(aID, bID string, matches []CompletedMatch, s Settings) int {
	var aPoints, bPoints int
	var aScoreDiff, bScoreDiff int
	var aScoreFor, bScoreFor int

	for _, m := range matches {
		var homeIsA bool
		switch {
		case m.HomeParticipantID == aID && m.AwayParticipantID == bID:
			homeIsA = true
		case m.HomeParticipantID == bID && m.AwayParticipantID == aID:
			homeIsA = false
		default:
			continue
		}

		if homeIsA {
			aScoreFor += m.HomeScore
			bScoreFor += m.AwayScore
			aScoreDiff += m.HomeScore - m.AwayScore
			bScoreDiff += m.AwayScore - m.HomeScore
			switch m.WinnerID {
			case aID:
				aPoints += s.WinPoints
				margin := m.HomeScore - m.AwayScore
				if margin < 0 {
					margin = -margin
				}
				bPoints += closeLossPoints(s, margin, m.IsWalkover)
			case bID:
				bPoints += s.WinPoints
				margin := m.HomeScore - m.AwayScore
				if margin < 0 {
					margin = -margin
				}
				aPoints += closeLossPoints(s, margin, m.IsWalkover)
			default:
				aPoints += s.DrawPoints
				bPoints += s.DrawPoints
			}
		} else {
			bScoreFor += m.HomeScore
			aScoreFor += m.AwayScore
			bScoreDiff += m.HomeScore - m.AwayScore
			aScoreDiff += m.AwayScore - m.HomeScore
			switch m.WinnerID {
			case bID:
				bPoints += s.WinPoints
				margin := m.HomeScore - m.AwayScore
				if margin < 0 {
					margin = -margin
				}
				aPoints += closeLossPoints(s, margin, m.IsWalkover)
			case aID:
				aPoints += s.WinPoints
				margin := m.HomeScore - m.AwayScore
				if margin < 0 {
					margin = -margin
				}
				bPoints += closeLossPoints(s, margin, m.IsWalkover)
			default:
				aPoints += s.DrawPoints
				bPoints += s.DrawPoints
			}
		}
	}

	if aPoints != bPoints {
		return sign(aPoints - bPoints)
	}
	if aScoreDiff != bScoreDiff {
		return sign(aScoreDiff - bScoreDiff)
	}
	if aScoreFor != bScoreFor {
		return sign(aScoreFor - bScoreFor)
	}
	return 0
}

// seedCompare compares two optional seed numbers.
// Returns negative when a ranks higher, positive when b ranks higher, 0 when equal.
// A seeded participant always ranks above an unseeded one.
func seedCompare(a, b *int16) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	return sign(int(*a) - int(*b))
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	if n > 0 {
		return 1
	}
	return 0
}
