package standings

import (
	"testing"
	"time"
)

// indexByID maps standings rows by participant id for assertion convenience.
func indexByID(rows []StandingsRow) map[string]StandingsRow {
	m := make(map[string]StandingsRow, len(rows))
	for _, r := range rows {
		m[r.ParticipantID] = r
	}
	return m
}

func twoParticipants() []RegistrationInfo {
	return []RegistrationInfo{
		{ParticipantID: "home", RegisteredAt: time.Unix(1, 0)},
		{ParticipantID: "away", RegisteredAt: time.Unix(2, 0)},
	}
}

// TestCompute_Walkover_AwardsWinLossPoints proves a walkover is counted as a
// win/loss in the table — the core of the FE-8A standings-visibility fix. A
// forfeit awards full WinPoints to the present side and LossPoints to the
// absent side, with a 0-0 score contribution.
func TestCompute_Walkover_AwardsWinLossPoints(t *testing.T) {
	matches := []CompletedMatch{{
		HomeParticipantID: "home",
		AwayParticipantID: "away",
		HomeScore:         0,
		AwayScore:         0,
		WinnerID:          "home",
		IsWalkover:        true,
	}}

	rows := indexByID(Compute(matches, twoParticipants(), DefaultSettings()))

	if h := rows["home"]; h.Wins != 1 || h.Losses != 0 || h.Played != 1 || h.Points != 3 {
		t.Errorf("home walkover winner: wins=%d losses=%d played=%d points=%d, want 1/0/1/3",
			h.Wins, h.Losses, h.Played, h.Points)
	}
	if a := rows["away"]; a.Losses != 1 || a.Wins != 0 || a.Played != 1 || a.Points != 0 {
		t.Errorf("away walkover loser: wins=%d losses=%d played=%d points=%d, want 0/1/1/0",
			a.Wins, a.Losses, a.Played, a.Points)
	}
	// A walkover must not register as a draw.
	if rows["home"].Draws != 0 || rows["away"].Draws != 0 {
		t.Errorf("walkover wrongly counted as draw: home=%d away=%d",
			rows["home"].Draws, rows["away"].Draws)
	}
}

// TestCompute_Walkover_ExemptFromCloseLossBonus proves the 0-0 forfeit score of
// a walkover does NOT earn the close-loss bonus, even when CloseMargin is set so
// that a 0-margin loss would otherwise qualify. A forfeit is not a competitive
// result. (Adversarial finding A10.)
func TestCompute_Walkover_ExemptFromCloseLossBonus(t *testing.T) {
	s := DefaultSettings()
	s.CloseMargin = 5     // losses within 5 points earn the consolation bonus...
	s.CloseLossPoints = 1 // ...of 1 point

	matches := []CompletedMatch{{
		HomeParticipantID: "home",
		AwayParticipantID: "away",
		WinnerID:          "home",
		IsWalkover:        true, // 0-0; margin 0 ≤ CloseMargin, but it's a forfeit
	}}

	rows := indexByID(Compute(matches, twoParticipants(), s))

	if a := rows["away"]; a.Points != s.LossPoints {
		t.Errorf("walkover loser points=%d, want LossPoints=%d (no close-loss bonus for forfeits)",
			a.Points, s.LossPoints)
	}
}

// TestCompute_CloseLoss_AppliesBonusForScoredMatch is the contrast case: a real
// (non-walkover) close loss DOES earn the bonus. This guards against a fix that
// accidentally disables the close-loss rule for everyone.
func TestCompute_CloseLoss_AppliesBonusForScoredMatch(t *testing.T) {
	s := DefaultSettings()
	s.CloseMargin = 5
	s.CloseLossPoints = 1

	matches := []CompletedMatch{{
		HomeParticipantID: "home",
		AwayParticipantID: "away",
		HomeScore:         30,
		AwayScore:         27, // margin 3 ≤ 5
		WinnerID:          "home",
		IsWalkover:        false,
	}}

	rows := indexByID(Compute(matches, twoParticipants(), s))

	if a := rows["away"]; a.Points != s.CloseLossPoints {
		t.Errorf("scored close loss points=%d, want CloseLossPoints=%d", a.Points, s.CloseLossPoints)
	}
}
