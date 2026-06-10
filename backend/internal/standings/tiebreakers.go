package standings

import "sort"

// makeLess returns the comparator function passed to sort.SliceStable.
//
// Tiebreaker chain (highest priority first):
//
//  1. Points DESC — primary ranking criterion.
//  2. Head-to-head sub-table — applied to ALL N participants that share the
//     same points total (N ≥ 2).  The sub-table considers only matches played
//     among the tied group and is sorted by h2h points → h2h score difference →
//     h2h score for.  If two participants are still tied after all h2h criteria
//     (e.g., a cyclic A>B>C>A result), they share the same h2h rank and this
//     criterion is inconclusive for that pair.
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
func makeLess(rows []StandingsRow, matches []CompletedMatch, s Settings) func(i, j int) bool {
	h2hRanks := buildH2HRanks(rows, matches, s)

	return func(i, j int) bool {
		ri, rj := rows[i], rows[j]

		// 1. Points DESC
		if ri.Points != rj.Points {
			return ri.Points > rj.Points
		}

		// 2. Head-to-head rank (N-way sub-table among all tied participants)
		aRank, bRank := h2hRanks[ri.ParticipantID], h2hRanks[rj.ParticipantID]
		if aRank != bRank {
			return aRank < bRank
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

// buildH2HRanks pre-computes an ordinal h2h rank for every participant within
// its tied-points group. Ranks are 1-indexed (rank 1 = best in sub-table).
//
// Algorithm per group:
//  1. Filter matches to those played exclusively among group members.
//  2. Accumulate h2h points, score difference, and score for each member.
//  3. Sort members by h2h points DESC → h2h score diff DESC → h2h score for DESC.
//  4. Assign ranks; members with identical stats share the same rank.
//
// A shared rank means h2h is inconclusive for that pair — the main comparator
// falls through to the next global tiebreaker (score difference, etc.).
// Cyclic results (A>B>C>A) naturally produce a three-way shared rank.
func buildH2HRanks(rows []StandingsRow, matches []CompletedMatch, s Settings) map[string]int {
	groups := make(map[int][]string)
	for _, r := range rows {
		groups[r.Points] = append(groups[r.Points], r.ParticipantID)
	}

	out := make(map[string]int, len(rows))

	for _, group := range groups {
		if len(group) == 1 {
			out[group[0]] = 1
			continue
		}

		inGroup := make(map[string]bool, len(group))
		for _, pid := range group {
			inGroup[pid] = true
		}

		type h2hStats struct {
			id        string
			points    int
			scoreDiff int
			scoreFor  int
		}

		statsMap := make(map[string]*h2hStats, len(group))
		for _, pid := range group {
			statsMap[pid] = &h2hStats{id: pid}
		}

		for _, m := range matches {
			if !inGroup[m.HomeParticipantID] || !inGroup[m.AwayParticipantID] {
				continue
			}
			home := statsMap[m.HomeParticipantID]
			away := statsMap[m.AwayParticipantID]

			home.scoreFor += m.HomeScore
			away.scoreFor += m.AwayScore
			home.scoreDiff += m.HomeScore - m.AwayScore
			away.scoreDiff += m.AwayScore - m.HomeScore

			margin := m.HomeScore - m.AwayScore
			if margin < 0 {
				margin = -margin
			}

			switch m.WinnerID {
			case m.HomeParticipantID:
				home.points += s.WinPoints
				away.points += closeLossPoints(s, margin, m.IsWalkover)
			case m.AwayParticipantID:
				away.points += s.WinPoints
				home.points += closeLossPoints(s, margin, m.IsWalkover)
			default:
				home.points += s.DrawPoints
				away.points += s.DrawPoints
			}
		}

		sorted := make([]*h2hStats, 0, len(group))
		for _, pid := range group {
			sorted = append(sorted, statsMap[pid])
		}
		sort.SliceStable(sorted, func(i, j int) bool {
			a, b := sorted[i], sorted[j]
			if a.points != b.points {
				return a.points > b.points
			}
			if a.scoreDiff != b.scoreDiff {
				return a.scoreDiff > b.scoreDiff
			}
			return a.scoreFor > b.scoreFor
		})

		// Assign ordinal ranks; tied entries share the same rank.
		rank := 1
		for i, st := range sorted {
			if i > 0 {
				prev := sorted[i-1]
				if st.points != prev.points || st.scoreDiff != prev.scoreDiff || st.scoreFor != prev.scoreFor {
					rank = i + 1
				}
			}
			out[st.id] = rank
		}
	}

	return out
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
