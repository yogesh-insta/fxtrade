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
	if fix.SmartPicks[0].Why == "" {
		t.Fatal("expected why on h2h pick")
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

func TestBuildHighConfidenceBetsModelOnly(t *testing.T) {
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS",
			AwayTeam: "STK",
		},
		HomeWinProb: 0.38,
		Score: ScoreProjection{
			PredictedWinner: "STK", Margin: 8, TotalScore: 156,
		},
		MarketOdds: map[TeamID]MarketOdds{
			"STK": {DecimalOdds: 1.26},
		},
		TotalsLine: &TotalsOdds{Line: 168.5, Bookmaker: "sportsbet"},
	}
	hc := BuildHighConfidenceBets(report)
	if len(hc.MatchBets) == 0 {
		t.Fatal("expected model high-confidence match bets")
	}
	if !strings.Contains(hc.MatchBets[0].Label, "St Kilda Head-to-Head") {
		t.Fatalf("first bet = %q", hc.MatchBets[0].Label)
	}
	if hc.MatchBets[0].Source != "model" {
		t.Fatalf("source = %q, want model", hc.MatchBets[0].Source)
	}
}

func TestBuildHighConfidenceBetsCuratedPlusModelTotals(t *testing.T) {
	odds := 1.20
	report := MatchReport{
		Context: MatchDayContext{HomeTeam: "ESS", AwayTeam: "STK"},
		HomeWinProb: 0.38,
		Score: ScoreProjection{
			PredictedWinner: "STK", Margin: 8, TotalScore: 156,
		},
		MarketOdds: map[TeamID]MarketOdds{"STK": {DecimalOdds: 1.20}},
		TotalsLine: &TotalsOdds{Line: 168.5, Bookmaker: "sportsbet"},
		Editorial: &FixtureEditorial{
			SmartPicks: []EditorialPick{
				{Type: "h2h", Label: "St Kilda Head-to-Head", Odds: &odds, Why: "10-game losing streak"},
				{Type: "margin", Label: "St Kilda 1-39 Margin", Why: "Fatigue risk"},
			},
		},
	}
	hc := BuildHighConfidenceBets(report)
	if len(hc.MatchBets) < 3 {
		t.Fatalf("expected curated h2h+margin plus model UNDER, got %d: %+v", len(hc.MatchBets), hc.MatchBets)
	}
	foundUnder := false
	for _, b := range hc.MatchBets {
		if b.Type == "totals" && b.Source == "model" {
			foundUnder = true
		}
		if b.Type == "h2h" && b.Source != "curated" {
			t.Fatalf("duplicate h2h from model: %+v", b)
		}
	}
	if !foundUnder {
		t.Fatalf("expected model UNDER totals bet in %+v", hc.MatchBets)
	}
}

func TestFormatRoundReportEmailESSSTKHighConfidence(t *testing.T) {
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
			Breakdown: ScoreBreakdown{
				RawMargin: -8.4, ProfileEdge: -0.25, WinEdge: -0.23,
			},
		},
		MarketOdds: map[TeamID]MarketOdds{
			"STK": {DecimalOdds: 1.20},
		},
		TotalsLine: &TotalsOdds{Line: 168.5, Bookmaker: "sportsbet"},
		Editorial: &FixtureEditorial{
			MatchLabel: "Match 1",
			Home:       "ESS",
			Away:       "STK",
			SmartPicks: []EditorialPick{
				{Type: "h2h", Label: "St Kilda Head-to-Head", Odds: &odds, Why: "10-game losing streak"},
				{Type: "margin", Label: "St Kilda 1-39 Margin", Why: "Fatigue factor ~49%"},
			},
			PlayerProps: []PlayerPropPick{{
				Player: "Nasiah Wanganeen-Milera",
				Team:   "STK",
				Market: "25+ Disposals",
				Why:    "Coming off a 44-disposal game.",
			}},
		},
	}
	body := FormatRoundReportEmail([]MatchReport{report}, nil)

	// Model analysis sections must remain
	for _, want := range []string{
		"PREDICTION",
		"WHY",
		"Model favours STK",
		"CONTEXT",
		"MODEL DETAIL",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing model section %q in body:\n%s", want, body)
		}
	}

	// High-confidence bets come after model analysis
	predIdx := strings.Index(body, "PREDICTION")
	hcIdx := strings.Index(body, "HIGH CONFIDENCE BETS")
	if hcIdx <= predIdx {
		t.Fatalf("high confidence bets should follow prediction:\n%s", body)
	}

	for _, want := range []string{
		"additional to model prediction above",
		"Match 1: Essendon vs St Kilda",
		"St Kilda Head-to-Head ($1.20) [curated]",
		"St Kilda 1-39 Margin [curated]",
		"Player props (Same Game Multi)",
		"Nasiah Wanganeen-Milera 25+ Disposals",
		"44-disposal game",
		"losing streak",
		"margin market may suit",
		"Key outs: Jack Sinclair",
		"UNDER 168.5 [model]",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}

	// Editorial notes should not pollute WHY
	whySection := body[strings.Index(body, "WHY\n"):strings.Index(body, "CONTEXT")]
	if strings.Contains(whySection, "Fatigue factor ~49%") {
		t.Fatalf("editorial note leaked into WHY:\n%s", whySection)
	}
}

func floatPtr(v float64) *float64 { return &v }
