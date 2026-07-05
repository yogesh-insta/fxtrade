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
	raw := `{"match_details":{"late_changes":"Smith in","weather_impact":"calm"},"main_bet":{"market":"H2H","selection":"STK","confidence":"high","reasons":["a","b","c"]},"player_prop":{"player":"X","market":"25+ disp","reasons":["r1","r2"]},"risk_note":"none","disclaimer":"bet responsibly"}`
	resp, err := ParsePregameLLMResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.MainBet.Selection != "STK" {
		t.Fatalf("selection = %q", resp.MainBet.Selection)
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
	llm.MainBet.Market = "H2H"
	llm.MainBet.Selection = "St Kilda"
	llm.MainBet.Confidence = "high"
	llm.MainBet.Reasons = []string{"form", "lineups", "odds"}

	body := FormatPregameEmail(game, report, llm)
	if !strings.Contains(body, "T-30") {
		t.Fatal("missing T-30 header")
	}
	if !strings.Contains(body, "St Kilda") {
		t.Fatal("missing main bet")
	}
	if !strings.Contains(body, "Squiggle game ID: 99") {
		t.Fatal("missing squiggle id")
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
