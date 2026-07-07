package notify

import (
	"strings"
	"testing"
)

func TestFormatTradeOpen(t *testing.T) {
	sl := 1.33810
	tp := 1.34010
	body := FormatTradeOpen(TradeOpen{
		Instrument: "GBP_USD",
		Direction:  "LONG",
		Units:      62500,
		FillPrice:  1.33890,
		StopLoss:   &sl,
		TakeProfit: &tp,
		TradeID:    "110",
	})
	for _, want := range []string{
		"GBP_USD",
		"LONG",
		"62500",
		"1.33890",
		"Notional:",
		"Stop loss:",
		"Take profit:",
		"Trade ID:    110",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestFormatTradeCloseProfit(t *testing.T) {
	body := FormatTradeClose(TradeClose{
		Instrument: "EUR_USD",
		Direction:  "SHORT",
		Units:      10000,
		EntryPrice: 1.08500,
		ExitPrice:  1.08400,
		RealizedPL: 10.50,
		PLKnown:    true,
		Reason:     "take profit",
		TradeID:    "96",
	})
	if !strings.Contains(body, "PROFIT +$10.50") {
		t.Fatalf("expected profit line, got:\n%s", body)
	}
}

func TestFormatTradeCloseUnknownPL(t *testing.T) {
	body := FormatTradeClose(TradeClose{
		Instrument: "EUR_USD",
		TradeID:    "96",
		PLKnown:    false,
		Reason:     "closed externally (stop loss / take profit / manual)",
	})
	if !strings.Contains(body, "P/L unknown") {
		t.Fatalf("expected unknown P/L, got:\n%s", body)
	}
}

func TestTradeCloseSubject(t *testing.T) {
	if got := TradeCloseSubject("GBP_USD", -5.85, true, false); got != "fxtrade: CLOSED GBP_USD (-$5.85)" {
		t.Fatalf("unexpected subject: %s", got)
	}
}
