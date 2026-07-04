package stockscan

import (
	"math"
	"strings"
	"testing"
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
	sl, tgt := Levels(1000, 0.015, 0.03)
	if math.Abs(sl-985) > 1e-9 {
		t.Fatalf("stop loss = %v", sl)
	}
	if math.Abs(tgt-1030) > 1e-9 {
		t.Fatalf("target = %v", tgt)
	}
}

func TestFormatAlertEmail(t *testing.T) {
	body := FormatAlertEmail(Pick{
		Candidate: Candidate{Symbol: "TATAMOTORS"},
		Entry:     910,
		StopLoss:  896.35,
		Target:    937.30,
	})
	want := []string{
		"Instrument: TATAMOTORS (Cash Equity Stock)",
		"Action: BUY",
		"Limit Price: ₹910.00",
		"Stop Loss: ₹896.35 (Strict 1.5% protection)",
		"Target: ₹937.30 (Strict 3% profit goal)",
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

func TestFormatAlertSubject(t *testing.T) {
	p := Pick{
		Candidate: Candidate{Symbol: "reliance"},
		Entry:     2500,
		StopLoss:  2462.50,
		Target:    2575,
	}
	got := FormatAlertSubject("[NiftyPulse]", p)
	want := "[NiftyPulse] BUY RELIANCE · Limit ₹2500.00 · SL ₹2462.50 · TGT ₹2575.00"
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
