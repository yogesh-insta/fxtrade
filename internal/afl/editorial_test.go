package afl

import (
	"strings"
	"testing"
)

func TestLoadRoundEditorialESSSTK(t *testing.T) {
	ed, err := LoadRoundEditorial("../../data/afl/round_picks.json")
	if err != nil {
		t.Fatal(err)
	}
	if ed.Round != 17 {
		t.Fatalf("round = %d, want 17", ed.Round)
	}
	fix := ed.LookupFixtureEditorial("ESS", "STK")
	if fix == nil {
		t.Fatal("expected ESS vs STK editorial")
	}
	if fix.MatchLabel != "Match 1" {
		t.Fatalf("match_label = %q", fix.MatchLabel)
	}
	if len(fix.SmartPicks) != 2 {
		t.Fatalf("smart_picks = %d, want 2", len(fix.SmartPicks))
	}
	if fix.SmartPicks[0].FormatSmartPickLine() != "St Kilda Head-to-Head ($1.20)" {
		t.Fatalf("h2h pick = %q", fix.SmartPicks[0].FormatSmartPickLine())
	}
	if len(fix.PlayerProps) != 1 || fix.PlayerProps[0].Market != "25+ Disposals" {
		t.Fatalf("player_props = %+v", fix.PlayerProps)
	}
}

func TestAttachEditorialPicks(t *testing.T) {
	ed := RoundEditorial{
		Fixtures: []FixtureEditorial{{
			MatchLabel: "Match 1",
			Home:       "ESS",
			Away:       "STK",
			SmartPicks: []EditorialPick{{Label: "St Kilda Head-to-Head", Odds: floatPtr(1.20)}},
		}},
	}
	reports := []MatchReport{{
		Context: MatchDayContext{HomeTeam: "ESS", AwayTeam: "STK"},
	}}
	AttachEditorialPicks(reports, ed)
	if reports[0].Editorial == nil || reports[0].Editorial.MatchLabel != "Match 1" {
		t.Fatalf("editorial not attached: %+v", reports[0].Editorial)
	}
}

func TestFormatRoundReportEmailESSSTKSmartPick(t *testing.T) {
	odds := 1.20
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS",
			AwayTeam: "STK",
			Venue:    VenueProfile{Name: "Marvel Stadium", Dimension: VenueClosed},
			HomeStats: TeamStats{
				FormWinsLast10: 0, FormLossesLast10: 10,
				PointsForPerGame: 73, RecentPointsForPerGame: 59, ScoringTrendFor: -14,
			},
			AwayStats: TeamStats{
				FormWinsLast10: 4, FormLossesLast10: 6,
				PointsForPerGame: 89, PointsAgainstPerGame: 88,
			},
			Players: PlayerAvailabilityMatrix{
				Away: []PlayerImpact{{PlayerID: "stk_sinclair", DisplayName: "Jack Sinclair", Available: false}},
			},
		},
		HomeWinProb: 0.38,
		Score: ScoreProjection{
			HomeScore: 74, AwayScore: 82, TotalScore: 156, Margin: 8,
			PredictedWinner: "STK",
		},
		MarketOdds: map[TeamID]MarketOdds{
			"STK": {DecimalOdds: 1.20},
		},
		Editorial: &FixtureEditorial{
			MatchLabel: "Match 1",
			Home:       "ESS",
			Away:       "STK",
			SmartPicks: []EditorialPick{
				{Label: "St Kilda Head-to-Head", Odds: &odds},
				{Label: "St Kilda 1-39 Margin"},
			},
			PlayerProps: []PlayerPropPick{{
				Player: "Nasiah Wanganeen-Milera",
				Team:   "STK",
				Market: "25+ Disposals",
				Why:    "Coming off a 44-disposal game.",
			}},
			Notes: []string{"Essendon are on a 10-game losing streak."},
		},
	}
	body := FormatRoundReportEmail([]MatchReport{report}, nil)
	for _, want := range []string{
		"Match 1: Essendon vs St Kilda",
		"SMART PICK",
		"St Kilda Head-to-Head ($1.20)",
		"St Kilda 1-39 Margin",
		"PLAYER PROP (Same Game Multi)",
		"Nasiah Wanganeen-Milera 25+ Disposals",
		"44-disposal game",
		"losing streak",
		"margin market may suit",
		"Key outs: Jack Sinclair",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
}

func floatPtr(v float64) *float64 { return &v }
