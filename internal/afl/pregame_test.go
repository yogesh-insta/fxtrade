package afl

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildPregameLLMPayloadCompactJSON(t *testing.T) {
	game := SquiggleFixture{
		ID: 42, HomeTeam: "Essendon", AwayTeam: "St Kilda", Round: 17, Venue: "M.C.G.",
		Kickoff: time.Date(2026, 7, 5, 8, 20, 0, 0, time.UTC),
	}
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS", AwayTeam: "STK",
			Kickoff: game.Kickoff,
			HomeStats: TeamStats{FormWinsLast10: 3, FormLossesLast10: 7},
			AwayStats: TeamStats{FormWinsLast10: 6, FormLossesLast10: 4},
		},
		Score: ScoreProjection{
			PredictedWinner: "STK",
			HomeScore:       78, AwayScore: 92, Margin: 14, TotalScore: 170,
		},
	}
	data, err := BuildPregameLLMPayload(game, report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Analyze the upcoming") {
		t.Fatal("payload should be JSON not prose")
	}
	var p PregameLLMPayload
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.SquiggleID != 42 || p.HomeTeam != "ESS" || p.Model.Winner != "STK" {
		t.Fatalf("payload = %+v", p)
	}
}

func TestParsePregameLLMResponse(t *testing.T) {
	raw := `{"match_details":{"late_changes":"Smith in","weather_impact":"calm","lineups":"confirmed"},"main_bet":{"market":"H2H","selection":"STK","confidence":"high","odds_note":"STK $1.65","reasons":["a","b","c"]},"player_prop":{"player":"X","market":"25+ disp","reasons":["r1","r2"]},"risk_note":"none","model_agreement":"aligns","disclaimer":"bet responsibly"}`
	resp, err := ParsePregameLLMResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.MainBet.Selection != "STK" || resp.MainBet.OddsNote != "STK $1.65" {
		t.Fatalf("selection/odds = %q / %q", resp.MainBet.Selection, resp.MainBet.OddsNote)
	}
	if resp.ModelAgreement != "aligns" || resp.MatchDetails.Lineups != "confirmed" {
		t.Fatalf("agreement/lineups = %q / %q", resp.ModelAgreement, resp.MatchDetails.Lineups)
	}

	fenced := "```json\n" + raw + "\n```"
	resp2, err := ParsePregameLLMResponse(fenced)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.MainBet.Selection != "STK" {
		t.Fatal("fence strip failed")
	}
}

func TestParsePregameLLMResponseAlternateKeys(t *testing.T) {
	raw := `{
  "mainBet": {
    "pick": "STK H2H",
    "type": "H2H",
    "conf": "medium",
    "odds": "STK $1.70",
    "why": ["form edge", "venue"]
  },
  "modelAgreement": "mixed",
  "risks": "injury cloud",
  "matchDetails": {"lineupStatus": "unconfirmed — late team news pending"}
}`
	resp, err := ParsePregameLLMResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.MainBet.Selection != "STK H2H" || resp.MainBet.Market != "H2H" {
		t.Fatalf("main bet = %+v", resp.MainBet)
	}
	if len(resp.MainBet.Reasons) != 2 {
		t.Fatalf("reasons = %#v", resp.MainBet.Reasons)
	}
	if resp.ModelAgreement != "mixed" || resp.RiskNote != "injury cloud" {
		t.Fatalf("agreement/risk = %q / %q", resp.ModelAgreement, resp.RiskNote)
	}
	if resp.MatchDetails.Lineups == "" {
		t.Fatal("expected lineups from alternate key")
	}
}

func TestParsePregameLLMResponsePartialAndProse(t *testing.T) {
	// Empty main_bet is accepted as partial, not an error.
	partial := `{"match_details":{"weather_impact":"light wind"},"main_bet":{}}`
	resp, err := ParsePregameLLMResponse(partial)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Incomplete() || resp.IsEmpty() {
		t.Fatalf("expected incomplete non-empty partial: incomplete=%v empty=%v", resp.Incomplete(), resp.IsEmpty())
	}

	prose := "Here is the analysis:\n```json\n" + partial + "\n```\nGood luck."
	resp2, err := ParsePregameLLMResponse(prose)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.MatchDetails.WeatherImpact != "light wind" {
		t.Fatalf("weather = %q", resp2.MatchDetails.WeatherImpact)
	}
}

func TestFormatPregameEmail(t *testing.T) {
	game := SquiggleFixture{ID: 99, Venue: "M.C.G."}
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS", AwayTeam: "STK",
			Kickoff: time.Date(2026, 7, 5, 8, 20, 0, 0, time.UTC),
			Venue:   VenueProfile{Name: "Melbourne Cricket Ground"},
		},
		Score: ScoreProjection{PredictedWinner: "STK", HomeScore: 80, AwayScore: 90, Margin: 10, TotalScore: 170},
	}
	llm := PregameLLMResponse{}
	llm.MatchDetails.LateChanges = "No late changes"
	llm.MatchDetails.Lineups = "confirmed"
	llm.MainBet.Market = "H2H"
	llm.MainBet.Selection = "STK"
	llm.MainBet.Confidence = "high"
	llm.MainBet.OddsNote = "STK $1.65"
	llm.MainBet.Reasons = []string{"form", "lineups", "odds"}
	llm.PlayerProp.Player = "Nasiah Wanganeen-Milera"
	llm.PlayerProp.Market = "25+ disposals"
	llm.RiskNote = "Essendon midfield rotation"
	llm.ModelAgreement = "aligns"

	body := FormatPregameEmail(game, report, llm)
	for _, want := range []string{
		"T-30",
		"MODEL BASELINE",
		"LIVE ANALYTICS (Gemini)",
		"Main bet: STK H2H (high)",
		"Odds note: STK $1.65",
		"Why:",
		"Player prop: Nasiah Wanganeen-Milera — 25+ disposals",
		"Risks: Essendon midfield rotation",
		"Model agreement: aligns",
		"Lineups: confirmed",
		"Squiggle game ID: 99",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
	if strings.Contains(body, "[]") {
		t.Fatalf("must not print empty slice: %s", body)
	}
}

func TestFormatPregameEmailEmptyLLM(t *testing.T) {
	game := SquiggleFixture{ID: 38629}
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS", AwayTeam: "STK",
			Kickoff: time.Date(2026, 7, 5, 5, 15, 0, 0, time.UTC),
			Venue:   VenueProfile{Name: "Docklands"},
		},
		Score: ScoreProjection{PredictedWinner: "STK", HomeScore: 79, AwayScore: 88, Margin: 9, TotalScore: 167},
	}
	body := FormatPregameEmail(game, report, PregameLLMResponse{})
	if strings.Contains(body, "[]") {
		t.Fatalf("must not print [] for empty LLM:\n%s", body)
	}
	if !strings.Contains(body, "Gemini returned incomplete analysis") {
		t.Fatal("expected incomplete note")
	}
	if !strings.Contains(body, "Live analytics: unavailable") {
		t.Fatal("expected unavailable line")
	}
	if !strings.Contains(body, "MODEL BASELINE") || !strings.Contains(body, "St Kilda") {
		t.Fatalf("model baseline missing:\n%s", body)
	}
	if strings.Contains(body, "Main bet (medium confidence):") {
		t.Fatal("old empty main-bet format must not appear")
	}
}

func TestFormatPregameEmailPartialLLM(t *testing.T) {
	game := SquiggleFixture{ID: 1}
	report := MatchReport{
		Context: MatchDayContext{HomeTeam: "PORT", AwayTeam: "NMFC"},
		Score:   ScoreProjection{PredictedWinner: "PORT", HomeScore: 90, AwayScore: 80, Margin: 10, TotalScore: 170},
	}
	llm := PregameLLMResponse{}
	llm.MatchDetails.WeatherImpact = "calm, no scoring impact"
	llm.RiskNote = "key defender in doubt"
	body := FormatPregameEmail(game, report, llm)
	if strings.Contains(body, "[]") {
		t.Fatal("must not print []")
	}
	if !strings.Contains(body, "Gemini returned incomplete analysis") {
		t.Fatal("expected incomplete note for partial")
	}
	if !strings.Contains(body, "Main bet: unavailable") {
		t.Fatal("expected unavailable main bet")
	}
	if !strings.Contains(body, "Weather: calm") || !strings.Contains(body, "Risks: key defender") {
		t.Fatalf("partial fields missing:\n%s", body)
	}
}

func TestFormatPregameFailureAlert(t *testing.T) {
	game := SquiggleFixture{
		ID: 55, Kickoff: time.Date(2026, 7, 5, 8, 20, 0, 0, time.UTC), Venue: "Docklands",
	}
	body := FormatPregameFailureAlert(game, "ESS", "STK", "Docklands", errTest("timeout"), "/opt/fxtrade/logs/afl-pulse-pregame.log")
	if !strings.Contains(body, "Gemini failed") {
		t.Fatal("missing failure headline")
	}
	if !strings.Contains(body, "Squiggle ID: 55") {
		t.Fatal("missing squiggle id")
	}
	if !strings.Contains(body, "timeout") {
		t.Fatal("missing error")
	}
	if !strings.Contains(body, "afl-pulse-pregame.log") {
		t.Fatal("missing log path")
	}
	if !strings.Contains(body, "Model-baseline pregame email was still sent") {
		t.Fatal("expected note that baseline email was sent")
	}
}

func TestFormatPregameFailureSubject(t *testing.T) {
	subj := FormatPregameFailureSubject("[AFLPulse PRE]", "ESS", "STK")
	if subj != "[AFLPulse PRE] ALERT · Gemini failed · ESS vs STK" {
		t.Fatalf("subject = %q", subj)
	}
}

func TestMatchSquiggleToOdds(t *testing.T) {
	kick := time.Date(2026, 7, 5, 8, 20, 0, 0, time.UTC)
	game := SquiggleFixture{
		ID: 1, HomeTeam: "Essendon", AwayTeam: "St Kilda", Kickoff: kick,
	}
	h2h := []MarketOdds{
		{EventID: "ev1", HomeTeam: "ESS", AwayTeam: "STK", Kickoff: kick, DecimalOdds: 2.1, Bookmaker: "sportsbet"},
		{EventID: "ev1", HomeTeam: "ESS", AwayTeam: "STK", Kickoff: kick, DecimalOdds: 2.0, Bookmaker: "tab"},
	}
	totals := []TotalsOdds{
		{EventID: "ev1", HomeTeam: "ESS", AwayTeam: "STK", Line: 168.5},
	}
	resolve := func(name string) (TeamID, error) {
		switch name {
		case "Essendon":
			return "ESS", nil
		case "St Kilda":
			return "STK", nil
		default:
			return "", errTest("unknown")
		}
	}
	fixture, matchedH2H, matchedTotals, ok := MatchSquiggleToOdds(game, h2h, totals, resolve)
	if !ok || fixture.EventID != "ev1" || len(matchedH2H) != 2 || len(matchedTotals) != 1 {
		t.Fatalf("match failed: ok=%v fixture=%+v h2h=%d totals=%d", ok, fixture, len(matchedH2H), len(matchedTotals))
	}
}

func TestPregameStateDedup(t *testing.T) {
	st := &PregameState{Sent: make(map[string]time.Time), LLMAttempted: make(map[string]time.Time)}
	if st.WasSent(7) {
		t.Fatal("should not be sent initially")
	}
	st.MarkSent(7)
	if !st.WasSent(7) {
		t.Fatal("should be sent after mark")
	}
	if st.WasLLMAttempted(7) {
		t.Fatal("should not show LLM attempted until marked")
	}
	st.MarkLLMAttempted(7)
	if !st.WasLLMAttempted(7) {
		t.Fatal("should show LLM attempted after mark")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
