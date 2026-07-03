package strategy

import (
	"testing"

	"github.com/ym/fxtrade/internal/oanda"
)

func TestAccountExposureBlocked(t *testing.T) {
	open := []oanda.Trade{{ID: "1", Instrument: "EUR_USD"}}
	pending := []oanda.PendingOrder{{Instrument: "EUR_USD", Units: "100"}}
	blocked, reason := accountExposureBlocked(open, nil, "AUD_USD", 1)
	if !blocked || reason == "" {
		t.Fatalf("expected block for other open trade, got blocked=%v reason=%q", blocked, reason)
	}
	blocked, reason = accountExposureBlocked(nil, pending, "AUD_USD", 1)
	if !blocked || reason == "" {
		t.Fatalf("expected block for other pending, got blocked=%v reason=%q", blocked, reason)
	}
	blocked, _ = accountExposureBlocked(nil, pending, "EUR_USD", 1)
	if blocked {
		t.Fatal("should not block instrument that owns the pending order")
	}
}

func TestClassifyPendingPerInstrument(t *testing.T) {
	orders := []oanda.PendingOrder{
		{Instrument: "AUD_USD", Units: "100"},
		{Instrument: "EUR_USD", Units: "-100"},
	}
	hasBuy, hasSell := classifyPending(orders, "AUD_USD")
	if !hasBuy || hasSell {
		t.Fatalf("AUD_USD: got buy=%v sell=%v, want buy only", hasBuy, hasSell)
	}
	hasBuy, hasSell = classifyPending(orders, "EUR_USD")
	if hasBuy || !hasSell {
		t.Fatalf("EUR_USD: got buy=%v sell=%v, want sell only", hasBuy, hasSell)
	}
}

func TestPendingDirectionsElsewhere(t *testing.T) {
	orders := []oanda.PendingOrder{
		{Instrument: "AUD_USD", Units: "100"},
		{Instrument: "EUR_USD", Units: "-100"},
	}
	long, short := pendingDirectionsElsewhere(orders, "AUD_USD")
	if long || !short {
		t.Fatalf("from AUD_USD view: got long=%v short=%v, want short only", long, short)
	}
	long, short = pendingDirectionsElsewhere(orders, "EUR_USD")
	if !long || short {
		t.Fatalf("from EUR_USD view: got long=%v short=%v, want long only", long, short)
	}
}

func TestSplitTradesByInstrument(t *testing.T) {
	trades := []oanda.Trade{
		{ID: "1", Instrument: "AUD_USD"},
		{ID: "2", Instrument: "EUR_USD"},
	}
	own, other := splitTradesByInstrument(trades, "AUD_USD")
	if len(own) != 1 || own[0].ID != "1" {
		t.Fatalf("own = %+v, want trade 1", own)
	}
	if len(other) != 1 || other[0].ID != "2" {
		t.Fatalf("other = %+v, want trade 2", other)
	}
}
