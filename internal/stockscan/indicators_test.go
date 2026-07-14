package stockscan

import (
	"math"
	"strings"
	"testing"

	"github.com/ym/fxtrade/internal/config"
)

func TestSMA(t *testing.T) {
	tests := []struct {
		name   string
		closes []float64
		period int
		want   float64
		wantOK bool
	}{
		{
			name:   "exact period",
			closes: []float64{10, 20, 30, 40, 50},
			period: 5,
			want:   30,
			wantOK: true,
		},
		{
			name:   "last three of five",
			closes: []float64{10, 20, 30, 40, 50},
			period: 3,
			want:   40,
			wantOK: true,
		},
		{
			name:   "insufficient data",
			closes: []float64{10, 20},
			period: 5,
			wantOK: false,
		},
		{
			name:   "zero period",
			closes: []float64{10, 20, 30},
			period: 0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SMA(tt.closes, tt.period)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("SMA = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRSI(t *testing.T) {
	tests := []struct {
		name    string
		closes  []float64
		period  int
		wantOK  bool
		minWant float64
		maxWant float64
	}{
		{
			name:    "steady gains",
			closes:  []float64{44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84, 46.08, 45.89, 46.03, 45.61, 46.28, 46.28, 46.00},
			period:  14,
			wantOK:  true,
			minWant: 50,
			maxWant: 100,
		},
		{
			name:    "steady losses",
			closes:  []float64{46, 45.5, 45, 44.5, 44, 43.5, 43, 42.5, 42, 41.5, 41, 40.5, 40, 39.5, 39, 38.5},
			period:  14,
			wantOK:  true,
			minWant: 0,
			maxWant: 50,
		},
		{
			name:   "insufficient data",
			closes: []float64{10, 11, 12},
			period: 14,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RSI(tt.closes, tt.period)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got < tt.minWant || got > tt.maxWant {
				t.Fatalf("RSI = %v, want in [%v, %v]", got, tt.minWant, tt.maxWant)
			}
		})
	}
}

func TestPassesFilter(t *testing.T) {
	tests := []struct {
		name   string
		close  float64
		sma    float64
		rsi    float64
		rsiMin float64
		rsiMax float64
		want   bool
	}{
		{name: "pass", close: 110, sma: 100, rsi: 35, rsiMin: 30, rsiMax: 45, want: true},
		{name: "close below sma", close: 90, sma: 100, rsi: 35, rsiMin: 30, rsiMax: 45, want: false},
		{name: "close equals sma", close: 100, sma: 100, rsi: 35, rsiMin: 30, rsiMax: 45, want: false},
		{name: "rsi too low", close: 110, sma: 100, rsi: 29, rsiMin: 30, rsiMax: 45, want: false},
		{name: "rsi too high", close: 110, sma: 100, rsi: 46, rsiMin: 30, rsiMax: 45, want: false},
		{name: "rsi at lower bound", close: 110, sma: 100, rsi: 30, rsiMin: 30, rsiMax: 45, want: true},
		{name: "rsi at upper bound", close: 110, sma: 100, rsi: 45, rsiMin: 30, rsiMax: 45, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PassesFilter(tt.close, tt.sma, tt.rsi, tt.rsiMin, tt.rsiMax)
			if got != tt.want {
				t.Fatalf("PassesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRankByRSI(t *testing.T) {
	in := []Candidate{
		{Symbol: "B", RSI: 40},
		{Symbol: "A", RSI: 35},
		{Symbol: "C", RSI: 35},
	}
	ranked := RankByRSI(in)
	if len(ranked) != 3 {
		t.Fatalf("len = %d", len(ranked))
	}
	if ranked[0].Symbol != "A" || ranked[1].Symbol != "C" || ranked[2].Symbol != "B" {
		t.Fatalf("unexpected order: %+v", ranked)
	}
}

func TestLevels(t *testing.T) {
	sl, tgt := Levels(1000, 0.02, 0.03)
	if math.Abs(sl-980) > 1e-9 {
		t.Fatalf("stop loss = %v", sl)
	}
	if math.Abs(tgt-1030) > 1e-9 {
		t.Fatalf("target = %v", tgt)
	}
}

func TestFormatAlertEmail(t *testing.T) {
	cfg := config.StockScanConfig{SMAPeriod: 50, RSIMin: 30, RSIMax: 45}
	candidate := Candidate{Symbol: "TATAMOTORS", Name: "Tata Motors Limited", Close: 910, SMA: 880, RSI: 32.1}
	reasons := BuildReasons(candidate, cfg, "Positive Sentiment (confidence 82%) — strong outlook")
	contenders := []Contender{
		{Rank: 1, Candidate: candidate, OneLiner: BuildOneLiner(candidate, cfg), Selected: true},
		{Rank: 2, Candidate: Candidate{Symbol: "RELIANCE", Name: "Reliance Industries Limited", Close: 2500, SMA: 2450, RSI: 38.5}, OneLiner: BuildOneLiner(Candidate{Symbol: "RELIANCE", Close: 2500, SMA: 2450, RSI: 38.5}, cfg)},
	}
	body := FormatAlertEmail(Pick{
		Candidate:   candidate,
		Entry:       910,
		StopLoss:    891.80,
		Target:      937.30,
		StopLossPct: 0.02,
		TargetPct:   0.03,
		Reasons:     reasons,
	}, contenders, 2)
	want := []string{
		"NiftyPulse daily picks: 1 suggestion(s)",
		"Instrument: TATAMOTORS — Tata Motors Limited (Cash Equity Stock)",
		"Action: BUY",
		"Limit Price: ₹910.00",
		"Stop Loss: ₹891.80 (Strict 2% protection)",
		"Target: ₹937.30 (Strict 3% profit goal)",
		"Why this pick:",
		"above SMA(50)",
		"pullback zone [30–45]",
		"Sentiment: Positive Sentiment (confidence 82%) — strong outlook",
		"Top contenders (2 passed filters):",
		"1. TATAMOTORS (Tata Motors Limited)",
		"★ selected",
		"2. RELIANCE (Reliance Industries Limited)",
		"Only 2 symbol(s) passed today's SMA/RSI filters.",
		"GTT OCO",
		"NiftyPulse does NOT place orders automatically",
	}
	for _, line := range want {
		if !strings.Contains(body, line) {
			t.Fatalf("missing %q in body:\n%s", line, body)
		}
	}
	if strings.Contains(body, "This bot") {
		t.Fatal("footer should say NiftyPulse, not This bot")
	}
}

func TestFormatPicksEmailDual(t *testing.T) {
	cfg := config.StockScanConfig{SMAPeriod: 50, RSIMin: 30, RSIMax: 45}
	p1 := BuildPickUniverse(
		Candidate{Symbol: "INFY", Name: "Infosys Limited", Close: 1500, SMA: 1450, RSI: 34},
		cfg,
		BuildReasons(Candidate{Symbol: "INFY", Close: 1500, SMA: 1450, RSI: 34}, cfg, ""),
		UniverseNifty200,
	)
	p2 := BuildPickUniverse(
		Candidate{Symbol: "AFFLE", Name: "Affle (India) Limited", Close: 1400, SMA: 1350, RSI: 36},
		cfg,
		BuildReasons(Candidate{Symbol: "AFFLE", Close: 1400, SMA: 1350, RSI: 36}, cfg, ""),
		UniverseNifty500Rest,
	)
	body := FormatPicksEmail([]Pick{p1, p2}, nil, nil)
	for _, want := range []string{
		"NiftyPulse daily picks: 2 suggestion(s)",
		"=== Nifty 200 ===",
		"Instrument: INFY — Infosys Limited (Cash Equity Stock)",
		"=== Nifty 500 (ex-200) ===",
		"Instrument: AFFLE — Affle (India) Limited (Cash Equity Stock)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
}

func TestFormatPicksSubjectDual(t *testing.T) {
	picks := []Pick{
		{Candidate: Candidate{Symbol: "INFY", Name: "Infosys Limited"}, Universe: UniverseNifty200, Entry: 1500, StopLoss: 1470, Target: 1545},
		{Candidate: Candidate{Symbol: "AFFLE", Name: "Affle (India) Limited"}, Universe: UniverseNifty500Rest, Entry: 1400, StopLoss: 1372, Target: 1442},
	}
	got := FormatPicksSubject("[NiftyPulse]", picks)
	want := "[NiftyPulse] BUY N200 INFY (Infosys Limited) · N500 AFFLE (Affle (India) Limited)"
	if got != want {
		t.Fatalf("subject = %q, want %q", got, want)
	}
}

func TestCandidateDisplayName(t *testing.T) {
	if got := (Candidate{Symbol: "infy"}).DisplayName(); got != "INFY" {
		t.Fatalf("got %q", got)
	}
	if got := (Candidate{Symbol: "INFY", Name: "Infosys Limited"}).DisplayName(); got != "INFY (Infosys Limited)" {
		t.Fatalf("got %q", got)
	}
}

func TestExcludeSymbols(t *testing.T) {
	got := ExcludeSymbols([]string{"AFFLE", "INFY", "RELIANCE", "KFINTECH"}, []string{"infy", "RELIANCE"})
	if len(got) != 2 || got[0] != "AFFLE" || got[1] != "KFINTECH" {
		t.Fatalf("got %#v, want [AFFLE KFINTECH]", got)
	}
}

func TestBuildReasons(t *testing.T) {
	cfg := config.StockScanConfig{SMAPeriod: 50, RSIMin: 30, RSIMax: 45}
	c := Candidate{Symbol: "INFY", Close: 1500, SMA: 1450, RSI: 35, H1Trend: "bullish"}
	reasons := BuildReasons(c, cfg, "")
	if len(reasons) != 3 {
		t.Fatalf("len(reasons) = %d, want 3", len(reasons))
	}
	if !strings.Contains(reasons[0], "3.4%") {
		t.Fatalf("expected pct above SMA in reason[0]: %q", reasons[0])
	}
	if !strings.Contains(reasons[1], "pullback zone") {
		t.Fatalf("expected pullback zone in reason[1]: %q", reasons[1])
	}
	if !strings.Contains(reasons[2], "H1 trend: bullish") {
		t.Fatalf("expected H1 trend in reason[2]: %q", reasons[2])
	}
}

func TestBuildContendersMarksSelected(t *testing.T) {
	cfg := config.StockScanConfig{SMAPeriod: 50, RSIMin: 30, RSIMax: 45}
	ranked := []Candidate{
		{Symbol: "A", Close: 100, SMA: 90, RSI: 32},
		{Symbol: "B", Close: 200, SMA: 190, RSI: 35},
	}
	got := BuildContenders(ranked, cfg, "B")
	if !got[1].Selected || got[0].Selected {
		t.Fatalf("unexpected selected flags: %+v", got)
	}
}

func TestFormatAlertSubject(t *testing.T) {
	p := Pick{
		Candidate: Candidate{Symbol: "reliance", Name: "Reliance Industries Limited"},
		Entry:     2500,
		StopLoss:  2462.50,
		Target:    2575,
	}
	got := FormatAlertSubject("[NiftyPulse]", p)
	want := "[NiftyPulse] BUY RELIANCE (Reliance Industries Limited) · Limit ₹2500.00 · SL ₹2462.50 · TGT ₹2575.00"
	if got != want {
		t.Fatalf("subject = %q, want %q", got, want)
	}
}

func TestFormatAlertSubjectEmptyPrefix(t *testing.T) {
	p := Pick{Candidate: Candidate{Symbol: "INFY"}, Entry: 1500, StopLoss: 1477.50, Target: 1545}
	got := FormatAlertSubject("", p)
	if !strings.HasPrefix(got, "[NiftyPulse] BUY INFY") {
		t.Fatalf("expected fallback prefix, got %q", got)
	}
}
