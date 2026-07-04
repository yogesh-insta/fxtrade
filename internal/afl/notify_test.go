package afl

import (
	"strings"
	"testing"
	"time"
)

func TestFormatAlertEmail(t *testing.T) {
	bet := ValueBet{
		HomeTeam:    "BRI",
		AwayTeam:    "COLL",
		Team:        "BRI",
		Bookmaker:   "sportsbet",
		DecimalOdds: 2.10,
		ModelProb:   0.58,
		ImpliedProb: 0.476,
		EV:          0.218,
		Kickoff:     time.Date(2025, 4, 17, 19, 30, 0, 0, time.UTC),
		IsHomePick:  true,
		Reasons:     []string{"Model edge 10.4%"},
	}
	body := FormatAlertEmail(bet, []ValueBet{bet})
	for _, want := range []string{
		"BRI vs COLL",
		"Why this bet:",
		"Manual execution required",
		"AFLPulse does NOT place bets",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
}

func TestFormatRoundReportEmailRichCarl(t *testing.T) {
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "RICH",
			AwayTeam: "CARL",
			Venue:    VenueProfile{Name: "Melbourne Cricket Ground", Dimension: VenueWide},
			Weather:  WeatherMetrics{RainMM: 0, WindKPH: 12},
			Kickoff:  time.Date(2025, 4, 17, 19, 30, 0, 0, time.UTC),
		},
		HomeWinProb: 0.58,
		Score: ScoreProjection{
			HomeScore: 92, AwayScore: 78, TotalScore: 170, Margin: 14,
			PredictedWinner: "RICH", HomeWinProb: 0.58,
		},
		TotalsLine: &TotalsOdds{Line: 168.5, Bookmaker: "sportsbet"},
		MarketOdds: map[TeamID]MarketOdds{
			"RICH": {Team: "RICH", DecimalOdds: 1.55, Bookmaker: "sportsbet"},
		},
		ValueBets: []ValueBet{{
			HomeTeam: "RICH", AwayTeam: "CARL", Team: "RICH",
			Bookmaker: "sportsbet", DecimalOdds: 1.95, EV: 0.08, IsHomePick: true,
		}},
	}
	body := FormatRoundReportEmail([]MatchReport{report}, report.ValueBets)
	for _, want := range []string{
		"AFLPulse Round Scan",
		"RICH vs CARL",
		"Fri 18 Apr ·",
		"MCG",
		"PREDICTION",
		"Winner:     Richmond (58%)",
		"Score:      Richmond 92 – Carlton 78  (margin 14)",
		"Total:      170 points",
		"VS BOOKMAKER",
		"H2H: RICH",
		"Total: line 168.5 · model 170",
		"Lean: NEAR",
		"★ VALUE BET: Richmond",
		"VALUE BETS (1)",
		"Manual execution required",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
}

func TestMatrixPredictor(t *testing.T) {
	coeffs := make([]float64, FeatureCount)
	for i := range coeffs {
		coeffs[i] = 0.05
	}
	p, err := NewMatrixPredictorFromModel(MatrixModel{Bias: 0, Coefficients: coeffs})
	if err != nil {
		t.Fatal(err)
	}
	fv := FeatureVector{Values: make([]float64, FeatureCount)}
	for i := range fv.Values {
		fv.Values[i] = 0.5
	}
	prob, err := p.Predict(fv)
	if err != nil {
		t.Fatal(err)
	}
	if prob <= 0 || prob >= 1 {
		t.Fatalf("expected probability in (0,1), got %.4f", prob)
	}
}

func TestFormatKickoffMelbourne(t *testing.T) {
	if melbourneLoc == nil {
		t.Skip("Australia/Melbourne timezone unavailable")
	}
	kick := time.Date(2025, 7, 5, 5, 15, 0, 0, time.UTC)
	got := formatKickoffMelbourne(kick)
	if !strings.Contains(got, "Jul") || !strings.Contains(got, "3:15 PM") {
		t.Fatalf("unexpected kickoff format: %q", got)
	}
	short := formatKickoffMelbourneShort(kick)
	if !strings.Contains(short, "Sat 5 Jul") || !strings.Contains(short, "3:15 PM") {
		t.Fatalf("unexpected short kickoff format: %q", short)
	}
}

func TestWrapIndented(t *testing.T) {
	text := "Model favours STK to win (62%) — away side rated 62% despite playing at Melbourne Cricket Ground"
	got := wrapIndented(bulletPrefix, text, 40, bulletContIndent)
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 40 {
			t.Fatalf("line exceeds width 40: len=%d %q", len(line), line)
		}
	}
	if !strings.HasPrefix(got, bulletPrefix) {
		t.Fatalf("expected bullet prefix on first line: %q", got)
	}
	if !strings.Contains(got, "\n"+bulletContIndent) {
		t.Fatalf("expected continuation indent: %q", got)
	}
}

func TestWriteLastFiveSectionMobileLayout(t *testing.T) {
	var b strings.Builder
	writeLastFiveSection(&b, MatchDayContext{
		HomeTeam: "ESS",
		AwayTeam: "STK",
		Venue:    VenueProfile{Name: "Melbourne Cricket Ground"},
		HomeStats: TeamStats{
			Last5Scores: []RecentMatchScore{
				{Opponent: "RICH", For: 56, Against: 74, Venue: "M.C.G.", Total: 130},
				{Opponent: "WCE", For: 55, Against: 85, Venue: "Optus Stadium", Total: 140},
			},
		},
		AwayStats: TeamStats{
			Last5Scores: []RecentMatchScore{
				{Opponent: "CARL", For: 80, Against: 70, Venue: "M.C.G.", Total: 150},
			},
		},
	})
	body := b.String()
	for _, want := range []string{
		"LAST 5 GAMES",
		"— ESS",
		"56-74 vs RICH @ MCG · total 130 · L",
		"55-85 vs WCE @ Optus · total 140 · L",
		"At this venue",
		"— STK",
		"80-70 vs CARL @ MCG · total 150 · W",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, " | ") {
		t.Fatalf("expected one game per line, found pipe separator:\n%s", body)
	}
}

func TestWriteBookmakerSectionMobileLayout(t *testing.T) {
	var b strings.Builder
	writeBookmakerSection(&b, MatchReport{
		HomeWinProb: 0.38,
		Score: ScoreProjection{
			TotalScore: 155, PredictedWinner: "STK",
		},
		MarketOdds: map[TeamID]MarketOdds{
			"STK": {DecimalOdds: 1.26},
		},
		TotalsLine: &TotalsOdds{Line: 174.5, Bookmaker: "sportsbet"},
	})
	body := b.String()
	for _, want := range []string{
		"VS BOOKMAKER",
		"H2H: STK $1.26 (~79% market)",
		"Model: less bullish than market",
		"Total: line 174.5 · model 155",
		"Lean: UNDER (Δ -19.5)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}
